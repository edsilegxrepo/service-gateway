package services

import (
	"context"
	"net/http"
)

type Service interface {
	Name() string
	Type() string
	ServeHTTP(w http.ResponseWriter, r *http.Request)
	HealthCheck(ctx context.Context) error
	Close() error
}

type ServiceConfig struct {
	Type        string `json:"type"`
	Description string `json:"description"`
	Version     string `json:"version"`
}
