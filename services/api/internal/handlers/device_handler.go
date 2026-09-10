package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/controlhub/controlhub/services/api/internal/middleware"
	"github.com/controlhub/controlhub/services/api/internal/models"
	"github.com/controlhub/controlhub/services/api/internal/store"
	"github.com/google/uuid"
)

type DeviceHandler struct {
	store store.Store
}

func NewDeviceHandler(st store.Store) *DeviceHandler {
	return &DeviceHandler{store: st}
}

func (h *DeviceHandler) ListDevices(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.GetClaims(r.Context())
	if !ok {
		respondError(w, http.StatusUnauthorized, "Not authenticated")
		return
	}

	devices, err := h.store.ListDevices(r.Context(), claims.OrganizationID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to query devices")
		return
	}

	respondJSON(w, http.StatusOK, devices)
}

func (h *DeviceHandler) GetDevice(w http.ResponseWriter, r *http.Request) {
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

	dev, err := h.store.GetDevice(r.Context(), claims.OrganizationID, deviceID)
	if err != nil {
		respondError(w, http.StatusNotFound, "Device not found in organization inventory")
		return
	}

	respondJSON(w, http.StatusOK, dev)
}

func (h *DeviceHandler) ApproveDevice(w http.ResponseWriter, r *http.Request) {
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

	dev, err := h.store.GetDevice(r.Context(), claims.OrganizationID, deviceID)
	if err != nil {
		respondError(w, http.StatusNotFound, "Device not found in organization inventory")
		return
	}

	if dev.Status != models.DeviceStatusPending {
		respondError(w, http.StatusConflict, "Device is not in PENDING state")
		return
	}

	if err := h.store.UpdateDeviceStatus(r.Context(), claims.OrganizationID, deviceID, models.DeviceStatusActive); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to update device status")
		return
	}

	// Record audit
	ip := r.RemoteAddr
	ua := r.UserAgent()
	_ = h.store.InsertAuditLog(r.Context(), models.AuditLog{
		OrganizationID: claims.OrganizationID,
		ActorUserID:    &claims.UserID,
		Action:         "device.approved",
		ResourceType:   "device",
		ResourceID:     deviceID.String(),
		IPAddress:      &ip,
		UserAgent:      &ua,
		Status:         "SUCCESS",
		Metadata: map[string]interface{}{
			"hostname": dev.Hostname,
		},
	})

	respondJSON(w, http.StatusOK, map[string]string{
		"message": "Device successfully approved and activated",
	})
}

func (h *DeviceHandler) RevokeDevice(w http.ResponseWriter, r *http.Request) {
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

	if err := h.store.UpdateDeviceStatus(r.Context(), claims.OrganizationID, deviceID, models.DeviceStatusRevoked); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to revoke device")
		return
	}

	ip := r.RemoteAddr
	ua := r.UserAgent()
	_ = h.store.InsertAuditLog(r.Context(), models.AuditLog{
		OrganizationID: claims.OrganizationID,
		ActorUserID:    &claims.UserID,
		Action:         "device.revoked",
		ResourceType:   "device",
		ResourceID:     deviceID.String(),
		IPAddress:      &ip,
		UserAgent:      &ua,
		Status:         "SUCCESS",
	})

	respondJSON(w, http.StatusOK, map[string]string{
		"message": "Device credentials revoked",
	})
}

func (h *DeviceHandler) ListAuditLogs(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.GetClaims(r.Context())
	if !ok {
		respondError(w, http.StatusUnauthorized, "Not authenticated")
		return
	}

	limit := 50
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}

	logs, err := h.store.ListAuditLogs(r.Context(), claims.OrganizationID, limit)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to retrieve audit logs")
		return
	}

	respondJSON(w, http.StatusOK, logs)
}
