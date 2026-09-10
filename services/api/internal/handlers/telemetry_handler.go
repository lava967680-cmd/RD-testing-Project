package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/controlhub/controlhub/services/api/internal/middleware"
	"github.com/controlhub/controlhub/services/api/internal/models"
	"github.com/controlhub/controlhub/services/api/internal/store"
	"github.com/google/uuid"
)

type TelemetryHandler struct {
	store store.Store
}

func NewTelemetryHandler(st store.Store) *TelemetryHandler {
	return &TelemetryHandler{store: st}
}

type TelemetryResponse struct {
	DeviceID   uuid.UUID                 `json:"device_id"`
	Latest     *models.DeviceTelemetry   `json:"latest,omitempty"`
	History    []models.DeviceTelemetry  `json:"history,omitempty"`
	Status     models.DeviceStatus       `json:"status"`
	LastSeen   *time.Time                `json:"last_seen,omitempty"`
}

type IngestTelemetryRequest struct {
	DeviceID          uuid.UUID `json:"device_id"`
	OrganizationID    uuid.UUID `json:"organization_id"`
	CPUPercent        float64   `json:"cpu_percent"`
	RAMUsedBytes      int64     `json:"ram_used_bytes"`
	RAMTotalBytes     int64     `json:"ram_total_bytes"`
	RAMPercent        float64   `json:"ram_percent"`
	DiskUsedBytes     int64     `json:"disk_used_bytes"`
	DiskTotalBytes    int64     `json:"disk_total_bytes"`
	DiskPercent       float64   `json:"disk_percent"`
	NetworkRxBytesSec int64     `json:"network_rx_bytes_sec"`
	NetworkTxBytesSec int64     `json:"network_tx_bytes_sec"`
}

func (h *TelemetryHandler) GetDeviceTelemetry(w http.ResponseWriter, r *http.Request) {
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

	latest, _ := h.store.GetLatestTelemetry(ctx, claims.OrganizationID, deviceID)

	includeHistory := r.URL.Query().Get("history") == "true"
	var history []models.DeviceTelemetry
	if includeHistory {
		limit := 30
		if lStr := r.URL.Query().Get("limit"); lStr != "" {
			if l, err := strconv.Atoi(lStr); err == nil && l > 0 && l <= 100 {
				limit = l
			}
		}
		history, _ = h.store.GetTelemetryHistory(ctx, claims.OrganizationID, deviceID, limit)
	}

	respondJSON(w, http.StatusOK, TelemetryResponse{
		DeviceID: deviceID,
		Latest:   latest,
		History:  history,
		Status:   dev.Status,
		LastSeen: dev.LastSeen,
	})
}

func (h *TelemetryHandler) IngestTelemetry(w http.ResponseWriter, r *http.Request) {
	var req IngestTelemetryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid JSON body")
		return
	}

	ctx := r.Context()
	t := models.DeviceTelemetry{
		OrganizationID:    req.OrganizationID,
		DeviceID:          req.DeviceID,
		CPUPercent:        req.CPUPercent,
		RAMUsedBytes:      req.RAMUsedBytes,
		RAMTotalBytes:     req.RAMTotalBytes,
		RAMPercent:        req.RAMPercent,
		DiskUsedBytes:     req.DiskUsedBytes,
		DiskTotalBytes:    req.DiskTotalBytes,
		DiskPercent:       req.DiskPercent,
		NetworkRxBytesSec: req.NetworkRxBytesSec,
		NetworkTxBytesSec: req.NetworkTxBytesSec,
		RecordedAt:        time.Now().UTC(),
	}

	if err := h.store.SaveTelemetry(ctx, t); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to persist telemetry")
		return
	}

	respondJSON(w, http.StatusCreated, map[string]string{
		"status": "ingested",
	})
}
