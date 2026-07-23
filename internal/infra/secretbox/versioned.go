package secretbox

import (
	"errors"
	"fmt"
	"strings"
)

var (
	ErrMissingCurrentKeyID = errors.New("secretbox: current key ID is required")
	ErrUnknownKeyID        = errors.New("secretbox: unknown key ID")
)

type VersionedBox struct {
	currentKeyID string
	boxes        map[string]Box
}

func NewVersioned(currentKeyID string, keys map[string][]byte) (VersionedBox, error) {
	currentKeyID = strings.TrimSpace(currentKeyID)
	if currentKeyID == "" {
		return VersionedBox{}, ErrMissingCurrentKeyID
	}

	boxes := make(map[string]Box, len(keys))
	for keyID, key := range keys {
		trimmedKeyID := strings.TrimSpace(keyID)
		if trimmedKeyID == "" {
			return VersionedBox{}, fmt.Errorf("%w: key ID is empty", ErrUnknownKeyID)
		}
		if _, exists := boxes[trimmedKeyID]; exists {
			return VersionedBox{}, fmt.Errorf("%w: duplicate key ID %q", ErrUnknownKeyID, trimmedKeyID)
		}
		if len(key) != 32 {
			return VersionedBox{}, fmt.Errorf("%w for key ID %q", ErrInvalidKey, trimmedKeyID)
		}
		boxes[trimmedKeyID] = New(key)
	}
	if _, exists := boxes[currentKeyID]; !exists {
		return VersionedBox{}, fmt.Errorf("%w: current key ID %q", ErrUnknownKeyID, currentKeyID)
	}

	return VersionedBox{currentKeyID: currentKeyID, boxes: boxes}, nil
}

func (b VersionedBox) CurrentKeyID() string {
	return b.currentKeyID
}

func (b VersionedBox) Encrypt(plain string) (keyID, ciphertext string, err error) {
	if b.currentKeyID == "" {
		return "", "", ErrMissingCurrentKeyID
	}
	box, exists := b.boxes[b.currentKeyID]
	if !exists {
		return "", "", fmt.Errorf("%w: current key ID %q", ErrUnknownKeyID, b.currentKeyID)
	}
	ciphertext, err = box.Encrypt(plain)
	if err != nil {
		return "", "", err
	}
	return b.currentKeyID, ciphertext, nil
}

func (b VersionedBox) Decrypt(keyID, ciphertext string) (string, error) {
	box, exists := b.boxes[keyID]
	if !exists {
		return "", fmt.Errorf("%w: %q", ErrUnknownKeyID, keyID)
	}
	return box.Decrypt(ciphertext)
}
