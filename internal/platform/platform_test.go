package platform

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestCurrentPlatform(t *testing.T) {
	plat := Current()
	if plat == nil {
		t.Fatal("Current() returned nil platform interface")
	}

	if err := plat.LockMemory(); err != nil {
		t.Logf("LockMemory returned: %v", err)
	}

	if err := plat.SetProcessPriority(0); err != nil {
		t.Errorf("SetProcessPriority failed: %v", err)
	}

	if err := plat.SetResourceLimits(1024, 512); err != nil {
		t.Errorf("SetResourceLimits failed: %v", err)
	}

	if err := plat.DropPrivileges("", ""); err != nil {
		t.Errorf("DropPrivileges failed: %v", err)
	}

	_ = plat.SetupReloadSignal()

	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.key")
	if err := os.WriteFile(tmpFile, []byte("key-data"), 0o600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	if err := plat.ValidateKeyFilePermissions(tmpFile); err != nil {
		t.Errorf("ValidateKeyFilePermissions failed: %v", err)
	}

	_ = plat.SecureFilePermissions(tmpFile)

	// Restore permissions on Windows so t.TempDir cleanup can delete the test file
	if runtime.GOOS == "windows" {
		_ = exec.Command("icacls", tmpFile, "/grant", os.Getenv("USERNAME")+":F").Run()
	} else {
		_ = os.Chmod(tmpFile, 0o600)
	}
}
