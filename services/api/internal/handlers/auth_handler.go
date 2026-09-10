package handlers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"time"

	"github.com/controlhub/controlhub/services/api/internal/auth"
	"github.com/controlhub/controlhub/services/api/internal/middleware"
	"github.com/controlhub/controlhub/services/api/internal/models"
	"github.com/controlhub/controlhub/services/api/internal/store"
	"github.com/google/uuid"
)

type AuthHandler struct {
	store     store.Store
	jwtSecret string
}

func NewAuthHandler(st store.Store, jwtSecret string) *AuthHandler {
	return &AuthHandler{
		store:     st,
		jwtSecret: jwtSecret,
	}
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginResponse struct {
	AccessToken  string       `json:"access_token,omitempty"`
	RefreshToken string       `json:"refresh_token,omitempty"`
	ExpiresIn    int          `json:"expires_in,omitempty"`
	MFARequired  bool         `json:"mfa_required"`
	MFAToken     string       `json:"mfa_token,omitempty"`
	User         *models.User `json:"user,omitempty"`
	Role         string       `json:"role,omitempty"`
}

type MFAVerifyRequest struct {
	MFAToken string `json:"mfa_token"`
	Code     string `json:"code"`
}

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid JSON body")
		return
	}

	ctx := r.Context()
	user, err := h.store.GetUserByEmail(ctx, req.Email)
	if err != nil {
		// Log failed attempt
		h.recordAudit(ctx, uuid.Nil, nil, "auth.login.failure", "user", req.Email, "FAILURE", map[string]interface{}{
			"reason": "user_not_found",
		}, r)
		respondError(w, http.StatusUnauthorized, "Invalid credentials")
		return
	}

	// Verify Argon2id password
	valid, err := auth.VerifyPassword(req.Password, user.PasswordHash)
	if err != nil || !valid {
		h.recordAudit(ctx, user.OrganizationID, &user.ID, "auth.login.failure", "user", user.ID.String(), "FAILURE", map[string]interface{}{
			"reason": "invalid_password",
		}, r)
		respondError(w, http.StatusUnauthorized, "Invalid credentials")
		return
	}

	// Check if MFA is required
	if user.MFAEnabled && user.MFASecret != nil {
		// Generate temporary MFA challenge token (15-minute TTL)
		mfaClaims := auth.Claims{
			UserID:         user.ID,
			OrganizationID: user.OrganizationID,
			Role:           "MFA_CHALLENGE",
		}
		mfaToken, _ := auth.GenerateAccessToken(mfaClaims.UserID, mfaClaims.OrganizationID, mfaClaims.Role, nil, h.jwtSecret)

		h.recordAudit(ctx, user.OrganizationID, &user.ID, "auth.mfa.challenge", "user", user.ID.String(), "SUCCESS", nil, r)

		respondJSON(w, http.StatusOK, LoginResponse{
			MFARequired: true,
			MFAToken:    mfaToken,
		})
		return
	}

	// Direct login success
	h.issueTokensAndRespond(w, r, user)
}

func (h *AuthHandler) VerifyMFA(w http.ResponseWriter, r *http.Request) {
	var req MFAVerifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid JSON body")
		return
	}

	claims, err := auth.ValidateAccessToken(req.MFAToken, h.jwtSecret)
	if err != nil || claims.Role != "MFA_CHALLENGE" {
		respondError(w, http.StatusUnauthorized, "Invalid or expired MFA session")
		return
	}

	ctx := r.Context()
	user, err := h.store.GetUserByID(ctx, claims.UserID)
	if err != nil || user.MFASecret == nil {
		respondError(w, http.StatusUnauthorized, "User or MFA configuration not found")
		return
	}

	if !auth.ValidateTOTP(*user.MFASecret, req.Code, time.Now().UTC()) {
		h.recordAudit(ctx, user.OrganizationID, &user.ID, "auth.mfa.verify", "user", user.ID.String(), "FAILURE", map[string]interface{}{
			"reason": "invalid_totp_code",
		}, r)
		respondError(w, http.StatusUnauthorized, "Invalid authentication code")
		return
	}

	h.recordAudit(ctx, user.OrganizationID, &user.ID, "auth.mfa.success", "user", user.ID.String(), "SUCCESS", nil, r)
	h.issueTokensAndRespond(w, r, user)
}

func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	var req RefreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid JSON body")
		return
	}

	ctx := r.Context()
	rt, err := h.store.GetRefreshToken(ctx, req.RefreshToken)
	if err != nil {
		respondError(w, http.StatusUnauthorized, "Invalid or expired refresh token")
		return
	}

	// Revoke old refresh token (token rotation)
	_ = h.store.RevokeRefreshToken(ctx, req.RefreshToken)

	user, err := h.store.GetUserByID(ctx, rt.UserID)
	if err != nil {
		respondError(w, http.StatusUnauthorized, "User no longer exists")
		return
	}

	h.issueTokensAndRespond(w, r, user)
}

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	var req RefreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err == nil && req.RefreshToken != "" {
		_ = h.store.RevokeRefreshToken(r.Context(), req.RefreshToken)
	}

	if claims, ok := middleware.GetClaims(r.Context()); ok {
		h.recordAudit(r.Context(), claims.OrganizationID, &claims.UserID, "auth.logout", "user", claims.UserID.String(), "SUCCESS", nil, r)
	}

	respondJSON(w, http.StatusOK, map[string]string{
		"message": "Logged out successfully",
	})
}

func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.GetClaims(r.Context())
	if !ok {
		respondError(w, http.StatusUnauthorized, "Not authenticated")
		return
	}

	user, err := h.store.GetUserByID(r.Context(), claims.UserID)
	if err != nil {
		respondError(w, http.StatusNotFound, "User not found")
		return
	}

	org, err := h.store.GetOrganization(r.Context(), claims.OrganizationID)
	if err != nil {
		respondError(w, http.StatusNotFound, "Organization not found")
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"user":         user,
		"organization": org,
		"role":         claims.Role,
		"permissions":  claims.Permissions,
	})
}

func (h *AuthHandler) issueTokensAndRespond(w http.ResponseWriter, r *http.Request, user *models.User) {
	ctx := r.Context()
	perms, role, err := h.store.GetUserPermissions(ctx, user.ID)
	if err != nil {
		perms = []string{}
		role = "Viewer"
	}

	accessToken, err := auth.GenerateAccessToken(user.ID, user.OrganizationID, role, perms, h.jwtSecret)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to generate access token")
		return
	}

	// Generate secure random refresh token
	rfBytes := make([]byte, 32)
	_, _ = rand.Read(rfBytes)
	refreshTokenStr := "rt_" + hex.EncodeToString(rfBytes)

	_ = h.store.SaveRefreshToken(ctx, store.RefreshToken{
		Token:          refreshTokenStr,
		UserID:         user.ID,
		OrganizationID: user.OrganizationID,
		ExpiresAt:      time.Now().UTC().Add(7 * 24 * time.Hour),
		Revoked:        false,
	})

	h.recordAudit(ctx, user.OrganizationID, &user.ID, "auth.login.success", "user", user.ID.String(), "SUCCESS", map[string]interface{}{
		"role": role,
	}, r)

	respondJSON(w, http.StatusOK, LoginResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshTokenStr,
		ExpiresIn:    900,
		MFARequired:  false,
		User:         user,
		Role:         role,
	})
}

func (h *AuthHandler) recordAudit(ctx context.Context, orgID uuid.UUID, userID *uuid.UUID, action, resType, resID, status string, meta map[string]interface{}, r *http.Request) {
	if meta == nil {
		meta = make(map[string]interface{})
	}
	ip := r.RemoteAddr
	ua := r.UserAgent()

	_ = h.store.InsertAuditLog(ctx, models.AuditLog{
		OrganizationID: orgID,
		ActorUserID:    userID,
		Action:         action,
		ResourceType:   resType,
		ResourceID:     resID,
		IPAddress:      &ip,
		UserAgent:      &ua,
		Status:         status,
		Metadata:       meta,
	})
}

func respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func respondError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error": message,
	})
}
