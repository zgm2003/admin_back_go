package session

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
)

var ErrUnsafePepper = errors.New("token pepper is empty or unsafe")

func HashToken(token string, pepper string) (string, error) {
	if pepper == "" || pepper == "change_me_to_long_random" {
		return "", ErrUnsafePepper
	}

	sum := sha256.Sum256([]byte(token + "|" + pepper))
	return hex.EncodeToString(sum[:]), nil
}
