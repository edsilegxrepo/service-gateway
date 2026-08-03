package pdfextractor

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"criticalsys.net/pdf-extractor/pkg/extractor"

	"criticalsys.net/service-gateway/internal/services"
)

func init() {
	services.RegisterType("pdf-extractor", NewPDFExtractorService)
}

type PDFExtractorService struct {
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
	WorkspacePath  string `json:"workspace_path"`
	FilePattern    string `json:"file_pattern"`
	SearchPattern  string `json:"search_pattern"`
	MutoolBin      string `json:"mutool_bin"`
	TimeoutSeconds int    `json:"timeout_seconds"`
	Workers        int    `json:"workers"`
}

func NewPDFExtractorService(name string, rawConfig json.RawMessage) (services.Service, error) {
	var cfg Config
	if err := json.Unmarshal(rawConfig, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	if cfg.Settings.WorkspacePath == "" {
		return nil, fmt.Errorf("workspace_path is required")
	}

	if err := validatePath(cfg.Settings.WorkspacePath); err != nil {
		return nil, fmt.Errorf("workspace_path: %w", err)
	}

	if cfg.Settings.MutoolBin != "" {
		if err := validatePath(cfg.Settings.MutoolBin); err != nil {
			return nil, fmt.Errorf("mutool_bin: %w", err)
		}
	}

	if cfg.Settings.FilePattern == "" {
		cfg.Settings.FilePattern = "*.pdf"
	}

	return &PDFExtractorService{
		BaseService: services.NewBaseService(name, "pdf-extractor"),
		config:      cfg,
	}, nil
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

func (s *PDFExtractorService) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/")

	if path != "extract" && path != "" {
		s.WriteError(w, http.StatusNotFound, "Unknown operation. Use /extract")
		return
	}

	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		s.WriteError(w, http.StatusMethodNotAllowed, "GET or POST required")
		return
	}

	var req struct {
		FilePattern   string `json:"file_pattern"`
		SearchPattern string `json:"search_pattern"`
	}

	if r.Method == http.MethodPost {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.WriteError(w, http.StatusBadRequest, "Invalid request body")
			return
		}
	}

	filePattern := req.FilePattern
	if filePattern == "" {
		filePattern = s.config.Settings.FilePattern
	}

	searchPattern := req.SearchPattern
	if searchPattern == "" {
		searchPattern = s.config.Settings.SearchPattern
	}

	if searchPattern == "" {
		s.WriteError(w, http.StatusBadRequest, "search_pattern is required")
		return
	}

	timeout := time.Duration(s.config.Settings.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	opts := extractor.Options{
		Path:        s.config.Settings.WorkspacePath,
		FilePattern: filePattern,
		Search:      searchPattern,
		MutoolBin:   s.config.Settings.MutoolBin,
		Timeout:     timeout,
		Workers:     s.config.Settings.Workers,
	}

	results, err := extractor.Extract(r.Context(), opts)
	if err != nil {
		s.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.WriteJSON(w, http.StatusOK, map[string]any{
		"count":   len(results),
		"results": results,
	})
}

func (s *PDFExtractorService) HealthCheck(ctx context.Context) error {
	return nil
}

func (s *PDFExtractorService) Close() error {
	return nil
}
