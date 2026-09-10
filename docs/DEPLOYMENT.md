# ControlHub: Deployment & Infrastructure Architecture

## 1. Scalability Architecture & Sizing

ControlHub is architected to scale linearly from small labs of 10–50 machines to MSPs and large university campuses managing thousands of concurrent endpoints.

```mermaid
graph TD
    subgraph Edge Layer
        DNS[DNS / Cloudflare]
        LB[Load Balancer / Reverse Proxy - Nginx / Envoy]
    end

    subgraph Service Tier - Horizontally Scalable
        API1[API Service Pod 1]
        API2[API Service Pod 2]
        RT1[Realtime Gateway Pod 1]
        RT2[Realtime Gateway Pod 2]
        Worker1[Worker Service Pod 1]
    end

    subgraph Stateful Infrastructure
        RedisCluster[(Redis Cluster / Sentinel)]
        PGPrimary[(PostgreSQL 16 Primary)]
        PGReplica[(PostgreSQL 16 Read Replica)]
    end

    subgraph Media Infrastructure
        COTURN[Coturn STUN/TURN Relays]
    end

    DNS --> LB
    LB -->|REST /api| API1 & API2
    LB -->|WebSocket /ws| RT1 & RT2
    
    API1 & API2 --> RedisCluster
    RT1 & RT2 --> RedisCluster
    Worker1 --> RedisCluster

    API1 & API2 --> PGPrimary
    Worker1 --> PGPrimary
    API1 & API2 -.->|Read Queries| PGReplica

    RT1 & RT2 -.-> COTURN
```

---

## 2. Infrastructure Sizing by Device Fleet

| Device Fleet Size | API / Gateway Replicas | PostgreSQL Sizing | Redis Sizing | Network Bandwidth |
| :--- | :--- | :--- | :--- | :--- |
| **Small Lab (10 - 50 Devices)** | 1 instance (combined) | 2 vCPU, 4 GB RAM | 1 vCPU, 1 GB RAM | 10 Mbps |
| **Medium Campus (50 - 500 Devices)** | 2 API, 2 Realtime | 4 vCPU, 8 GB RAM | 2 vCPU, 4 GB RAM | 50 Mbps |
| **Large Enterprise (500 - 2,500 Devices)** | 4 API, 4 Realtime, 2 Workers | 8 vCPU, 32 GB RAM (Dedicated NVMe) | 4 vCPU, 8 GB RAM Cluster | 250 Mbps |
| **MSP Fleet (2,500 - 20,000+ Devices)** | Auto-scaled Kubernetes Deployments | Multi-node Patroni HA + PgBouncer | Redis Sentinel 3-node HA | 1 Gbps+ |

---

## 3. Local Development Deployment (Docker Compose)

For rapid development and local testing, use the included Docker Compose configuration:

```bash
# Navigate to Docker environment
cd infrastructure/docker

# Start PostgreSQL and Redis
docker compose up -d postgres redis

# Verify health status
docker compose ps
```

### Environment Configuration (`.env`):
```ini
# Platform Configuration
ENVIRONMENT=development
PORT=8080
REALTIME_PORT=8081

# Database Configuration
DATABASE_URL=postgres://controlhub:controlhub_dev_secret_password@localhost:5432/controlhub_db?sslmode=disable
DB_MAX_OPEN_CONNS=25
DB_MAX_IDLE_CONNS=10

# Redis Cache & PubSub
REDIS_URL=redis://localhost:6379/0

# Cryptography & Security
JWT_SECRET=development_jwt_signing_key_must_be_over_32_characters_long!
AGENT_ENROLLMENT_HMAC_KEY=development_enrollment_hmac_secret_key_32_bytes!

# WebRTC STUN/TURN
STUN_SERVER=stun:stun.l.google.com:19302
```

---

## 4. Production Security & Network Configuration

1. **TLS 1.3 Strict Mode**: HTTPS and WSS connections terminated at the edge with HSTS headers (`Strict-Transport-Security: max-age=31536000; includeSubDomains`).
2. **WebSocket Keep-Alive**: Proxy timeout set to 60s with ping-pong frames dispatched every 15s to keep connections alive across stateful firewalls.
3. **Database Connection Pooling**: Built-in Go connection pool (`pgxpool` / `sql.DB`) configured with connection lifespans of 30 minutes to prevent resource exhaustion.
