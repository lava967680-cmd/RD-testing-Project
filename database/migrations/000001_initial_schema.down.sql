-- ControlHub Initial Schema Migration Rollback (PostgreSQL 16)
-- Migration: 000001_initial_schema.down.sql

DROP TRIGGER IF EXISTS trg_audit_logs_immutable ON audit_logs;
DROP FUNCTION IF EXISTS prevent_audit_log_tampering();

DROP TABLE IF EXISTS update_jobs CASCADE;
DROP TABLE IF EXISTS agent_versions CASCADE;
DROP TABLE IF EXISTS licenses CASCADE;
DROP TABLE IF EXISTS audit_logs CASCADE;
DROP TABLE IF EXISTS alerts CASCADE;
DROP TABLE IF EXISTS software_deployments CASCADE;
DROP TABLE IF EXISTS software_packages CASCADE;
DROP TABLE IF EXISTS command_results CASCADE;
DROP TABLE IF EXISTS commands CASCADE;
DROP TABLE IF EXISTS session_events CASCADE;
DROP TABLE IF EXISTS remote_sessions CASCADE;
DROP TABLE IF EXISTS device_telemetry CASCADE;
DROP TABLE IF EXISTS enrollment_invitations CASCADE;
DROP TABLE IF EXISTS group_memberships CASCADE;
DROP TABLE IF EXISTS device_credentials CASCADE;
DROP TABLE IF EXISTS devices CASCADE;
DROP TABLE IF EXISTS device_groups CASCADE;
DROP TABLE IF EXISTS user_roles CASCADE;
DROP TABLE IF EXISTS role_permissions CASCADE;
DROP TABLE IF EXISTS roles CASCADE;
DROP TABLE IF EXISTS permissions CASCADE;
DROP TABLE IF EXISTS users CASCADE;
DROP TABLE IF EXISTS organizations CASCADE;
