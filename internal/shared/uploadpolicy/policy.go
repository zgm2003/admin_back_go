package uploadpolicy

import (
	"context"
	"crypto/sha256"
	"errors"
)

var ErrRuleSnapshotChanged = errors.New("active upload rule snapshot changed")

type ConsistencyToken [sha256.Size]byte

type Rule struct {
	MaxFileBytes     int64
	ImageExtensions  []string
	FileExtensions   []string
	ConsistencyToken ConsistencyToken
}

type Resolver interface {
	ResolveActive(context.Context) (Rule, error)
}

type ResolverFunc func(context.Context) (Rule, error)

func (resolve ResolverFunc) ResolveActive(ctx context.Context) (Rule, error) {
	return resolve(ctx)
}
