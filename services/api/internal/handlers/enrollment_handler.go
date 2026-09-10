package handlers

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"time"

	"github.com/controlhub/controlhub/services/api/internal/middleware"
	"github.com/controlhub/controlhub/services/api/internal/models"
	"github.com/controlhub/controlhub/services/api/internal/store"
	"github.com/google/uuid"
)

type EnrollmentHandler struct {
	store store.Store
}

func NewEnrollmentHandler(st store.Store) *EnrollmentHandler {
	return &EnrollmentHandler{store: st}
}

type CreateInvitationRequest struct {
	TargetGroupID    *uuid.UUID `json:"target_group_id,omitempty"`
	ExpiresInHours   int        `json:"expires_in_hours"`
	MaxUses          int        `json:"max_uses"`
	RequiresApproval *bool      `json:"requires_approval"`
}

type CreateInvitationResponse struct {
	InvitationID    uuid.UUID  `json:"invitation_id"`
	EnrollmentToken string     `json:"enrollment_token"`
	ExpiresAt       time.Time  `json:"expires_at"`
	MaxUses         int        `json:"max_uses"`
	TargetGroupID   *uuid.UUID `json:"target_group_id,omitempty"`
}

type AgentHandshakeRequest struct {
	EnrollmentToken string `json:"enrollment_token"`
	Hostname        string `json:"hostname"`
	OSName          string `json:"os_name"`
	OSVersion       string `json:"os_version"`
	OSArchitecture  string `json:"os_architecture"`
	AgentVersion    string `json:"agent_version"`
	PublicKeyHex    string `json:"public_key_hex"`
}

type AgentHandshakeResponse struct {
	DeviceID         uuid.UUID           `json:"device_id"`
	OrganizationID   uuid.UUID           `json:"organization_id"`
	OrganizationName string              `json:"organization_name"`
	Status           models.DeviceStatus `json:"status"`
	Message          string              `json:"message"`
}

func (h *EnrollmentHandler) CreateInvitation(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.GetClaims(r.Context())
	if !ok {
		respondError(w, http.StatusUnauthorized, "Not authenticated")
		return
	}

	var req CreateInvitationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid JSON body")
		return
	}

	if req.ExpiresInHours <= 0 {
		req.ExpiresInHours = 24
	}
	if req.ExpiresInHours > 168 { // Max 7 days
		req.ExpiresInHours = 168
	}
	if req.MaxUses <= 0 {
		req.MaxUses = 1
	}

	reqApproval := true
	if req.RequiresApproval != nil {
		reqApproval = *req.RequiresApproval
	}

	// Generate 256-bit cryptographically secure token
	randBytes := make([]byte, 16)
	if _, err := rand.Read(randBytes); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to generate token entropy")
		return
	}
	tokenStr := "CH-ENROLL-" + hex.EncodeToString(randBytes)

	// Hash token with SHA-256 for secure database storage
	hBytes := sha256.Sum256([]byte(tokenStr))
	tokenHash := hex.EncodeToString(hBytes[:])

	invitationID := uuid.New()
	expiresAt := time.Now().UTC().Add(time.Duration(req.ExpiresInHours) * time.Hour)

	inv := models.EnrollmentInvitation{
		ID:               invitationID,
		OrganizationID:   claims.OrganizationID,
		TokenHash:        tokenHash,
		TargetGroupID:    req.TargetGroupID,
		RequiresApproval: reqApproval,
		MaxUses:          req.MaxUses,
		TimesUsed:        0,
		ExpiresAt:        expiresAt,
		CreatedByUserID:  &claims.UserID,
		CreatedAt:        time.Now().UTC(),
	}

	if err := h.store.CreateEnrollmentInvitation(r.Context(), inv); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to save invitation")
		return
	}

	// Audit log
	ip := r.RemoteAddr
	ua := r.UserAgent()
	_ = h.store.InsertAuditLog(r.Context(), models.AuditLog{
		OrganizationID: claims.OrganizationID,
		ActorUserID:    &claims.UserID,
		Action:         "device.invitation.created",
		ResourceType:   "enrollment_invitation",
		ResourceID:     invitationID.String(),
		IPAddress:      &ip,
		UserAgent:      &ua,
		Status:         "SUCCESS",
		Metadata: map[string]interface{}{
			"max_uses":          req.MaxUses,
			"expires_at":        expiresAt,
			"requires_approval": reqApproval,
		},
	})

	respondJSON(w, http.StatusCreated, CreateInvitationResponse{
		InvitationID:    invitationID,
		EnrollmentToken: tokenStr,
		ExpiresAt:       expiresAt,
		MaxUses:         req.MaxUses,
		TargetGroupID:   req.TargetGroupID,
	})
}

func (h *EnrollmentHandler) ListInvitations(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.GetClaims(r.Context())
	if !ok {
		respondError(w, http.StatusUnauthorized, "Not authenticated")
		return
	}

	invs, err := h.store.ListEnrollmentInvitations(r.Context(), claims.OrganizationID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to query invitations")
		return
	}

	respondJSON(w, http.StatusOK, invs)
}

func (h *EnrollmentHandler) RevokeInvitation(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.GetClaims(r.Context())
	if !ok {
		respondError(w, http.StatusUnauthorized, "Not authenticated")
		return
	}

	invIDStr := r.PathValue("id")
	invID, err := uuid.Parse(invIDStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "Invalid invitation ID format")
		return
	}

	if err := h.store.DeleteEnrollmentInvitation(r.Context(), claims.OrganizationID, invID); err != nil {
		respondError(w, http.StatusNotFound, "Invitation not found or unauthorized")
		return
	}

	ip := r.RemoteAddr
	ua := r.UserAgent()
	_ = h.store.InsertAuditLog(r.Context(), models.AuditLog{
		OrganizationID: claims.OrganizationID,
		ActorUserID:    &claims.UserID,
		Action:         "device.invitation.revoked",
		ResourceType:   "enrollment_invitation",
		ResourceID:     invID.String(),
		IPAddress:      &ip,
		UserAgent:      &ua,
		Status:         "SUCCESS",
	})

	respondJSON(w, http.StatusOK, map[string]string{
		"message": "Enrollment invitation revoked",
	})
}

// Agent Handshake (Public endpoint accessed by agent during installation)
func (h *EnrollmentHandler) Handshake(w http.ResponseWriter, r *http.Request) {
	var req AgentHandshakeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	if req.EnrollmentToken == "" || req.PublicKeyHex == "" || req.Hostname == "" {
		respondError(w, http.StatusBadRequest, "Missing required enrollment fields (token, hostname, public_key)")
		return
	}

	// Validate Ed25519 public key hex encoding
	pubKeyBytes, err := hex.DecodeString(req.PublicKeyHex)
	if err != nil || len(pubKeyBytes) != 32 {
		respondError(w, http.StatusBadRequest, "Invalid Ed25519 public key format. Must be 32 bytes hex encoded.")
		return
	}

	// Hash submitted plaintext token
	hBytes := sha256.Sum256([]byte(req.EnrollmentToken))
	tokenHash := hex.EncodeToString(hBytes[:])

	ctx := r.Context()
	inv, err := h.store.GetEnrollmentInvitationByHash(ctx, tokenHash)
	if err != nil {
		if err == store.ErrTokenExpired {
			respondError(w, http.StatusForbidden, "Enrollment token has expired")
			return
		}
		if err == store.ErrTokenExhausted {
			respondError(w, http.StatusForbidden, "Enrollment token usage limit reached")
			return
		}
		respondError(w, http.StatusForbidden, "Invalid enrollment token")
		return
	}

	// Consume one usage
	_ = h.store.ConsumeEnrollmentInvitation(ctx, inv.ID)

	org, err := h.store.GetOrganization(ctx, inv.OrganizationID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Associated organization unavailable")
		return
	}

	initialStatus := models.DeviceStatusPending
	if !inv.RequiresApproval {
		initialStatus = models.DeviceStatusActive
	}

	deviceID := uuid.New()
	now := time.Now().UTC()
	dev := models.Device{
		ID:             deviceID,
		OrganizationID: inv.OrganizationID,
		Hostname:       req.Hostname,
		OSName:         req.OSName,
		OSVersion:      req.OSVersion,
		OSArchitecture: req.OSArchitecture,
		AgentVersion:   req.AgentVersion,
		Status:         initialStatus,
		LastSeen:       &now,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	if err := h.store.RegisterDevice(ctx, dev, pubKeyBytes); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to register endpoint device")
		return
	}

	// Audit log
	ip := r.RemoteAddr
	ua := r.UserAgent()
	_ = h.store.InsertAuditLog(ctx, models.AuditLog{
		OrganizationID: inv.OrganizationID,
		Action:         "device.enrolled",
		ResourceType:   "device",
		ResourceID:     deviceID.String(),
		IPAddress:      &ip,
		UserAgent:      &ua,
		Status:         "SUCCESS",
		Metadata: map[string]interface{}{
			"hostname":          req.Hostname,
			"os_name":           req.OSName,
			"agent_version":     req.AgentVersion,
			"initial_status":    initialStatus,
			"invitation_id":     inv.ID.String(),
			"requires_approval": inv.RequiresApproval,
		},
	})

	msg := "Device successfully enrolled and awaiting administrator approval."
	if initialStatus == models.DeviceStatusActive {
		msg = "Device successfully enrolled and active."
	}

	respondJSON(w, http.StatusCreated, AgentHandshakeResponse{
		DeviceID:         deviceID,
		OrganizationID:   inv.OrganizationID,
		OrganizationName: org.Name,
		Status:           initialStatus,
		Message:          msg,
	})
}
