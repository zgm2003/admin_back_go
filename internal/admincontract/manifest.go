package admincontract

import (
	"crypto/sha256"
	"encoding/hex"
)

const (
	BundleVersion           = "admin-2026-07-15.1"
	OpenAPIVersion          = "3.1.0"
	PermissionSchemaVersion = "admin-permissions-2026-07-15.1"
	ViewSchemaVersion       = "admin-views-2026-07-15.1"
	RealtimeSchemaVersion   = "admin-realtime-2026-07-18.1"
)

type Manifest struct {
	BundleVersion     string              `json:"bundle_version"`
	OpenAPIVersion    string              `json:"openapi_version"`
	PermissionVersion string              `json:"permission_version"`
	RealtimeVersion   string              `json:"realtime_version"`
	BackendCommit     string              `json:"backend_commit"`
	Artifacts         map[string]Artifact `json:"artifacts"`
}

type Artifact struct {
	SHA256        string `json:"sha256"`
	SchemaVersion string `json:"schema_version"`
}

func newManifest(backendCommit string, artifacts map[string][]byte) Manifest {
	entries := make(map[string]Artifact, len(artifacts))
	for name, data := range artifacts {
		entries[name] = Artifact{
			SHA256:        sha256Hex(data),
			SchemaVersion: artifactSchemaVersion(name),
		}
	}
	return Manifest{
		BundleVersion:     BundleVersion,
		OpenAPIVersion:    OpenAPIVersion,
		PermissionVersion: PermissionSchemaVersion,
		RealtimeVersion:   RealtimeSchemaVersion,
		BackendCommit:     backendCommit,
		Artifacts:         entries,
	}
}

func artifactSchemaVersion(name string) string {
	switch name {
	case "openapi.json":
		return OpenAPIVersion
	case "permissions.json":
		return PermissionSchemaVersion
	case "views.json":
		return ViewSchemaVersion
	case "realtime/envelope.schema.json", "realtime/events.schema.json":
		return RealtimeSchemaVersion
	default:
		return BundleVersion
	}
}

func sha256Hex(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}
