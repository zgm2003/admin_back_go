package secretkey

import (
	"crypto/hkdf"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strings"
)

const keyLength = 32

type KeyRing struct {
	secretboxKey    []byte
	tokenPepper     string
	jwtSigningKey   []byte
	sessionCacheKey []byte
}

func NewKeyRing(rootSecret string) (*KeyRing, error) {
	root := strings.TrimSpace(rootSecret)
	if root == "" || root == "change_me_to_at_least_64_random_chars" || root == "change_me_to_long_random" {
		return nil, fmt.Errorf("APP_SECRET is missing or unsafe")
	}
	if len(root) < 32 {
		return nil, fmt.Errorf("APP_SECRET is too short: got %d chars, need at least 32", len(root))
	}
	tokenPepperKey, err := derive(root, "admin_go:token-pepper:v1")
	if err != nil {
		return nil, err
	}
	secretboxKey, err := derive(root, "admin_go:secretbox:v1")
	if err != nil {
		return nil, err
	}
	jwtSigningKey, err := derive(root, "admin_go:jwt-signing:v1")
	if err != nil {
		return nil, err
	}
	sessionCacheKey, err := derive(root, "admin_go:session-cache:v1")
	if err != nil {
		return nil, err
	}
	return &KeyRing{
		secretboxKey:    secretboxKey,
		tokenPepper:     base64.RawURLEncoding.EncodeToString(tokenPepperKey),
		jwtSigningKey:   jwtSigningKey,
		sessionCacheKey: sessionCacheKey,
	}, nil
}

func (k *KeyRing) SecretboxKey() []byte {
	if k == nil {
		return nil
	}
	return clone(k.secretboxKey)
}

func (k *KeyRing) TokenPepper() string {
	if k == nil {
		return ""
	}
	return k.tokenPepper
}

func (k *KeyRing) JWTSigningKey() []byte {
	if k == nil {
		return nil
	}
	return clone(k.jwtSigningKey)
}

func (k *KeyRing) SessionCacheKey() []byte {
	if k == nil {
		return nil
	}
	return clone(k.sessionCacheKey)
}

func derive(root string, info string) ([]byte, error) {
	key, err := hkdf.Key(sha256.New, []byte(root), nil, info, keyLength)
	if err != nil {
		return nil, fmt.Errorf("derive %s: %w", info, err)
	}
	return key, nil
}

func clone(in []byte) []byte {
	out := make([]byte, len(in))
	copy(out, in)
	return out
}
