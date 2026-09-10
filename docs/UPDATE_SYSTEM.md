# ControlHub: Secure Agent Update & Installer Subsystem

## 1. Secure Update Workflow

ControlHub uses an automated, fail-safe update pipeline with mandatory cryptographic verification.

```mermaid
sequenceDiagram
    autonumber
    participant UpdateServer as ControlHub Release Server
    participant Agent as ControlHub Windows Agent
    participant Service as Windows Service Manager (SCM)
    participant Overlay as ControlHub User Overlay

    UpdateServer->>Agent: UPDATE_AVAILABLE {version: "1.1.0", url: "...", sha256: "...", signature: "..."}
    Agent->>Agent: 1. Check organization update policy (Allowed maintenance window?)
    Agent->>UpdateServer: Download update package over TLS 1.3
    Agent->>Agent: 2. Verify SHA-256 Checksum against expected hash
    Agent->>Agent: 3. Verify Ed25519 digital signature against baked-in platform public key
    alt Signature Fails or Mismatches
        Agent->>Agent: Abort & quarantine payload
        Agent->>UpdateServer: Report UPDATE_FAILED (Signature mismatch error)
    else Signature Valid
        Agent->>Agent: Stage new binary to %ProgramData%\ControlHub\updates\staged.exe
        Agent->>Agent: Spawn updater helper process with rollback backup
        Agent->>Service: Stop ControlHubSvc
        Agent->>Service: Replace binary and restart ControlHubSvc
        Service->>Agent: Start ControlHubSvc v1.1.0
        Agent->>Agent: 4. Perform self-diagnostic health check (Ping local loopback)
        alt Health Check Fails
            Agent->>Agent: Rollback to previous known good binary
            Agent->>UpdateServer: Report UPDATE_ROLLED_BACK
        else Health Check Passes
            Agent->>Agent: Finalize installation & delete backup
            Agent->>UpdateServer: Report UPDATE_SUCCESS
        end
    end
```

---

## 2. Integrity & Cryptographic Signing

- **Release Signing**: Official release binaries are signed using a detached **Ed25519** signature by the ControlHub build pipeline.
- **Root of Trust**: The agent binary embeds the ControlHub Release Authority Public Key in compiled read-only memory.
- **Zero Downgrade Attacks**: The agent verifies that the update version number is strictly greater than the currently installed version, preventing downgrade attacks.

---

## 3. Windows Installer & Service Architecture

The Windows installer (`ControlHub-Setup.exe` / `.msi`) is designed for enterprise deployments via Microsoft Intune, Group Policy (GPO), or manual installation.

### 3.1 Installation Actions
1. **Target Directory**: Installs binaries to `%ProgramFiles%\ControlHub\`.
2. **Protected Storage**: Creates `%ProgramData%\ControlHub\` with restricted NTFS ACLs:
   - `NT AUTHORITY\SYSTEM`: Full Control
   - `BUILTIN\Administrators`: Full Control
   - `BUILTIN\Users`: Read & Execute only (no write/tamper permissions)
3. **Windows Service Registration**:
   - Registers `ControlHubSvc` running under `NT AUTHORITY\SYSTEM`.
   - Startup type: `Automatic` (delayed start supported).
   - Recovery options: Restart service on failure (1 min, 5 min delay).
4. **User Session Companion**:
   - Registers `ControlHubOverlay.exe` in the Run registry key (`HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\Run`) to launch inside the active interactive user session.
   - Responsible for rendering the top-most visual screen border and consent popups during remote desktop sessions.

### 3.2 Clean Uninstallation & Ethical Transparency
- **Add/Remove Programs**: Properly registered in Windows Apps & Features with publisher name, version, and icon.
- **Uninstall Password / Token**: Can be configured with an administrative uninstall token to prevent unauthorized student/user removal.
- **Complete Clean Removal**: Cleanly terminates services, removes drivers, purges configuration caches, and informs the backend to mark device `REMOVED`.
- **Zero Stealth**: No rootkits, driver disguises, or unkillable watchdog hooks.
