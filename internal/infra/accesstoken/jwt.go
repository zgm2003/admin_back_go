package accesstoken

import (
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	SessionID int64
	UserID    int64
	Issuer    string
	Platform  string
	DeviceID  string
	IssuedAt  time.Time
	NotBefore time.Time
	ExpiresAt time.Time
}

type Codec interface {
	Issue(Claims) (string, error)
	Parse(token string, now time.Time) (Claims, error)
}

type Options struct {
	Issuer string
}

type JWTCodec struct {
	signingKey      []byte
	signingKeyID    string
	verificationKey map[string][]byte
	issuer          string
}

func NewJWTCodec(signingKey []byte, opts Options) *JWTCodec {
	keyID := defaultKeyID(signingKey)
	keys := make(map[string][]byte, 1)
	if len(signingKey) > 0 {
		keys[keyID] = cloneKey(signingKey)
	}
	return newJWTCodec(keyID, signingKey, keys, opts)
}

func NewRotatingJWTCodec(currentKeyID string, verificationKeys map[string][]byte, opts Options) (*JWTCodec, error) {
	currentKeyID = strings.TrimSpace(currentKeyID)
	if currentKeyID == "" {
		return nil, errors.New("current access token key ID is not configured")
	}
	if len(verificationKeys) == 0 || len(verificationKeys) > 2 {
		return nil, errors.New("access token verification keys require current and at most one previous key")
	}
	keys := make(map[string][]byte, len(verificationKeys))
	for rawKeyID, key := range verificationKeys {
		keyID := strings.TrimSpace(rawKeyID)
		if keyID == "" || keyID != rawKeyID {
			return nil, errors.New("access token key ID is invalid")
		}
		if len(key) < 32 {
			return nil, fmt.Errorf("access token key %q is too short", keyID)
		}
		keys[keyID] = cloneKey(key)
	}
	currentKey, ok := keys[currentKeyID]
	if !ok {
		return nil, errors.New("current access token key is absent from verification keys")
	}
	return newJWTCodec(currentKeyID, currentKey, keys, opts), nil
}

func newJWTCodec(currentKeyID string, signingKey []byte, verificationKeys map[string][]byte, opts Options) *JWTCodec {
	issuer := strings.TrimSpace(opts.Issuer)
	if issuer == "" {
		issuer = "admin_go"
	}
	return &JWTCodec{
		signingKey:      cloneKey(signingKey),
		signingKeyID:    currentKeyID,
		verificationKey: verificationKeys,
		issuer:          issuer,
	}
}

func (c *JWTCodec) Issue(claims Claims) (string, error) {
	if c == nil || len(c.signingKey) == 0 || strings.TrimSpace(c.signingKeyID) == "" {
		return "", errors.New("access token signing key is not configured")
	}
	if claims.SessionID <= 0 || claims.UserID <= 0 {
		return "", errors.New("access token claims require session_id and user_id")
	}
	if !claims.ExpiresAt.After(claims.IssuedAt) {
		return "", errors.New("access token expiry must be after issued_at")
	}
	notBefore := claims.NotBefore
	if notBefore.IsZero() {
		notBefore = claims.IssuedAt
	}
	payload := jwt.MapClaims{
		"iss":       c.issuer,
		"sub":       strconv.FormatInt(claims.UserID, 10),
		"iat":       claims.IssuedAt.Unix(),
		"nbf":       notBefore.Unix(),
		"exp":       claims.ExpiresAt.Unix(),
		"sid":       claims.SessionID,
		"platform":  claims.Platform,
		"device_id": claims.DeviceID,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, payload)
	token.Header["kid"] = c.signingKeyID
	return token.SignedString(c.signingKey)
}

func (c *JWTCodec) Parse(tokenString string, now time.Time) (Claims, error) {
	if c == nil || len(c.verificationKey) == 0 {
		return Claims{}, errors.New("access token signing key is not configured")
	}
	claims := jwt.MapClaims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		keyID, ok := token.Header["kid"].(string)
		if !ok || strings.TrimSpace(keyID) == "" || keyID != strings.TrimSpace(keyID) {
			return nil, errors.New("access token key ID is missing or invalid")
		}
		key, ok := c.verificationKey[keyID]
		if !ok || len(key) == 0 {
			return nil, errors.New("access token key ID is unknown")
		}
		return key, nil
	}, jwt.WithIssuer(c.issuer), jwt.WithIssuedAt(), jwt.WithTimeFunc(func() time.Time { return now }))
	if err != nil {
		return Claims{}, err
	}
	if token == nil || !token.Valid {
		return Claims{}, errors.New("invalid access token")
	}
	userID, err := strconv.ParseInt(fmt.Sprint(claims["sub"]), 10, 64)
	if err != nil {
		return Claims{}, errors.New("invalid access token subject")
	}
	sessionID, err := claimInt64(claims["sid"])
	if err != nil {
		return Claims{}, errors.New("invalid access token session id")
	}
	iat, err := claimInt64(claims["iat"])
	if err != nil {
		return Claims{}, errors.New("invalid access token iat")
	}
	exp, err := claimInt64(claims["exp"])
	if err != nil {
		return Claims{}, errors.New("invalid access token exp")
	}
	nbf, err := claimInt64(claims["nbf"])
	if err != nil {
		return Claims{}, errors.New("invalid access token nbf")
	}
	return Claims{
		SessionID: sessionID,
		UserID:    userID,
		Issuer:    fmt.Sprint(claims["iss"]),
		Platform:  fmt.Sprint(claims["platform"]),
		DeviceID:  fmt.Sprint(claims["device_id"]),
		IssuedAt:  time.Unix(iat, 0),
		NotBefore: time.Unix(nbf, 0),
		ExpiresAt: time.Unix(exp, 0),
	}, nil
}

func defaultKeyID(key []byte) string {
	digest := sha256.Sum256(key)
	return "jwt-v1-" + base64.RawURLEncoding.EncodeToString(digest[:16])
}

func cloneKey(key []byte) []byte {
	return append([]byte(nil), key...)
}

func claimInt64(value any) (int64, error) {
	switch v := value.(type) {
	case float64:
		return int64(v), nil
	case int64:
		return v, nil
	default:
		return 0, fmt.Errorf("invalid number claim %T", value)
	}
}
