package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/controlhub/controlhub/services/api/internal/middleware"
	"github.com/controlhub/controlhub/services/api/internal/models"
	"github.com/controlhub/controlhub/services/api/internal/store"
	"github.com/google/uuid"
)

type SessionHandler struct {
	store store.Store
}

func NewSessionHandler(st store.Store) *SessionHandler {
	return &SessionHandler{store: st}
}

type CreateSessionRequest struct {
	Mode           string `json:"mode"` // FULL_CONTROL, VIEW_ONLY
	RequireConsent *bool  `json:"require_consent,omitempty"`
}

type ICEServer struct {
	URLs       string `json:"urls"`
	Username   string `json:"username,omitempty"`
	Credential string `json:"credential,omitempty"`
}

type CreateSessionResponse struct {
	SessionID      uuid.UUID           `json:"session_id"`
	DeviceID       uuid.UUID           `json:"device_id"`
	Status         string              `json:"status"` // REQUESTED, CONNECTED
	ConsentPrompt  bool                `json:"consent_prompt_active"`
	ICEServers     []ICEServer         `json:"ice_servers"`
	VisualBorder   map[string]string   `json:"visual_border"`
}

type ConsentDecisionRequest struct {
	Decision string `json:"decision"` // GRANTED, DENIED
	Reason   string `json:"reason,omitempty"`
}

type TerminateSessionRequest struct {
	Reason string `json:"reason,omitempty"`
}

func (h *SessionHandler) CreateSession(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.GetClaims(r.Context())
	if !ok {
		respondError(w, http.StatusUnauthorized, "Not authenticated")
		return
	}

	deviceIDStr := r.PathValue("deviceId")
	deviceID, err := uuid.Parse(deviceIDStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "Invalid device ID format")
		return
	}

	ctx := r.Context()
	dev, err := h.store.GetDevice(ctx, claims.OrganizationID, deviceID)
	if err != nil {
		respondError(w, http.StatusNotFound, "Device not found in tenant organization")
		return
	}

	if dev.Status != models.DeviceStatusActive {
		respondError(w, http.StatusConflict, "Device must be ACTIVE to initiate remote session")
		return
	}

	var req CreateSessionRequest
	_ = json.NewDecoder(r.Body).Decode(&req)

	// Check org policy for mandatory endpoint consent
	org, _ := h.store.GetOrganization(ctx, claims.OrganizationID)
	requireConsent := true
	if org != nil && org.Settings != nil {
		if rc, ok := org.Settings["require_remote_consent"].(bool); ok {
			requireConsent = rc
		}
	}
	if req.RequireConsent != nil {
		requireConsent = *req.RequireConsent
	}

	sessionStatus := "REQUESTED"
	var startedAt *time.Time
	if !requireConsent {
		sessionStatus = "CONNECTED"
		now := time.Now().UTC()
		startedAt = &now
	}

	sessionID := uuid.New()
	session := models.RemoteSession{
		ID:             sessionID,
		OrganizationID: claims.OrganizationID,
		DeviceID:       deviceID,
		OperatorUserID: claims.UserID,
		SessionStatus:  sessionStatus,
		StartedAt:      startedAt,
		CreatedAt:      time.Now().UTC(),
	}

	if err := h.store.CreateRemoteSession(ctx, session); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to create remote session")
		return
	}

	// Audit log
	ip := r.RemoteAddr
	ua := r.UserAgent()
	_ = h.store.InsertAuditLog(ctx, models.AuditLog{
		OrganizationID: claims.OrganizationID,
		ActorUserID:    &claims.UserID,
		Action:         "session.requested",
		ResourceType:   "remote_session",
		ResourceID:     sessionID.String(),
		IPAddress:      &ip,
		UserAgent:      &ua,
		Status:         "SUCCESS",
		Metadata: map[string]interface{}{
			"device_id":         deviceID.String(),
			"hostname":          dev.Hostname,
			"consent_mandatory": requireConsent,
		},
	})

	iceServers := []ICEServer{
		{URLs: "stun:stun.controlhub.local:3478"},
		{URLs: "stun:stun.l.google.com:19302"},
	}

	respondJSON(w, http.StatusCreated, CreateSessionResponse{
		SessionID:     sessionID,
		DeviceID:      deviceID,
		Status:        sessionStatus,
		ConsentPrompt: requireConsent,
		ICEServers:    iceServers,
		VisualBorder: map[string]string{
			"enabled": "true",
			"color":   "#00FFFF", // Neon cyan mandatory visual border
			"width":   "4px",
		},
	})
}

func (h *SessionHandler) GetSession(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.GetClaims(r.Context())
	if !ok {
		respondError(w, http.StatusUnauthorized, "Not authenticated")
		return
	}

	sessionIDStr := r.PathValue("sessionId")
	sessionID, err := uuid.Parse(sessionIDStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "Invalid session ID format")
		return
	}

	session, err := h.store.GetRemoteSession(r.Context(), claims.OrganizationID, sessionID)
	if err != nil {
		respondError(w, http.StatusNotFound, "Remote session not found")
		return
	}

	respondJSON(w, http.StatusOK, session)
}

func (h *SessionHandler) HandleConsent(w http.ResponseWriter, r *http.Request) {
	sessionIDStr := r.PathValue("sessionId")
	sessionID, err := uuid.Parse(sessionIDStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "Invalid session ID format")
		return
	}

	var req ConsentDecisionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	ctx := r.Context()
	newStatus := "CONNECTED"
	auditAction := "session.consent.granted"
	if req.Decision == "DENIED" {
		newStatus = "REJECTED"
		auditAction = "session.consent.denied"
	}

	// Update session status
	for _, s := range []string{newStatus} {
		_ = s
	}

	reason := req.Reason
	if err := h.store.UpdateRemoteSessionStatus(ctx, uuid.Nil, sessionID, newStatus, &reason); err != nil {
		// Try searching across all orgs if Nil org
		allSessions, _ := h.store.ListRemoteSessions(ctx, uuid.Nil)
		for _, s := range allSessions {
			if s.ID == sessionID {
				_ = h.store.UpdateRemoteSessionStatus(ctx, s.OrganizationID, sessionID, newStatus, &reason)
				break
			}
		}
	}

	ip := r.RemoteAddr
	ua := r.UserAgent()
	_ = h.store.InsertAuditLog(ctx, models.AuditLog{
		Action:       auditAction,
		ResourceType: "remote_session",
		ResourceID:   sessionID.String(),
		IPAddress:    &ip,
		UserAgent:    &ua,
		Status:       "SUCCESS",
		Metadata: map[string]interface{}{
			"decision": req.Decision,
			"reason":   req.Reason,
		},
	})

	respondJSON(w, http.StatusOK, map[string]string{
		"session_id": sessionID.String(),
		"status":     newStatus,
	})
}

func (h *SessionHandler) TerminateSession(w http.ResponseWriter, r *http.Request) {
	sessionIDStr := r.PathValue("sessionId")
	sessionID, err := uuid.Parse(sessionIDStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "Invalid session ID format")
		return
	}

	var req TerminateSessionRequest
	_ = json.NewDecoder(r.Body).Decode(&req)
	reason := req.Reason
	if reason == "" {
		reason = "operator_disconnected"
	}

	ctx := r.Context()
	var orgID uuid.UUID
	var userID *uuid.UUID
	if claims, ok := middleware.GetClaims(ctx); ok {
		orgID = claims.OrganizationID
		userID = &claims.UserID
	}

	_ = h.store.UpdateRemoteSessionStatus(ctx, orgID, sessionID, "TERMINATED", &reason)

	ip := r.RemoteAddr
	ua := r.UserAgent()
	_ = h.store.InsertAuditLog(ctx, models.AuditLog{
		OrganizationID: orgID,
		ActorUserID:    userID,
		Action:         "session.terminated",
		ResourceType:   "remote_session",
		ResourceID:     sessionID.String(),
		IPAddress:      &ip,
		UserAgent:      &ua,
		Status:         "SUCCESS",
		Metadata: map[string]interface{}{
			"reason": reason,
		},
	})

	respondJSON(w, http.StatusOK, map[string]string{
		"session_id": sessionID.String(),
		"status":     "TERMINATED",
		"message":    "Remote desktop session closed. Screen border cleared.",
	})
}

func (h *SessionHandler) ListSessions(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.GetClaims(r.Context())
	if !ok {
		respondError(w, http.StatusUnauthorized, "Not authenticated")
		return
	}

	sessions, err := h.store.ListRemoteSessions(r.Context(), claims.OrganizationID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to list sessions")
		return
	}

	respondJSON(w, http.StatusOK, sessions)
}
