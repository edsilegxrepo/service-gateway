package o365mbx

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/edsilegxrepo/service-gateway/internal/services"
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
	TenantID              string               `json:"tenant_id"`
	EncryptedClientID     string               `json:"encrypted_client_id"`
	EncryptedClientSecret string               `json:"encrypted_client_secret"`
	Mailbox               string               `json:"mailbox"`
	Operations            map[string]Operation `json:"operations"`
}

type Operation struct {
	Description string           `json:"description"`
	Action      string           `json:"action"`
	Folder      string           `json:"folder"`
	Params      map[string]Param `json:"params"`
}

type Param struct {
	Type     string `json:"type"`
	Required bool   `json:"required"`
	Default  any    `json:"default"`
}

func NewO365MBXService(name string, rawConfig json.RawMessage) (services.Service, error) {
	var cfg Config
	if err := json.Unmarshal(rawConfig, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	return &O365MBXService{
		BaseService: services.NewBaseService(name, "o365mbx"),
		config:      cfg,
	}, nil
}

func (s *O365MBXService) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.WriteError(w, http.StatusNotImplemented,
		"O365MBX service is a placeholder. Implement using msgraph-sdk-go (https://github.com/microsoftgraph/msgraph-sdk-go)")
}

func (s *O365MBXService) HealthCheck(ctx context.Context) error {
	return nil
}

func (s *O365MBXService) Close() error {
	return nil
}
