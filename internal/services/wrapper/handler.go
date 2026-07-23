package wrapper

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/edsilegxrepo/service-gateway/internal/services"
)

func init() {
	services.RegisterType("binary-wrapper", NewWrapperService)
}

type WrapperService struct {
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
	Executable     string            `json:"executable"`
	WorkingDir     string            `json:"working_dir"`
	TimeoutSeconds int               `json:"timeout_seconds"`
	AllowedArgs    []string          `json:"allowed_args"`
	Env            map[string]string `json:"env"`
}

func NewWrapperService(name string, rawConfig json.RawMessage) (services.Service, error) {
	var cfg Config
	if err := json.Unmarshal(rawConfig, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	if !filepath.IsAbs(cfg.Settings.Executable) {
		return nil, fmt.Errorf("executable path must be absolute: %s", cfg.Settings.Executable)
	}

	return &WrapperService{
		BaseService: services.NewBaseService(name, "binary-wrapper"),
		config:      cfg,
	}, nil
}

func (s *WrapperService) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/")

	if path != "exec" && path != "" {
		s.WriteError(w, http.StatusNotFound, "Unknown operation")
		return
	}

	if r.Method != http.MethodPost {
		s.WriteError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}

	var req struct {
		Args []string `json:"args"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if err := s.validateArgs(req.Args); err != nil {
		s.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	timeout := time.Duration(s.config.Settings.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 60 * time.Second
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()

	// #nosec G204 -- Executable from admin service config (validated path), Args validated against AllowedArgs whitelist
	cmd := exec.CommandContext(ctx, s.config.Settings.Executable, req.Args...)

	if s.config.Settings.WorkingDir != "" {
		cmd.Dir = s.config.Settings.WorkingDir
	}

	if len(s.config.Settings.Env) > 0 {
		cmd.Env = append(os.Environ(), formatEnv(s.config.Settings.Env)...)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	err := cmd.Run()
	duration := time.Since(start)

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else if ctx.Err() == context.DeadlineExceeded {
			s.WriteError(w, http.StatusGatewayTimeout, "Command timed out")
			return
		} else {
			s.WriteError(w, http.StatusInternalServerError, "Command failed: "+err.Error())
			return
		}
	}

	s.WriteJSON(w, http.StatusOK, map[string]any{
		"exit_code":   exitCode,
		"stdout":      stdout.String(),
		"stderr":      stderr.String(),
		"duration_ms": duration.Milliseconds(),
	})
}

func (s *WrapperService) validateArgs(args []string) error {
	if len(s.config.Settings.AllowedArgs) == 0 {
		return nil
	}

	allowedSet := make(map[string]bool)
	for _, a := range s.config.Settings.AllowedArgs {
		allowedSet[a] = true
	}

	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			parts := strings.SplitN(arg, "=", 2)
			flag := parts[0]
			if !allowedSet[flag] {
				return fmt.Errorf("argument %q not allowed", flag)
			}
		} else {
			if !allowedSet[arg] {
				return fmt.Errorf("argument %q not allowed", arg)
			}
		}

		if containsShellMeta(arg) {
			return fmt.Errorf("argument %q contains shell metacharacters", arg)
		}
	}

	return nil
}

func containsShellMeta(s string) bool {
	shellMeta := []string{";", "|", "&", "$", "`", "(", ")", "{", "}", "<", ">", "\n", "\r"}
	for _, meta := range shellMeta {
		if strings.Contains(s, meta) {
			return true
		}
	}
	return false
}

func (s *WrapperService) HealthCheck(ctx context.Context) error {
	return nil
}

func (s *WrapperService) Close() error {
	return nil
}

func formatEnv(env map[string]string) []string {
	res := make([]string, 0, len(env))
	for k, v := range env {
		res = append(res, k+"="+v)
	}
	return res
}
