package system

type HealthResponse struct {
	Service string `json:"service"`
	Status  string `json:"status"`
	Version string `json:"version"`
}

type PingResponse struct {
	Message string `json:"message"`
}
