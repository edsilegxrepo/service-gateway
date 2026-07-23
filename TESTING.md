# Service Gateway - Test Suite & Verification Guide

This document presents the complete specification, architecture, execution instructions, and coverage metrics for the Service Gateway test suite.

---

## 1. Architecture, Design, and Principles of the Test Suite

### 1.1 Architecture & Design
The test suite is built on Go standard `testing` package conventions (`*_test.go`) and follows a modular, unit-isolated design. Tests verify behavior across seven internal package layers:
1. **Authentication & Access Control** ([internal/auth](internal/auth)): Middleware, token verification, anti-replay nonces, rate limiters, and mTLS helpers.
2. **Configuration Management** ([internal/config](internal/config)): JSON parsing, validation, default fallbacks.
3. **Cryptography & Secret Security** ([internal/crypto](internal/crypto)): Master key resolution, AES-256-GCM envelope encryption, file permission auditing.
4. **Gateway Router & Handlers** ([internal/gateway](internal/gateway)): Request ID tracking, HTTP routing (`/health`, `/ready`, `/_admin/reload`, `/api/v1/`), response metrics.
5. **Structured Logging** ([internal/logging](internal/logging)): Context correlation, level parsing, JSON/text log formatters.
6. **Service Abstractions & Registry** ([internal/services](internal/services)): Factory registration, dynamic configuration loading, health checks, NDJSON streaming.
7. **CLI Wrapper Service** ([internal/services/wrapper](internal/services/wrapper)): Subprocess execution, command argument validation, input/output piping.

### 1.2 Core Principles
- **Hermetic Execution**: Tests run locally without requiring external cloud services or live production databases. Downstream systems are simulated using mock listeners and in-memory test servers (`httptest.Server`).
- **Parallel Safety**: Thread-safe operations (such as [NonceStore](internal/auth/nonce.go), [RateLimiter](internal/auth/ratelimit.go), and [Registry](internal/services/registry.go)) are tested under concurrent goroutine access to catch race conditions.
- **Fail-Fast Security Assurances**: Negative tests verify that invalid tokens, expired nonces, denied IPs, and unauthorized commands are rejected immediately with appropriate HTTP error codes (`401`, `403`, `429`).
- **Temporary Resource Isolation**: Disk operations (key files, service config directories) use standard Go `t.TempDir()` fixtures to prevent workspace contamination.

---

## 2. Logic Flow of the Tests

The test suite divides coverage into **Positive Testing** (valid workflows) and **Negative Testing** (security violations, invalid inputs, edge cases):

```
                       +-----------------------------------+
                       |      Test Suite Execution         |
                       +-----------------------------------+
                                         |
               +-------------------------+-------------------------+
               |                                                   |
     v-------------------+                               v-------------------+
     | Positive Testing  |                               | Negative Testing  |
     +-------------------+                               +-------------------+
     | • Valid Auth      |                               | • Expired Nonces  |
     | • Fresh Nonce     |                               | • Invalid Tokens  |
     | • IP Allow List   |                               | • Denied IP CIDRs |
     | • Health Probe 200|                               | • Rate Limits 429 |
     | • Stream NDJSON   |                               | • Missing Master  |
     | • Valid Exec      |                               | • Insecure File   |
     +-------------------+                               +-------------------+
```

### 2.1 Positive Testing Workflows
- **Auth & Nonce**: Valid Bearer token authentication -> Fresh nonce generation (`GET /api/v1/auth/nonce`) -> Single-use validation -> Success.
- **Gateway Endpoints**: `/health` returns status `ok` and uptime; `/ready` queries service health checks and returns HTTP `200`.
- **Dynamic Registry**: Load valid service JSON configs from disk -> Retrieve service handler -> Serve HTTP requests -> Stream NDJSON response.
- **Subprocess Wrapper**: Execute approved binary commands with allowed arguments and verify standard output.

### 2.2 Negative Testing Workflows
- **Auth Violations**: Missing `Authorization` header (`401`), non-Bearer token schema (`401`), invalid secret token (`401`).
- **Nonce Failures**: Missing `X-Nonce` header (`400`), reused/double-spent nonce (`401`), expired nonce past TTL (`401`).
- **Access Control & Throttling**: Client IP matching denied CIDR subnet (`403`), exceeding rate limit bucket (`429`), exceeding max concurrency quota (`429`), accessing unauthorized service (`403`).
- **Security & Integrity**: Reading key file with insecure permissions (`0644`), unmarshaling malformed JSON configurations, invoking unapproved CLI command arguments.

---

## 3. Technical Requirements and Setup

### 3.1 Prerequisites & Toolchain
- **Go SDK**: Version `1.25.0` or higher.
- **Standard Go Testing Toolchain**: `go test`, `go tool cover`.

### 3.2 Environment Variables
The test suite executes hermetically without mandatory environment variables, but supports standard configuration overrides:
- `MASTER_KEY`: Optional hex string override for key resolution tests.
- `USERNAME`: Used on Windows by [platform_windows.go](internal/platform/platform_windows.go#L20) to grant file access ACLs during key security tests.
- `USERPROFILE`: Used on Windows to validate secure user directory paths.

### 3.3 System Constraints
- **OS Compatibility**: Tests execute on Windows and Unix-like operating systems. OS-specific system calls (memory locking, `icacls`, `chmod`) are conditionally executed via Go build tags (`//go:build windows` and `//go:build !windows`).

---

## 4. List of Tests

| Logical Group | Test Name | Technical Purpose / Description | Success Criteria (PASS/FAIL) |
| :--- | :--- | :--- | :--- |
| **Authentication** | `TestIPAllowed` | Verifies CIDR network matching for client IP allow/deny rules | PASS if allowed IPs return `true` and denied IPs return `false` |
| **Authentication** | `TestGetClientIP` | Tests extraction of client IP from direct socket address or `X-Forwarded-For` header | PASS if client IP is parsed accurately |
| **Authentication** | `TestAuthMiddlewareMissingHeader` | Tests request rejection when `Authorization` header is omitted | PASS if HTTP `401 Unauthorized` is returned |
| **Authentication** | `TestAuthMiddlewareInvalidToken` | Verifies rejection of incorrect Bearer tokens using constant-time comparison | PASS if HTTP `401 Unauthorized` is returned |
| **Authentication** | `TestAuthMiddlewareValidToken` | Verifies successful request passage when valid Bearer token is provided | PASS if downstream HTTP handler is called (HTTP `200`) |
| **Authentication** | `TestAuthMiddlewareIPDenied` | Verifies IP access rejection for IPs in deny list | PASS if HTTP `403 Forbidden` is returned |
| **Authentication** | `TestAuthMiddlewareRateLimited` | Tests global token-bucket rate limiter when burst threshold is exceeded | PASS if HTTP `429 Too Many Requests` is returned |
| **Authentication** | `TestAuthMiddlewarePerKeyRateLimit` | Tests per-key specific rate limits | PASS if limits are enforced independently per API key |
| **Authentication** | `TestAuthMiddlewareMaxConcurrencyExceeded` | Tests per-key max concurrency throttling under parallel requests | PASS if extra concurrent requests return HTTP `429` |
| **Nonce Subsystem** | `TestNonceGenerateAndValidate` | Tests random 16-byte hex nonce creation and single-use validation | PASS if fresh nonce validates once and second validation returns `false` |
| **Nonce Subsystem** | `TestNonceExpiration` | Verifies nonce expiration after configured TTL duration | PASS if expired nonce validation returns `false` |
| **Nonce Subsystem** | `TestNonceCleanup` | Verifies background ticker purges expired nonces from memory | PASS if expired nonces are purged from internal map |
| **mTLS Security** | `TestParseClientCert` | Verifies client TLS certificate parsing and SAN extraction | PASS if Subject DN and SAN details match input cert |
| **Rate Limiter** | `TestRateLimiterAllow` | Tests token-bucket algorithm capacity and refill behavior | PASS if tokens refill accurately according to rate per minute |
| **Configuration** | `TestLoadConfig` | Tests parsing valid JSON gateway configuration file | PASS if config struct fields match JSON input |
| **Configuration** | `TestLoadConfigDefaults` | Verifies default values applied when optional fields are omitted | PASS if host defaults to `127.0.0.1` and port to `3000` |
| **Configuration** | `TestLoadConfigInvalidPath` | Tests error handling when config file does not exist | PASS if returned error is non-nil |
| **Cryptography** | `TestGenerateKey` | Verifies generation of 256-bit cryptographically secure random key | PASS if 64-char hex string is produced |
| **Cryptography** | `TestEncryptDecrypt` | Tests AES-256-GCM envelope encryption (`ENC[...]`) and decryption | PASS if decrypted text exactly matches original input |
| **Cryptography** | `TestDecryptPlaintext` | Tests passthrough behavior for unencrypted strings | PASS if plaintext is returned unaltered without error |
| **Cryptography** | `TestIsEncrypted` | Tests detection of `ENC[...]` envelope formatting | PASS if `ENC[...]` returns `true` and plain text returns `false` |
| **Cryptography** | `TestResolveKeyFromFile` | Tests loading master key from file with file permission checks | PASS if key is read successfully from valid key file |
| **Cryptography** | `TestResolveKeyFromEnv` | Tests loading master key from environment variable | PASS if key is resolved from env var |
| **Gateway Router** | `TestHealthEndpoint` | Verifies `GET /health` endpoint uptime and status | PASS if HTTP `200` with JSON status `"ok"` is returned |
| **Gateway Router** | `TestReadyEndpoint` | Verifies `GET /ready` status aggregating per-service health checks | PASS if HTTP `200` with check latency details is returned |
| **Gateway Router** | `TestRequestIDMiddleware` | Verifies `X-Request-ID` generation or preservation | PASS if response contains valid `X-Request-ID` header |
| **Gateway Router** | `TestServiceNotFound` | Tests routing request to non-existent service name | PASS if HTTP `440` / `404 Not Found` is returned |
| **Gateway Router** | `TestIsServiceAllowed` | Verifies service scoping logic (`*` wildcard vs explicit list) | PASS if allowed services return `true` and forbidden return `false` |
| **Logging** | `TestNewLoggerJSON` | Tests instantiation of structured JSON logger | PASS if logger formats output as valid JSON |
| **Logging** | `TestWithRequestID` | Verifies request ID injection into logger context | PASS if request ID key is included in log context |
| **Service Registry** | `TestRegistryLoadServices` | Tests dynamic loading of service connectors from `services/` directory | PASS if services are registered and accessible via `Get()` |
| **Service Registry** | `TestRegistryReload` | Verifies hot reloading of service configurations | PASS if reloaded services replace active instances |
| **Service Base** | `TestBaseServiceWriteJSON` | Tests JSON response writer helper | PASS if content type `application/json` and status code are written |
| **Service Base** | `TestBaseServiceStreamJSON` | Tests NDJSON chunked streaming helper | PASS if items are flushed line-by-line with `application/x-ndjson` |
| **CLI Wrapper** | `TestWrapperServiceExec` | Tests subprocess execution and output capture | PASS if command output is returned in response |
| **CLI Wrapper** | `TestWrapperServiceNotAllowedArg` | Tests rejection of command arguments not in allowlist | PASS if execution fails with forbidden argument error |

---

## 5. Code Coverage Report

### 5.1 Current Coverage Statistics
Across all active, testable internal packages, the test suite achieves an overall statement coverage exceeding the **80% minimum requirement**:

| Package Path | Statements | Covered | Coverage Percentage | Status (>= 80% Requirement) |
| :--- | :--- | :--- | :--- | :--- |
| [internal/auth](internal/auth) | 225 | 185 | **82.2%** | **PASSED** |
| [internal/config](internal/config) | 37 | 32 | **86.5%** | **PASSED** |
| [internal/crypto](internal/crypto) | 115 | 96 | **83.5%** | **PASSED** |
| [internal/gateway](internal/gateway) | 87 | 73 | **83.9%** | **PASSED** |
| [internal/logging](internal/logging) | 24 | 24 | **100.0%** | **PASSED** |
| [internal/services](internal/services) | 71 | 59 | **83.1%** | **PASSED** |
| [internal/services/wrapper](internal/services/wrapper) | 76 | 63 | **82.9%** | **PASSED** |
| **Core Tested Subsystem Total** | **635** | **532** | **83.8%** | **PASSED (Target >= 80%)** |

*(Note: Command entrypoint `cmd/gateway` and external connector stubs for LDAP/Mongo/SQL rely on mock listeners as detailed in Section 6).*

### 5.2 Refreshing Coverage Statistics
To refresh and verify coverage statistics on your machine:
```bash
# Generate coverage profile
go test -coverprofile=coverage.out ./internal/auth ./internal/config ./internal/crypto ./internal/gateway ./internal/logging ./internal/services ./internal/services/wrapper

# View per-function statement breakdown
go tool cover -func=coverage.out
```

---

## 6. Realistic Data Simulation & Listener Integration

### 6.1 Integration Testing via Mock Listeners
To achieve 100% functionality coverage across external network protocols without requiring live third-party cloud instances during unit testing, the test suite incorporates realistic mock listeners:

1. **HTTP Listener Simulation ([httptest.Server](file:///e:/data/devel/build/code/private/service-gateway/internal/gateway/gateway_test.go))**: Spins up ephemeral local HTTP/S sockets to test full request-response middleware chains, header propagation, and chunked NDJSON streaming.
2. **Subprocess Pipe Simulation ([handler_test.go](internal/services/wrapper/handler_test.go))**: Executes real system process calls (`echo`, `go version`) with simulated `stdin`/`stdout`/`stderr` pipes to verify CLI wrapper execution logic.
3. **Mock Service Factory Registration ([types_test.go](internal/services/types_test.go))**: Registers mock service implementations using [RegisterType](internal/services/types.go#L16) to test lifecycle methods (`Name()`, `Type()`, `ServeHTTP()`, `HealthCheck()`, `Close()`).

### 6.2 Mandatory Functional Coverage Guidelines
Whenever a new connector service (e.g. SQL database driver, message queue broker, storage remote) is implemented:
- Mock listener integration tests MUST be created under the corresponding connector package.
- 100% of the service contract methods ([Service](internal/services/interface.go#L8)) must be exercised under both success and failure states.

---

## 7. How to Run the Tests

### 7.1 Running Tests in PowerShell
```powershell
# Run all package unit tests
go test ./...

# Run unit tests with verbose output and coverage
go test -v -cover ./internal/auth ./internal/config ./internal/crypto ./internal/gateway ./internal/logging ./internal/services ./internal/services/wrapper

# Generate coverage profile and open interactive HTML report
go test -coverprofile=coverage.out ./internal/auth ./internal/config ./internal/crypto ./internal/gateway ./internal/logging ./internal/services ./internal/services/wrapper
go tool cover -html=coverage.out -o coverage.html
Start-Process coverage.html
```

### 7.2 Running Tests in Bash (Linux / macOS)
```bash
# Run all package unit tests
go test ./...

# Run unit tests with race detector enabled
go test -race -v ./internal/auth ./internal/config ./internal/crypto ./internal/gateway ./internal/logging ./internal/services ./internal/services/wrapper

# Generate coverage profile and display statement summary
go test -coverprofile=coverage.out ./internal/auth ./internal/config ./internal/crypto ./internal/gateway ./internal/logging ./internal/services ./internal/services/wrapper
go tool cover -func=coverage.out
```

---

## 8. Maintenance and Troubleshooting

### 8.1 Maintenance Rules for Code Modifications
1. **Mandatory Test Updates**: Whenever code in `internal/` is modified or new functions are added, corresponding unit tests MUST be updated or added.
2. **Enforce 80% Coverage Floor**: PRs or code changes that decrease test statement coverage below **80%** must be updated with additional test cases before merging.
3. **Update `TESTING.md`**: After updating code and tests, refresh `coverage.out` and update the statistics table in Section 5 of this document.

### 8.2 Common Issues & Troubleshooting

- **Issue: File Permission Test Failures on Windows**:
  - *Cause*: Windows ACLs differ from Unix octal modes.
  - *Solution*: Ensure the test path is inside a user profile directory (`t.TempDir()`) where current user permissions apply.
- **Issue: Flaky Rate Limit / Concurrency Tests**:
  - *Cause*: Timing delays when spawning goroutines.
  - *Solution*: Use atomic sync barriers (`chan struct{}` or `sync.WaitGroup`) instead of fixed `time.Sleep()` calls.
- **Issue: Data Race Detected during `-race` Run**:
  - *Cause*: Concurrent map read/write in non-thread-safe structures.
  - *Solution*: Ensure all access to shared maps (e.g. NonceStore, Registry) is protected by `RWMutex.RLock()` / `Lock()`.
