package dict

const (
	ProviderCommonStatus           = "common_status"
	ProviderCommonYesNo            = "common_yes_no"
	ProviderPlatform               = "platform"
	ProviderSystemSettingValueType = "system_setting_value_type"
)

type providerFunc func() any

type Registry struct {
	providers map[string]providerFunc
}

func NewRegistry() *Registry {
	registry := &Registry{providers: map[string]providerFunc{}}
	registry.Register(ProviderCommonStatus, func() any { return CommonStatusOptions() })
	registry.Register(ProviderCommonYesNo, func() any { return CommonYesNoOptions() })
	registry.Register(ProviderPlatform, func() any { return PlatformOptions() })
	registry.Register(ProviderSystemSettingValueType, func() any { return SystemSettingValueTypeOptions() })
	return registry
}

func (r *Registry) Register(name string, provider providerFunc) {
	if r == nil || name == "" || provider == nil {
		return
	}
	r.providers[name] = provider
}

func (r *Registry) Options(name string) (any, bool) {
	if r == nil {
		return nil, false
	}
	provider, ok := r.providers[name]
	if !ok {
		return nil, false
	}
	return provider(), true
}

type Service struct {
	registry *Registry
}

func NewService(registry *Registry) *Service {
	if registry == nil {
		registry = NewRegistry()
	}
	return &Service{registry: registry}
}

func (s *Service) Options(name string) (any, bool) {
	if s == nil {
		return nil, false
	}
	return s.registry.Options(name)
}
