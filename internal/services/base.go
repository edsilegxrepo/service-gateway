package services

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type BaseService struct {
	name        string
	serviceType string
}

func NewBaseService(name, serviceType string) BaseService {
	return BaseService{
		name:        name,
		serviceType: serviceType,
	}
}

func (s *BaseService) Name() string {
	return s.name
}

func (s *BaseService) Type() string {
	return s.serviceType
}

func (s *BaseService) WriteJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		http.Error(w, "encoding error", http.StatusInternalServerError)
	}
}

func (s *BaseService) WriteError(w http.ResponseWriter, status int, message string) {
	s.WriteJSON(w, status, map[string]string{"error": message})
}

func (s *BaseService) StreamJSON(w http.ResponseWriter, emit func(func(any) error) error) error {
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Transfer-Encoding", "chunked")
	w.WriteHeader(http.StatusOK)

	flusher, ok := w.(http.Flusher)
	if !ok {
		return fmt.Errorf("streaming not supported")
	}

	encoder := json.NewEncoder(w)
	err := emit(func(item any) error {
		if err := encoder.Encode(item); err != nil {
			return err
		}
		flusher.Flush()
		return nil
	})

	return err
}

func (s *BaseService) WithTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return context.WithTimeout(ctx, timeout)
}
