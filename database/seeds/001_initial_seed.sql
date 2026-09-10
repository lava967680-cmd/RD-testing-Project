-- ControlHub Initial Seed Data
-- Seed: 001_initial_seed.sql

-- 1. Standard Permissions
INSERT INTO permissions (id, category, description) VALUES
('organization.view', 'organization', 'View organization details & settings'),
('organization.manage', 'organization', 'Update organization policies & settings'),
('user.view', 'user', 'View users and roles in organization'),
('user.manage', 'user', 'Create, invite, update, or deactivate users'),
('device.view', 'device', 'View device inventory and health stats'),
('device.enroll', 'device', 'Generate device enrollment invitations'),
('device.approve', 'device', 'Approve pending devices into inventory'),
('device.remove', 'device', 'Revoke and delete enrolled devices'),
('device.remote', 'device', 'Initiate WebRTC remote desktop sessions'),
('device.file_transfer', 'device', 'Upload or download files on device'),
('device.terminal', 'device', 'Run interactive terminal or queued commands'),
('device.restart', 'device', 'Trigger reboot or shutdown on endpoint'),
('device.software_manage', 'device', 'Deploy software packages or updates'),
('group.view', 'group', 'View device groups and hierarchies'),
('group.manage', 'group', 'Create, edit, or delete device groups'),
('group.assign_device', 'group', 'Move devices between groups'),
('alert.view', 'alert', 'View device health alerts'),
('alert.manage', 'alert', 'Acknowledge or dismiss alerts'),
('audit.view', 'audit', 'View immutable audit trail records'),
('license.view', 'license', 'View license tier, limits and entitlements'),
('license.manage', 'license', 'Purchase or update organization license')
ON CONFLICT (id) DO NOTHING;

-- 2. Demo Organization
INSERT INTO organizations (id, name, slug, status, settings)
VALUES (
    '771e8bfb-cf98-4c28-98e3-0d6e6443c21a',
    'Acme IT Academy',
    'acme-academy',
    'ACTIVE',
    '{"require_remote_consent": true, "telemetry_interval_sec": 15}'
) ON CONFLICT (id) DO NOTHING;

-- 3. Default Roles for Organization
INSERT INTO roles (id, organization_id, name, is_system_role) VALUES
('11111111-1111-1111-1111-111111111111', '771e8bfb-cf98-4c28-98e3-0d6e6443c21a', 'Owner', TRUE),
('22222222-2222-2222-2222-222222222222', '771e8bfb-cf98-4c28-98e3-0d6e6443c21a', 'Administrator', TRUE),
('33333333-3333-3333-3333-333333333333', '771e8bfb-cf98-4c28-98e3-0d6e6443c21a', 'IT Support', TRUE),
('44444444-4444-4444-4444-444444444444', '771e8bfb-cf98-4c28-98e3-0d6e6443c21a', 'Operator', TRUE),
('55555555-5555-5555-5555-555555555555', '771e8bfb-cf98-4c28-98e3-0d6e6443c21a', 'Viewer', TRUE)
ON CONFLICT (id) DO NOTHING;

-- Grant all permissions to Owner
INSERT INTO role_permissions (role_id, permission_id)
SELECT '11111111-1111-1111-1111-111111111111', id FROM permissions
ON CONFLICT DO NOTHING;

-- Grant Admin permissions
INSERT INTO role_permissions (role_id, permission_id)
SELECT '22222222-2222-2222-2222-222222222222', id FROM permissions
WHERE id NOT IN ('organization.manage', 'license.manage')
ON CONFLICT DO NOTHING;

-- 4. Initial Administrator User (Password: AdminSecure123!)
-- Argon2id hash for 'AdminSecure123!'
INSERT INTO users (id, organization_id, email, password_hash, full_name, mfa_enabled, status)
VALUES (
    'c1f72776-9ec6-4f40-8b17-7435f3dfd720',
    '771e8bfb-cf98-4c28-98e3-0d6e6443c21a',
    'admin@acme-academy.org',
    '$argon2id$v=19$m=65536,t=3,p=4$dGVzdHNhbHQxMjM0NTY3OA$mJ0q/eH8gK4K5o4b0N7tTq1Jq0d2S8v7Y3m2k1p0e4E',
    'Lab Administrator',
    FALSE,
    'ACTIVE'
) ON CONFLICT (organization_id, email) DO NOTHING;

INSERT INTO user_roles (user_id, role_id)
VALUES (
    'c1f72776-9ec6-4f40-8b17-7435f3dfd720',
    '22222222-2222-2222-2222-222222222222'
) ON CONFLICT DO NOTHING;

-- 5. Seed Initial Groups
INSERT INTO device_groups (id, organization_id, name, description)
VALUES 
('8f8b1b9e-6441-4796-9f44-93e18a0029b1', '771e8bfb-cf98-4c28-98e3-0d6e6443c21a', 'Exam Lab 1', 'Primary certification exam workstations'),
('8f8b1b9e-6441-4796-9f44-93e18a0029b2', '771e8bfb-cf98-4c28-98e3-0d6e6443c21a', 'Training Room', 'General software development training lab')
ON CONFLICT (organization_id, name) DO NOTHING;
