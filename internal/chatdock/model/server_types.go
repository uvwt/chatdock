package model

type ServerConfig struct {
	Addr             string
	DataDir          string
	WebDir           string
	AuthToken        string
	AuthUsername     string
	AuthCredential   string
	PublicBaseURL    string
	EmbeddingBaseURL string
	EmbeddingAPIKey  string
	EmbeddingModel   string
}

type AuthStatusResponse struct {
	Enabled      bool   `json:"enabled"`
	LoginEnabled bool   `json:"login_enabled"`
	Username     string `json:"username,omitempty"`
}

type AuthLoginRequest struct {
	Username   string `json:"username"`
	Credential string `json:"credential"`
}

type AuthLoginResponse struct {
	OK       bool   `json:"ok"`
	Token    string `json:"token"`
	Username string `json:"username,omitempty"`
}
