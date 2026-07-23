package services

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestBaseServiceWriteJSON(t *testing.T) {
	bs := NewBaseService("test", "test-type")

	rr := httptest.NewRecorder()
	bs.WriteJSON(rr, http.StatusOK, map[string]string{"key": "value"})

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}

	ct := rr.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %s, want application/json", ct)
	}

	var result map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	if result["key"] != "value" {
		t.Errorf("response key = %s, want value", result["key"])
	}
}

func TestBaseServiceWriteError(t *testing.T) {
	bs := NewBaseService("test", "test-type")

	rr := httptest.NewRecorder()
	bs.WriteError(rr, http.StatusBadRequest, "something went wrong")

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}

	var result map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if result["error"] != "something went wrong" {
		t.Errorf("error = %s, want 'something went wrong'", result["error"])
	}
}

func TestBaseServiceStreamJSON(t *testing.T) {
	bs := NewBaseService("test", "test-type")

	rr := httptest.NewRecorder()
	items := []map[string]int{{"a": 1}, {"b": 2}, {"c": 3}}

	err := bs.StreamJSON(rr, func(emit func(any) error) error {
		for _, item := range items {
			if err := emit(item); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("StreamJSON failed: %v", err)
	}

	ct := rr.Header().Get("Content-Type")
	if ct != "application/x-ndjson" {
		t.Errorf("Content-Type = %s, want application/x-ndjson", ct)
	}

	lines := strings.Split(strings.TrimSpace(rr.Body.String()), "\n")
	if len(lines) != 3 {
		t.Errorf("expected 3 lines, got %d", len(lines))
	}
}

func TestBaseServiceWithTimeout(t *testing.T) {
	bs := NewBaseService("test", "test-type")

	ctx, cancel := bs.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	deadline, ok := ctx.Deadline()
	if !ok {
		t.Error("expected deadline to be set")
	}

	if time.Until(deadline) > 100*time.Millisecond {
		t.Error("deadline too far in future")
	}
}

func TestBaseServiceWithTimeoutDefault(t *testing.T) {
	bs := NewBaseService("test", "test-type")

	ctx, cancel := bs.WithTimeout(context.Background(), 0)
	defer cancel()

	deadline, ok := ctx.Deadline()
	if !ok {
		t.Error("expected deadline to be set")
	}

	if time.Until(deadline) > 30*time.Second {
		t.Error("default timeout should be 30s")
	}
}

func TestBaseServiceNameType(t *testing.T) {
	bs := NewBaseService("my-service", "my-type")

	if bs.Name() != "my-service" {
		t.Errorf("Name() = %s, want my-service", bs.Name())
	}

	if bs.Type() != "my-type" {
		t.Errorf("Type() = %s, want my-type", bs.Type())
	}
}
