package config

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

type Config struct {
	Server      ServerConfig   `json:"server"`
	Auth        AuthConfig     `json:"auth"`
	Logging     LoggingConfig  `json:"logging"`
	Security    SecurityConfig `json:"security"`
	ServicesDir string         `json:"services_dir"`
}

type ServerConfig struct {
	Host         string        `json:"host"`
	Port         int           `json:"port"`
	ReadTimeout  time.Duration `json:"read_timeout"`
	WriteTimeout time.Duration `json:"write_timeout"`
	IdleTimeout  time.Duration `json:"idle_timeout"`
}

type AuthConfig struct {
	MasterKeyPath   string            `json:"master_key_path"`
	MasterKeyEnvVar string            `json:"master_key_env_var"`
	Keys            map[string]APIKey `json:"keys"`
	NonceTTL        time.Duration     `json:"nonce_ttl"`
	RateLimit       RateLimitConfig   `json:"rate_limit"`
	IPRules         IPRulesConfig     `json:"ip_rules"`
	MTLS            MTLSConfig        `json:"mtls"`
}

type APIKey struct {
	EncryptedSecret string   `json:"encrypted_secret"`
	Services        []string `json:"services"`
	Roles           []string `json:"roles"`
	Description     string   `json:"description"`
	RateLimitRPM    int      `json:"rate_limit_rpm"`
	MaxConcurrent   int      `json:"max_concurrent"`
}

type RateLimitConfig struct {
	Enabled        bool `json:"enabled"`
	RequestsPerMin int  `json:"requests_per_min"`
	BurstSize      int  `json:"burst_size"`
}

type IPRulesConfig struct {
	Enabled   bool     `json:"enabled"`
	AllowList []string `json:"allow_list"`
	DenyList  []string `json:"deny_list"`
}

type MTLSConfig struct {
	Enabled       bool   `json:"enabled"`
	ClientCAPath  string `json:"client_ca_path"`
	RequireClient bool   `json:"require_client"`
}

type LoggingConfig struct {
	Level  string `json:"level"`
	Format string `json:"format"`
}

type SecurityConfig struct {
	LockMemory bool   `json:"lock_memory"`
	RunAsUser  string `json:"run_as_user"`
	RunAsGroup string `json:"run_as_group"`
}

func Load(path string) (*Config, error) {
	// #nosec G304 -- path from CLI flag (-config), admin controlled
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}

	cfg := &Config{
		Server: ServerConfig{
			Host:         "127.0.0.1",
			Port:         3000,
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 60 * time.Second,
			IdleTimeout:  120 * time.Second,
		},
		Auth: AuthConfig{
			NonceTTL: 5 * time.Minute,
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "json",
		},
		ServicesDir: "services",
	}

	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	return cfg, nil
}

func (c *ServerConfig) UnmarshalJSON(data []byte) error {
	type Alias ServerConfig
	aux := &struct {
		ReadTimeout  string `json:"read_timeout"`
		WriteTimeout string `json:"write_timeout"`
		IdleTimeout  string `json:"idle_timeout"`
		*Alias
	}{
		Alias: (*Alias)(c),
	}

	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	if aux.ReadTimeout != "" {
		d, err := time.ParseDuration(aux.ReadTimeout)
		if err != nil {
			return fmt.Errorf("parse read_timeout: %w", err)
		}
		c.ReadTimeout = d
	}
	if aux.WriteTimeout != "" {
		d, err := time.ParseDuration(aux.WriteTimeout)
		if err != nil {
			return fmt.Errorf("parse write_timeout: %w", err)
		}
		c.WriteTimeout = d
	}
	if aux.IdleTimeout != "" {
		d, err := time.ParseDuration(aux.IdleTimeout)
		if err != nil {
			return fmt.Errorf("parse idle_timeout: %w", err)
		}
		c.IdleTimeout = d
	}

	return nil
}

func (c *AuthConfig) UnmarshalJSON(data []byte) error {
	type Alias AuthConfig
	aux := &struct {
		NonceTTL string `json:"nonce_ttl"`
		*Alias
	}{
		Alias: (*Alias)(c),
	}

	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	if aux.NonceTTL != "" {
		d, err := time.ParseDuration(aux.NonceTTL)
		if err != nil {
			return fmt.Errorf("parse nonce_ttl: %w", err)
		}
		c.NonceTTL = d
	}

	return nil
}
