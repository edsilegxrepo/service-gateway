package wrapper

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"
)

func TestWrapperServiceExec(t *testing.T) {
	var executable string
	var args []string

	if runtime.GOOS == "windows" {
		executable = "C:\\Windows\\System32\\cmd.exe"
		args = []string{"/c", "echo", "hello"}
	} else {
		executable = "/bin/echo"
		args = []string{"hello"}
	}

	cfg := Config{
		Type: "binary-wrapper",
		Settings: Settings{
			Executable:     executable,
			TimeoutSeconds: 10,
			AllowedArgs:    []string{"/c", "echo", "hello"},
		},
	}

	cfgJSON, _ := json.Marshal(cfg)
	svc, err := NewWrapperService("test-wrapper", cfgJSON)
	if err != nil {
		t.Fatalf("NewWrapperService failed: %v", err)
	}

	body, _ := json.Marshal(map[string][]string{"args": args})
	req := httptest.NewRequest("POST", "/exec", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	svc.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200, body: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if resp["exit_code"].(float64) != 0 {
		t.Errorf("exit_code = %v, want 0", resp["exit_code"])
	}
}

func TestWrapperServiceInvalidPath(t *testing.T) {
	cfg := Config{
		Type: "binary-wrapper",
		Settings: Settings{
			Executable: "relative/path",
		},
	}

	cfgJSON, _ := json.Marshal(cfg)
	_, err := NewWrapperService("test", cfgJSON)
	if err == nil {
		t.Error("expected error for relative path")
	}
}

func TestWrapperServiceNotAllowedArg(t *testing.T) {
	var executable string
	if runtime.GOOS == "windows" {
		executable = "C:\\Windows\\System32\\cmd.exe"
	} else {
		executable = "/bin/echo"
	}

	cfg := Config{
		Type: "binary-wrapper",
		Settings: Settings{
			Executable:  executable,
			AllowedArgs: []string{"--safe"},
		},
	}

	cfgJSON, _ := json.Marshal(cfg)
	svc, _ := NewWrapperService("test", cfgJSON)

	body, _ := json.Marshal(map[string][]string{"args": {"--dangerous"}})
	req := httptest.NewRequest("POST", "/exec", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	svc.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestWrapperServiceUnknownOperation(t *testing.T) {
	var executable string
	if runtime.GOOS == "windows" {
		executable = "C:\\Windows\\System32\\cmd.exe"
	} else {
		executable = "/bin/echo"
	}

	cfg := Config{
		Type: "binary-wrapper",
		Settings: Settings{
			Executable: executable,
		},
	}

	cfgJSON, _ := json.Marshal(cfg)
	svc, _ := NewWrapperService("test", cfgJSON)

	req := httptest.NewRequest("POST", "/unknown", nil)
	rr := httptest.NewRecorder()

	svc.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

func TestWrapperServiceMethodNotAllowed(t *testing.T) {
	var executable string
	if runtime.GOOS == "windows" {
		executable = "C:\\Windows\\System32\\cmd.exe"
	} else {
		executable = "/bin/echo"
	}

	cfg := Config{
		Type: "binary-wrapper",
		Settings: Settings{
			Executable: executable,
		},
	}

	cfgJSON, _ := json.Marshal(cfg)
	svc, _ := NewWrapperService("test", cfgJSON)

	req := httptest.NewRequest("GET", "/exec", nil)
	rr := httptest.NewRecorder()

	svc.ServeHTTP(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rr.Code)
	}
}

func TestWrapperServiceHealthCheck(t *testing.T) {
	var executable string
	if runtime.GOOS == "windows" {
		executable = "C:\\Windows\\System32\\cmd.exe"
	} else {
		executable = "/bin/echo"
	}

	cfg := Config{
		Type: "binary-wrapper",
		Settings: Settings{
			Executable: executable,
		},
	}

	cfgJSON, _ := json.Marshal(cfg)
	svc, _ := NewWrapperService("test", cfgJSON)

	if err := svc.HealthCheck(context.Background()); err != nil {
		t.Errorf("HealthCheck failed: %v", err)
	}
}

func TestWrapperServiceClose(t *testing.T) {
	var executable string
	if runtime.GOOS == "windows" {
		executable = "C:\\Windows\\System32\\cmd.exe"
	} else {
		executable = "/bin/echo"
	}

	cfg := Config{
		Type: "binary-wrapper",
		Settings: Settings{
			Executable: executable,
		},
	}

	cfgJSON, _ := json.Marshal(cfg)
	svc, _ := NewWrapperService("test", cfgJSON)

	if err := svc.Close(); err != nil {
		t.Errorf("Close failed: %v", err)
	}
}

func TestWrapperServiceInvalidJSON(t *testing.T) {
	_, err := NewWrapperService("test", []byte("{invalid"))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestWrapperServiceInvalidBody(t *testing.T) {
	var executable string
	if runtime.GOOS == "windows" {
		executable = "C:\\Windows\\System32\\cmd.exe"
	} else {
		executable = "/bin/echo"
	}

	cfg := Config{
		Type: "binary-wrapper",
		Settings: Settings{
			Executable: executable,
		},
	}

	cfgJSON, _ := json.Marshal(cfg)
	svc, _ := NewWrapperService("test", cfgJSON)

	req := httptest.NewRequest("POST", "/exec", bytes.NewReader([]byte("{invalid")))
	rr := httptest.NewRecorder()

	svc.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestWrapperServiceNameType(t *testing.T) {
	var executable string
	if runtime.GOOS == "windows" {
		executable = "C:\\Windows\\System32\\cmd.exe"
	} else {
		executable = "/bin/echo"
	}

	cfg := Config{
		Type: "binary-wrapper",
		Settings: Settings{
			Executable: executable,
		},
	}

	cfgJSON, _ := json.Marshal(cfg)
	svc, _ := NewWrapperService("my-wrapper", cfgJSON)

	ws := svc.(*WrapperService)
	if ws.Name() != "my-wrapper" {
		t.Errorf("Name() = %s, want my-wrapper", ws.Name())
	}
	if ws.Type() != "binary-wrapper" {
		t.Errorf("Type() = %s, want binary-wrapper", ws.Type())
	}
}

func TestWrapperServiceEmptyPath(t *testing.T) {
	var executable string
	if runtime.GOOS == "windows" {
		executable = "C:\\Windows\\System32\\cmd.exe"
	} else {
		executable = "/bin/echo"
	}

	cfg := Config{
		Type: "binary-wrapper",
		Settings: Settings{
			Executable: executable,
		},
	}

	cfgJSON, _ := json.Marshal(cfg)
	svc, _ := NewWrapperService("test", cfgJSON)

	body, _ := json.Marshal(map[string][]string{"args": {}})
	req := httptest.NewRequest("POST", "/", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	svc.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
}
