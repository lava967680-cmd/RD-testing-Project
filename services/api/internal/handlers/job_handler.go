package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"time"

	"github.com/controlhub/controlhub/services/api/internal/middleware"
	"github.com/controlhub/controlhub/services/api/internal/models"
	"github.com/controlhub/controlhub/services/api/internal/store"
	"github.com/google/uuid"
)

type JobHandler struct {
	store store.Store
}

func NewJobHandler(st store.Store) *JobHandler {
	return &JobHandler{store: st}
}

type CreateCommandRequest struct {
	CommandType string                 `json:"command_type"` // REBOOT, SHUTDOWN, RUN_SCRIPT
	Parameters  map[string]interface{} `json:"parameters"`
	ExpiresInSec int                   `json:"expires_in_sec"`
}

type CreateCommandResponse struct {
	CommandID uuid.UUID `json:"command_id"`
	DeviceID  uuid.UUID `json:"device_id"`
	Status    string    `json:"status"`
	Nonce     string    `json:"nonce"`
	ExpiresAt time.Time `json:"expires_at"`
}

type BulkCommandRequest struct {
	DeviceIDs    []uuid.UUID            `json:"device_ids"`
	CommandType  string                 `json:"command_type"`
	Parameters   map[string]interface{} `json:"parameters"`
	Confirmation string                 `json:"confirmation"` // Mandatory confirmation string e.g. "CONFIRM_DISRUPTIVE_ACTION"
}

type BulkCommandResponse struct {
	TotalQueued int         `json:"total_queued"`
	CommandIDs  []uuid.UUID `json:"command_ids"`
	Status      string      `json:"status"`
}

type AgentCommandResultRequest struct {
	ExitCode            int    `json:"exit_code"`
	StdoutOutput        string `json:"stdout_output"`
	StderrOutput        string `json:"stderr_output"`
	ExecutionDurationMs int    `json:"execution_duration_ms"`
}

func (h *JobHandler) CreateCommand(w http.ResponseWriter, r *http.Request) {
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

	var req CreateCommandRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.CommandType == "" {
		respondError(w, http.StatusBadRequest, "Invalid JSON payload or missing command_type")
		return
	}

	// Granular permission check: REBOOT / SHUTDOWN requires 'device.restart', RUN_SCRIPT requires 'device.terminal'
	requiredPerm := "device.restart"
	if req.CommandType == "RUN_SCRIPT" {
		requiredPerm = "device.terminal"
	}
	hasPerm := false
	for _, p := range claims.Permissions {
		if p == requiredPerm || p == "*" {
			hasPerm = true
			break
		}
	}
	if !hasPerm {
		respondError(w, http.StatusForbidden, "Insufficient permissions to execute command type")
		return
	}

	if req.ExpiresInSec <= 0 {
		req.ExpiresInSec = 300 // 5 minutes default
	}

	// Generate 128-bit cryptographic nonce for replay protection
	nonceBytes := make([]byte, 16)
	_, _ = rand.Read(nonceBytes)
	nonce := hex.EncodeToString(nonceBytes)

	commandID := uuid.New()
	expiresAt := time.Now().UTC().Add(time.Duration(req.ExpiresInSec) * time.Second)

	cmd := models.Command{
		ID:             commandID,
		OrganizationID: claims.OrganizationID,
		DeviceID:       deviceID,
		IssuedByUserID: claims.UserID,
		CommandType:    req.CommandType,
		Parameters:     req.Parameters,
		Status:         "QUEUED",
		Nonce:          nonce,
		ExpiresAt:      expiresAt,
		CreatedAt:      time.Now().UTC(),
	}

	if err := h.store.CreateCommand(ctx, cmd); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to queue command job")
		return
	}

	ip := r.RemoteAddr
	ua := r.UserAgent()
	_ = h.store.InsertAuditLog(ctx, models.AuditLog{
		OrganizationID: claims.OrganizationID,
		ActorUserID:    &claims.UserID,
		Action:         "command.queued",
		ResourceType:   "command",
		ResourceID:     commandID.String(),
		IPAddress:      &ip,
		UserAgent:      &ua,
		Status:         "SUCCESS",
		Metadata: map[string]interface{}{
			"device_id":    deviceID.String(),
			"hostname":     dev.Hostname,
			"command_type": req.CommandType,
		},
	})

	respondJSON(w, http.StatusAccepted, CreateCommandResponse{
		CommandID: commandID,
		DeviceID:  deviceID,
		Status:    "QUEUED",
		Nonce:     nonce,
		ExpiresAt: expiresAt,
	})
}

func (h *JobHandler) GetCommand(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.GetClaims(r.Context())
	if !ok {
		respondError(w, http.StatusUnauthorized, "Not authenticated")
		return
	}

	commandIDStr := r.PathValue("commandId")
	commandID, err := uuid.Parse(commandIDStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "Invalid command ID format")
		return
	}

	cmd, err := h.store.GetCommand(r.Context(), claims.OrganizationID, commandID)
	if err != nil {
		respondError(w, http.StatusNotFound, "Command job not found")
		return
	}

	result, _ := h.store.GetCommandResult(r.Context(), commandID)

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"command": cmd,
		"result":  result,
	})
}

func (h *JobHandler) CreateBulkCommand(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.GetClaims(r.Context())
	if !ok {
		respondError(w, http.StatusUnauthorized, "Not authenticated")
		return
	}

	var req BulkCommandRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.DeviceIDs) == 0 {
		respondError(w, http.StatusBadRequest, "Invalid JSON payload or empty device list")
		return
	}

	// Mandatory confirmation protection against accidental bulk disruptive execution
	if req.Confirmation != "CONFIRM_BULK_ACTION" {
		respondError(w, http.StatusBadRequest, "Bulk disruptive execution requires explicit confirmation string 'CONFIRM_BULK_ACTION'")
		return
	}

	ctx := r.Context()
	queuedIDs := make([]uuid.UUID, 0, len(req.DeviceIDs))

	for _, devID := range req.DeviceIDs {
		cmdID := uuid.New()
		nonceBytes := make([]byte, 16)
		_, _ = rand.Read(nonceBytes)
		nonce := hex.EncodeToString(nonceBytes)

		cmd := models.Command{
			ID:             cmdID,
			OrganizationID: claims.OrganizationID,
			DeviceID:       devID,
			IssuedByUserID: claims.UserID,
			CommandType:    req.CommandType,
			Parameters:     req.Parameters,
			Status:         "QUEUED",
			Nonce:          nonce,
			ExpiresAt:      time.Now().UTC().Add(10 * time.Minute),
			CreatedAt:      time.Now().UTC(),
		}
		_ = h.store.CreateCommand(ctx, cmd)
		queuedIDs = append(queuedIDs, cmdID)
	}

	ip := r.RemoteAddr
	ua := r.UserAgent()
	_ = h.store.InsertAuditLog(ctx, models.AuditLog{
		OrganizationID: claims.OrganizationID,
		ActorUserID:    &claims.UserID,
		Action:         "bulk.command.queued",
		ResourceType:   "bulk_command",
		ResourceID:     uuid.NewString(),
		IPAddress:      &ip,
		UserAgent:      &ua,
		Status:         "SUCCESS",
		Metadata: map[string]interface{}{
			"devices_targeted": len(req.DeviceIDs),
			"command_type":     req.CommandType,
		},
	})

	respondJSON(w, http.StatusAccepted, BulkCommandResponse{
		TotalQueued: len(queuedIDs),
		CommandIDs:  queuedIDs,
		Status:      "BATCH_QUEUED",
	})
}

// Agent reporting command execution result
func (h *JobHandler) ReportResult(w http.ResponseWriter, r *http.Request) {
	commandIDStr := r.PathValue("commandId")
	commandID, err := uuid.Parse(commandIDStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "Invalid command ID format")
		return
	}

	var req AgentCommandResultRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	ctx := r.Context()
	res := models.CommandResult{
		CommandID:           commandID,
		ExitCode:            req.ExitCode,
		StdoutOutput:        req.StdoutOutput,
		StderrOutput:        req.StderrOutput,
		ExecutionDurationMs: req.ExecutionDurationMs,
		CompletedAt:         time.Now().UTC(),
	}

	if err := h.store.SaveCommandResult(ctx, res); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to save command result")
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{
		"message": "Command result recorded",
	})
}
