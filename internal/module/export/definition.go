package exporttask

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

const (
	ScopeSelected = "selected"
	ScopeFiltered = "filtered"
)

type Definition struct {
	Kind     string
	Title    string
	Provider Provider
}

type Provider interface {
	BuildExportData(ctx context.Context, input BuildInput) (*FileData, error)
}

type BuildInput struct {
	TaskID   int64
	UserID   int64
	Platform string
	Kind     string
	Scope    string
	IDs      []int64
	Params   json.RawMessage
}

type Registry struct {
	definitions map[string]Definition
}

func NewRegistry(definitions ...Definition) (*Registry, error) {
	registry := &Registry{definitions: make(map[string]Definition, len(definitions))}
	for _, definition := range definitions {
		definition.Kind = strings.TrimSpace(definition.Kind)
		definition.Title = strings.TrimSpace(definition.Title)
		if definition.Kind == "" {
			return nil, fmt.Errorf("export registry kind is required")
		}
		if definition.Provider == nil {
			return nil, fmt.Errorf("export registry provider is required for %s", definition.Kind)
		}
		if _, exists := registry.definitions[definition.Kind]; exists {
			return nil, fmt.Errorf("export registry duplicate kind: %s", definition.Kind)
		}
		if definition.Title == "" {
			definition.Title = kindText(definition.Kind)
		}
		registry.definitions[definition.Kind] = definition
	}
	return registry, nil
}

func (r *Registry) Resolve(kind string) (Definition, bool) {
	if r == nil {
		return Definition{}, false
	}
	kind = strings.TrimSpace(kind)
	if kind == "" {
		return Definition{}, false
	}
	definition, ok := r.definitions[kind]
	return definition, ok
}
