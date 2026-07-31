package uploadtoken

import (
	"context"
	"errors"
	"math"
	"reflect"
	"testing"

	"admin_back_go/internal/shared/uploadpolicy"
)

type activeRuleRepository struct {
	config *EnabledConfig
	err    error
	calls  int
}

func (repository *activeRuleRepository) GetEnabledConfig(context.Context) (*EnabledConfig, error) {
	repository.calls++
	return repository.config, repository.err
}

func TestActiveRuleResolverNormalizesCurrentEnabledRule(t *testing.T) {
	repository := &activeRuleRepository{config: validActiveRuleConfig()}
	resolver := NewActiveRuleResolver(repository)

	got, err := resolver.ResolveActive(context.Background())
	if err != nil {
		t.Fatalf("ResolveActive returned error: %v", err)
	}
	want := uploadpolicy.Rule{
		MaxFileBytes:    100 << 20,
		ImageExtensions: []string{"jpeg", "png"},
		FileExtensions:  []string{"pdf", "md", "go", "zip"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("active rule=%#v want=%#v", got, want)
	}
}

func TestActiveRuleResolverFailsClosed(t *testing.T) {
	type testCase struct {
		name       string
		repository Repository
	}
	tests := []testCase{
		{name: "repository missing", repository: nil},
		{name: "enabled config missing", repository: &activeRuleRepository{}},
		{name: "repository error", repository: &activeRuleRepository{err: errors.New("database unavailable")}},
		{name: "zero max size", repository: &activeRuleRepository{config: activeRuleConfigWith(0, `["png"]`, `["pdf"]`)}},
		{name: "negative max size", repository: &activeRuleRepository{config: activeRuleConfigWith(-1, `["png"]`, `["pdf"]`)}},
		{name: "malformed image extensions", repository: &activeRuleRepository{config: activeRuleConfigWith(1, `{`, `["pdf"]`)}},
		{name: "malformed file extensions", repository: &activeRuleRepository{config: activeRuleConfigWith(1, `["png"]`, `{`)}},
		{name: "unknown image extension", repository: &activeRuleRepository{config: activeRuleConfigWith(1, `["png","exe"]`, `["pdf"]`)}},
		{name: "unknown file extension", repository: &activeRuleRepository{config: activeRuleConfigWith(1, `["png"]`, `["pdf","exe"]`)}},
	}
	overflowMB64 := int64(math.MaxInt64)/(1<<20) + 1
	if overflowMB := int(overflowMB64); int64(overflowMB) == overflowMB64 {
		tests = append(tests, testCase{name: "max size overflow", repository: &activeRuleRepository{config: activeRuleConfigWith(overflowMB, `["png"]`, `["pdf"]`)}})
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resolver := NewActiveRuleResolver(test.repository)
			if _, err := resolver.ResolveActive(context.Background()); err == nil {
				t.Fatal("ResolveActive unexpectedly succeeded")
			}
		})
	}
}

func TestActiveRuleResolverReadsRepositoryOnEveryResolution(t *testing.T) {
	repository := &activeRuleRepository{config: validActiveRuleConfig()}
	resolver := NewActiveRuleResolver(repository)

	first, err := resolver.ResolveActive(context.Background())
	if err != nil {
		t.Fatalf("first ResolveActive returned error: %v", err)
	}
	repository.config = activeRuleConfigWith(2, `["avif"]`, `["yaml"]`)
	second, err := resolver.ResolveActive(context.Background())
	if err != nil {
		t.Fatalf("second ResolveActive returned error: %v", err)
	}

	if repository.calls != 2 {
		t.Fatalf("GetEnabledConfig calls=%d want=2", repository.calls)
	}
	if first.MaxFileBytes != 100<<20 || second.MaxFileBytes != 2<<20 {
		t.Fatalf("rules did not reflect repository changes: first=%#v second=%#v", first, second)
	}
	if !reflect.DeepEqual(second.ImageExtensions, []string{"avif"}) || !reflect.DeepEqual(second.FileExtensions, []string{"yaml"}) {
		t.Fatalf("second active rule=%#v", second)
	}
}

func TestUploadPolicyResolverFuncDelegates(t *testing.T) {
	called := false
	resolver := uploadpolicy.ResolverFunc(func(context.Context) (uploadpolicy.Rule, error) {
		called = true
		return uploadpolicy.Rule{MaxFileBytes: 1}, nil
	})

	got, err := resolver.ResolveActive(context.Background())
	if err != nil || !called || got.MaxFileBytes != 1 {
		t.Fatalf("ResolverFunc result=%#v called=%t err=%v", got, called, err)
	}
}

func validActiveRuleConfig() *EnabledConfig {
	return activeRuleConfigWith(100, `["png","jpeg"]`, `["pdf","md","go","zip"]`)
}

func activeRuleConfigWith(maxSizeMB int, imageExts string, fileExts string) *EnabledConfig {
	return &EnabledConfig{
		SettingID: 1,
		MaxSizeMB: maxSizeMB,
		ImageExts: imageExts,
		FileExts:  fileExts,
	}
}
