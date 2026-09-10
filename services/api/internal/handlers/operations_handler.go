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

type OperationsHandler struct {
	store store.Store
}

func NewOperationsHandler(st store.Store) *OperationsHandler {
	return &OperationsHandler{store: st}
}

// 1. Alerts Endpoints
func (h *OperationsHandler) ListAlerts(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.GetClaims(r.Context())
	if !ok {
		respondError(w, http.StatusUnauthorized, "Not authenticated")
		return
	}

	alerts, err := h.store.ListAlerts(r.Context(), claims.OrganizationID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to list alerts")
		return
	}

	respondJSON(w, http.StatusOK, alerts)
}

func (h *OperationsHandler) AcknowledgeAlert(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.GetClaims(r.Context())
	if !ok {
		respondError(w, http.StatusUnauthorized, "Not authenticated")
		return
	}

	alertIDStr := r.PathValue("id")
	alertID, err := uuid.Parse(alertIDStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "Invalid alert ID format")
		return
	}

	ctx := r.Context()
	if err := h.store.AcknowledgeAlert(ctx, claims.OrganizationID, alertID, claims.UserID); err != nil {
		respondError(w, http.StatusNotFound, "Alert not found in organization")
		return
	}

	ip := r.RemoteAddr
	ua := r.UserAgent()
	_ = h.store.InsertAuditLog(ctx, models.AuditLog{
		OrganizationID: claims.OrganizationID,
		ActorUserID:    &claims.UserID,
		Action:         "alert.acknowledged",
		ResourceType:   "alert",
		ResourceID:     alertID.String(),
		IPAddress:      &ip,
		UserAgent:      &ua,
		Status:         "SUCCESS",
	})

	respondJSON(w, http.StatusOK, map[string]string{
		"message": "Alert successfully acknowledged",
	})
}

// 2. Software Packages & Deployments
type CreateSoftwareRequest struct {
	Name              string `json:"name"`
	Version           string `json:"version"`
	InstallerURL      string `json:"installer_url"`
	SHA256Hash        string `json:"sha256_hash"`
	SilentInstallArgs string `json:"silent_install_args"`
}

type DeploySoftwareRequest struct {
	DeviceIDs []uuid.UUID `json:"device_ids"`
}

func (h *OperationsHandler) ListSoftware(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.GetClaims(r.Context())
	if !ok {
		respondError(w, http.StatusUnauthorized, "Not authenticated")
		return
	}

	pkgs, err := h.store.ListSoftwarePackages(r.Context(), claims.OrganizationID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to list software packages")
		return
	}

	respondJSON(w, http.StatusOK, pkgs)
}

func (h *OperationsHandler) CreateSoftware(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.GetClaims(r.Context())
	if !ok {
		respondError(w, http.StatusUnauthorized, "Not authenticated")
		return
	}

	var req CreateSoftwareRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" || req.SHA256Hash == "" {
		respondError(w, http.StatusBadRequest, "Invalid JSON payload or missing name/hash")
		return
	}

	pkgID := uuid.New()
	pkg := models.SoftwarePackage{
		ID:                pkgID,
		OrganizationID:    claims.OrganizationID,
		Name:              req.Name,
		Version:           req.Version,
		InstallerURL:      req.InstallerURL,
		SHA256Hash:        req.SHA256Hash,
		SilentInstallArgs: req.SilentInstallArgs,
		CreatedAt:         time.Now().UTC(),
	}

	if err := h.store.CreateSoftwarePackage(r.Context(), pkg); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to register software package")
		return
	}

	respondJSON(w, http.StatusCreated, pkg)
}

func (h *OperationsHandler) DeploySoftware(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.GetClaims(r.Context())
	if !ok {
		respondError(w, http.StatusUnauthorized, "Not authenticated")
		return
	}

	pkgIDStr := r.PathValue("id")
	pkgID, err := uuid.Parse(pkgIDStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "Invalid software package ID format")
		return
	}

	var req DeploySoftwareRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.DeviceIDs) == 0 {
		respondError(w, http.StatusBadRequest, "Invalid JSON payload or empty device list")
		return
	}

	ctx := r.Context()
	for _, devID := range req.DeviceIDs {
		depID := uuid.New()
		dep := models.SoftwareDeployment{
			ID:             depID,
			OrganizationID: claims.OrganizationID,
			PackageID:      pkgID,
			DeviceID:       devID,
			Status:         "QUEUED",
			CreatedAt:      time.Now().UTC(),
		}
		_ = h.store.CreateDeployment(ctx, dep)
	}

	respondJSON(w, http.StatusAccepted, map[string]interface{}{
		"message":  "Software deployment initiated across target workstations",
		"targeted": len(req.DeviceIDs),
	})
}

// 3. Licensing & Entitlements
func (h *OperationsHandler) GetCurrentLicense(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.GetClaims(r.Context())
	if !ok {
		respondError(w, http.StatusUnauthorized, "Not authenticated")
		return
	}

	lic, err := h.store.GetLicense(r.Context(), claims.OrganizationID)
	if err != nil {
		respondError(w, http.StatusNotFound, "No active license found for organization")
		return
	}

	devices, _ := h.store.ListDevices(r.Context(), claims.OrganizationID)

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"license":         lic,
		"enrolled_count":  len(devices),
		"device_limit":    lic.MaxDevices,
		"seats_remaining": lic.MaxDevices - len(devices),
		"in_grace_period": lic.Status == "GRACE_PERIOD",
	})
}
