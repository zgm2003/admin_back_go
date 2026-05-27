package verifycode

import (
	"crypto/md5"
	"encoding/hex"
	"strings"
)

const defaultRedisPrefix = "auth:verify_code:"

func CacheKey(accountType string, scene string, account string) string {
	return defaultRedisPrefix + key(accountType, scene, account)
}

func key(accountType string, scene string, account string) string {
	sum := md5.Sum([]byte(strings.TrimSpace(account)))
	return accountType + ":" + strings.TrimSpace(scene) + ":" + hex.EncodeToString(sum[:])
}
