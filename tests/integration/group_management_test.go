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

func setupGroupTestApp() (*store.MemoryStore, http.Handler, string) {
	jwtSecret := "group_test_secret_32_bytes_long_key_54321"
	st := store.NewMemoryStore()
	authMw := middleware.NewAuthMiddleware(jwtSecret)
	authH := handlers.NewAuthHandler(st, jwtSecret)
	groupH := handlers.NewGroupHandler(st)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/auth/login", authH.Login)

	mux.Handle("GET /api/v1/groups", authMw.Authenticate(
		authMw.RequirePermission("group.view", groupH.ListGroups),
	))
	mux.Handle("POST /api/v1/groups", authMw.Authenticate(
		authMw.RequirePermission("group.manage", groupH.CreateGroup),
	))
	mux.Handle("GET /api/v1/groups/{id}", authMw.Authenticate(
		authMw.RequirePermission("group.view", groupH.GetGroup),
	))
	mux.Handle("PATCH /api/v1/groups/{id}", authMw.Authenticate(
		authMw.RequirePermission("group.manage", groupH.UpdateGroup),
	))
	mux.Handle("DELETE /api/v1/groups/{id}", authMw.Authenticate(
		authMw.RequirePermission("group.manage", groupH.DeleteGroup),
	))
	mux.Handle("POST /api/v1/groups/{id}/devices", authMw.Authenticate(
		authMw.RequirePermission("group.assign_device", groupH.AssignDevices),
	))
	mux.Handle("DELETE /api/v1/groups/{id}/devices/{deviceId}", authMw.Authenticate(
		authMw.RequirePermission("group.assign_device", groupH.RemoveDevice),
	))

	return st, mux, jwtSecret
}

func TestGroupLifecycleAndHealthAggregation(t *testing.T) {
	st, handler, _ := setupGroupTestApp()
	adminToken := getAdminToken(t, handler)

	// 1. List pre-seeded groups and check health stats
	listReq := httptest.NewRequest("GET", "/api/v1/groups", nil)
	listReq.Header.Set("Authorization", "Bearer "+adminToken)
	listRec := httptest.NewRecorder()
	handler.ServeHTTP(listRec, listReq)

	if listRec.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK on list groups, got %d", listRec.Code)
	}

	type GroupItem struct {
		models.DeviceGroup
		Health *store.GroupHealthSummary `json:"health"`
	}
	var groups []GroupItem
	_ = json.NewDecoder(listRec.Body).Decode(&groups)

	if len(groups) < 2 {
		t.Fatalf("Expected at least 2 seeded groups, got %d", len(groups))
	}

	// Exam Lab 1 should have 2 online devices
	var examLab *GroupItem
	for _, g := range groups {
		if g.Name == "Exam Lab 1" {
			examLab = &g
			break
		}
	}
	if examLab == nil || examLab.Health == nil || examLab.Health.TotalDevices != 2 {
		t.Fatalf("Expected Exam Lab 1 to have 2 devices, got %+v", examLab)
	}

	// 2. Create a new group
	desc := "Third certification laboratory"
	createReq := handlers.CreateGroupRequest{
		Name:        "Exam Lab 3",
		Description: &desc,
		PolicyOverrides: map[string]interface{}{
			"prompt_for_remote_consent": true,
		},
	}
	cBody, _ := json.Marshal(createReq)
	cReq := httptest.NewRequest("POST", "/api/v1/groups", bytes.NewReader(cBody))
	cReq.Header.Set("Authorization", "Bearer "+adminToken)
	cReq.Header.Set("Content-Type", "application/json")
	cRec := httptest.NewRecorder()
	handler.ServeHTTP(cRec, cReq)

	if cRec.Code != http.StatusCreated {
		t.Fatalf("Expected 201 Created on group create, got %d", cRec.Code)
	}

	var newGroup models.DeviceGroup
	_ = json.NewDecoder(cRec.Body).Decode(&newGroup)

	// 3. Assign a device to Exam Lab 3
	devID := uuid.MustParse("90e722c1-bb3e-4623-a128-44fb6b907a01")
	assignReq := handlers.AssignDevicesRequest{
		DeviceIDs: []uuid.UUID{devID},
	}
	aBody, _ := json.Marshal(assignReq)
	aReq := httptest.NewRequest("POST", "/api/v1/groups/"+newGroup.ID.String()+"/devices", bytes.NewReader(aBody))
	aReq.Header.Set("Authorization", "Bearer "+adminToken)
	aReq.Header.Set("Content-Type", "application/json")
	aRec := httptest.NewRecorder()
	handler.ServeHTTP(aRec, aReq)

	if aRec.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK on device assignment, got %d", aRec.Code)
	}

	// 4. Verify group detail contains the device
	dReq := httptest.NewRequest("GET", "/api/v1/groups/"+newGroup.ID.String(), nil)
	dReq.Header.Set("Authorization", "Bearer "+adminToken)
	dRec := httptest.NewRecorder()
	handler.ServeHTTP(dRec, dReq)

	var detail handlers.GroupDetailResponse
	_ = json.NewDecoder(dRec.Body).Decode(&detail)
	if len(detail.Devices) != 1 || detail.Devices[0].ID != devID {
		t.Fatalf("Expected 1 assigned device with ID %s, got %+v", devID, detail.Devices)
	}

	// 5. Remove device from group
	remReq := httptest.NewRequest("DELETE", "/api/v1/groups/"+newGroup.ID.String()+"/devices/"+devID.String(), nil)
	remReq.Header.Set("Authorization", "Bearer "+adminToken)
	remRec := httptest.NewRecorder()
	handler.ServeHTTP(remRec, remReq)

	if remRec.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK on device removal, got %d", remRec.Code)
	}

	// 6. Delete group
	delReq := httptest.NewRequest("DELETE", "/api/v1/groups/"+newGroup.ID.String(), nil)
	delReq.Header.Set("Authorization", "Bearer "+adminToken)
	delRec := httptest.NewRecorder()
	handler.ServeHTTP(delRec, delReq)

	if delRec.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK on group delete, got %d", delRec.Code)
	}

	// Verify group is gone
	chkReq := httptest.NewRequest("GET", "/api/v1/groups/"+newGroup.ID.String(), nil)
	chkReq.Header.Set("Authorization", "Bearer "+adminToken)
	chkRec := httptest.NewRecorder()
	handler.ServeHTTP(chkRec, chkReq)

	if chkRec.Code != http.StatusNotFound {
		t.Fatalf("Expected 404 Not Found after deleting group, got %d", chkRec.Code)
	}

	_ = st
}
