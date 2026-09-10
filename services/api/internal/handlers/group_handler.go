package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/controlhub/controlhub/services/api/internal/middleware"
	"github.com/controlhub/controlhub/services/api/internal/models"
	"github.com/controlhub/controlhub/services/api/internal/store"
	"github.com/google/uuid"
)

type GroupHandler struct {
	store store.Store
}

func NewGroupHandler(st store.Store) *GroupHandler {
	return &GroupHandler{store: st}
}

type CreateGroupRequest struct {
	Name            string                 `json:"name"`
	Description     *string                `json:"description,omitempty"`
	ParentGroupID   *uuid.UUID             `json:"parent_group_id,omitempty"`
	PolicyOverrides map[string]interface{} `json:"policy_overrides,omitempty"`
}

type UpdateGroupRequest struct {
	Name            string                 `json:"name,omitempty"`
	Description     *string                `json:"description,omitempty"`
	PolicyOverrides map[string]interface{} `json:"policy_overrides,omitempty"`
}

type AssignDevicesRequest struct {
	DeviceIDs []uuid.UUID `json:"device_ids"`
}

type GroupDetailResponse struct {
	Group   *models.DeviceGroup       `json:"group"`
	Health  *store.GroupHealthSummary `json:"health"`
	Devices []models.Device           `json:"devices"`
}

func (h *GroupHandler) ListGroups(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.GetClaims(r.Context())
	if !ok {
		respondError(w, http.StatusUnauthorized, "Not authenticated")
		return
	}

	ctx := r.Context()
	groups, err := h.store.ListGroups(ctx, claims.OrganizationID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to list groups")
		return
	}

	type GroupListItem struct {
		models.DeviceGroup
		Health *store.GroupHealthSummary `json:"health"`
	}

	result := make([]GroupListItem, 0, len(groups))
	for _, g := range groups {
		health, _ := h.store.GetGroupHealthSummary(ctx, claims.OrganizationID, g.ID)
		result = append(result, GroupListItem{
			DeviceGroup: g,
			Health:      health,
		})
	}

	respondJSON(w, http.StatusOK, result)
}

func (h *GroupHandler) CreateGroup(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.GetClaims(r.Context())
	if !ok {
		respondError(w, http.StatusUnauthorized, "Not authenticated")
		return
	}

	var req CreateGroupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		respondError(w, http.StatusBadRequest, "Invalid JSON payload or missing group name")
		return
	}

	groupID := uuid.New()
	grp := models.DeviceGroup{
		ID:              groupID,
		OrganizationID:  claims.OrganizationID,
		ParentGroupID:   req.ParentGroupID,
		Name:            req.Name,
		Description:     req.Description,
		PolicyOverrides: req.PolicyOverrides,
	}

	ctx := r.Context()
	if err := h.store.CreateGroup(ctx, grp); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to create group")
		return
	}

	ip := r.RemoteAddr
	ua := r.UserAgent()
	_ = h.store.InsertAuditLog(ctx, models.AuditLog{
		OrganizationID: claims.OrganizationID,
		ActorUserID:    &claims.UserID,
		Action:         "group.created",
		ResourceType:   "device_group",
		ResourceID:     groupID.String(),
		IPAddress:      &ip,
		UserAgent:      &ua,
		Status:         "SUCCESS",
		Metadata: map[string]interface{}{
			"group_name": req.Name,
		},
	})

	respondJSON(w, http.StatusCreated, grp)
}

func (h *GroupHandler) GetGroup(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.GetClaims(r.Context())
	if !ok {
		respondError(w, http.StatusUnauthorized, "Not authenticated")
		return
	}

	groupIDStr := r.PathValue("id")
	groupID, err := uuid.Parse(groupIDStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "Invalid group ID format")
		return
	}

	ctx := r.Context()
	grp, err := h.store.GetGroup(ctx, claims.OrganizationID, groupID)
	if err != nil {
		respondError(w, http.StatusNotFound, "Group not found in organization")
		return
	}

	health, _ := h.store.GetGroupHealthSummary(ctx, claims.OrganizationID, groupID)
	devices, _ := h.store.GetGroupDevices(ctx, claims.OrganizationID, groupID)

	respondJSON(w, http.StatusOK, GroupDetailResponse{
		Group:   grp,
		Health:  health,
		Devices: devices,
	})
}

func (h *GroupHandler) UpdateGroup(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.GetClaims(r.Context())
	if !ok {
		respondError(w, http.StatusUnauthorized, "Not authenticated")
		return
	}

	groupIDStr := r.PathValue("id")
	groupID, err := uuid.Parse(groupIDStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "Invalid group ID format")
		return
	}

	var req UpdateGroupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	ctx := r.Context()
	if err := h.store.UpdateGroup(ctx, claims.OrganizationID, groupID, req.Name, req.Description, req.PolicyOverrides); err != nil {
		respondError(w, http.StatusNotFound, "Group not found or update failed")
		return
	}

	ip := r.RemoteAddr
	ua := r.UserAgent()
	_ = h.store.InsertAuditLog(ctx, models.AuditLog{
		OrganizationID: claims.OrganizationID,
		ActorUserID:    &claims.UserID,
		Action:         "group.updated",
		ResourceType:   "device_group",
		ResourceID:     groupID.String(),
		IPAddress:      &ip,
		UserAgent:      &ua,
		Status:         "SUCCESS",
	})

	respondJSON(w, http.StatusOK, map[string]string{
		"message": "Group successfully updated",
	})
}

func (h *GroupHandler) DeleteGroup(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.GetClaims(r.Context())
	if !ok {
		respondError(w, http.StatusUnauthorized, "Not authenticated")
		return
	}

	groupIDStr := r.PathValue("id")
	groupID, err := uuid.Parse(groupIDStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "Invalid group ID format")
		return
	}

	ctx := r.Context()
	if err := h.store.DeleteGroup(ctx, claims.OrganizationID, groupID); err != nil {
		respondError(w, http.StatusNotFound, "Group not found in organization")
		return
	}

	ip := r.RemoteAddr
	ua := r.UserAgent()
	_ = h.store.InsertAuditLog(ctx, models.AuditLog{
		OrganizationID: claims.OrganizationID,
		ActorUserID:    &claims.UserID,
		Action:         "group.deleted",
		ResourceType:   "device_group",
		ResourceID:     groupID.String(),
		IPAddress:      &ip,
		UserAgent:      &ua,
		Status:         "SUCCESS",
	})

	respondJSON(w, http.StatusOK, map[string]string{
		"message": "Group successfully deleted",
	})
}

func (h *GroupHandler) AssignDevices(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.GetClaims(r.Context())
	if !ok {
		respondError(w, http.StatusUnauthorized, "Not authenticated")
		return
	}

	groupIDStr := r.PathValue("id")
	groupID, err := uuid.Parse(groupIDStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "Invalid group ID format")
		return
	}

	var req AssignDevicesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.DeviceIDs) == 0 {
		respondError(w, http.StatusBadRequest, "Invalid JSON payload or empty device_ids list")
		return
	}

	ctx := r.Context()
	for _, devID := range req.DeviceIDs {
		_ = h.store.AssignDeviceToGroup(ctx, claims.OrganizationID, groupID, devID)
	}

	ip := r.RemoteAddr
	ua := r.UserAgent()
	_ = h.store.InsertAuditLog(ctx, models.AuditLog{
		OrganizationID: claims.OrganizationID,
		ActorUserID:    &claims.UserID,
		Action:         "group.assign_device",
		ResourceType:   "device_group",
		ResourceID:     groupID.String(),
		IPAddress:      &ip,
		UserAgent:      &ua,
		Status:         "SUCCESS",
		Metadata: map[string]interface{}{
			"devices_assigned": len(req.DeviceIDs),
		},
	})

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"message":  "Devices assigned to group",
		"assigned": len(req.DeviceIDs),
	})
}

func (h *GroupHandler) RemoveDevice(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.GetClaims(r.Context())
	if !ok {
		respondError(w, http.StatusUnauthorized, "Not authenticated")
		return
	}

	groupIDStr := r.PathValue("id")
	groupID, err := uuid.Parse(groupIDStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "Invalid group ID format")
		return
	}

	devIDStr := r.PathValue("deviceId")
	devID, err := uuid.Parse(devIDStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "Invalid device ID format")
		return
	}

	ctx := r.Context()
	if err := h.store.RemoveDeviceFromGroup(ctx, claims.OrganizationID, groupID, devID); err != nil {
		respondError(w, http.StatusNotFound, "Device or group not found")
		return
	}

	ip := r.RemoteAddr
	ua := r.UserAgent()
	_ = h.store.InsertAuditLog(ctx, models.AuditLog{
		OrganizationID: claims.OrganizationID,
		ActorUserID:    &claims.UserID,
		Action:         "group.remove_device",
		ResourceType:   "device_group",
		ResourceID:     groupID.String(),
		IPAddress:      &ip,
		UserAgent:      &ua,
		Status:         "SUCCESS",
		Metadata: map[string]interface{}{
			"device_id": devID.String(),
		},
	})

	respondJSON(w, http.StatusOK, map[string]string{
		"message": "Device removed from group",
	})
}
