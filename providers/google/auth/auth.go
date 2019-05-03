package auth

import (
	"golang.org/x/oauth2/google"
	"golang.org/x/oauth2/jwt"
)

// Credentials represents a GCE providers credentials.
// This is compatible with golang.org/x/oauth2/google.JWTConfigFromJSON.
type Credentials struct {
	ClientEmail  string `json:"client_email"`
	PrivateKeyID string `json:"private_key_id"`
	PrivateKey   string `json:"private_key"`
	TokenURL     string `json:"token_uri"`
}

// JWTConfig returns JSON Web Tokens configuration for the credentials.
func (c *Credentials) JWTConfig(scopes []string) *jwt.Config {
	cfg := &jwt.Config{
		Email:        c.ClientEmail,
		PrivateKey:   []byte(c.PrivateKey),
		PrivateKeyID: c.PrivateKeyID,
		Scopes:       scopes,
		TokenURL:     c.TokenURL,
	}
	if cfg.TokenURL == "" {
		cfg.TokenURL = google.JWTTokenURL
	}
	return cfg
}
