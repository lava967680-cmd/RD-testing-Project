# ControlHub: Phased Engineering Roadmap & Milestones

This roadmap defines the structured implementation progression for ControlHub from foundational specifications through to enterprise-ready commercial deployment.

```mermaid
gantt
    title ControlHub Implementation Roadmap
    dateFormat  YYYY-MM-DD
    section Foundation
    Stage 1: Requirements & Scope       :done,    des1, 2026-09-10, 2d
    Stage 2: Architecture & Threat Model :done,    des2, after des1, 2d
    Stage 3: Database Schema & Migrations:active,  des3, after des2, 3d
    section Core MVP
    Stage 4: Auth, MFA & RBAC           :         des4, after des3, 4d
    Stage 5: Secure Device Enrollment   :         des5, after des4, 4d
    Stage 6: Windows Agent Foundation   :         des6, after des5, 5d
    Stage 7: Telemetry & Heartbeats     :         des7, after des6, 3d
    Stage 8: Admin Dashboard UI         :         des8, after des7, 4d
    Stage 9: Group Management           :         des9, after des8, 3d
    Stage 10: Authorized Remote Support :         des10, after des9, 6d
    section Enterprise Operations
    Stage 11: Controlled Job Pipeline   :         des11, after des10, 4d
    Stage 12: Safe Bulk Operations      :         des12, after des11, 3d
    Stage 13: Health Alerts             :         des13, after des12, 3d
    Stage 14: Software Inventory        :         des14, after des13, 4d
    Stage 15: Security Hardening & Audit:         des15, after des14, 4d
    Stage 16: Licensing & Offline Grace :         des16, after des15, 3d
    Stage 17: Signed Installer & Updates:         des17, after des16, 4d
    Stage 18: Controlled Beta Pilot     :         des18, after des17, 7d
```

---

## Milestone Specifications

### Stage 1: Requirements Definition
- **Purpose**: Freeze product scope, target personas, ethical security boundary, and commercial MVP feature boundary.
- **Deliverable**: `docs/PRODUCT_REQUIREMENTS.md`.
- **Status**: **Completed**.

### Stage 2: Architecture & Security Trust Model
- **Purpose**: Finalize microservice boundaries, communication protocols (REST, WSS, WebRTC), zero-trust endpoint model, and STRIDE threat analysis.
- **Deliverables**: `docs/SYSTEM_ARCHITECTURE.md`, `docs/SECURITY_ARCHITECTURE.md`, `docs/THREAT_MODEL.md`.
- **Status**: **Completed**.

### Stage 3: Database Schema & Migrations
- **Purpose**: Implement PostgreSQL migrations with foreign key constraints, tenant isolation indexing, immutable audit triggers, and database seed scripts.
- **Deliverables**: `database/migrations/000001_initial_schema.up.sql`, `database/seeds/001_initial_seed.sql`, `docs/DATABASE_DESIGN.md`.
- **Status**: **In Progress / Current Milestone**.

### Stage 4: Authentication, MFA & RBAC
- **Purpose**: Implement secure password hashing (Argon2id), TOTP multi-factor verification, JWT token rotation, and granular RBAC middleware in Go.
- **Deliverables**: `services/api/internal/auth/`, `packages/auth/`, test suite verifying token expiry and permission denials.
- **Status**: Queued.

### Stage 5: Secure Device Enrollment
- **Purpose**: One-time cryptographically random enrollment invitation tokens, Ed25519 device keypair exchange, and administrator approval gate.
- **Deliverables**: `services/api/internal/enrollment/`, enrollment handler and database state transitions.
- **Status**: Queued.

### Stage 6: Windows Agent Foundation
- **Purpose**: Native Rust agent service with DPAPI key storage, TLS WebSocket client, and heartbeat daemon.
- **Deliverables**: `apps/windows-agent/src/`, `Cargo.toml`, Windows service scaffolding.
- **Status**: Queued.

### Stage 7: Telemetry & Monitoring Pipeline
- **Purpose**: System metrics collection (CPU, RAM, Disk, Network), Redis high-speed caching, and PostgreSQL batch rollup worker.
- **Deliverables**: `services/realtime/`, `services/worker/`, telemetry ingestion handlers.
- **Status**: Queued.

### Stage 8: Real-Time Admin Dashboard
- **Purpose**: Modern React 18 + TypeScript admin interface displaying live device status cards, search, filters, and offline/error states.
- **Deliverables**: `apps/admin-console/`, interactive dashboard views and API client.
- **Status**: Queued.

### Stage 9: Device Group Management
- **Purpose**: Hierarchical classroom/lab organization containers, drag-and-drop or bulk assignment, and group health stats.
- **Deliverables**: Group controllers, UI group explorer tree, and group membership APIs.
- **Status**: Queued.

### Stage 10: Authorized Remote Support (WebRTC)
- **Purpose**: One-to-one low latency remote desktop with mandatory visual border, consent dialog, and input injection.
- **Deliverables**: WebRTC signaling gateway, DXGI desktop capture, and browser video receiver canvas.
- **Status**: Queued.

### Stages 11 – 18: Enterprise Expansion
- **Stage 11**: Controlled Job Pipeline & Command Execution.
- **Stage 12**: Safe Bulk Management with Explicit Confirmation Dialogs.
- **Stage 13**: Health Threshold Alerts & Notification Dispatch.
- **Stage 14**: Software Inventory & MSI Deployment.
- **Stage 15**: Multi-Tenant Security Hardening, Penetration Testing & Token Replay Audits.
- **Stage 16**: Tiered Licensing, Entitlements & Resilient Offline Grace Period.
- **Stage 17**: Cryptographically Signed Windows Installer & Self-Update Pipeline.
- **Stage 18**: Controlled Customer Pilot & Reliability Hardening.
