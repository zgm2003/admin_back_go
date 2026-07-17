package admincontract

import (
	"sort"

	"admin_back_go/internal/server/adminroute"
)

type PermissionsDocument struct {
	SchemaVersion   string            `json:"schema_version"`
	PermissionCodes []string          `json:"permission_codes"`
	Operations      []OperationPolicy `json:"operations"`
}

type OperationPolicy struct {
	OperationID string                   `json:"operation_id"`
	Method      string                   `json:"method"`
	Path        string                   `json:"path"`
	Access      adminroute.Access        `json:"access"`
	Audit       adminroute.AuditDecision `json:"audit"`
}

func buildPermissionsDocument(definitions []adminroute.Definition, views ViewsDocument) PermissionsDocument {
	codes := viewPermissionCodes(views)
	operations := make([]OperationPolicy, 0, len(definitions))
	for _, definition := range definitions {
		if definition.Access.Kind == adminroute.AccessPermission {
			codes = append(codes, definition.Access.PermissionCode)
		}
		operations = append(operations, OperationPolicy{
			OperationID: definition.OperationID,
			Method:      definition.Method,
			Path:        definition.Path,
			Access:      definition.Access,
			Audit:       definition.Audit,
		})
	}
	sort.Slice(operations, func(left int, right int) bool {
		if operations[left].Path != operations[right].Path {
			return operations[left].Path < operations[right].Path
		}
		if operations[left].Method != operations[right].Method {
			return operations[left].Method < operations[right].Method
		}
		return operations[left].OperationID < operations[right].OperationID
	})
	return PermissionsDocument{
		SchemaVersion:   PermissionSchemaVersion,
		PermissionCodes: sortedUnique(codes),
		Operations:      operations,
	}
}
