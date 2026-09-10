package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/controlhub/controlhub/services/api/internal/models"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresStore is a real, persistent implementation of the Store interface,
// backed by PostgreSQL. Unlike MemoryStore, data survives restarts and is
// shared correctly across multiple API instances.
type PostgresStore struct {
	pool *pgxpool.Pool
}

// NewPostgresStore opens a connection pool against databaseURL. Call this
// once at startup; the returned pool is safe for concurrent use.
func NewPostgresStore(ctx context.Context, databaseURL string) (*PostgresStore, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return &PostgresStore{pool: pool}, nil
}

func (s *PostgresStore) Close() {
	s.pool.Close()
}

// ---- Organizations ----

func (s *PostgresStore) GetOrganization(ctx context.Context, orgID uuid.UUID) (*models.Organization, error) {
	var o models.Organization
	var settingsRaw []byte
	err := s.pool.QueryRow(ctx,
		`SELECT id, name, slug, status, settings, created_at, updated_at FROM organizations WHERE id=$1`,
		orgID,
	).Scan(&o.ID, &o.Name, &o.Slug, &o.Status, &settingsRaw, &o.CreatedAt, &o.UpdatedAt)
	if err != nil {
		return nil, ErrNotFound
	}
	_ = json.Unmarshal(settingsRaw, &o.Settings)
	return &o, nil
}

// ---- Users & Auth ----

func (s *PostgresStore) GetUserByEmail(ctx context.Context, email string) (*models.User, error) {
	var u models.User
	err := s.pool.QueryRow(ctx,
		`SELECT id, organization_id, email, password_hash, full_name, mfa_enabled, mfa_secret, status, last_login_at, created_at, updated_at
		 FROM users WHERE email=$1`, email,
	).Scan(&u.ID, &u.OrganizationID, &u.Email, &u.PasswordHash, &u.FullName, &u.MFAEnabled, &u.MFASecret, &u.Status, &u.LastLoginAt, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, ErrNotFound
	}
	return &u, nil
}

func (s *PostgresStore) GetUserByID(ctx context.Context, userID uuid.UUID) (*models.User, error) {
	var u models.User
	err := s.pool.QueryRow(ctx,
		`SELECT id, organization_id, email, password_hash, full_name, mfa_enabled, mfa_secret, status, last_login_at, created_at, updated_at
		 FROM users WHERE id=$1`, userID,
	).Scan(&u.ID, &u.OrganizationID, &u.Email, &u.PasswordHash, &u.FullName, &u.MFAEnabled, &u.MFASecret, &u.Status, &u.LastLoginAt, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, ErrNotFound
	}
	return &u, nil
}

func (s *PostgresStore) UpdateUserMFA(ctx context.Context, userID uuid.UUID, enabled bool, secret *string) error {
	ct, err := s.pool.Exec(ctx,
		`UPDATE users SET mfa_enabled=$1, mfa_secret=$2, updated_at=NOW() WHERE id=$3`,
		enabled, secret, userID)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// GetUserPermissions returns the union of permissions across all roles
// assigned to the user, plus the user's primary role name for display.
func (s *PostgresStore) GetUserPermissions(ctx context.Context, userID uuid.UUID) ([]string, string, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT DISTINCT rp.permission_id, r.name
		FROM user_roles ur
		JOIN roles r ON r.id = ur.role_id
		JOIN role_permissions rp ON rp.role_id = r.id
		WHERE ur.user_id = $1`, userID)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()

	var perms []string
	var roleName string
	for rows.Next() {
		var perm, role string
		if err := rows.Scan(&perm, &role); err != nil {
			return nil, "", err
		}
		perms = append(perms, perm)
		roleName = role
	}
	return perms, roleName, nil
}

// ---- Refresh Tokens ----
// Stored as a SHA-256 hash of the token in a dedicated table would be ideal;
// for now this mirrors the existing RefreshToken struct 1:1.

func (s *PostgresStore) SaveRefreshToken(ctx context.Context, token RefreshToken) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO refresh_tokens (token, user_id, organization_id, expires_at, revoked)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (token) DO UPDATE SET revoked = EXCLUDED.revoked`,
		token.Token, token.UserID, token.OrganizationID, token.ExpiresAt, token.Revoked)
	return err
}

func (s *PostgresStore) GetRefreshToken(ctx context.Context, token string) (*RefreshToken, error) {
	var t RefreshToken
	err := s.pool.QueryRow(ctx,
		`SELECT token, user_id, organization_id, expires_at, revoked FROM refresh_tokens WHERE token=$1`, token,
	).Scan(&t.Token, &t.UserID, &t.OrganizationID, &t.ExpiresAt, &t.Revoked)
	if err != nil {
		return nil, ErrNotFound
	}
	if t.Revoked {
		return nil, ErrUnauthorized
	}
	if time.Now().After(t.ExpiresAt) {
		return nil, ErrTokenExpired
	}
	return &t, nil
}

func (s *PostgresStore) RevokeRefreshToken(ctx context.Context, token string) error {
	_, err := s.pool.Exec(ctx, `UPDATE refresh_tokens SET revoked=true WHERE token=$1`, token)
	return err
}

// ---- Devices ----

func (s *PostgresStore) ListDevices(ctx context.Context, orgID uuid.UUID) ([]models.Device, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, organization_id, hostname, friendly_name, os_name, os_version, os_architecture, agent_version,
		       status, ip_address_public, ip_address_local, mac_address, serial_number, last_seen, uptime_seconds,
		       created_at, updated_at
		FROM devices WHERE organization_id=$1 ORDER BY created_at DESC`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.Device
	for rows.Next() {
		var d models.Device
		if err := rows.Scan(&d.ID, &d.OrganizationID, &d.Hostname, &d.FriendlyName, &d.OSName, &d.OSVersion,
			&d.OSArchitecture, &d.AgentVersion, &d.Status, &d.IPAddressPublic, &d.IPAddressLocal, &d.MACAddress,
			&d.SerialNumber, &d.LastSeen, &d.UptimeSeconds, &d.CreatedAt, &d.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, nil
}

func (s *PostgresStore) GetDevice(ctx context.Context, orgID, deviceID uuid.UUID) (*models.Device, error) {
	var d models.Device
	err := s.pool.QueryRow(ctx, `
		SELECT id, organization_id, hostname, friendly_name, os_name, os_version, os_architecture, agent_version,
		       status, ip_address_public, ip_address_local, mac_address, serial_number, last_seen, uptime_seconds,
		       created_at, updated_at
		FROM devices WHERE organization_id=$1 AND id=$2`, orgID, deviceID,
	).Scan(&d.ID, &d.OrganizationID, &d.Hostname, &d.FriendlyName, &d.OSName, &d.OSVersion, &d.OSArchitecture,
		&d.AgentVersion, &d.Status, &d.IPAddressPublic, &d.IPAddressLocal, &d.MACAddress, &d.SerialNumber,
		&d.LastSeen, &d.UptimeSeconds, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		return nil, ErrUnauthorized
	}
	return &d, nil
}

func (s *PostgresStore) UpdateDeviceStatus(ctx context.Context, orgID, deviceID uuid.UUID, status models.DeviceStatus) error {
	ct, err := s.pool.Exec(ctx,
		`UPDATE devices SET status=$1, updated_at=NOW() WHERE organization_id=$2 AND id=$3`,
		status, orgID, deviceID)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrUnauthorized
	}
	return nil
}

func (s *PostgresStore) RegisterDevice(ctx context.Context, dev models.Device, pubKey []byte) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, `
		INSERT INTO devices (id, organization_id, hostname, friendly_name, os_name, os_version, os_architecture,
		                      agent_version, status, ip_address_public, ip_address_local, mac_address, serial_number)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
		dev.ID, dev.OrganizationID, dev.Hostname, dev.FriendlyName, dev.OSName, dev.OSVersion, dev.OSArchitecture,
		dev.AgentVersion, dev.Status, dev.IPAddressPublic, dev.IPAddressLocal, dev.MACAddress, dev.SerialNumber)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx,
		`INSERT INTO device_credentials (device_id, public_key_bytes) VALUES ($1, $2)`,
		dev.ID, pubKey)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// ---- Device Groups ----

func (s *PostgresStore) CreateGroup(ctx context.Context, grp models.DeviceGroup) error {
	policyRaw, _ := json.Marshal(grp.PolicyOverrides)
	_, err := s.pool.Exec(ctx,
		`INSERT INTO device_groups (id, organization_id, parent_group_id, name, description, policy_overrides)
		 VALUES ($1,$2,$3,$4,$5,$6)`,
		grp.ID, grp.OrganizationID, grp.ParentGroupID, grp.Name, grp.Description, policyRaw)
	return err
}

func (s *PostgresStore) GetGroup(ctx context.Context, orgID, groupID uuid.UUID) (*models.DeviceGroup, error) {
	var g models.DeviceGroup
	var policyRaw []byte
	err := s.pool.QueryRow(ctx,
		`SELECT id, organization_id, parent_group_id, name, description, policy_overrides, created_at
		 FROM device_groups WHERE organization_id=$1 AND id=$2`, orgID, groupID,
	).Scan(&g.ID, &g.OrganizationID, &g.ParentGroupID, &g.Name, &g.Description, &policyRaw, &g.CreatedAt)
	if err != nil {
		return nil, ErrUnauthorized
	}
	_ = json.Unmarshal(policyRaw, &g.PolicyOverrides)
	return &g, nil
}

func (s *PostgresStore) ListGroups(ctx context.Context, orgID uuid.UUID) ([]models.DeviceGroup, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, organization_id, parent_group_id, name, description, policy_overrides, created_at
		 FROM device_groups WHERE organization_id=$1 ORDER BY name`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.DeviceGroup
	for rows.Next() {
		var g models.DeviceGroup
		var policyRaw []byte
		if err := rows.Scan(&g.ID, &g.OrganizationID, &g.ParentGroupID, &g.Name, &g.Description, &policyRaw, &g.CreatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(policyRaw, &g.PolicyOverrides)
		out = append(out, g)
	}
	return out, nil
}

func (s *PostgresStore) UpdateGroup(ctx context.Context, orgID, groupID uuid.UUID, name string, desc *string, policies map[string]interface{}) error {
	policyRaw, _ := json.Marshal(policies)
	ct, err := s.pool.Exec(ctx,
		`UPDATE device_groups SET name=$1, description=$2, policy_overrides=$3 WHERE organization_id=$4 AND id=$5`,
		name, desc, policyRaw, orgID, groupID)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrUnauthorized
	}
	return nil
}

func (s *PostgresStore) DeleteGroup(ctx context.Context, orgID, groupID uuid.UUID) error {
	ct, err := s.pool.Exec(ctx, `DELETE FROM device_groups WHERE organization_id=$1 AND id=$2`, orgID, groupID)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrUnauthorized
	}
	return nil
}

func (s *PostgresStore) AssignDeviceToGroup(ctx context.Context, orgID, groupID, deviceID uuid.UUID) error {
	// Verify both belong to this org before linking, to enforce tenant isolation.
	if _, err := s.GetGroup(ctx, orgID, groupID); err != nil {
		return err
	}
	if _, err := s.GetDevice(ctx, orgID, deviceID); err != nil {
		return err
	}
	_, err := s.pool.Exec(ctx,
		`INSERT INTO group_memberships (group_id, device_id) VALUES ($1,$2)
		 ON CONFLICT (group_id, device_id) DO NOTHING`, groupID, deviceID)
	return err
}

func (s *PostgresStore) RemoveDeviceFromGroup(ctx context.Context, orgID, groupID, deviceID uuid.UUID) error {
	if _, err := s.GetGroup(ctx, orgID, groupID); err != nil {
		return err
	}
	_, err := s.pool.Exec(ctx,
		`DELETE FROM group_memberships WHERE group_id=$1 AND device_id=$2`, groupID, deviceID)
	return err
}

func (s *PostgresStore) GetGroupDevices(ctx context.Context, orgID, groupID uuid.UUID) ([]models.Device, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT d.id, d.organization_id, d.hostname, d.friendly_name, d.os_name, d.os_version, d.os_architecture,
		       d.agent_version, d.status, d.ip_address_public, d.ip_address_local, d.mac_address, d.serial_number,
		       d.last_seen, d.uptime_seconds, d.created_at, d.updated_at
		FROM devices d
		JOIN group_memberships gm ON gm.device_id = d.id
		WHERE gm.group_id = $1 AND d.organization_id = $2`, groupID, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.Device
	for rows.Next() {
		var d models.Device
		if err := rows.Scan(&d.ID, &d.OrganizationID, &d.Hostname, &d.FriendlyName, &d.OSName, &d.OSVersion,
			&d.OSArchitecture, &d.AgentVersion, &d.Status, &d.IPAddressPublic, &d.IPAddressLocal, &d.MACAddress,
			&d.SerialNumber, &d.LastSeen, &d.UptimeSeconds, &d.CreatedAt, &d.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, nil
}

func (s *PostgresStore) GetGroupHealthSummary(ctx context.Context, orgID, groupID uuid.UUID) (*GroupHealthSummary, error) {
	grp, err := s.GetGroup(ctx, orgID, groupID)
	if err != nil {
		return nil, err
	}
	summary := &GroupHealthSummary{GroupID: groupID, GroupName: grp.Name}

	err = s.pool.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE true) AS total,
			COUNT(*) FILTER (WHERE d.status = 'ACTIVE') AS online,
			COUNT(*) FILTER (WHERE d.status != 'ACTIVE') AS offline
		FROM devices d
		JOIN group_memberships gm ON gm.device_id = d.id
		WHERE gm.group_id = $1`, groupID,
	).Scan(&summary.TotalDevices, &summary.OnlineDevices, &summary.OfflineDevices)
	if err != nil {
		return nil, err
	}

	// Average of the most recent telemetry row per device in the group.
	_ = s.pool.QueryRow(ctx, `
		SELECT COALESCE(AVG(t.cpu_percent), 0), COALESCE(AVG(t.ram_percent), 0)
		FROM device_telemetry t
		JOIN group_memberships gm ON gm.device_id = t.device_id
		WHERE gm.group_id = $1
		  AND t.recorded_at = (SELECT MAX(t2.recorded_at) FROM device_telemetry t2 WHERE t2.device_id = t.device_id)`,
		groupID,
	).Scan(&summary.AvgCPUPercent, &summary.AvgRAMPercent)

	return summary, nil
}

// ---- Enrollment Invitations ----

func (s *PostgresStore) CreateEnrollmentInvitation(ctx context.Context, inv models.EnrollmentInvitation) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO enrollment_invitations
			(id, organization_id, token_hash, target_group_id, requires_approval, max_uses, times_used, expires_at, created_by_user_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		inv.ID, inv.OrganizationID, inv.TokenHash, inv.TargetGroupID, inv.RequiresApproval, inv.MaxUses,
		inv.TimesUsed, inv.ExpiresAt, inv.CreatedByUserID)
	return err
}

func (s *PostgresStore) GetEnrollmentInvitationByHash(ctx context.Context, tokenHash string) (*models.EnrollmentInvitation, error) {
	var inv models.EnrollmentInvitation
	err := s.pool.QueryRow(ctx, `
		SELECT id, organization_id, token_hash, target_group_id, requires_approval, max_uses, times_used, expires_at, created_by_user_id, created_at
		FROM enrollment_invitations WHERE token_hash=$1`, tokenHash,
	).Scan(&inv.ID, &inv.OrganizationID, &inv.TokenHash, &inv.TargetGroupID, &inv.RequiresApproval, &inv.MaxUses,
		&inv.TimesUsed, &inv.ExpiresAt, &inv.CreatedByUserID, &inv.CreatedAt)
	if err != nil {
		return nil, ErrNotFound
	}
	if time.Now().After(inv.ExpiresAt) {
		return nil, ErrTokenExpired
	}
	if inv.TimesUsed >= inv.MaxUses {
		return nil, ErrTokenExhausted
	}
	return &inv, nil
}

func (s *PostgresStore) ConsumeEnrollmentInvitation(ctx context.Context, invID uuid.UUID) error {
	ct, err := s.pool.Exec(ctx,
		`UPDATE enrollment_invitations SET times_used = times_used + 1 WHERE id=$1 AND times_used < max_uses`, invID)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrTokenExhausted
	}
	return nil
}

func (s *PostgresStore) ListEnrollmentInvitations(ctx context.Context, orgID uuid.UUID) ([]models.EnrollmentInvitation, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, organization_id, token_hash, target_group_id, requires_approval, max_uses, times_used, expires_at, created_by_user_id, created_at
		FROM enrollment_invitations WHERE organization_id=$1 ORDER BY created_at DESC`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.EnrollmentInvitation
	for rows.Next() {
		var inv models.EnrollmentInvitation
		if err := rows.Scan(&inv.ID, &inv.OrganizationID, &inv.TokenHash, &inv.TargetGroupID, &inv.RequiresApproval,
			&inv.MaxUses, &inv.TimesUsed, &inv.ExpiresAt, &inv.CreatedByUserID, &inv.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, inv)
	}
	return out, nil
}

func (s *PostgresStore) DeleteEnrollmentInvitation(ctx context.Context, orgID, invID uuid.UUID) error {
	ct, err := s.pool.Exec(ctx,
		`DELETE FROM enrollment_invitations WHERE organization_id=$1 AND id=$2`, orgID, invID)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrUnauthorized
	}
	return nil
}

// ---- Telemetry ----

func (s *PostgresStore) SaveTelemetry(ctx context.Context, t models.DeviceTelemetry) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO device_telemetry
			(organization_id, device_id, cpu_percent, ram_used_bytes, ram_total_bytes, ram_percent,
			 disk_used_bytes, disk_total_bytes, disk_percent, network_rx_bytes_sec, network_tx_bytes_sec)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
		t.OrganizationID, t.DeviceID, t.CPUPercent, t.RAMUsedBytes, t.RAMTotalBytes, t.RAMPercent,
		t.DiskUsedBytes, t.DiskTotalBytes, t.DiskPercent, t.NetworkRxBytesSec, t.NetworkTxBytesSec)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `UPDATE devices SET last_seen=NOW() WHERE id=$1`, t.DeviceID)
	return err
}

func (s *PostgresStore) GetLatestTelemetry(ctx context.Context, orgID, deviceID uuid.UUID) (*models.DeviceTelemetry, error) {
	var t models.DeviceTelemetry
	err := s.pool.QueryRow(ctx, `
		SELECT id, organization_id, device_id, cpu_percent, ram_used_bytes, ram_total_bytes, ram_percent,
		       disk_used_bytes, disk_total_bytes, disk_percent, network_rx_bytes_sec, network_tx_bytes_sec, recorded_at
		FROM device_telemetry WHERE organization_id=$1 AND device_id=$2
		ORDER BY recorded_at DESC LIMIT 1`, orgID, deviceID,
	).Scan(&t.ID, &t.OrganizationID, &t.DeviceID, &t.CPUPercent, &t.RAMUsedBytes, &t.RAMTotalBytes, &t.RAMPercent,
		&t.DiskUsedBytes, &t.DiskTotalBytes, &t.DiskPercent, &t.NetworkRxBytesSec, &t.NetworkTxBytesSec, &t.RecordedAt)
	if err != nil {
		return nil, ErrNotFound
	}
	return &t, nil
}

func (s *PostgresStore) GetTelemetryHistory(ctx context.Context, orgID, deviceID uuid.UUID, limit int) ([]models.DeviceTelemetry, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, organization_id, device_id, cpu_percent, ram_used_bytes, ram_total_bytes, ram_percent,
		       disk_used_bytes, disk_total_bytes, disk_percent, network_rx_bytes_sec, network_tx_bytes_sec, recorded_at
		FROM device_telemetry WHERE organization_id=$1 AND device_id=$2
		ORDER BY recorded_at DESC LIMIT $3`, orgID, deviceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.DeviceTelemetry
	for rows.Next() {
		var t models.DeviceTelemetry
		if err := rows.Scan(&t.ID, &t.OrganizationID, &t.DeviceID, &t.CPUPercent, &t.RAMUsedBytes, &t.RAMTotalBytes,
			&t.RAMPercent, &t.DiskUsedBytes, &t.DiskTotalBytes, &t.DiskPercent, &t.NetworkRxBytesSec,
			&t.NetworkTxBytesSec, &t.RecordedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, nil
}

// ---- Remote Sessions ----

func (s *PostgresStore) CreateRemoteSession(ctx context.Context, session models.RemoteSession) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO remote_sessions (id, organization_id, device_id, operator_user_id, session_status, started_at, ended_at, termination_reason)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		session.ID, session.OrganizationID, session.DeviceID, session.OperatorUserID, session.SessionStatus,
		session.StartedAt, session.EndedAt, session.TerminationReason)
	return err
}

func (s *PostgresStore) GetRemoteSession(ctx context.Context, orgID, sessionID uuid.UUID) (*models.RemoteSession, error) {
	var sess models.RemoteSession
	err := s.pool.QueryRow(ctx, `
		SELECT id, organization_id, device_id, operator_user_id, session_status, started_at, ended_at, termination_reason, created_at
		FROM remote_sessions WHERE organization_id=$1 AND id=$2`, orgID, sessionID,
	).Scan(&sess.ID, &sess.OrganizationID, &sess.DeviceID, &sess.OperatorUserID, &sess.SessionStatus,
		&sess.StartedAt, &sess.EndedAt, &sess.TerminationReason, &sess.CreatedAt)
	if err != nil {
		return nil, ErrUnauthorized
	}
	return &sess, nil
}

func (s *PostgresStore) UpdateRemoteSessionStatus(ctx context.Context, orgID, sessionID uuid.UUID, status string, reason *string) error {
	ct, err := s.pool.Exec(ctx, `
		UPDATE remote_sessions
		SET session_status=$1, termination_reason=$2,
		    started_at = CASE WHEN $1='CONNECTED' AND started_at IS NULL THEN NOW() ELSE started_at END,
		    ended_at = CASE WHEN $1 IN ('REJECTED','TERMINATED') THEN NOW() ELSE ended_at END
		WHERE organization_id=$3 AND id=$4`,
		status, reason, orgID, sessionID)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrUnauthorized
	}
	return nil
}

func (s *PostgresStore) ListRemoteSessions(ctx context.Context, orgID uuid.UUID) ([]models.RemoteSession, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, organization_id, device_id, operator_user_id, session_status, started_at, ended_at, termination_reason, created_at
		FROM remote_sessions WHERE organization_id=$1 ORDER BY created_at DESC`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.RemoteSession
	for rows.Next() {
		var sess models.RemoteSession
		if err := rows.Scan(&sess.ID, &sess.OrganizationID, &sess.DeviceID, &sess.OperatorUserID, &sess.SessionStatus,
			&sess.StartedAt, &sess.EndedAt, &sess.TerminationReason, &sess.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, sess)
	}
	return out, nil
}

// ---- Controlled Commands / Jobs ----

func (s *PostgresStore) CreateCommand(ctx context.Context, cmd models.Command) error {
	paramsRaw, _ := json.Marshal(cmd.Parameters)
	_, err := s.pool.Exec(ctx, `
		INSERT INTO commands (id, organization_id, device_id, issued_by_user_id, command_type, parameters, status, expires_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		cmd.ID, cmd.OrganizationID, cmd.DeviceID, cmd.IssuedByUserID, cmd.CommandType, paramsRaw, cmd.Status, cmd.ExpiresAt)
	return err
}

func (s *PostgresStore) GetCommand(ctx context.Context, orgID, commandID uuid.UUID) (*models.Command, error) {
	var c models.Command
	var paramsRaw []byte
	err := s.pool.QueryRow(ctx, `
		SELECT id, organization_id, device_id, issued_by_user_id, command_type, parameters, status, expires_at, created_at
		FROM commands WHERE organization_id=$1 AND id=$2`, orgID, commandID,
	).Scan(&c.ID, &c.OrganizationID, &c.DeviceID, &c.IssuedByUserID, &c.CommandType, &paramsRaw, &c.Status, &c.ExpiresAt, &c.CreatedAt)
	if err != nil {
		return nil, ErrUnauthorized
	}
	_ = json.Unmarshal(paramsRaw, &c.Parameters)
	return &c, nil
}

func (s *PostgresStore) UpdateCommandStatus(ctx context.Context, orgID, commandID uuid.UUID, status string) error {
	ct, err := s.pool.Exec(ctx,
		`UPDATE commands SET status=$1 WHERE organization_id=$2 AND id=$3`, status, orgID, commandID)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrUnauthorized
	}
	return nil
}

func (s *PostgresStore) ListDeviceCommands(ctx context.Context, orgID, deviceID uuid.UUID) ([]models.Command, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, organization_id, device_id, issued_by_user_id, command_type, parameters, status, expires_at, created_at
		FROM commands WHERE organization_id=$1 AND device_id=$2 ORDER BY created_at DESC`, orgID, deviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.Command
	for rows.Next() {
		var c models.Command
		var paramsRaw []byte
		if err := rows.Scan(&c.ID, &c.OrganizationID, &c.DeviceID, &c.IssuedByUserID, &c.CommandType, &paramsRaw,
			&c.Status, &c.ExpiresAt, &c.CreatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(paramsRaw, &c.Parameters)
		out = append(out, c)
	}
	return out, nil
}

func (s *PostgresStore) SaveCommandResult(ctx context.Context, res models.CommandResult) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO command_results (command_id, exit_code, stdout_output, stderr_output, execution_duration_ms)
		VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (command_id) DO UPDATE SET
			exit_code=EXCLUDED.exit_code, stdout_output=EXCLUDED.stdout_output,
			stderr_output=EXCLUDED.stderr_output, execution_duration_ms=EXCLUDED.execution_duration_ms`,
		res.CommandID, res.ExitCode, res.StdoutOutput, res.StderrOutput, res.ExecutionDurationMs)
	return err
}

func (s *PostgresStore) GetCommandResult(ctx context.Context, commandID uuid.UUID) (*models.CommandResult, error) {
	var r models.CommandResult
	err := s.pool.QueryRow(ctx,
		`SELECT command_id, exit_code, stdout_output, stderr_output, execution_duration_ms, completed_at
		 FROM command_results WHERE command_id=$1`, commandID,
	).Scan(&r.CommandID, &r.ExitCode, &r.StdoutOutput, &r.StderrOutput, &r.ExecutionDurationMs, &r.CompletedAt)
	if err != nil {
		return nil, ErrNotFound
	}
	return &r, nil
}

// ---- Alerts ----

func (s *PostgresStore) CreateAlert(ctx context.Context, alert models.Alert) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO alerts (id, organization_id, device_id, severity, alert_type, title, details)
		VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		alert.ID, alert.OrganizationID, alert.DeviceID, alert.Severity, alert.AlertType, alert.Title, alert.Details)
	return err
}

func (s *PostgresStore) ListAlerts(ctx context.Context, orgID uuid.UUID) ([]models.Alert, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, organization_id, device_id, severity, alert_type, title, details, acknowledged, acknowledged_by, acknowledged_at, created_at
		FROM alerts WHERE organization_id=$1 ORDER BY created_at DESC`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.Alert
	for rows.Next() {
		var a models.Alert
		if err := rows.Scan(&a.ID, &a.OrganizationID, &a.DeviceID, &a.Severity, &a.AlertType, &a.Title, &a.Details,
			&a.Acknowledged, &a.AcknowledgedBy, &a.AcknowledgedAt, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, nil
}

func (s *PostgresStore) AcknowledgeAlert(ctx context.Context, orgID, alertID, userID uuid.UUID) error {
	ct, err := s.pool.Exec(ctx, `
		UPDATE alerts SET acknowledged=true, acknowledged_by=$1, acknowledged_at=NOW()
		WHERE organization_id=$2 AND id=$3`, userID, orgID, alertID)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrUnauthorized
	}
	return nil
}

// ---- Software Packages & Deployments ----

func (s *PostgresStore) CreateSoftwarePackage(ctx context.Context, pkg models.SoftwarePackage) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO software_packages (id, organization_id, name, version, installer_url, sha256_hash, silent_install_args)
		VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		pkg.ID, pkg.OrganizationID, pkg.Name, pkg.Version, pkg.InstallerURL, pkg.SHA256Hash, pkg.SilentInstallArgs)
	return err
}

func (s *PostgresStore) ListSoftwarePackages(ctx context.Context, orgID uuid.UUID) ([]models.SoftwarePackage, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, organization_id, name, version, installer_url, sha256_hash, silent_install_args, created_at
		FROM software_packages WHERE organization_id=$1 ORDER BY created_at DESC`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.SoftwarePackage
	for rows.Next() {
		var p models.SoftwarePackage
		if err := rows.Scan(&p.ID, &p.OrganizationID, &p.Name, &p.Version, &p.InstallerURL, &p.SHA256Hash,
			&p.SilentInstallArgs, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

func (s *PostgresStore) CreateDeployment(ctx context.Context, dep models.SoftwareDeployment) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO software_deployments (id, organization_id, package_id, device_id, status)
		VALUES ($1,$2,$3,$4,$5)`,
		dep.ID, dep.OrganizationID, dep.PackageID, dep.DeviceID, dep.Status)
	return err
}

func (s *PostgresStore) ListDeployments(ctx context.Context, orgID uuid.UUID) ([]models.SoftwareDeployment, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, organization_id, package_id, device_id, status, created_at
		FROM software_deployments WHERE organization_id=$1 ORDER BY created_at DESC`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.SoftwareDeployment
	for rows.Next() {
		var d models.SoftwareDeployment
		if err := rows.Scan(&d.ID, &d.OrganizationID, &d.PackageID, &d.DeviceID, &d.Status, &d.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, nil
}

// ---- Licensing ----

func (s *PostgresStore) GetLicense(ctx context.Context, orgID uuid.UUID) (*models.License, error) {
	var l models.License
	var featuresRaw []byte
	err := s.pool.QueryRow(ctx, `
		SELECT organization_id, plan_code, max_devices, features, valid_from, valid_until, grace_period_days
		FROM licenses WHERE organization_id=$1`, orgID,
	).Scan(&l.OrganizationID, &l.PlanCode, &l.MaxDevices, &featuresRaw, &l.ValidFrom, &l.ValidUntil, &l.GracePeriodDays)
	if err != nil {
		return nil, ErrNotFound
	}
	_ = json.Unmarshal(featuresRaw, &l.Features)

	now := time.Now()
	switch {
	case now.After(l.ValidUntil.AddDate(0, 0, l.GracePeriodDays)):
		l.Status = "EXPIRED"
	case now.After(l.ValidUntil):
		l.Status = "GRACE_PERIOD"
	default:
		l.Status = "ACTIVE"
	}
	return &l, nil
}

func (s *PostgresStore) SaveLicense(ctx context.Context, lic models.License) error {
	featuresRaw, _ := json.Marshal(lic.Features)
	_, err := s.pool.Exec(ctx, `
		INSERT INTO licenses (id, organization_id, plan_code, max_devices, features, valid_from, valid_until, grace_period_days, signature)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		ON CONFLICT (organization_id) DO UPDATE SET
			plan_code=EXCLUDED.plan_code, max_devices=EXCLUDED.max_devices, features=EXCLUDED.features,
			valid_from=EXCLUDED.valid_from, valid_until=EXCLUDED.valid_until, grace_period_days=EXCLUDED.grace_period_days,
			signature=EXCLUDED.signature`,
		lic.ID, lic.OrganizationID, lic.PlanCode, lic.MaxDevices, featuresRaw, lic.ValidFrom, lic.ValidUntil,
		lic.GracePeriodDays, []byte{}) // signature verification wiring left for you to complete
	return err
}

// ---- Audit Logs ----
// Note: audit_logs has a DB trigger forbidding UPDATE/DELETE (see migration),
// so this store only ever inserts and reads — that immutability is enforced
// at the database level, not just in application code.

func (s *PostgresStore) InsertAuditLog(ctx context.Context, log models.AuditLog) error {
	metaRaw, _ := json.Marshal(log.Metadata)
	_, err := s.pool.Exec(ctx, `
		INSERT INTO audit_logs (organization_id, actor_user_id, action, resource_type, resource_id, ip_address, user_agent, status, metadata)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		log.OrganizationID, log.ActorUserID, log.Action, log.ResourceType, log.ResourceID, log.IPAddress,
		log.UserAgent, log.Status, metaRaw)
	return err
}

func (s *PostgresStore) ListAuditLogs(ctx context.Context, orgID uuid.UUID, limit int) ([]models.AuditLog, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, organization_id, actor_user_id, action, resource_type, resource_id, ip_address, user_agent, status, metadata, created_at
		FROM audit_logs WHERE organization_id=$1 ORDER BY created_at DESC LIMIT $2`, orgID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.AuditLog
	for rows.Next() {
		var a models.AuditLog
		var metaRaw []byte
		if err := rows.Scan(&a.ID, &a.OrganizationID, &a.ActorUserID, &a.Action, &a.ResourceType, &a.ResourceID,
			&a.IPAddress, &a.UserAgent, &a.Status, &metaRaw, &a.CreatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(metaRaw, &a.Metadata)
		out = append(out, a)
	}
	return out, nil
}
