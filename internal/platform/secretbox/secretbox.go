package secretbox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
)

const (
	nonceLength = 12
	tagLength   = 16
)

var (
	ErrMissingKey        = errors.New("secretbox: key is not configured")
	ErrInvalidKey        = errors.New("secretbox: key must be 32 bytes")
	ErrInvalidCiphertext = errors.New("secretbox: invalid ciphertext")
)

type Box struct {
	key []byte
}

func New(key []byte) Box {
	cloned := make([]byte, len(key))
	copy(cloned, key)
	return Box{key: cloned}
}

func (b Box) Encrypt(plain string) (string, error) {
	if plain == "" {
		return "", nil
	}

	aead, err := b.aead()
	if err != nil {
		return "", err
	}

	nonce := make([]byte, nonceLength)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("secretbox: generate nonce: %w", err)
	}

	sealed := aead.Seal(nil, nonce, []byte(plain), nil)
	ciphertext := sealed[:len(sealed)-tagLength]
	tag := sealed[len(sealed)-tagLength:]

	payload := make([]byte, 0, nonceLength+tagLength+len(ciphertext))
	payload = append(payload, nonce...)
	payload = append(payload, tag...)
	payload = append(payload, ciphertext...)

	return base64.StdEncoding.EncodeToString(payload), nil
}

func (b Box) Decrypt(ciphertext string) (string, error) {
	if ciphertext == "" {
		return "", nil
	}

	aead, err := b.aead()
	if err != nil {
		return "", err
	}

	payload, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", fmt.Errorf("%w: base64 decode", ErrInvalidCiphertext)
	}
	if len(payload) < nonceLength+tagLength {
		return "", fmt.Errorf("%w: payload too short", ErrInvalidCiphertext)
	}

	nonce := payload[:nonceLength]
	tag := payload[nonceLength : nonceLength+tagLength]
	encrypted := payload[nonceLength+tagLength:]

	sealed := make([]byte, 0, len(encrypted)+tagLength)
	sealed = append(sealed, encrypted...)
	sealed = append(sealed, tag...)

	plain, err := aead.Open(nil, nonce, sealed, nil)
	if err != nil {
		return "", fmt.Errorf("secretbox: decrypt: %w", err)
	}

	return string(plain), nil
}

func Hint(plain string) string {
	if plain == "" {
		return ""
	}
	if len(plain) <= 4 {
		return "***" + plain
	}
	return "***" + plain[len(plain)-4:]
}

func (b Box) aead() (cipher.AEAD, error) {
	if len(b.key) == 0 {
		return nil, ErrMissingKey
	}
	if len(b.key) != 32 {
		return nil, ErrInvalidKey
	}
	block, err := aes.NewCipher(b.key)
	if err != nil {
		return nil, fmt.Errorf("secretbox: create aes cipher: %w", err)
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("secretbox: create gcm: %w", err)
	}
	return aead, nil
}
