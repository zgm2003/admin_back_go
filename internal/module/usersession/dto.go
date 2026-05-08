package usersession

import "time"

const (
	SessionStatusActive  = "active"
	SessionStatusExpired = "expired"
	SessionStatusRevoked = "revoked"
)

type Option[T string] struct {
	Label string `json:"label"`
	Value T      `json:"value"`
}

type PageInitResponse struct {
	Dict PageInitDict `json:"dict"`
}

type PageInitDict struct {
	PlatformArr []Option[string] `json:"platformArr"`
	StatusArr   []Option[string] `json:"statusArr"`
}

type ListQuery struct {
	CurrentPage int
	PageSize    int
	Username    string
	Platform    string
	Status      string
	Now         time.Time
}

type Page struct {
	PageSize    int   `json:"page_size"`
	CurrentPage int   `json:"current_page"`
	TotalPage   int   `json:"total_page"`
	Total       int64 `json:"total"`
}

type ListResponse struct {
	List []ListItem `json:"list"`
	Page Page       `json:"page"`
}

type ListItem struct {
	ID               int64   `json:"id"`
	UserID           int64   `json:"user_id"`
	Username         string  `json:"username"`
	Platform         string  `json:"platform"`
	PlatformName     string  `json:"platform_name"`
	DeviceID         string  `json:"device_id"`
	IP               string  `json:"ip"`
	UserAgent        string  `json:"ua"`
	LastSeenAt       string  `json:"last_seen_at"`
	CreatedAt        string  `json:"created_at"`
	ExpiresAt        string  `json:"expires_at"`
	RefreshExpiresAt string  `json:"refresh_expires_at"`
	RevokedAt        *string `json:"revoked_at"`
	Status           string  `json:"status"`
}

type ListRow struct {
	ID               int64
	UserID           int64
	Username         string
	Platform         string
	DeviceID         string
	IP               string
	UserAgent        string
	LastSeenAt       time.Time
	CreatedAt        time.Time
	ExpiresAt        time.Time
	RefreshExpiresAt time.Time
	RevokedAt        *time.Time
}

type StatsResponse struct {
	TotalActive          int64            `json:"total_active"`
	PlatformDistribution map[string]int64 `json:"platform_distribution"`
}

type StatsRow struct {
	Platform string
	Total    int64
}

type RevokeResponse struct {
	ID      int64 `json:"id"`
	Revoked bool  `json:"revoked"`
}

type BatchRevokeInput struct {
	IDs []int64
}

type BatchRevokeResponse struct {
	Count                 int64 `json:"count"`
	SkippedCurrent        int   `json:"skipped_current"`
	SkippedAlreadyRevoked int   `json:"skipped_already_revoked"`
}

type SessionRecord struct {
	ID              int64
	UserID          int64
	Platform        string
	AccessTokenHash string
	RevokedAt       *time.Time
}
