package o365mbx

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"criticalsys.net/o365mbx/downloader"
	"criticalsys.net/o365mbx/engine"

	"criticalsys.net/service-gateway/internal/crypto"
	"criticalsys.net/service-gateway/internal/services"
)

func init() {
	services.RegisterType("o365mbx", NewO365MBXService)
}

type O365MBXService struct {
	services.BaseService
	config Config
}

type Config struct {
	Type        string   `json:"type"`
	Description string   `json:"description"`
	Version     string   `json:"version"`
	Settings    Settings `json:"settings"`
}

type Settings struct {
	TenantID              string               `json:"tenant_id,omitempty"`
	ClientID              string               `json:"client_id,omitempty"`
	EncryptedClientID     string               `json:"encrypted_client_id,omitempty"`
	ClientSecret          string               `json:"client_secret,omitempty"`
	EncryptedClientSecret string               `json:"encrypted_client_secret,omitempty"`
	TokenString           string               `json:"token_string,omitempty"`
	EncryptedToken        string               `json:"encrypted_token,omitempty"`
	Mailbox               string               `json:"mailbox"`
	Defaults              DefaultSettings      `json:"defaults"`
	Operations            map[string]Operation `json:"operations"`
}

type DefaultSettings struct {
	TimeoutSeconds      int     `json:"timeout_seconds,omitempty"`
	MaxExecutionTimeMsg int     `json:"max_execution_time_msg,omitempty"`
	Parallel            int     `json:"parallel,omitempty"`
	MaxRetries          int     `json:"max_retries,omitempty"`
	APIRate             float64 `json:"api_rate,omitempty"`
	APIBurst            int     `json:"api_burst,omitempty"`
	ChromiumPath        string  `json:"chromium_path,omitempty"`
}

type Operation struct {
	Description     string           `json:"description"`
	ProcessingMode  string           `json:"processing_mode"`
	InboxFolder     string           `json:"inbox_folder,omitempty"`
	ProcessedFolder string           `json:"processed_folder,omitempty"`
	ErrorFolder     string           `json:"error_folder,omitempty"`
	ConvertBody     string           `json:"convert_body,omitempty"`
	Params          map[string]Param `json:"params"`
}

type Param struct {
	Type        string `json:"type"`
	Required    bool   `json:"required"`
	Default     any    `json:"default,omitempty"`
	Description string `json:"description,omitempty"`
}

func NewO365MBXService(name string, rawConfig json.RawMessage) (services.Service, error) {
	var cfg Config
	if err := json.Unmarshal(rawConfig, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	if cfg.Settings.Mailbox == "" {
		return nil, fmt.Errorf("mailbox is required in settings")
	}

	return &O365MBXService{
		BaseService: services.NewBaseService(name, "o365mbx"),
		config:      cfg,
	}, nil
}

func (s *O365MBXService) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/")
	parts := strings.SplitN(path, "/", 2)

	if len(parts) == 0 || parts[0] == "" {
		s.WriteError(w, http.StatusBadRequest, "Endpoint required. Use /op/{name}")
		return
	}

	if parts[0] != "op" {
		s.WriteError(w, http.StatusNotFound, "Unknown endpoint")
		return
	}

	if len(parts) < 2 || parts[1] == "" {
		s.WriteError(w, http.StatusBadRequest, "Operation name required")
		return
	}

	opName := parts[1]
	s.handleNamedOperation(w, r, opName)
}

func (s *O365MBXService) handleNamedOperation(w http.ResponseWriter, r *http.Request, opName string) {
	op, exists := s.config.Settings.Operations[opName]
	if !exists {
		s.WriteError(w, http.StatusNotFound, fmt.Sprintf("Operation '%s' not found", opName))
		return
	}

	var reqBody map[string]any
	if r.Method == http.MethodPost || r.Method == http.MethodPut {
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil && err.Error() != "EOF" {
			s.WriteError(w, http.StatusBadRequest, "Invalid request body")
			return
		}
	}
	if reqBody == nil {
		reqBody = make(map[string]any)
	}

	// Apply param defaults & check required params
	for name, paramDef := range op.Params {
		val, found := reqBody[name]
		if !found {
			if paramDef.Default != nil {
				reqBody[name] = paramDef.Default
			} else if paramDef.Required {
				s.WriteError(w, http.StatusBadRequest, fmt.Sprintf("Missing required param: %s", name))
				return
			}
		} else if paramDef.Required && val == "" {
			s.WriteError(w, http.StatusBadRequest, fmt.Sprintf("Param '%s' cannot be empty", name))
			return
		}
	}

	// Validate path params
	if ws, ok := reqBody["workspace"].(string); ok && ws != "" {
		if err := validatePath(ws); err != nil {
			s.WriteError(w, http.StatusBadRequest, fmt.Sprintf("workspace path: %v", err))
			return
		}
	}
	if sf, ok := reqBody["state_file"].(string); ok && sf != "" {
		if err := validatePath(sf); err != nil {
			s.WriteError(w, http.StatusBadRequest, fmt.Sprintf("state_file path: %v", err))
			return
		}
	}

	// Build engine.Config
	engineCfg, err := s.buildEngineConfig(r.Context(), opName, op, reqBody)
	if err != nil {
		s.WriteError(w, http.StatusInternalServerError, fmt.Sprintf("failed to build configuration: %v", err))
		return
	}

	timeout := time.Duration(s.config.Settings.Defaults.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 300 * time.Second
	}
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()

	var outBuf bytes.Buffer
	dl, err := downloader.New(engineCfg, nil)
	if err != nil {
		s.WriteError(w, http.StatusBadRequest, fmt.Sprintf("invalid engine config: %v", err))
		return
	}

	if err := dl.Execute(ctx, &outBuf); err != nil {
		s.WriteError(w, http.StatusInternalServerError, fmt.Sprintf("execution failed: %v", err))
		return
	}

	s.WriteJSON(w, http.StatusOK, map[string]any{
		"status":    "success",
		"operation": opName,
		"mailbox":   s.config.Settings.Mailbox,
		"output":    outBuf.String(),
	})
}

func (s *O365MBXService) buildEngineConfig(ctx context.Context, opName string, op Operation, reqBody map[string]any) (*engine.Config, error) {
	cfg := &engine.Config{
		MailboxName: s.config.Settings.Mailbox,
	}

	// Credential resolution
	if s.config.Settings.TenantID != "" {
		cfg.TenantID = s.config.Settings.TenantID

		// Client ID
		if s.config.Settings.EncryptedClientID != "" {
			decID, err := crypto.Decrypt(s.config.Settings.EncryptedClientID)
			if err != nil {
				return nil, fmt.Errorf("decrypt client_id: %w", err)
			}
			cfg.ClientID = decID
		} else {
			cfg.ClientID = s.config.Settings.ClientID
		}

		// Client Secret
		if s.config.Settings.EncryptedClientSecret != "" {
			decSecret, err := crypto.Decrypt(s.config.Settings.EncryptedClientSecret)
			if err != nil {
				return nil, fmt.Errorf("decrypt client_secret: %w", err)
			}
			cfg.ClientSecret = decSecret
		} else {
			cfg.ClientSecret = s.config.Settings.ClientSecret
		}
	} else if s.config.Settings.EncryptedToken != "" {
		decToken, err := crypto.Decrypt(s.config.Settings.EncryptedToken)
		if err != nil {
			return nil, fmt.Errorf("decrypt token: %w", err)
		}
		cfg.TokenString = decToken
	} else if s.config.Settings.TokenString != "" {
		cfg.TokenString = s.config.Settings.TokenString
	}

	// Defaults
	defaults := s.config.Settings.Defaults
	cfg.MaxParallelDownloads = defaults.Parallel
	cfg.APICallsPerSecond = defaults.APIRate
	cfg.APIBurst = defaults.APIBurst
	cfg.MaxRetries = defaults.MaxRetries
	cfg.HTTPClientTimeoutSeconds = defaults.TimeoutSeconds
	cfg.MaxExecutionTimeMsg = defaults.MaxExecutionTimeMsg
	cfg.ChromiumPath = defaults.ChromiumPath

	// Operation specifics
	cfg.ProcessingMode = op.ProcessingMode
	if cfg.ProcessingMode == "" {
		cfg.ProcessingMode = "full"
	}
	if opName == "healthcheck" || op.ProcessingMode == "healthcheck" {
		cfg.HealthCheck = true
	}

	cfg.InboxFolder = op.InboxFolder
	cfg.ProcessedFolder = op.ProcessedFolder
	cfg.ErrorFolder = op.ErrorFolder
	cfg.ConvertBody = op.ConvertBody

	if ws, ok := reqBody["workspace"].(string); ok {
		cfg.WorkspacePath = ws
	}
	if sf, ok := reqBody["state_file"].(string); ok {
		cfg.StateFilePath = sf
	}
	if f, ok := reqBody["folder"].(string); ok && f != "" {
		cfg.InboxFolder = f
	}

	return cfg, nil
}

func validatePath(path string) error {
	if !filepath.IsAbs(path) {
		return fmt.Errorf("path must be absolute: %s", path)
	}
	if strings.ContainsAny(path, "\x00") {
		return fmt.Errorf("path contains null byte")
	}
	cleaned := filepath.Clean(path)
	if cleaned != path && cleaned+"/" != path {
		return fmt.Errorf("path must be canonical (no .. or //): %s", path)
	}
	return nil
}

func (s *O365MBXService) HealthCheck(ctx context.Context) error {
	cfg, err := s.buildEngineConfig(ctx, "healthcheck", Operation{ProcessingMode: "healthcheck"}, nil)
	if err != nil {
		return fmt.Errorf("healthcheck config error: %w", err)
	}
	cfg.HealthCheck = true
	dl, err := downloader.New(cfg, nil)
	if err != nil {
		return fmt.Errorf("healthcheck downloader init error: %w", err)
	}
	var out bytes.Buffer
	if err := dl.Execute(ctx, &out); err != nil {
		return fmt.Errorf("healthcheck execution error: %w", err)
	}
	return nil
}

func (s *O365MBXService) Close() error {
	return nil
}
