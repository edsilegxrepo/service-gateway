# Service Gateway

A secure, enterprise-grade API gateway and reverse proxy built in Go (`1.25.0`). The Service Gateway provides a unified, zero-trust integration point between clients and internal backend services including LDAP/Active Directory, MongoDB, SQL databases, Message Queues (Watermill), Office 365, Rclone remotes, PDF extraction engines, and CLI wrappers.

---

## 1. Application Overview and Objectives

### Overview
Modern enterprise architectures require connecting API consumers to diverse downstream infrastructure without exposing credentials, database endpoints, or internal networks directly. The Service Gateway acts as a central proxy and security boundary, translating standardized HTTP REST/NDJSON requests into native backend driver calls.

### Core Objectives
- **Unified Proxy Boundary**: Aggregate multiple enterprise service connectors (LDAP, MongoDB, SQL, MQ, O365, Rclone, PDF Extractor, CLI Executables) under a single `/api/v1/{service}/` endpoint namespace.
- **Zero-Trust Security**: Require authentication, single-use anti-replay nonces, IP access control lists, token-bucket rate limiting, and per-key concurrency caps on all API calls.
- **Secrets Management & Envelope Encryption**: Ensure passwords and access tokens in configuration files are encrypted using AES-256-GCM (`ENC[...]` envelope format).
- **Hardened Execution Environment**: Provide platform-specific security protections such as process memory locking (`mlockall`), key file permission enforcement (`0600`/`0400`), and privilege dropping (`setuid`/`setgid`).
- **High Availability & Hot Reloading**: Allow dynamic loading and hot reloading of service configurations via `SIGHUP` or `POST /_admin/reload` without server downtime.

---

## 2. Security Assessment

### 2.1 Encryption in Transit
- **TLS / mTLS Support**: The gateway supports standard TLS termination and mutual TLS (mTLS) client certificate verification via [mtls.go](internal/auth/mtls.go).
- **Forward Secrecy**: Uses Go standard library `crypto/tls` defaults enforcing modern cipher suites with Forward Secrecy (ECDHE).

### 2.2 Secret Management
- **AES-256-GCM Envelope Encryption**: Secrets embedded in service configuration files (e.g. database credentials, API tokens) are encrypted using AES-256-GCM with an `ENC[...]` prefix and suffix envelope via [crypto.go](internal/crypto/crypto.go).
- **Master Key Resolution**: The 256-bit master key is loaded at boot from an environment variable (`MASTER_KEY`) or a restricted file ([InitMasterKey](internal/crypto/crypto.go#L38)).
- **File Access Hardening**: On Unix, key files must have `0600` or `0400` permissions. On Windows, permissions are restricted via `icacls` to the running user account, and temporary paths are rejected ([validateWindowsKeyPath](internal/crypto/crypto.go#L199)).

### 2.3 Authentication & Anti-Replay
- **Bearer Token Verification**: Requests are authenticated via `Authorization: Bearer <token>`. Tokens are verified using constant-time comparison ([subtle.ConstantTimeCompare](internal/auth/middleware.go#L167)) to prevent timing side-channel attacks.
- **Single-Use Anti-Replay Nonce**: Protected API routes require a one-time nonce passed in the `X-Nonce` header. Nonces are issued via `GET /api/v1/auth/nonce`, stored in memory with a TTL (default 5 minutes), and atomically deleted after first use by [NonceStore](internal/auth/nonce.go).
- **IP Access Control (ACL)**: Configurable CIDR-based allow/deny lists ([isIPAllowed](internal/auth/middleware.go#L179)) permit restricting access to trusted networks.

### 2.4 Role-Based Access Control (RBAC) & Service Scoping
- **Per-Key Service Whitelisting**: Each API key configuration defines an allowed list of services (`services: ["sql-example", "ldap-corp-ad"]` or wildcard `services: ["*"]`). Access is verified per request by [isServiceAllowed](internal/gateway/gateway.go#L173).
- **Role Assignments**: Key definitions support role labels for custom policy checks.

### 2.5 Dependencies & Vulnerability Audit
All dependencies are maintained on current, non-vulnerable module versions:
- `github.com/ThreeDotsLabs/watermill` (`v1.5.2`)
- `github.com/go-ldap/ldap/v3` (`v3.4.14`)
- `github.com/rclone/rclone` (`v1.74.4`)
- `github.com/xo/dburl` (`v0.24.2`)
- `go.mongodb.org/mongo-driver` (`v1.17.9`)
- `golang.org/x/sys` (`v0.47.0`)

### 2.6 Unprivileged Process Context & Memory Protection
- **Privilege Dropping**: On Unix, the gateway can bind to privileged ports as `root` and immediately drop privileges to an unprivileged user/group via [DropPrivileges](internal/platform/platform_unix.go#L64) (`setuid`/`setgid`).
- **Memory Locking (`mlockall`)**: Prevents process memory containing decrypted keys and sensitive credentials from being swapped out to disk via [LockMemory](internal/platform/platform_unix.go#L78).

---

## 3. Code Quality Assessment & Best Practices

- **Go Standard Idioms**: Adheres to Go project layout standards (`cmd/`, `internal/`), using standard library `log/slog` for structured logging, `net/http` for routing, and native context propagation.
- **Thread Safety**: Shared internal structures (Service Registry, NonceStore, RateLimiter) employ `sync.RWMutex` locks and atomic primitives to prevent data races under high concurrency.
- **Defensive Error Handling**: Explicit error checking with wrapped error contexts (`fmt.Errorf("...: %w", err)`). Sensitive key memory buffers can be wiped using [ZeroBuffer](internal/crypto/crypto.go#L269).
- **Static Analysis Compliance**: Code passes standard Go linters (`go vet`) and includes security annotations (`#nosec G304`, `#nosec G204`) for audited file and process interactions.

---

## 4. Command Line Arguments

The `service-gateway` binary accepts the following command-line flags:

| Flag | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| `-config` | `string` | `"config.json"` | Path to the gateway JSON configuration file. |
| `-version` | `bool` | `false` | Display binary version and build timestamp, then exit. |
| `-key-gen` | `string` | `""` | Generate a new 256-bit AES master key (64 hex characters), save it securely to the specified file path with restricted permissions, and exit. |
| `-reload` | `bool` | `false` | Signal the running gateway process to reload its configuration and exit. |

---

## 5. Deployment and Usage Examples

### 5.1 Step 1: Generate a Master Encryption Key
Generate a master encryption key and store it in `master.key`:
```bash
./service-gateway -key-gen master.key
```
**Output Sample:**
```text
Master key generated: master.key
```

### 5.2 Step 2: Configure the Gateway (`config.json`)
Create `config.json` defining server port, authentication keys, and rate limits:
```json
{
  "server": {
    "host": "127.0.0.1",
    "port": 3000,
    "read_timeout": "30s",
    "write_timeout": "60s",
    "idle_timeout": "120s"
  },
  "auth": {
    "master_key_path": "master.key",
    "nonce_ttl": "5m",
    "keys": {
      "ga-production": {
        "encrypted_secret": "ENC[V2hhdEFCZWF1dGlmdWxTZWNyZXQxMjM0NTY3ODkwMTIzNDU2]",
        "services": ["*"],
        "roles": ["read", "write"],
        "description": "Production access key"
      }
    },
    "rate_limit": {
      "enabled": true,
      "requests_per_min": 600,
      "burst_size": 50
    },
    "ip_rules": {
      "enabled": false,
      "allow_list": ["127.0.0.1"],
      "deny_list": []
    }
  },
  "logging": {
    "level": "info",
    "format": "json"
  },
  "security": {
    "lock_memory": false
  },
  "services_dir": "services"
}
```

### 5.3 Step 3: Launch the Gateway Server
Start the gateway server:
```bash
./service-gateway -config config.json
```
**Output Sample (Structured JSON Log):**
```json
{"time":"2026-07-22T22:15:00.000Z","level":"INFO","msg":"loaded service","name":"sql-example"}
{"time":"2026-07-22T22:15:00.001Z","level":"INFO","msg":"loaded service","name":"ldap-corp-ad"}
{"time":"2026-07-22T22:15:00.002Z","level":"INFO","msg":"starting server","addr":"127.0.0.1:3000","version":"dev"}
```

### 5.4 Step 4: Obtain an Anti-Replay Nonce
Request a single-use nonce prior to executing API requests:
```bash
curl -s -H "Authorization: Bearer WhatABeautifulSecret1234567890123456" \
  http://127.0.0.1:3000/api/v1/auth/nonce
```
**Output Sample:**
```json
{
  "nonce": "a7b3c9d1e2f458690123456789abcdef"
}
```

### 5.5 Step 5: Execute a Service Request
Execute a query against the `sql-example` service connector using the bearer token and acquired nonce:
```bash
curl -s -X POST \
  -H "Authorization: Bearer WhatABeautifulSecret1234567890123456" \
  -H "X-Nonce: a7b3c9d1e2f458690123456789abcdef" \
  -H "Content-Type: application/json" \
  -d '{"query": "SELECT id, username, email FROM users WHERE status = ?", "args": ["active"]}' \
  http://127.0.0.1:3000/api/v1/sql-example/query
```
**Output Sample (Streamed NDJSON):**
```json
{"id":1,"username":"alice","email":"alice@example.com"}
{"id":2,"username":"bob","email":"bob@example.com"}
```

### 5.6 Step 6: Health and Readiness Probes
Check application health:
```bash
curl -s http://127.0.0.1:3000/health
```
**Output Sample:**
```json
{
  "status": "ok",
  "timestamp": "2026-07-22T22:16:00Z",
  "version": "dev",
  "uptime": "1m0s"
}
```

Check system readiness across all active service connectors:
```bash
curl -s http://127.0.0.1:3000/ready
```
**Output Sample:**
```json
{
  "status": "ok",
  "timestamp": "2026-07-22T22:16:05Z",
  "checks": {
    "ldap-corp-ad": { "status": "ok", "latency": "2.1ms" },
    "sql-example": { "status": "ok", "latency": "1.4ms" }
  }
}
```

Check operational performance metrics:
```bash
curl -s http://127.0.0.1:3000/metrics
```
**Output Sample:**
```json
{
  "status": "ok",
  "version": "dev",
  "uptime": "5m12s",
  "requests_total": 42,
  "requests_2xx": 40,
  "requests_4xx": 2,
  "requests_5xx": 0
}
```

### 5.7 Step 7: Hot Reload Configuration (End-to-End Example)

The gateway supports dynamic zero-downtime hot reloading of service configurations without interrupting active API traffic. Administrative reloads are protected via **Localhost Socket Isolation** (`127.0.0.1` / `::1` origin validation with `X-Forwarded-For` proxy protection).

#### Option 1: Automated Reload via CLI (Recommended)
Trigger a hot reload using the `-reload` CLI flag:

```bash
./service-gateway -config config.json -reload
```
**Output Sample:**
```text
Configuration reload triggered successfully.
```

#### Option 2: Direct HTTP POST Request (Localhost Only)
Issue an administrative reload directly via HTTP over the loopback interface:

```bash
curl -s -X POST http://127.0.0.1:3000/_admin/reload
```

**Output Sample (HTTP 200 OK):**
```json
{
  "status": "reloaded"
}
```

**Output Sample (HTTP 403 Forbidden - Non-Localhost Origin):**
```text
Forbidden
```

#### Option 3: OS Signal (Unix-Only)
Send a `SIGHUP` signal directly to the running gateway process:
```bash
kill -HUP <pid>
```


---

## 6. Testing Guide

For detailed information on the unit test suite, running coverage reports, and writing integration tests, refer to [TESTING.md](TESTING.md).
