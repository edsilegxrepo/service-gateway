package o365mbx

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/jarcoal/httpmock"
	"github.com/stretchr/testify/require"
)

func sampleConfigJSON(t *testing.T) json.RawMessage {
	t.Helper()
	cfg := map[string]any{
		"type":        "o365mbx",
		"description": "Invoice mailbox test service",
		"version":     "1.0",
		"settings": map[string]any{
			"mailbox":      "invoices@company.com",
			"token_string": "mock-jwt-token",
			"defaults": map[string]any{
				"timeout_seconds":         120,
				"max_execution_time_msg": 120,
				"parallel":                5,
				"api_rate":                5.0,
				"api_burst":               10,
			},
			"operations": map[string]any{
				"process-all": map[string]any{
					"description":     "Process all messages",
					"processing_mode": "full",
					"params": map[string]any{
						"workspace": map[string]any{
							"type":     "string",
							"required": true,
						},
					},
				},
				"process-incremental": map[string]any{
					"description":     "Incremental processing",
					"processing_mode": "incremental",
					"params": map[string]any{
						"workspace": map[string]any{
							"type":     "string",
							"required": true,
						},
						"state_file": map[string]any{
							"type":     "string",
							"required": true,
						},
					},
				},
			},
		},
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal sample config: %v", err)
	}
	return json.RawMessage(data)
}

func TestNewO365MBXService(t *testing.T) {
	rawCfg := sampleConfigJSON(t)
	svc, err := NewO365MBXService("o365mbx-invoices", rawCfg)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if svc.Name() != "o365mbx-invoices" {
		t.Errorf("expected service name 'o365mbx-invoices', got '%s'", svc.Name())
	}
	if svc.Type() != "o365mbx" {
		t.Errorf("expected service type 'o365mbx', got '%s'", svc.Type())
	}
}

func TestNewO365MBXService_MissingMailbox(t *testing.T) {
	cfg := map[string]any{
		"type": "o365mbx",
		"settings": map[string]any{
			"mailbox": "",
		},
	}
	data, _ := json.Marshal(cfg)
	_, err := NewO365MBXService("o365mbx-test", json.RawMessage(data))
	if err == nil {
		t.Fatal("expected error for missing mailbox, got nil")
	}
}

func TestServeHTTP_EndpointRouting(t *testing.T) {
	svc, err := NewO365MBXService("o365mbx-invoices", sampleConfigJSON(t))
	if err != nil {
		t.Fatalf("init service: %v", err)
	}

	// 1. Invalid endpoint (not /op)
	req := httptest.NewRequest("POST", "/invalid", nil)
	rec := httptest.NewRecorder()
	svc.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected HTTP 404 for invalid endpoint, got %d", rec.Code)
	}

	// 2. Missing operation name (/op/)
	req = httptest.NewRequest("POST", "/op/", nil)
	rec = httptest.NewRecorder()
	svc.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected HTTP 400 for missing operation name, got %d", rec.Code)
	}

	// 3. Unknown operation (/op/unknown)
	req = httptest.NewRequest("POST", "/op/unknown", nil)
	rec = httptest.NewRecorder()
	svc.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected HTTP 404 for unknown operation, got %d", rec.Code)
	}
}

func TestServeHTTP_ParamValidation(t *testing.T) {
	svc, err := NewO365MBXService("o365mbx-invoices", sampleConfigJSON(t))
	if err != nil {
		t.Fatalf("init service: %v", err)
	}

	// Missing required 'workspace' parameter
	reqBody, _ := json.Marshal(map[string]any{})
	req := httptest.NewRequest("POST", "/op/process-all", bytes.NewReader(reqBody))
	rec := httptest.NewRecorder()
	svc.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected HTTP 400 for missing workspace param, got %d", rec.Code)
	}

	// Relative workspace path validation error
	reqBody, _ = json.Marshal(map[string]any{
		"workspace": "relative/path",
	})
	req = httptest.NewRequest("POST", "/op/process-all", bytes.NewReader(reqBody))
	rec = httptest.NewRecorder()
	svc.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected HTTP 400 for relative workspace path, got %d", rec.Code)
	}
}

func TestBuildEngineConfig_ClientCredentials(t *testing.T) {
	cfgMap := map[string]any{
		"type": "o365mbx",
		"settings": map[string]any{
			"mailbox":       "test@company.com",
			"tenant_id":     "tenant-uuid",
			"client_id":     "client-uuid",
			"client_secret": "client-secret-val",
			"operations": map[string]any{
				"healthcheck": map[string]any{
					"description":     "Health check",
					"processing_mode": "healthcheck",
				},
			},
		},
	}
	data, _ := json.Marshal(cfgMap)
	svc, err := NewO365MBXService("o365mbx-test", json.RawMessage(data))
	if err != nil {
		t.Fatalf("init service: %v", err)
	}

	typedSvc := svc.(*O365MBXService)
	engineCfg, err := typedSvc.buildEngineConfig(context.Background(), "healthcheck", typedSvc.config.Settings.Operations["healthcheck"], nil)
	if err != nil {
		t.Fatalf("buildEngineConfig error: %v", err)
	}

	if engineCfg.TenantID != "tenant-uuid" {
		t.Errorf("expected tenantID 'tenant-uuid', got '%s'", engineCfg.TenantID)
	}
	if engineCfg.ClientID != "client-uuid" {
		t.Errorf("expected clientID 'client-uuid', got '%s'", engineCfg.ClientID)
	}
	if engineCfg.ClientSecret != "client-secret-val" {
		t.Errorf("expected clientSecret 'client-secret-val', got '%s'", engineCfg.ClientSecret)
	}
	if engineCfg.MailboxName != "test@company.com" {
		t.Errorf("expected mailboxName 'test@company.com', got '%s'", engineCfg.MailboxName)
	}
}

func TestValidatePath(t *testing.T) {
	tempDir := t.TempDir()
	absPath := filepath.Join(tempDir, "output")

	if err := validatePath(absPath); err != nil {
		t.Errorf("expected valid path, got: %v", err)
	}

	if err := validatePath("relative/path"); err == nil {
		t.Error("expected error for relative path, got nil")
	}

	if err := validatePath(absPath + "/../outside"); err == nil {
		t.Error("expected error for non-canonical path with .., got nil")
	}

	_ = os.Remove
}

func TestServeHTTP_HealthCheckOperation(t *testing.T) {
	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	httpmock.RegisterResponder("POST", "https://login.microsoftonline.com/tenant-123/oauth2/v2.0/token",
		httpmock.NewStringResponder(200, `{"access_token": "mock-token", "expires_in": 3600}`))

	httpmock.RegisterRegexpResponder("GET", regexp.MustCompile(`https://graph\.microsoft\.com/v1\.0/users/.*`),
		httpmock.NewStringResponder(200, `{"value": []}`))

	cfgMap := map[string]any{
		"type": "o365mbx",
		"settings": map[string]any{
			"mailbox":       "invoices@company.com",
			"tenant_id":     "tenant-123",
			"client_id":     "client-456",
			"client_secret": "secret-789",
			"operations": map[string]any{
				"healthcheck": map[string]any{
					"description":     "Check health",
					"processing_mode": "healthcheck",
				},
			},
		},
	}
	data, _ := json.Marshal(cfgMap)
	svc, err := NewO365MBXService("o365mbx-invoices", json.RawMessage(data))
	if err != nil {
		t.Fatalf("init service: %v", err)
	}

	req := httptest.NewRequest("POST", "/op/healthcheck", nil)
	rec := httptest.NewRecorder()
	svc.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected HTTP 200 for healthcheck operation, got %d (body: %s)", rec.Code, rec.Body.String())
	}
	var res map[string]any
	json.NewDecoder(rec.Body).Decode(&res)
	if res["status"] != "success" {
		t.Errorf("expected status 'success', got '%v'", res["status"])
	}
}

func TestServeHTTP_ProcessAllOperation(t *testing.T) {
	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	httpmock.RegisterResponder("POST", "https://login.microsoftonline.com/tenant-123/oauth2/v2.0/token",
		httpmock.NewStringResponder(200, `{"access_token": "mock-token", "expires_in": 3600}`))

	httpmock.RegisterRegexpResponder("GET", regexp.MustCompile(`https://graph\.microsoft\.com/v1\.0/users/.*`),
		httpmock.NewStringResponder(200, `{"value": []}`))

	cfgMap := map[string]any{
		"type": "o365mbx",
		"settings": map[string]any{
			"mailbox":       "invoices@company.com",
			"tenant_id":     "tenant-123",
			"client_id":     "client-456",
			"client_secret": "secret-789",
			"operations": map[string]any{
				"process-all": map[string]any{
					"description":     "Process all messages",
					"processing_mode": "full",
					"inbox_folder":     "Inbox",
					"convert_body":     "none",
					"params": map[string]any{
						"workspace": map[string]any{"type": "string", "required": true},
					},
				},
			},
		},
	}
	data, _ := json.Marshal(cfgMap)
	svc, err := NewO365MBXService("o365mbx-invoices", json.RawMessage(data))
	require.NoError(t, err)

	workspaceDir := t.TempDir()
	reqBody, _ := json.Marshal(map[string]string{
		"workspace": workspaceDir,
	})

	req := httptest.NewRequest("POST", "/op/process-all", bytes.NewReader(reqBody))
	rec := httptest.NewRecorder()
	svc.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}
	var res map[string]any
	json.NewDecoder(rec.Body).Decode(&res)
	if res["status"] != "success" {
		t.Errorf("expected status 'success', got '%v'", res["status"])
	}
}

func TestServeHTTP_ProcessIncrementalOperation(t *testing.T) {
	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	httpmock.RegisterResponder("POST", "https://login.microsoftonline.com/tenant-123/oauth2/v2.0/token",
		httpmock.NewStringResponder(200, `{"access_token": "mock-token", "expires_in": 3600}`))

	httpmock.RegisterRegexpResponder("GET", regexp.MustCompile(`https://graph\.microsoft\.com/v1\.0/users/.*`),
		httpmock.NewStringResponder(200, `{"value": []}`))

	cfgMap := map[string]any{
		"type": "o365mbx",
		"settings": map[string]any{
			"mailbox":       "invoices@company.com",
			"tenant_id":     "tenant-123",
			"client_id":     "client-456",
			"client_secret": "secret-789",
			"operations": map[string]any{
				"process-incremental": map[string]any{
					"description":     "Incremental delta sync",
					"processing_mode": "incremental",
					"inbox_folder":     "Inbox",
					"params": map[string]any{
						"workspace":  map[string]any{"type": "string", "required": true},
						"state_file": map[string]any{"type": "string", "required": true},
					},
				},
			},
		},
	}
	data, _ := json.Marshal(cfgMap)
	svc, err := NewO365MBXService("o365mbx-invoices", json.RawMessage(data))
	require.NoError(t, err)

	workspaceDir := t.TempDir()
	stateFile := filepath.Join(workspaceDir, "state.json")
	_ = os.WriteFile(stateFile, []byte(`{"delta_link": "https://graph.microsoft.com/v1.0/delta-123"}`), 0o600)

	reqBody, _ := json.Marshal(map[string]string{
		"workspace":  workspaceDir,
		"state_file": stateFile,
	})

	req := httptest.NewRequest("POST", "/op/process-incremental", bytes.NewReader(reqBody))
	rec := httptest.NewRecorder()
	svc.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}
	var res map[string]any
	json.NewDecoder(rec.Body).Decode(&res)
	if res["status"] != "success" {
		t.Errorf("expected status 'success', got '%v'", res["status"])
	}
}
