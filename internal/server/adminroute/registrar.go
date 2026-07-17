package adminroute

import (
	"fmt"

	"github.com/gin-gonic/gin"
)

// Registrar registers the Gin handler and its access/audit definition as one
// operation. A nil Registry is supported only for isolated transport tests.
type Registrar struct {
	router   gin.IRoutes
	registry *Registry
}

func NewRegistrar(router gin.IRoutes, registries ...*Registry) Registrar {
	var registry *Registry
	for _, candidate := range registries {
		if candidate != nil {
			registry = candidate
			break
		}
	}
	return Registrar{router: router, registry: registry}
}

func (r Registrar) Handle(definition Definition, handlers ...gin.HandlerFunc) {
	if r.router == nil {
		panic("admin route Gin registrar is nil")
	}
	if len(handlers) == 0 {
		panic("admin route handler is required")
	}
	normalized, err := normalizeDefinition(definition)
	if err != nil {
		panic(fmt.Sprintf("register route policy: %v", err))
	}
	if r.registry != nil {
		if err := r.registry.Add(normalized); err != nil {
			panic(fmt.Sprintf("register route policy: %v", err))
		}
	}
	r.router.Handle(normalized.Method, normalized.Path, handlers...)
}
