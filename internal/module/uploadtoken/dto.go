package uploadtoken

type CreateInput struct {
	Folder   string
	FileName string
	FileSize int64
	FileKind string
}

type CreateResponse struct {
	Provider     string         `json:"provider"`
	Bucket       string         `json:"bucket"`
	Region       string         `json:"region"`
	Key          string         `json:"key"`
	UploadPath   string         `json:"upload_path"`
	BucketDomain *string        `json:"bucket_domain"`
	Credentials  CredentialsDTO `json:"credentials"`
	StartTime    int64          `json:"start_time"`
	ExpiredTime  int64          `json:"expired_time"`
	Rule         UploadRuleDTO  `json:"rule"`
}

type CredentialsDTO struct {
	TmpSecretID  string `json:"tmp_secret_id"`
	TmpSecretKey string `json:"tmp_secret_key"`
	SessionToken string `json:"session_token"`
}

type UploadRuleDTO struct {
	MaxSizeMB int      `json:"max_size_mb"`
	ImageExts []string `json:"image_exts"`
	FileExts  []string `json:"file_exts"`
}
