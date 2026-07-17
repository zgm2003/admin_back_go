package adminroute

import (
	"errors"
	"fmt"
	"net/http"
	pathpkg "path"
	"sort"
	"strings"
	"sync"

	"admin_back_go/internal/middleware"
)

var (
	ErrMethodRequired         = errors.New("route method is required")
	ErrPathRequired           = errors.New("route path is required")
	ErrAccessRequired         = errors.New("route access policy is required")
	ErrPermissionCodeRequired = errors.New("permission code is required")
	ErrAuditDecisionRequired  = errors.New("route audit decision is required")
	ErrAuditMetadataRequired  = errors.New("route audit metadata is required")
	ErrDuplicateRoute         = errors.New("duplicate route definition")
	ErrUnknownPermission      = errors.New("unknown permission code")
	ErrPublicCallbackAudit    = errors.New("public provider callback must not be audited")
	ErrSuccessStatusInvalid   = errors.New("route success status must be a 2xx status")
)

type routeKey struct {
	method string
	path   string
}

type Option func(*Registry)

func WithPermissionCatalog(catalog map[string]struct{}) Option {
	return func(registry *Registry) {
		registry.permissionCatalog = cloneCatalog(catalog)
	}
}

func WithMutationFallback(reason string) Option {
	return func(registry *Registry) {
		registry.mutationFallback = strings.TrimSpace(reason)
	}
}

func WithNoAuditPrefix(prefix string, reason string) Option {
	return func(registry *Registry) {
		prefix = strings.TrimSpace(prefix)
		reason = strings.TrimSpace(reason)
		if prefix == "" || reason == "" {
			return
		}
		if !strings.HasPrefix(prefix, "/") {
			prefix = "/" + prefix
		}
		registry.noAuditPrefixes[prefix] = reason
	}
}

type Registry struct {
	mu                sync.RWMutex
	definitions       map[routeKey]Definition
	permissionCatalog map[string]struct{}
	publicCandidates  map[string]struct{}
	noAuditReasons    map[routeKey]string
	noAuditPrefixes   map[string]string
	mutationFallback  string

	publicPaths     map[string]struct{}
	permissionRules map[middleware.RouteKey]string
	operationRules  map[middleware.RouteKey]middleware.OperationRule
}

func NewRegistry(options ...Option) *Registry {
	registry := &Registry{
		definitions:      make(map[routeKey]Definition),
		publicCandidates: make(map[string]struct{}),
		noAuditReasons:   make(map[routeKey]string),
		noAuditPrefixes:  make(map[string]string),
		publicPaths:      make(map[string]struct{}),
		permissionRules:  make(map[middleware.RouteKey]string),
		operationRules:   make(map[middleware.RouteKey]middleware.OperationRule),
	}
	for _, option := range options {
		if option != nil {
			option(registry)
		}
	}
	return registry
}

func (registry *Registry) Add(definition Definition) error {
	if registry == nil {
		return errors.New("route registry is nil")
	}
	normalized, err := normalizeDefinition(definition)
	if err != nil {
		return err
	}
	key := routeKey{method: normalized.Method, path: normalized.Path}

	registry.mu.Lock()
	defer registry.mu.Unlock()
	if registry.definitions == nil {
		registry.definitions = make(map[routeKey]Definition)
	}
	if _, exists := registry.definitions[key]; exists {
		return fmt.Errorf("%w: %s %s", ErrDuplicateRoute, normalized.Method, normalized.Path)
	}
	registry.definitions[key] = normalized
	return nil
}

func (registry *Registry) Compile() error {
	if registry == nil {
		return errors.New("route registry is nil")
	}
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	if len(registry.permissionCatalog) == 0 {
		return nil
	}
	for _, definition := range registry.definitions {
		if definition.Access.Kind != AccessPermission {
			continue
		}
		if _, exists := registry.permissionCatalog[definition.Access.PermissionCode]; !exists {
			return fmt.Errorf("%w: %s (%s %s)", ErrUnknownPermission, definition.Access.PermissionCode, definition.Method, definition.Path)
		}
	}
	return nil
}

func (registry *Registry) Definitions() []Definition {
	if registry == nil {
		return nil
	}
	registry.mu.RLock()
	definitions := make([]Definition, 0, len(registry.definitions))
	for _, definition := range registry.definitions {
		definition.Tags = append([]string(nil), definition.Tags...)
		definitions = append(definitions, definition)
	}
	registry.mu.RUnlock()

	sort.Slice(definitions, func(left int, right int) bool {
		if definitions[left].Method != definitions[right].Method {
			return definitions[left].Method < definitions[right].Method
		}
		return definitions[left].Path < definitions[right].Path
	})
	return definitions
}

func normalizeDefinition(definition Definition) (Definition, error) {
	definition.Method = strings.ToUpper(strings.TrimSpace(definition.Method))
	if definition.Method == "" {
		return Definition{}, ErrMethodRequired
	}
	definition.Path = normalizePath(definition.Path)
	if definition.Path == "" {
		return Definition{}, ErrPathRequired
	}
	definition.OperationID = strings.TrimSpace(definition.OperationID)
	definition.RequestSchema = strings.TrimSpace(definition.RequestSchema)
	definition.ResponseSchema = strings.TrimSpace(definition.ResponseSchema)
	definition.Access.PermissionCode = strings.TrimSpace(definition.Access.PermissionCode)
	definition.Audit.Module = strings.TrimSpace(definition.Audit.Module)
	definition.Audit.Action = strings.TrimSpace(definition.Audit.Action)
	definition.Audit.Title = strings.TrimSpace(definition.Audit.Title)
	definition.Audit.Reason = strings.TrimSpace(definition.Audit.Reason)
	definition.Tags = normalizeTags(definition.Tags)
	if definition.SuccessStatus != 0 && (definition.SuccessStatus < http.StatusOK || definition.SuccessStatus >= http.StatusMultipleChoices) {
		return Definition{}, fmt.Errorf("%w: %d (%s %s)", ErrSuccessStatusInvalid, definition.SuccessStatus, definition.Method, definition.Path)
	}

	switch definition.Access.Kind {
	case AccessPublic, AccessAuthenticated:
		definition.Access.PermissionCode = ""
	case AccessPermission:
		if definition.Access.PermissionCode == "" {
			return Definition{}, fmt.Errorf("%w: %s %s", ErrPermissionCodeRequired, definition.Method, definition.Path)
		}
	default:
		return Definition{}, fmt.Errorf("%w: %s %s", ErrAccessRequired, definition.Method, definition.Path)
	}

	if isPublicProviderCallback(definition) && definition.Audit.Enabled {
		return Definition{}, fmt.Errorf("%w: %s %s", ErrPublicCallbackAudit, definition.Method, definition.Path)
	}
	if definition.Audit.Enabled && (definition.Audit.Module == "" || definition.Audit.Action == "" || definition.Audit.Title == "") {
		return Definition{}, fmt.Errorf("%w: %s %s", ErrAuditMetadataRequired, definition.Method, definition.Path)
	}
	if isMutation(definition.Method) && !definition.Audit.Enabled && definition.Audit.Reason == "" {
		return Definition{}, fmt.Errorf("%w: %s %s", ErrAuditDecisionRequired, definition.Method, definition.Path)
	}
	return definition, nil
}

func normalizePath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || !strings.HasPrefix(value, "/") {
		return ""
	}
	normalized := pathpkg.Clean(value)
	if normalized == "." {
		return ""
	}
	return normalized
}

func normalizeTags(tags []string) []string {
	set := make(map[string]struct{}, len(tags))
	for _, tag := range tags {
		if tag = strings.TrimSpace(tag); tag != "" {
			set[tag] = struct{}{}
		}
	}
	normalized := make([]string, 0, len(set))
	for tag := range set {
		normalized = append(normalized, tag)
	}
	sort.Strings(normalized)
	return normalized
}

func isMutation(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func isPublicProviderCallback(definition Definition) bool {
	return definition.Access.Kind == AccessPublic && strings.HasPrefix(definition.Path, "/api/payment/callbacks/")
}

func cloneCatalog(catalog map[string]struct{}) map[string]struct{} {
	if len(catalog) == 0 {
		return nil
	}
	cloned := make(map[string]struct{}, len(catalog))
	for code := range catalog {
		if code = strings.TrimSpace(code); code != "" {
			cloned[code] = struct{}{}
		}
	}
	return cloned
}
