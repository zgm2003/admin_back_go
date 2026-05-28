package profile

type SaveInput struct {
	PermissionIDs []int64
}

type SaveResponse struct {
	QuickEntry []QuickEntry `json:"quick_entry"`
}

type QuickEntry struct {
	ID           int64 `json:"id"`
	PermissionID int64 `json:"permission_id"`
	Sort         int   `json:"sort"`
}
