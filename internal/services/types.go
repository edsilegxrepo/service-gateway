package services

import (
	"encoding/json"
	"fmt"
	"sync"
)

type ServiceFactory func(name string, config json.RawMessage) (Service, error)

var (
	factoryMu sync.RWMutex
	factories = make(map[string]ServiceFactory)
)

func RegisterType(typeName string, factory ServiceFactory) {
	factoryMu.Lock()
	defer factoryMu.Unlock()
	factories[typeName] = factory
}

func CreateService(name string, config json.RawMessage) (Service, error) {
	var sc ServiceConfig
	if err := json.Unmarshal(config, &sc); err != nil {
		return nil, fmt.Errorf("parse service config: %w", err)
	}

	factoryMu.RLock()
	factory, exists := factories[sc.Type]
	factoryMu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("unknown service type: %s", sc.Type)
	}

	return factory(name, config)
}

func RegisteredTypes() []string {
	factoryMu.RLock()
	defer factoryMu.RUnlock()

	types := make([]string, 0, len(factories))
	for t := range factories {
		types = append(types, t)
	}
	return types
}
