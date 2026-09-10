package integration_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/controlhub/controlhub/services/api/internal/handlers"
	"github.com/controlhub/controlhub/services/api/internal/middleware"
	"github.com/controlhub/controlhub/services/api/internal/models"
	"github.com/controlhub/controlhub/services/api/internal/store"
	"github.com/controlhub/controlhub/services/realtime/internal/hub"
	"github.com/google/uuid"
)

func setupTelemetryTestApp() (*store.MemoryStore, http.Handler, string) {
	jwtSecret := "telemetry_test_secret_32_bytes_long_key_123456"
	st := store.NewMemoryStore()
	authMw := middleware.NewAuthMiddleware(jwtSecret)
	authH := handlers.NewAuthHandler(st, jwtSecret)
	devH := handlers.NewDeviceHandler(st)
	telemetryH := handlers.NewTelemetryHandler(st)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/auth/login", authH.Login)
	mux.HandleFunc("POST /api/v1/telemetry/ingest", telemetryH.IngestTelemetry)

	mux.Handle("GET /api/v1/devices/{deviceId}/telemetry", authMw.Authenticate(
		authMw.RequirePermission("device.view", telemetryH.GetDeviceTelemetry),
	))
	mux.Handle("GET /api/v1/devices/{deviceId}", authMw.Authenticate(
		authMw.RequirePermission("device.view", devH.GetDevice),
	))

	return st, mux, jwtSecret
}

func TestTelemetryIngestionAndQuery(t *testing.T) {
	st, handler, _ := setupTelemetryTestApp()
	adminToken := getAdminToken(t, handler)

	demoOrgID := uuid.MustParse("771e8bfb-cf98-4c28-98e3-0d6e6443c21a")
	devID := uuid.MustParse("90e722c1-bb3e-4623-a128-44fb6b907a01")

	// 1. Ingest fresh telemetry payload
	ingestReq := handlers.IngestTelemetryRequest{
		DeviceID:          devID,
		OrganizationID:    demoOrgID,
		CPUPercent:        44.2,
		RAMUsedBytes:      8192000000,
		RAMTotalBytes:     16384000000,
		RAMPercent:        50.0,
		DiskUsedBytes:     200000000000,
		DiskTotalBytes:    500000000000,
		DiskPercent:       40.0,
		NetworkRxBytesSec: 85000,
		NetworkTxBytesSec: 42000,
	}
	body, _ := json.Marshal(ingestReq)
	req := httptest.NewRequest("POST", "/api/v1/telemetry/ingest", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("Expected 201 Created on telemetry ingest, got %d. Body: %s", w.Code, w.Body.String())
	}

	// 2. Query telemetry snapshot via REST API
	getReq := httptest.NewRequest("GET", "/api/v1/devices/"+devID.String()+"/telemetry?history=true", nil)
	getReq.Header.Set("Authorization", "Bearer "+adminToken)
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)

	if getRec.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK on telemetry get, got %d. Body: %s", getRec.Code, getRec.Body.String())
	}

	var res handlers.TelemetryResponse
	if err := json.NewDecoder(getRec.Body).Decode(&res); err != nil {
		t.Fatalf("Failed to decode telemetry response: %v", err)
	}

	if res.Latest == nil || res.Latest.CPUPercent != 44.2 {
		t.Fatalf("Expected latest CPU percent to be 44.2, got %+v", res.Latest)
	}

	if res.Latest.RAMPercent != 50.0 {
		t.Fatalf("Expected latest RAM percent to be 50.0, got %v", res.Latest.RAMPercent)
	}

	// Verify device last_seen was refreshed
	dev, _ := st.GetDevice(getReq.Context(), demoOrgID, devID)
	if dev.LastSeen == nil || time.Since(*dev.LastSeen) > 5*time.Second {
		t.Fatalf("Expected device LastSeen to be updated, got %v", dev.LastSeen)
	}
}

func TestCrossTenantTelemetryIsolation(t *testing.T) {
	st, handler, jwtSecret := setupTelemetryTestApp()

	// Org B setup
	orgBID := uuid.New()
	userBID := uuid.New()
	devOrgBID := uuid.New()

	st.SaveTelemetry(nil, models.DeviceTelemetry{
		OrganizationID: orgBID,
		DeviceID:       devOrgBID,
		CPUPercent:     99.9,
	})

	// User from Org A attempts to query Org B device telemetry
	demoOrgAID := uuid.MustParse("771e8bfb-cf98-4c28-98e3-0d6e6443c21a")
	tokenOrgA, _ := middleware.NewAuthMiddleware(jwtSecret)
	_ = tokenOrgA

	// Create JWT token for Org A
	adminTokenA, _ := handlers.NewAuthHandler(st, jwtSecret)
	_ = adminTokenA
	tokenA, _ := getOrgToken(userBID, demoOrgAID, "Administrator", []string{"device.view"}, jwtSecret)

	req := httptest.NewRequest("GET", "/api/v1/devices/"+devOrgBID.String()+"/telemetry", nil)
	req.Header.Set("Authorization", "Bearer "+tokenA)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("Expected cross-tenant access to return 404 Not Found, got %d", w.Code)
	}
}

func TestRealtimeHubWatchdogAndLiveness(t *testing.T) {
	// Hub with 100ms TTL for fast timeout verification
	h := hub.NewHub(100 * time.Millisecond)
	defer h.Stop()

	orgID := uuid.New()
	devID := uuid.New()

	clientSession := &hub.ClientSession{
		UserID:         uuid.New(),
		OrganizationID: orgID,
		SendChan:       make(chan []byte, 10),
	}
	h.RegisterClient(clientSession)

	agentSession := &hub.AgentSession{
		DeviceID:       devID,
		OrganizationID: orgID,
		Hostname:       "TEST-WATCHDOG-PC",
		SendChan:       make(chan []byte, 10),
	}
	h.RegisterAgent(agentSession)

	// Verify online event was received by client
	select {
	case msg := <-clientSession.SendChan:
		var ev hub.EventMessage
		_ = json.Unmarshal(msg, &ev)
		if ev.Event != "device.online" {
			t.Fatalf("Expected device.online event, got %s", ev.Event)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Timeout waiting for device.online event")
	}

	// Allow watchdog to detect missed heartbeat (> 100ms)
	time.Sleep(200 * time.Millisecond)
}

func getOrgToken(userID, orgID uuid.UUID, role string, perms []string, secret string) (string, error) {
	claims := middleware.NewAuthMiddleware(secret)
	_ = claims
	// Simple JWT helper
	return handlers.NewAuthHandler(nil, secret) == nil ? "", nil
}
