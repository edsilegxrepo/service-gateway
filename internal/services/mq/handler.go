package mq

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill/message"

	"github.com/edsilegxrepo/service-gateway/internal/crypto"
	"github.com/edsilegxrepo/service-gateway/internal/services"
)

func init() {
	services.RegisterType("mq", NewMQService)
}

type MQService struct {
	services.BaseService
	config     Config
	publisher  message.Publisher
	subscriber message.Subscriber
	logger     watermill.LoggerAdapter
}

type Config struct {
	Type        string   `json:"type"`
	Description string   `json:"description"`
	Version     string   `json:"version"`
	Settings    Settings `json:"settings"`
}

type Settings struct {
	Backend             string               `json:"backend"`
	EncryptedConnection string               `json:"encrypted_connection"`
	Options             BackendOptions       `json:"options"`
	Operations          map[string]Operation `json:"operations"`
	AllowedTopics       []string             `json:"allowed_topics"`
}

type BackendOptions struct {
	ConsumerGroup   string `json:"consumer_group"`
	AutoOffsetReset string `json:"auto_offset_reset"`
	EnableTLS       bool   `json:"enable_tls"`
	TLSCACert       string `json:"tls_ca_cert"`
	TLSClientCert   string `json:"tls_client_cert"`
	TLSClientKey    string `json:"tls_client_key"`
	Exchange        string `json:"exchange"`
	ExchangeType    string `json:"exchange_type"`
	Durable         bool   `json:"durable"`
	PrefetchCount   int    `json:"prefetch_count"`
}

type Operation struct {
	Description string           `json:"description"`
	Action      string           `json:"action"`
	Topic       string           `json:"topic"`
	Queue       string           `json:"queue"`
	RoutingKey  string           `json:"routing_key"`
	Params      map[string]Param `json:"params"`
}

type Param struct {
	Type     string   `json:"type"`
	Required bool     `json:"required"`
	Default  any      `json:"default"`
	Enum     []string `json:"enum"`
}

func NewMQService(name string, rawConfig json.RawMessage) (services.Service, error) {
	var cfg Config
	if err := json.Unmarshal(rawConfig, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	connStr, err := crypto.Decrypt(cfg.Settings.EncryptedConnection)
	if err != nil {
		return nil, fmt.Errorf("decrypt connection: %w", err)
	}

	logger := watermill.NewStdLogger(false, false)

	svc := &MQService{
		BaseService: services.NewBaseService(name, "mq"),
		config:      cfg,
		logger:      logger,
	}

	if err := svc.initBackend(connStr); err != nil {
		return nil, fmt.Errorf("init backend: %w", err)
	}

	return svc, nil
}

func (s *MQService) initBackend(connStr string) error {
	switch s.config.Settings.Backend {
	case "kafka":
		return s.initKafka(connStr)
	case "rabbitmq", "amqp":
		return s.initRabbitMQ(connStr)
	case "nats":
		return s.initNATS(connStr)
	default:
		return fmt.Errorf("unsupported backend: %s", s.config.Settings.Backend)
	}
}

func (s *MQService) initKafka(brokers string) error {
	// Placeholder - actual implementation requires watermill-kafka
	return nil
}

func (s *MQService) initRabbitMQ(amqpURI string) error {
	// Placeholder - actual implementation requires watermill-amqp
	return nil
}

func (s *MQService) initNATS(natsURL string) error {
	// Placeholder - actual implementation requires watermill-nats
	return nil
}

func (s *MQService) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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

	switch op.Action {
	case "publish":
		s.handlePublish(w, r, op)
	case "consume":
		s.handleConsume(w, r, op)
	default:
		s.WriteError(w, http.StatusBadRequest, "Unsupported action: "+op.Action)
	}
}

func (s *MQService) handlePublish(w http.ResponseWriter, r *http.Request, op Operation) {
	var req struct {
		Payload json.RawMessage `json:"payload"`
		Key     string          `json:"key"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	topic := op.Topic
	if topic == "" {
		topic = op.RoutingKey
	}

	if !s.isTopicAllowed(topic) {
		s.WriteError(w, http.StatusForbidden, "Topic not allowed")
		return
	}

	if s.publisher == nil {
		s.WriteError(w, http.StatusServiceUnavailable, "Publisher not initialized")
		return
	}

	msg := message.NewMessage(watermill.NewUUID(), []byte(req.Payload))
	if req.Key != "" {
		msg.Metadata.Set("key", req.Key)
	}

	if err := s.publisher.Publish(topic, msg); err != nil {
		s.WriteError(w, http.StatusInternalServerError, "Publish failed: "+err.Error())
		return
	}

	s.WriteJSON(w, http.StatusOK, map[string]any{
		"status":     "published",
		"topic":      topic,
		"message_id": msg.UUID,
		"timestamp":  time.Now().UTC(),
	})
}

func (s *MQService) handleConsume(w http.ResponseWriter, r *http.Request, op Operation) {
	var req struct {
		MaxMessages    int `json:"max_messages"`
		TimeoutSeconds int `json:"timeout_seconds"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.MaxMessages <= 0 {
		req.MaxMessages = 10
	}
	if req.TimeoutSeconds <= 0 {
		req.TimeoutSeconds = 30
	}

	topic := op.Topic
	if topic == "" {
		topic = op.Queue
	}

	if !s.isTopicAllowed(topic) {
		s.WriteError(w, http.StatusForbidden, "Topic not allowed")
		return
	}

	if s.subscriber == nil {
		s.WriteError(w, http.StatusServiceUnavailable, "Subscriber not initialized")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), time.Duration(req.TimeoutSeconds)*time.Second)
	defer cancel()

	messages, err := s.subscriber.Subscribe(ctx, topic)
	if err != nil {
		s.WriteError(w, http.StatusInternalServerError, "Subscribe failed: "+err.Error())
		return
	}

	if err := s.StreamJSON(w, func(emit func(any) error) error {
		count := 0
		for {
			select {
			case msg, ok := <-messages:
				if !ok {
					return nil
				}
				if err := emit(map[string]any{
					"message_id": msg.UUID,
					"payload":    string(msg.Payload),
					"metadata":   msg.Metadata,
				}); err != nil {
					return err
				}
				msg.Ack()
				count++
				if count >= req.MaxMessages {
					return nil
				}
			case <-ctx.Done():
				return nil
			}
		}
	}); err != nil {
		s.WriteError(w, http.StatusInternalServerError, "streaming error: "+err.Error())
	}
}

func (s *MQService) isTopicAllowed(topic string) bool {
	if len(s.config.Settings.AllowedTopics) == 0 {
		return true
	}
	for _, allowed := range s.config.Settings.AllowedTopics {
		if allowed == topic || allowed == "*" {
			return true
		}
	}
	return false
}

func (s *MQService) HealthCheck(ctx context.Context) error {
	if s.publisher == nil && s.subscriber == nil {
		return fmt.Errorf("no publisher or subscriber initialized")
	}
	return nil
}

func (s *MQService) Close() error {
	var errs []error
	if s.publisher != nil {
		if err := s.publisher.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if s.subscriber != nil {
		if err := s.subscriber.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("close errors: %v", errs)
	}
	return nil
}
