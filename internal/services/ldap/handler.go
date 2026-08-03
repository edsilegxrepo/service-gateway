package ldap

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"

	"github.com/go-ldap/ldap/v3"

	"criticalsys.net/service-gateway/internal/crypto"
	"criticalsys.net/service-gateway/internal/services"
)

func init() {
	services.RegisterType("ldap", NewLDAPService)
}

type LDAPService struct {
	services.BaseService
	config    Config
	tlsConfig *tls.Config
	bindDN    string
	bindPwd   string
}

type Config struct {
	Type        string   `json:"type"`
	Description string   `json:"description"`
	Version     string   `json:"version"`
	Settings    Settings `json:"settings"`
}

type Settings struct {
	Server                string               `json:"server"`
	EncryptedBindDN       string               `json:"encrypted_bind_dn"`
	EncryptedBindPassword string               `json:"encrypted_bind_password"`
	TLS                   TLSSettings          `json:"tls"`
	Connection            ConnectionSettings   `json:"connection"`
	BaseDN                string               `json:"base_dn"`
	Operations            map[string]Operation `json:"operations"`
	RawSearch             RawSearchSettings    `json:"raw_search"`
}

type TLSSettings struct {
	StartTLS   bool   `json:"start_tls"`
	CACert     string `json:"ca_cert"`
	SkipVerify bool   `json:"skip_verify"`
}

type ConnectionSettings struct {
	TimeoutSeconds int `json:"timeout_seconds"`
	PoolSize       int `json:"pool_size"`
}

type Operation struct {
	Description string           `json:"description"`
	SearchBase  string           `json:"search_base"`
	Filter      string           `json:"filter"`
	Attributes  []string         `json:"attributes"`
	Params      map[string]Param `json:"params"`
	SizeLimit   int              `json:"size_limit"`
}

type Param struct {
	Type     string `json:"type"`
	Required bool   `json:"required"`
	Pattern  string `json:"pattern"`
}

type RawSearchSettings struct {
	Enabled bool `json:"enabled"`
}

func NewLDAPService(name string, rawConfig json.RawMessage) (services.Service, error) {
	var cfg Config
	if err := json.Unmarshal(rawConfig, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	isLDAPS := strings.HasPrefix(cfg.Settings.Server, "ldaps://")
	useStartTLS := cfg.Settings.TLS.StartTLS

	if !isLDAPS && !useStartTLS {
		return nil, fmt.Errorf("TLS required: use ldaps:// or enable start_tls")
	}

	bindDN, err := crypto.Decrypt(cfg.Settings.EncryptedBindDN)
	if err != nil {
		return nil, fmt.Errorf("decrypt bind_dn: %w", err)
	}

	bindPwd, err := crypto.Decrypt(cfg.Settings.EncryptedBindPassword)
	if err != nil {
		return nil, fmt.Errorf("decrypt bind_password: %w", err)
	}

	tlsConfig := &tls.Config{
		// #nosec G402 -- skip_verify is admin config option for dev/testing with self-signed certs
		InsecureSkipVerify: cfg.Settings.TLS.SkipVerify,
		MinVersion:         tls.VersionTLS12,
	}

	if cfg.Settings.TLS.CACert != "" {
		caCert, err := os.ReadFile(cfg.Settings.TLS.CACert)
		if err != nil {
			return nil, fmt.Errorf("read CA cert: %w", err)
		}
		tlsConfig.RootCAs = x509.NewCertPool()
		tlsConfig.RootCAs.AppendCertsFromPEM(caCert)
	}

	svc := &LDAPService{
		BaseService: services.NewBaseService(name, "ldap"),
		config:      cfg,
		tlsConfig:   tlsConfig,
		bindDN:      bindDN,
		bindPwd:     bindPwd,
	}

	if err := svc.HealthCheck(context.Background()); err != nil {
		return nil, fmt.Errorf("initial connection test: %w", err)
	}

	return svc, nil
}

func (s *LDAPService) connect() (*ldap.Conn, error) {
	serverURL := s.config.Settings.Server
	if !strings.HasPrefix(serverURL, "ldap://") && !strings.HasPrefix(serverURL, "ldaps://") {
		serverURL = "ldap://" + serverURL
	}

	conn, err := ldap.DialURL(serverURL, ldap.DialWithTLSConfig(s.tlsConfig))
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}

	if !strings.HasPrefix(s.config.Settings.Server, "ldaps://") && s.config.Settings.TLS.StartTLS {
		if err := conn.StartTLS(s.tlsConfig); err != nil {
			_ = conn.Close() // #nosec G104 -- cleanup on error path
			return nil, fmt.Errorf("starttls: %w", err)
		}
	}

	if err := conn.Bind(s.bindDN, s.bindPwd); err != nil {
		_ = conn.Close() // #nosec G104 -- cleanup on error path
		return nil, fmt.Errorf("bind: %w", err)
	}

	return conn, nil
}

func (s *LDAPService) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/")
	parts := strings.SplitN(path, "/", 2)

	if len(parts) < 2 || parts[0] != "op" {
		s.WriteError(w, http.StatusBadRequest, "Operation required: /op/{operation_name}")
		return
	}

	opName := parts[1]
	op, exists := s.config.Settings.Operations[opName]
	if !exists {
		s.WriteError(w, http.StatusNotFound, "Operation not found")
		return
	}

	var params map[string]string
	if r.Method == http.MethodPost {
		if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
			s.WriteError(w, http.StatusBadRequest, "Invalid request body")
			return
		}
	}

	if err := s.validateParams(params, op.Params); err != nil {
		s.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	filter := s.interpolateFilter(op.Filter, params)

	conn, err := s.connect()
	if err != nil {
		s.WriteError(w, http.StatusServiceUnavailable, "LDAP connection failed: "+err.Error())
		return
	}
	defer conn.Close() //nolint:errcheck // cleanup in defer

	searchBase := op.SearchBase
	if searchBase == "" {
		searchBase = s.config.Settings.BaseDN
	}

	timeout := s.config.Settings.Connection.TimeoutSeconds
	if timeout <= 0 {
		timeout = 30
	}

	sizeLimit := op.SizeLimit
	if sizeLimit <= 0 {
		sizeLimit = 500
	}

	searchReq := ldap.NewSearchRequest(
		searchBase,
		ldap.ScopeWholeSubtree,
		ldap.NeverDerefAliases,
		sizeLimit,
		timeout,
		false,
		filter,
		op.Attributes,
		nil,
	)

	result, err := conn.Search(searchReq)
	if err != nil {
		s.WriteError(w, http.StatusInternalServerError, "Search failed: "+err.Error())
		return
	}

	entries := make([]map[string]any, 0, len(result.Entries))
	for _, entry := range result.Entries {
		attrs := make(map[string]any)
		for _, attr := range entry.Attributes {
			if len(attr.Values) == 1 {
				attrs[attr.Name] = attr.Values[0]
			} else {
				attrs[attr.Name] = attr.Values
			}
		}
		entries = append(entries, map[string]any{
			"dn":         entry.DN,
			"attributes": attrs,
		})
	}

	s.WriteJSON(w, http.StatusOK, map[string]any{
		"found":   len(entries) > 0,
		"count":   len(entries),
		"entries": entries,
	})
}

func (s *LDAPService) validateParams(params map[string]string, defs map[string]Param) error {
	for name, def := range defs {
		val, exists := params[name]
		if def.Required && !exists {
			return fmt.Errorf("missing required parameter: %s", name)
		}
		if exists && def.Pattern != "" {
			matched, _ := regexp.MatchString(def.Pattern, val)
			if !matched {
				return fmt.Errorf("parameter %s does not match pattern", name)
			}
		}
	}
	return nil
}

func (s *LDAPService) interpolateFilter(filter string, params map[string]string) string {
	result := filter
	for key, val := range params {
		escaped := ldap.EscapeFilter(val)
		result = strings.ReplaceAll(result, "{{"+key+"}}", escaped)
	}
	return result
}

func (s *LDAPService) HealthCheck(ctx context.Context) error {
	conn, err := s.connect()
	if err != nil {
		return err
	}
	_ = conn.Close() // #nosec G104 -- cleanup after health check
	return nil
}

func (s *LDAPService) Close() error {
	return nil
}
