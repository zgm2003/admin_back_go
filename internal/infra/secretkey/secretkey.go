package secretkey

import (
	"crypto/hkdf"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strings"
)

const keyLength = 32

const maxPreviousKeys = 1

const mailDiagnosticPurpose = "admin_go:mail-verification-diagnostic:v1"

type KeyRing struct {
	secretboxKey                 []byte
	tokenPepper                  string
	jwtSigningKey                []byte
	jwtSigningKeyID              string
	jwtVerifyKeys                map[string][]byte
	mailDiagnosticKey            []byte
	mailDiagnosticKeyID          string
	mailDiagnosticDecryptionKeys map[string][]byte
}

func NewKeyRing(rootSecret string) (*KeyRing, error) {
	return NewKeyRingWithPrevious(rootSecret, nil)
}

func NewKeyRingWithPrevious(rootSecret string, previousSecrets []string) (*KeyRing, error) {
	if len(previousSecrets) > maxPreviousKeys {
		return nil, fmt.Errorf("APP_SECRET_PREVIOUS supports at most %d key", maxPreviousKeys)
	}
	root := strings.TrimSpace(rootSecret)
	if err := validateRootSecret("APP_SECRET", root); err != nil {
		return nil, err
	}
	tokenPepperKey, err := derive(root, "admin_go:token-pepper:v1")
	if err != nil {
		return nil, err
	}
	secretboxKey, err := derive(root, "admin_go:secretbox:v1")
	if err != nil {
		return nil, err
	}
	mailDiagnosticKey, err := derive(root, mailDiagnosticPurpose)
	if err != nil {
		return nil, err
	}
	mailDiagnosticKeyID := diagnosticKeyID(mailDiagnosticKey)
	mailDiagnosticDecryptionKeys := map[string][]byte{mailDiagnosticKeyID: clone(mailDiagnosticKey)}
	jwtSigningKey, err := derive(root, "admin_go:jwt-signing:v1")
	if err != nil {
		return nil, err
	}
	jwtSigningKeyID := jwtKeyID(jwtSigningKey)
	jwtVerifyKeys := map[string][]byte{jwtSigningKeyID: clone(jwtSigningKey)}
	for _, candidate := range previousSecrets {
		previousRoot := strings.TrimSpace(candidate)
		if err := validateRootSecret("APP_SECRET_PREVIOUS", previousRoot); err != nil {
			return nil, err
		}
		if previousRoot == root {
			return nil, fmt.Errorf("APP_SECRET_PREVIOUS must differ from APP_SECRET")
		}
		previousJWTKey, err := derive(previousRoot, "admin_go:jwt-signing:v1")
		if err != nil {
			return nil, err
		}
		previousKeyID := jwtKeyID(previousJWTKey)
		if _, exists := jwtVerifyKeys[previousKeyID]; exists {
			return nil, fmt.Errorf("APP_SECRET_PREVIOUS duplicates the current JWT key ID")
		}
		jwtVerifyKeys[previousKeyID] = clone(previousJWTKey)

		previousMailDiagnosticKey, err := derive(previousRoot, mailDiagnosticPurpose)
		if err != nil {
			return nil, err
		}
		previousMailDiagnosticKeyID := diagnosticKeyID(previousMailDiagnosticKey)
		if _, exists := mailDiagnosticDecryptionKeys[previousMailDiagnosticKeyID]; exists {
			return nil, fmt.Errorf("APP_SECRET_PREVIOUS duplicates the current mail diagnostic key ID")
		}
		mailDiagnosticDecryptionKeys[previousMailDiagnosticKeyID] = clone(previousMailDiagnosticKey)
	}
	return &KeyRing{
		secretboxKey:                 secretboxKey,
		tokenPepper:                  base64.RawURLEncoding.EncodeToString(tokenPepperKey),
		jwtSigningKey:                jwtSigningKey,
		jwtSigningKeyID:              jwtSigningKeyID,
		jwtVerifyKeys:                jwtVerifyKeys,
		mailDiagnosticKey:            mailDiagnosticKey,
		mailDiagnosticKeyID:          mailDiagnosticKeyID,
		mailDiagnosticDecryptionKeys: mailDiagnosticDecryptionKeys,
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

func (k *KeyRing) JWTSigningKeyID() string {
	if k == nil {
		return ""
	}
	return k.jwtSigningKeyID
}

func (k *KeyRing) JWTVerificationKeys() map[string][]byte {
	if k == nil {
		return nil
	}
	keys := make(map[string][]byte, len(k.jwtVerifyKeys))
	for keyID, key := range k.jwtVerifyKeys {
		keys[keyID] = clone(key)
	}
	return keys
}

func (k *KeyRing) MailDiagnosticKey() []byte {
	if k == nil {
		return nil
	}
	return clone(k.mailDiagnosticKey)
}

func (k *KeyRing) MailDiagnosticKeyID() string {
	if k == nil {
		return ""
	}
	return k.mailDiagnosticKeyID
}

func (k *KeyRing) MailDiagnosticDecryptionKeys() map[string][]byte {
	if k == nil {
		return nil
	}
	keys := make(map[string][]byte, len(k.mailDiagnosticDecryptionKeys))
	for keyID, key := range k.mailDiagnosticDecryptionKeys {
		keys[keyID] = clone(key)
	}
	return keys
}

func derive(root string, info string) ([]byte, error) {
	key, err := hkdf.Key(sha256.New, []byte(root), nil, info, keyLength)
	if err != nil {
		return nil, fmt.Errorf("derive %s: %w", info, err)
	}
	return key, nil
}

func validateRootSecret(name string, root string) error {
	if root == "" || root == "change_me_to_at_least_64_random_chars" || root == "change_me_to_long_random" {
		return fmt.Errorf("%s is missing or unsafe", name)
	}
	if len(root) < 32 {
		return fmt.Errorf("%s is too short: got %d chars, need at least 32", name, len(root))
	}
	return nil
}

func jwtKeyID(key []byte) string {
	digest := sha256.Sum256(key)
	return "jwt-v1-" + base64.RawURLEncoding.EncodeToString(digest[:16])
}

func diagnosticKeyID(key []byte) string {
	digest := sha256.Sum256(key)
	return "mail-diagnostic-v1-" + base64.RawURLEncoding.EncodeToString(digest[:16])
}

func clone(in []byte) []byte {
	out := make([]byte, len(in))
	copy(out, in)
	return out
}
