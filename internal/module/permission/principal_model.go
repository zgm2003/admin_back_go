package permission

import (
	"fmt"
	"sort"
	"strings"
)

const PrincipalSchemaVersion = 1

type PrincipalSnapshot struct {
	UserID      int64    `json:"user_id"`
	RoleID      int64    `json:"role_id"`
	Platform    string   `json:"platform"`
	Version     uint64   `json:"version"`
	UserActive  bool     `json:"user_active"`
	RoleActive  bool     `json:"role_active"`
	RouteCodes  []string `json:"route_codes"`
	ButtonCodes []string `json:"button_codes"`
}

type PrincipalSubject struct {
	UserID   int64
	Platform string
}

type PrincipalVersion struct {
	UserID   int64
	RoleID   int64
	Platform string
	Version  uint64
}

type PrincipalCacheState int

const (
	PrincipalCacheMiss PrincipalCacheState = iota
	PrincipalCacheHit
	PrincipalCacheInvalidating
)

func PrincipalKey(userID, roleID int64, platform string, version uint64) string {
	return fmt.Sprintf("authz:principal:v1:%s:%d:%d:%d", strings.TrimSpace(platform), userID, roleID, version)
}

func principalStateKey(userID int64, platform string) string {
	return fmt.Sprintf("authz:principal-state:v1:%s:%d", strings.TrimSpace(platform), userID)
}

func normalizePrincipalSubjects(subjects []PrincipalSubject) []PrincipalSubject {
	seen := make(map[string]struct{}, len(subjects))
	result := make([]PrincipalSubject, 0, len(subjects))
	for _, subject := range subjects {
		subject.Platform = strings.TrimSpace(subject.Platform)
		if subject.UserID <= 0 || subject.Platform == "" {
			continue
		}
		key := fmt.Sprintf("%s:%d", subject.Platform, subject.UserID)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, subject)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Platform == result[j].Platform {
			return result[i].UserID < result[j].UserID
		}
		return result[i].Platform < result[j].Platform
	})
	return result
}

func principalSubjectsFromVersions(versions []PrincipalVersion) []PrincipalSubject {
	subjects := make([]PrincipalSubject, 0, len(versions))
	for _, version := range versions {
		subjects = append(subjects, PrincipalSubject{UserID: version.UserID, Platform: version.Platform})
	}
	return normalizePrincipalSubjects(subjects)
}
