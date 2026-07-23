package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func TestGenerateMasterKey(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "master.key")
	if err := generateMasterKey(tmpFile); err != nil {
		t.Fatalf("generateMasterKey failed: %v", err)
	}

	data, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	keyStr := strings.TrimSpace(string(data))
	if len(keyStr) != 64 {
		t.Errorf("key length = %d, want 64", len(keyStr))
	}

	if runtime.GOOS == "windows" {
		_ = exec.Command("icacls", tmpFile, "/grant", os.Getenv("USERNAME")+":F").Run()
	} else {
		_ = os.Chmod(tmpFile, 0o600)
	}
}

func TestTriggerReload(t *testing.T) {
	reloadHit := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/_admin/reload" && r.Method == "POST" {
			reloadHit = true
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	parts := strings.Split(server.URL, ":")
	portStr := parts[len(parts)-1]
	port, _ := strconv.Atoi(portStr)

	cfgJSON := `{"server": {"host": "127.0.0.1", "port": ` + strconv.Itoa(port) + `}}`
	tmpCfg := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(tmpCfg, []byte(cfgJSON), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	if err := triggerReload(tmpCfg); err != nil {
		t.Fatalf("triggerReload failed: %v", err)
	}

	if !reloadHit {
		t.Error("expected reload endpoint to be called")
	}
}
