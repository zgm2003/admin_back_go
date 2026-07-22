package authplatform

import (
	"context"
	"errors"
	"testing"
	"time"

	authmodule "admin_back_go/internal/module/auth"
)

type fakeRepository struct {
	code     string
	platform *Platform
	err      error
}

func (f *fakeRepository) FindActiveByCode(ctx context.Context, code string) (*Platform, error) {
	f.code = code
	return f.platform, f.err
}

func TestServiceReturnsNilPolicyWhenPlatformIsMissing(t *testing.T) {
	repo := &fakeRepository{}
	service := NewService(repo)

	policy, err := service.Policy(context.Background(), "missing")

	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if policy != nil {
		t.Fatalf("expected nil policy, got %#v", policy)
	}
	if repo.code != "missing" {
		t.Fatalf("expected repository lookup by raw code, got %q", repo.code)
	}
}

func TestServiceDerivesSessionPolicyFromMaxSessions(t *testing.T) {
	for _, test := range []struct {
		name        string
		maxSessions int
		wantSingle  bool
	}{
		{name: "unlimited", maxSessions: 0, wantSingle: false},
		{name: "single", maxSessions: 1, wantSingle: true},
		{name: "limited", maxSessions: 3, wantSingle: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			service := NewService(&fakeRepository{platform: &Platform{
				Code:          "admin",
				BindPlatform:  1,
				BindDevice:    1,
				BindIP:        2,
				MaxSessions:   test.maxSessions,
				AllowRegister: 2,
				AccessTTL:     14400,
				RefreshTTL:    1209600,
			}})

			policy, err := service.Policy(context.Background(), "admin")
			if err != nil {
				t.Fatalf("expected nil error, got %v", err)
			}
			want := authmodule.AuthPolicy{
				BindPlatform:             true,
				BindDevice:               true,
				BindIP:                   false,
				SingleSessionPerPlatform: test.wantSingle,
				MaxSessions:              test.maxSessions,
				AllowRegister:            false,
				AccessTTL:                4 * time.Hour,
				RefreshTTL:               14 * 24 * time.Hour,
			}
			if *policy != want {
				t.Fatalf("unexpected policy: %#v", policy)
			}
		})
	}
}

func TestServiceReturnsLoginTypesInLegacyEnumOrder(t *testing.T) {
	service := NewService(&fakeRepository{platform: &Platform{
		Code:       "admin",
		LoginTypes: `["password","email","phone","unknown"]`,
	}})

	loginTypes, err := service.LoginTypes(context.Background(), "admin")

	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	want := []string{"email", "phone", "password"}
	if len(loginTypes) != len(want) {
		t.Fatalf("expected login types %#v, got %#v", want, loginTypes)
	}
	for i := range want {
		if loginTypes[i] != want[i] {
			t.Fatalf("expected login types %#v, got %#v", want, loginTypes)
		}
	}
}

func TestServiceReturnsNilLoginTypesWhenPlatformMissing(t *testing.T) {
	service := NewService(&fakeRepository{})

	loginTypes, err := service.LoginTypes(context.Background(), "missing")

	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if loginTypes != nil {
		t.Fatalf("expected nil login types, got %#v", loginTypes)
	}
}

func TestServiceReturnsRepositoryError(t *testing.T) {
	service := NewService(&fakeRepository{err: errors.New("mysql down")})

	policy, err := service.Policy(context.Background(), "admin")

	if policy != nil {
		t.Fatalf("expected nil policy, got %#v", policy)
	}
	if err == nil || err.Error() != "mysql down" {
		t.Fatalf("expected repository error, got %v", err)
	}
}
