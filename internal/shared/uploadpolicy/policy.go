package uploadpolicy

import "context"

type Rule struct {
	MaxFileBytes    int64
	ImageExtensions []string
	FileExtensions  []string
}

type Resolver interface {
	ResolveActive(context.Context) (Rule, error)
}

type ResolverFunc func(context.Context) (Rule, error)

func (resolve ResolverFunc) ResolveActive(ctx context.Context) (Rule, error) {
	return resolve(ctx)
}
