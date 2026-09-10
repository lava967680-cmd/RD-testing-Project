package models

import (
	"time"

	"github.com/google/uuid"
)

type Organization struct {
	ID        uuid.UUID              `json:"id"`
	Name      string                 `json:"name"`
	Slug      string                 `json:"slug"`
	Status    string                 `json:"status"` // ACTIVE, SUSPENDED, TRIAL
	Settings  map[string]interface{} `json:"settings"`
	CreatedAt time.Time              `json:"created_at"`
	UpdatedAt time.Time              `json:"updated_at"`
}

type User struct {
	ID             uuid.UUID  `json:"id"`
	OrganizationID uuid.UUID  `json:"organization_id"`
	Email          string     `json:"email"`
	PasswordHash   string     `json:"-"`
	FullName       string     `json:"full_name"`
	MFAEnabled     bool       `json:"mfa_enabled"`
	MFASecret      *string    `json:"-"`
	Status         string     `json:"status"` // ACTIVE, INVITED, DISABLED
	LastLoginAt    *time.Time `json:"last_login_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type DeviceStatus string

const (
	DeviceStatusPending  DeviceStatus = "PENDING"
	DeviceStatusActive   DeviceStatus = "ACTIVE"
	DeviceStatusOffline  DeviceStatus = "OFFLINE"
	DeviceStatusDisabled DeviceStatus = "DISABLED"
	DeviceStatusRevoked  DeviceStatus = "REVOKED"
	DeviceStatusRemoved  DeviceStatus = "REMOVED"
)

type Device struct {
	ID              uuid.UUID    `json:"id"`
	OrganizationID  uuid.UUID    `json:"organization_id"`
	Hostname        string       `json:"hostname"`
	FriendlyName    *string      `json:"friendly_name,omitempty"`
	OSName          string       `json:"os_name"`
	OSVersion       string       `json:"os_version"`
	OSArchitecture  string       `json:"os_architecture"`
	AgentVersion    string       `json:"agent_version"`
	Status          DeviceStatus `json:"status"`
	IPAddressPublic *string      `json:"ip_address_public,omitempty"`
	IPAddressLocal  *string      `json:"ip_address_local,omitempty"`
	MACAddress      *string      `json:"mac_address,omitempty"`
	SerialNumber    *string      `json:"serial_number,omitempty"`
	LastSeen        *time.Time   `json:"last_seen,omitempty"`
	UptimeSeconds   int64        `json:"uptime_seconds"`
	CreatedAt       time.Time    `json:"created_at"`
	UpdatedAt       time.Time    `json:"updated_at"`
}

type DeviceGroup struct {
	ID              uuid.UUID              `json:"id"`
	OrganizationID  uuid.UUID              `json:"organization_id"`
	ParentGroupID   *uuid.UUID             `json:"parent_group_id,omitempty"`
	Name            string                 `json:"name"`
	Description     *string                `json:"description,omitempty"`
	PolicyOverrides map[string]interface{} `json:"policy_overrides,omitempty"`
	CreatedAt       time.Time              `json:"created_at"`
}

type EnrollmentInvitation struct {
	ID               uuid.UUID  `json:"id"`
	OrganizationID   uuid.UUID  `json:"organization_id"`
	TokenHash        string     `json:"-"`
	TargetGroupID    *uuid.UUID `json:"target_group_id,omitempty"`
	RequiresApproval bool       `json:"requires_approval"`
	MaxUses          int        `json:"max_uses"`
	TimesUsed        int        `json:"times_used"`
	ExpiresAt        time.Time  `json:"expires_at"`
	CreatedByUserID  *uuid.UUID `json:"created_by_user_id,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
}

type DeviceTelemetry struct {
	ID                int64     `json:"id"`
	OrganizationID    uuid.UUID `json:"organization_id"`
	DeviceID          uuid.UUID `json:"device_id"`
	CPUPercent        float64   `json:"cpu_percent"`
	RAMUsedBytes      int64     `json:"ram_used_bytes"`
	RAMTotalBytes     int64     `json:"ram_total_bytes"`
	RAMPercent        float64   `json:"ram_percent"`
	DiskUsedBytes     int64     `json:"disk_used_bytes"`
	DiskTotalBytes    int64     `json:"disk_total_bytes"`
	DiskPercent       float64   `json:"disk_percent"`
	NetworkRxBytesSec int64     `json:"network_rx_bytes_sec"`
	NetworkTxBytesSec int64     `json:"network_tx_bytes_sec"`
	RecordedAt        time.Time `json:"recorded_at"`
}

type RemoteSession struct {
	ID                uuid.UUID  `json:"id"`
	OrganizationID    uuid.UUID  `json:"organization_id"`
	DeviceID          uuid.UUID  `json:"device_id"`
	OperatorUserID    uuid.UUID  `json:"operator_user_id"`
	SessionStatus     string     `json:"session_status"` // REQUESTED, CONNECTED, REJECTED, TERMINATED
	StartedAt         *time.Time `json:"started_at,omitempty"`
	EndedAt           *time.Time `json:"ended_at,omitempty"`
	TerminationReason *string    `json:"termination_reason,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
}

type Command struct {
	ID             uuid.UUID              `json:"id"`
	OrganizationID uuid.UUID              `json:"organization_id"`
	DeviceID       uuid.UUID              `json:"device_id"`
	IssuedByUserID uuid.UUID              `json:"issued_by_user_id"`
	CommandType    string                 `json:"command_type"` // REBOOT, SHUTDOWN, RUN_SCRIPT, FILE_TRANSFER
	Parameters     map[string]interface{} `json:"parameters"`
	Status         string                 `json:"status"` // QUEUED, DISPATCHED, RUNNING, SUCCESS, FAILED, CANCELLED
	Nonce          string                 `json:"nonce"`
	ExpiresAt      time.Time              `json:"expires_at"`
	CreatedAt      time.Time              `json:"created_at"`
}

type CommandResult struct {
	CommandID           uuid.UUID `json:"command_id"`
	ExitCode            int       `json:"exit_code"`
	StdoutOutput        string    `json:"stdout_output"`
	StderrOutput        string    `json:"stderr_output"`
	ExecutionDurationMs int       `json:"execution_duration_ms"`
	CompletedAt         time.Time `json:"completed_at"`
}

type Alert struct {
	ID             uuid.UUID  `json:"id"`
	OrganizationID uuid.UUID  `json:"organization_id"`
	DeviceID       *uuid.UUID `json:"device_id,omitempty"`
	Severity       string     `json:"severity"` // INFO, WARNING, CRITICAL
	AlertType      string     `json:"alert_type"` // CPU_HIGH, RAM_HIGH, DISK_FULL, DEVICE_OFFLINE
	Title          string     `json:"title"`
	Details        string     `json:"details"`
	Acknowledged   bool       `json:"acknowledged"`
	AcknowledgedBy *uuid.UUID `json:"acknowledged_by,omitempty"`
	AcknowledgedAt *time.Time `json:"acknowledged_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
}

type SoftwarePackage struct {
	ID               uuid.UUID `json:"id"`
	OrganizationID   uuid.UUID `json:"organization_id"`
	Name             string    `json:"name"`
	Version          string    `json:"version"`
	InstallerURL     string    `json:"installer_url"`
	SHA256Hash       string    `json:"sha256_hash"`
	SilentInstallArgs string   `json:"silent_install_args"`
	CreatedAt        time.Time `json:"created_at"`
}

type SoftwareDeployment struct {
	ID             uuid.UUID `json:"id"`
	OrganizationID uuid.UUID `json:"organization_id"`
	PackageID      uuid.UUID `json:"package_id"`
	DeviceID       uuid.UUID `json:"device_id"`
	Status         string    `json:"status"` // QUEUED, DOWNLOADING, INSTALLING, SUCCESS, FAILED
	CreatedAt      time.Time `json:"created_at"`
}

type License struct {
	ID             uuid.UUID `json:"id"`
	OrganizationID uuid.UUID `json:"organization_id"`
	PlanCode       string    `json:"plan_code"` // FREE_TRIAL, PLAN_5, PLAN_25, PLAN_50, PLAN_100, ENTERPRISE
	MaxDevices     int       `json:"max_devices"`
	Features       []string  `json:"features"`
	ValidFrom      time.Time `json:"valid_from"`
	ValidUntil     time.Time `json:"valid_until"`
	GracePeriodDays int      `json:"grace_period_days"`
	Status         string    `json:"status"` // ACTIVE, GRACE_PERIOD, EXPIRED
}

type AuditLog struct {
	ID             int64                  `json:"id"`
	OrganizationID uuid.UUID              `json:"organization_id"`
	ActorUserID    *uuid.UUID             `json:"actor_user_id,omitempty"`
	Action         string                 `json:"action"`
	ResourceType   string                 `json:"resource_type"`
	ResourceID     string                 `json:"resource_id"`
	IPAddress      *string                `json:"ip_address,omitempty"`
	UserAgent      *string                `json:"user_agent,omitempty"`
	Status         string                 `json:"status"`
	Metadata       map[string]interface{} `json:"metadata"`
	CreatedAt      time.Time              `json:"created_at"`
}
