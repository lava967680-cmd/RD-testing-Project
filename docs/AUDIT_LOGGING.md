# ControlHub: Compliance & Audit Logging Specification

## 1. Audit Principles & Immutability

Audit records form the definitive legal and operational record of all security-sensitive actions within ControlHub.
- **Append-Only**: The database allows only `INSERT` queries on `audit_logs`. Update and delete operations are rejected by database triggers.
- **Privacy-Preserving**: Audit logs record metadata (actor, timestamp, IP address, target device, file names, file sizes, exit codes) but **never** record sensitive passwords, secret keys, or confidential file contents.

---

## 2. Comprehensive Event Taxonomy

| Category | Action Identifier | Triggers & Context |
| :--- | :--- | :--- |
| **Authentication** | `auth.login.success` | User logged into console successfully. Includes IP and User-Agent. |
| | `auth.login.failure` | Failed password attempt. Includes attempted email and client IP. |
| | `auth.mfa.challenge` | MFA challenge presented. |
| | `auth.mfa.success` | MFA challenge solved. |
| | `auth.logout` | User logged out; session token invalidated. |
| **Device Lifecycle**| `device.invitation.created` | New enrollment token generated with usage limits and expiration. |
| | `device.enrolled` | Agent performed initial cryptographic handshake. |
| | `device.approved` | Admin promoted device from `PENDING` to `ACTIVE`. |
| | `device.revoked` | Device credentials revoked; connections terminated. |
| | `device.deleted` | Device unlinked and purged from organization inventory. |
| **Remote Support** | `session.requested` | Admin requested remote control session. |
| | `session.consent.granted` | Local workstation user granted permission. |
| | `session.connected` | WebRTC peer connection established. |
| | `session.terminated` | Remote session closed. Records total duration and initiator. |
| **File Transfer** | `file.transfer.upload` | File uploaded to device (Records: path, size_bytes, sha256). |
| | `file.transfer.download` | File downloaded from device (Records: path, size_bytes, sha256). |
| **Command Execution**| `command.queued` | Job submitted to queue with parameters. |
| | `command.completed` | Job completed on agent (Records: exit_code, duration_ms). |
| **Power Management**| `device.restart.initiated` | Reboot command sent to endpoint. |
| | `device.shutdown.initiated`| Shutdown command sent to endpoint. |
| **Administration** | `group.created` / `group.deleted`| Lab or classroom group modified. |
| | `user.role.updated` | User permissions or roles elevated or reduced. |
| | `license.updated` | Subscription tier or device quota changed. |

---

## 3. Audit Record Schema & Structure

```json
{
  "id": 10482,
  "organization_id": "771e8bfb-cf98-4c28-98e3-0d6e6443c21a",
  "actor": {
    "user_id": "c1f72776-9ec6-4f40-8b17-7435f3dfd720",
    "email": "admin@acme-academy.org",
    "full_name": "John Doe",
    "role": "Administrator"
  },
  "action": "session.connected",
  "resource": {
    "type": "device",
    "id": "90e722c1-bb3e-4623-a128-44fb6b907a01",
    "name": "LAB-PC-042"
  },
  "client_info": {
    "ip_address": "198.51.100.45",
    "user_agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64)..."
  },
  "status": "SUCCESS",
  "metadata": {
    "session_id": "18fba81b-5e91-4473-8208-8f85f1c24e6b",
    "mode": "FULL_CONTROL",
    "consent_required": true,
    "consent_granted_by": "STUDENT-04"
  },
  "created_at": "2026-09-10T00:18:24.120Z"
}
```

---

## 4. Database Immutability Enforcements

```sql
-- Trigger preventing modifications or deletions of audit logs
CREATE OR REPLACE FUNCTION prevent_audit_log_tampering()
RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'Audit logs are immutable. UPDATE and DELETE operations are forbidden.';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_audit_logs_immutable
BEFORE UPDATE OR DELETE ON audit_logs
FOR EACH ROW
EXECUTE FUNCTION prevent_audit_log_tampering();
```
