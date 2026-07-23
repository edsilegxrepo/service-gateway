package mongodb

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/edsilegxrepo/service-gateway/internal/crypto"
	"github.com/edsilegxrepo/service-gateway/internal/services"
)

func init() {
	services.RegisterType("mongodb", NewMongoDBService)
}

type MongoDBService struct {
	services.BaseService
	client   *mongo.Client
	database string
	config   Config
}

type Config struct {
	Type        string   `json:"type"`
	Description string   `json:"description"`
	Version     string   `json:"version"`
	Settings    Settings `json:"settings"`
}

type Settings struct {
	EncryptedURI   string              `json:"encrypted_uri"`
	Database       string              `json:"database"`
	TimeoutSeconds int                 `json:"timeout_seconds"`
	Queries        map[string]QueryDef `json:"queries"`
	AdHocAccess    AdHocAccess         `json:"adhoc_access"`
}

type QueryDef struct {
	Description string           `json:"description"`
	Collection  string           `json:"collection"`
	Operation   string           `json:"operation"`
	Filter      json.RawMessage  `json:"filter"`
	Projection  json.RawMessage  `json:"projection"`
	Sort        json.RawMessage  `json:"sort"`
	Limit       int64            `json:"limit"`
	Params      map[string]Param `json:"params"`
}

type Param struct {
	Type     string   `json:"type"`
	Required bool     `json:"required"`
	Default  any      `json:"default"`
	Pattern  string   `json:"pattern"`
	Enum     []string `json:"enum"`
}

type AdHocAccess struct {
	Enabled            bool     `json:"enabled"`
	AllowedCollections []string `json:"allowed_collections"`
	AllowedOperations  []string `json:"allowed_operations"`
	MaxLimit           int64    `json:"max_limit"`
}

func NewMongoDBService(name string, rawConfig json.RawMessage) (services.Service, error) {
	var cfg Config
	if err := json.Unmarshal(rawConfig, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	uri, err := decryptURI(cfg.Settings.EncryptedURI)
	if err != nil {
		return nil, fmt.Errorf("decrypt URI: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		return nil, fmt.Errorf("connect to MongoDB: %w", err)
	}

	if err := client.Ping(ctx, nil); err != nil {
		return nil, fmt.Errorf("ping MongoDB: %w", err)
	}

	return &MongoDBService{
		BaseService: services.NewBaseService(name, "mongodb"),
		client:      client,
		database:    cfg.Settings.Database,
		config:      cfg,
	}, nil
}

func (s *MongoDBService) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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

func (s *MongoDBService) handleNamedQuery(w http.ResponseWriter, r *http.Request, queryName string) {
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

	filter, err := interpolateFilter(query.Filter, params)
	if err != nil {
		s.WriteError(w, http.StatusBadRequest, "Invalid filter: "+err.Error())
		return
	}

	timeout := time.Duration(s.config.Settings.TimeoutSeconds) * time.Second
	ctx, cancel := s.WithTimeout(r.Context(), timeout)
	defer cancel()

	collection := s.client.Database(s.database).Collection(query.Collection)

	switch query.Operation {
	case "find", "":
		s.executeFind(ctx, w, collection, filter, query)
	case "findOne":
		s.executeFindOne(ctx, w, collection, filter, query)
	case "count":
		s.executeCount(ctx, w, collection, filter)
	case "aggregate":
		s.executeAggregate(ctx, w, collection, filter)
	default:
		s.WriteError(w, http.StatusBadRequest, "Unsupported operation: "+query.Operation)
	}
}

func (s *MongoDBService) executeFind(ctx context.Context, w http.ResponseWriter, coll *mongo.Collection, filter bson.M, query QueryDef) {
	opts := options.Find()
	if query.Projection != nil {
		var proj bson.M
		if err := json.Unmarshal(query.Projection, &proj); err != nil {
			s.WriteError(w, http.StatusBadRequest, "invalid projection: "+err.Error())
			return
		}
		opts.SetProjection(proj)
	}
	if query.Sort != nil {
		var sort bson.M
		if err := json.Unmarshal(query.Sort, &sort); err != nil {
			s.WriteError(w, http.StatusBadRequest, "invalid sort: "+err.Error())
			return
		}
		opts.SetSort(sort)
	}
	if query.Limit > 0 {
		opts.SetLimit(query.Limit)
	}

	cursor, err := coll.Find(ctx, filter, opts)
	if err != nil {
		s.WriteError(w, http.StatusInternalServerError, "Query failed: "+err.Error())
		return
	}
	defer cursor.Close(ctx) //nolint:errcheck // cleanup in defer

	// StreamJSON error ignored: headers already sent during streaming,
	// cannot write HTTP error response. Client detects via incomplete stream.
	_ = s.StreamJSON(w, func(emit func(any) error) error {
		for cursor.Next(ctx) {
			var doc bson.M
			if err := cursor.Decode(&doc); err != nil {
				return err
			}
			if err := emit(doc); err != nil {
				return err
			}
		}
		return cursor.Err()
	})
}

func (s *MongoDBService) executeFindOne(ctx context.Context, w http.ResponseWriter, coll *mongo.Collection, filter bson.M, query QueryDef) {
	opts := options.FindOne()
	if query.Projection != nil {
		var proj bson.M
		if err := json.Unmarshal(query.Projection, &proj); err != nil {
			s.WriteError(w, http.StatusBadRequest, "invalid projection: "+err.Error())
			return
		}
		opts.SetProjection(proj)
	}

	var result bson.M
	err := coll.FindOne(ctx, filter, opts).Decode(&result)
	if err == mongo.ErrNoDocuments {
		s.WriteJSON(w, http.StatusOK, map[string]any{"found": false})
		return
	}
	if err != nil {
		s.WriteError(w, http.StatusInternalServerError, "Query failed: "+err.Error())
		return
	}

	s.WriteJSON(w, http.StatusOK, map[string]any{"found": true, "document": result})
}

func (s *MongoDBService) executeCount(ctx context.Context, w http.ResponseWriter, coll *mongo.Collection, filter bson.M) {
	count, err := coll.CountDocuments(ctx, filter)
	if err != nil {
		s.WriteError(w, http.StatusInternalServerError, "Count failed: "+err.Error())
		return
	}

	s.WriteJSON(w, http.StatusOK, map[string]any{"count": count})
}

func (s *MongoDBService) executeAggregate(ctx context.Context, w http.ResponseWriter, coll *mongo.Collection, pipeline bson.M) {
	var pipelineArray []bson.M
	if stages, ok := pipeline["stages"].([]any); ok {
		for _, stage := range stages {
			if m, ok := stage.(bson.M); ok {
				pipelineArray = append(pipelineArray, m)
			}
		}
	}

	cursor, err := coll.Aggregate(ctx, pipelineArray)
	if err != nil {
		s.WriteError(w, http.StatusInternalServerError, "Aggregation failed: "+err.Error())
		return
	}
	defer cursor.Close(ctx) //nolint:errcheck // cleanup in defer

	// StreamJSON error ignored: headers already sent during streaming,
	// cannot write HTTP error response. Client detects via incomplete stream.
	_ = s.StreamJSON(w, func(emit func(any) error) error {
		for cursor.Next(ctx) {
			var doc bson.M
			if err := cursor.Decode(&doc); err != nil {
				return err
			}
			if err := emit(doc); err != nil {
				return err
			}
		}
		return cursor.Err()
	})
}

func (s *MongoDBService) handleAdHoc(w http.ResponseWriter, r *http.Request) {
	if !s.config.Settings.AdHocAccess.Enabled {
		s.WriteError(w, http.StatusForbidden, "Ad-hoc queries disabled")
		return
	}

	var req struct {
		Collection string          `json:"collection"`
		Operation  string          `json:"operation"`
		Filter     json.RawMessage `json:"filter"`
		Projection json.RawMessage `json:"projection"`
		Sort       json.RawMessage `json:"sort"`
		Limit      int64           `json:"limit"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if !s.isCollectionAllowed(req.Collection) {
		s.WriteError(w, http.StatusForbidden, "Collection not allowed")
		return
	}

	if !s.isOperationAllowed(req.Operation) {
		s.WriteError(w, http.StatusForbidden, "Operation not allowed")
		return
	}

	if req.Limit <= 0 || req.Limit > s.config.Settings.AdHocAccess.MaxLimit {
		req.Limit = s.config.Settings.AdHocAccess.MaxLimit
	}

	var filter bson.M
	if req.Filter != nil {
		if err := json.Unmarshal(req.Filter, &filter); err != nil {
			s.WriteError(w, http.StatusBadRequest, "invalid filter: "+err.Error())
			return
		}
	}

	query := QueryDef{
		Collection: req.Collection,
		Operation:  req.Operation,
		Projection: req.Projection,
		Sort:       req.Sort,
		Limit:      req.Limit,
	}

	timeout := time.Duration(s.config.Settings.TimeoutSeconds) * time.Second
	ctx, cancel := s.WithTimeout(r.Context(), timeout)
	defer cancel()

	collection := s.client.Database(s.database).Collection(req.Collection)

	switch req.Operation {
	case "find", "":
		s.executeFind(ctx, w, collection, filter, query)
	case "findOne":
		s.executeFindOne(ctx, w, collection, filter, query)
	case "count":
		s.executeCount(ctx, w, collection, filter)
	default:
		s.WriteError(w, http.StatusBadRequest, "Unsupported operation")
	}
}

func (s *MongoDBService) isCollectionAllowed(coll string) bool {
	for _, allowed := range s.config.Settings.AdHocAccess.AllowedCollections {
		if allowed == coll || allowed == "*" {
			return true
		}
	}
	return false
}

func (s *MongoDBService) isOperationAllowed(op string) bool {
	if op == "" {
		op = "find"
	}
	for _, allowed := range s.config.Settings.AdHocAccess.AllowedOperations {
		if allowed == op || allowed == "*" {
			return true
		}
	}
	return false
}

func (s *MongoDBService) HealthCheck(ctx context.Context) error {
	return s.client.Ping(ctx, nil)
}

func (s *MongoDBService) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return s.client.Disconnect(ctx)
}

func decryptURI(encrypted string) (string, error) {
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

func interpolateFilter(filterTemplate json.RawMessage, params map[string]any) (bson.M, error) {
	if filterTemplate == nil {
		return bson.M{}, nil
	}

	var filter bson.M
	if err := json.Unmarshal(filterTemplate, &filter); err != nil {
		return nil, err
	}

	return interpolateBSON(filter, params), nil
}

func interpolateBSON(m bson.M, params map[string]any) bson.M {
	result := make(bson.M, len(m))
	for k, v := range m {
		result[k] = interpolateValue(v, params)
	}
	return result
}

func interpolateValue(v any, params map[string]any) any {
	switch val := v.(type) {
	case string:
		if strings.HasPrefix(val, "{{") && strings.HasSuffix(val, "}}") {
			paramName := strings.TrimSuffix(strings.TrimPrefix(val, "{{"), "}}")
			if paramVal, ok := params[paramName]; ok {
				return sanitizeValue(paramVal)
			}
		}
		return val
	case bson.M:
		return interpolateBSON(val, params)
	case map[string]any:
		result := make(bson.M, len(val))
		for k, v := range val {
			result[k] = interpolateValue(v, params)
		}
		return result
	case []any:
		result := make([]any, len(val))
		for i, item := range val {
			result[i] = interpolateValue(item, params)
		}
		return result
	default:
		return v
	}
}

func sanitizeValue(v any) any {
	switch val := v.(type) {
	case string:
		if strings.HasPrefix(val, "$") {
			return nil
		}
		return val
	case map[string]any:
		for k := range val {
			if strings.HasPrefix(k, "$") {
				return nil
			}
		}
		return val
	default:
		return val
	}
}
