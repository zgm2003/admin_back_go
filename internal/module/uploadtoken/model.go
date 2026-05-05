package uploadtoken

type EnabledConfig struct {
	SettingID    int64
	DriverID     int64
	RuleID       int64
	Driver       string
	SecretIDEnc  string
	SecretKeyEnc string
	Bucket       string
	Region       string
	AppID        string
	Endpoint     string
	BucketDomain string
	RoleARN      string
	MaxSizeMB    int
	ImageExts    string
	FileExts     string
}
