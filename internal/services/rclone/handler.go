package rclone

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/rclone/rclone/librclone/librclone"

	"criticalsys.net/service-gateway/internal/crypto"
	"criticalsys.net/service-gateway/internal/services"
)

func init() {
	services.RegisterType("rclone", NewRcloneService)
}

type RcloneService struct {
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
	ConfigFile          string               `json:"config_file"`
	EncryptedConfigPass string               `json:"encrypted_config_pass"`
	RcloneOptions       RcloneOptions        `json:"rclone_options"`
	Operations          map[string]Operation `json:"operations"`
	RawAccess           RawAccess            `json:"raw_access"`
}

type RcloneOptions struct {
	UseMmap        bool   `json:"use_mmap"`
	NoCheckDest    bool   `json:"no_check_dest"`
	CacheWorkers   int    `json:"cache_workers"`
	Checkers       int    `json:"checkers"`
	Transfers      int    `json:"transfers"`
	Retries        int    `json:"retries"`
	MaxBacklog     int    `json:"max_backlog"`
	BandwidthLimit string `json:"bandwidth_limit"`
	TimeoutSeconds int    `json:"timeout_seconds"`
}

type Operation struct {
	Description string           `json:"description"`
	Operation   string           `json:"operation"`
	SrcRemote   string           `json:"src_remote"`
	DstRemote   string           `json:"dst_remote"`
	DstBucket   string           `json:"dst_bucket"`
	Remote      string           `json:"remote"`
	Params      map[string]Param `json:"params"`
}

type Param struct {
	Type        string `json:"type"`
	Required    bool   `json:"required"`
	Default     any    `json:"default"`
	Description string `json:"description"`
}

type RawAccess struct {
	Enabled           bool     `json:"enabled"`
	AllowedRemotes    []string `json:"allowed_remotes"`
	AllowedOperations []string `json:"allowed_operations"`
}

func NewRcloneService(name string, rawConfig json.RawMessage) (services.Service, error) {
	var cfg Config
	if err := json.Unmarshal(rawConfig, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	if cfg.Settings.ConfigFile != "" {
		if err := validateFilePath(cfg.Settings.ConfigFile); err != nil {
			return nil, fmt.Errorf("config_file: %w", err)
		}
	}

	librclone.Initialize()

	if cfg.Settings.EncryptedConfigPass != "" {
		pass, err := crypto.Decrypt(cfg.Settings.EncryptedConfigPass)
		if err != nil {
			return nil, fmt.Errorf("decrypt config pass: %w", err)
		}
		librclone.RPC("config/setpath", fmt.Sprintf(`{"path":%q}`, cfg.Settings.ConfigFile))
		if pass != "" {
			librclone.RPC("config/password", fmt.Sprintf(`{"pass":%q}`, pass))
		}
	}

	return &RcloneService{
		BaseService: services.NewBaseService(name, "rclone"),
		config:      cfg,
	}, nil
}

func (s *RcloneService) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/")
	parts := strings.SplitN(path, "/", 2)

	if len(parts) == 0 || parts[0] == "" {
		s.WriteError(w, http.StatusBadRequest, "Operation required")
		return
	}

	switch parts[0] {
	case "op":
		if len(parts) < 2 {
			s.WriteError(w, http.StatusBadRequest, "Operation name required")
			return
		}
		s.handleNamedOperation(w, r, parts[1])
	case "raw":
		s.handleRaw(w, r)
	default:
		s.WriteError(w, http.StatusNotFound, "Unknown endpoint")
	}
}

func (s *RcloneService) handleNamedOperation(w http.ResponseWriter, r *http.Request, opName string) {
	op, exists := s.config.Settings.Operations[opName]
	if !exists {
		s.WriteError(w, http.StatusNotFound, "Operation not found")
		return
	}

	var params map[string]any
	if r.Method == http.MethodPost {
		if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
			s.WriteError(w, http.StatusBadRequest, "Invalid request body")
			return
		}
	}

	if err := validateParams(params, op.Params); err != nil {
		s.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := validatePathParams(params); err != nil {
		s.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	for name, def := range op.Params {
		if _, exists := params[name]; !exists && def.Default != nil {
			params[name] = def.Default
		}
	}

	timeout := time.Duration(s.config.Settings.RcloneOptions.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 300 * time.Second
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()

	var result string
	var err error

	switch op.Operation {
	case "move":
		result, err = s.executeMove(ctx, op, params)
	case "copy":
		result, err = s.executeCopy(ctx, op, params)
	case "sync":
		result, err = s.executeSync(ctx, op, params)
	case "list":
		result, err = s.executeList(ctx, op, params)
	case "delete":
		result, err = s.executeDelete(ctx, op, params)
	default:
		s.WriteError(w, http.StatusBadRequest, "Unsupported operation: "+op.Operation)
		return
	}

	if err != nil {
		s.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	// nosemgrep: go.lang.security.audit.xss.no-direct-write-to-responsewriter.no-direct-write-to-responsewriter
	if _, err := w.Write([]byte(result)); err != nil {
		s.WriteError(w, http.StatusInternalServerError, "write error: "+err.Error())
	}
}

func (s *RcloneService) executeMove(ctx context.Context, op Operation, params map[string]any) (string, error) {
	srcPath := fmt.Sprintf("%s:%v", op.SrcRemote, params["src_folder"])
	dstPath := fmt.Sprintf("%s:%s", op.DstRemote, op.DstBucket)

	input := map[string]any{
		"srcFs":              srcPath,
		"dstFs":              dstPath,
		"deleteEmptySrcDirs": true,
	}

	inputJSON, _ := json.Marshal(input)
	out, status := librclone.RPC("sync/move", string(inputJSON))

	if status != 200 {
		return "", fmt.Errorf("rclone move failed: %s", out)
	}

	return out, nil
}

func (s *RcloneService) executeCopy(ctx context.Context, op Operation, params map[string]any) (string, error) {
	srcPath := fmt.Sprintf("%s:%v", op.SrcRemote, params["src_path"])
	dstPath := fmt.Sprintf("%s:%v", op.DstRemote, params["dst_path"])

	input := map[string]any{
		"srcFs": srcPath,
		"dstFs": dstPath,
	}

	inputJSON, _ := json.Marshal(input)
	out, status := librclone.RPC("sync/copy", string(inputJSON))

	if status != 200 {
		return "", fmt.Errorf("rclone copy failed: %s", out)
	}

	return out, nil
}

func (s *RcloneService) executeSync(ctx context.Context, op Operation, params map[string]any) (string, error) {
	srcPath := fmt.Sprintf("%s:%v", op.SrcRemote, params["src_path"])
	dstPath := fmt.Sprintf("%s:%v", op.DstRemote, params["dst_path"])

	input := map[string]any{
		"srcFs": srcPath,
		"dstFs": dstPath,
	}

	inputJSON, _ := json.Marshal(input)
	out, status := librclone.RPC("sync/sync", string(inputJSON))

	if status != 200 {
		return "", fmt.Errorf("rclone sync failed: %s", out)
	}

	return out, nil
}

func (s *RcloneService) executeList(ctx context.Context, op Operation, params map[string]any) (string, error) {
	remote := op.Remote
	if remote == "" {
		remote = op.SrcRemote
	}

	remotePath := fmt.Sprintf("%s:%v", remote, params["path"])

	input := map[string]any{
		"fs":     remotePath,
		"remote": "",
	}

	if recursive, ok := params["recursive"].(bool); ok && recursive {
		input["recurse"] = true
	}

	inputJSON, _ := json.Marshal(input)
	out, status := librclone.RPC("operations/list", string(inputJSON))

	if status != 200 {
		return "", fmt.Errorf("rclone list failed: %s", out)
	}

	return out, nil
}

func (s *RcloneService) executeDelete(ctx context.Context, op Operation, params map[string]any) (string, error) {
	remote := op.Remote
	if remote == "" {
		remote = op.SrcRemote
	}

	remotePath := fmt.Sprintf("%s:%v", remote, params["path"])

	input := map[string]any{
		"fs":     remotePath,
		"remote": "",
	}

	inputJSON, _ := json.Marshal(input)
	out, status := librclone.RPC("operations/purge", string(inputJSON))

	if status != 200 {
		return "", fmt.Errorf("rclone delete failed: %s", out)
	}

	return out, nil
}

func (s *RcloneService) handleRaw(w http.ResponseWriter, r *http.Request) {
	if !s.config.Settings.RawAccess.Enabled {
		s.WriteError(w, http.StatusForbidden, "Raw access disabled")
		return
	}

	var req struct {
		Method string `json:"method"`
		Params string `json:"params"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if !s.isOperationAllowed(req.Method) {
		s.WriteError(w, http.StatusForbidden, "Operation not allowed")
		return
	}

	out, status := librclone.RPC(req.Method, req.Params)

	w.Header().Set("Content-Type", "application/json")
	if status != 200 {
		w.WriteHeader(http.StatusInternalServerError)
	} else {
		w.WriteHeader(http.StatusOK)
	}
	// nosemgrep: go.lang.security.audit.xss.no-direct-write-to-responsewriter.no-direct-write-to-responsewriter
	if _, err := w.Write([]byte(out)); err != nil {
		s.WriteError(w, http.StatusInternalServerError, "write error: "+err.Error())
	}
}

func (s *RcloneService) isOperationAllowed(method string) bool {
	for _, allowed := range s.config.Settings.RawAccess.AllowedOperations {
		if allowed == method || allowed == "*" {
			return true
		}
	}
	return false
}

func (s *RcloneService) HealthCheck(ctx context.Context) error {
	out, status := librclone.RPC("core/version", "{}")
	if status != 200 {
		return fmt.Errorf("rclone health check failed: %s", out)
	}
	return nil
}

func (s *RcloneService) Close() error {
	librclone.Finalize()
	return nil
}

func validateParams(params map[string]any, defs map[string]Param) error {
	for name, def := range defs {
		_, exists := params[name]
		if def.Required && !exists {
			return fmt.Errorf("missing required parameter: %s", name)
		}
	}
	return nil
}

func validateFilePath(path string) error {
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

func validatePathParams(params map[string]any) error {
	pathKeys := []string{"path", "src_path", "dst_path", "src_folder", "dst_folder"}
	for _, key := range pathKeys {
		if val, ok := params[key]; ok {
			if str, isStr := val.(string); isStr {
				if err := validateRemotePath(str); err != nil {
					return fmt.Errorf("parameter %s: %w", key, err)
				}
			}
		}
	}
	return nil
}

func validateRemotePath(path string) error {
	if strings.Contains(path, "\x00") {
		return fmt.Errorf("path contains null byte")
	}
	if strings.Contains(path, "..") {
		return fmt.Errorf("path traversal not allowed: %s", path)
	}
	return nil
}
