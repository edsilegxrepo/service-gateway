package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	configJSON := `{
		"server": {
			"host": "127.0.0.1",
			"port": 8080,
			"read_timeout": "15s",
			"write_timeout": "30s",
			"idle_timeout": "60s"
		},
		"auth": {
			"master_key_path": "/path/to/key",
			"nonce_ttl": "10m",
			"keys": {
				"test-key": {
					"encrypted_secret": "secret123",
					"services": ["*"],
					"roles": ["admin"]
				}
			}
		},
		"logging": {
			"level": "debug",
			"format": "text"
		},
		"services_dir": "my-services"
	}`

	if err := os.WriteFile(configPath, []byte(configJSON), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.Server.Host != "127.0.0.1" {
		t.Errorf("Host = %s, want 127.0.0.1", cfg.Server.Host)
	}
	if cfg.Server.Port != 8080 {
		t.Errorf("Port = %d, want 8080", cfg.Server.Port)
	}
	if cfg.Server.ReadTimeout != 15*time.Second {
		t.Errorf("ReadTimeout = %v, want 15s", cfg.Server.ReadTimeout)
	}
	if cfg.Server.WriteTimeout != 30*time.Second {
		t.Errorf("WriteTimeout = %v, want 30s", cfg.Server.WriteTimeout)
	}
	if cfg.Auth.NonceTTL != 10*time.Minute {
		t.Errorf("NonceTTL = %v, want 10m", cfg.Auth.NonceTTL)
	}
	if cfg.Logging.Level != "debug" {
		t.Errorf("Level = %s, want debug", cfg.Logging.Level)
	}
	if cfg.ServicesDir != "my-services" {
		t.Errorf("ServicesDir = %s, want my-services", cfg.ServicesDir)
	}
}

func TestLoadConfigDefaults(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	if err := os.WriteFile(configPath, []byte(`{}`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.Server.Host != "127.0.0.1" {
		t.Errorf("default Host = %s, want 127.0.0.1", cfg.Server.Host)
	}
	if cfg.Server.Port != 3000 {
		t.Errorf("default Port = %d, want 3000", cfg.Server.Port)
	}
	if cfg.Auth.NonceTTL != 5*time.Minute {
		t.Errorf("default NonceTTL = %v, want 5m", cfg.Auth.NonceTTL)
	}
	if cfg.ServicesDir != "services" {
		t.Errorf("default ServicesDir = %s, want services", cfg.ServicesDir)
	}
}

func TestLoadConfigFileNotFound(t *testing.T) {
	_, err := Load("/nonexistent/config.json")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestLoadConfigInvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	if err := os.WriteFile(configPath, []byte(`{invalid`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, err := Load(configPath)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestLoadConfigInvalidDuration(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	configJSON := `{"server": {"read_timeout": "invalid"}}`
	if err := os.WriteFile(configPath, []byte(configJSON), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, err := Load(configPath)
	if err == nil {
		t.Error("expected error for invalid duration")
	}
}
