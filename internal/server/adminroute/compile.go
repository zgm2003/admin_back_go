package adminroute

import (
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"unicode"

	"admin_back_go/internal/middleware"
)

var (
	ErrDuplicateActualRoute   = errors.New("duplicate actual route")
	ErrUnknownRouteDefinition = errors.New("route definition has no actual route")
)

type Route struct {
	Method string
	Path   string
}

func NewLegacyRegistry(
	permissionRules map[middleware.RouteKey]string,
	operationRules map[middleware.RouteKey]middleware.OperationRule,
	publicPaths map[string]struct{},
	noAuditReasons map[middleware.RouteKey]string,
	options ...Option,
) (*Registry, error) {
	normalizedPermissions := make(map[middleware.RouteKey]string, len(permissionRules))
	for key, code := range permissionRules {
		key = normalizedMiddlewareKey(key)
		if key.Method == "" || key.Path == "" {
			return nil, fmt.Errorf("invalid permission route key")
		}
		if _, exists := normalizedPermissions[key]; exists {
			return nil, fmt.Errorf("%w: %s %s", ErrDuplicateRoute, key.Method, key.Path)
		}
		normalizedPermissions[key] = strings.TrimSpace(code)
	}
	normalizedOperations := make(map[middleware.RouteKey]middleware.OperationRule, len(operationRules))
	for key, rule := range operationRules {
		key = normalizedMiddlewareKey(key)
		if key.Method == "" || key.Path == "" {
			return nil, fmt.Errorf("invalid operation route key")
		}
		if _, exists := normalizedOperations[key]; exists {
			return nil, fmt.Errorf("%w: %s %s", ErrDuplicateRoute, key.Method, key.Path)
		}
		normalizedOperations[key] = rule
	}

	catalog := make(map[string]struct{}, len(normalizedPermissions))
	for _, code := range normalizedPermissions {
		if code = strings.TrimSpace(code); code != "" {
			catalog[code] = struct{}{}
		}
	}
	registryOptions := make([]Option, 0, len(options)+1)
	registryOptions = append(registryOptions, WithPermissionCatalog(catalog))
	registryOptions = append(registryOptions, options...)
	registry := NewRegistry(registryOptions...)
	for path := range publicPaths {
		if path = normalizePath(path); path != "" {
			registry.publicCandidates[path] = struct{}{}
		}
	}
	for key, reason := range noAuditReasons {
		normalized := normalizedMiddlewareKey(key)
		if normalized.Method != "" && normalized.Path != "" {
			registry.noAuditReasons[routeKey{method: normalized.Method, path: normalized.Path}] = strings.TrimSpace(reason)
		}
	}

	keys := make(map[middleware.RouteKey]struct{}, len(normalizedPermissions)+len(normalizedOperations))
	for key := range normalizedPermissions {
		keys[key] = struct{}{}
	}
	for key := range normalizedOperations {
		keys[key] = struct{}{}
	}
	ordered := make([]middleware.RouteKey, 0, len(keys))
	for key := range keys {
		ordered = append(ordered, key)
	}
	sort.Slice(ordered, func(left int, right int) bool {
		if ordered[left].Method != ordered[right].Method {
			return ordered[left].Method < ordered[right].Method
		}
		return ordered[left].Path < ordered[right].Path
	})

	for _, key := range ordered {
		access := Authenticated()
		if code := strings.TrimSpace(normalizedPermissions[key]); code != "" {
			access = Permission(code)
		} else if _, public := registry.publicCandidates[key.Path]; public {
			access = Public()
		}

		decision := NoAudit("read-only")
		if rule, exists := normalizedOperations[key]; exists {
			decision = Audit(rule.Module, rule.Action, rule.Title)
			decision.SkipRequestPayload = rule.SkipRequestPayload
			decision.SkipResponsePayload = rule.SkipResponsePayload
		} else if isMutation(key.Method) {
			reason := registry.noAuditReason(routeKey{method: key.Method, path: key.Path})
			decision = NoAudit(reason)
		}
		if err := registry.Add(Definition{
			Method:      key.Method,
			Path:        key.Path,
			OperationID: legacyOperationID(key.Method, key.Path),
			Access:      access,
			Audit:       decision,
		}); err != nil {
			return nil, err
		}
	}
	return registry, nil
}

func (registry *Registry) CompileRoutes(routes []Route) error {
	if registry == nil {
		return errors.New("route registry is nil")
	}
	actual := make(map[routeKey]struct{}, len(routes))
	ordered := make([]routeKey, 0, len(routes))
	for _, route := range routes {
		method := strings.ToUpper(strings.TrimSpace(route.Method))
		path := normalizePath(route.Path)
		if method == "" {
			return ErrMethodRequired
		}
		if path == "" {
			return ErrPathRequired
		}
		key := routeKey{method: method, path: path}
		if _, exists := actual[key]; exists {
			return fmt.Errorf("%w: %s %s", ErrDuplicateActualRoute, method, path)
		}
		actual[key] = struct{}{}
		ordered = append(ordered, key)
	}
	sort.Slice(ordered, func(left int, right int) bool {
		if ordered[left].method != ordered[right].method {
			return ordered[left].method < ordered[right].method
		}
		return ordered[left].path < ordered[right].path
	})

	registry.mu.RLock()
	existing := make(map[routeKey]Definition, len(registry.definitions))
	for key, definition := range registry.definitions {
		existing[key] = definition
	}
	publicCandidates := clonePaths(registry.publicCandidates)
	noAuditReasons := make(map[routeKey]string, len(registry.noAuditReasons))
	for key, reason := range registry.noAuditReasons {
		noAuditReasons[key] = reason
	}
	noAuditPrefixes := make(map[string]string, len(registry.noAuditPrefixes))
	for prefix, reason := range registry.noAuditPrefixes {
		noAuditPrefixes[prefix] = reason
	}
	mutationFallback := registry.mutationFallback
	registry.mu.RUnlock()

	for key := range existing {
		if _, exists := actual[key]; !exists {
			return fmt.Errorf("%w: %s %s", ErrUnknownRouteDefinition, key.method, key.path)
		}
	}
	var policyErrors []error
	for _, key := range ordered {
		if definition, exists := existing[key]; exists {
			if definition.OperationID == "" || (!definition.Audit.Enabled && definition.Audit.Reason == "") {
				registry.mu.Lock()
				if definition.OperationID == "" {
					definition.OperationID = legacyOperationID(key.method, key.path)
				}
				if !definition.Audit.Enabled && definition.Audit.Reason == "" && !isMutation(key.method) {
					definition.Audit = NoAudit("read-only")
				}
				registry.definitions[key] = definition
				registry.mu.Unlock()
			}
			continue
		}

		access := Authenticated()
		if key.method == http.MethodOptions {
			access = Public()
		} else if _, public := publicCandidates[key.path]; public {
			access = Public()
		}
		decision := NoAudit("read-only")
		if isMutation(key.method) {
			reason := resolvedNoAuditReason(key, noAuditReasons, noAuditPrefixes, mutationFallback)
			if reason == "" {
				policyErrors = append(policyErrors, fmt.Errorf("%w: %s %s", ErrAuditDecisionRequired, key.method, key.path))
				continue
			}
			decision = NoAudit(reason)
		}
		if err := registry.Add(Definition{
			Method:      key.method,
			Path:        key.path,
			OperationID: legacyOperationID(key.method, key.path),
			Access:      access,
			Audit:       decision,
		}); err != nil {
			return err
		}
	}
	if len(policyErrors) > 0 {
		return errors.Join(policyErrors...)
	}
	if err := registry.Compile(); err != nil {
		return err
	}
	return registry.compileRuntimeMaps()
}

func (registry *Registry) PublicPaths() map[string]struct{} {
	if registry == nil {
		return nil
	}
	return registry.publicPaths
}

func (registry *Registry) PermissionRules() map[middleware.RouteKey]string {
	if registry == nil {
		return nil
	}
	return registry.permissionRules
}

func (registry *Registry) OperationRules() map[middleware.RouteKey]middleware.OperationRule {
	if registry == nil {
		return nil
	}
	return registry.operationRules
}

func (registry *Registry) compileRuntimeMaps() error {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	clear(registry.publicPaths)
	clear(registry.permissionRules)
	clear(registry.operationRules)

	for _, definition := range registry.definitions {
		key := middleware.NewRouteKey(definition.Method, definition.Path)
		switch definition.Access.Kind {
		case AccessPublic:
			if definition.Method != http.MethodOptions {
				registry.publicPaths[definition.Path] = struct{}{}
			}
		case AccessPermission:
			registry.permissionRules[key] = definition.Access.PermissionCode
		}
		if definition.Audit.Enabled {
			registry.operationRules[key] = middleware.OperationRule{
				Module:              definition.Audit.Module,
				Action:              definition.Audit.Action,
				Title:               definition.Audit.Title,
				SkipRequestPayload:  definition.Audit.SkipRequestPayload,
				SkipResponsePayload: definition.Audit.SkipResponsePayload,
			}
		}
	}
	return nil
}

func normalizedMiddlewareKey(key middleware.RouteKey) middleware.RouteKey {
	return middleware.NewRouteKey(key.Method, normalizePath(key.Path))
}

func legacyOperationID(method string, path string) string {
	raw := strings.ToLower(strings.TrimSpace(method) + "_" + strings.TrimSpace(path))
	var builder strings.Builder
	lastUnderscore := false
	for _, character := range raw {
		if unicode.IsLetter(character) || unicode.IsDigit(character) {
			builder.WriteRune(character)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore && builder.Len() > 0 {
			builder.WriteByte('_')
			lastUnderscore = true
		}
	}
	return strings.Trim(builder.String(), "_")
}

func clonePaths(paths map[string]struct{}) map[string]struct{} {
	cloned := make(map[string]struct{}, len(paths))
	for path := range paths {
		cloned[path] = struct{}{}
	}
	return cloned
}

func (registry *Registry) noAuditReason(key routeKey) string {
	return resolvedNoAuditReason(key, registry.noAuditReasons, registry.noAuditPrefixes, registry.mutationFallback)
}

func resolvedNoAuditReason(key routeKey, exact map[routeKey]string, prefixes map[string]string, fallback string) string {
	if reason := strings.TrimSpace(exact[key]); reason != "" {
		return reason
	}
	matchedPrefix := ""
	matchedReason := ""
	for prefix, reason := range prefixes {
		if strings.HasPrefix(key.path, prefix) && len(prefix) > len(matchedPrefix) {
			matchedPrefix = prefix
			matchedReason = reason
		}
	}
	if matchedReason != "" {
		return strings.TrimSpace(matchedReason)
	}
	return strings.TrimSpace(fallback)
}
