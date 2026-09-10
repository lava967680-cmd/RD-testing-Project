package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/controlhub/controlhub/services/api/internal/auth"
	"github.com/controlhub/controlhub/services/api/internal/handlers"
	"github.com/controlhub/controlhub/services/api/internal/middleware"
	"github.com/controlhub/controlhub/services/api/internal/models"
	"github.com/controlhub/controlhub/services/api/internal/store"
	"github.com/google/uuid"
)

func setupTestApp() (*store.MemoryStore, http.Handler, string) {
	jwtSecret := "integration_test_secret_32_bytes_long_key_12345"
	st := store.NewMemoryStore()
	authMw := middleware.NewAuthMiddleware(jwtSecret)
	authH := handlers.NewAuthHandler(st, jwtSecret)
	devH := handlers.NewDeviceHandler(st)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/auth/login", authH.Login)
	mux.HandleFunc("POST /api/v1/auth/mfa/verify", authH.VerifyMFA)
	mux.HandleFunc("POST /api/v1/auth/refresh", authH.Refresh)
	mux.Handle("POST /api/v1/auth/logout", authMw.Authenticate(http.HandlerFunc(authH.Logout)))
	mux.Handle("GET /api/v1/auth/me", authMw.Authenticate(http.HandlerFunc(authH.Me)))

	mux.Handle("GET /api/v1/devices", authMw.Authenticate(
		authMw.RequirePermission("device.view", devH.ListDevices),
	))
	mux.Handle("POST /api/v1/devices/{deviceId}/approve", authMw.Authenticate(
		authMw.RequirePermission("device.approve", devH.ApproveDevice),
	))
	mux.Handle("GET /api/v1/audit-logs", authMw.Authenticate(
		authMw.RequirePermission("audit.view", devH.ListAuditLogs),
	))

	return st, mux, jwtSecret
}

func TestSuccessfulLoginAndProfileRetrieval(t *testing.T) {
	_, handler, _ := setupTestApp()

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
		t.Fatalf("Expected status 200, got %d. Body: %s", w.Code, w.Body.String())
	}

	var res handlers.LoginResponse
	if err := json.NewDecoder(w.Body).Decode(&res); err != nil {
		t.Fatalf("Failed to decode login response: %v", err)
	}

	if res.AccessToken == "" || res.RefreshToken == "" {
		t.Fatalf("Expected access and refresh tokens in response")
	}

	// Verify profile query
	meReq := httptest.NewRequest("GET", "/api/v1/auth/me", nil)
	meReq.Header.Set("Authorization", "Bearer "+res.AccessToken)
	meRec := httptest.NewRecorder()

	handler.ServeHTTP(meRec, meReq)

	if meRec.Code != http.StatusOK {
		t.Fatalf("Expected status 200 on /auth/me, got %d. Body: %s", meRec.Code, meRec.Body.String())
	}
}

func TestInvalidCredentialsRejected(t *testing.T) {
	_, handler, _ := setupTestApp()

	loginReq := handlers.LoginRequest{
		Email:    "admin@acme-academy.org",
		Password: "WrongPassword999!",
	}
	body, _ := json.Marshal(loginReq)

	req := httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("Expected status 401 Unauthorized, got %d", w.Code)
	}
}

func TestRBACPermissionEnforcement(t *testing.T) {
	st, handler, jwtSecret := setupTestApp()
	ctx := context.Background()

	// Seed Operator User with only 'device.view' permission (lacks 'device.approve')
	operatorID := uuid.New()
	demoOrgID := uuid.MustParse("771e8bfb-cf98-4c28-98e3-0d6e6443c21a")
	hash, _ := auth.HashPassword("OperatorPass123!", nil)

	st.SaveRefreshToken(ctx, store.RefreshToken{}) // warm-up
	// Generate Operator token directly
	operatorToken, _ := auth.GenerateAccessToken(operatorID, demoOrgID, "Operator", []string{"device.view"}, jwtSecret)

	// Add pending device
	pendingDevID := uuid.New()
	_ = st.UpdateDeviceStatus(ctx, demoOrgID, pendingDevID, models.DeviceStatusPending)

	// Attempt approve with Operator token -> Should be 403 Forbidden
	approveReq := httptest.NewRequest("POST", "/api/v1/devices/"+pendingDevID.String()+"/approve", nil)
	approveReq.Header.Set("Authorization", "Bearer "+operatorToken)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, approveReq)

	if w.Code != http.StatusForbidden {
		t.Fatalf("Expected status 403 Forbidden for operator attempting device approval, got %d", w.Code)
	}
}

func TestTOTPMultiFactorAuthenticationFlow(t *testing.T) {
	st, handler, _ := setupTestApp()
	ctx := context.Background()

	// Enable MFA for demo admin
	demoAdminID := uuid.MustParse("c1f72776-9ec6-4f40-8b17-7435f3dfd720")
	secret, err := auth.GenerateTOTPSecret()
	if err != nil {
		t.Fatalf("Failed to generate TOTP secret: %v", err)
	}

	_ = st.UpdateUserMFA(ctx, demoAdminID, true, &secret)

	// 1. Initial login should return MFA challenge
	loginReq := handlers.LoginRequest{
		Email:    "admin@acme-academy.org",
		Password: "AdminSecure123!",
	}
	body, _ := json.Marshal(loginReq)
	req := httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewReader(body))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", w.Code)
	}

	var res handlers.LoginResponse
	_ = json.NewDecoder(w.Body).Decode(&res)

	if !res.MFARequired || res.MFAToken == "" {
		t.Fatalf("Expected MFA challenge required and MFAToken, got %+v", res)
	}

	// 2. Generate valid TOTP code
	validCode, err := auth.GenerateTOTP(secret, time.Now().UTC())
	if err != nil {
		t.Fatalf("Failed to generate TOTP code: %v", err)
	}

	// 3. Verify MFA challenge
	verifyReq := handlers.MFAVerifyRequest{
		MFAToken: res.MFAToken,
		Code:     validCode,
	}
	vBody, _ := json.Marshal(verifyReq)
	vReq := httptest.NewRequest("POST", "/api/v1/auth/mfa/verify", bytes.NewReader(vBody))
	vRec := httptest.NewRecorder()
	handler.ServeHTTP(vRec, vReq)

	if vRec.Code != http.StatusOK {
		t.Fatalf("Expected status 200 on MFA verify, got %d. Body: %s", vRec.Code, vRec.Body.String())
	}

	var finalRes handlers.LoginResponse
	_ = json.NewDecoder(vRec.Body).Decode(&finalRes)

	if finalRes.AccessToken == "" {
		t.Fatalf("Expected valid access token after solving MFA challenge")
	}
}
