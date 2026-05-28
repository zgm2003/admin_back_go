package accesstoken

import (
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
	Platform  string
	DeviceID  string
	IssuedAt  time.Time
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
	signingKey []byte
	issuer     string
}

func NewJWTCodec(signingKey []byte, opts Options) *JWTCodec {
	key := make([]byte, len(signingKey))
	copy(key, signingKey)
	issuer := strings.TrimSpace(opts.Issuer)
	if issuer == "" {
		issuer = "admin_go"
	}
	return &JWTCodec{signingKey: key, issuer: issuer}
}

func (c *JWTCodec) Issue(claims Claims) (string, error) {
	if c == nil || len(c.signingKey) == 0 {
		return "", errors.New("access token signing key is not configured")
	}
	if claims.SessionID <= 0 || claims.UserID <= 0 {
		return "", errors.New("access token claims require session_id and user_id")
	}
	if !claims.ExpiresAt.After(claims.IssuedAt) {
		return "", errors.New("access token expiry must be after issued_at")
	}
	payload := jwt.MapClaims{
		"iss":       c.issuer,
		"sub":       strconv.FormatInt(claims.UserID, 10),
		"iat":       claims.IssuedAt.Unix(),
		"nbf":       claims.IssuedAt.Unix(),
		"exp":       claims.ExpiresAt.Unix(),
		"sid":       claims.SessionID,
		"platform":  claims.Platform,
		"device_id": claims.DeviceID,
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, payload).SignedString(c.signingKey)
}

func (c *JWTCodec) Parse(tokenString string, now time.Time) (Claims, error) {
	if c == nil || len(c.signingKey) == 0 {
		return Claims{}, errors.New("access token signing key is not configured")
	}
	claims := jwt.MapClaims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return c.signingKey, nil
	}, jwt.WithIssuer(c.issuer), jwt.WithTimeFunc(func() time.Time { return now }))
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
	return Claims{SessionID: sessionID, UserID: userID, Platform: fmt.Sprint(claims["platform"]), DeviceID: fmt.Sprint(claims["device_id"]), IssuedAt: time.Unix(iat, 0), ExpiresAt: time.Unix(exp, 0)}, nil
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
