package uploadtoken

type createRequest struct {
	Folder   string `json:"folder" binding:"required,upload_folder"`
	FileName string `json:"file_name" binding:"required,max=255"`
	FileSize int64  `json:"file_size" binding:"required,min=1"`
	FileKind string `json:"file_kind" binding:"required,oneof=image file"`
}
