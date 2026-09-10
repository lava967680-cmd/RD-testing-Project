# ControlHub: Device Enrollment & Cryptographic Identity

## 1. Enrollment Overview & Security Boundary

Device enrollment is the foundational gate of the ControlHub zero-trust architecture. An endpoint computer can **never** be managed without completing this cryptographic enrollment protocol.

```mermaid
sequenceDiagram
    autonumber
    actor Admin as Admin Console
    participant API as ControlHub API
    participant DB as Database
    participant Agent as Windows Agent Installer

    Admin->>API: POST /enrollment/invitations {group_id, expires_in, approval_required}
    API->>API: Generate 256-bit cryptographically random token
    API->>DB: Store SHA-256(token), expiration, organization_id
    API-->>Admin: Return enrollment invitation string (CH-ENROLL-...)

    Note over Agent: Installer runs on authorized Windows workstation
    Agent->>Agent: Generate Ed25519 Keypair (Private key in DPAPI)
    Agent->>API: POST /enrollment/handshake {token, hostname, os_info, public_key}
    API->>DB: Verify token hash, active status, and not expired
    alt Token Invalid / Expired / Exhausted
        API-->>Agent: 403 Forbidden (Invalid enrollment token)
    else Token Valid
        API->>DB: Increment token usage count
        API->>DB: Insert device with status = 'PENDING'
        API->>DB: Store public key in device_credentials
        API->>DB: Emit audit_log("device.enroll", device_id)
        API-->>Agent: 201 Created {device_id, status: "PENDING"}
    end

    alt Policy requires Admin Approval
        Note over Admin,API: Admin reviews pending devices in dashboard
        Admin->>API: POST /devices/{id}/approve
        API->>DB: UPDATE devices SET status = 'ACTIVE' WHERE id = {id}
    else Auto-Approval Policy Active
        API->>DB: UPDATE devices SET status = 'ACTIVE'
    end

    Agent->>API: Connect WSS /ws/agent with Ed25519 Challenge Signature
    API-->>Agent: Connection Accepted -> Telemetry Streaming Begins
```

---

## 2. Enrollment Token Properties

- **Entropy**: 256 bits generated via cryptographically secure pseudo-random number generator (`crypto/rand`).
- **Encoding**: Alphanumeric base32 or hex string prefixed with `CH-ENROLL-` for easy validation and scanning.
- **Hash-at-Rest**: The database **never** stores the raw plaintext invitation token. Only the `SHA-256` hash of the token is persisted.
- **Expiration**: Strict TTL (default: 24 hours, maximum 7 days).
- **Usage Limit**: Default single-use (`max_uses: 1`), configurable for bulk laboratory deployments with an explicit limit (e.g., 30 uses for Computer Lab 2).
- **Tenant & Group Binding**: Bound permanently to the issuing `organization_id` and optional target `device_group_id`.

---

## 3. Cryptographic Device Identity & Keystore

1. **Algorithm**: **Ed25519** (RFC 8032 Edwards-curve Digital Signature Algorithm).
2. **Key Generation**: Executed locally on the Windows machine during agent first run.
3. **Private Key Storage on Windows**:
   - Stored in `%ProgramData%\ControlHub\keys\identity.key`.
   - Encrypted with Windows DPAPI (`CryptProtectData`) using `CRYPTPROTECT_LOCAL_MACHINE` scope.
   - Access Control List (ACL) restricted strictly to `NT AUTHORITY\SYSTEM` and `BUILTIN\Administrators`.
   - The private key is non-exportable and is never transmitted over the network.
4. **Public Key Registration**:
   - 32-byte raw public key transmitted during the TLS handshake to the backend.

---

## 4. Device Lifecycle State Machine

```mermaid
stateDiagram-v2
    [*] --> PENDING: Initial Handshake Completed
    PENDING --> ACTIVE: Admin Approval Granted
    PENDING --> REVOKED: Admin Rejects Enrollment
    ACTIVE --> OFFLINE: Heartbeat Missed (> 45s)
    OFFLINE --> ACTIVE: Heartbeat Resumed
    ACTIVE --> DISABLED: Admin Temporarily Disables
    DISABLED --> ACTIVE: Admin Re-enables
    ACTIVE --> REVOKED: Admin Revokes Credentials
    DISABLED --> REVOKED: Admin Revokes Credentials
    OFFLINE --> REVOKED: Admin Revokes Credentials
    REVOKED --> REMOVED: Soft/Hard Deletion from Inventory
    REMOVED --> [*]
```

### State Definitions:
- **`PENDING`**: Device registered its identity and hardware specs but awaits administrator approval.
- **`ACTIVE`**: Fully authorized. Telemetry, remote assistance, and maintenance commands are allowed.
- **`OFFLINE`**: Device is active in inventory but has disconnected from the realtime gateway.
- **`DISABLED`**: Device is administratively silenced; agent connections are rejected with code 403.
- **`REVOKED`**: Cryptographic credentials invalidated; cannot reconnect without fresh re-enrollment.
- **`REMOVED`**: Deleted from organization inventory and archived in compliance audit logs.
