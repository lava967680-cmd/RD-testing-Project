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

func setupSessionTestApp() (*store.MemoryStore, http.Handler, string) {
	jwtSecret := "session_test_secret_32_bytes_long_key_77777"
	st := store.NewMemoryStore()
	authMw := middleware.NewAuthMiddleware(jwtSecret)
	authH := handlers.NewAuthHandler(st, jwtSecret)
	sessionH := handlers.NewSessionHandler(st)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/auth/login", authH.Login)
	mux.HandleFunc("POST /api/v1/sessions/{sessionId}/consent", sessionH.HandleConsent)

	mux.Handle("POST /api/v1/devices/{deviceId}/sessions", authMw.Authenticate(
		authMw.RequirePermission("device.remote", sessionH.CreateSession),
	))
	mux.Handle("GET /api/v1/sessions/{sessionId}", authMw.Authenticate(
		authMw.RequirePermission("device.remote", sessionH.GetSession),
	))
	mux.Handle("POST /api/v1/sessions/{sessionId}/terminate", authMw.Authenticate(
		authMw.RequirePermission("device.remote", sessionH.TerminateSession),
	))

	return st, mux, jwtSecret
}

func TestRemoteSessionLifecycleWithConsent(t *testing.T) {
	st, handler, _ := setupSessionTestApp()
	adminToken := getAdminToken(t, handler)
	devID := uuid.MustParse("90e722c1-bb3e-4623-a128-44fb6b907a01")

	// 1. Initiate Remote Support Request
	reqBody := handlers.CreateSessionRequest{
		Mode: "FULL_CONTROL",
	}
	bBytes, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("POST", "/api/v1/devices/"+devID.String()+"/sessions", bytes.NewReader(bBytes))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("Expected 201 Created on session create, got %d. Body: %s", w.Code, w.Body.String())
	}

	var res handlers.CreateSessionResponse
	if err := json.NewDecoder(w.Body).Decode(&res); err != nil {
		t.Fatalf("Failed to decode session response: %v", err)
	}

	if res.Status != "REQUESTED" || !res.ConsentPrompt {
		t.Fatalf("Expected session to be REQUESTED with consent prompt active, got status=%s prompt=%v", res.Status, res.ConsentPrompt)
	}

	if res.VisualBorder["enabled"] != "true" || res.VisualBorder["color"] != "#00FFFF" {
		t.Fatalf("Expected visual border config in response, got %+v", res.VisualBorder)
	}

	// 2. Endpoint User grants consent
	consentReq := handlers.ConsentDecisionRequest{
		Decision: "GRANTED",
	}
	cBytes, _ := json.Marshal(consentReq)
	cReq := httptest.NewRequest("POST", "/api/v1/sessions/"+res.SessionID.String()+"/consent", bytes.NewReader(cBytes))
	cReq.Header.Set("Content-Type", "application/json")
	cRec := httptest.NewRecorder()
	handler.ServeHTTP(cRec, cReq)

	if cRec.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK on consent submission, got %d", cRec.Code)
	}

	// 3. Verify session transitioned to CONNECTED
	getReq := httptest.NewRequest("GET", "/api/v1/sessions/"+res.SessionID.String(), nil)
	getReq.Header.Set("Authorization", "Bearer "+adminToken)
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)

	var session models.RemoteSession
	_ = json.NewDecoder(getRec.Body).Decode(&session)
	if session.SessionStatus != "CONNECTED" || session.StartedAt == nil {
		t.Fatalf("Expected session to be CONNECTED, got %s", session.SessionStatus)
	}

	// 4. Terminate session
	termReq := handlers.TerminateSessionRequest{
		Reason: "Support ticket resolved",
	}
	tBytes, _ := json.Marshal(termReq)
	tReq := httptest.NewRequest("POST", "/api/v1/sessions/"+res.SessionID.String()+"/terminate", bytes.NewReader(tBytes))
	tReq.Header.Set("Authorization", "Bearer "+adminToken)
	tReq.Header.Set("Content-Type", "application/json")
	tRec := httptest.NewRecorder()
	handler.ServeHTTP(tRec, tReq)

	if tRec.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK on session terminate, got %d", tRec.Code)
	}

	// Verify session is TERMINATED in store
	endedSession, _ := st.GetRemoteSession(req.Context(), uuid.MustParse("771e8bfb-cf98-4c28-98e3-0d6e6443c21a"), res.SessionID)
	if endedSession.SessionStatus != "TERMINATED" || endedSession.EndedAt == nil {
		t.Fatalf("Expected session to be TERMINATED, got %s", endedSession.SessionStatus)
	}
}

func TestRemoteSessionConsentDenied(t *testing.T) {
	_, handler, _ := setupSessionTestApp()
	adminToken := getAdminToken(t, handler)
	devID := uuid.MustParse("90e722c1-bb3e-4623-a128-44fb6b907a01")

	// 1. Initiate session
	req := httptest.NewRequest("POST", "/api/v1/devices/"+devID.String()+"/sessions", bytes.NewReader([]byte(`{"mode":"FULL_CONTROL"}`)))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	var res handlers.CreateSessionResponse
	_ = json.NewDecoder(w.Body).Decode(&res)

	// 2. User clicks "Deny"
	cBytes, _ := json.Marshal(handlers.ConsentDecisionRequest{
		Decision: "DENIED",
		Reason:   "User in private exam",
	})
	cReq := httptest.NewRequest("POST", "/api/v1/sessions/"+res.SessionID.String()+"/consent", bytes.NewReader(cBytes))
	cReq.Header.Set("Content-Type", "application/json")
	cRec := httptest.NewRecorder()
	handler.ServeHTTP(cRec, cReq)

	// 3. Verify session status is REJECTED
	getReq := httptest.NewRequest("GET", "/api/v1/sessions/"+res.SessionID.String(), nil)
	getReq.Header.Set("Authorization", "Bearer "+adminToken)
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)

	var session models.RemoteSession
	_ = json.NewDecoder(getRec.Body).Decode(&session)
	if session.SessionStatus != "REJECTED" {
		t.Fatalf("Expected session status to be REJECTED, got %s", session.SessionStatus)
	}
}
