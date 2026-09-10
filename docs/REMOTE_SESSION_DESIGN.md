# ControlHub: Authorized Remote Support & WebRTC Architecture

## 1. Remote Session Authorization Flow

Remote desktop control is an intensely privileged operation. ControlHub implements multi-stage authorization, real-time consent, and mandatory endpoint visibility.

```mermaid
sequenceDiagram
    autonumber
    actor Admin as Admin Console (Browser)
    participant API as API Service
    participant Signaling as Realtime Gateway (WS)
    participant Agent as Windows Agent
    actor User as Workstation User

    Admin->>API: POST /devices/{id}/sessions {mode: "FULL_CONTROL"}
    API->>API: Verify Admin RBAC ('device.remote')
    API->>API: Verify Device status == 'ACTIVE'
    API->>Signaling: Send SESSION_REQUEST to Agent
    
    opt Organization Policy Requires User Consent
        Signaling->>Agent: Display Consent Dialog on Endpoint
        Agent->>User: Prompt: "Admin John Doe is requesting Remote Control. Allow?"
        User->>Agent: Click "Allow Support Session"
    end

    Agent->>Signaling: Send CONSENT_GRANTED
    Signaling-->>Admin: Consent Approved; Dispatch WebRTC ICE / SDP Offer

    Note over Agent: Display Persistent Neon Blue Screen Border & Overlay Banner
    Admin->>Agent: Establish Direct WebRTC PeerConnection (SRTP/DTLS)
    Agent-->>Admin: Video Stream (Desktop Duplication API + H.264 / VP8)
    Admin->>Agent: Keyboard & Mouse Input DataChannel
    
    alt Either Party Terminates
        Admin->>Signaling: Terminate Session
    else User Clicks Local "End Session"
        User->>Agent: Click "Disconnect Admin" on Overlay
    end

    Agent->>Agent: Remove Screen Border & Overlay Banner
    Signaling->>API: Session Ended -> Write Audit Log & Event Duration
```

---

## 2. Low-Latency Media Transport Pipeline

ControlHub uses **WebRTC** over raw UDP with SRTP encryption for desktop video, input injection, and file transfer.

| Layer | Implementation | Notes |
| :--- | :--- | :--- |
| **Screen Capture** | Windows Desktop Duplication API (DXGI) | Zero-copy GPU surface acquisition; operates at up to 60 FPS with minimal CPU overhead. |
| **Video Encoding** | OpenH264 / VP8 Software + Hardware NVENC / Intel QSV | Dynamically adapts bitrates (500 kbps to 8 Mbps) according to network packet loss and RTT. |
| **Signaling** | ControlHub Realtime Gateway (WebSockets) | Exchanges SDP offers, answers, and ICE candidates between browser and agent. |
| **NAT Traversal** | STUN / TURN (coturn) | P2P direct connection prioritized via STUN; automatic fallback to encrypted TLS TURN relay. |
| **Input Injection** | Windows `SendInput` API | Injects mouse clicks, movements, and key events only when session is active and input mode is permitted. |
| **Clipboard Sync** | Encrypted WebRTC DataChannel | Bidirectional text clipboard synchronization with user privacy controls. |

---

## 3. Mandatory Endpoint Transparency & Anti-Surveillance Controls

> [!IMPORTANT]
> Hidden surveillance, covert monitoring, and undetectable remote control are strictly prohibited. ControlHub guarantees local user awareness:

1. **Persistent Visual Border**:
   - A non-bypassable, 4-pixel solid colored border wraps the entire perimeter of all active monitors for the duration of the session.
   - Rendered using a native GDI / Direct2D transparent layered window with `WS_EX_TOPMOST` and `WS_EX_TRANSPARENT`.
2. **Interactive Status Overlay Banner**:
   - Positioned visibly at top-center of the primary monitor.
   - Displays: `ControlHub Active Remote Session | Connected to: John Doe (IT Admin)`.
   - Includes a red **[Terminate Session]** button empowering the local workstation user to abort the session instantly.
3. **Session Audit Trail**:
   - The start timestamp, end timestamp, operator identity, IP address, and termination initiator are recorded in `remote_sessions` and `audit_logs`.
   - Video frames are transported strictly in memory and are **never** recorded or archived on disk without explicit organizational recording policies.
