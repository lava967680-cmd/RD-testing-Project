package integration_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/controlhub/controlhub/services/api/internal/handlers"
	"github.com/controlhub/controlhub/services/api/internal/middleware"
	"github.com/controlhub/controlhub/services/api/internal/models"
	"github.com/controlhub/controlhub/services/api/internal/store"
	"github.com/google/uuid"
)

func setupEnterpriseTestApp() (*store.MemoryStore, http.Handler, string) {
	jwtSecret := "enterprise_test_secret_32_bytes_long_key_112233"
	st := store.NewMemoryStore()
	authMw := middleware.NewAuthMiddleware(jwtSecret)
	authH := handlers.NewAuthHandler(st, jwtSecret)
	jobH := handlers.NewJobHandler(st)
	opsH := handlers.NewOperationsHandler(st)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/auth/login", authH.Login)
	mux.HandleFunc("POST /api/v1/agent/commands/{commandId}/result", jobH.ReportResult)

	mux.Handle("POST /api/v1/devices/{deviceId}/commands", authMw.Authenticate(
		http.HandlerFunc(jobH.CreateCommand),
	))
	mux.Handle("GET /api/v1/commands/{commandId}", authMw.Authenticate(
		authMw.RequirePermission("device.view", jobH.GetCommand),
	))
	mux.Handle("POST /api/v1/bulk/commands", authMw.Authenticate(
		authMw.RequirePermission("device.restart", jobH.CreateBulkCommand),
	))

	mux.Handle("GET /api/v1/alerts", authMw.Authenticate(
		authMw.RequirePermission("alert.view", opsH.ListAlerts),
	))
	mux.Handle("POST /api/v1/alerts/{id}/acknowledge", authMw.Authenticate(
		authMw.RequirePermission("alert.manage", opsH.AcknowledgeAlert),
	))

	mux.Handle("GET /api/v1/software", authMw.Authenticate(
		authMw.RequirePermission("device.software_manage", opsH.ListSoftware),
	))
	mux.Handle("POST /api/v1/software", authMw.Authenticate(
		authMw.RequirePermission("device.software_manage", opsH.CreateSoftware),
	))
	mux.Handle("POST /api/v1/software/{id}/deploy", authMw.Authenticate(
		authMw.RequirePermission("device.software_manage", opsH.DeploySoftware),
	))

	mux.Handle("GET /api/v1/licensing/current", authMw.Authenticate(
		authMw.RequirePermission("license.view", opsH.GetCurrentLicense),
	))

	return st, mux, jwtSecret
}

func TestCommandQueueAndExecutionLifecycle(t *testing.T) {
	st, handler, _ := setupEnterpriseTestApp()
	adminToken := getAdminToken(t, handler)
	devID := uuid.MustParse("90e722c1-bb3e-4623-a128-44fb6b907a01")

	// 1. Dispatch controlled REBOOT command
	cmdReq := handlers.CreateCommandRequest{
		CommandType: "REBOOT",
		Parameters: map[string]interface{}{
			"delay_seconds": 60,
			"message":       "Maintenance restart",
		},
		ExpiresInSec: 300,
	}
	bBytes, _ := json.Marshal(cmdReq)
	req := httptest.NewRequest("POST", "/api/v1/devices/"+devID.String()+"/commands", bytes.NewReader(bBytes))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("Expected 202 Accepted on command queue, got %d. Body: %s", w.Code, w.Body.String())
	}

	var res handlers.CreateCommandResponse
	_ = json.NewDecoder(w.Body).Decode(&res)
	if res.Status != "QUEUED" || res.Nonce == "" {
		t.Fatalf("Expected QUEUED status and cryptographic nonce, got %+v", res)
	}

	// 2. Agent reports execution success
	agentReport := handlers.AgentCommandResultRequest{
		ExitCode:            0,
		StdoutOutput:        "Reboot initiated successfully in 60s",
		ExecutionDurationMs: 140,
	}
	aBytes, _ := json.Marshal(agentReport)
	aReq := httptest.NewRequest("POST", "/api/v1/agent/commands/"+res.CommandID.String()+"/result", bytes.NewReader(aBytes))
	aReq.Header.Set("Content-Type", "application/json")
	aRec := httptest.NewRecorder()
	handler.ServeHTTP(aRec, aReq)

	if aRec.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK on agent result report, got %d", aRec.Code)
	}

	// 3. Verify command status transitioned to SUCCESS
	cmd, err := st.GetCommand(req.Context(), uuid.MustParse("771e8bfb-cf98-4c28-98e3-0d6e6443c21a"), res.CommandID)
	if err != nil || cmd.Status != "SUCCESS" {
		t.Fatalf("Expected command status SUCCESS, got %s err=%v", cmd.Status, err)
	}
}

func TestSafeBulkActionConfirmationProtection(t *testing.T) {
	_, handler, _ := setupEnterpriseTestApp()
	adminToken := getAdminToken(t, handler)
	dev1ID := uuid.MustParse("90e722c1-bb3e-4623-a128-44fb6b907a01")
	dev2ID := uuid.MustParse("90e722c1-bb3e-4623-a128-44fb6b907a02")

	// 1. Unconfirmed bulk command attempt -> MUST FAIL
	badReq := handlers.BulkCommandRequest{
		DeviceIDs:    []uuid.UUID{dev1ID, dev2ID},
		CommandType:  "REBOOT",
		Confirmation: "WRONG_STRING",
	}
	bBytes, _ := json.Marshal(badReq)
	req := httptest.NewRequest("POST", "/api/v1/bulk/commands", bytes.NewReader(bBytes))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("Expected 400 Bad Request for missing bulk confirmation, got %d", w.Code)
	}

	// 2. Confirmed bulk command attempt -> Succeeded
	goodReq := handlers.BulkCommandRequest{
		DeviceIDs:    []uuid.UUID{dev1ID, dev2ID},
		CommandType:  "REBOOT",
		Confirmation: "CONFIRM_BULK_ACTION",
	}
	gBytes, _ := json.Marshal(goodReq)
	gReq := httptest.NewRequest("POST", "/api/v1/bulk/commands", bytes.NewReader(gBytes))
	gReq.Header.Set("Authorization", "Bearer "+adminToken)
	gReq.Header.Set("Content-Type", "application/json")
	gRec := httptest.NewRecorder()
	handler.ServeHTTP(gRec, gReq)

	if gRec.Code != http.StatusAccepted {
		t.Fatalf("Expected 202 Accepted on confirmed bulk command, got %d", gRec.Code)
	}

	var bRes handlers.BulkCommandResponse
	_ = json.NewDecoder(gRec.Body).Decode(&bRes)
	if bRes.TotalQueued != 2 {
		t.Fatalf("Expected 2 commands queued, got %d", bRes.TotalQueued)
	}
}

func TestAlertsAndLicensingWorkflow(t *testing.T) {
	_, handler, _ := setupEnterpriseTestApp()
	adminToken := getAdminToken(t, handler)

	// 1. Query seeded alert
	aReq := httptest.NewRequest("GET", "/api/v1/alerts", nil)
	aReq.Header.Set("Authorization", "Bearer "+adminToken)
	aRec := httptest.NewRecorder()
	handler.ServeHTTP(aRec, aReq)

	if aRec.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK on get alerts, got %d", aRec.Code)
	}

	var alerts []models.Alert
	_ = json.NewDecoder(aRec.Body).Decode(&alerts)
	if len(alerts) == 0 {
		t.Fatal("Expected seeded alert to be present")
	}

	// 2. Acknowledge alert
	ackReq := httptest.NewRequest("POST", "/api/v1/alerts/"+alerts[0].ID.String()+"/acknowledge", nil)
	ackReq.Header.Set("Authorization", "Bearer "+adminToken)
	ackRec := httptest.NewRecorder()
	handler.ServeHTTP(ackRec, ackReq)

	if ackRec.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK on acknowledge alert, got %d", ackRec.Code)
	}

	// 3. Query tenant licensing entitlements
	licReq := httptest.NewRequest("GET", "/api/v1/licensing/current", nil)
	licReq.Header.Set("Authorization", "Bearer "+adminToken)
	licRec := httptest.NewRecorder()
	handler.ServeHTTP(licRec, licReq)

	if licRec.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK on get license, got %d", licRec.Code)
	}

	var licPayload map[string]interface{}
	_ = json.NewDecoder(licRec.Body).Decode(&licPayload)
	if licPayload["device_limit"].(float64) != 50 {
		t.Fatalf("Expected 50 max devices in license, got %v", licPayload["device_limit"])
	}
}
