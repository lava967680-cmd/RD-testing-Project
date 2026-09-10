# ControlHub: REST API & Realtime WebSocket Specification

**Base URL**: `https://api.controlhub.local/api/v1`  
**Authentication**: `Authorization: Bearer <JWT>`  
**Content-Type**: `application/json`

---

## 1. Authentication & Session Endpoints

### 1.1 `POST /auth/login`
Authenticates an administrator or operator. If MFA is enabled on the account, returns an intermediate challenge token.
- **Request**:
  ```json
  {
    "email": "admin@acme-academy.org",
    "password": "SecurePassword123!"
  }
  ```
- **Response (Standard Success - 200 OK)**:
  ```json
  {
    "access_token": "eyJhbGciOiJIUzI1NiIsIn...",
    "refresh_token": "rt_8fbc2e77e23b890918...",
    "expires_in": 900,
    "user": {
      "id": "c1f72776-9ec6-4f40-8b17-7435f3dfd720",
      "email": "admin@acme-academy.org",
      "full_name": "Admin User",
      "organization_id": "771e8bfb-cf98-4c28-98e3-0d6e6443c21a",
      "role": "Administrator"
    }
  }
  ```
- **Response (MFA Required - 200 OK)**:
  ```json
  {
    "mfa_required": true,
    "mfa_token": "mfa_challenge_8829103e..."
  }
  ```

### 1.2 `POST /auth/mfa/verify`
Completes MFA login using a TOTP 6-digit code.
- **Request**:
  ```json
  {
    "mfa_token": "mfa_challenge_8829103e...",
    "code": "582194"
  }
  ```
- **Response (200 OK)**:
  Same payload as standard login success (JWT access + refresh token).

### 1.3 `POST /auth/refresh`
Rotates the refresh token and returns a fresh access token.
- **Request**:
  ```json
  { "refresh_token": "rt_8fbc2e77e23b890918..." }
  ```
- **Response (200 OK)**:
  ```json
  {
    "access_token": "eyJhbGciOiJIUzI1NiIsIn...",
    "refresh_token": "rt_910ca3148ab091823...",
    "expires_in": 900
  }
  ```

### 1.4 `POST /auth/logout`
Revokes active refresh tokens and blacklists the current JWT access token in Redis.

---

## 2. Organization Management

### 2.1 `GET /organizations/current`
Fetches tenant details, policies, device quota, and active license tier.

### 2.2 `PATCH /organizations/current`
Updates organizational policies (e.g. `require_remote_consent: true`, `telemetry_interval_sec: 15`).

---

## 3. Devices & Enrollment

### 3.1 `POST /enrollment/invitations`
Generates a cryptographically random, one-time enrollment token bound to the organization.
- **Request**:
  ```json
  {
    "target_group_id": "8f8b1b9e-6441-4796-9f44-93e18a0029b1",
    "expires_in_hours": 24,
    "max_uses": 1,
    "requires_approval": true
  }
  ```
- **Response (201 Created)**:
  ```json
  {
    "invitation_id": "406a4b18-b80c-44ae-b5bb-7fa51b88e1bb",
    "enrollment_token": "CH-ENROLL-7FA51B88E1BB94028A1",
    "expires_at": "2026-09-11T00:00:00Z"
  }
  ```

### 3.2 `POST /enrollment/handshake` (Agent Endpoint)
Executed by the agent installer to register the device public key.
- **Request**:
  ```json
  {
    "enrollment_token": "CH-ENROLL-7FA51B88E1BB94028A1",
    "hostname": "LAB-PC-042",
    "os_name": "Windows 11 Pro 64-bit",
    "os_version": "10.0.22631",
    "public_key": "MCowBQYDK2VwAyEAx5w...",
    "agent_version": "1.0.0"
  }
  ```
- **Response (201 Created)**:
  ```json
  {
    "device_id": "90e722c1-bb3e-4623-a128-44fb6b907a01",
    "status": "PENDING",
    "organization_name": "Acme IT Academy"
  }
  ```

### 3.3 `GET /devices`
Lists managed devices for the tenant, filterable by group, status, or search query.

### 3.4 `GET /devices/{deviceId}`
Fetches detailed device hardware inventory, status, and network configuration.

### 3.5 `POST /devices/{deviceId}/approve`
Approves an enrolled device in `PENDING` state, promoting it to `ACTIVE`.

### 3.6 `POST /devices/{deviceId}/revoke`
Revokes device cryptographic identity and terminates any active connections.

### 3.7 `DELETE /devices/{deviceId}`
Removes device from inventory and audits deletion.

---

## 4. Device Groups

- `GET /groups`: Lists all groups and member counts.
- `POST /groups`: Creates a new device group (e.g., "Exam Lab 1").
- `GET /groups/{id}`: Group details and member device list.
- `PATCH /groups/{id}`: Rename group or adjust parent group.
- `DELETE /groups/{id}`: Deletes group.
- `POST /groups/{id}/devices`: Adds device IDs to group.
- `DELETE /groups/{id}/devices/{deviceId}`: Removes device from group.

---

## 5. Remote Support Sessions & WebRTC

### 5.1 `POST /devices/{deviceId}/sessions`
Initiates an authorized remote desktop session.
- **Request**:
  ```json
  {
    "mode": "FULL_CONTROL",
    "prompt_consent": true
  }
  ```
- **Response (201 Created)**:
  ```json
  {
    "session_id": "18fba81b-5e91-4473-8208-8f85f1c24e6b",
    "status": "REQUESTED",
    "ice_servers": [
      { "urls": "stun:stun.controlhub.local:3478" },
      { "urls": "turn:turn.controlhub.local:3478", "username": "...", "credential": "..." }
    ]
  }
  ```

### 5.2 `POST /sessions/{sessionId}/terminate`
Closes the active WebRTC connection and clears endpoint visual indicators.

---

## 6. Remote Commands & Job Queue

### 6.1 `POST /devices/{deviceId}/commands`
Dispatches a controlled command job.
- **Request**:
  ```json
  {
    "command_type": "REBOOT",
    "parameters": {
      "delay_seconds": 60,
      "message": "Scheduled maintenance reboot in 60 seconds."
    }
  }
  ```
- **Response (202 Accepted)**:
  ```json
  {
    "command_id": "cd5e1003-8821-4d1a-b6d1-419b4899c43b",
    "status": "QUEUED"
  }
  ```

### 6.2 `GET /commands/{commandId}`
Polls status and fetches execution stdout/stderr results.

---

## 7. Realtime WebSocket Events

**WebSocket URL**: `wss://api.controlhub.local/ws/client`

### Event Schema
```json
{
  "event": "device.telemetry",
  "organization_id": "771e8bfb-cf98-4c28-98e3-0d6e6443c21a",
  "timestamp": "2026-09-10T00:15:30Z",
  "data": {
    "device_id": "90e722c1-bb3e-4623-a128-44fb6b907a01",
    "cpu_percent": 24.5,
    "ram_percent": 58.2,
    "disk_percent": 41.0,
    "status": "ONLINE"
  }
}
```

### Supported Event Types
- `device.online`: Device completed cryptographic handshake.
- `device.offline`: Device connection dropped or timed out.
- `device.telemetry`: Periodic CPU, RAM, Disk and network metrics.
- `device.alert`: Threshold exceeded (e.g. CPU > 90%).
- `device.command_status`: Job transitioned to `RUNNING`, `SUCCESS`, or `FAILED`.
- `session.status`: Remote desktop consent or state update.
- `enrollment.status`: Device enrollment handshake event.
