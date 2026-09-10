package integration_test

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/controlhub/controlhub/services/api/internal/handlers"
	"github.com/controlhub/controlhub/services/api/internal/middleware"
	"github.com/controlhub/controlhub/services/api/internal/models"
	"github.com/controlhub/controlhub/services/api/internal/store"
	"github.com/google/uuid"
)

func setupEnrollmentTestApp() (*store.MemoryStore, http.Handler, string) {
	jwtSecret := "enrollment_test_secret_32_bytes_long_key_98765"
	st := store.NewMemoryStore()
	authMw := middleware.NewAuthMiddleware(jwtSecret)
	authH := handlers.NewAuthHandler(st, jwtSecret)
	devH := handlers.NewDeviceHandler(st)
	enrollH := handlers.NewEnrollmentHandler(st)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/auth/login", authH.Login)
	mux.HandleFunc("POST /api/v1/enrollment/handshake", enrollH.Handshake)

	mux.Handle("POST /api/v1/enrollment/invitations", authMw.Authenticate(
		authMw.RequirePermission("device.enroll", enrollH.CreateInvitation),
	))
	mux.Handle("GET /api/v1/enrollment/invitations", authMw.Authenticate(
		authMw.RequirePermission("device.enroll", enrollH.ListInvitations),
	))
	mux.Handle("DELETE /api/v1/enrollment/invitations/{id}", authMw.Authenticate(
		authMw.RequirePermission("device.enroll", enrollH.RevokeInvitation),
	))

	mux.Handle("GET /api/v1/devices/{deviceId}", authMw.Authenticate(
		authMw.RequirePermission("device.view", devH.GetDevice),
	))
	mux.Handle("POST /api/v1/devices/{deviceId}/approve", authMw.Authenticate(
		authMw.RequirePermission("device.approve", devH.ApproveDevice),
	))
	mux.Handle("GET /api/v1/audit-logs", authMw.Authenticate(
		authMw.RequirePermission("audit.view", devH.ListAuditLogs),
	))

	return st, mux, jwtSecret
}

func getAdminToken(t *testing.T, handler http.Handler) string {
	loginReq := handlers.LoginRequest{
		Email:    "admin@acme-academy.org",
		Password: "AdminSecure123!",
	}
	body, _ := json.Marshal(loginReq)
	req := httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Failed to login admin: %d - %s", w.Code, w.Body.String())
	}

	var res handlers.LoginResponse
	_ = json.NewDecoder(w.Body).Decode(&res)
	return res.AccessToken
}

func TestCompleteEnrollmentAndApprovalLifecycle(t *testing.T) {
	st, handler, _ := setupEnrollmentTestApp()
	adminToken := getAdminToken(t, handler)

	// 1. Admin creates enrollment invitation token
	invReq := handlers.CreateInvitationRequest{
		ExpiresInHours: 24,
		MaxUses:        1,
	}
	body, _ := json.Marshal(invReq)
	req := httptest.NewRequest("POST", "/api/v1/enrollment/invitations", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("Expected 201 Created on invitation create, got %d. Body: %s", w.Code, w.Body.String())
	}

	var invRes handlers.CreateInvitationResponse
	if err := json.NewDecoder(w.Body).Decode(&invRes); err != nil {
		t.Fatalf("Failed to decode invitation response: %v", err)
	}

	if invRes.EnrollmentToken == "" {
		t.Fatalf("Expected enrollment token string in response")
	}

	// 2. Windows Agent generates 32-byte Ed25519 public key and calls Handshake
	ed25519PubKey := make([]byte, 32)
	_, _ = rand.Read(ed25519PubKey)
	pubKeyHex := hex.EncodeToString(ed25519PubKey)

	agentReq := handlers.AgentHandshakeRequest{
		EnrollmentToken: invRes.EnrollmentToken,
		Hostname:        "EXAM-LAB-PC-42",
		OSName:          "Windows 11 Enterprise",
		OSVersion:       "10.0.22631",
		OSArchitecture:  "x86_64",
		AgentVersion:    "1.0.0",
		PublicKeyHex:    pubKeyHex,
	}
	aBody, _ := json.Marshal(agentReq)
	hReq := httptest.NewRequest("POST", "/api/v1/enrollment/handshake", bytes.NewReader(aBody))
	hReq.Header.Set("Content-Type", "application/json")
	hRec := httptest.NewRecorder()
	handler.ServeHTTP(hRec, hReq)

	if hRec.Code != http.StatusCreated {
		t.Fatalf("Expected 201 Created on agent handshake, got %d. Body: %s", hRec.Code, hRec.Body.String())
	}

	var hRes handlers.AgentHandshakeResponse
	_ = json.NewDecoder(hRec.Body).Decode(&hRes)

	if hRes.Status != models.DeviceStatusPending {
		t.Fatalf("Expected newly enrolled device to be PENDING, got %s", hRes.Status)
	}

	// 3. Verify device status via Admin API
	devReq := httptest.NewRequest("GET", "/api/v1/devices/"+hRes.DeviceID.String(), nil)
	devReq.Header.Set("Authorization", "Bearer "+adminToken)
	devRec := httptest.NewRecorder()
	handler.ServeHTTP(devRec, devReq)

	if devRec.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK on device fetch, got %d", devRec.Code)
	}

	// 4. Admin approves the pending device
	appReq := httptest.NewRequest("POST", "/api/v1/devices/"+hRes.DeviceID.String()+"/approve", nil)
	appReq.Header.Set("Authorization", "Bearer "+adminToken)
	appRec := httptest.NewRecorder()
	handler.ServeHTTP(appRec, appReq)

	if appRec.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK on device approval, got %d. Body: %s", appRec.Code, appRec.Body.String())
	}

	// 5. Verify device is now ACTIVE
	dev, err := st.GetDevice(devReq.Context(), hRes.OrganizationID, hRes.DeviceID)
	if err != nil || dev.Status != models.DeviceStatusActive {
		t.Fatalf("Expected device to be ACTIVE in store, got status=%v err=%v", dev.Status, err)
	}
}

func TestEnrollmentReplayAndUsageExhaustion(t *testing.T) {
	_, handler, _ := setupEnrollmentTestApp()
	adminToken := getAdminToken(t, handler)

	// Create single-use token (max_uses: 1)
	invReq := handlers.CreateInvitationRequest{
		ExpiresInHours: 24,
		MaxUses:        1,
	}
	body, _ := json.Marshal(invReq)
	req := httptest.NewRequest("POST", "/api/v1/enrollment/invitations", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	var invRes handlers.CreateInvitationResponse
	_ = json.NewDecoder(w.Body).Decode(&invRes)

	pubKey := hex.EncodeToString(make([]byte, 32))
	agentReq := handlers.AgentHandshakeRequest{
		EnrollmentToken: invRes.EnrollmentToken,
		Hostname:        "MACHINE-01",
		OSName:          "Windows 11",
		OSVersion:       "10.0",
		OSArchitecture:  "x86_64",
		AgentVersion:    "1.0.0",
		PublicKeyHex:    pubKey,
	}
	aBody, _ := json.Marshal(agentReq)

	// First use: Should succeed (201)
	hReq1 := httptest.NewRequest("POST", "/api/v1/enrollment/handshake", bytes.NewReader(aBody))
	hReq1.Header.Set("Content-Type", "application/json")
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, hReq1)
	if rec1.Code != http.StatusCreated {
		t.Fatalf("Expected first handshake to succeed, got %d", rec1.Code)
	}

	// Second use: Should fail (403 Forbidden - Exhausted)
	agentReq.Hostname = "ROGUE-MACHINE-02"
	aBody2, _ := json.Marshal(agentReq)
	hReq2 := httptest.NewRequest("POST", "/api/v1/enrollment/handshake", bytes.NewReader(aBody2))
	hReq2.Header.Set("Content-Type", "application/json")
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, hReq2)

	if rec2.Code != http.StatusForbidden {
		t.Fatalf("Expected second handshake with exhausted token to return 403 Forbidden, got %d", rec2.Code)
	}
}

func TestInvalidPublicKeyFormatRejected(t *testing.T) {
	_, handler, _ := setupEnrollmentTestApp()
	adminToken := getAdminToken(t, handler)

	invReq := handlers.CreateInvitationRequest{ExpiresInHours: 24, MaxUses: 5}
	body, _ := json.Marshal(invReq)
	req := httptest.NewRequest("POST", "/api/v1/enrollment/invitations", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	var invRes handlers.CreateInvitationResponse
	_ = json.NewDecoder(w.Body).Decode(&invRes)

	// Submitting invalid public key (only 16 bytes instead of 32 bytes)
	badPubKey := hex.EncodeToString(make([]byte, 16))
	agentReq := handlers.AgentHandshakeRequest{
		EnrollmentToken: invRes.EnrollmentToken,
		Hostname:        "MACHINE-TEST",
		OSName:          "Windows 11",
		OSVersion:       "10.0",
		OSArchitecture:  "x86_64",
		AgentVersion:    "1.0.0",
		PublicKeyHex:    badPubKey,
	}
	aBody, _ := json.Marshal(agentReq)
	hReq := httptest.NewRequest("POST", "/api/v1/enrollment/handshake", bytes.NewReader(aBody))
	hRec := httptest.NewRecorder()
	handler.ServeHTTP(hRec, hReq)

	if hRec.Code != http.StatusBadRequest {
		t.Fatalf("Expected 400 Bad Request for malformed public key, got %d", hRec.Code)
	}
}
