package session

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

type Session struct {
	ID               int64      `gorm:"column:id"`
	UserID           int64      `gorm:"column:user_id"`
	AccessTokenHash  string     `gorm:"column:access_token_hash"`
	RefreshTokenHash string     `gorm:"column:refresh_token_hash"`
	Platform         string     `gorm:"column:platform"`
	DeviceID         string     `gorm:"column:device_id"`
	IP               string     `gorm:"column:ip"`
	UserAgent        string     `gorm:"column:ua"`
	ExpiresAt        time.Time  `gorm:"column:expires_at"`
	RefreshExpiresAt time.Time  `gorm:"column:refresh_expires_at"`
	RevokedAt        *time.Time `gorm:"column:revoked_at"`
	IsDel            int        `gorm:"column:is_del"`
}

func (Session) TableName() string {
	return "user_sessions"
}

func parseCachedSession(value string, loc *time.Location) (*Session, error) {
	parts := strings.Split(value, "|")
	if len(parts) < 4 {
		return nil, fmt.Errorf("invalid cached session")
	}

	userID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return nil, err
	}
	expiresAt, err := parseSessionTime(parts[1], loc)
	if err != nil {
		return nil, err
	}

	session := &Session{
		UserID:    userID,
		ExpiresAt: expiresAt,
		IP:        parts[2],
		Platform:  parts[3],
	}
	if len(parts) >= 5 {
		session.DeviceID = parts[4]
	}
	if len(parts) >= 6 && parts[5] != "" {
		session.ID, err = strconv.ParseInt(parts[5], 10, 64)
		if err != nil {
			return nil, err
		}
	}
	return session, nil
}

func cacheValue(session *Session) string {
	return fmt.Sprintf("%d|%s|%s|%s|%s|%d",
		session.UserID,
		session.ExpiresAt.Format("2006-01-02 15:04:05"),
		session.IP,
		session.Platform,
		session.DeviceID,
		session.ID,
	)
}

func parseSessionTime(value string, loc *time.Location) (time.Time, error) {
	if loc == nil {
		loc = time.Local
	}
	if parsed, err := time.ParseInLocation("2006-01-02 15:04:05", value, loc); err == nil {
		return parsed, nil
	}
	return time.Parse(time.RFC3339, value)
}
