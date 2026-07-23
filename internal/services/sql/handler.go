package sql

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/xo/dburl"

	"github.com/edsilegxrepo/service-gateway/internal/crypto"
	"github.com/edsilegxrepo/service-gateway/internal/services"
)

func init() {
	services.RegisterType("sql", NewSQLService)
}

type SQLService struct {
	services.BaseService
	db     *sql.DB
	config Config
}

type Config struct {
	Type        string   `json:"type"`
	Description string   `json:"description"`
	Version     string   `json:"version"`
	Settings    Settings `json:"settings"`
}

type Settings struct {
	EncryptedDSN   string              `json:"encrypted_dsn"`
	TimeoutSeconds int                 `json:"timeout_seconds"`
	MaxOpenConns   int                 `json:"max_open_conns"`
	MaxIdleConns   int                 `json:"max_idle_conns"`
	ConnMaxLife    string              `json:"conn_max_lifetime"`
	Queries        map[string]QueryDef `json:"queries"`
	AdHocAccess    AdHocAccess         `json:"adhoc_access"`
}

type QueryDef struct {
	Description string           `json:"description"`
	SQL         string           `json:"sql"`
	Params      map[string]Param `json:"params"`
	SingleRow   bool             `json:"single_row"`
}

type Param struct {
	Type     string   `json:"type"`
	Required bool     `json:"required"`
	Default  any      `json:"default"`
	Pattern  string   `json:"pattern"`
	Enum     []string `json:"enum"`
}

type AdHocAccess struct {
	Enabled         bool     `json:"enabled"`
	AllowedPatterns []string `json:"allowed_patterns"`
	MaxRows         int      `json:"max_rows"`
}

func NewSQLService(name string, rawConfig json.RawMessage) (services.Service, error) {
	var cfg Config
	if err := json.Unmarshal(rawConfig, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	dsn, err := decryptDSN(cfg.Settings.EncryptedDSN)
	if err != nil {
		return nil, fmt.Errorf("decrypt DSN: %w", err)
	}

	db, err := dburl.Open(dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	if cfg.Settings.MaxOpenConns > 0 {
		db.SetMaxOpenConns(cfg.Settings.MaxOpenConns)
	}
	if cfg.Settings.MaxIdleConns > 0 {
		db.SetMaxIdleConns(cfg.Settings.MaxIdleConns)
	}
	if cfg.Settings.ConnMaxLife != "" {
		d, _ := time.ParseDuration(cfg.Settings.ConnMaxLife)
		db.SetConnMaxLifetime(d)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("ping database: %w", err)
	}

	return &SQLService{
		BaseService: services.NewBaseService(name, "sql"),
		db:          db,
		config:      cfg,
	}, nil
}

func (s *SQLService) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/")
	parts := strings.SplitN(path, "/", 2)

	if len(parts) == 0 || parts[0] == "" {
		s.WriteError(w, http.StatusBadRequest, "Operation required")
		return
	}

	switch parts[0] {
	case "query":
		if len(parts) < 2 {
			s.WriteError(w, http.StatusBadRequest, "Query name required")
			return
		}
		s.handleNamedQuery(w, r, parts[1])
	case "adhoc":
		s.handleAdHoc(w, r)
	default:
		s.WriteError(w, http.StatusNotFound, "Unknown operation")
	}
}

func (s *SQLService) handleNamedQuery(w http.ResponseWriter, r *http.Request, queryName string) {
	query, exists := s.config.Settings.Queries[queryName]
	if !exists {
		s.WriteError(w, http.StatusNotFound, "Query not found")
		return
	}

	var params map[string]any
	if r.Method == http.MethodPost {
		if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
			s.WriteError(w, http.StatusBadRequest, "Invalid request body")
			return
		}
	}

	if err := validateParams(params, query.Params); err != nil {
		s.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	sqlStr, args := interpolateSQL(query.SQL, params)

	timeout := time.Duration(s.config.Settings.TimeoutSeconds) * time.Second
	ctx, cancel := s.WithTimeout(r.Context(), timeout)
	defer cancel()

	if query.SingleRow {
		s.executeSingleRow(ctx, w, sqlStr, args)
	} else {
		s.executeQuery(ctx, w, sqlStr, args)
	}
}

func (s *SQLService) executeQuery(ctx context.Context, w http.ResponseWriter, sqlStr string, args []any) {
	rows, err := s.db.QueryContext(ctx, sqlStr, args...)
	if err != nil {
		s.WriteError(w, http.StatusInternalServerError, "Query failed: "+err.Error())
		return
	}
	defer rows.Close() //nolint:errcheck // cleanup in defer

	columns, err := rows.Columns()
	if err != nil {
		s.WriteError(w, http.StatusInternalServerError, "Failed to get columns: "+err.Error())
		return
	}

	// StreamJSON error ignored: headers already sent during streaming,
	// cannot write HTTP error response. Client detects via incomplete stream.
	_ = s.StreamJSON(w, func(emit func(any) error) error {
		values := make([]any, len(columns))
		valuePtrs := make([]any, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		for rows.Next() {
			if err := rows.Scan(valuePtrs...); err != nil {
				return err
			}

			row := make(map[string]any)
			for i, col := range columns {
				row[col] = convertValue(values[i])
			}

			if err := emit(row); err != nil {
				return err
			}
		}
		return rows.Err()
	})
}

func (s *SQLService) executeSingleRow(ctx context.Context, w http.ResponseWriter, sqlStr string, args []any) {
	rows, err := s.db.QueryContext(ctx, sqlStr, args...)
	if err != nil {
		s.WriteError(w, http.StatusInternalServerError, "Query failed: "+err.Error())
		return
	}
	defer rows.Close() //nolint:errcheck // cleanup in defer

	columns, err := rows.Columns()
	if err != nil {
		s.WriteError(w, http.StatusInternalServerError, "Failed to get columns: "+err.Error())
		return
	}

	if !rows.Next() {
		s.WriteJSON(w, http.StatusOK, map[string]any{"found": false})
		return
	}

	values := make([]any, len(columns))
	valuePtrs := make([]any, len(columns))
	for i := range values {
		valuePtrs[i] = &values[i]
	}

	if err := rows.Scan(valuePtrs...); err != nil {
		s.WriteError(w, http.StatusInternalServerError, "Scan failed: "+err.Error())
		return
	}

	row := make(map[string]any)
	for i, col := range columns {
		row[col] = convertValue(values[i])
	}

	s.WriteJSON(w, http.StatusOK, map[string]any{"found": true, "row": row})
}

func (s *SQLService) handleAdHoc(w http.ResponseWriter, r *http.Request) {
	if !s.config.Settings.AdHocAccess.Enabled {
		s.WriteError(w, http.StatusForbidden, "Ad-hoc queries disabled")
		return
	}

	var req struct {
		SQL    string `json:"sql"`
		Params []any  `json:"params"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if !s.isQueryAllowed(req.SQL) {
		s.WriteError(w, http.StatusForbidden, "Query pattern not allowed")
		return
	}

	timeout := time.Duration(s.config.Settings.TimeoutSeconds) * time.Second
	ctx, cancel := s.WithTimeout(r.Context(), timeout)
	defer cancel()

	s.executeQuery(ctx, w, req.SQL, req.Params)
}

func (s *SQLService) isQueryAllowed(query string) bool {
	normalized := strings.ToUpper(strings.TrimSpace(query))

	if !strings.HasPrefix(normalized, "SELECT") {
		return false
	}

	for _, pattern := range s.config.Settings.AdHocAccess.AllowedPatterns {
		matched, _ := regexp.MatchString(pattern, query)
		if matched {
			return true
		}
	}

	return len(s.config.Settings.AdHocAccess.AllowedPatterns) == 0
}

func (s *SQLService) HealthCheck(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

func (s *SQLService) Close() error {
	return s.db.Close()
}

func decryptDSN(encrypted string) (string, error) {
	return crypto.Decrypt(encrypted)
}

func validateParams(params map[string]any, defs map[string]Param) error {
	for name, def := range defs {
		val, exists := params[name]
		if def.Required && !exists {
			return fmt.Errorf("missing required parameter: %s", name)
		}
		if !exists && def.Default != nil {
			params[name] = def.Default
		}
		if exists && len(def.Enum) > 0 {
			strVal := fmt.Sprintf("%v", val)
			found := false
			for _, e := range def.Enum {
				if e == strVal {
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("parameter %s must be one of: %v", name, def.Enum)
			}
		}
	}
	return nil
}

var placeholderRegex = regexp.MustCompile(`\{\{(\w+)\}\}`)

func interpolateSQL(sqlTemplate string, params map[string]any) (string, []any) {
	var args []any
	argIndex := 1
	seen := make(map[string]int)

	result := placeholderRegex.ReplaceAllStringFunc(sqlTemplate, func(match string) string {
		paramName := match[2 : len(match)-2]

		if idx, ok := seen[paramName]; ok {
			return fmt.Sprintf("$%d", idx)
		}

		if val, ok := params[paramName]; ok {
			seen[paramName] = argIndex
			args = append(args, val)
			idx := argIndex
			argIndex++
			return fmt.Sprintf("$%d", idx)
		}

		return match
	})

	return result, args
}

func convertValue(v any) any {
	switch val := v.(type) {
	case []byte:
		return string(val)
	case nil:
		return nil
	default:
		return val
	}
}
