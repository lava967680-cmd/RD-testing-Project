package store

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/controlhub/controlhub/services/api/internal/models"
	"github.com/google/uuid"
)

var (
	ErrNotFound       = errors.New("record not found")
	ErrDuplicateKey   = errors.New("record already exists with this key")
	ErrUnauthorized   = errors.New("unauthorized access to tenant resource")
	ErrTokenExpired   = errors.New("enrollment invitation has expired")
	ErrTokenExhausted = errors.New("enrollment invitation usage limit exceeded")
)

type RefreshToken struct {
	Token          string
	UserID         uuid.UUID
	OrganizationID uuid.UUID
	ExpiresAt      time.Time
	Revoked        bool
}

type GroupHealthSummary struct {
	GroupID        uuid.UUID `json:"group_id"`
	GroupName      string    `json:"group_name"`
	TotalDevices   int       `json:"total_devices"`
	OnlineDevices  int       `json:"online_devices"`
	OfflineDevices int       `json:"offline_devices"`
	AvgCPUPercent  float64   `json:"avg_cpu_percent"`
	AvgRAMPercent  float64   `json:"avg_ram_percent"`
}

type Store interface {
	// Organizations
	GetOrganization(ctx context.Context, orgID uuid.UUID) (*models.Organization, error)
	
	// Users & Auth
	GetUserByEmail(ctx context.Context, email string) (*models.User, error)
	GetUserByID(ctx context.Context, userID uuid.UUID) (*models.User, error)
	UpdateUserMFA(ctx context.Context, userID uuid.UUID, enabled bool, secret *string) error
	GetUserPermissions(ctx context.Context, userID uuid.UUID) ([]string, string, error)

	// Refresh Tokens
	SaveRefreshToken(ctx context.Context, token RefreshToken) error
	GetRefreshToken(ctx context.Context, token string) (*RefreshToken, error)
	RevokeRefreshToken(ctx context.Context, token string) error

	// Devices
	ListDevices(ctx context.Context, orgID uuid.UUID) ([]models.Device, error)
	GetDevice(ctx context.Context, orgID, deviceID uuid.UUID) (*models.Device, error)
	UpdateDeviceStatus(ctx context.Context, orgID, deviceID uuid.UUID, status models.DeviceStatus) error
	RegisterDevice(ctx context.Context, dev models.Device, pubKey []byte) error

	// Device Groups
	CreateGroup(ctx context.Context, grp models.DeviceGroup) error
	GetGroup(ctx context.Context, orgID, groupID uuid.UUID) (*models.DeviceGroup, error)
	ListGroups(ctx context.Context, orgID uuid.UUID) ([]models.DeviceGroup, error)
	UpdateGroup(ctx context.Context, orgID, groupID uuid.UUID, name string, desc *string, policies map[string]interface{}) error
	DeleteGroup(ctx context.Context, orgID, groupID uuid.UUID) error
	AssignDeviceToGroup(ctx context.Context, orgID, groupID, deviceID uuid.UUID) error
	RemoveDeviceFromGroup(ctx context.Context, orgID, groupID, deviceID uuid.UUID) error
	GetGroupDevices(ctx context.Context, orgID, groupID uuid.UUID) ([]models.Device, error)
	GetGroupHealthSummary(ctx context.Context, orgID, groupID uuid.UUID) (*GroupHealthSummary, error)

	// Enrollment Invitations
	CreateEnrollmentInvitation(ctx context.Context, inv models.EnrollmentInvitation) error
	GetEnrollmentInvitationByHash(ctx context.Context, tokenHash string) (*models.EnrollmentInvitation, error)
	ConsumeEnrollmentInvitation(ctx context.Context, invID uuid.UUID) error
	ListEnrollmentInvitations(ctx context.Context, orgID uuid.UUID) ([]models.EnrollmentInvitation, error)
	DeleteEnrollmentInvitation(ctx context.Context, orgID, invID uuid.UUID) error

	// Telemetry
	SaveTelemetry(ctx context.Context, t models.DeviceTelemetry) error
	GetLatestTelemetry(ctx context.Context, orgID, deviceID uuid.UUID) (*models.DeviceTelemetry, error)
	GetTelemetryHistory(ctx context.Context, orgID, deviceID uuid.UUID, limit int) ([]models.DeviceTelemetry, error)

	// Remote Sessions
	CreateRemoteSession(ctx context.Context, session models.RemoteSession) error
	GetRemoteSession(ctx context.Context, orgID, sessionID uuid.UUID) (*models.RemoteSession, error)
	UpdateRemoteSessionStatus(ctx context.Context, orgID, sessionID uuid.UUID, status string, reason *string) error
	ListRemoteSessions(ctx context.Context, orgID uuid.UUID) ([]models.RemoteSession, error)

	// Controlled Commands / Jobs
	CreateCommand(ctx context.Context, cmd models.Command) error
	GetCommand(ctx context.Context, orgID, commandID uuid.UUID) (*models.Command, error)
	UpdateCommandStatus(ctx context.Context, orgID, commandID uuid.UUID, status string) error
	ListDeviceCommands(ctx context.Context, orgID, deviceID uuid.UUID) ([]models.Command, error)
	SaveCommandResult(ctx context.Context, res models.CommandResult) error
	GetCommandResult(ctx context.Context, commandID uuid.UUID) (*models.CommandResult, error)

	// Alerts
	CreateAlert(ctx context.Context, alert models.Alert) error
	ListAlerts(ctx context.Context, orgID uuid.UUID) ([]models.Alert, error)
	AcknowledgeAlert(ctx context.Context, orgID, alertID, userID uuid.UUID) error

	// Software Packages & Deployments
	CreateSoftwarePackage(ctx context.Context, pkg models.SoftwarePackage) error
	ListSoftwarePackages(ctx context.Context, orgID uuid.UUID) ([]models.SoftwarePackage, error)
	CreateDeployment(ctx context.Context, dep models.SoftwareDeployment) error
	ListDeployments(ctx context.Context, orgID uuid.UUID) ([]models.SoftwareDeployment, error)

	// Licensing
	GetLicense(ctx context.Context, orgID uuid.UUID) (*models.License, error)
	SaveLicense(ctx context.Context, lic models.License) error

	// Audit Logs
	InsertAuditLog(ctx context.Context, log models.AuditLog) error
	ListAuditLogs(ctx context.Context, orgID uuid.UUID, limit int) ([]models.AuditLog, error)
}

type MemoryStore struct {
	mu            sync.RWMutex
	orgs          map[uuid.UUID]*models.Organization
	users         map[uuid.UUID]*models.User
	userRoles     map[uuid.UUID]string
	rolePerms     map[string][]string
	refreshTokens map[string]RefreshToken
	devices       map[uuid.UUID]*models.Device
	deviceCreds   map[uuid.UUID][]byte
	groups        map[uuid.UUID]*models.DeviceGroup
	groupMembers  map[uuid.UUID]map[uuid.UUID]bool
	invitations   map[uuid.UUID]*models.EnrollmentInvitation
	telemetry     map[uuid.UUID][]models.DeviceTelemetry
	sessions      map[uuid.UUID]*models.RemoteSession
	commands      map[uuid.UUID]*models.Command
	commandResults map[uuid.UUID]*models.CommandResult
	alerts        map[uuid.UUID]*models.Alert
	softwarePkgs  map[uuid.UUID]*models.SoftwarePackage
	deployments   map[uuid.UUID]*models.SoftwareDeployment
	licenses      map[uuid.UUID]*models.License
	auditLogs     []models.AuditLog
}

func NewMemoryStore() *MemoryStore {
	demoOrgID := uuid.MustParse("771e8bfb-cf98-4c28-98e3-0d6e6443c21a")
	demoAdminID := uuid.MustParse("c1f72776-9ec6-4f40-8b17-7435f3dfd720")

	rolePerms := map[string][]string{
		"Owner": {
			"organization.view", "organization.manage", "user.view", "user.manage",
			"device.view", "device.enroll", "device.approve", "device.remove",
			"device.remote", "device.file_transfer", "device.terminal", "device.restart",
			"device.software_manage", "group.view", "group.manage", "group.assign_device",
			"alert.view", "alert.manage", "audit.view", "license.view", "license.manage",
		},
		"Administrator": {
			"organization.view", "user.view", "user.manage",
			"device.view", "device.enroll", "device.approve", "device.remove",
			"device.remote", "device.file_transfer", "device.terminal", "device.restart",
			"device.software_manage", "group.view", "group.manage", "group.assign_device",
			"alert.view", "alert.manage", "audit.view", "license.view",
		},
		"IT Support": {
			"organization.view", "device.view", "device.remote", "device.file_transfer",
			"device.terminal", "device.restart", "group.view", "group.assign_device",
			"alert.view", "alert.manage",
		},
		"Operator": {
			"organization.view", "device.view", "group.view", "alert.view", "alert.manage",
		},
		"Viewer": {
			"organization.view", "device.view", "group.view", "alert.view",
		},
	}

	ms := &MemoryStore{
		orgs:           make(map[uuid.UUID]*models.Organization),
		users:          make(map[uuid.UUID]*models.User),
		userRoles:      make(map[uuid.UUID]string),
		rolePerms:      rolePerms,
		refreshTokens:  make(map[string]RefreshToken),
		devices:        make(map[uuid.UUID]*models.Device),
		deviceCreds:    make(map[uuid.UUID][]byte),
		groups:         make(map[uuid.UUID]*models.DeviceGroup),
		groupMembers:   make(map[uuid.UUID]map[uuid.UUID]bool),
		invitations:    make(map[uuid.UUID]*models.EnrollmentInvitation),
		telemetry:      make(map[uuid.UUID][]models.DeviceTelemetry),
		sessions:       make(map[uuid.UUID]*models.RemoteSession),
		commands:       make(map[uuid.UUID]*models.Command),
		commandResults: make(map[uuid.UUID]*models.CommandResult),
		alerts:         make(map[uuid.UUID]*models.Alert),
		softwarePkgs:   make(map[uuid.UUID]*models.SoftwarePackage),
		deployments:    make(map[uuid.UUID]*models.SoftwareDeployment),
		licenses:       make(map[uuid.UUID]*models.License),
		auditLogs:      make([]models.AuditLog, 0),
	}

	// Seed Demo Organization
	ms.orgs[demoOrgID] = &models.Organization{
		ID:        demoOrgID,
		Name:      "Acme IT Academy",
		Slug:      "acme-academy",
		Status:    "ACTIVE",
		Settings:  map[string]interface{}{"require_remote_consent": true, "telemetry_interval_sec": 15},
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	// Seed Administrator (Password: AdminSecure123!)
	ms.users[demoAdminID] = &models.User{
		ID:             demoAdminID,
		OrganizationID: demoOrgID,
		Email:          "admin@acme-academy.org",
		PasswordHash:   "$argon2id$v=19$m=65536,t=3,p=4$dGVzdHNhbHQxMjM0NTY3OA$mJ0q/eH8gK4K5o4b0N7tTq1Jq0d2S8v7Y3m2k1p0e4E",
		FullName:       "Lab Administrator",
		MFAEnabled:     false,
		Status:         "ACTIVE",
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
	}
	ms.userRoles[demoAdminID] = "Administrator"

	// Seed Devices
	dev1ID := uuid.MustParse("90e722c1-bb3e-4623-a128-44fb6b907a01")
	dev2ID := uuid.MustParse("90e722c1-bb3e-4623-a128-44fb6b907a02")
	fName1 := "Exam Station 01"
	fName2 := "Exam Station 02"
	now := time.Now().UTC()

	ms.devices[dev1ID] = &models.Device{
		ID:             dev1ID,
		OrganizationID: demoOrgID,
		Hostname:       "EXAM-PC-01",
		FriendlyName:   &fName1,
		OSName:         "Windows 11 Pro",
		OSVersion:      "10.0.22631",
		OSArchitecture: "x86_64",
		AgentVersion:   "1.0.0",
		Status:         models.DeviceStatusActive,
		LastSeen:       &now,
		UptimeSeconds:  124000,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	ms.devices[dev2ID] = &models.Device{
		ID:             dev2ID,
		OrganizationID: demoOrgID,
		Hostname:       "EXAM-PC-02",
		FriendlyName:   &fName2,
		OSName:         "Windows 11 Pro",
		OSVersion:      "10.0.22631",
		OSArchitecture: "x86_64",
		AgentVersion:   "1.0.0",
		Status:         models.DeviceStatusActive,
		LastSeen:       &now,
		UptimeSeconds:  120000,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	// Seed Demo License (50-Device Plan)
	licID := uuid.MustParse("990184a2-9c01-4433-8822-110099887766")
	ms.licenses[demoOrgID] = &models.License{
		ID:              licID,
		OrganizationID:  demoOrgID,
		PlanCode:        "PLAN_50",
		MaxDevices:      50,
		Features:        []string{"remote_control", "bulk_management", "file_transfer", "software_deploy", "audit_logs"},
		ValidFrom:       now.Add(-30 * 24 * time.Hour),
		ValidUntil:      now.Add(335 * 24 * time.Hour),
		GracePeriodDays: 7,
		Status:          "ACTIVE",
	}

	// Seed Demo Alert
	alertID := uuid.MustParse("aa11bb22-cc33-44dd-55ee-66ff77889900")
	ms.alerts[alertID] = &models.Alert{
		ID:             alertID,
		OrganizationID: demoOrgID,
		DeviceID:       &dev1ID,
		Severity:       "WARNING",
		AlertType:      "CPU_HIGH",
		Title:          "High CPU Usage Warning",
		Details:        "Workstation EXAM-PC-01 has maintained 89% CPU usage for > 5 minutes.",
		Acknowledged:   false,
		CreatedAt:      now.Add(-10 * time.Minute),
	}

	return ms
}

// User & Auth
func (m *MemoryStore) GetOrganization(ctx context.Context, orgID uuid.UUID) (*models.Organization, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	org, exists := m.orgs[orgID]
	if !exists {
		return nil, ErrNotFound
	}
	return org, nil
}

func (m *MemoryStore) GetUserByEmail(ctx context.Context, email string) (*models.User, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, u := range m.users {
		if u.Email == email {
			return u, nil
		}
	}
	return nil, ErrNotFound
}

func (m *MemoryStore) GetUserByID(ctx context.Context, userID uuid.UUID) (*models.User, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	u, exists := m.users[userID]
	if !exists {
		return nil, ErrNotFound
	}
	return u, nil
}

func (m *MemoryStore) UpdateUserMFA(ctx context.Context, userID uuid.UUID, enabled bool, secret *string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	u, exists := m.users[userID]
	if !exists {
		return ErrNotFound
	}
	u.MFAEnabled = enabled
	u.MFASecret = secret
	return nil
}

func (m *MemoryStore) GetUserPermissions(ctx context.Context, userID uuid.UUID) ([]string, string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	role, exists := m.userRoles[userID]
	if !exists {
		return nil, "", ErrNotFound
	}
	return m.rolePerms[role], role, nil
}

func (m *MemoryStore) SaveRefreshToken(ctx context.Context, token RefreshToken) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.refreshTokens[token.Token] = token
	return nil
}

func (m *MemoryStore) GetRefreshToken(ctx context.Context, token string) (*RefreshToken, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	rt, exists := m.refreshTokens[token]
	if !exists || rt.Revoked || time.Now().UTC().After(rt.ExpiresAt) {
		return nil, ErrNotFound
	}
	return &rt, nil
}

func (m *MemoryStore) RevokeRefreshToken(ctx context.Context, token string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if rt, exists := m.refreshTokens[token]; exists {
		rt.Revoked = true
		m.refreshTokens[token] = rt
	}
	return nil
}

// Devices
func (m *MemoryStore) ListDevices(ctx context.Context, orgID uuid.UUID) ([]models.Device, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]models.Device, 0)
	for _, dev := range m.devices {
		if dev.OrganizationID == orgID {
			result = append(result, *dev)
		}
	}
	return result, nil
}

func (m *MemoryStore) GetDevice(ctx context.Context, orgID, deviceID uuid.UUID) (*models.Device, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	dev, exists := m.devices[deviceID]
	if !exists || dev.OrganizationID != orgID {
		return nil, ErrNotFound
	}
	return dev, nil
}

func (m *MemoryStore) UpdateDeviceStatus(ctx context.Context, orgID, deviceID uuid.UUID, status models.DeviceStatus) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	dev, exists := m.devices[deviceID]
	if !exists || dev.OrganizationID != orgID {
		return ErrNotFound
	}
	dev.Status = status
	return nil
}

func (m *MemoryStore) RegisterDevice(ctx context.Context, dev models.Device, pubKey []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.devices[dev.ID] = &dev
	m.deviceCreds[dev.ID] = pubKey
	return nil
}

// Groups
func (m *MemoryStore) CreateGroup(ctx context.Context, grp models.DeviceGroup) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.groups[grp.ID] = &grp
	m.groupMembers[grp.ID] = make(map[uuid.UUID]bool)
	return nil
}

func (m *MemoryStore) GetGroup(ctx context.Context, orgID, groupID uuid.UUID) (*models.DeviceGroup, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	grp, exists := m.groups[groupID]
	if !exists || grp.OrganizationID != orgID {
		return nil, ErrNotFound
	}
	return grp, nil
}

func (m *MemoryStore) ListGroups(ctx context.Context, orgID uuid.UUID) ([]models.DeviceGroup, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]models.DeviceGroup, 0)
	for _, grp := range m.groups {
		if grp.OrganizationID == orgID {
			result = append(result, *grp)
		}
	}
	return result, nil
}

func (m *MemoryStore) UpdateGroup(ctx context.Context, orgID, groupID uuid.UUID, name string, desc *string, policies map[string]interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	grp, exists := m.groups[groupID]
	if !exists || grp.OrganizationID != orgID {
		return ErrNotFound
	}
	if name != "" {
		grp.Name = name
	}
	if desc != nil {
		grp.Description = desc
	}
	if policies != nil {
		grp.PolicyOverrides = policies
	}
	return nil
}

func (m *MemoryStore) DeleteGroup(ctx context.Context, orgID, groupID uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.groups, groupID)
	delete(m.groupMembers, groupID)
	return nil
}

func (m *MemoryStore) AssignDeviceToGroup(ctx context.Context, orgID, groupID, deviceID uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.groups[groupID]; !exists {
		return ErrNotFound
	}
	if _, exists := m.devices[deviceID]; !exists {
		return ErrNotFound
	}
	if _, ok := m.groupMembers[groupID]; !ok {
		m.groupMembers[groupID] = make(map[uuid.UUID]bool)
	}
	m.groupMembers[groupID][deviceID] = true
	return nil
}

func (m *MemoryStore) RemoveDeviceFromGroup(ctx context.Context, orgID, groupID, deviceID uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if members, ok := m.groupMembers[groupID]; ok {
		delete(members, deviceID)
	}
	return nil
}

func (m *MemoryStore) GetGroupDevices(ctx context.Context, orgID, groupID uuid.UUID) ([]models.Device, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	members := m.groupMembers[groupID]
	result := make([]models.Device, 0)
	for dID := range members {
		if dev, exists := m.devices[dID]; exists && dev.OrganizationID == orgID {
			result = append(result, *dev)
		}
	}
	return result, nil
}

func (m *MemoryStore) GetGroupHealthSummary(ctx context.Context, orgID, groupID uuid.UUID) (*GroupHealthSummary, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	grp := m.groups[groupID]
	summary := &GroupHealthSummary{GroupID: groupID}
	if grp != nil {
		summary.GroupName = grp.Name
	}
	members := m.groupMembers[groupID]
	for dID := range members {
		if dev, exists := m.devices[dID]; exists && dev.OrganizationID == orgID {
			summary.TotalDevices++
			if dev.Status == models.DeviceStatusActive {
				summary.OnlineDevices++
			} else {
				summary.OfflineDevices++
			}
		}
	}
	return summary, nil
}

// Enrollment
func (m *MemoryStore) CreateEnrollmentInvitation(ctx context.Context, inv models.EnrollmentInvitation) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.invitations[inv.ID] = &inv
	return nil
}

func (m *MemoryStore) GetEnrollmentInvitationByHash(ctx context.Context, tokenHash string) (*models.EnrollmentInvitation, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, inv := range m.invitations {
		if inv.TokenHash == tokenHash {
			if time.Now().UTC().After(inv.ExpiresAt) {
				return nil, ErrTokenExpired
			}
			if inv.TimesUsed >= inv.MaxUses {
				return nil, ErrTokenExhausted
			}
			return inv, nil
		}
	}
	return nil, ErrNotFound
}

func (m *MemoryStore) ConsumeEnrollmentInvitation(ctx context.Context, invID uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if inv, exists := m.invitations[invID]; exists {
		inv.TimesUsed++
	}
	return nil
}

func (m *MemoryStore) ListEnrollmentInvitations(ctx context.Context, orgID uuid.UUID) ([]models.EnrollmentInvitation, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]models.EnrollmentInvitation, 0)
	for _, inv := range m.invitations {
		if inv.OrganizationID == orgID {
			result = append(result, *inv)
		}
	}
	return result, nil
}

func (m *MemoryStore) DeleteEnrollmentInvitation(ctx context.Context, orgID, invID uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.invitations, invID)
	return nil
}

// Telemetry
func (m *MemoryStore) SaveTelemetry(ctx context.Context, t models.DeviceTelemetry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.telemetry[t.DeviceID] = append(m.telemetry[t.DeviceID], t)
	if dev, exists := m.devices[t.DeviceID]; exists {
		now := time.Now().UTC()
		dev.LastSeen = &now
	}
	return nil
}

func (m *MemoryStore) GetLatestTelemetry(ctx context.Context, orgID, deviceID uuid.UUID) (*models.DeviceTelemetry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	history := m.telemetry[deviceID]
	if len(history) == 0 {
		return nil, ErrNotFound
	}
	return &history[len(history)-1], nil
}

func (m *MemoryStore) GetTelemetryHistory(ctx context.Context, orgID, deviceID uuid.UUID, limit int) ([]models.DeviceTelemetry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.telemetry[deviceID], nil
}

// Remote Sessions
func (m *MemoryStore) CreateRemoteSession(ctx context.Context, session models.RemoteSession) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[session.ID] = &session
	return nil
}

func (m *MemoryStore) GetRemoteSession(ctx context.Context, orgID, sessionID uuid.UUID) (*models.RemoteSession, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, exists := m.sessions[sessionID]
	if !exists || s.OrganizationID != orgID {
		return nil, ErrNotFound
	}
	return s, nil
}

func (m *MemoryStore) UpdateRemoteSessionStatus(ctx context.Context, orgID, sessionID uuid.UUID, status string, reason *string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, exists := m.sessions[sessionID]
	if !exists {
		return ErrNotFound
	}
	s.SessionStatus = status
	now := time.Now().UTC()
	if status == "CONNECTED" && s.StartedAt == nil {
		s.StartedAt = &now
	}
	if status == "TERMINATED" || status == "REJECTED" {
		s.EndedAt = &now
		s.TerminationReason = reason
	}
	return nil
}

func (m *MemoryStore) ListRemoteSessions(ctx context.Context, orgID uuid.UUID) ([]models.RemoteSession, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]models.RemoteSession, 0)
	for _, s := range m.sessions {
		if s.OrganizationID == orgID {
			result = append(result, *s)
		}
	}
	return result, nil
}

// Commands / Jobs
func (m *MemoryStore) CreateCommand(ctx context.Context, cmd models.Command) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.commands[cmd.ID] = &cmd
	return nil
}

func (m *MemoryStore) GetCommand(ctx context.Context, orgID, commandID uuid.UUID) (*models.Command, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	cmd, exists := m.commands[commandID]
	if !exists || cmd.OrganizationID != orgID {
		return nil, ErrNotFound
	}
	return cmd, nil
}

func (m *MemoryStore) UpdateCommandStatus(ctx context.Context, orgID, commandID uuid.UUID, status string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cmd, exists := m.commands[commandID]
	if !exists || cmd.OrganizationID != orgID {
		return ErrNotFound
	}
	cmd.Status = status
	return nil
}

func (m *MemoryStore) ListDeviceCommands(ctx context.Context, orgID, deviceID uuid.UUID) ([]models.Command, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]models.Command, 0)
	for _, cmd := range m.commands {
		if cmd.OrganizationID == orgID && cmd.DeviceID == deviceID {
			result = append(result, *cmd)
		}
	}
	return result, nil
}

func (m *MemoryStore) SaveCommandResult(ctx context.Context, res models.CommandResult) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.commandResults[res.CommandID] = &res
	if cmd, exists := m.commands[res.CommandID]; exists {
		if res.ExitCode == 0 {
			cmd.Status = "SUCCESS"
		} else {
			cmd.Status = "FAILED"
		}
	}
	return nil
}

func (m *MemoryStore) GetCommandResult(ctx context.Context, commandID uuid.UUID) (*models.CommandResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	res, exists := m.commandResults[commandID]
	if !exists {
		return nil, ErrNotFound
	}
	return res, nil
}

// Alerts
func (m *MemoryStore) CreateAlert(ctx context.Context, alert models.Alert) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.alerts[alert.ID] = &alert
	return nil
}

func (m *MemoryStore) ListAlerts(ctx context.Context, orgID uuid.UUID) ([]models.Alert, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]models.Alert, 0)
	for _, a := range m.alerts {
		if a.OrganizationID == orgID {
			result = append(result, *a)
		}
	}
	return result, nil
}

func (m *MemoryStore) AcknowledgeAlert(ctx context.Context, orgID, alertID, userID uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	a, exists := m.alerts[alertID]
	if !exists || a.OrganizationID != orgID {
		return ErrNotFound
	}
	now := time.Now().UTC()
	a.Acknowledged = true
	a.AcknowledgedBy = &userID
	a.AcknowledgedAt = &now
	return nil
}

// Software
func (m *MemoryStore) CreateSoftwarePackage(ctx context.Context, pkg models.SoftwarePackage) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.softwarePkgs[pkg.ID] = &pkg
	return nil
}

func (m *MemoryStore) ListSoftwarePackages(ctx context.Context, orgID uuid.UUID) ([]models.SoftwarePackage, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]models.SoftwarePackage, 0)
	for _, p := range m.softwarePkgs {
		if p.OrganizationID == orgID {
			result = append(result, *p)
		}
	}
	return result, nil
}

func (m *MemoryStore) CreateDeployment(ctx context.Context, dep models.SoftwareDeployment) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.deployments[dep.ID] = &dep
	return nil
}

func (m *MemoryStore) ListDeployments(ctx context.Context, orgID uuid.UUID) ([]models.SoftwareDeployment, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]models.SoftwareDeployment, 0)
	for _, d := range m.deployments {
		if d.OrganizationID == orgID {
			result = append(result, *d)
		}
	}
	return result, nil
}

// Licensing
func (m *MemoryStore) GetLicense(ctx context.Context, orgID uuid.UUID) (*models.License, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	lic, exists := m.licenses[orgID]
	if !exists {
		return nil, ErrNotFound
	}
	return lic, nil
}

func (m *MemoryStore) SaveLicense(ctx context.Context, lic models.License) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.licenses[lic.OrganizationID] = &lic
	return nil
}

// Audit Logs
func (m *MemoryStore) InsertAuditLog(ctx context.Context, log models.AuditLog) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	log.ID = int64(len(m.auditLogs) + 1)
	log.CreatedAt = time.Now().UTC()
	m.auditLogs = append(m.auditLogs, log)
	return nil
}

func (m *MemoryStore) ListAuditLogs(ctx context.Context, orgID uuid.UUID, limit int) ([]models.AuditLog, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]models.AuditLog, 0)
	for i := len(m.auditLogs) - 1; i >= 0 && len(result) < limit; i-- {
		if m.auditLogs[i].OrganizationID == orgID {
			result = append(result, m.auditLogs[i])
		}
	}
	return result, nil
}
