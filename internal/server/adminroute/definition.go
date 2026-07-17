package adminroute

import "strings"

type AccessKind string

const (
	AccessPublic        AccessKind = "public"
	AccessAuthenticated AccessKind = "authenticated"
	AccessPermission    AccessKind = "permission"
)

type Access struct {
	Kind           AccessKind `json:"kind"`
	PermissionCode string     `json:"permission_code,omitempty"`
}

func Public() Access {
	return Access{Kind: AccessPublic}
}

func Authenticated() Access {
	return Access{Kind: AccessAuthenticated}
}

func Permission(code string) Access {
	return Access{Kind: AccessPermission, PermissionCode: strings.TrimSpace(code)}
}

type AuditDecision struct {
	Enabled bool   `json:"enabled"`
	Module  string `json:"module,omitempty"`
	Action  string `json:"action,omitempty"`
	Title   string `json:"title,omitempty"`
	Reason  string `json:"reason,omitempty"`

	SkipRequestPayload  bool `json:"skip_request_payload,omitempty"`
	SkipResponsePayload bool `json:"skip_response_payload,omitempty"`
}

func Audit(module string, action string, title string) AuditDecision {
	return AuditDecision{
		Enabled: true,
		Module:  strings.TrimSpace(module),
		Action:  strings.TrimSpace(action),
		Title:   strings.TrimSpace(title),
	}
}

func NoAudit(reason string) AuditDecision {
	return AuditDecision{Reason: strings.TrimSpace(reason)}
}

type Definition struct {
	Method         string        `json:"method"`
	Path           string        `json:"path"`
	OperationID    string        `json:"operation_id,omitempty"`
	Access         Access        `json:"access"`
	Audit          AuditDecision `json:"audit"`
	Tags           []string      `json:"tags,omitempty"`
	RequestSchema  string        `json:"request_schema,omitempty"`
	ResponseSchema string        `json:"response_schema,omitempty"`
	SuccessStatus  int           `json:"success_status,omitempty"`
}
