package captcha

const TypeSlide = "slide"

// Answer is the public user answer submitted with the login request.
type Answer struct {
	X int `json:"x"`
	Y int `json:"y"`
}

// ChallengeResponse is the REST payload for a generated CAPTCHA challenge.
type ChallengeResponse struct {
	CaptchaID   string `json:"captcha_id"`
	CaptchaType string `json:"captcha_type"`
	MasterImage string `json:"master_image"`
	TileImage   string `json:"tile_image"`
	TileX       int    `json:"tile_x"`
	TileY       int    `json:"tile_y"`
	TileWidth   int    `json:"tile_width"`
	TileHeight  int    `json:"tile_height"`
	ImageWidth  int    `json:"image_width"`
	ImageHeight int    `json:"image_height"`
	ExpiresIn   int    `json:"expires_in"`
}

// VerifyInput is the service boundary used by auth login.
type VerifyInput struct {
	ID        string
	Answer    *Answer
	ClientIP  string
	UserAgent string
}
