# Service Gateway Design Document

## Overview

A lightweight, authenticated REST gateway written in Go that provides integration platforms with access to internal services. The gateway enforces security via pre-shared secret authentication with replay protection, and all sensitive credentials are encrypted using [secretprotector](https://github.com/edsilegxrepo/secretprotector).

### Design Goals

1. **Security First** — All secrets encrypted at rest; replay-resistant authentication
2. **Localhost Only** — Gateway binds exclusively to `127.0.0.1`; external access requires a reverse proxy with SSL termination
3. **Extensible** — Easy to add new backend services (Go code, HTTP proxies, binary wrappers)
4. **Dynamic Configuration** — Service configs are hot-reloadable without restart
5. **Minimal Dependencies** — Standard library where possible
6. **Integration Platform Compatible** — Works with any REST-capable integration platform

### Supported Service Types

| Type | Backend | Access Pattern | Use Cases |
|------|---------|----------------|-----------|
| `mongodb` | MongoDB Go driver | Named queries + restricted ad-hoc | Database access, reports, lookups |
| `sql` | xo/dburl + database/sql (45+ DBs) | Named queries + restricted ad-hoc | PostgreSQL, MySQL, Oracle, SQL Server |
| `rclone` | librclone (40+ backends) | Named operations + restricted raw | GCS, S3, Azure Blob, SFTP transfers |
| `mq` | Watermill (Kafka, RabbitMQ, NATS, SQS, Pub/Sub) | Named operations | Message queue produce/consume |
| `ldap` | go-ldap | Named operations | User/group lookups, auth validation |
| `o365mbx` | o365mbx library (Graph API) | Named operations | Email processing, attachment extraction |
| `pdf-extractor` | pdf-extractor library | One service per task | PDF text/data extraction |
| `binary-wrapper` | exec.Command | Argument whitelist | Legacy CLI tools |

**Planned:**

| Type | Backend | Access Pattern | Use Cases |
|------|---------|----------------|-----------|
| `http-proxy` | net/http | Path whitelist | Internal REST API forwarding |

---

## Architecture

```mermaid
flowchart TB
    subgraph IP[Integration Platform]
        T1[REST Task 1<br/>GET /auth/nonce<br/>Bearer: secret]
        T2[REST Task 2<br/>GET /api/v1/service/resource<br/>Bearer: secret, X-Nonce: uuid]
        T1 --> T2
    end

    subgraph GW[Service Gateway]
        Auth[Auth Layer<br/>Bearer+Nonce+RBAC]
        Router[Router<br/>/api/v1/svc]
        Registry[Service Registry]
        Auth --> Router --> Registry
    end

    subgraph Services[Backend Services]
        Mongo[(MongoDB<br/>Go driver)]
        SQL[(SQL DBs<br/>dburl)]
        Rclone[Rclone<br/>librclone]
        MQ[Message Queue<br/>Watermill]
        LDAP[LDAP/AD<br/>go-ldap]
        O365[O365 Mailbox<br/>Graph API]
        PDF[PDF Extractor<br/>Go library]
        Wrapper[Binary Wrapper<br/>exec.Cmd]
    end

    subgraph Cloud[Cloud Storage via Rclone]
        GCS[Google Cloud<br/>Storage]
        S3[Amazon S3]
        Azure[Azure Blob]
        SFTP[SFTP Servers]
    end

    subgraph Databases[SQL Databases via dburl]
        PG[PostgreSQL]
        MySQL[MySQL]
        MSSQL[SQL Server]
        Oracle[Oracle]
    end

    subgraph MessageBrokers[Message Brokers via Watermill]
        Kafka[Apache Kafka]
        RabbitMQ[RabbitMQ]
        NATS[NATS]
        SQS[AWS SQS]
    end

    subgraph Directory[Directory Services]
        AD[Active Directory]
        OpenLDAP[OpenLDAP]
    end

    IP -->|HTTP 127.0.0.1:3000| GW
    Registry --> Mongo
    Registry --> SQL
    Registry --> Rclone
    Registry --> MQ
    Registry --> LDAP
    Registry --> O365
    Registry --> PDF
    Registry --> Wrapper
    Rclone --> GCS
    Rclone --> S3
    Rclone --> Azure
    Rclone --> SFTP
    SQL --> PG
    SQL --> MySQL
    SQL --> MSSQL
    SQL --> Oracle
    MQ --> Kafka
    MQ --> RabbitMQ
    MQ --> NATS
    MQ --> SQS
    LDAP --> AD
    LDAP --> OpenLDAP
```

---

## Project Structure

```
service-gateway/
├── cmd/
│   └── gateway/
│       └── main.go                 # Entry point, bootstrap sequence
├── internal/
│   ├── config/
│   │   └── config.go               # Global config loading
│   ├── auth/
│   │   ├── middleware.go           # Bearer token + nonce + RBAC validation
│   │   ├── nonce.go                # In-memory nonce store with TTL
│   │   └── ratelimit.go            # Per-key rate limiting
│   ├── gateway/
│   │   ├── router.go               # Service dispatch based on URL
│   │   └── reload.go               # Hot-reload signal handling
│   └── services/
│       ├── interface.go            # Service interface definition
│       ├── types.go                # Service type registry (RegisterType, CreateService)
│       ├── registry.go             # Runtime service instance registry
│       ├── loader.go               # Dynamic service config loading
│       ├── base.go                 # Base service with common functionality
│       ├── mongodb/
│       │   └── handler.go          # MongoDB query service (init: RegisterType)
│       ├── sql/
│       │   └── handler.go          # SQL databases via dburl (init: RegisterType)
│       ├── rclone/
│       │   └── handler.go          # Cloud storage via librclone (init: RegisterType)
│       ├── mq/
│       │   └── handler.go          # Message queues via Watermill (init: RegisterType)
│       ├── ldap/
│       │   └── handler.go          # LDAP/AD queries via go-ldap (init: RegisterType)
│       ├── o365mbx/
│       │   └── handler.go          # O365 mailbox via Graph API (init: RegisterType)
│       ├── pdfextractor/
│       │   └── handler.go          # PDF extraction (init: RegisterType)
│       └── wrapper/
│           └── handler.go          # Binary wrapper (init: RegisterType)
├── config.json                     # Global config (auth, master key, service list)
├── services/                       # Service-specific configs (hot-reloadable)
│   ├── mongodb-production/
│   │   └── config.json
│   ├── mongodb-analytics/
│   │   └── config.json
│   ├── sql-postgres-production/
│   │   └── config.json
│   ├── sql-mysql-analytics/
│   │   └── config.json
│   ├── rclone-gcs-archive/
│   │   └── config.json
│   ├── rclone-sftp-backup/
│   │   └── config.json
│   ├── mq-kafka-orders/
│   │   └── config.json
│   ├── mq-rabbitmq-events/
│   │   └── config.json
│   ├── ldap-corp-ad/
│   │   └── config.json
│   ├── o365mbx-invoices/
│   │   └── config.json
│   ├── pdf-extractor-invoices/
│   │   └── config.json
│   └── legacy-report-daily/
│       └── config.json
├── go.mod
├── go.sum
├── DESIGN.md                       # This document
└── README.md
```

---

## Configuration Architecture

### Two-Level Configuration

| Level | File | Contains | Hot-Reload |
|-------|------|----------|------------|
| **Global** | `config.json` | Server, auth keys, master key, service list | On signal |
| **Service** | `services/{name}/config.json` | Connection, queries, limits | On signal |

### Global Config (`config.json`)

```json
{
  "server": {
    "port": 3000,
    "admin_endpoint": true
  },
  
  "master_key_env": "GATEWAY_MASTER_KEY",
  "master_key_path": "/etc/gateway/master.key",
  
  "services_dir": "services",
  
  "services": {
    "mongodb-production": {"enabled": true},
    "mongodb-analytics": {"enabled": true},
    "mongodb-metrics": {"enabled": false},
    "pdf-extractor-invoices": {"enabled": true},
    "pdf-extractor-contracts": {"enabled": true},
    "rclone-gcs-archive": {"enabled": true},
    "rclone-sftp-backup": {"enabled": true},
    "legacy-report-daily": {"enabled": true}
  },
  
  "auth": {
    "keys": {
      "ga-full-access": {
        "encrypted_secret": "BASE64_AES_GCM_CIPHERTEXT",
        "services": ["*"],
        "max_concurrent": 10,
        "rate_limit_rpm": 600,
        "description": "Integration Platform - all services"
      },
      "ga-production": {
        "encrypted_secret": "BASE64_AES_GCM_CIPHERTEXT",
        "services": ["mongodb-production", "pdf-extractor-invoices", "rclone-gcs-archive"],
        "rclone_access": "operations_only",
        "max_concurrent": 5,
        "rate_limit_rpm": 300,
        "description": "Integration Platform - production workflows"
      },
      "ga-analytics": {
        "encrypted_secret": "BASE64_AES_GCM_CIPHERTEXT",
        "services": ["mongodb-analytics"],
        "max_concurrent": 20,
        "rate_limit_rpm": 1000,
        "description": "Integration Platform - analytics team (higher limits)"
      },
      "monitoring": {
        "encrypted_secret": "BASE64_AES_GCM_CIPHERTEXT",
        "services": ["mongodb-metrics"],
        "max_concurrent": 2,
        "rate_limit_rpm": 60,
        "description": "Monitoring system - metrics only"
      }
    }
  }
}
```

**Key points:**
- `services` lists all authorized services — only listed services are loaded
- `enabled: false` disables a service without removing its folder
- Folder exists but not listed: ignored (security)
- Listed but no folder: warning at startup

### Service Config (`services/mongodb-production/config.json`)

Each service folder contains its own `config.json` with connection details, queries, and limits:

```json
{
  "type": "mongodb",
  "description": "Production database - orders and customers",
  "version": "1.3",
  "updated": "2024-01-15T10:00:00Z",
  "updated_by": "john.doe",
  
  "connection": {
    "encrypted_uri": "BASE64_AES_GCM_CIPHERTEXT",
    "database": "prod_db",
    "pool_size": 10,
    "timeout_seconds": 30
  },
  
  "limits": {
    "max_results": 1000,
    "default_limit": 100,
    "max_response_mb": 16,
    "allow_streaming": false
  },
  
  "collections": {
    "orders": {
      "enabled": true,
      "allow_adhoc": true,
      "adhoc_restrictions": {
        "required_filters": ["date"],
        "allowed_filters": ["date", "status", "customer_id", "region"],
        "denied_operators": ["$where", "$expr", "$function"],
        "denied_projections": ["internal_notes", "cost_price"]
      },
      "queries": {
        "recent-pending": {
          "description": "Pending orders from last 7 days",
          "filter": {"date": {"$gte": "{{today-7d}}"}, "status": "pending"},
          "sort": {"date": -1}
        },
        "by-customer": {
          "description": "Orders for a customer",
          "params": {
            "customer_id": {"type": "string", "required": true}
          },
          "filter": {"customer_id": "{{customer_id}}"}
        },
        "monthly-summary": {
          "description": "Order summary by status for a month",
          "params": {
            "year": {"type": "int", "required": true},
            "month": {"type": "int", "required": true, "min": 1, "max": 12}
          },
          "aggregation": [
            {"$match": {"date": {"$gte": "{{year}}-{{month}}-01", "$lt": "{{year}}-{{month+1}}-01"}}},
            {"$group": {"_id": "$status", "count": {"$sum": 1}, "total": {"$sum": "$amount"}}}
          ]
        }
      }
    },
    "customers": {
      "enabled": true,
      "allow_adhoc": false,
      "queries": {
        "by-id": {
          "params": {"id": {"type": "objectid", "required": true}},
          "filter": {"_id": "{{id}}"}
        },
        "search": {
          "params": {"name": {"type": "string", "required": true, "min_length": 3}},
          "filter": {"name": {"$regex": "{{name}}", "$options": "i"}},
          "max_results": 50
        }
      }
    },
    "audit_logs": {
      "enabled": false
    }
  }
}
```

### Service Type vs Service Name

Services are identified by **name** (folder name, used in URLs and RBAC), with **type** determining the handler:

| Service Name | Type | Purpose |
|--------------|------|---------|
| `mongodb-production` | `mongodb` | Production database |
| `mongodb-analytics` | `mongodb` | Analytics database (different creds, limits) |
| `pdf-extractor-invoices` | `pdf-extractor` | Invoice PDFs |
| `pdf-extractor-contracts` | `pdf-extractor` | Contract PDFs |
| `rclone-gcs-archive` | `rclone` | SFTP to GCS archive operations |
| `rclone-sftp-backup` | `rclone` | GCS to SFTP backup operations |
| `legacy-report-daily` | `binary-wrapper` | Daily report CLI |

**URLs use service name:**
```
GET /api/v1/mongodb-production/orders?query=recent-pending
GET /api/v1/mongodb-production/orders?filter={"status":"shipped"}
GET /api/v1/mongodb-analytics/reports?limit=5000
POST /api/v1/pdf-extractor-invoices/extract
POST /api/v1/rclone-gcs-archive/op/sftp-to-gcs-live
POST /api/v1/rclone-sftp-backup/op/gcs-to-sftp
```

---

## Query Definition System

### Query Modes

| Mode | Config | Use Case |
|------|--------|----------|
| **Named queries only** | `allow_adhoc: false` | High security, strict control |
| **Restricted ad-hoc** | `allow_adhoc: true` + restrictions | Flexible but bounded |
| **Open ad-hoc** | `allowed_filters: ["*"]` | Trusted internal use |

### Named Query Usage

```bash
# Simple named query
GET /api/v1/mongodb-production/orders?query=recent-pending

# Named query with parameters
GET /api/v1/mongodb-production/orders?query=by-customer&customer_id=C12345

# Named aggregation
GET /api/v1/mongodb-production/orders?query=monthly-summary&year=2024&month=6
```

### Ad-hoc Query Usage (where allowed)

```bash
# Ad-hoc with allowed filters
GET /api/v1/mongodb-production/orders?filter={"date":{"$gte":"2024-01-01"},"status":"pending"}

# Blocked - "amount" not in allowed_filters
GET /api/v1/mongodb-production/orders?filter={"amount":{"$gt":10000}}
# Returns 400 Bad Request: filter field "amount" not allowed

# Blocked - collection has allow_adhoc: false
GET /api/v1/mongodb-production/customers?filter={"email":"test@example.com"}
# Returns 400 Bad Request: ad-hoc queries not allowed, use a named query
```

### Template Variables

| Variable | Example | Description |
|----------|---------|-------------|
| `{{param_name}}` | `{{customer_id}}` | Parameter from query string |
| `{{today}}` | `2024-01-15` | Current date |
| `{{today-7d}}` | `2024-01-08` | Date offset |
| `{{now}}` | `2024-01-15T10:30:00Z` | Current timestamp |

---

## Hot-Reload System

### Reload Triggers

| Platform | Method | Command |
|----------|--------|---------|
| **Linux** | SIGHUP signal | `kill -HUP $(pidof gateway)` |
| **Windows** | Admin endpoint | `curl http://127.0.0.1:3000/_admin/reload` |
| **Both** | CLI flag | `gateway -reload` (signals running process) |

### What Gets Reloaded

| Component | Hot-Reload | Notes |
|-----------|------------|-------|
| Service query definitions | Yes | Immediate |
| Service limits | Yes | Immediate |
| Service enabled/disabled | Yes | Immediate |
| New services (in master list) | Yes | Discovered |
| API keys | Yes | Immediate |
| Connection credentials | No | Requires restart (reconnect) |
| Server port | No | Requires restart |
| Master key | No | Requires restart |

### Reload Implementation

```go
// Linux: SIGHUP handler
func (g *Gateway) setupSignalHandler() {
    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, syscall.SIGHUP)
    
    go func() {
        for range sigCh {
            log.Println("Received SIGHUP, reloading configuration...")
            if err := g.reload(); err != nil {
                log.Printf("Reload failed: %v (keeping previous config)", err)
            } else {
                log.Println("Configuration reloaded successfully")
            }
        }
    }()
}

// Admin endpoint (localhost only)
func (g *Gateway) handleAdminReload(w http.ResponseWriter, r *http.Request) {
    // Only allow from localhost
    if !isLocalhost(r.RemoteAddr) {
        http.Error(w, "Forbidden", http.StatusForbidden)
        return
    }
    
    if err := g.reload(); err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    
    w.Write([]byte(`{"status": "reloaded"}`))
}

// Reload logic
func (g *Gateway) reload() error {
    // 1. Load and validate new config
    newCfg, err := config.Load(g.configPath)
    if err != nil {
        return fmt.Errorf("invalid config: %w", err)
    }
    
    // 2. Decrypt new API keys
    newKeys, err := g.decryptAPIKeys(newCfg)
    if err != nil {
        return fmt.Errorf("key decryption failed: %w", err)
    }
    
    // 3. Load service configs
    newServices, err := g.loadServiceConfigs(newCfg)
    if err != nil {
        return fmt.Errorf("service config failed: %w", err)
    }
    
    // 4. Atomic swap
    g.mu.Lock()
    g.apiKeys = newKeys
    g.serviceConfigs = newServices
    g.mu.Unlock()
    
    return nil
}
```

### CLI Reload Command

```bash
# Send reload signal to running gateway
gateway -reload

# Windows: uses named pipe or HTTP
gateway.exe -reload
```

---

## Key Resolution

Master key is resolved using `libsecsecrets.ResolveKey()` with the following precedence:

1. **Environment Variable** (`master_key_env`) — **recommended for Windows**
2. **Key File** (`master_key_path`) — **recommended for Linux** (must have 0400/0600 permissions)

```go
masterKey, err := libsecsecrets.ResolveKey(ctx, "", cfg.MasterKeyEnv, cfg.MasterKeyPath)
defer libsecsecrets.ZeroBuffer(masterKey)
```

### Platform-Specific Key Storage

| Platform | Recommended Method | Reason |
|----------|-------------------|--------|
| **Windows** | Environment variable (`GATEWAY_MASTER_KEY`) | Service accounts complicate file ACLs; env vars set by admin are secure |
| **Linux** | Key file with `0400` permissions | Standard Unix security model; secretprotector enforces permissions |

**Windows setup:**
```powershell
# Set system-wide (requires admin)
setx GATEWAY_MASTER_KEY "64-char-hex-string" /M

# Verify
[System.Environment]::GetEnvironmentVariable("GATEWAY_MASTER_KEY", "Machine")
```

**Linux setup:**
```bash
# Create key file
secretprotector -generate > /etc/gateway/master.key
chmod 0400 /etc/gateway/master.key
chown gateway:gateway /etc/gateway/master.key
```

### Encrypting Secrets

**All secrets across all services MUST be encrypted with secretprotector.** This includes:

| Service Type | Encrypted Fields |
|--------------|------------------|
| **Global** | `auth.keys.*.encrypted_secret` (API keys) |
| **MongoDB** | `connection.encrypted_uri` |
| **SQL** | `connection.encrypted_dsn` |
| **Rclone** | `encrypted_config_pass` (rclone.conf password) |
| **O365MBX** | `encrypted_token` (JWT token) |
| **Binary Wrapper** | Any `encrypted_*` env vars |

Use the `secretprotector` CLI to encrypt values before adding to config:

```powershell
# Generate master key (once)
secretprotector -generate > D:\secrets\gateway.key

# Encrypt MongoDB connection string
secretprotector -key-file "D:\secrets\gateway.key" -encrypt "mongodb://user:pass@host:27017"

# Encrypt PostgreSQL DSN
secretprotector -key-file "D:\secrets\gateway.key" -encrypt "postgres://user:pass@host:5432/db"

# Encrypt SQL Server DSN
secretprotector -key-file "D:\secrets\gateway.key" -encrypt "sqlserver://user:pass@host/instance/db"

# Encrypt API key
secretprotector -key-file "D:\secrets\gateway.key" -encrypt "your-shared-secret"

# Encrypt JWT token
secretprotector -key-file "D:\secrets\gateway.key" -encrypt "eyJ0eXAiOiJKV1QiLCJhbGciOiJSUzI1NiIs..."
```

**Convention:** Any config field containing credentials should be named `encrypted_*` (e.g., `encrypted_uri`, `encrypted_dsn`, `encrypted_token`). The gateway decrypts these at startup using the master key.

---

## Authentication

### Authentication Layers

The gateway supports **defense-in-depth** with multiple optional authentication factors:

| Layer | Required | Purpose |
|-------|----------|---------|
| **Bearer Token** | Yes | Identity verification |
| **Server Nonce** | Yes | Replay protection |
| **RBAC** | Yes | Service-level access control |
| **IP Allowlist/Denylist** | Optional | Network-level restriction |
| **mTLS Client Certificate** | Optional | Cryptographic client identity |

All layers are evaluated in order — request must pass ALL enabled layers.

### Scheme: Bearer Token + Server-Issued Nonce + RBAC

Integration Platform cannot compute HMAC signatures, so we use a two-step flow with server-issued nonces for replay protection. Each API key has RBAC restrictions defining which services it can access.

### RBAC Model

**RBAC Rules:**
- `"services": ["*"]` — Access to all services
- `"services": ["mongodb-production", "pdf-extractor-invoices", "rclone-gcs-archive"]` — Access to listed services only
- Request to unauthorized service returns `403 Forbidden`
- `"rclone_access": "operations_only"` — Limits rclone to named operations (no raw access)

**Rate Limiting (per API key):**

| Setting | Default | Description |
|---------|---------|-------------|
| `max_concurrent` | 10 | Max simultaneous in-flight requests |
| `rate_limit_rpm` | 600 | Max requests per minute (sliding window) |

- Exceeding `max_concurrent` returns `429 Too Many Requests` with `Retry-After` header
- Exceeding `rate_limit_rpm` returns `429` with remaining window time
- Different keys can have different limits (analytics team gets higher limits than monitoring)

### Flow

```mermaid
sequenceDiagram
    participant IP as Integration Platform
    participant GW as Service Gateway

    IP->>GW: GET /api/v1/auth/nonce<br/>Authorization: Bearer api_key
    GW-->>IP: {"nonce": "uuid", "expires_in": 300}

    IP->>GW: GET /api/v1/mongodb-production/orders<br/>Authorization: Bearer api_key<br/>X-Nonce: uuid
    Note right of GW: RBAC check: does key<br/>have access to<br/>"mongodb-production"?
    GW-->>IP: {"data": [...]}
```

### Endpoints

| Endpoint | Auth Required | Description |
|----------|---------------|-------------|
| `GET /health` | No | Health check, returns `{"status": "ok"}` |
| `GET /_admin/reload` | Localhost only | Trigger config reload |
| `GET /api/v1/auth/nonce` | Bearer only | Get single-use nonce |
| `GET /api/v1/{service}/*` | Bearer + Nonce + RBAC | Service requests |

### Nonce Rules

- Nonces are UUIDs generated server-side
- Each nonce is valid for **one request only**
- Unused nonces expire after **5 minutes**
- Nonces are stored in-memory with automatic cleanup

### IP Allowlist/Denylist (Optional)

Per-key IP restrictions using CIDR notation:

```json
"ga-production": {
  "encrypted_secret": "...",
  "services": ["mongodb-production"],
  "ip_rules": {
    "enabled": true,
    "mode": "allowlist",
    "cidrs": [
      "10.0.0.0/8",
      "192.168.1.0/24",
      "172.16.50.100/32"
    ]
  }
}
```

**Modes:**
- `allowlist` — Only listed CIDRs can use this key (default when enabled)
- `denylist` — Listed CIDRs are blocked, all others allowed

**Implementation:**

```go
// internal/auth/ip.go

import "net"

type IPRules struct {
    Enabled bool     `json:"enabled"`
    Mode    string   `json:"mode"` // "allowlist" or "denylist"
    CIDRs   []string `json:"cidrs"`
    
    // Parsed at load time
    networks []*net.IPNet
}

func (r *IPRules) Parse() error {
    r.networks = make([]*net.IPNet, 0, len(r.CIDRs))
    for _, cidr := range r.CIDRs {
        _, network, err := net.ParseCIDR(cidr)
        if err != nil {
            // Try as single IP
            ip := net.ParseIP(cidr)
            if ip == nil {
                return fmt.Errorf("invalid CIDR or IP: %s", cidr)
            }
            // Convert single IP to /32 or /128
            bits := 32
            if ip.To4() == nil {
                bits = 128
            }
            network = &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)}
        }
        r.networks = append(r.networks, network)
    }
    return nil
}

func (r *IPRules) IsAllowed(remoteAddr string) bool {
    if !r.Enabled {
        return true
    }
    
    // Extract IP from "ip:port"
    host, _, err := net.SplitHostPort(remoteAddr)
    if err != nil {
        host = remoteAddr
    }
    ip := net.ParseIP(host)
    if ip == nil {
        return false
    }
    
    // Check if IP matches any CIDR
    matches := false
    for _, network := range r.networks {
        if network.Contains(ip) {
            matches = true
            break
        }
    }
    
    if r.Mode == "denylist" {
        return !matches // Deny if matches
    }
    return matches // Allow only if matches
}
```

**Extracting real client IP (behind proxy):**

```go
func getRealIP(r *http.Request, trustProxy bool) string {
    if trustProxy {
        // Trust X-Forwarded-For from nginx
        if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
            // Take first IP (original client)
            if idx := strings.Index(xff, ","); idx != -1 {
                return strings.TrimSpace(xff[:idx])
            }
            return strings.TrimSpace(xff)
        }
        if xri := r.Header.Get("X-Real-IP"); xri != "" {
            return xri
        }
    }
    // Fallback to direct connection
    host, _, _ := net.SplitHostPort(r.RemoteAddr)
    return host
}
```

### mTLS Client Certificates (Optional)

For maximum security, require client certificates:

```json
// config.json
{
  "server": {
    "port": 3000,
    "mtls": {
      "enabled": true,
      "ca_cert": "D:\\certs\\ca.crt",
      "require_client_cert": true,
      "verify_client_cert": true
    }
  }
}
```

**Per-key certificate binding:**

```json
"ga-production": {
  "encrypted_secret": "...",
  "services": ["mongodb-production"],
  "client_cert": {
    "enabled": true,
    "allowed_subjects": [
      "CN=goanywhere.internal.local,O=MyOrg",
      "CN=automation-svc,O=MyOrg"
    ],
    "allowed_fingerprints": [
      "SHA256:2CF24DBA5FB0A30E26E83B2AC5B9E29E1B161E5C1FA7425E73043362938B9824"
    ]
  }
}
```

**Matching rules:**
- `allowed_subjects` — Certificate subject must match one (exact or wildcard)
- `allowed_fingerprints` — Certificate SHA256 fingerprint must match one
- If both specified, either can match (OR logic)

**Implementation:**

```go
// internal/auth/mtls.go

import (
    "crypto/sha256"
    "crypto/tls"
    "crypto/x509"
    "encoding/hex"
    "fmt"
    "os"
)

type MTLSConfig struct {
    Enabled           bool   `json:"enabled"`
    CACert            string `json:"ca_cert"`
    RequireClientCert bool   `json:"require_client_cert"`
    VerifyClientCert  bool   `json:"verify_client_cert"`
}

type ClientCertRules struct {
    Enabled             bool     `json:"enabled"`
    AllowedSubjects     []string `json:"allowed_subjects"`
    AllowedFingerprints []string `json:"allowed_fingerprints"`
}

func (c *ClientCertRules) Validate(certs []*x509.Certificate) error {
    if !c.Enabled || len(certs) == 0 {
        return nil // mTLS not required for this key
    }
    
    cert := certs[0] // Leaf certificate
    
    // Check subject
    subject := cert.Subject.String()
    for _, allowed := range c.AllowedSubjects {
        if matchSubject(subject, allowed) {
            return nil
        }
    }
    
    // Check fingerprint
    fingerprint := sha256Fingerprint(cert)
    for _, allowed := range c.AllowedFingerprints {
        if strings.EqualFold(fingerprint, allowed) {
            return nil
        }
    }
    
    return fmt.Errorf("certificate not authorized: subject=%s fingerprint=%s", subject, fingerprint)
}

func sha256Fingerprint(cert *x509.Certificate) string {
    hash := sha256.Sum256(cert.Raw)
    return "SHA256:" + strings.ToUpper(hex.EncodeToString(hash[:]))
}

// Server TLS config with client cert verification
func CreateTLSConfig(cfg *MTLSConfig) (*tls.Config, error) {
    if !cfg.Enabled {
        return nil, nil
    }
    
    // Load CA certificate
    caCert, err := os.ReadFile(cfg.CACert)
    if err != nil {
        return nil, fmt.Errorf("read CA cert: %w", err)
    }
    
    caPool := x509.NewCertPool()
    if !caPool.AppendCertsFromPEM(caCert) {
        return nil, fmt.Errorf("invalid CA cert")
    }
    
    tlsConfig := &tls.Config{
        ClientCAs: caPool,
    }
    
    if cfg.RequireClientCert {
        if cfg.VerifyClientCert {
            tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
        } else {
            tlsConfig.ClientAuth = tls.RequireAnyClientCert
        }
    } else {
        tlsConfig.ClientAuth = tls.RequestClientCert
    }
    
    return tlsConfig, nil
}
```

**Accessing client cert in middleware:**

```go
func (m *AuthMiddleware) validateClientCert(r *http.Request, key *APIKey) error {
    if !key.ClientCert.Enabled {
        return nil
    }
    
    if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
        return fmt.Errorf("client certificate required")
    }
    
    return key.ClientCert.Validate(r.TLS.PeerCertificates)
}
```

### Combined Authentication Flow

```mermaid
sequenceDiagram
    participant Client
    participant Gateway
    participant Service

    Note over Client,Gateway: mTLS Handshake (if enabled)
    Client->>Gateway: TLS ClientHello + Client Cert
    Gateway->>Gateway: Verify cert against CA
    Gateway->>Gateway: Check cert subject/fingerprint
    
    Note over Client,Gateway: HTTP Request
    Client->>Gateway: GET /api/v1/auth/nonce<br/>Authorization: Bearer key
    Gateway->>Gateway: 1. Validate Bearer token
    Gateway->>Gateway: 2. Check IP allowlist
    Gateway->>Gateway: 3. Check client cert binding
    Gateway->>Gateway: 4. Rate limit check
    Gateway-->>Client: {"nonce": "uuid"}
    
    Client->>Gateway: GET /api/v1/service/...<br/>Authorization: Bearer key<br/>X-Nonce: uuid
    Gateway->>Gateway: 1-4. Same checks
    Gateway->>Gateway: 5. Validate nonce
    Gateway->>Gateway: 6. RBAC check
    Gateway->>Service: Forward request
    Service-->>Gateway: Response
    Gateway-->>Client: Response
```

### Security Configuration Examples

**Minimal (development):**
```json
{
  "auth": {
    "keys": {
      "dev-key": {
        "encrypted_secret": "...",
        "services": ["*"]
      }
    }
  }
}
```

**Standard (production):**
```json
{
  "auth": {
    "keys": {
      "ga-production": {
        "encrypted_secret": "...",
        "services": ["mongodb-production"],
        "ip_rules": {
          "enabled": true,
          "mode": "allowlist",
          "cidrs": ["10.0.0.0/8"]
        }
      }
    }
  }
}
```

**Maximum (high-security):**
```json
{
  "server": {
    "mtls": {
      "enabled": true,
      "ca_cert": "D:\\certs\\ca.crt",
      "require_client_cert": true
    }
  },
  "auth": {
    "keys": {
      "ga-production": {
        "encrypted_secret": "...",
        "services": ["mongodb-production"],
        "ip_rules": {
          "enabled": true,
          "mode": "allowlist",
          "cidrs": ["10.0.50.10/32"]
        },
        "client_cert": {
          "enabled": true,
          "allowed_fingerprints": ["SHA256:2CF24DBA..."]
        }
      }
    }
  }
}
```

### Key Struct

```go
type APIKey struct {
    Name            string   // Key identifier (for logging)
    Secret          string   // Decrypted secret (from encrypted_secret)
    AllowedServices []string // ["*"] or ["mongodb-production", ...]
    Description     string   // Human-readable description
    MaxConcurrent   int      // Max simultaneous requests (default: 10)
    RateLimitRPM    int      // Max requests per minute (default: 600)
    
    // Optional hardening
    IPRules         *IPRules         // IP allowlist/denylist
    ClientCert      *ClientCertRules // mTLS client cert binding
    
    // Runtime state
    inFlight        int64         // Current in-flight requests (atomic)
    rateLimiter     *RateLimiter  // Sliding window rate limiter
}

func (k *APIKey) CanAccessService(serviceName string) bool {
    for _, s := range k.AllowedServices {
        if s == "*" || s == serviceName {
            return true
        }
    }
    return false
}

func (k *APIKey) AcquireSlot() bool {
    current := atomic.AddInt64(&k.inFlight, 1)
    if current > int64(k.MaxConcurrent) {
        atomic.AddInt64(&k.inFlight, -1)
        return false
    }
    return true
}

func (k *APIKey) ReleaseSlot() {
    atomic.AddInt64(&k.inFlight, -1)
}
```

### Validation Middleware

```go
func AuthMiddleware(keys map[string]*APIKey, nonceStore *NonceStore, cfg *ServerConfig) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            // 1. Extract and validate Bearer token
            token := extractBearerToken(r)
            var matchedKey *APIKey
            for _, key := range keys {
                if subtle.ConstantTimeCompare([]byte(token), []byte(key.Secret)) == 1 {
                    matchedKey = key
                    break
                }
            }
            if matchedKey == nil {
                http.Error(w, "Unauthorized", http.StatusUnauthorized)
                return
            }
            
            // 2. IP allowlist/denylist check (if enabled)
            if matchedKey.IPRules != nil && matchedKey.IPRules.Enabled {
                clientIP := getRealIP(r, cfg.TrustProxy)
                if !matchedKey.IPRules.IsAllowed(clientIP) {
                    http.Error(w, "Forbidden: IP not allowed", http.StatusForbidden)
                    return
                }
            }
            
            // 3. mTLS client certificate check (if enabled)
            if matchedKey.ClientCert != nil && matchedKey.ClientCert.Enabled {
                if err := matchedKey.ClientCert.Validate(r.TLS.PeerCertificates); err != nil {
                    http.Error(w, "Forbidden: "+err.Error(), http.StatusForbidden)
                    return
                }
            }
            
            // 4. Rate limit check (applies to all endpoints including /auth/nonce)
            if !matchedKey.rateLimiter.Allow() {
                w.Header().Set("Retry-After", "60")
                http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
                return
            }
            
            // 5. Concurrency limit check
            if !matchedKey.AcquireSlot() {
                w.Header().Set("Retry-After", "5")
                http.Error(w, "Too many concurrent requests", http.StatusTooManyRequests)
                return
            }
            defer matchedKey.ReleaseSlot()
            
            // 6. Skip nonce + RBAC check for /auth/nonce endpoint
            if r.URL.Path == "/api/v1/auth/nonce" {
                next.ServeHTTP(w, r)
                return
            }
            
            // 7. Validate and consume nonce
            nonce := r.Header.Get("X-Nonce")
            if !nonceStore.Consume(nonce) {
                http.Error(w, "Invalid or expired nonce", http.StatusUnauthorized)
                return
            }
            
            // 8. RBAC check - extract service name from URL
            serviceName := extractServiceName(r.URL.Path) // e.g., "mongodb-production"
            if !matchedKey.CanAccessService(serviceName) {
                http.Error(w, "Forbidden: no access to this service", http.StatusForbidden)
                return
            }
            
            // 9. Add key info to context for logging
            ctx := context.WithValue(r.Context(), "api_key_name", matchedKey.Name)
            next.ServeHTTP(w, r.WithContext(ctx))
        })
    }
}
```

---

## Service Interface

All backend services implement a common interface:

```go
package services

import (
    "context"
    "net/http"
)

// Service defines the contract for all backend services.
type Service interface {
    // Name returns the service identifier used in URLs.
    Name() string
    
    // Type returns the service type (mongodb, sql, rclone, o365mbx, pdf-extractor, binary-wrapper).
    Type() string
    
    // Initialize is called during gateway startup with decrypted master key.
    Initialize(ctx context.Context, masterKey []byte) error
    
    // Reload is called on hot-reload to update config.
    Reload(ctx context.Context, cfg json.RawMessage) error
    
    // Handle processes an incoming request.
    // pathParts contains URL segments after /api/v1/{service}/
    Handle(ctx context.Context, w http.ResponseWriter, r *http.Request, pathParts []string)
    
    // HealthCheck returns nil if the service is healthy.
    HealthCheck(ctx context.Context) error
    
    // Shutdown gracefully stops the service.
    Shutdown(ctx context.Context) error
}

// ServiceFactory creates a new service instance from config.
// Each service type registers its factory function.
type ServiceFactory func(name string, rawConfig json.RawMessage) (Service, error)
```

### Service Type Registry

```go
// internal/services/types.go

package services

import (
    "encoding/json"
    "fmt"
    "sync"
)

// Global registry of service type factories
var (
    typeRegistry = make(map[string]ServiceFactory)
    typeMu       sync.RWMutex
)

// RegisterType registers a service type factory.
// Called by each service package's init() function.
func RegisterType(typeName string, factory ServiceFactory) {
    typeMu.Lock()
    defer typeMu.Unlock()
    if _, exists := typeRegistry[typeName]; exists {
        panic(fmt.Sprintf("service type %q already registered", typeName))
    }
    typeRegistry[typeName] = factory
}

// CreateService creates a service instance by type name.
func CreateService(typeName, name string, rawConfig json.RawMessage) (Service, error) {
    typeMu.RLock()
    factory, exists := typeRegistry[typeName]
    typeMu.RUnlock()
    
    if !exists {
        return nil, fmt.Errorf("unknown service type: %s", typeName)
    }
    
    return factory(name, rawConfig)
}

// RegisteredTypes returns all registered service type names.
func RegisteredTypes() []string {
    typeMu.RLock()
    defer typeMu.RUnlock()
    types := make([]string, 0, len(typeRegistry))
    for t := range typeRegistry {
        types = append(types, t)
    }
    return types
}
```

### Service Self-Registration Pattern

Each service type registers itself via `init()`:

```go
// internal/services/mongodb/handler.go

package mongodb

import (
    "encoding/json"
    
    "github.com/edsilegxrepo/service-gateway/internal/services"
)

func init() {
    services.RegisterType("mongodb", NewService)
}

func NewService(name string, rawConfig json.RawMessage) (services.Service, error) {
    var cfg Config
    if err := json.Unmarshal(rawConfig, &cfg); err != nil {
        return nil, fmt.Errorf("parse mongodb config: %w", err)
    }
    
    return &Service{
        name:   name,
        config: cfg,
    }, nil
}
```

```go
// internal/services/sql/handler.go

package sql

func init() {
    services.RegisterType("sql", NewService)
}
```

```go
// internal/services/rclone/handler.go

package rclone

func init() {
    services.RegisterType("rclone", NewService)
}
```

### Base Service (DRY)

Common functionality embedded by all services:

```go
// internal/services/base.go

package services

import (
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "sync"
    "time"
)

// BaseService provides common functionality for all services.
// Embed this in your service struct.
type BaseService struct {
    name      string
    svcType   string
    mu        sync.RWMutex
    startTime time.Time
}

func NewBaseService(name, svcType string) BaseService {
    return BaseService{
        name:      name,
        svcType:   svcType,
        startTime: time.Now(),
    }
}

func (b *BaseService) Name() string { return b.name }
func (b *BaseService) Type() string { return b.svcType }

// Lock/Unlock helpers for config access
func (b *BaseService) RLock()   { b.mu.RLock() }
func (b *BaseService) RUnlock() { b.mu.RUnlock() }
func (b *BaseService) Lock()    { b.mu.Lock() }
func (b *BaseService) Unlock()  { b.mu.Unlock() }

// WriteJSON writes a JSON response with proper headers
func (b *BaseService) WriteJSON(w http.ResponseWriter, status int, data interface{}) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(status)
    json.NewEncoder(w).Encode(data)
}

// WriteError writes a standard error response
func (b *BaseService) WriteError(w http.ResponseWriter, status int, code, message string) {
    b.WriteJSON(w, status, map[string]interface{}{
        "error": map[string]string{
            "code":    code,
            "message": message,
        },
    })
}

// ParseJSONBody parses request body into target struct
func (b *BaseService) ParseJSONBody(r *http.Request, target interface{}) error {
    if r.Body == nil {
        return fmt.Errorf("empty request body")
    }
    return json.NewDecoder(r.Body).Decode(target)
}

// WithTimeout creates a context with configured timeout
func (b *BaseService) WithTimeout(ctx context.Context, seconds int) (context.Context, context.CancelFunc) {
    return context.WithTimeout(ctx, time.Duration(seconds)*time.Second)
}

// StreamJSON streams NDJSON response
func (b *BaseService) StreamJSON(w http.ResponseWriter, items <-chan interface{}) error {
    w.Header().Set("Content-Type", "application/x-ndjson")
    w.Header().Set("Transfer-Encoding", "chunked")
    
    flusher, ok := w.(http.Flusher)
    if !ok {
        return fmt.Errorf("streaming not supported")
    }
    
    encoder := json.NewEncoder(w)
    for item := range items {
        if err := encoder.Encode(item); err != nil {
            return err
        }
        flusher.Flush()
    }
    return nil
}

// Uptime returns service uptime
func (b *BaseService) Uptime() time.Duration {
    return time.Since(b.startTime)
}
```

### Using BaseService

```go
// internal/services/myservice/handler.go

package myservice

type Service struct {
    services.BaseService  // Embed base service
    config Config
    client *MyClient
}

func NewService(name string, rawConfig json.RawMessage) (services.Service, error) {
    var cfg Config
    if err := json.Unmarshal(rawConfig, &cfg); err != nil {
        return nil, err
    }
    
    return &Service{
        BaseService: services.NewBaseService(name, "myservice"),
        config:      cfg,
    }, nil
}

func (s *Service) Handle(ctx context.Context, w http.ResponseWriter, r *http.Request, pathParts []string) {
    // Use base service helpers
    s.RLock()
    cfg := s.config
    s.RUnlock()
    
    var req MyRequest
    if err := s.ParseJSONBody(r, &req); err != nil {
        s.WriteError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
        return
    }
    
    ctx, cancel := s.WithTimeout(ctx, cfg.TimeoutSeconds)
    defer cancel()
    
    result, err := s.client.DoSomething(ctx, req)
    if err != nil {
        s.WriteError(w, http.StatusInternalServerError, "SERVICE_ERROR", err.Error())
        return
    }
    
    s.WriteJSON(w, http.StatusOK, result)
}
```

### Service Loader

```go
// internal/services/loader.go

package services

import (
    "context"
    "encoding/json"
    "fmt"
    "os"
    "path/filepath"
)

type ServiceConfig struct {
    Type string `json:"type"`
    // ... rest is passed to factory as raw JSON
}

// LoadServices loads all enabled services from the services directory.
func LoadServices(ctx context.Context, servicesDir string, enabledServices map[string]bool, masterKey []byte) (*Registry, error) {
    registry := NewRegistry()
    
    for name, enabled := range enabledServices {
        if !enabled {
            continue
        }
        
        configPath := filepath.Join(servicesDir, name, "config.json")
        rawConfig, err := os.ReadFile(configPath)
        if err != nil {
            return nil, fmt.Errorf("read config for %s: %w", name, err)
        }
        
        // Extract type from config
        var baseCfg ServiceConfig
        if err := json.Unmarshal(rawConfig, &baseCfg); err != nil {
            return nil, fmt.Errorf("parse config for %s: %w", name, err)
        }
        
        // Create service using registered factory
        svc, err := CreateService(baseCfg.Type, name, rawConfig)
        if err != nil {
            return nil, fmt.Errorf("create service %s: %w", name, err)
        }
        
        // Initialize with master key for secret decryption
        if err := svc.Initialize(ctx, masterKey); err != nil {
            return nil, fmt.Errorf("initialize service %s: %w", name, err)
        }
        
        registry.Register(svc)
    }
    
    return registry, nil
}
```

### Service Registry

```go
type Registry struct {
    services map[string]Service
    mu       sync.RWMutex
}

func (r *Registry) Register(svc Service) error {
    r.mu.Lock()
    defer r.mu.Unlock()
    if _, exists := r.services[svc.Name()]; exists {
        return fmt.Errorf("service %s already registered", svc.Name())
    }
    r.services[svc.Name()] = svc
    return nil
}

func (r *Registry) Get(name string) (Service, bool) {
    r.mu.RLock()
    defer r.mu.RUnlock()
    svc, ok := r.services[name]
    return svc, ok
}

func (r *Registry) ReloadAll(ctx context.Context, configs map[string]json.RawMessage) error {
    r.mu.RLock()
    defer r.mu.RUnlock()
    for name, svc := range r.services {
        if cfg, ok := configs[name]; ok {
            if err := svc.Reload(ctx, cfg); err != nil {
                return fmt.Errorf("reload %s: %w", name, err)
            }
        }
    }
    return nil
}
```

---

## Service: MongoDB

### Configuration (`services/mongodb-production/config.json`)

```json
{
  "type": "mongodb",
  "description": "Production database - orders and customers",
  "version": "1.3",
  "updated": "2024-01-15T10:00:00Z",
  "updated_by": "john.doe",
  
  "connection": {
    "encrypted_uri": "BASE64_AES_GCM_CIPHERTEXT",
    "database": "prod_db",
    "pool_size": 10,
    "timeout_seconds": 30
  },
  
  "limits": {
    "max_results": 1000,
    "default_limit": 100,
    "max_response_mb": 16,
    "allow_streaming": false
  },
  
  "collections": {
    "orders": {
      "enabled": true,
      "allow_adhoc": true,
      "adhoc_restrictions": {
        "required_filters": ["date"],
        "allowed_filters": ["date", "status", "customer_id", "region"],
        "denied_operators": ["$where", "$expr", "$function"],
        "denied_projections": ["internal_notes", "cost_price"]
      },
      "queries": {
        "recent-pending": {
          "description": "Pending orders from last 7 days",
          "filter": {"date": {"$gte": "{{today-7d}}"}, "status": "pending"},
          "sort": {"date": -1}
        },
        "by-customer": {
          "description": "Orders for a customer",
          "params": {
            "customer_id": {"type": "string", "required": true}
          },
          "filter": {"customer_id": "{{customer_id}}"}
        }
      }
    },
    "customers": {
      "enabled": true,
      "allow_adhoc": false,
      "queries": {
        "by-id": {
          "params": {"id": {"type": "objectid", "required": true}},
          "filter": {"_id": "{{id}}"}
        }
      }
    }
  }
}
```

### Connection Pool & Limits

| Setting | Default | Description |
|---------|---------|-------------|
| `pool_size` | 10 | Max concurrent MongoDB connections |
| `max_results` | 1000 | Hard cap on documents returned |
| `default_limit` | 100 | Default if client doesn't specify `?limit=` |
| `timeout_seconds` | 30 | Per-query timeout |
| `max_response_mb` | 16 | Max response body size before error |
| `allow_streaming` | false | Enable NDJSON streaming mode |

### Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/{service}/` | List enabled collections |
| GET | `/api/v1/{service}/{collection}` | Query with named query or ad-hoc filter |
| GET | `/api/v1/{service}/{collection}/{id}` | Get document by `_id` |

### Query Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `query` | string | Named query to execute |
| `filter` | JSON | Ad-hoc filter (if allowed) |
| `projection` | JSON | Fields to include/exclude |
| `sort` | JSON | Sort specification |
| `limit` | int | Max results (capped by `max_results`) |
| `skip` | int | Pagination offset |
| `stream` | bool | Enable NDJSON streaming (if allowed) |
| `{param}` | varies | Parameters for named queries |

### Example Requests

```bash
# List collections
curl -H "Authorization: Bearer $KEY" -H "X-Nonce: $NONCE" \
  http://127.0.0.1:3000/api/v1/mongodb-production/

# Named query
curl -H "Authorization: Bearer $KEY" -H "X-Nonce: $NONCE" \
  'http://127.0.0.1:3000/api/v1/mongodb-production/orders?query=recent-pending'

# Named query with parameters
curl -H "Authorization: Bearer $KEY" -H "X-Nonce: $NONCE" \
  'http://127.0.0.1:3000/api/v1/mongodb-production/orders?query=by-customer&customer_id=C12345'

# Ad-hoc query (where allowed)
curl -H "Authorization: Bearer $KEY" -H "X-Nonce: $NONCE" \
  'http://127.0.0.1:3000/api/v1/mongodb-production/orders?filter={"date":{"$gte":"2024-01-01"}}&limit=100'

# Get specific document
curl -H "Authorization: Bearer $KEY" -H "X-Nonce: $NONCE" \
  http://127.0.0.1:3000/api/v1/mongodb-production/orders/65a1b2c3d4e5f6789012345
```

### Security Restrictions

1. **Collection whitelist** — Only `enabled: true` collections accessible
2. **Query restrictions** — Named queries only, or restricted ad-hoc
3. **Filter validation** — Required fields, allowed fields, denied operators
4. **Projection control** — Can deny sensitive fields
5. **Read-only** — Only GET methods allowed; no insert/update/delete
6. **Result limits** — `max_results` caps query results
7. **Timeout** — Per-query timeout prevents long-running queries
8. **RBAC** — API key must have access to the specific service name

---

## Performance & Streaming

**These requirements apply to ALL services returning data.**

### Concurrency Model

Go's `net/http` handles each request in its own goroutine — no additional threading needed for request handling. Each service type manages its own connection/resource pool.

```mermaid
flowchart TB
    subgraph Process[Gateway Process]
        subgraph Goroutines[Request Goroutines]
            G1[Request 1]
            G2[Request 2]
            G3[Request 3]
            G4[...]
        end
        
        subgraph Pools[Connection Pools]
            MongoPool[(MongoDB<br/>pool_size: 10)]
            SQLPool[(SQL<br/>max_open: 10)]
            RclonePool[(Rclone<br/>transfers: 10)]
        end
        
        G1 --> MongoPool
        G2 --> SQLPool
        G3 --> RclonePool
    end
```

### Service-Specific Pooling

| Service Type | Pool Mechanism | Config |
|--------------|----------------|--------|
| `mongodb` | MongoDB driver connection pool | `pool_size` |
| `sql` | `database/sql` connection pool | `max_open_conns`, `max_idle_conns` |
| `rclone` | librclone parallel transfers | `transfers`, `checkers` |
| `o365mbx` | HTTP client + rate limiter | `parallel`, `api_rate`, `api_burst` |
| `pdf-extractor` | Worker goroutine pool | `max_workers` |

### Universal Response Contract

All services returning data MUST support these response modes:

| Mode | Header | Use Case | Memory |
|------|--------|----------|--------|
| **Paginated JSON** | `Content-Type: application/json` | Default, Integration Platform compatible | Buffered |
| **NDJSON Streaming** | `Content-Type: application/x-ndjson` | Large exports | Minimal |
| **Chunked Transfer** | `Transfer-Encoding: chunked` | Real-time data | Minimal |

### Efficiency Requirements

Every service handler MUST implement:

```go
// 1. Context with timeout — prevent runaway operations
ctx, cancel := context.WithTimeout(r.Context(), time.Duration(cfg.TimeoutSeconds)*time.Second)
defer cancel()

// 2. Result limiting — cap memory usage
results := make([]T, 0, cfg.Limits.MaxResults)

// 3. Streaming for large results — don't buffer everything
if r.URL.Query().Get("stream") == "true" && cfg.AllowStreaming {
    w.Header().Set("Content-Type", "application/x-ndjson")
    w.Header().Set("Transfer-Encoding", "chunked")
    encoder := json.NewEncoder(w)
    for item := range cursor {
        encoder.Encode(item)
        w.(http.Flusher).Flush()
    }
    return
}

// 4. Early termination — respect context cancellation
select {
case <-ctx.Done():
    return ctx.Err()
default:
    // continue processing
}
```

### Chunked Data Transfer

For large file operations (rclone, o365mbx attachments):

```go
// Stream file download without buffering entire file
func (s *Service) streamFile(ctx context.Context, w http.ResponseWriter, reader io.Reader, size int64) {
    w.Header().Set("Content-Type", "application/octet-stream")
    w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
    
    // Use io.Copy with chunked buffer — 32KB chunks
    buf := make([]byte, 32*1024)
    io.CopyBuffer(w, reader, buf)
}

// For unknown size, use chunked transfer
func (s *Service) streamUnknownSize(ctx context.Context, w http.ResponseWriter, reader io.Reader) {
    w.Header().Set("Transfer-Encoding", "chunked")
    
    buf := make([]byte, 32*1024)
    for {
        select {
        case <-ctx.Done():
            return
        default:
            n, err := reader.Read(buf)
            if n > 0 {
                w.Write(buf[:n])
                w.(http.Flusher).Flush()
            }
            if err == io.EOF {
                return
            }
        }
    }
}
```

### Memory Protection (All Services)

| Protection | Config Key | Default | Description |
|------------|------------|---------|-------------|
| **Response size cap** | `max_response_mb` | 16 | Error if buffered response exceeds |
| **Result count cap** | `max_results` | 1000 | Limit rows/documents returned |
| **Query timeout** | `timeout_seconds` | 30 | Cancel and return error |
| **Connection pool** | `pool_size` / `max_open_conns` | 10 | Queue when exhausted |
| **Streaming threshold** | `stream_threshold_mb` | 1 | Auto-stream if response > threshold |

### Parallel Processing Pattern

For services processing multiple items (pdf-extractor, o365mbx):

```go
func (s *Service) processItems(ctx context.Context, items []Item) ([]Result, error) {
    results := make([]Result, len(items))
    errCh := make(chan error, 1)
    
    // Semaphore for max concurrency
    sem := make(chan struct{}, s.config.MaxWorkers)
    var wg sync.WaitGroup
    
    for i, item := range items {
        select {
        case <-ctx.Done():
            return nil, ctx.Err()
        case sem <- struct{}{}:
            wg.Add(1)
            go func(idx int, it Item) {
                defer wg.Done()
                defer func() { <-sem }()
                
                result, err := s.processOne(ctx, it)
                if err != nil {
                    select {
                    case errCh <- err:
                    default:
                    }
                    return
                }
                results[idx] = result
            }(i, item)
        }
    }
    
    wg.Wait()
    
    select {
    case err := <-errCh:
        return results, err
    default:
        return results, nil
    }
}
```

### Response Modes

#### Mode 1: Paginated JSON (Default)

Standard JSON array response with pagination. Works with all REST clients including Integration Platform.

```bash
# First page
GET /api/v1/mongodb-production/orders?limit=100&skip=0

# Response
{
  "data": [...100 documents...],
  "meta": {
    "count": 100,
    "skip": 0,
    "limit": 100,
    "has_more": true
  }
}

# Next page
GET /api/v1/mongodb-production/orders?limit=100&skip=100
```

**Behavior:**
- Default `limit` from config (`default_limit: 100`)
- Capped at `max_results` (e.g., 1000)
- Response buffered in memory before sending
- Protected by `max_response_mb` limit

#### Mode 2: NDJSON Streaming (Opt-in)

Newline-delimited JSON, streamed as cursor iterates. For large exports with clients that support streaming.

```bash
# Request streaming mode
GET /api/v1/mongodb-analytics/reports?stream=true&limit=10000

# Response (Transfer-Encoding: chunked)
{"_id": "1", "name": "Report A", ...}
{"_id": "2", "name": "Report B", ...}
{"_id": "3", "name": "Report C", ...}
...
```

**Behavior:**
- Only available if `allow_streaming: true` in service config
- `Content-Type: application/x-ndjson`
- `Transfer-Encoding: chunked`
- Documents sent as cursor returns them — minimal memory
- No `max_response_mb` limit (streaming)
- Client reads line by line

**Client support:**
- curl: `curl ... | while read line; do echo "$line" | jq .; done`
- Python: `for line in response.iter_lines(): ...`
- Integration Platform: May not support — use pagination instead

#### Mode 3: Single Document

No pagination needed for single document retrieval.

```bash
GET /api/v1/mongodb-production/orders/65a1b2c3d4e5f6789012345

# Response
{
  "_id": "65a1b2c3d4e5f6789012345",
  "customer": "Acme Corp",
  ...
}
```

### Memory Protection

| Protection | Setting | Behavior |
|------------|---------|----------|
| **Response size cap** | `max_response_mb: 16` | Error if buffered response exceeds limit |
| **Result count cap** | `max_results: 1000` | Query limited regardless of client `?limit=` |
| **Query timeout** | `timeout_seconds: 30` | Cancel query and return error |
| **Connection pool** | `pool_size: 10` | Queue requests when pool exhausted |

### Error Responses

```json
// Response too large
{
  "error": {
    "code": "RESPONSE_TOO_LARGE",
    "message": "Response exceeds 16MB limit",
    "details": "Use pagination with smaller limit or enable streaming"
  }
}

// Query timeout
{
  "error": {
    "code": "TIMEOUT",
    "message": "Query timed out after 30 seconds"
  }
}
```

---

## Service Design Patterns

When adding a new service type, choose the appropriate access pattern based on complexity:

### Pattern 1: One Service Per Task (Simple)

**Use when:** Service has few fixed parameters, complexity is low.

**Example:** `pdf-extractor` — extracts text matching a delimiter from PDFs. Workspace, search delimiter, and pattern are fixed per service instance.

```
services/
├── pdf-extractor-invoices/    # workspace=invoices, search="Invoice Number:"
├── pdf-extractor-po/          # workspace=invoices, search="PO Number:"
└── pdf-extractor-edi/         # workspace=edi, search="EDI-REF:"
```

Each service is configured for a specific extraction task — the delimiter and workspace are examples, not hardcoded.

**API:** Single endpoint, optional parameter overrides.
```http
POST /api/v1/pdf-extractor-invoices/extract
{"pattern": "*.pdf"}
```

**RBAC:** Service-level access — key has access to specific service or not.

### Pattern 2: Named Operations + Restricted Raw (Complex)

**Use when:** Service has many parameter combinations, shared credentials/connections, or security-sensitive operations.

**Example:** `mongodb`, `rclone` — multiple queries/operations share the same connection, need fine-grained control.

```json
{
  "operations": {
    "sftp-to-gcs-live": { "src_remote": "...", "dst_remote": "..." },
    "sftp-to-gcs-dev": { "src_remote": "...", "dst_remote": "..." }
  },
  "raw_access": {
    "enabled": true,
    "allowed_remotes": ["sftp-internal", "gcs-archive"]
  }
}
```

**API:** Named operations + optional raw access.
```http
POST /api/v1/rclone-gcs-archive/op/sftp-to-gcs-live    # Named
POST /api/v1/rclone-gcs-archive/move                    # Raw (admin only)
```

**RBAC:** Service + access level — `operations_only` vs `raw`.

### Pattern Selection Guide

| Criteria | Pattern 1 (Simple) | Pattern 2 (Operations) |
|----------|-------------------|------------------------|
| Parameters | 1-3 fixed | Many combinations |
| Credentials | Per-service | Shared across operations |
| Security risk | Low | High (credentials, network) |
| Examples | pdf-extractor | mongodb, rclone, o365mbx |

### Adding a New Service Type

Follow this checklist to add a new service type (e.g., `myservice`):

#### Step 1: Create Service Package

```
internal/services/myservice/
├── handler.go      # Main service implementation
├── config.go       # Config types
└── handler_test.go # Unit tests
```

#### Step 2: Define Config Schema

```go
// internal/services/myservice/config.go

package myservice

type Config struct {
    // Connection/credentials (encrypted)
    EncryptedSecret string `json:"encrypted_secret"`
    
    // Service-specific settings
    Timeout    int    `json:"timeout_seconds"`
    MaxWorkers int    `json:"max_workers"`
    
    // Operations (if using Pattern 2)
    Operations map[string]Operation `json:"operations,omitempty"`
    
    // Raw access (if applicable)
    RawAccess RawAccessConfig `json:"raw_access,omitempty"`
}

type Operation struct {
    Description string              `json:"description"`
    Params      map[string]ParamDef `json:"params"`
    // ... operation-specific fields
}

type RawAccessConfig struct {
    Enabled bool     `json:"enabled"`
    Allowed []string `json:"allowed"`
}
```

#### Step 3: Implement Service Interface

```go
// internal/services/myservice/handler.go

package myservice

import (
    "context"
    "encoding/json"
    "net/http"
    "sync"
    
    "github.com/edsilegxrepo/service-gateway/internal/services"
    "github.com/edsilegxrepo/secretprotector/pkg/libsecsecrets"
)

// Register this service type on package init
func init() {
    services.RegisterType("myservice", NewService)
}

// Service implements services.Service
type Service struct {
    name   string
    config Config
    mu     sync.RWMutex
    
    // Backend client/connection
    client *MyClient
}

// NewService is the factory function (called by RegisterType)
func NewService(name string, rawConfig json.RawMessage) (services.Service, error) {
    var cfg Config
    if err := json.Unmarshal(rawConfig, &cfg); err != nil {
        return nil, fmt.Errorf("parse config: %w", err)
    }
    
    return &Service{
        name:   name,
        config: cfg,
    }, nil
}

func (s *Service) Name() string { return s.name }
func (s *Service) Type() string { return "myservice" }

func (s *Service) Initialize(ctx context.Context, masterKey []byte) error {
    // Decrypt secrets
    secret, err := libsecsecrets.Decrypt(ctx, s.config.EncryptedSecret, masterKey)
    if err != nil {
        return fmt.Errorf("decrypt secret: %w", err)
    }
    
    // Initialize backend client
    s.client, err = NewMyClient(secret, s.config)
    if err != nil {
        return fmt.Errorf("init client: %w", err)
    }
    
    return nil
}

func (s *Service) Handle(ctx context.Context, w http.ResponseWriter, r *http.Request, pathParts []string) {
    s.mu.RLock()
    cfg := s.config
    s.mu.RUnlock()
    
    // Route to appropriate handler
    if len(pathParts) > 0 && pathParts[0] == "op" && len(pathParts) > 1 {
        s.handleOperation(ctx, w, r, pathParts[1])
        return
    }
    
    // Handle other endpoints...
}

func (s *Service) handleOperation(ctx context.Context, w http.ResponseWriter, r *http.Request, opName string) {
    // Named operation handling
}

func (s *Service) Reload(ctx context.Context, rawConfig json.RawMessage) error {
    var newCfg Config
    if err := json.Unmarshal(rawConfig, &newCfg); err != nil {
        return err
    }
    
    s.mu.Lock()
    s.config = newCfg
    s.mu.Unlock()
    
    return nil
}

func (s *Service) HealthCheck(ctx context.Context) error {
    return s.client.Ping(ctx)
}

func (s *Service) Shutdown(ctx context.Context) error {
    if s.client != nil {
        return s.client.Close()
    }
    return nil
}
```

#### Step 4: Import Package in Main

```go
// cmd/gateway/main.go

import (
    // ... other imports
    
    // Service type registrations (blank imports trigger init())
    _ "github.com/edsilegxrepo/service-gateway/internal/services/mongodb"
    _ "github.com/edsilegxrepo/service-gateway/internal/services/sql"
    _ "github.com/edsilegxrepo/service-gateway/internal/services/rclone"
    _ "github.com/edsilegxrepo/service-gateway/internal/services/o365mbx"
    _ "github.com/edsilegxrepo/service-gateway/internal/services/pdfextractor"
    _ "github.com/edsilegxrepo/service-gateway/internal/services/wrapper"
    _ "github.com/edsilegxrepo/service-gateway/internal/services/myservice"  // NEW
)
```

#### Step 5: Create Service Config

```json
// services/myservice-production/config.json
{
  "type": "myservice",
  "description": "My new service",
  "version": "1.0",
  
  "encrypted_secret": "AES256-GCM:base64...",
  "timeout_seconds": 30,
  "max_workers": 4,
  
  "operations": {
    "do-something": {
      "description": "Does something",
      "params": {
        "input": {"type": "string", "required": true}
      }
    }
  }
}
```

#### Step 6: Enable in Global Config

```json
// config.json
{
  "services": {
    "myservice-production": {"enabled": true}
  }
}
```

#### Step 7: Write Tests

```go
// internal/services/myservice/handler_test.go

package myservice

import (
    "testing"
)

func TestNewService(t *testing.T) {
    // Test factory
}

func TestService_Handle(t *testing.T) {
    // Test request handling
}

func TestService_HandleOperation(t *testing.T) {
    // Test named operations
}
```

#### Step 8: Document

Add a section to DESIGN.md under "Adding Services: MyService Integration".

### Service Extension Checklist

- [ ] Package created under `internal/services/{type}/`
- [ ] Config types defined
- [ ] Service interface implemented
- [ ] `init()` calls `services.RegisterType()`
- [ ] Blank import added to `cmd/gateway/main.go`
- [ ] Unit tests written (80% coverage)
- [ ] Integration tests written
- [ ] DESIGN.md section added
- [ ] Example config.json created

---

## Integrating External Go Code

### Integration Patterns

| Pattern | When to Use | Code Duplication | Performance |
|---------|-------------|------------------|-------------|
| **Go Module Import** | You control the repo, can expose library API | None | Best |
| **Binary Wrapper** | CLI-only tool, cannot modify | None | Process overhead |
| **Git Submodule** | Need local copy, can expose library API | None | Best |
| **Copy/Vendor** | Small utility, unlikely to change | Yes | Best |

### Recommended: Go Module Import

This requires the external repository to expose a **library API** separate from its CLI.

#### Required Repository Structure

External repos should follow this structure to be importable:

```
pdf-extractor/
├── cmd/
│   └── pdf-extractor/
│       └── main.go              # CLI entry point - calls pkg/
├── pkg/
│   └── extractor/
│       ├── extractor.go         # Public API: Extract(), Options{}
│       ├── extractor_test.go
│       └── types.go             # Public types: Result{}, Error{}
├── internal/                    # Private implementation details
│   └── mutool/
│       └── wrapper.go
├── go.mod
└── README.md
```

**Key principles:**

1. **`cmd/`** - CLI entry point only, minimal code
2. **`pkg/`** - Public library API, importable by other projects
3. **`internal/`** - Private code, not importable outside the module

#### Library API Design

The `pkg/` package must expose a clean, importable API:

```go
// pkg/extractor/extractor.go
package extractor

import "context"

// Options configures the extraction.
type Options struct {
    Path       string   // Workspace directory
    Pattern    string   // File glob pattern
    Search     string   // Delimiter to find
    MutoolBin  string   // Path to mutool binary
    Workers    int      // Concurrent workers (default: NumCPU)
    Timeout    time.Duration
}

// Result represents extraction output for one file.
type Result struct {
    Filename string   `json:"filename"`
    Values   []string `json:"values,omitempty"`
    Error    string   `json:"error,omitempty"`
}

// Extract processes PDFs matching the pattern and extracts values.
func Extract(ctx context.Context, opts Options) ([]Result, error) {
    // Implementation calls internal/ packages
}
```

#### CLI Calls Library

The CLI becomes a thin wrapper:

```go
// cmd/pdf-extractor/main.go
package main

import (
    "flag"
    "github.com/edsilegxrepo/pdf-extractor/pkg/extractor"
)

func main() {
    // Parse flags into Options
    opts := extractor.Options{
        Path:    *pathFlag,
        Pattern: *patternFlag,
        // ...
    }
    
    results, err := extractor.Extract(context.Background(), opts)
    // Output results
}
```

#### Gateway Imports Library

```go
// go.mod
module github.com/edsilegxrepo/service-gateway

require github.com/edsilegxrepo/pdf-extractor v1.0.0

// For local development (repos side-by-side):
replace github.com/edsilegxrepo/pdf-extractor => ../pdf-extractor
```

```go
// internal/services/pdfextractor/handler.go
package pdfextractor

import (
    "github.com/edsilegxrepo/pdf-extractor/pkg/extractor"
)

func (s *Service) Handle(ctx context.Context, w http.ResponseWriter, r *http.Request, pathParts []string) {
    // Parse request
    var req ExtractRequest
    json.NewDecoder(r.Body).Decode(&req)
    
    // Call library directly - no process spawn
    results, err := extractor.Extract(ctx, extractor.Options{
        Path:      req.Path,
        Pattern:   req.Pattern,
        Search:    req.Search,
        MutoolBin: s.config.MutoolBin,
        Workers:   s.config.MaxWorkers,
        Timeout:   time.Duration(s.config.TimeoutSeconds) * time.Second,
    })
    
    // Return results
    json.NewEncoder(w).Encode(results)
}
```

### Migration Path for Existing CLI-Only Repos

If an external repo is currently CLI-only (all code in `main.go`):

1. **Create `pkg/` directory** with the core logic
2. **Move types and functions** from `main.go` to `pkg/`
3. **Keep `cmd/`** as thin CLI wrapper
4. **Ensure `pkg/` has no dependencies on `cmd/`**
5. **Add tests** to `pkg/` to verify the API works standalone

Example refactoring:

```
Before:
  main.go (500 lines - everything)

After:
  cmd/tool/main.go (50 lines - flag parsing, calls pkg/)
  pkg/tool/api.go (200 lines - public API)
  pkg/tool/types.go (50 lines - public types)
  internal/impl/core.go (200 lines - private implementation)
```

### When to Use Binary Wrapper Instead

Use binary wrapper if:

- External repo you cannot modify
- Complex CLI with many dependencies you don't want to import
- Tool with unstable API that changes frequently
- Quick integration without refactoring

```json
{
  "type": "binary-wrapper",
  "executable": "D:\\tools\\external-tool.exe",
  "timeout_seconds": 60,
  "allowed_args": ["--input", "--output", "--format"]
}
```

---

## Adding Services: Go Library Integration

### Example: PDF Extractor

The [pdf-extractor](https://github.com/edsilegxrepo/pdf-extractor) extracts delimiter-based text values from PDFs. It must expose a `pkg/extractor` library API (see above).

Unlike MongoDB/rclone, pdf-extractor is simple — just 3 fixed params (workspace, pattern, search). Instead of operations within a service, create **one service per use case**:

```
services/
├── pdf-extractor-invoices/config.json     # Extract invoice numbers
├── pdf-extractor-po/config.json           # Extract PO numbers  
├── pdf-extractor-edi/config.json          # Extract EDI references
```

RBAC controls access — API keys grant access to specific services.

#### Service Configs

**`services/pdf-extractor-invoices/config.json`**
```json
{
  "type": "pdf-extractor",
  "description": "Extract invoice numbers from incoming PDFs",
  "version": "1.0",
  "updated": "2026-07-22T10:00:00Z",
  
  "settings": {
    "workspace": "D:\\docs\\invoices\\incoming",
    "search": "Invoice Number:",
    "pattern": "*.pdf",
    "mutool_bin": "D:\\tools\\mupdf\\mutool.exe",
    "timeout_seconds": 30,
    "max_workers": 4,
    "allow_pattern_override": true
  }
}
```

**`services/pdf-extractor-po/config.json`**
```json
{
  "type": "pdf-extractor",
  "description": "Extract PO numbers from incoming PDFs",
  "version": "1.0",
  
  "settings": {
    "workspace": "D:\\docs\\invoices\\incoming",
    "search": "PO Number:",
    "pattern": "*.pdf",
    "mutool_bin": "D:\\tools\\mupdf\\mutool.exe",
    "timeout_seconds": 30,
    "max_workers": 4,
    "allow_pattern_override": true
  }
}
```

**`services/pdf-extractor-edi/config.json`**
```json
{
  "type": "pdf-extractor",
  "description": "Extract EDI references",
  "version": "1.0",
  
  "settings": {
    "workspace": "D:\\docs\\edi",
    "search": "EDI-REF:",
    "pattern": "*.pdf",
    "mutool_bin": "D:\\tools\\mupdf\\mutool.exe",
    "timeout_seconds": 30,
    "max_workers": 4,
    "allow_pattern_override": true
  }
}
```

#### API Usage

```http
# Extract all invoice numbers (uses default pattern)
POST /api/v1/pdf-extractor-invoices/extract
Authorization: Bearer <token>
X-Nonce: <server-nonce>

{}
```

```http
# Extract with pattern override (if allow_pattern_override: true)
POST /api/v1/pdf-extractor-invoices/extract
Authorization: Bearer <token>
X-Nonce: <server-nonce>

{
  "pattern": "2026-07-*.pdf"
}
```

```http
# Extract single file
POST /api/v1/pdf-extractor-edi/extract
Authorization: Bearer <token>
X-Nonce: <server-nonce>

{
  "pattern": "edi_shipment_20260722.pdf"
}
```

#### Response

```json
{
  "results": [
    {"filename": "invoice_001.pdf", "values": ["INV-2026-0001"]},
    {"filename": "invoice_002.pdf", "values": ["INV-2026-0002", "INV-2026-0003"]},
    {"filename": "invoice_003.pdf", "values": [], "error": "no match found"}
  ],
  "processed": 3,
  "matched": 2,
  "errors": 1
}
```

#### Handler Implementation

```go
// internal/services/pdfextractor/handler.go
package pdfextractor

import (
    "context"
    "encoding/json"
    "net/http"
    "sync"
    "time"
    
    "github.com/edsilegxrepo/pdf-extractor/pkg/extractor"
)

type Service struct {
    name   string
    config Config
    mu     sync.RWMutex
}

type Config struct {
    Workspace            string `json:"workspace"`
    Search               string `json:"search"`
    Pattern              string `json:"pattern"`
    MutoolBin            string `json:"mutool_bin"`
    TimeoutSeconds       int    `json:"timeout_seconds"`
    MaxWorkers           int    `json:"max_workers"`
    AllowPatternOverride bool   `json:"allow_pattern_override"`
}

type ExtractRequest struct {
    Pattern string `json:"pattern,omitempty"`
}

func (s *Service) Handle(ctx context.Context, w http.ResponseWriter, r *http.Request, pathParts []string) {
    if len(pathParts) == 0 || pathParts[0] != "extract" {
        http.Error(w, "not found", http.StatusNotFound)
        return
    }
    
    var req ExtractRequest
    json.NewDecoder(r.Body).Decode(&req)
    
    s.mu.RLock()
    cfg := s.config
    s.mu.RUnlock()
    
    // Determine pattern
    pattern := cfg.Pattern
    if req.Pattern != "" && cfg.AllowPatternOverride {
        pattern = req.Pattern
    }
    
    // Build options from service config
    opts := extractor.Options{
        Path:      cfg.Workspace,
        Pattern:   pattern,
        Search:    cfg.Search,
        MutoolBin: cfg.MutoolBin,
        Workers:   cfg.MaxWorkers,
        Timeout:   time.Duration(cfg.TimeoutSeconds) * time.Second,
    }
    
    results, err := extractor.Extract(ctx, opts)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    
    json.NewEncoder(w).Encode(results)
}

func (s *Service) Reload(ctx context.Context, cfg json.RawMessage) error {
    var newCfg Config
    if err := json.Unmarshal(cfg, &newCfg); err != nil {
        return err
    }
    s.mu.Lock()
    s.config = newCfg
    s.mu.Unlock()
    return nil
}
```

#### Security Model

| Control | Mechanism |
|---------|-----------|
| **Workspace locked** | Defined in service config, not exposed to caller |
| **Search delimiter locked** | Defined in service config, not exposed to caller |
| **Pattern override** | Optional via `allow_pattern_override` |
| **Access control** | RBAC — API key must have service in `allowed_services` |
| **Path traversal** | Pattern validated — no `..` or absolute paths |

**Simple, locked-down, one service = one extraction task.**

---

## Adding Services: Binary Wrapper

For existing executables that don't have Go libraries.

### Service Config (`services/legacy-report-daily/config.json`)

```json
{
  "type": "binary-wrapper",
  "description": "Daily report generator",
  "version": "1.0",
  
  "settings": {
    "executable": "D:\\tools\\legacy-report.exe",
    "working_dir": "D:\\data\\reports\\daily",
    "timeout_seconds": 60,
    "allowed_args": ["--format", "--output", "--date-range"],
    "env": {
      "REPORT_CONFIG": "D:\\config\\report.ini"
    }
  }
}
```

### API Usage

```http
# Execute with allowed arguments
POST /api/v1/legacy-report-daily/exec
Authorization: Bearer <token>
X-Nonce: <server-nonce>

{
  "args": ["--format", "csv", "--output", "report.csv", "--date-range", "2026-07-01:2026-07-22"]
}
```

Response:
```json
{
  "exit_code": 0,
  "stdout": "Report generated: 150 records\n",
  "stderr": "",
  "duration_ms": 2340
}
```

```http
# Blocked — argument not in allowed_args
POST /api/v1/legacy-report-daily/exec
{
  "args": ["--format", "csv", "--delete-all"]
}
# Returns 400 Bad Request: argument "--delete-all" not allowed
```

### Security Considerations

1. **Argument whitelist** — Only explicitly allowed flags can be passed
2. **Path validation** — Executable path must be absolute
3. **Working directory isolation** — Each wrapper has its own working dir
4. **Timeout enforcement** — Prevents runaway processes
5. **No shell execution** — Uses `exec.Command`, not shell interpretation

---

## Adding Services: Message Queue (MQ)

### Overview

The [Watermill](https://github.com/ThreeDotsLabs/watermill) library provides a unified interface for message queue operations across multiple backends: Kafka, RabbitMQ, NATS, AWS SQS/SNS, Google Pub/Sub, and Redis Streams.

### Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                      service-gateway                        │
├─────────────────────────────────────────────────────────────┤
│  POST /api/v1/mq-{name}/publish                             │
│  POST /api/v1/mq-{name}/consume                             │
│  GET  /api/v1/mq-{name}/topics                              │
│  GET  /api/v1/mq-{name}/health                              │
├─────────────────────────────────────────────────────────────┤
│                   Watermill (abstraction)                   │
│                                                             │
│   ┌─────────┐  ┌─────────┐  ┌─────────┐  ┌─────────┐       │
│   │  Kafka  │  │RabbitMQ │  │  NATS   │  │   SQS   │  ...  │
│   └────┬────┘  └────┬────┘  └────┬────┘  └────┬────┘       │
└────────┼────────────┼───────────┼───────────┼──────────────┘
         │            │           │           │
         ▼            ▼           ▼           ▼
    Message Brokers (configured per service instance)
```

### Supported Backends

| Backend | Library | Connection String Format |
|---------|---------|--------------------------|
| Kafka | `watermill-kafka` | `kafka://host1:9092,host2:9092?tls=true` |
| RabbitMQ | `watermill-amqp` | `amqp://user:pass@host:5672/vhost` |
| NATS | `watermill-nats` | `nats://host:4222` |
| NATS JetStream | `watermill-nats` | `nats://host:4222?jetstream=true` |
| AWS SQS | `watermill-aws` | `sqs://region/queue-name?aws_access_key=...` |
| Google Pub/Sub | `watermill-googlecloud` | `pubsub://project-id` |
| Redis Streams | `watermill-redisstream` | `redis://host:6379/0` |

### Service Config (`services/mq-kafka-orders/config.json`)

```json
{
  "type": "mq",
  "description": "Order processing via Kafka",
  "version": "1.0",
  "updated": "2026-07-22T10:00:00Z",
  
  "settings": {
    "backend": "kafka",
    "encrypted_connection": "AES256-GCM:base64...",
    
    "options": {
      "consumer_group": "service-gateway",
      "auto_offset_reset": "latest",
      "enable_tls": true,
      "tls_ca_cert": "D:\\certs\\kafka-ca.pem",
      "tls_client_cert": "D:\\certs\\kafka-client.pem",
      "tls_client_key": "D:\\certs\\kafka-client-key.pem"
    },
    
    "operations": {
      "publish-order": {
        "description": "Publish new order to processing queue",
        "action": "publish",
        "topic": "orders.new",
        "params": {
          "order_id": {"type": "string", "required": true},
          "payload": {"type": "object", "required": true}
        }
      },
      "consume-results": {
        "description": "Consume processed order results",
        "action": "consume",
        "topic": "orders.processed",
        "params": {
          "max_messages": {"type": "int", "required": false, "default": 10},
          "timeout_seconds": {"type": "int", "required": false, "default": 30}
        }
      },
      "publish-audit": {
        "description": "Publish audit event",
        "action": "publish",
        "topic": "audit.events",
        "params": {
          "event_type": {"type": "string", "required": true},
          "payload": {"type": "object", "required": true}
        }
      }
    },
    
    "allowed_topics": ["orders.new", "orders.processed", "audit.events"]
  }
}
```

### RabbitMQ Example (`services/mq-rabbitmq-events/config.json`)

```json
{
  "type": "mq",
  "description": "Event notifications via RabbitMQ",
  "version": "1.0",
  
  "settings": {
    "backend": "rabbitmq",
    "encrypted_connection": "AES256-GCM:base64...",
    
    "options": {
      "exchange": "events",
      "exchange_type": "topic",
      "durable": true,
      "prefetch_count": 10
    },
    
    "operations": {
      "publish-notification": {
        "description": "Publish user notification",
        "action": "publish",
        "routing_key": "notifications.user",
        "params": {
          "user_id": {"type": "string", "required": true},
          "message": {"type": "string", "required": true}
        }
      },
      "consume-failures": {
        "description": "Consume failed job notifications",
        "action": "consume",
        "queue": "jobs.failed",
        "params": {
          "max_messages": {"type": "int", "default": 50},
          "ack_mode": {"type": "string", "default": "auto", "enum": ["auto", "manual"]}
        }
      }
    }
  }
}
```

### API Usage

```http
# Publish message to Kafka topic
POST /api/v1/mq-kafka-orders/op/publish-order
Authorization: Bearer <token>
X-Nonce: <server-nonce>

{
  "order_id": "ORD-2026-00001",
  "payload": {
    "customer": "ACME Corp",
    "items": [{"sku": "WIDGET-01", "qty": 100}],
    "total": 5000.00
  }
}
```

Response:
```json
{
  "status": "published",
  "topic": "orders.new",
  "partition": 3,
  "offset": 12847,
  "timestamp": "2026-07-22T10:30:00Z"
}
```

```http
# Consume messages from topic
POST /api/v1/mq-kafka-orders/op/consume-results
Authorization: Bearer <token>
X-Nonce: <server-nonce>

{
  "max_messages": 5,
  "timeout_seconds": 10
}
```

Response (NDJSON streaming):
```json
{"offset": 100, "key": "ORD-001", "value": {"status": "completed"}, "timestamp": "..."}
{"offset": 101, "key": "ORD-002", "value": {"status": "failed", "error": "..."}, "timestamp": "..."}
```

```http
# List available topics (admin only)
GET /api/v1/mq-kafka-orders/topics
Authorization: Bearer <admin-token>
X-Nonce: <server-nonce>
```

### Handler Implementation

```go
// internal/services/mq/handler.go

package mq

import (
    "github.com/ThreeDotsLabs/watermill"
    "github.com/ThreeDotsLabs/watermill-kafka/v2/pkg/kafka"
    "github.com/ThreeDotsLabs/watermill-amqp/v2/pkg/amqp"
    "github.com/ThreeDotsLabs/watermill/message"
)

func init() {
    services.RegisterType("mq", NewMQService)
}

type MQService struct {
    services.BaseService
    publisher  message.Publisher
    subscriber message.Subscriber
    config     MQConfig
}

func (s *MQService) createPublisher() (message.Publisher, error) {
    switch s.config.Backend {
    case "kafka":
        return kafka.NewPublisher(
            kafka.PublisherConfig{
                Brokers:   s.config.Brokers,
                Marshaler: kafka.DefaultMarshaler{},
            },
            watermill.NewStdLogger(false, false),
        )
    case "rabbitmq":
        return amqp.NewPublisher(
            amqp.NewDurableQueueConfig(s.config.ConnectionURL),
            watermill.NewStdLogger(false, false),
        )
    // ... other backends
    }
}

func (s *MQService) handlePublish(w http.ResponseWriter, r *http.Request, op Operation) {
    var req PublishRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        s.WriteError(w, http.StatusBadRequest, "Invalid request body")
        return
    }
    
    // Validate topic is allowed
    if !s.isTopicAllowed(op.Topic) {
        s.WriteError(w, http.StatusForbidden, "Topic not allowed")
        return
    }
    
    msg := message.NewMessage(watermill.NewUUID(), req.Payload)
    if err := s.publisher.Publish(op.Topic, msg); err != nil {
        s.WriteError(w, http.StatusInternalServerError, "Publish failed: "+err.Error())
        return
    }
    
    s.WriteJSON(w, http.StatusOK, map[string]any{
        "status":    "published",
        "topic":     op.Topic,
        "message_id": msg.UUID,
        "timestamp": time.Now().UTC(),
    })
}

func (s *MQService) handleConsume(w http.ResponseWriter, r *http.Request, op Operation) {
    ctx, cancel := context.WithTimeout(r.Context(), op.Timeout)
    defer cancel()
    
    messages, err := s.subscriber.Subscribe(ctx, op.Topic)
    if err != nil {
        s.WriteError(w, http.StatusInternalServerError, "Subscribe failed")
        return
    }
    
    // Stream messages as NDJSON
    s.StreamJSON(w, func(emit func(any) error) error {
        count := 0
        for msg := range messages {
            if count >= op.MaxMessages {
                break
            }
            if err := emit(map[string]any{
                "message_id": msg.UUID,
                "payload":    string(msg.Payload),
                "metadata":   msg.Metadata,
            }); err != nil {
                return err
            }
            msg.Ack()
            count++
        }
        return nil
    })
}
```

### Security Considerations

1. **Topic allowlist** — Only configured topics can be accessed
2. **Connection encryption** — Credentials encrypted via secretprotector
3. **TLS support** — mTLS for Kafka, AMQPS for RabbitMQ
4. **Consumer group isolation** — Each service uses its own consumer group
5. **Message size limits** — Configurable max payload size

---

## Adding Services: LDAP/Active Directory

### Overview

The [go-ldap](https://github.com/go-ldap/ldap) library provides LDAP v3 operations for querying Active Directory or other LDAP servers. Common use cases: user lookups, group membership checks, authentication validation.

**SECURITY: TLS IS MANDATORY.** The service REFUSES to connect without encryption:
- **LDAPS** (port 636) — TLS from connection start (recommended)
- **StartTLS** (port 389) — Upgrades plain connection to TLS

Plain LDAP (port 389 without StartTLS) is **blocked** — credentials would be transmitted in cleartext.

### Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                      service-gateway                        │
├─────────────────────────────────────────────────────────────┤
│  POST /api/v1/ldap-{name}/op/lookup-user                    │
│  POST /api/v1/ldap-{name}/op/lookup-group                   │
│  POST /api/v1/ldap-{name}/op/check-membership               │
│  POST /api/v1/ldap-{name}/op/search                         │
├─────────────────────────────────────────────────────────────┤
│                    go-ldap (LDAP v3)                        │
│                                                             │
│   ┌─────────────┐  ┌─────────────┐  ┌─────────────┐        │
│   │   Active    │  │  OpenLDAP   │  │   389 DS    │  ...   │
│   │  Directory  │  │             │  │             │        │
│   └──────┬──────┘  └──────┬──────┘  └──────┬──────┘        │
└──────────┼────────────────┼────────────────┼───────────────┘
           │                │                │
           ▼                ▼                ▼
      Directory Servers (LDAPS 636 / StartTLS 389)
```

### Service Config (`services/ldap-corp-ad/config.json`)

```json
{
  "type": "ldap",
  "description": "Corporate Active Directory lookups",
  "version": "1.0",
  "updated": "2026-07-22T10:00:00Z",
  
  "settings": {
    "server": "ldaps://dc01.corp.example.com:636",
    "encrypted_bind_dn": "AES256-GCM:base64...",
    "encrypted_bind_password": "AES256-GCM:base64...",
    
    "tls": {
      "start_tls": false,
      "ca_cert": "D:\\certs\\corp-ca.pem",
      "skip_verify": false
    },
    
    "connection": {
      "timeout_seconds": 10,
      "pool_size": 5
    },
    
    "base_dn": "DC=corp,DC=example,DC=com",
    
    "operations": {
      "lookup-user": {
        "description": "Get user details by sAMAccountName",
        "search_base": "OU=Users,DC=corp,DC=example,DC=com",
        "filter": "(&(objectClass=user)(sAMAccountName={{username}}))",
        "attributes": ["cn", "mail", "department", "title", "manager", "memberOf"],
        "params": {
          "username": {"type": "string", "required": true, "pattern": "^[a-zA-Z0-9._-]+$"}
        }
      },
      "lookup-user-by-email": {
        "description": "Get user details by email address",
        "search_base": "OU=Users,DC=corp,DC=example,DC=com",
        "filter": "(&(objectClass=user)(mail={{email}}))",
        "attributes": ["cn", "sAMAccountName", "department", "title"],
        "params": {
          "email": {"type": "string", "required": true, "pattern": "^[^@]+@[^@]+$"}
        }
      },
      "lookup-group": {
        "description": "Get group members",
        "search_base": "OU=Groups,DC=corp,DC=example,DC=com",
        "filter": "(&(objectClass=group)(cn={{group_name}}))",
        "attributes": ["cn", "description", "member"],
        "params": {
          "group_name": {"type": "string", "required": true}
        }
      },
      "check-membership": {
        "description": "Check if user is member of group (recursive)",
        "search_base": "OU=Users,DC=corp,DC=example,DC=com",
        "filter": "(&(objectClass=user)(sAMAccountName={{username}})(memberOf:1.2.840.113556.1.4.1941:={{group_dn}}))",
        "attributes": ["cn"],
        "params": {
          "username": {"type": "string", "required": true},
          "group_dn": {"type": "string", "required": true}
        }
      },
      "search-users-by-dept": {
        "description": "Find all users in a department",
        "search_base": "OU=Users,DC=corp,DC=example,DC=com",
        "filter": "(&(objectClass=user)(department={{department}}))",
        "attributes": ["cn", "sAMAccountName", "mail", "title"],
        "params": {
          "department": {"type": "string", "required": true}
        },
        "size_limit": 500
      }
    },
    
    "raw_search": {
      "enabled": false
    }
  }
}
```

### API Usage

```http
# Lookup user by username
POST /api/v1/ldap-corp-ad/op/lookup-user
Authorization: Bearer <token>
X-Nonce: <server-nonce>

{
  "username": "jsmith"
}
```

Response:
```json
{
  "found": true,
  "entries": [
    {
      "dn": "CN=John Smith,OU=Users,DC=corp,DC=example,DC=com",
      "attributes": {
        "cn": "John Smith",
        "mail": "jsmith@example.com",
        "department": "Engineering",
        "title": "Senior Developer",
        "manager": "CN=Jane Doe,OU=Users,DC=corp,DC=example,DC=com",
        "memberOf": [
          "CN=Developers,OU=Groups,DC=corp,DC=example,DC=com",
          "CN=VPN-Users,OU=Groups,DC=corp,DC=example,DC=com"
        ]
      }
    }
  ]
}
```

```http
# Check group membership (recursive)
POST /api/v1/ldap-corp-ad/op/check-membership
Authorization: Bearer <token>
X-Nonce: <server-nonce>

{
  "username": "jsmith",
  "group_dn": "CN=Admin-Access,OU=Groups,DC=corp,DC=example,DC=com"
}
```

Response:
```json
{
  "is_member": true,
  "user_dn": "CN=John Smith,OU=Users,DC=corp,DC=example,DC=com",
  "group_dn": "CN=Admin-Access,OU=Groups,DC=corp,DC=example,DC=com"
}
```

```http
# Search users by department
POST /api/v1/ldap-corp-ad/op/search-users-by-dept
Authorization: Bearer <token>
X-Nonce: <server-nonce>

{
  "department": "Finance"
}
```

Response (NDJSON for large results):
```json
{"dn": "CN=Alice Brown,OU=Users,...", "cn": "Alice Brown", "mail": "abrown@...", "title": "Analyst"}
{"dn": "CN=Bob Wilson,OU=Users,...", "cn": "Bob Wilson", "mail": "bwilson@...", "title": "Manager"}
```

### Handler Implementation

```go
// internal/services/ldap/handler.go

package ldap

import (
    "crypto/tls"
    "fmt"
    "strings"
    
    "github.com/go-ldap/ldap/v3"
    "github.com/edsilegxrepo/service-gateway/internal/services"
)

func init() {
    services.RegisterType("ldap", NewLDAPService)
}

type LDAPService struct {
    services.BaseService
    pool   *ldap.ConnPool
    config LDAPConfig
}

func NewLDAPService(name string, cfg json.RawMessage) (services.Service, error) {
    var config LDAPConfig
    if err := json.Unmarshal(cfg, &config); err != nil {
        return nil, err
    }
    
    // Decrypt bind credentials
    bindDN, err := secretprotector.Decrypt(config.EncryptedBindDN)
    if err != nil {
        return nil, fmt.Errorf("decrypt bind_dn: %w", err)
    }
    bindPwd, err := secretprotector.Decrypt(config.EncryptedBindPassword)
    if err != nil {
        return nil, fmt.Errorf("decrypt bind_password: %w", err)
    }
    
    // Create connection pool
    pool, err := createPool(config, bindDN, bindPwd)
    if err != nil {
        return nil, err
    }
    
    return &LDAPService{
        BaseService: services.NewBaseService(name),
        pool:        pool,
        config:      config,
    }, nil
}

func createPool(cfg LDAPConfig, bindDN, bindPwd string) (*ldap.ConnPool, error) {
    // SECURITY: Enforce TLS - refuse plaintext LDAP connections
    isLDAPS := strings.HasPrefix(cfg.Server, "ldaps://")
    useStartTLS := cfg.TLS.StartTLS
    
    if !isLDAPS && !useStartTLS {
        return nil, fmt.Errorf("SECURITY: TLS required - use ldaps:// or enable StartTLS (plaintext LDAP rejected)")
    }
    
    tlsConfig := &tls.Config{
        InsecureSkipVerify: cfg.TLS.SkipVerify,
        MinVersion:         tls.VersionTLS12,  // Reject TLS 1.0/1.1
    }
    
    if cfg.TLS.CACert != "" {
        caCert, err := os.ReadFile(cfg.TLS.CACert)
        if err != nil {
            return nil, fmt.Errorf("load CA cert: %w", err)
        }
        tlsConfig.RootCAs = x509.NewCertPool()
        tlsConfig.RootCAs.AppendCertsFromPEM(caCert)
    }
    
    dialOpts := []ldap.DialOpt{
        ldap.DialWithTLSConfig(tlsConfig),
    }
    
    return ldap.NewConnPool(
        cfg.Server,
        cfg.Connection.PoolSize,
        func(conn *ldap.Conn) error {
            return conn.Bind(bindDN, bindPwd)
        },
        dialOpts...,
    )
}

func (s *LDAPService) handleOperation(w http.ResponseWriter, r *http.Request, op Operation) {
    // Parse request parameters
    var params map[string]string
    if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
        s.WriteError(w, http.StatusBadRequest, "Invalid request body")
        return
    }
    
    // Validate and sanitize parameters
    for name, spec := range op.Params {
        val, ok := params[name]
        if spec.Required && !ok {
            s.WriteError(w, http.StatusBadRequest, fmt.Sprintf("Missing required param: %s", name))
            return
        }
        if spec.Pattern != "" && !regexp.MustCompile(spec.Pattern).MatchString(val) {
            s.WriteError(w, http.StatusBadRequest, fmt.Sprintf("Invalid param format: %s", name))
            return
        }
        // Escape LDAP special characters to prevent injection
        params[name] = ldap.EscapeFilter(val)
    }
    
    // Build filter from template
    filter := op.Filter
    for name, val := range params {
        filter = strings.ReplaceAll(filter, "{{"+name+"}}", val)
    }
    
    // Execute search
    conn, err := s.pool.Get()
    if err != nil {
        s.WriteError(w, http.StatusServiceUnavailable, "LDAP connection failed")
        return
    }
    defer s.pool.Put(conn)
    
    searchReq := ldap.NewSearchRequest(
        op.SearchBase,
        ldap.ScopeWholeSubtree,
        ldap.NeverDerefAliases,
        op.SizeLimit,
        int(s.config.Connection.TimeoutSeconds),
        false,
        filter,
        op.Attributes,
        nil,
    )
    
    result, err := conn.Search(searchReq)
    if err != nil {
        s.WriteError(w, http.StatusInternalServerError, "LDAP search failed: "+err.Error())
        return
    }
    
    // Format response
    entries := make([]map[string]any, 0, len(result.Entries))
    for _, entry := range result.Entries {
        attrs := make(map[string]any)
        for _, attr := range entry.Attributes {
            if len(attr.Values) == 1 {
                attrs[attr.Name] = attr.Values[0]
            } else {
                attrs[attr.Name] = attr.Values
            }
        }
        entries = append(entries, map[string]any{
            "dn":         entry.DN,
            "attributes": attrs,
        })
    }
    
    s.WriteJSON(w, http.StatusOK, map[string]any{
        "found":   len(entries) > 0,
        "count":   len(entries),
        "entries": entries,
    })
}
```

### Security Considerations

1. **LDAPS required** — Always use LDAPS (port 636) or StartTLS
2. **Bind credential encryption** — DN and password encrypted via secretprotector
3. **LDAP injection prevention** — All parameters escaped with `ldap.EscapeFilter()`
4. **Parameter validation** — Regex patterns enforce input format
5. **No raw search** — Disable `raw_search` in production
6. **Size limits** — Prevent unbounded queries
7. **Connection pooling** — Reuse authenticated connections

### Common Active Directory Patterns

| Operation | LDAP Filter |
|-----------|-------------|
| User by username | `(&(objectClass=user)(sAMAccountName={{user}}))` |
| User by email | `(&(objectClass=user)(mail={{email}}))` |
| Group members | `(&(objectClass=group)(cn={{group}}))` |
| Recursive membership | `(memberOf:1.2.840.113556.1.4.1941:={{group_dn}})` |
| Enabled users only | `(&(objectClass=user)(!(userAccountControl:1.2.840.113556.1.4.803:=2)))` |
| Users in OU | `(&(objectClass=user))` with specific `search_base` |

---

## Adding Services: Rclone Integration

### Overview

The [librclone](https://github.com/rclone/rclone/tree/master/librclone) library embeds rclone's full functionality, enabling cloud storage operations across 40+ backends (GCS, S3, Azure, SFTP, etc.) without subprocess spawning. Rclone manages all the complex details: retries, checksums, bandwidth throttling, parallel transfers, and backend-specific quirks.

### Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                      service-gateway                        │
├─────────────────────────────────────────────────────────────┤
│  POST /api/v1/rclone-{name}/move                            │
│  POST /api/v1/rclone-{name}/copy                            │
│  POST /api/v1/rclone-{name}/sync                            │
│  POST /api/v1/rclone-{name}/list                            │
│  POST /api/v1/rclone-{name}/delete                          │
├─────────────────────────────────────────────────────────────┤
│                   librclone (embedded)                      │
│                                                             │
│   ┌─────────┐  ┌─────────┐  ┌─────────┐  ┌─────────┐       │
│   │   GCS   │  │   S3    │  │  Azure  │  │  SFTP   │  ...  │
│   └────┬────┘  └────┬────┘  └────┬────┘  └────┬────┘       │
└────────┼────────────┼───────────┼───────────┼──────────────┘
         │            │           │           │
         ▼            ▼           ▼           ▼
    Cloud Storage Backends (configured via rclone.conf)
```

### Service Config (`services/rclone-gcs-archive/config.json`)

```json
{
  "type": "rclone",
  "description": "Archive files to GCS bucket",
  "version": "1.0",
  "updated": "2026-07-22T10:00:00Z",
  
  "settings": {
    "config_file": "D:\\config\\rclone\\rclone.conf",
    "encrypted_config_pass": "AES256-GCM:base64...",
    
    "rclone_options": {
      "use_mmap": true,
      "no_check_dest": true,
      "cache_workers": 2,
      "checkers": 2,
      "transfers": 10,
      "retries": 2,
      "max_backlog": 1000000,
      "bandwidth_limit": "",
      "timeout_seconds": 300
    },
    
    "operations": {
      "sftp-to-gcs-live": {
        "description": "Move files from SFTP to GCS live bucket",
        "operation": "move",
        "src_remote": "icp-001-SFTP",
        "dst_remote": "finna-globdocrepo-dev_GCS-ECM",
        "dst_bucket": "gfusw-decm400-live",
        "params": {
          "src_folder": {"type": "string", "required": true, "description": "Source folder under SFTP root"}
        }
      },
      "sftp-to-gcs-dev": {
        "description": "Move files from SFTP to GCS dev bucket",
        "operation": "move",
        "src_remote": "icp-001-SFTP",
        "dst_remote": "finna-globdocrepo-dev_GCS-ECM",
        "dst_bucket": "gfusw-decm400-dev",
        "params": {
          "src_folder": {"type": "string", "required": true}
        }
      },
      "gcs-to-backup": {
        "description": "Copy files from GCS live to backup bucket",
        "operation": "copy",
        "src_remote": "finna-globdocrepo-dev_GCS-ECM",
        "dst_remote": "gcs-backup",
        "params": {
          "src_bucket": {"type": "string", "required": true},
          "src_path": {"type": "string", "required": true},
          "dst_path": {"type": "string", "required": false, "default": "{{src_path}}"}
        }
      },
      "list-sftp": {
        "description": "List files in SFTP folder",
        "operation": "list",
        "remote": "icp-001-SFTP",
        "params": {
          "path": {"type": "string", "required": true},
          "recursive": {"type": "bool", "required": false, "default": false}
        }
      }
    },
    
    "raw_access": {
      "enabled": true,
      "allowed_remotes": ["icp-001-SFTP", "finna-globdocrepo-dev_GCS-ECM", "gcs-backup"],
      "allowed_operations": ["move", "copy", "sync", "list", "delete"]
    }
  }
}
```

### Operation Modes

Like MongoDB queries, rclone operations support two modes:

| Mode | Config | Use Case |
|------|--------|----------|
| **Named operations only** | `raw_access.enabled: false` | Production — strict control |
| **Named + raw access** | `raw_access.enabled: true` + restrictions | Admin — flexible but bounded |

### API Key Access Levels

```json
"auth": {
  "keys": {
    "ga-production": {
      "encrypted_secret": "...",
      "services": ["rclone-gcs-archive"],
      "rclone_access": "operations_only",
      "description": "Integration Platform — named operations only"
    },
    "ga-admin": {
      "encrypted_secret": "...",
      "services": ["rclone-gcs-archive"],
      "rclone_access": "raw",
      "description": "Admin — full raw access"
    }
  }
}
```

| Access Level | Named Operations | Raw Access |
|--------------|------------------|------------|
| `operations_only` | ✓ | ✗ |
| `raw` | ✓ | ✓ (within allowed_remotes) |

### Named Operation Usage

```http
# Named operation (like named MongoDB query)
POST /api/v1/rclone-gcs-archive/op/sftp-to-gcs-live
Authorization: Bearer <token>
X-Nonce: <server-nonce>

{
  "src_folder": "Default"
}
```

Equivalent to:
```bash
rclone move icp-001-SFTP:/Default/ finna-globdocrepo-dev_GCS-ECM:/gfusw-decm400-live/
```

```http
# List operation
POST /api/v1/rclone-gcs-archive/op/list-sftp
Authorization: Bearer <token>
X-Nonce: <server-nonce>

{
  "path": "/Default/incoming/",
  "recursive": true
}
```

### Raw Access Usage (Admin Only)

```http
# Raw move (requires rclone_access: "raw")
POST /api/v1/rclone-gcs-archive/move
Authorization: Bearer <admin-token>
X-Nonce: <server-nonce>

{
  "src_remote": "icp-001-SFTP",
  "src_path": "/Default/",
  "dst_remote": "finna-globdocrepo-dev_GCS-ECM",
  "dst_path": "/gfusw-decm400-live/"
}
```

```http
# Blocked — remote not in allowed_remotes
POST /api/v1/rclone-gcs-archive/move
{
  "src_remote": "untrusted-sftp",
  ...
}
# Returns 403 Forbidden: remote "untrusted-sftp" not allowed
```

### Template Variables

| Variable | Example | Description |
|----------|---------|-------------|
| `{{param_name}}` | `{{src_folder}}` | Parameter from request body |
| `{{today}}` | `2026-07-22` | Current date |
| `{{today-7d}}` | `2026-07-15` | Date offset |
| `{{timestamp}}` | `20260722_143052` | Current timestamp |

Example with date-based destination:
```json
"sftp-to-gcs-dated": {
  "operation": "move",
  "src_remote": "icp-001-SFTP",
  "src_path": "/{{src_folder}}/",
  "dst_remote": "finna-globdocrepo-dev_GCS-ECM",
  "dst_path": "/{{dst_bucket}}/{{today}}/",
  "params": {
    "src_folder": {"type": "string", "required": true},
    "dst_bucket": {"type": "string", "required": true}
  }
}
```

**Rclone options mapping from gsmove_realtime.sh:**

| Script Flag | Config Key | Description |
|-------------|------------|-------------|
| `--use-mmap` | `use_mmap` | Use memory-mapped I/O for large files |
| `--no-check-dest` | `no_check_dest` | Skip checking destination (faster) |
| `--cache-workers 2` | `cache_workers` | Number of cache workers |
| `--checkers 2` | `checkers` | Parallel file checkers |
| `--transfers 10` | `transfers` | Parallel file transfers |
| `--retries 2` | `retries` | Retry failed operations |
| `--max-backlog 1000000` | `max_backlog` | Max files in transfer queue |
| `-v` | (logging) | Handled by gateway logging |

### Handler Implementation

```go
// internal/services/rclone/handler.go
package rclone

import (
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "sync"

    "github.com/rclone/rclone/librclone/librclone"
)

type Service struct {
    name        string
    config      Config
    mu          sync.RWMutex
    initialized bool
}

type Config struct {
    ConfigFile        string   `json:"config_file"`
    DefaultRemote     string   `json:"default_remote"`
    AllowedRemotes    []string `json:"allowed_remotes"`
    AllowedOperations []string `json:"allowed_operations"`
    MaxTransferSizeMB int      `json:"max_transfer_size_mb"`
    TimeoutSeconds    int      `json:"timeout_seconds"`
    BandwidthLimit    string   `json:"bandwidth_limit"`
    Transfers         int      `json:"transfers"`
    Checkers          int      `json:"checkers"`
}

// MoveRequest represents a file move operation
type MoveRequest struct {
    SrcRemote string `json:"src_remote"`
    SrcPath   string `json:"src_path"`
    DstRemote string `json:"dst_remote"`
    DstPath   string `json:"dst_path"`
}

func NewService(name string) *Service {
    return &Service{name: name}
}

func (s *Service) Initialize() error {
    librclone.Initialize()
    s.initialized = true
    return nil
}

func (s *Service) Shutdown() {
    if s.initialized {
        librclone.Finalize()
    }
}

func (s *Service) Reload(ctx context.Context, cfg json.RawMessage) error {
    var newCfg Config
    if err := json.Unmarshal(cfg, &newCfg); err != nil {
        return err
    }
    s.mu.Lock()
    s.config = newCfg
    s.mu.Unlock()
    return nil
}

// Move handles POST /api/v1/rclone-{name}/move
func (s *Service) Move(w http.ResponseWriter, r *http.Request) {
    var req MoveRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, "invalid request", http.StatusBadRequest)
        return
    }

    s.mu.RLock()
    cfg := s.config
    s.mu.RUnlock()

    // Validate remotes are allowed
    if !s.isRemoteAllowed(req.SrcRemote) || !s.isRemoteAllowed(req.DstRemote) {
        http.Error(w, "remote not allowed", http.StatusForbidden)
        return
    }

    // Build RPC params
    params := map[string]interface{}{
        "srcFs": fmt.Sprintf("%s:%s", req.SrcRemote, req.SrcPath),
        "dstFs": fmt.Sprintf("%s:%s", req.DstRemote, req.DstPath),
        "_config": map[string]interface{}{
            "ConfigPath": cfg.ConfigFile,
        },
    }
    
    if cfg.BandwidthLimit != "" {
        params["_config"].(map[string]interface{})["BwLimit"] = cfg.BandwidthLimit
    }

    paramsJSON, _ := json.Marshal(params)
    output, status := librclone.RPC("sync/move", string(paramsJSON))

    if status != 200 {
        http.Error(w, output, http.StatusInternalServerError)
        return
    }

    w.Header().Set("Content-Type", "application/json")
    w.Write([]byte(output))
}

func (s *Service) isRemoteAllowed(remote string) bool {
    s.mu.RLock()
    defer s.mu.RUnlock()
    for _, allowed := range s.config.AllowedRemotes {
        if allowed == remote {
            return true
        }
    }
    return false
}
```

### API Endpoints

#### Move Files
```http
POST /api/v1/rclone-gcs-archive/move
Authorization: Bearer <token>
X-Nonce: <server-nonce>

{
  "src_remote": "sftp-internal",
  "src_path": "/Default/EDI/incoming/",
  "dst_remote": "gcs-archive",
  "dst_path": "/edi/2026/07/22/"
}
```

#### Move Files (gsmove_realtime.sh equivalent)

The original bash script:
```bash
# gsmove_realtime.sh icp-001-SFTP Default finna-globdocrepo-dev_GCS-ECM gfusw-decm400-live
rclone move ${REMOTE_SOURCE_ID}:/${BASE_FOLDER}/ ${BUCKET_SOURCE_ID}:/${BUCKET_NAME}/
```

Equivalent API call:
```http
POST /api/v1/rclone-gcs-archive/move
Authorization: Bearer <token>
X-Nonce: <server-nonce>

{
  "src_remote": "icp-001-SFTP",
  "src_path": "/Default/",
  "dst_remote": "finna-globdocrepo-dev_GCS-ECM",
  "dst_path": "/gfusw-decm400-live/"
}
```

The rclone options from the script are configured in the service config:
```json
{
  "settings": {
    "use_mmap": true,
    "no_check_dest": true,
    "cache_workers": 2,
    "checkers": 2,
    "transfers": 10,
    "retries": 2,
    "max_backlog": 1000000
  }
}
```

#### Copy Files
```http
POST /api/v1/rclone-gcs-archive/copy
Authorization: Bearer <token>
X-Nonce: <server-nonce>

{
  "src_remote": "gcs-archive",
  "src_path": "/edi/2026/07/22/file.txt",
  "dst_remote": "gcs-backup",
  "dst_path": "/edi/2026/07/22/file.txt"
}
```

#### List Directory
```http
POST /api/v1/rclone-gcs-archive/list
Authorization: Bearer <token>
X-Nonce: <server-nonce>

{
  "remote": "gcs-archive",
  "path": "/edi/2026/07/",
  "recursive": false
}
```

#### Sync Directory
```http
POST /api/v1/rclone-gcs-archive/sync
Authorization: Bearer <token>
X-Nonce: <server-nonce>

{
  "src_remote": "sftp-internal",
  "src_path": "/Default/EDI/",
  "dst_remote": "gcs-archive",
  "dst_path": "/edi/",
  "delete_missing": true
}
```

### RPC Operations Mapping

| API Endpoint | librclone RPC Method |
|-------------|---------------------|
| `/move`     | `sync/move`         |
| `/copy`     | `sync/copy`         |
| `/sync`     | `sync/sync`         |
| `/list`     | `operations/list`   |
| `/delete`   | `operations/delete` |

### What Rclone Manages For You

| Concern | Rclone Handles |
|---------|----------------|
| **Retries** | Automatic retry with exponential backoff |
| **Checksums** | Verifies file integrity after transfer |
| **Bandwidth** | Throttling via `bandwidth_limit` |
| **Parallelism** | Multiple simultaneous transfers (`transfers`) |
| **Chunking** | Large file multipart uploads |
| **Resume** | Resumes interrupted transfers |
| **Rate Limits** | Backend-specific rate limit handling |
| **Auth Refresh** | OAuth token refresh for GCS/S3/etc. |

### Security Considerations

1. **Remote whitelist** — Only configured remotes can be used
2. **Operation whitelist** — Only allowed operations are exposed
3. **Path validation** — Prevent path traversal attacks
4. **Size limits** — Maximum transfer size enforced
5. **Bandwidth throttling** — Prevent network saturation
6. **Credential isolation** — rclone.conf contains backend credentials, secured via file permissions

### Rclone Configuration & Encryption

#### Defense-in-Depth Approach (Recommended)

rclone.conf contains sensitive credentials (API keys, service account paths, passwords). We use a **hybrid encryption** approach:

1. **All credentials inlined** — no external key files or JSON files
2. **rclone.conf encrypted at rest** — using rclone's native config encryption
3. **Config password protected by secretprotector** — encrypted with your master key
4. **Password only in memory** — never written to disk

```
┌─────────────────────────────────────────────────────────────┐
│                    Encryption Flow                          │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│   Master Key (gateway.key file, 0400 permissions)           │
│       │                                                     │
│       ▼ secretprotector.Decrypt()                           │
│   RCLONE_CONFIG_PASS (in memory only)                       │
│       │                                                     │
│       ▼ librclone uses env var                              │
│   rclone.conf (encrypted on disk, all creds inlined)        │
│       │                                                     │
│       ▼ contains                                            │
│   GCS/S3/SFTP credentials (no external file references)     │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

**Security benefits:**
- Single encrypted file contains ALL secrets (no loose key files)
- rclone.conf alone is useless (encrypted)
- Service config alone is useless (only has encrypted password blob)
- Need master key + service config + rclone.conf to access credentials

#### CLI Tools for Setup

The gateway binary includes built-in commands to simplify secure setup:

##### Generate Master Key: `-key-gen`

```powershell
# Generate master key with secure permissions (recommended first step)
gateway.exe -key-gen D:\secrets\gateway.key

# Output:
# Generated 256-bit key: D:\secrets\gateway.key
# Permissions set: owner read-only (0400 equivalent)
# 
# Add to environment or config:
#   Windows: setx GATEWAY_MASTER_KEY_FILE "D:\secrets\gateway.key" /M
#   Linux:   export GATEWAY_MASTER_KEY_FILE="/etc/gateway/master.key"
```

**What it does:**
1. Generates cryptographically secure 256-bit (32-byte) key
2. Writes to specified path
3. Sets restrictive file permissions (Windows ACL / Unix 0400)
4. Prints next steps

##### Convert rclone.conf: `-rclone-convert`

```powershell
# Convert existing rclone.conf: inline all external files + encrypt
gateway.exe -rclone-convert `
  -input "C:\Users\mftadmin\.config\rclone\rclone.conf" `
  -output "D:\config\rclone\rclone.conf" `
  -key-file "D:\secrets\gateway.key"

# Output:
# Reading: C:\Users\mftadmin\.config\rclone\rclone.conf
# 
# Processing remotes:
#   [sftp-internal]
#     - Inlining key_file: C:\Users\mftadmin\.ssh\mckesson_ed25519 → key_pem
#   [gcs-archive]
#     - Inlining service_account_file: D:\vault\gcs-creds.json → service_account_credentials
#   [gcs-backup]
#     - Inlining service_account_file: D:\vault\gcs-backup.json → service_account_credentials
#   [s3-offsite]
#     - Already inline (access_key_id, secret_access_key)
#
# Encrypting with rclone config encryption...
# Generated config password (random 32 bytes)
#
# Writing: D:\config\rclone\rclone.conf (encrypted)
#
# Encrypted config password for your service config:
#   "encrypted_config_pass": "AES256-GCM:base64..."
#
# Add this to your service config.json
#
# ┌─────────────────────────────────────────────────────────────────────┐
# │ SECURITY WARNING: External credential files are now duplicated!    │
# │                                                                     │
# │ The following files have been inlined into the encrypted config.   │
# │ They should be SECURELY DELETED to avoid leaving secrets on disk:  │
# │                                                                     │
# │   1. C:\Users\mftadmin\.ssh\mckesson_ed25519                        │
# │   2. D:\vault\gcs-creds.json                                        │
# │   3. D:\vault\gcs-backup.json                                       │
# │                                                                     │
# │ To delete automatically, re-run with -delete-external-keys:         │
# │   gateway.exe -rclone-convert ... -delete-external-keys             │
# │                                                                     │
# │ Or verify first, then delete manually:                              │
# │   gateway.exe -rclone-test -config D:\config\rclone\rclone.conf     │
# │   shred -u <file>  (Linux) or cipher /w + del (Windows)             │
# └─────────────────────────────────────────────────────────────────────┘

# With automatic deletion (after verification prompt):
gateway.exe -rclone-convert `
  -input "C:\Users\mftadmin\.config\rclone\rclone.conf" `
  -output "D:\config\rclone\rclone.conf" `
  -key-file "D:\secrets\gateway.key" `
  -delete-external-keys

# Additional output when -delete-external-keys is used:
#
# Testing new config before deleting external files...
# ✓ Config decryption: OK
# ✓ Remote [sftp-internal]: OK (connection test passed)
# ✓ Remote [gcs-archive]: OK (bucket access verified)
# ✓ Remote [gcs-backup]: OK (bucket access verified)
# ✓ Remote [s3-offsite]: OK (bucket access verified)
#
# All tests passed. Proceeding with secure deletion...
#
# Securely deleting external credential files:
#   ✓ Deleted: C:\Users\mftadmin\.ssh\mckesson_ed25519
#   ✓ Deleted: D:\vault\gcs-creds.json
#   ✓ Deleted: D:\vault\gcs-backup.json
#
# Done. All credentials are now exclusively in the encrypted rclone.conf.
```

**What it does:**
1. Reads existing rclone.conf
2. For each remote, converts file references to inline:
   - `key_file` → `key_pem` (PEM with `\n` separators)
   - `service_account_file` → `service_account_credentials` (JSON blob)
3. Generates random password for rclone config encryption
4. Encrypts the rclone.conf with that password
5. Encrypts the password with secretprotector (using your master key)
6. Outputs the `encrypted_config_pass` value for your service config

##### Full Setup Workflow

```powershell
# 1. Generate master key (one-time)
gateway.exe -key-gen D:\secrets\gateway.key

# 2. Convert and encrypt rclone config
gateway.exe -rclone-convert `
  -input "$env:USERPROFILE\.config\rclone\rclone.conf" `
  -output "D:\config\rclone\rclone.conf" `
  -key-file "D:\secrets\gateway.key"

# 3. Copy the encrypted_config_pass output to your service config.json

# 4. (Optional) Delete original external key files after verifying
# Remove-Item D:\vault\gcs-creds.json
# Remove-Item C:\Users\mftadmin\.ssh\mckesson_ed25519
```

#### Inline Credential Mapping

| Backend | File Option | Inline Option | Format |
|---------|-------------|---------------|--------|
| **SFTP** | `key_file` | `key_pem` | PEM with `\n` line separators |
| **GCS** | `service_account_file` | `service_account_credentials` | JSON blob (single line) |
| **S3** | — | `access_key_id` + `secret_access_key` | Already inline |
| **Azure** | — | `sas_url` or `key` | Already inline |

#### Service Config

```json
{
  "type": "rclone",
  "description": "Archive files to GCS bucket",
  "version": "1.0",
  
  "settings": {
    "config_file": "D:\\config\\rclone\\rclone.conf",
    "encrypted_config_pass": "AES256-GCM:base64...",
    "allowed_remotes": ["gcs-archive", "gcs-backup", "sftp-internal"],
    "allowed_operations": ["move", "copy", "list", "delete"]
  }
}
```

#### Handler Implementation with Decryption

```go
// internal/services/rclone/handler.go

type Config struct {
    ConfigFile          string   `json:"config_file"`
    EncryptedConfigPass string   `json:"encrypted_config_pass"`
    AllowedRemotes      []string `json:"allowed_remotes"`
    AllowedOperations   []string `json:"allowed_operations"`
    // ... other fields
}

func (s *Service) Initialize(ctx context.Context, masterKey []byte) error {
    // Decrypt rclone config password
    configPass, err := libsecsecrets.Decrypt(ctx, s.config.EncryptedConfigPass, masterKey)
    if err != nil {
        return fmt.Errorf("failed to decrypt rclone config password: %w", err)
    }
    
    // Set env var for librclone (in memory only, never logged)
    os.Setenv("RCLONE_CONFIG_PASS", configPass)
    
    // Also set config path
    os.Setenv("RCLONE_CONFIG", s.config.ConfigFile)
    
    // Initialize librclone
    librclone.Initialize()
    s.initialized = true
    
    return nil
}

func (s *Service) Shutdown() {
    if s.initialized {
        librclone.Finalize()
        // Clear sensitive env var
        os.Unsetenv("RCLONE_CONFIG_PASS")
    }
}
```

#### What Gets Encrypted Where

| Secret | Encrypted By | Stored In |
|--------|-------------|-----------|
| SFTP private keys | rclone.conf encryption | rclone.conf (inlined as key_pem) |
| GCS service account JSON | rclone.conf encryption | rclone.conf (inlined as service_account_credentials) |
| S3 access keys | rclone.conf encryption | rclone.conf |
| rclone.conf password | secretprotector | service config.json |
| secretprotector master key | file permissions (0400) | gateway.key file |

#### Example: Before and After Conversion

**Before (`-rclone-convert` input):**

```ini
[sftp-internal]
type = sftp
host = sftp.internal.local
user = mftadmin
key_file = /home/mftadmin/.ssh/mckesson-mftmigrate_ed25519_dev

[gcs-archive]
type = google cloud storage
project_number = 123456789
service_account_file = /var/opt/system/vault/gcs-creds_gfusw-decm400.json
bucket_policy_only = true

[s3-offsite]
type = s3
provider = AWS
access_key_id = AKIAXXXXXXXXXXXXXXXX
secret_access_key = xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
region = us-east-1
```

**After (decrypted view, all inlined):**

```ini
[sftp-internal]
type = sftp
host = sftp.internal.local
user = mftadmin
key_pem = -----BEGIN OPENSSH PRIVATE KEY-----\nb3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQAAAAAAAAABAAAAMwAAAAtzc2gtZW\nQyNTUxOQAAACBHK9toNqPF0rEPxS8ysHF...\n-----END OPENSSH PRIVATE KEY-----

[gcs-archive]
type = google cloud storage
project_number = 123456789
service_account_credentials = {"type":"service_account","project_id":"my-project","private_key_id":"abc123def456","private_key":"-----BEGIN PRIVATE KEY-----\nMIIEvQIBADANBgkqhkiG9w0BAQEFAASC...\n-----END PRIVATE KEY-----\n","client_email":"archive-sa@my-project.iam.gserviceaccount.com","client_id":"123456789012345678901","auth_uri":"https://accounts.google.com/o/oauth2/auth","token_uri":"https://oauth2.googleapis.com/token"}
bucket_policy_only = true

[s3-offsite]
type = s3
provider = AWS
access_key_id = AKIAXXXXXXXXXXXXXXXX
secret_access_key = xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
region = us-east-1
```

**On disk (encrypted):**

```
# Encrypted rclone configuration File

RCLONE_ENCRYPT_V0:
dGhpcyBpcyBub3QgcmVhbGx5IGVuY3J5cHRlZCBidXQgeW91IGdldCB0aGUgaWRl
YS4gVGhlIGFjdHVhbCBlbmNyeXB0ZWQgY29udGVudCB3b3VsZCBiZSBtdWNoIGxv
bmdlciBhbmQgY29tcGxldGVseSB1bnJlYWRhYmxlLg==
```

#### -rclone-convert Implementation

```go
// cmd/gateway/rclone_convert.go
package main

import (
    "crypto/rand"
    "encoding/base64"
    "encoding/json"
    "fmt"
    "os"
    "os/exec"
    "path/filepath"
    "regexp"
    "strings"

    "github.com/edsilegxrepo/secretprotector/pkg/libsecsecrets"
)

type RcloneConvertCmd struct {
    Input   string
    Output  string
    KeyFile string
}

func (c *RcloneConvertCmd) Run() error {
    // 1. Read original rclone.conf
    content, err := os.ReadFile(c.Input)
    if err != nil {
        return fmt.Errorf("read input: %w", err)
    }
    
    fmt.Printf("Reading: %s\n\n", c.Input)
    fmt.Println("Processing remotes:")
    
    // 2. Parse and inline external files
    converted := c.inlineExternalFiles(string(content))
    
    // 3. Generate random password for rclone encryption
    passBytes := make([]byte, 32)
    if _, err := rand.Read(passBytes); err != nil {
        return fmt.Errorf("generate password: %w", err)
    }
    configPass := base64.StdEncoding.EncodeToString(passBytes)
    
    // 4. Write temp file, encrypt with rclone
    tempFile := c.Output + ".tmp"
    if err := os.WriteFile(tempFile, []byte(converted), 0600); err != nil {
        return fmt.Errorf("write temp: %w", err)
    }
    defer os.Remove(tempFile)
    
    // Use rclone to encrypt the config
    fmt.Println("\nEncrypting with rclone config encryption...")
    cmd := exec.Command("rclone", "config", "encryption", "set",
        "--config", tempFile,
        "--password-command", fmt.Sprintf("echo %s", configPass))
    if err := cmd.Run(); err != nil {
        return fmt.Errorf("rclone encrypt: %w", err)
    }
    
    // Move encrypted file to output
    if err := os.Rename(tempFile, c.Output); err != nil {
        return fmt.Errorf("move output: %w", err)
    }
    
    fmt.Printf("\nWriting: %s (encrypted)\n", c.Output)
    
    // 5. Encrypt the config password with secretprotector
    masterKey, err := libsecsecrets.LoadKeyFile(c.KeyFile)
    if err != nil {
        return fmt.Errorf("load master key: %w", err)
    }
    defer libsecsecrets.ZeroBuffer(masterKey)
    
    encryptedPass, err := libsecsecrets.Encrypt(configPass, masterKey)
    if err != nil {
        return fmt.Errorf("encrypt config pass: %w", err)
    }
    
    fmt.Println("\nEncrypted config password for your service config:")
    fmt.Printf("  \"encrypted_config_pass\": \"%s\"\n", encryptedPass)
    fmt.Println("\nAdd this to your service config.json")
    
    return nil
}

func (c *RcloneConvertCmd) inlineExternalFiles(content string) string {
    lines := strings.Split(content, "\n")
    var result []string
    currentRemote := ""
    
    remoteRegex := regexp.MustCompile(`^\[([^\]]+)\]`)
    keyFileRegex := regexp.MustCompile(`^key_file\s*=\s*(.+)`)
    saFileRegex := regexp.MustCompile(`^service_account_file\s*=\s*(.+)`)
    
    for _, line := range lines {
        // Track current remote
        if match := remoteRegex.FindStringSubmatch(line); match != nil {
            currentRemote = match[1]
            fmt.Printf("  [%s]\n", currentRemote)
            result = append(result, line)
            continue
        }
        
        // Convert key_file to key_pem
        if match := keyFileRegex.FindStringSubmatch(line); match != nil {
            keyPath := strings.TrimSpace(match[1])
            keyPath = expandPath(keyPath)
            
            keyContent, err := os.ReadFile(keyPath)
            if err != nil {
                fmt.Printf("    - WARNING: Cannot read key_file: %s (%v)\n", keyPath, err)
                result = append(result, line)
                continue
            }
            
            // Convert to single line with \n
            keyPem := strings.ReplaceAll(string(keyContent), "\n", "\\n")
            keyPem = strings.TrimSuffix(keyPem, "\\n")
            
            fmt.Printf("    - Inlining key_file: %s → key_pem\n", keyPath)
            result = append(result, fmt.Sprintf("key_pem = %s", keyPem))
            continue
        }
        
        // Convert service_account_file to service_account_credentials
        if match := saFileRegex.FindStringSubmatch(line); match != nil {
            saPath := strings.TrimSpace(match[1])
            saPath = expandPath(saPath)
            
            saContent, err := os.ReadFile(saPath)
            if err != nil {
                fmt.Printf("    - WARNING: Cannot read service_account_file: %s (%v)\n", saPath, err)
                result = append(result, line)
                continue
            }
            
            // Compact JSON to single line
            var jsonObj map[string]interface{}
            if err := json.Unmarshal(saContent, &jsonObj); err != nil {
                fmt.Printf("    - WARNING: Invalid JSON in service_account_file: %s\n", saPath)
                result = append(result, line)
                continue
            }
            compactJSON, _ := json.Marshal(jsonObj)
            
            fmt.Printf("    - Inlining service_account_file: %s → service_account_credentials\n", saPath)
            result = append(result, fmt.Sprintf("service_account_credentials = %s", string(compactJSON)))
            continue
        }
        
        result = append(result, line)
    }
    
    return strings.Join(result, "\n")
}

func expandPath(path string) string {
    if strings.HasPrefix(path, "~") {
        home, _ := os.UserHomeDir()
        path = filepath.Join(home, path[1:])
    }
    path = os.ExpandEnv(path)
    return path
}
```

### Use Cases

1. **SFTP to GCS Archive** — Move incoming files to cloud storage (replaces gsmove_realtime.sh)
2. **Cross-Cloud Replication** — Copy between GCS and S3
3. **Backup Sync** — Sync directories with deletion of removed files
4. **Directory Listing** — Remote file browser functionality
5. **Multi-Cloud Gateway** — Single API for 40+ storage backends

---

## Adding Services: O365 Mailbox Integration

### Overview

The `o365mbx` service provides Microsoft Graph API access for Office 365 mailbox processing. It wraps the [o365mbx](https://github.com/edsilegxrepo/o365mbx) tool as a Go library, enabling email processing, attachment extraction, and message routing via REST API.

### Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                      service-gateway                        │
├─────────────────────────────────────────────────────────────┤
│  POST /api/v1/o365mbx-{name}/op/process                     │
│  POST /api/v1/o365mbx-{name}/op/route                       │
│  POST /api/v1/o365mbx-{name}/op/healthcheck                 │
├─────────────────────────────────────────────────────────────┤
│                   o365mbx library                           │
│                                                             │
│   ┌─────────────┐  ┌─────────────┐  ┌─────────────┐        │
│   │  Inbox      │  │ Attachments │  │  Body       │        │
│   │  Processing │  │ Extraction  │  │ Conversion  │        │
│   └──────┬──────┘  └──────┬──────┘  └──────┬──────┘        │
└──────────┼────────────────┼────────────────┼───────────────┘
           │                │                │
           ▼                ▼                ▼
      Microsoft Graph API (OAuth2 / JWT)
           │
           ▼
      Office 365 Mailboxes
```

### Service Config (`services/o365mbx-invoices/config.json`)

```json
{
  "type": "o365mbx",
  "description": "Invoice mailbox processing",
  "version": "1.0",
  "updated": "2026-07-22T10:00:00Z",
  
  "settings": {
    "mailbox": "invoices@company.com",
    "encrypted_token": "AES256-GCM:base64...",
    
    "defaults": {
      "timeout_seconds": 120,
      "max_execution_time_msg": 120,
      "parallel": 10,
      "max_retries": 2,
      "api_rate": 5.0,
      "api_burst": 10,
      "chromium_path": "D:\\tools\\chromium\\chrome.exe"
    },
    
    "operations": {
      "process-all": {
        "description": "Process all messages in inbox (full mode)",
        "processing_mode": "full",
        "inbox_folder": "Inbox",
        "convert_body": "none",
        "params": {
          "workspace": {"type": "string", "required": true, "description": "Output directory for artifacts"}
        }
      },
      "process-incremental": {
        "description": "Process new messages since last run",
        "processing_mode": "incremental",
        "inbox_folder": "Inbox",
        "convert_body": "none",
        "params": {
          "workspace": {"type": "string", "required": true},
          "state_file": {"type": "string", "required": true, "description": "Path to state file"}
        }
      },
      "route-invoices": {
        "description": "Route invoice emails to processed/error folders",
        "processing_mode": "route",
        "inbox_folder": "Inbox",
        "processed_folder": "Processed",
        "error_folder": "Errors",
        "convert_body": "pdf",
        "params": {
          "workspace": {"type": "string", "required": true}
        }
      },
      "healthcheck": {
        "description": "Check mailbox connectivity and permissions",
        "params": {}
      },
      "list-folder": {
        "description": "List messages in a folder",
        "params": {
          "folder": {"type": "string", "required": true}
        }
      }
    }
  }
}
```

### API Key Configuration

```json
"ga-invoice-processing": {
  "encrypted_secret": "BASE64_AES_GCM_CIPHERTEXT",
  "services": ["o365mbx-invoices"],
  "o365mbx_access": "operations_only",
  "max_concurrent": 2,
  "rate_limit_rpm": 60,
  "description": "Integration Platform - invoice mailbox processing"
}
```

### Named Operation Usage

```http
# Process all messages (full mode)
POST /api/v1/o365mbx-invoices/op/process-all
Authorization: Bearer <token>
X-Nonce: <server-nonce>

{
  "workspace": "D:\\processing\\invoices\\2026-07-22"
}
```

Response:
```json
{
  "processed": 15,
  "attachments_extracted": 42,
  "errors": 0,
  "duration_ms": 8500,
  "workspace": "D:\\processing\\invoices\\2026-07-22"
}
```

```http
# Incremental processing (new messages only)
POST /api/v1/o365mbx-invoices/op/process-incremental
Authorization: Bearer <token>
X-Nonce: <server-nonce>

{
  "workspace": "D:\\processing\\invoices\\2026-07-22",
  "state_file": "D:\\state\\invoices.json"
}
```

```http
# Route messages to folders
POST /api/v1/o365mbx-invoices/op/route-invoices
Authorization: Bearer <token>
X-Nonce: <server-nonce>

{
  "workspace": "D:\\processing\\invoices\\2026-07-22"
}
```

Response:
```json
{
  "processed": 8,
  "routed_to_processed": 7,
  "routed_to_error": 1,
  "attachments_extracted": 23,
  "bodies_converted": 8,
  "errors": [
    {"message_id": "AAMk...", "error": "attachment download timeout"}
  ]
}
```

```http
# Health check
POST /api/v1/o365mbx-invoices/op/healthcheck
Authorization: Bearer <token>
X-Nonce: <server-nonce>

{}
```

Response:
```json
{
  "status": "ok",
  "mailbox": "invoices@company.com",
  "folders": {
    "Inbox": {"count": 15, "unread": 8},
    "Processed": {"count": 1250},
    "Errors": {"count": 3}
  },
  "token_expires": "2026-07-23T10:00:00Z"
}
```

```http
# List folder contents
POST /api/v1/o365mbx-invoices/op/list-folder
Authorization: Bearer <token>
X-Nonce: <server-nonce>

{
  "folder": "Inbox"
}
```

### Handler Implementation

```go
// internal/services/o365mbx/handler.go
package o365mbx

import (
    "context"
    "encoding/json"
    "net/http"
    "sync"
    "time"
    
    "github.com/edsilegxrepo/o365mbx/pkg/mailbox"
)

type Service struct {
    name   string
    config Config
    client *mailbox.Client
    mu     sync.RWMutex
}

type Config struct {
    Mailbox        string               `json:"mailbox"`
    EncryptedToken string               `json:"encrypted_token"`
    Defaults       DefaultSettings      `json:"defaults"`
    Operations     map[string]Operation `json:"operations"`
}

type DefaultSettings struct {
    TimeoutSeconds      int     `json:"timeout_seconds"`
    MaxExecutionTimeMsg int     `json:"max_execution_time_msg"`
    Parallel            int     `json:"parallel"`
    MaxRetries          int     `json:"max_retries"`
    APIRate             float64 `json:"api_rate"`
    APIBurst            int     `json:"api_burst"`
    ChromiumPath        string  `json:"chromium_path"`
}

type Operation struct {
    Description     string              `json:"description"`
    ProcessingMode  string              `json:"processing_mode"`
    InboxFolder     string              `json:"inbox_folder"`
    ProcessedFolder string              `json:"processed_folder"`
    ErrorFolder     string              `json:"error_folder"`
    ConvertBody     string              `json:"convert_body"`
    Params          map[string]ParamDef `json:"params"`
}

func (s *Service) Initialize(ctx context.Context, masterKey []byte) error {
    // Decrypt JWT token
    token, err := libsecsecrets.Decrypt(ctx, s.config.EncryptedToken, masterKey)
    if err != nil {
        return fmt.Errorf("failed to decrypt token: %w", err)
    }
    
    // Initialize mailbox client
    s.client, err = mailbox.NewClient(mailbox.Options{
        Mailbox:    s.config.Mailbox,
        Token:      token,
        Timeout:    time.Duration(s.config.Defaults.TimeoutSeconds) * time.Second,
        Parallel:   s.config.Defaults.Parallel,
        MaxRetries: s.config.Defaults.MaxRetries,
        APIRate:    s.config.Defaults.APIRate,
        APIBurst:   s.config.Defaults.APIBurst,
    })
    
    return err
}

// HandleOperation processes named operations: POST /op/{name}
func (s *Service) HandleOperation(ctx context.Context, w http.ResponseWriter, r *http.Request, opName string) {
    s.mu.RLock()
    op, exists := s.config.Operations[opName]
    s.mu.RUnlock()
    
    if !exists {
        http.Error(w, "operation not found", http.StatusNotFound)
        return
    }
    
    var req map[string]interface{}
    json.NewDecoder(r.Body).Decode(&req)
    
    // Validate required params
    for name, def := range op.Params {
        if def.Required {
            if _, ok := req[name]; !ok {
                http.Error(w, fmt.Sprintf("missing required param: %s", name), http.StatusBadRequest)
                return
            }
        }
    }
    
    // Execute based on operation type
    var result interface{}
    var err error
    
    switch opName {
    case "healthcheck":
        result, err = s.client.HealthCheck(ctx)
    case "list-folder":
        result, err = s.client.ListFolder(ctx, req["folder"].(string))
    default:
        result, err = s.client.Process(ctx, mailbox.ProcessOptions{
            Mode:            op.ProcessingMode,
            InboxFolder:     op.InboxFolder,
            ProcessedFolder: op.ProcessedFolder,
            ErrorFolder:     op.ErrorFolder,
            ConvertBody:     op.ConvertBody,
            Workspace:       req["workspace"].(string),
            StateFile:       req["state_file"].(string),
            ChromiumPath:    s.config.Defaults.ChromiumPath,
        })
    }
    
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    
    json.NewEncoder(w).Encode(result)
}

func (s *Service) Shutdown() {
    if s.client != nil {
        s.client.Close()
    }
}
```

### JWT Token Management

The JWT token for Graph API access is encrypted with secretprotector:

```powershell
# Encrypt the JWT token
secretprotector -key-file "D:\secrets\gateway.key" -encrypt "eyJ0eXAiOiJKV1QiLCJhbGciOiJSUzI1NiIs..."
# Output: AES256-GCM:base64...
```

**Token refresh:** The o365mbx library handles token refresh internally if using app registration with client credentials flow.

### Security Considerations

1. **Mailbox isolation** — Each service instance accesses one mailbox only
2. **Token encryption** — JWT tokens encrypted with secretprotector
3. **Operations whitelist** — Only configured operations are exposed
4. **Workspace validation** — Output paths validated to prevent traversal
5. **Rate limiting** — Respects Microsoft Graph API limits
6. **Per-key access** — `o365mbx_access: "operations_only"` restricts to named operations

### Use Cases

1. **Invoice Processing** — Extract attachments from invoice emails
2. **EDI Message Routing** — Route EDI confirmations to appropriate folders
3. **Report Distribution** — Process incoming report emails
4. **Health Monitoring** — Check mailbox connectivity and message counts

---

## Adding Services: SQL Database Integration

### Overview

The `sql` service provides unified access to 45+ SQL databases using [xo/dburl](https://github.com/xo/dburl) for connection URL parsing and Go's standard `database/sql` interface. This gives you PostgreSQL, MySQL, SQL Server, Oracle, SQLite, and many more — all through one service type.

### Supported Databases

| Database | URL Scheme | Go Driver |
|----------|------------|-----------|
| PostgreSQL | `postgres://`, `pg://` | `github.com/lib/pq` |
| MySQL | `mysql://`, `my://` | `github.com/go-sql-driver/mysql` |
| SQL Server | `sqlserver://`, `mssql://` | `github.com/microsoft/go-mssqldb` |
| Oracle | `oracle://`, `or://` | `github.com/sijms/go-ora/v2` |
| SQLite3 | `sqlite://`, `file://` | `modernc.org/sqlite` |
| ClickHouse | `clickhouse://`, `ch://` | `github.com/ClickHouse/clickhouse-go/v2` |
| Snowflake | `snowflake://`, `sf://` | `github.com/snowflakedb/gosnowflake` |
| DuckDB | `duckdb://` | `github.com/marcboeker/go-duckdb` |
| ... | ... | [45+ more](https://github.com/xo/usql#database-support) |

### Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                      service-gateway                        │
├─────────────────────────────────────────────────────────────┤
│  GET  /api/v1/sql-{name}/{table}?query=recent-orders        │
│  GET  /api/v1/sql-{name}/{table}?filter=...  (if allowed)   │
│  POST /api/v1/sql-{name}/exec?query=insert-order            │
├─────────────────────────────────────────────────────────────┤
│                   xo/dburl + database/sql                   │
│                                                             │
│   ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐   │
│   │ PostgreSQL│  │  MySQL   │  │SQL Server│  │  Oracle  │   │
│   └─────┬────┘  └─────┬────┘  └─────┬────┘  └─────┬────┘   │
└─────────┼─────────────┼───────────┼─────────────┼──────────┘
          │             │           │             │
          ▼             ▼           ▼             ▼
     Database Servers (connection pooled via database/sql)
```

### Service Config (`services/sql-postgres-production/config.json`)

```json
{
  "type": "sql",
  "description": "Production PostgreSQL - orders and inventory",
  "version": "1.0",
  "updated": "2026-07-22T10:00:00Z",
  
  "connection": {
    "encrypted_dsn": "AES256-GCM:base64...",
    "max_open_conns": 10,
    "max_idle_conns": 5,
    "conn_max_lifetime_minutes": 30,
    "timeout_seconds": 30
  },
  
  "limits": {
    "max_results": 1000,
    "default_limit": 100,
    "max_response_mb": 16
  },
  
  "tables": {
    "orders": {
      "enabled": true,
      "allow_adhoc": true,
      "adhoc_restrictions": {
        "required_filters": ["order_date"],
        "allowed_filters": ["order_date", "status", "customer_id", "region"],
        "denied_columns": ["internal_notes", "cost_price"]
      },
      "queries": {
        "recent-pending": {
          "description": "Pending orders from last 7 days",
          "sql": "SELECT id, customer_id, total, status, order_date FROM orders WHERE order_date >= $1 AND status = 'pending' ORDER BY order_date DESC",
          "params": {
            "date": {"type": "date", "required": false, "default": "{{today-7d}}"}
          }
        },
        "by-customer": {
          "description": "Orders for a specific customer",
          "sql": "SELECT id, total, status, order_date FROM orders WHERE customer_id = $1 ORDER BY order_date DESC",
          "params": {
            "customer_id": {"type": "string", "required": true}
          }
        },
        "monthly-summary": {
          "description": "Order summary by status for a month",
          "sql": "SELECT status, COUNT(*) as count, SUM(total) as total FROM orders WHERE order_date >= $1 AND order_date < $2 GROUP BY status",
          "params": {
            "start_date": {"type": "date", "required": true},
            "end_date": {"type": "date", "required": true}
          }
        }
      }
    },
    "customers": {
      "enabled": true,
      "allow_adhoc": false,
      "queries": {
        "by-id": {
          "sql": "SELECT id, name, email, region FROM customers WHERE id = $1",
          "params": {
            "id": {"type": "int", "required": true}
          }
        },
        "search": {
          "sql": "SELECT id, name, email FROM customers WHERE name ILIKE $1 LIMIT 50",
          "params": {
            "name": {"type": "string", "required": true, "min_length": 3}
          }
        }
      }
    },
    "audit_logs": {
      "enabled": false
    }
  },
  
  "exec_queries": {
    "insert-order": {
      "description": "Insert a new order",
      "sql": "INSERT INTO orders (customer_id, total, status, order_date) VALUES ($1, $2, 'pending', NOW()) RETURNING id",
      "params": {
        "customer_id": {"type": "string", "required": true},
        "total": {"type": "decimal", "required": true}
      }
    }
  }
}
```

### MySQL Example (`services/sql-mysql-analytics/config.json`)

```json
{
  "type": "sql",
  "description": "Analytics MySQL database",
  "version": "1.0",
  
  "connection": {
    "encrypted_dsn": "AES256-GCM:base64...",
    "max_open_conns": 20,
    "max_idle_conns": 10,
    "timeout_seconds": 60
  },
  
  "tables": {
    "reports": {
      "enabled": true,
      "allow_adhoc": true,
      "adhoc_restrictions": {
        "allowed_filters": ["*"],
        "denied_columns": []
      },
      "queries": {
        "daily-summary": {
          "sql": "SELECT report_date, metric_name, value FROM reports WHERE report_date >= ? ORDER BY report_date DESC LIMIT ?",
          "params": {
            "start_date": {"type": "date", "required": true},
            "limit": {"type": "int", "required": false, "default": 100}
          }
        }
      }
    }
  }
}
```

**Note:** MySQL uses `?` placeholders; PostgreSQL uses `$1`, `$2`, etc. The service auto-detects based on the driver.

### SQL Server Example (`services/sql-sqlserver-legacy/config.json`)

```json
{
  "type": "sql",
  "description": "Legacy ERP SQL Server",
  "version": "1.0",
  
  "connection": {
    "encrypted_dsn": "AES256-GCM:base64...",
    "max_open_conns": 5,
    "timeout_seconds": 120
  },
  
  "tables": {
    "invoices": {
      "enabled": true,
      "allow_adhoc": false,
      "queries": {
        "unpaid": {
          "sql": "SELECT invoice_id, customer_name, amount, due_date FROM invoices WHERE paid = 0 AND due_date < @p1",
          "params": {
            "due_date": {"type": "date", "required": true}
          }
        }
      }
    }
  }
}
```

**Note:** SQL Server uses `@p1`, `@p2`, etc. for named parameters.

### API Usage

#### Named Query (like MongoDB)
```http
GET /api/v1/sql-postgres-production/orders?query=recent-pending
Authorization: Bearer <token>
X-Nonce: <server-nonce>
```

#### Named Query with Parameters
```http
GET /api/v1/sql-postgres-production/orders?query=by-customer&customer_id=CUST-12345
Authorization: Bearer <token>
X-Nonce: <server-nonce>
```

#### Ad-hoc Query (where allowed)
```http
GET /api/v1/sql-postgres-production/orders?filter={"order_date":{"$gte":"2026-07-01"},"status":"shipped"}&limit=50
Authorization: Bearer <token>
X-Nonce: <server-nonce>
```

The filter syntax mirrors MongoDB for familiarity — the handler converts it to parameterized SQL.

#### Execute Query (INSERT/UPDATE)
```http
POST /api/v1/sql-postgres-production/exec?query=insert-order
Authorization: Bearer <token>
X-Nonce: <server-nonce>

{
  "customer_id": "CUST-12345",
  "total": 150.00
}
```

Response:
```json
{
  "rows_affected": 1,
  "last_insert_id": 98765,
  "result": [{"id": 98765}]
}
```

### Response Format

```json
{
  "data": [
    {"id": 1001, "customer_id": "CUST-123", "total": 250.00, "status": "pending", "order_date": "2026-07-20"},
    {"id": 1002, "customer_id": "CUST-456", "total": 175.50, "status": "pending", "order_date": "2026-07-19"}
  ],
  "meta": {
    "count": 2,
    "limit": 100,
    "offset": 0,
    "has_more": false,
    "query": "recent-pending"
  }
}
```

### Handler Implementation

```go
// internal/services/sql/handler.go
package sql

import (
    "context"
    "database/sql"
    "encoding/json"
    "fmt"
    "net/http"
    "sync"
    "time"
    
    "github.com/xo/dburl"
    _ "github.com/lib/pq"                    // PostgreSQL
    _ "github.com/go-sql-driver/mysql"       // MySQL
    _ "github.com/microsoft/go-mssqldb"      // SQL Server
    _ "github.com/sijms/go-ora/v2"           // Oracle
    _ "modernc.org/sqlite"                   // SQLite
)

type Service struct {
    name   string
    config Config
    db     *sql.DB
    mu     sync.RWMutex
}

type Config struct {
    Connection  ConnectionConfig         `json:"connection"`
    Limits      LimitsConfig             `json:"limits"`
    Tables      map[string]TableConfig   `json:"tables"`
    ExecQueries map[string]QueryDef      `json:"exec_queries"`
}

type ConnectionConfig struct {
    EncryptedDSN          string `json:"encrypted_dsn"`
    MaxOpenConns          int    `json:"max_open_conns"`
    MaxIdleConns          int    `json:"max_idle_conns"`
    ConnMaxLifetimeMinutes int   `json:"conn_max_lifetime_minutes"`
    TimeoutSeconds        int    `json:"timeout_seconds"`
}

func (s *Service) Initialize(ctx context.Context, masterKey []byte) error {
    // Decrypt DSN
    dsn, err := libsecsecrets.Decrypt(ctx, s.config.Connection.EncryptedDSN, masterKey)
    if err != nil {
        return fmt.Errorf("failed to decrypt DSN: %w", err)
    }
    
    // Open database using dburl
    s.db, err = dburl.Open(dsn)
    if err != nil {
        return fmt.Errorf("failed to open database: %w", err)
    }
    
    // Configure connection pool
    s.db.SetMaxOpenConns(s.config.Connection.MaxOpenConns)
    s.db.SetMaxIdleConns(s.config.Connection.MaxIdleConns)
    if s.config.Connection.ConnMaxLifetimeMinutes > 0 {
        s.db.SetConnMaxLifetime(time.Duration(s.config.Connection.ConnMaxLifetimeMinutes) * time.Minute)
    }
    
    // Verify connection
    if err := s.db.PingContext(ctx); err != nil {
        return fmt.Errorf("database ping failed: %w", err)
    }
    
    return nil
}

func (s *Service) HandleQuery(ctx context.Context, w http.ResponseWriter, r *http.Request, table string) {
    s.mu.RLock()
    tableCfg, exists := s.config.Tables[table]
    cfg := s.config
    s.mu.RUnlock()
    
    if !exists || !tableCfg.Enabled {
        http.Error(w, "table not found or disabled", http.StatusNotFound)
        return
    }
    
    queryName := r.URL.Query().Get("query")
    filterJSON := r.URL.Query().Get("filter")
    
    var rows *sql.Rows
    var err error
    
    if queryName != "" {
        // Named query
        queryDef, exists := tableCfg.Queries[queryName]
        if !exists {
            http.Error(w, "query not found", http.StatusNotFound)
            return
        }
        
        // Build params from query string
        params := s.buildParams(r, queryDef.Params)
        
        ctx, cancel := context.WithTimeout(ctx, time.Duration(cfg.Connection.TimeoutSeconds)*time.Second)
        defer cancel()
        
        rows, err = s.db.QueryContext(ctx, queryDef.SQL, params...)
        
    } else if filterJSON != "" {
        // Ad-hoc query
        if !tableCfg.AllowAdhoc {
            http.Error(w, "ad-hoc queries not allowed", http.StatusBadRequest)
            return
        }
        
        sql, params, err := s.buildAdhocQuery(table, filterJSON, tableCfg.AdhocRestrictions)
        if err != nil {
            http.Error(w, err.Error(), http.StatusBadRequest)
            return
        }
        
        ctx, cancel := context.WithTimeout(ctx, time.Duration(cfg.Connection.TimeoutSeconds)*time.Second)
        defer cancel()
        
        rows, err = s.db.QueryContext(ctx, sql, params...)
    } else {
        http.Error(w, "query or filter required", http.StatusBadRequest)
        return
    }
    
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    defer rows.Close()
    
    // Convert rows to JSON
    results, err := s.rowsToJSON(rows, cfg.Limits.MaxResults)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    
    json.NewEncoder(w).Encode(map[string]interface{}{
        "data": results,
        "meta": map[string]interface{}{
            "count": len(results),
            "query": queryName,
        },
    })
}

func (s *Service) rowsToJSON(rows *sql.Rows, maxResults int) ([]map[string]interface{}, error) {
    columns, err := rows.Columns()
    if err != nil {
        return nil, err
    }
    
    var results []map[string]interface{}
    
    for rows.Next() && len(results) < maxResults {
        values := make([]interface{}, len(columns))
        valuePtrs := make([]interface{}, len(columns))
        for i := range values {
            valuePtrs[i] = &values[i]
        }
        
        if err := rows.Scan(valuePtrs...); err != nil {
            return nil, err
        }
        
        row := make(map[string]interface{})
        for i, col := range columns {
            row[col] = values[i]
        }
        results = append(results, row)
    }
    
    return results, rows.Err()
}

func (s *Service) HealthCheck(ctx context.Context) error {
    return s.db.PingContext(ctx)
}

func (s *Service) Shutdown() {
    if s.db != nil {
        s.db.Close()
    }
}
```

### DSN Encryption

Encrypt the database URL with secretprotector:

```powershell
# PostgreSQL
secretprotector -key-file "D:\secrets\gateway.key" -encrypt "postgres://user:pass@host:5432/dbname?sslmode=require"

# MySQL
secretprotector -key-file "D:\secrets\gateway.key" -encrypt "mysql://user:pass@host:3306/dbname"

# SQL Server
secretprotector -key-file "D:\secrets\gateway.key" -encrypt "sqlserver://user:pass@host:1433/instance/dbname"

# Oracle
secretprotector -key-file "D:\secrets\gateway.key" -encrypt "oracle://user:pass@host:1521/service"
```

### mTLS / SSL Client Certificates (GCP Cloud SQL, etc.)

dburl parses the URL, but the underlying Go drivers handle mTLS natively. For databases requiring client certificates (e.g., GCP Cloud SQL), configure TLS separately:

#### Service Config with mTLS

```json
{
  "type": "sql",
  "description": "GCP Cloud SQL PostgreSQL",
  "version": "1.0",
  
  "connection": {
    "encrypted_dsn": "AES256-GCM:base64...",
    "driver": "postgres",
    
    "tls": {
      "enabled": true,
      "ca_cert": "D:\\certs\\cloudsql\\server-ca.pem",
      "client_cert": "D:\\certs\\cloudsql\\client-cert.pem",
      "client_key": "D:\\certs\\cloudsql\\client-key.pem",
      "server_name": "project:region:instance"
    }
  }
}
```

#### Implementation

The SQL service handler registers custom TLS configs before opening connections:

```go
// internal/services/sql/tls.go

import (
    "crypto/tls"
    "crypto/x509"
    "os"
    
    "github.com/go-sql-driver/mysql"
    "github.com/jackc/pgx/v5"
)

type TLSConfig struct {
    Enabled    bool   `json:"enabled"`
    CACert     string `json:"ca_cert"`
    ClientCert string `json:"client_cert"`
    ClientKey  string `json:"client_key"`
    ServerName string `json:"server_name"`
}

// RegisterMySQLTLS registers a custom TLS config for MySQL
func RegisterMySQLTLS(name string, cfg *TLSConfig) error {
    if !cfg.Enabled {
        return nil
    }
    
    // Load CA cert
    rootCertPool := x509.NewCertPool()
    pem, err := os.ReadFile(cfg.CACert)
    if err != nil {
        return fmt.Errorf("read CA cert: %w", err)
    }
    if !rootCertPool.AppendCertsFromPEM(pem) {
        return fmt.Errorf("failed to append CA cert")
    }
    
    // Load client cert/key pair
    clientCert, err := tls.LoadX509KeyPair(cfg.ClientCert, cfg.ClientKey)
    if err != nil {
        return fmt.Errorf("load client cert: %w", err)
    }
    
    // Register TLS config
    tlsConfig := &tls.Config{
        RootCAs:      rootCertPool,
        Certificates: []tls.Certificate{clientCert},
        ServerName:   cfg.ServerName,
    }
    
    return mysql.RegisterTLSConfig(name, tlsConfig)
}

// BuildPostgresDSN adds SSL params for PostgreSQL
func BuildPostgresDSN(baseDSN string, cfg *TLSConfig) string {
    if !cfg.Enabled {
        return baseDSN
    }
    
    // PostgreSQL uses connection string params
    // postgres://user:pass@host:5432/db?sslmode=verify-full&sslrootcert=...&sslcert=...&sslkey=...
    params := url.Values{}
    params.Set("sslmode", "verify-full")
    params.Set("sslrootcert", cfg.CACert)
    params.Set("sslcert", cfg.ClientCert)
    params.Set("sslkey", cfg.ClientKey)
    
    if strings.Contains(baseDSN, "?") {
        return baseDSN + "&" + params.Encode()
    }
    return baseDSN + "?" + params.Encode()
}
```

#### GCP Cloud SQL Examples

**PostgreSQL (Cloud SQL):**
```json
{
  "connection": {
    "encrypted_dsn": "AES256-GCM:...",
    "tls": {
      "enabled": true,
      "ca_cert": "D:\\certs\\gcp\\server-ca.pem",
      "client_cert": "D:\\certs\\gcp\\client-cert.pem",
      "client_key": "D:\\certs\\gcp\\client-key.pem"
    }
  }
}
```

Resulting connection:
```
postgres://user:pass@10.0.0.5:5432/mydb?sslmode=verify-full&sslrootcert=D:\certs\gcp\server-ca.pem&sslcert=D:\certs\gcp\client-cert.pem&sslkey=D:\certs\gcp\client-key.pem
```

**MySQL (Cloud SQL):**
```json
{
  "connection": {
    "encrypted_dsn": "AES256-GCM:...",
    "tls": {
      "enabled": true,
      "ca_cert": "D:\\certs\\gcp\\server-ca.pem",
      "client_cert": "D:\\certs\\gcp\\client-cert.pem",
      "client_key": "D:\\certs\\gcp\\client-key.pem",
      "server_name": "myproject:us-central1:myinstance"
    }
  }
}
```

Resulting connection uses registered TLS config:
```
mysql://user:pass@10.0.0.5:3306/mydb?tls=sql-mysql-gcp
```

#### Extending for New TLS Requirements

If a driver needs custom TLS handling not covered above:

1. Add new fields to `TLSConfig` struct
2. Implement driver-specific registration in `tls.go`
3. Call registration before `dburl.Open()` in `Initialize()`

The pattern is extensible — dburl handles URL parsing, we handle TLS setup before connection.

### Security Model (Mirrors MongoDB)

| Control | Mechanism |
|---------|-----------|
| **Table whitelist** | Only `enabled: true` tables accessible |
| **Named queries** | Predefined SQL, parameterized |
| **Ad-hoc restrictions** | Required filters, allowed columns, denied columns |
| **Parameterized only** | All queries use placeholders — no SQL injection |
| **Read-only default** | SELECT queries via GET; INSERT/UPDATE via POST with `exec_queries` |
| **Result limits** | `max_results` caps rows returned |
| **Timeout** | Per-query timeout prevents long-running queries |
| **Connection pooling** | Via `database/sql` (SetMaxOpenConns, etc.) |

### Use Cases

1. **PostgreSQL Production** — Orders, customers, inventory
2. **MySQL Analytics** — Reports, metrics, dashboards
3. **SQL Server Legacy** — ERP integration, invoice data
4. **Oracle ERP** — Financial systems, HR data
5. **SQLite Local** — Configuration, local state

---

## High-Performance HTTP Server

The gateway must handle high load from multiple Integration Platform workflows hitting various backend services concurrently.

### Server Configuration

```go
// cmd/gateway/main.go

func createServer(handler http.Handler, cfg *config.ServerConfig) *http.Server {
    return &http.Server{
        Addr:    fmt.Sprintf("127.0.0.1:%d", cfg.Port),
        Handler: handler,
        
        // Connection timeouts
        ReadTimeout:       30 * time.Second,   // Max time to read request
        ReadHeaderTimeout: 10 * time.Second,   // Max time to read headers
        WriteTimeout:      60 * time.Second,   // Max time to write response
        IdleTimeout:       120 * time.Second,  // Keep-alive timeout
        
        // Limits
        MaxHeaderBytes: 1 << 20, // 1MB max header size
        
        // Connection management
        ConnState: func(conn net.Conn, state http.ConnState) {
            // Optional: track connection states for monitoring
            switch state {
            case http.StateNew:
                metrics.ConnectionsTotal.Inc()
            case http.StateClosed:
                metrics.ConnectionsTotal.Dec()
            }
        },
    }
}
```

### Keep-Alive Configuration

```go
// Enable HTTP/1.1 keep-alive (enabled by default in Go)
// Tune for high-throughput scenarios

func createTransport() *http.Transport {
    return &http.Transport{
        // Connection pooling for outbound requests (to backends)
        MaxIdleConns:        100,              // Total across all hosts
        MaxIdleConnsPerHost: 20,               // Per backend host
        MaxConnsPerHost:     50,               // Max concurrent per host
        IdleConnTimeout:     90 * time.Second, // Keep idle connections alive
        
        // Timeouts
        ResponseHeaderTimeout: 30 * time.Second,
        ExpectContinueTimeout: 1 * time.Second,
        
        // TCP settings
        DialContext: (&net.Dialer{
            Timeout:   10 * time.Second,
            KeepAlive: 30 * time.Second,
        }).DialContext,
        
        // TLS (for HTTPS backends)
        TLSHandshakeTimeout: 10 * time.Second,
    }
}
```

### Global Config (`config.json`)

```json
{
  "server": {
    "port": 3000,
    "admin_endpoint": true,
    
    "performance": {
      "read_timeout_seconds": 30,
      "write_timeout_seconds": 60,
      "idle_timeout_seconds": 120,
      "max_header_bytes": 1048576,
      "max_concurrent_requests": 1000
    }
  }
}
```

### Graceful Shutdown

Ensure in-flight requests complete before shutdown:

```go
func (g *Gateway) ListenAndServe(addr string) error {
    server := createServer(g.handler, g.config)
    
    // Channel for shutdown signal
    stop := make(chan os.Signal, 1)
    signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
    
    // Start server in goroutine
    go func() {
        log.Printf("Gateway listening on %s", addr)
        if err := server.ListenAndServe(); err != http.ErrServerClosed {
            log.Fatalf("Server error: %v", err)
        }
    }()
    
    // Wait for shutdown signal
    <-stop
    log.Println("Shutting down gracefully...")
    
    // Give in-flight requests time to complete
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()
    
    if err := server.Shutdown(ctx); err != nil {
        log.Printf("Shutdown error: %v", err)
        return err
    }
    
    // Shutdown all services
    g.registry.ShutdownAll(ctx)
    
    log.Println("Shutdown complete")
    return nil
}
```

### Request Queuing & Backpressure

Prevent overload with request limiting:

```go
// internal/gateway/middleware.go

func MaxConcurrentRequests(max int) func(http.Handler) http.Handler {
    sem := make(chan struct{}, max)
    
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            select {
            case sem <- struct{}{}:
                defer func() { <-sem }()
                next.ServeHTTP(w, r)
            default:
                // Queue full - reject with 503
                w.Header().Set("Retry-After", "5")
                http.Error(w, "Server too busy", http.StatusServiceUnavailable)
            }
        })
    }
}

// Usage in router setup
handler := MaxConcurrentRequests(cfg.Performance.MaxConcurrentRequests)(router)
```

### Health Check Under Load

The `/health` endpoint must remain responsive even under heavy load:

```go
// Health check bypasses request limiting
func (g *Gateway) setupRoutes() {
    // Health check - no auth, no rate limiting, lightweight
    g.router.HandleFunc("/health", g.handleHealth).Methods("GET")
    
    // All other routes go through middleware
    api := g.router.PathPrefix("/api").Subrouter()
    api.Use(g.authMiddleware)
    api.Use(g.rateLimitMiddleware)
    // ...
}

func (g *Gateway) handleHealth(w http.ResponseWriter, r *http.Request) {
    // Fast path - just check if services are registered
    // Don't ping backends on every health check
    status := map[string]interface{}{
        "status":  "ok",
        "version": Version,
        "uptime":  time.Since(g.startTime).String(),
    }
    
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(status)
}
```

### Performance Tuning Checklist

| Setting | Recommended | Why |
|---------|-------------|-----|
| `GOMAXPROCS` | CPU cores | Default is correct |
| `idle_timeout` | 120s | Match nginx keepalive |
| `max_concurrent_requests` | 1000 | Prevent memory exhaustion |
| `connection pool per backend` | 20-50 | Balance load vs resources |
| Graceful shutdown timeout | 30s | Allow long queries to finish |

### Monitoring Metrics

Track these for performance visibility:

```go
// internal/metrics/metrics.go

var (
    RequestsTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "gateway_requests_total",
            Help: "Total requests by service and status",
        },
        []string{"service", "status"},
    )
    
    RequestDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "gateway_request_duration_seconds",
            Help:    "Request duration by service",
            Buckets: []float64{.01, .05, .1, .25, .5, 1, 2.5, 5, 10},
        },
        []string{"service"},
    )
    
    ConnectionsActive = prometheus.NewGauge(
        prometheus.GaugeOpts{
            Name: "gateway_connections_active",
            Help: "Active HTTP connections",
        },
    )
    
    BackendPoolSize = prometheus.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "gateway_backend_pool_size",
            Help: "Backend connection pool size",
        },
        []string{"service"},
    )
)
```

---

## Bootstrap Sequence

```go
func main() {
    ctx := context.Background()
    
    // 1. Parse flags
    configPath := flag.String("config", "config.json", "Path to config file")
    reload := flag.Bool("reload", false, "Signal running gateway to reload")
    flag.Parse()
    
    // Handle -reload flag
    if *reload {
        signalReload()
        return
    }
    
    // 2. Load global configuration
    cfg, err := config.Load(*configPath)
    if err != nil {
        log.Fatalf("Failed to load config: %v", err)
    }
    
    // 3. Resolve master key
    masterKey, err := libsecsecrets.ResolveKey(ctx, "", cfg.MasterKeyEnv, cfg.MasterKeyPath)
    if err != nil {
        log.Fatalf("Failed to resolve master key: %v", err)
    }
    defer libsecsecrets.ZeroBuffer(masterKey)
    
    // 4. Decrypt all API keys
    apiKeys := make(map[string]*auth.APIKey)
    for name, keyCfg := range cfg.Auth.Keys {
        secret, err := libsecsecrets.Decrypt(ctx, keyCfg.EncryptedSecret, masterKey)
        if err != nil {
            log.Fatalf("Failed to decrypt API key %s: %v", name, err)
        }
        apiKeys[name] = &auth.APIKey{
            Name:            name,
            Secret:          secret,
            AllowedServices: keyCfg.Services,
            MaxConcurrent:   keyCfg.MaxConcurrent,
            RateLimitRPM:    keyCfg.RateLimitRPM,
        }
    }
    
    // 5. Initialize nonce store
    nonceStore := auth.NewNonceStore(5 * time.Minute)
    defer nonceStore.Stop()
    
    // 6. Initialize service registry
    registry := services.NewRegistry()
    
    // 7. Load enabled services from services/ directory
    for name, svcCfg := range cfg.Services {
        if !svcCfg.Enabled {
            continue
        }
        svcConfigPath := filepath.Join(cfg.ServicesDir, name, "config.json")
        svc, err := services.Load(ctx, name, svcConfigPath, masterKey)
        if err != nil {
            log.Fatalf("Failed to load service %s: %v", name, err)
        }
        registry.Register(svc)
    }
    
    // 8. Build gateway
    gw := gateway.New(cfg, registry, apiKeys, nonceStore, masterKey)
    
    // 9. Setup signal handlers (SIGHUP for reload, SIGTERM for shutdown)
    gw.SetupSignalHandlers()
    
    // 10. Start server (localhost only - hardcoded for security)
    addr := fmt.Sprintf("127.0.0.1:%d", cfg.Server.Port)
    log.Printf("Starting gateway on %s", addr)
    
    if err := gw.ListenAndServe(addr); err != http.ErrServerClosed {
        log.Fatalf("Server error: %v", err)
    }
}
```

---

## Error Handling

### Standard Error Response

```json
{
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "Invalid filter JSON",
    "details": "unexpected end of JSON input"
  }
}
```

### Error Codes

| Code | HTTP Status | Description |
|------|-------------|-------------|
| `UNAUTHORIZED` | 401 | Invalid or missing bearer token |
| `INVALID_NONCE` | 401 | Nonce missing, expired, or already used |
| `FORBIDDEN` | 403 | No access to this service |
| `NOT_FOUND` | 404 | Service or resource not found |
| `VALIDATION_ERROR` | 400 | Invalid request parameters |
| `QUERY_NOT_FOUND` | 400 | Named query does not exist |
| `ADHOC_NOT_ALLOWED` | 400 | Ad-hoc queries disabled for this collection |
| `FILTER_FIELD_NOT_ALLOWED` | 400 | Filter uses disallowed field |
| `RATE_LIMIT_EXCEEDED` | 429 | Too many requests |
| `TOO_MANY_CONCURRENT` | 429 | Max concurrent requests reached |
| `SERVICE_ERROR` | 500 | Backend service error |
| `RESPONSE_TOO_LARGE` | 500 | Response exceeds size limit |
| `TIMEOUT` | 504 | Request timed out |

---

## Logging

### Log Format

```json
{
  "timestamp": "2024-01-15T10:30:00Z",
  "level": "info",
  "service": "mongodb-production",
  "collection": "orders",
  "query": "recent-pending",
  "api_key": "ga-production",
  "method": "GET",
  "path": "/api/v1/mongodb-production/orders",
  "status": 200,
  "duration_ms": 45,
  "result_count": 100
}
```

### Log Levels

- **error** — Failed requests, service errors
- **warn** — Invalid auth attempts, deprecated usage
- **info** — Successful requests, service lifecycle, config reloads
- **debug** — Request details (disabled in production)

### Security: Never Log

- Bearer tokens
- Decrypted secrets
- Full query results
- Nonce values
- Connection URIs

---

## Health Checks

### Endpoint

```
GET /health
```

### Response

```json
{
  "status": "ok",
  "version": "1.0.0",
  "uptime_seconds": 3600,
  "config_loaded_at": "2024-01-15T10:00:00Z",
  "last_reload_at": "2024-01-15T14:30:00Z",
  "services": {
    "mongodb-production": {"status": "ok", "latency_ms": 5, "version": "1.3"},
    "mongodb-analytics": {"status": "ok", "latency_ms": 8, "version": "1.1"},
    "pdf-extractor-invoices": {"status": "ok", "version": "1.0"},
    "rclone-gcs-archive": {"status": "ok", "remotes": 3, "version": "1.0"}
  }
}
```

---

## Client Integration

The gateway uses standard HTTP headers and JSON — it works with **any REST client**.

### Required Headers

| Header | Value | When Required |
|--------|-------|---------------|
| `Authorization` | `Bearer <api_key>` | All authenticated endpoints |
| `X-Nonce` | `<uuid>` | All `/api/v1/{service}/*` endpoints |
| `Content-Type` | `application/json` | POST/PUT requests with body |

### curl Examples

```bash
# Store API key
API_KEY="your-shared-secret"
BASE_URL="http://127.0.0.1:3000"

# Step 1: Get nonce
NONCE=$(curl -s -H "Authorization: Bearer $API_KEY" \
  "$BASE_URL/api/v1/auth/nonce" | jq -r '.nonce')

# Step 2: Query with named query
curl -H "Authorization: Bearer $API_KEY" \
     -H "X-Nonce: $NONCE" \
     "$BASE_URL/api/v1/mongodb-production/orders?query=recent-pending"

# Trigger config reload
curl http://127.0.0.1:3000/_admin/reload
```

### PowerShell Examples

```powershell
$ApiKey = "your-shared-secret"
$BaseUrl = "http://127.0.0.1:3000"
$Headers = @{ "Authorization" = "Bearer $ApiKey" }

# Step 1: Get nonce
$NonceResponse = Invoke-RestMethod -Uri "$BaseUrl/api/v1/auth/nonce" -Headers $Headers
$Nonce = $NonceResponse.nonce

# Step 2: Query MongoDB
$Headers["X-Nonce"] = $Nonce
$Orders = Invoke-RestMethod -Uri "$BaseUrl/api/v1/mongodb-production/orders?query=recent-pending" -Headers $Headers

# Trigger reload
Invoke-RestMethod -Uri "$BaseUrl/_admin/reload"
```

### Integration Platform Workflow

```
Project: MongoDB Query Workflow
├── Task 1: HTTP GET
│   ├── URL: http://127.0.0.1:3000/api/v1/auth/nonce
│   ├── Headers:
│   │   └── Authorization: Bearer ${encrypted_api_key}
│   └── Output Variable: ${nonce_response}
│
├── Task 2: Set Variable
│   └── ${nonce} = JSONPath(${nonce_response}, "$.nonce")
│
├── Task 3: HTTP GET
│   ├── URL: http://127.0.0.1:3000/api/v1/mongodb-production/orders?query=recent-pending
│   ├── Headers:
│   │   ├── Authorization: Bearer ${encrypted_api_key}
│   │   └── X-Nonce: ${nonce}
│   └── Output Variable: ${orders_response}
│
└── Task 4: Process Results
    └── ... (write to file, transform, etc.)
```

---

## Network Security

### Localhost-Only Binding (Mandatory)

The gateway **always** binds to `127.0.0.1`. This is hardcoded, not configurable.

```go
// This is NOT configurable - security by design
const bindAddress = "127.0.0.1"

addr := fmt.Sprintf("%s:%d", bindAddress, cfg.Server.Port)
```

**Rationale:**
- Gateway handles sensitive MongoDB credentials and API keys
- Direct network exposure would bypass SSL/TLS
- Forces proper architecture: reverse proxy handles external access
- Prevents accidental exposure via misconfiguration

### Reverse Proxy Architecture

```mermaid
flowchart LR
    Clients[External<br/>Clients]
    
    subgraph Proxy[nginx / IIS]
        SSL[SSL Termination]
    end
    
    subgraph Localhost[127.0.0.1]
        GW[Service Gateway<br/>localhost only]
    end
    
    Clients -->|HTTPS 443| SSL
    SSL -->|HTTP 3000| GW
```

### nginx Configuration

```nginx
# D:\data\nginx\conf\service-gateway.conf

upstream service_gateway {
    server 127.0.0.1:3000;
    keepalive 32;
}

server {
    listen 443 ssl http2;
    server_name gateway.internal.example.com;

    ssl_certificate     D:/data/nginx/ssl/gateway.crt;
    ssl_certificate_key D:/data/nginx/ssl/gateway.key;
    ssl_protocols       TLSv1.2 TLSv1.3;
    ssl_ciphers         ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256:ECDHE-ECDSA-AES256-GCM-SHA384:ECDHE-RSA-AES256-GCM-SHA384;
    ssl_prefer_server_ciphers on;

    add_header Strict-Transport-Security "max-age=31536000; includeSubDomains" always;
    add_header X-Content-Type-Options "nosniff" always;

    location / {
        proxy_pass http://service_gateway;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header Connection "";
    }
    
    # Block admin endpoint from external access
    location /_admin/ {
        deny all;
    }
}
```

---

## Testing Strategy

### Testing Levels

| Level | Scope | Data | Automation |
|-------|-------|------|------------|
| **Unit Tests** | Individual functions, handlers | Mocks | `go test` |
| **Integration Tests** | Service + real backend | Test instances | `go test -tags=integration` |
| **Real Data Tests** | Full stack + production-like data | Sanitized production data | `go test -tags=realdata` |

### Unit Tests

Every package must have `*_test.go` files with:

```go
// internal/auth/middleware_test.go
func TestAuthMiddleware_ValidToken(t *testing.T) {
    // Mock dependencies
    keys := map[string]*APIKey{
        "test-key": {Name: "test", Secret: "secret123", AllowedServices: []string{"*"}},
    }
    nonceStore := NewMockNonceStore()
    
    // Test valid auth
    req := httptest.NewRequest("GET", "/api/v1/test/resource", nil)
    req.Header.Set("Authorization", "Bearer secret123")
    req.Header.Set("X-Nonce", "valid-nonce")
    
    // Assert
    // ...
}

func TestAuthMiddleware_InvalidToken(t *testing.T) { ... }
func TestAuthMiddleware_ExpiredNonce(t *testing.T) { ... }
func TestAuthMiddleware_RBACDenied(t *testing.T) { ... }
```

**Coverage target:** 80% for all packages.

### Integration Tests

Tests against real service backends (test instances, not production):

```go
//go:build integration

// internal/services/mongodb/handler_integration_test.go
package mongodb

import (
    "context"
    "os"
    "testing"
)

func TestMongoDBService_RealConnection(t *testing.T) {
    // Skip if no test MongoDB available
    mongoURI := os.Getenv("TEST_MONGODB_URI")
    if mongoURI == "" {
        t.Skip("TEST_MONGODB_URI not set")
    }
    
    svc := NewService("test-mongo")
    err := svc.Initialize(context.Background(), testMasterKey)
    if err != nil {
        t.Fatalf("Initialize failed: %v", err)
    }
    defer svc.Shutdown()
    
    // Test real query
    // ...
}
```

**Integration test backends:**

| Service | Test Backend | Environment Variable |
|---------|--------------|---------------------|
| `mongodb` | Local MongoDB or Atlas test cluster | `TEST_MONGODB_URI` |
| `sql` (PostgreSQL) | Local PostgreSQL or test instance | `TEST_POSTGRES_DSN` |
| `sql` (MySQL) | Local MySQL or test instance | `TEST_MYSQL_DSN` |
| `sql` (SQL Server) | Local SQL Server or test instance | `TEST_SQLSERVER_DSN` |
| `rclone` | Local filesystem remotes | `TEST_RCLONE_CONFIG` |
| `o365mbx` | Test mailbox (non-production) | `TEST_O365_TOKEN` |

### Real Data Tests

Tests with sanitized production data to catch edge cases that synthetic data misses:

```go
//go:build realdata

// tests/realdata/mongodb_realdata_test.go
package realdata

import (
    "encoding/json"
    "os"
    "path/filepath"
    "testing"
)

func TestMongoDBQueries_RealData(t *testing.T) {
    // Load sanitized production data
    testDataDir := os.Getenv("REALDATA_DIR")
    if testDataDir == "" {
        t.Skip("REALDATA_DIR not set")
    }
    
    // Test cases from real scenarios
    testCases := loadTestCases(filepath.Join(testDataDir, "mongodb", "orders"))
    
    for _, tc := range testCases {
        t.Run(tc.Name, func(t *testing.T) {
            // Insert test data
            // Execute query
            // Verify expected results
        })
    }
}
```

**Real data test structure:**

```
testdata/
├── realdata/
│   ├── README.md                    # Data sanitization process
│   ├── mongodb/
│   │   ├── orders/
│   │   │   ├── testcase_001.json    # Input data + expected output
│   │   │   ├── testcase_002.json
│   │   │   └── edge_cases.json      # Known edge cases from production
│   │   └── customers/
│   │       └── ...
│   ├── sql/
│   │   ├── postgres/
│   │   └── sqlserver/
│   ├── rclone/
│   │   └── file_transfer_scenarios/
│   ├── pdf-extractor/
│   │   ├── invoices/                # Real PDFs (sanitized)
│   │   └── edge_cases/              # Problematic PDFs from production
│   └── o365mbx/
│       └── email_scenarios/
```

### Test Data Sanitization

Real data MUST be sanitized before use in tests:

```go
// tools/sanitize/sanitize.go

// SanitizeRecord removes/replaces PII and sensitive data
func SanitizeRecord(record map[string]interface{}) map[string]interface{} {
    sanitized := make(map[string]interface{})
    for k, v := range record {
        switch k {
        case "email":
            sanitized[k] = faker.Email()
        case "name", "customer_name":
            sanitized[k] = faker.Name()
        case "phone", "mobile":
            sanitized[k] = faker.Phone()
        case "ssn", "tax_id":
            sanitized[k] = "XXX-XX-XXXX"
        case "credit_card", "card_number":
            sanitized[k] = "XXXX-XXXX-XXXX-XXXX"
        case "password", "secret":
            continue // Remove entirely
        default:
            sanitized[k] = v
        }
    }
    return sanitized
}
```

### Test Execution

```bash
# Unit tests only (fast, no external dependencies)
go test ./...

# Unit + integration tests
go test -tags=integration ./...

# Full test suite including real data
go test -tags=integration,realdata ./...

# With coverage
go test -tags=integration -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

### Continuous Testing Checklist

Before each release:

- [ ] Unit tests pass (`go test ./...`)
- [ ] Integration tests pass (`go test -tags=integration ./...`)
- [ ] Real data tests pass (`go test -tags=realdata ./...`)
- [ ] Coverage ≥ 80%
- [ ] No race conditions (`go test -race ./...`)
- [ ] Security audit (`gosec ./...`)
- [ ] Linting clean (`golangci-lint run`)

---

## API Versioning

### Version Strategy

The API uses **URL path versioning**: `/api/v1/`, `/api/v2/`, etc.

| Version | Status | Support |
|---------|--------|---------|
| `v1` | Current | Full support |
| `v2` | Future | When breaking changes needed |

### What Constitutes a Breaking Change

| Change Type | Breaking? | Action |
|-------------|-----------|--------|
| Add new endpoint | No | Add to current version |
| Add optional parameter | No | Add to current version |
| Add field to response | No | Add to current version |
| Remove endpoint | **Yes** | New version |
| Remove/rename parameter | **Yes** | New version |
| Remove field from response | **Yes** | New version |
| Change field type | **Yes** | New version |
| Change authentication | **Yes** | New version |
| Change error format | **Yes** | New version |

### Version Lifecycle

```
v1 (current)     ──────────────────────────────────────────►
                              │
                              │ Breaking change needed
                              ▼
v2 (new)                      ├──────────────────────────────►
                              │
                              │ Deprecation period (6 months)
                              ▼
v1 (deprecated)  ─────────────┼─────────────────────────────► (read-only)
                              │
                              │ End of life
                              ▼
v1 (removed)     ─────────────┴─────────────────────────────X
```

### Implementation

```go
// internal/gateway/router.go

func (g *Gateway) setupRoutes() {
    // Version 1 routes
    v1 := g.router.PathPrefix("/api/v1").Subrouter()
    v1.Use(g.authMiddleware)
    v1.HandleFunc("/auth/nonce", g.handleNonce)
    v1.PathPrefix("/{service}/").HandlerFunc(g.handleService)
    
    // Version 2 routes (when needed)
    // v2 := g.router.PathPrefix("/api/v2").Subrouter()
    // v2.Use(g.authMiddlewareV2)
    // ...
}
```

### Deprecation Notice

When deprecating a version, add header to all responses:

```go
func DeprecationMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if strings.HasPrefix(r.URL.Path, "/api/v1") {
            w.Header().Set("Deprecation", "true")
            w.Header().Set("Sunset", "2027-01-01T00:00:00Z")
            w.Header().Set("Link", "</api/v2>; rel=\"successor-version\"")
        }
        next.ServeHTTP(w, r)
    })
}
```

### Client Migration Guide

When releasing a new version, provide:

1. **Changelog** — What changed and why
2. **Migration guide** — Step-by-step upgrade instructions
3. **Compatibility matrix** — Which clients work with which versions
4. **Timeline** — Deprecation and sunset dates

---

## Migration

### Service Configuration Migration

When config schema changes, provide automated migration:

```go
// internal/config/migrate.go

type Migration struct {
    FromVersion string
    ToVersion   string
    Migrate     func(cfg json.RawMessage) (json.RawMessage, error)
}

var migrations = []Migration{
    {
        FromVersion: "1.0",
        ToVersion:   "1.1",
        Migrate: func(cfg json.RawMessage) (json.RawMessage, error) {
            // Example: rename field
            var v map[string]interface{}
            json.Unmarshal(cfg, &v)
            
            if settings, ok := v["settings"].(map[string]interface{}); ok {
                // Rename pool_size → max_open_conns
                if poolSize, exists := settings["pool_size"]; exists {
                    settings["max_open_conns"] = poolSize
                    delete(settings, "pool_size")
                }
            }
            
            v["version"] = "1.1"
            return json.Marshal(v)
        },
    },
}

func MigrateConfig(cfg json.RawMessage, targetVersion string) (json.RawMessage, error) {
    var v struct {
        Version string `json:"version"`
    }
    json.Unmarshal(cfg, &v)
    currentVersion := v.Version
    
    for _, m := range migrations {
        if m.FromVersion == currentVersion {
            var err error
            cfg, err = m.Migrate(cfg)
            if err != nil {
                return nil, err
            }
            currentVersion = m.ToVersion
        }
        if currentVersion == targetVersion {
            break
        }
    }
    
    return cfg, nil
}
```

### CLI Migration Tool

```powershell
# Check if migration needed
gateway.exe -migrate-check -config config.json

# Output:
# Config version: 1.0
# Target version: 1.2
# Migrations needed:
#   1.0 → 1.1: Rename pool_size to max_open_conns
#   1.1 → 1.2: Add streaming configuration

# Perform migration (backup created automatically)
gateway.exe -migrate -config config.json

# Output:
# Backup created: config.json.bak.20260722_143052
# Migrating 1.0 → 1.1... done
# Migrating 1.1 → 1.2... done
# Config updated to version 1.2
```

### Service Data Migration

For migrations affecting stored data (rare):

```go
// Example: MongoDB index migration
func (s *MongoDBService) MigrateIndexes(ctx context.Context) error {
    // Check current index version
    currentVersion := s.getIndexVersion(ctx)
    
    if currentVersion < 2 {
        // Add new index for improved query performance
        _, err := s.collection.Indexes().CreateOne(ctx, mongo.IndexModel{
            Keys: bson.D{{Key: "order_date", Value: -1}, {Key: "status", Value: 1}},
        })
        if err != nil {
            return err
        }
        s.setIndexVersion(ctx, 2)
    }
    
    return nil
}
```

### Zero-Downtime Migration Strategy

For production deployments:

```
┌─────────────────────────────────────────────────────────────┐
│                   Zero-Downtime Migration                   │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  1. Deploy new version alongside old (different port)      │
│     ┌──────────┐  ┌──────────┐                             │
│     │ Gateway  │  │ Gateway  │                             │
│     │ v1.0     │  │ v1.1     │                             │
│     │ :3000    │  │ :3001    │                             │
│     └──────────┘  └──────────┘                             │
│                                                             │
│  2. Run smoke tests against new version                     │
│     curl http://127.0.0.1:3001/health                      │
│                                                             │
│  3. Switch traffic (nginx upstream)                         │
│     upstream service_gateway {                              │
│         server 127.0.0.1:3001;  # New                      │
│         # server 127.0.0.1:3000;  # Old (commented)        │
│     }                                                       │
│     nginx -s reload                                         │
│                                                             │
│  4. Monitor for errors                                      │
│                                                             │
│  5. If OK: stop old version                                 │
│     If errors: rollback nginx config, investigate          │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

### Rollback Procedure

```powershell
# 1. Stop new gateway
nssm stop ServiceGateway

# 2. Restore config backup
Copy-Item config.json.bak.20260722_143052 config.json

# 3. Restore binary
Copy-Item gateway.exe.bak gateway.exe

# 4. Start old gateway
nssm start ServiceGateway

# 5. Verify
curl http://127.0.0.1:3000/health
```

### Migration Checklist

Before any migration:

- [ ] Backup all configs (`config.json`, `services/*/config.json`)
- [ ] Backup gateway binary
- [ ] Test migration in staging environment
- [ ] Prepare rollback procedure
- [ ] Schedule maintenance window (if needed)
- [ ] Notify dependent teams (Integration Platform workflows)

After migration:

- [ ] Verify all services healthy (`/health`)
- [ ] Test critical workflows
- [ ] Monitor logs for errors
- [ ] Keep backups for 7 days

---

## Cross-Platform Support

### Design Principle

All code MUST work seamlessly on Windows and Linux. Use build tags for platform-specific optimizations.

```
internal/
├── platform/
│   ├── platform.go          # Common interface
│   ├── platform_windows.go  # Windows-specific (//go:build windows)
│   └── platform_unix.go     # Linux/Unix-specific (//go:build !windows)
```

### Platform Abstraction

```go
// internal/platform/platform.go

package platform

// Platform provides OS-specific functionality
type Platform interface {
    // Signal handling
    SetupReloadSignal() <-chan os.Signal
    
    // File permissions
    SecureFilePermissions(path string) error
    ValidateKeyFilePermissions(path string) error
    
    // Process management
    SetProcessPriority(priority int) error
    SetResourceLimits(maxFiles, maxMemoryMB int) error
    
    // Security
    DropPrivileges(user, group string) error
    LockMemory() error  // Prevent swapping sensitive data
}

// Current returns the platform implementation for the current OS
func Current() Platform {
    return currentPlatform
}
```

### Windows-Specific (`platform_windows.go`)

```go
//go:build windows

package platform

import (
    "os"
    "syscall"
    
    "golang.org/x/sys/windows"
)

type windowsPlatform struct{}

var currentPlatform Platform = &windowsPlatform{}

func (p *windowsPlatform) SetupReloadSignal() <-chan os.Signal {
    // Windows: no SIGHUP, use named pipe or admin endpoint
    // Return nil channel - reload via HTTP endpoint only
    return nil
}

func (p *windowsPlatform) SecureFilePermissions(path string) error {
    // Windows: Set ACL to owner-only
    // Using icacls or Windows API
    return setWindowsACL(path, windows.GENERIC_READ, ownerOnly)
}

func (p *windowsPlatform) ValidateKeyFilePermissions(path string) error {
    // Windows: Check ACL allows only current user/admin
    return validateWindowsACL(path)
}

func (p *windowsPlatform) SetProcessPriority(priority int) error {
    handle := windows.CurrentProcess()
    var class uint32
    switch {
    case priority < -10:
        class = windows.HIGH_PRIORITY_CLASS
    case priority < 0:
        class = windows.ABOVE_NORMAL_PRIORITY_CLASS
    case priority == 0:
        class = windows.NORMAL_PRIORITY_CLASS
    default:
        class = windows.BELOW_NORMAL_PRIORITY_CLASS
    }
    return windows.SetPriorityClass(handle, class)
}

func (p *windowsPlatform) SetResourceLimits(maxFiles, maxMemoryMB int) error {
    // Windows: Job objects for resource limits
    job, err := windows.CreateJobObject(nil, nil)
    if err != nil {
        return err
    }
    
    info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
        BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{
            LimitFlags: windows.JOB_OBJECT_LIMIT_PROCESS_MEMORY,
        },
        ProcessMemoryLimit: uintptr(maxMemoryMB * 1024 * 1024),
    }
    
    return windows.SetInformationJobObject(job,
        windows.JobObjectExtendedLimitInformation,
        uintptr(unsafe.Pointer(&info)),
        uint32(unsafe.Sizeof(info)))
}

func (p *windowsPlatform) DropPrivileges(user, group string) error {
    // Windows: Run as service account (configured via nssm/SCM)
    // Cannot drop privileges at runtime like Unix
    return nil
}

func (p *windowsPlatform) LockMemory() error {
    // Windows: VirtualLock to prevent swapping sensitive pages
    // Applied to specific buffers containing secrets
    return nil
}

// Windows-specific: Named pipe for reload signal
func SetupReloadPipe(pipeName string) (<-chan struct{}, error) {
    reload := make(chan struct{})
    
    go func() {
        for {
            pipe, err := windows.CreateNamedPipe(
                syscall.StringToUTF16Ptr(`\\.\pipe\`+pipeName),
                windows.PIPE_ACCESS_INBOUND,
                windows.PIPE_TYPE_BYTE,
                1, 0, 0, 0, nil)
            if err != nil {
                continue
            }
            
            windows.ConnectNamedPipe(pipe, nil)
            windows.CloseHandle(pipe)
            reload <- struct{}{}
        }
    }()
    
    return reload, nil
}
```

### Linux/Unix-Specific (`platform_unix.go`)

```go
//go:build !windows

package platform

import (
    "os"
    "os/signal"
    "syscall"
    
    "golang.org/x/sys/unix"
)

type unixPlatform struct{}

var currentPlatform Platform = &unixPlatform{}

func (p *unixPlatform) SetupReloadSignal() <-chan os.Signal {
    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, syscall.SIGHUP)
    return sigCh
}

func (p *unixPlatform) SecureFilePermissions(path string) error {
    // Unix: chmod 0600 (owner read/write only)
    return os.Chmod(path, 0600)
}

func (p *unixPlatform) ValidateKeyFilePermissions(path string) error {
    info, err := os.Stat(path)
    if err != nil {
        return err
    }
    
    mode := info.Mode().Perm()
    if mode&0077 != 0 {
        return fmt.Errorf("key file %s has insecure permissions %o (want 0600 or 0400)", path, mode)
    }
    return nil
}

func (p *unixPlatform) SetProcessPriority(priority int) error {
    return unix.Setpriority(unix.PRIO_PROCESS, 0, priority)
}

func (p *unixPlatform) SetResourceLimits(maxFiles, maxMemoryMB int) error {
    // Set file descriptor limit
    var rLimit unix.Rlimit
    rLimit.Max = uint64(maxFiles)
    rLimit.Cur = uint64(maxFiles)
    if err := unix.Setrlimit(unix.RLIMIT_NOFILE, &rLimit); err != nil {
        return fmt.Errorf("set RLIMIT_NOFILE: %w", err)
    }
    
    // Set memory limit
    rLimit.Max = uint64(maxMemoryMB * 1024 * 1024)
    rLimit.Cur = rLimit.Max
    if err := unix.Setrlimit(unix.RLIMIT_AS, &rLimit); err != nil {
        return fmt.Errorf("set RLIMIT_AS: %w", err)
    }
    
    return nil
}

func (p *unixPlatform) DropPrivileges(user, group string) error {
    // Look up user/group
    u, err := osuser.Lookup(user)
    if err != nil {
        return err
    }
    uid, _ := strconv.Atoi(u.Uid)
    gid, _ := strconv.Atoi(u.Gid)
    
    // Drop group first, then user
    if err := unix.Setgid(gid); err != nil {
        return err
    }
    return unix.Setuid(uid)
}

func (p *unixPlatform) LockMemory() error {
    // Unix: mlockall to prevent swapping
    return unix.Mlockall(unix.MCL_CURRENT | unix.MCL_FUTURE)
}
```

### Platform-Specific Optimizations

| Feature | Windows | Linux |
|---------|---------|-------|
| **Reload signal** | Named pipe or HTTP `/_admin/reload` | `SIGHUP` |
| **File permissions** | ACLs via Windows API | `chmod 0600` |
| **Service management** | NSSM / Windows SCM | systemd |
| **Priority boost** | `SetPriorityClass(HIGH)` | `nice -10` |
| **Memory locking** | `VirtualLock()` on secret buffers | `mlockall()` |
| **File descriptors** | Handle limits via Job Objects | `RLIMIT_NOFILE` |
| **Drop privileges** | Run as service account | `setuid()/setgid()` |

### Build Configuration

```go
// cmd/gateway/main.go

func main() {
    // Platform-specific initialization
    plat := platform.Current()
    
    // Lock memory to prevent secrets from being swapped (optional)
    if cfg.Security.LockMemory {
        if err := plat.LockMemory(); err != nil {
            log.Printf("Warning: could not lock memory: %v", err)
        }
    }
    
    // Set resource limits
    if cfg.Performance.MaxOpenFiles > 0 {
        plat.SetResourceLimits(cfg.Performance.MaxOpenFiles, cfg.Performance.MaxMemoryMB)
    }
    
    // Setup reload signal (platform-specific)
    if reloadCh := plat.SetupReloadSignal(); reloadCh != nil {
        go func() {
            for range reloadCh {
                gateway.Reload()
            }
        }()
    }
    
    // Drop privileges after binding port (Linux only, requires root start)
    if cfg.Security.RunAsUser != "" {
        if err := plat.DropPrivileges(cfg.Security.RunAsUser, cfg.Security.RunAsGroup); err != nil {
            log.Fatalf("Failed to drop privileges: %v", err)
        }
    }
}
```

### Global Config — Platform Section

```json
{
  "server": {
    "port": 3000
  },
  
  "performance": {
    "max_open_files": 65536,
    "max_memory_mb": 4096,
    "process_priority": -5
  },
  
  "security": {
    "lock_memory": true,
    "run_as_user": "gateway",
    "run_as_group": "gateway"
  }
}
```

### Path Handling

Always use `filepath` for cross-platform path handling:

```go
import "path/filepath"

// CORRECT - works on both platforms
configPath := filepath.Join(baseDir, "services", name, "config.json")

// WRONG - Unix-only
configPath := baseDir + "/services/" + name + "/config.json"
```

### Testing Cross-Platform

```bash
# Build for both platforms
GOOS=windows GOARCH=amd64 go build -o gateway.exe ./cmd/gateway
GOOS=linux GOARCH=amd64 go build -o gateway ./cmd/gateway

# Run platform-specific tests
go test -tags=windows ./internal/platform/...
go test -tags=!windows ./internal/platform/...
```

---

## Observability

### Health Endpoints

```go
// internal/gateway/health.go

package gateway

type HealthStatus struct {
    Status    string                    `json:"status"`
    Timestamp time.Time                 `json:"timestamp"`
    Version   string                    `json:"version"`
    Uptime    string                    `json:"uptime"`
    Checks    map[string]ComponentCheck `json:"checks,omitempty"`
}

type ComponentCheck struct {
    Status   string `json:"status"`
    Latency  string `json:"latency,omitempty"`
    Error    string `json:"error,omitempty"`
}
```

#### Liveness Probe — `GET /health`

Returns 200 if process is running. No backend checks — fast and always available.

```http
GET /health
```

Response:
```json
{
  "status": "ok",
  "timestamp": "2026-07-22T10:30:00Z",
  "version": "1.2.0",
  "uptime": "48h12m30s"
}
```

#### Readiness Probe — `GET /ready`

Returns 200 only if all configured backends are reachable. Use for load balancer health checks.

```http
GET /ready
Authorization: Bearer <token>
```

Response (all healthy):
```json
{
  "status": "ok",
  "timestamp": "2026-07-22T10:30:00Z",
  "checks": {
    "mongodb-production": {"status": "ok", "latency": "2ms"},
    "sql-postgres-main": {"status": "ok", "latency": "5ms"},
    "ldap-corp-ad": {"status": "ok", "latency": "12ms"},
    "mq-kafka-orders": {"status": "ok", "latency": "8ms"}
  }
}
```

Response (degraded — HTTP 503):
```json
{
  "status": "degraded",
  "timestamp": "2026-07-22T10:30:00Z",
  "checks": {
    "mongodb-production": {"status": "ok", "latency": "2ms"},
    "sql-postgres-main": {"status": "error", "error": "connection refused"},
    "ldap-corp-ad": {"status": "ok", "latency": "12ms"}
  }
}
```

#### Implementation

```go
func (g *Gateway) handleHealth(w http.ResponseWriter, r *http.Request) {
    json.NewEncoder(w).Encode(HealthStatus{
        Status:    "ok",
        Timestamp: time.Now().UTC(),
        Version:   version.Version,
        Uptime:    time.Since(g.startTime).Round(time.Second).String(),
    })
}

func (g *Gateway) handleReady(w http.ResponseWriter, r *http.Request) {
    checks := make(map[string]ComponentCheck)
    allOK := true
    
    for name, svc := range g.registry.Services() {
        start := time.Now()
        err := svc.HealthCheck(r.Context())
        latency := time.Since(start)
        
        if err != nil {
            checks[name] = ComponentCheck{Status: "error", Error: err.Error()}
            allOK = false
        } else {
            checks[name] = ComponentCheck{Status: "ok", Latency: latency.String()}
        }
    }
    
    status := "ok"
    httpCode := http.StatusOK
    if !allOK {
        status = "degraded"
        httpCode = http.StatusServiceUnavailable
    }
    
    w.WriteHeader(httpCode)
    json.NewEncoder(w).Encode(HealthStatus{
        Status:    status,
        Timestamp: time.Now().UTC(),
        Checks:    checks,
    })
}
```

#### Service Health Check Interface

```go
// internal/services/interface.go

type Service interface {
    // ... existing methods
    
    // HealthCheck verifies backend connectivity (quick, no heavy operations)
    HealthCheck(ctx context.Context) error
}

// Example implementations:
// MongoDB:  client.Ping(ctx, nil)
// SQL:      db.PingContext(ctx)
// LDAP:     pool.Get() + pool.Put()
// Kafka:    admin.DescribeCluster(ctx)
// Rclone:   rpc("core/version", nil)
```

### Structured Logging

JSON logs with correlation IDs for request tracing.

#### Log Format

```json
{
  "ts": "2026-07-22T10:30:00.123Z",
  "level": "info",
  "msg": "request completed",
  "request_id": "req_abc123",
  "method": "POST",
  "path": "/api/v1/mongodb-production/query/find-orders",
  "status": 200,
  "latency_ms": 45,
  "client_ip": "127.0.0.1",
  "api_key": "ga-production",
  "service": "mongodb-production",
  "operation": "find-orders",
  "bytes_out": 12840
}
```

#### Error Log

```json
{
  "ts": "2026-07-22T10:30:00.456Z",
  "level": "error",
  "msg": "backend operation failed",
  "request_id": "req_def456",
  "service": "sql-postgres-main",
  "operation": "get-customer",
  "error": "connection refused",
  "stack": "...",
  "latency_ms": 5002
}
```

#### Implementation

```go
// internal/logging/logger.go

package logging

import (
    "context"
    "log/slog"
    "os"
)

type contextKey string
const RequestIDKey contextKey = "request_id"

func NewLogger(cfg LogConfig) *slog.Logger {
    var handler slog.Handler
    
    opts := &slog.HandlerOptions{
        Level: parseLevel(cfg.Level),
    }
    
    if cfg.Format == "json" {
        handler = slog.NewJSONHandler(os.Stdout, opts)
    } else {
        handler = slog.NewTextHandler(os.Stdout, opts)
    }
    
    return slog.New(handler)
}

// Middleware injects request_id into context
func RequestIDMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        requestID := r.Header.Get("X-Request-ID")
        if requestID == "" {
            requestID = "req_" + generateID()
        }
        
        ctx := context.WithValue(r.Context(), RequestIDKey, requestID)
        w.Header().Set("X-Request-ID", requestID)
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}

// Logger extracts request_id from context
func FromContext(ctx context.Context, base *slog.Logger) *slog.Logger {
    if requestID, ok := ctx.Value(RequestIDKey).(string); ok {
        return base.With("request_id", requestID)
    }
    return base
}
```

#### Request Logging Middleware

```go
func LoggingMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            start := time.Now()
            
            // Wrap response writer to capture status/bytes
            wrapped := &responseRecorder{ResponseWriter: w, status: 200}
            
            next.ServeHTTP(wrapped, r)
            
            log := logging.FromContext(r.Context(), logger)
            log.Info("request completed",
                "method", r.Method,
                "path", r.URL.Path,
                "status", wrapped.status,
                "latency_ms", time.Since(start).Milliseconds(),
                "bytes_out", wrapped.bytesWritten,
                "client_ip", r.RemoteAddr,
            )
        })
    }
}
```

### Log Configuration

```json
{
  "logging": {
    "level": "info",
    "format": "json",
    "output": "stdout",
    "include_caller": false
  }
}
```

| Level | When to Use |
|-------|-------------|
| `debug` | Development, troubleshooting |
| `info` | Production default — requests, startup, config changes |
| `warn` | Degraded state, retries, approaching limits |
| `error` | Failed operations, backend errors |

### Graceful Shutdown

Drain in-flight requests before stopping.

```go
// cmd/gateway/main.go

func main() {
    // ... setup
    
    server := &http.Server{
        Addr:    cfg.Server.Addr,
        Handler: router,
    }
    
    // Start server
    go func() {
        if err := server.ListenAndServe(); err != http.ErrServerClosed {
            log.Fatal(err)
        }
    }()
    
    // Wait for shutdown signal
    quit := make(chan os.Signal, 1)
    signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
    <-quit
    
    log.Info("shutting down, draining requests...")
    
    // Grace period for in-flight requests
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()
    
    if err := server.Shutdown(ctx); err != nil {
        log.Error("shutdown error", "error", err)
    }
    
    // Close backend connections
    registry.CloseAll()
    
    log.Info("shutdown complete")
}
```

### Prometheus Metrics (Future — v2)

Prometheus metrics endpoint for observability, alerting, and dashboards.

```go
// internal/metrics/metrics.go

var (
    RequestsTotal = prometheus.NewCounterVec(...)      // by service, operation, status
    RequestDuration = prometheus.NewHistogramVec(...)  // latency distribution
    BackendConnections = prometheus.NewGaugeVec(...)   // connection pool size
    BackendErrors = prometheus.NewCounterVec(...)      // errors by type
    AuthFailures = prometheus.NewCounterVec(...)       // auth failures by reason
)

// GET /metrics — standard Prometheus scrape endpoint
```

### Request Tracing (Future — v2)

OpenTelemetry integration for distributed tracing.

```go
// Future: trace request → backend operations
// Spans: HTTP request → auth → service lookup → backend call → response
// Export to: Jaeger, Zipkin, OTLP collector
```

---

## Deployment

### Windows Service

```powershell
nssm install ServiceGateway "D:\apps\service-gateway\gateway.exe"
nssm set ServiceGateway AppDirectory "D:\apps\service-gateway"
nssm set ServiceGateway AppEnvironmentExtra "GATEWAY_MASTER_KEY=..."
nssm start ServiceGateway

# Reload config without restart
gateway.exe -reload
# or
curl http://127.0.0.1:3000/_admin/reload
```

### Systemd (Linux)

```ini
[Unit]
Description=Service Gateway
After=network.target

[Service]
Type=simple
User=gateway
WorkingDirectory=/opt/service-gateway
ExecStart=/opt/service-gateway/gateway
ExecReload=/bin/kill -HUP $MAINPID
Restart=on-failure
Environment=GATEWAY_MASTER_KEY=...

[Install]
WantedBy=multi-user.target
```

```bash
# Reload config without restart
sudo systemctl reload service-gateway
# or
kill -HUP $(pidof gateway)
```

---

## Dependencies

| Package | Purpose |
|---------|---------|
| `github.com/edsilegxrepo/secretprotector/pkg/libsecsecrets` | Secret encryption/decryption (all service credentials) |
| `go.mongodb.org/mongo-driver` | MongoDB driver |
| `github.com/google/uuid` | Nonce generation |
| `github.com/rclone/rclone/librclone/librclone` | Cloud storage operations (40+ backends) |
| `github.com/xo/dburl` | SQL database URL parsing (45+ databases) |
| `github.com/lib/pq` | PostgreSQL driver |
| `github.com/go-sql-driver/mysql` | MySQL driver |
| `github.com/microsoft/go-mssqldb` | SQL Server driver |
| `github.com/sijms/go-ora/v2` | Oracle driver |

---

## Future Enhancements

### Planned Services

1. **O365 Mailbox Service (`o365mbx`)** — Microsoft Graph API access for email processing
   - New service type: `o365mbx`
   - Named operations pattern (like rclone)
   - Operations: `process-inbox`, `route-messages`, `healthcheck`
   - Wraps existing o365mbx tool as Go library
   - JWT token management via secretprotector
   - Per-mailbox service instances

   ```json
   {
     "type": "o365mbx",
     "description": "Invoice mailbox processing",
     "settings": {
       "mailbox": "invoices@company.com",
       "encrypted_token": "AES256-GCM:base64...",
       "operations": {
         "process-invoices": {
           "inbox_folder": "Inbox",
           "processed_folder": "Processed",
           "error_folder": "Errors",
           "processing_mode": "route",
           "convert_body": "pdf",
           "params": {
             "workspace": {"type": "string", "required": true}
           }
         }
       }
     }
   }
   ```

2. **HTTP Proxy Service (`http-proxy`)** — Forward requests to internal REST APIs
   - New service type: `http-proxy`
   - Configure base URL, headers, auth
   - Path rewriting rules
   - Named endpoints pattern

### Planned Features

3. **Metrics** — Prometheus endpoint for monitoring
   - Request count, latency histograms
   - Per-service, per-key breakdown
   - `/metrics` endpoint (no auth, localhost only)

4. **Audit Log** — Persistent request audit trail
   - JSON log file or database
   - Who (key name), what (service/path/query), when, result
   - Rotation policy

### Considered

5. **Windows Credential Manager** — Alternative to env var for master key
   - More secure than environment variable
   - Requires additional code in secretprotector

---

## References

- [secretprotector](https://github.com/edsilegxrepo/secretprotector) — AES-256-GCM encryption library
- [pdf-extractor](https://github.com/edsilegxrepo/pdf-extractor) — PDF text extraction tool
- [librclone](https://github.com/rclone/rclone/tree/master/librclone) — Embedded rclone library for cloud storage
- [o365mbx](https://github.com/edsilegxrepo/o365mbx) — Office 365 mailbox processing via Graph API
- [xo/dburl](https://github.com/xo/dburl) — Database URL parsing for 45+ SQL databases
- [REST API Best Practices](https://restfulapi.net/) — REST design guidelines
- [MongoDB Go Driver](https://pkg.go.dev/go.mongodb.org/mongo-driver) — Official driver documentation
- [Microsoft Graph API](https://docs.microsoft.com/en-us/graph/overview) — Office 365 API documentation
