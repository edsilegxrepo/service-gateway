package services

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
)

type Registry struct {
	mu          sync.RWMutex
	reloadMu    sync.Mutex
	services    map[string]Service
	servicesDir string
	logger      *slog.Logger
}

func NewRegistry(logger *slog.Logger) *Registry {
	return &Registry{
		services: make(map[string]Service),
		logger:   logger,
	}
}

func (r *Registry) LoadServices(dir string) error {
	r.reloadMu.Lock()
	defer r.reloadMu.Unlock()

	r.servicesDir = dir

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			r.logger.Warn("services directory not found", "path", dir)
			return nil
		}
		return fmt.Errorf("read services dir: %w", err)
	}

	newServices := make(map[string]Service)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		name := entry.Name()
		configPath := filepath.Join(dir, name, "config.json")

		// #nosec G304 -- configPath constructed from admin-controlled services_dir config
		data, err := os.ReadFile(configPath)
		if err != nil {
			r.logger.Error("failed to read service config", "name", name, "error", err)
			continue
		}

		expandedData := os.ExpandEnv(string(data))
		svc, err := CreateService(name, json.RawMessage(expandedData))
		if err != nil {
			r.logger.Error("failed to load service", "name", name, "error", err)
			continue
		}

		newServices[name] = svc
		r.logger.Info("loaded service", "name", name)
	}

	r.mu.Lock()
	oldServices := r.services
	r.services = make(map[string]Service, len(newServices))
	for name, svc := range newServices {
		r.services[name] = svc
	}
	r.mu.Unlock()

	for name, oldSvc := range oldServices {
		if newSvc, kept := newServices[name]; !kept || newSvc != oldSvc {
			_ = oldSvc.Close()
			if !kept {
				r.logger.Info("unloaded removed service", "name", name)
			}
		}
	}

	return nil
}

func (r *Registry) Get(name string) (Service, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	svc, exists := r.services[name]
	return svc, exists
}

func (r *Registry) Services() map[string]Service {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[string]Service, len(r.services))
	for k, v := range r.services {
		result[k] = v
	}
	return result
}

func (r *Registry) Reload() error {
	r.logger.Info("reloading services")
	return r.LoadServices(r.servicesDir)
}

func (r *Registry) CloseAll() {
	r.mu.Lock()
	defer r.mu.Unlock()

	for name, svc := range r.services {
		if err := svc.Close(); err != nil {
			r.logger.Error("error closing service", "name", name, "error", err)
		}
	}
}
