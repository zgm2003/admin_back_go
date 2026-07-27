package admin

type feedbackRequest struct {
	Liked *bool `json:"liked" binding:"required"`
}
