// Package config holds the UI's configuration, which it reads from the
// environment.
package config

import (
	"os"
)

// Config is the UI's configuration.
type Config struct {
	// ClientID and ClientSecret identify the GitHub OAuth app users log in
	// with.
	ClientID     string
	ClientSecret string
	// RedirectURL is where GitHub sends the user back to after they logged
	// in: the UI's /oauth/redirect.
	RedirectURL string
	// DashboardURL is the UI's own host name; the login cookie is set for it.
	DashboardURL string
	// ShortlinkURL is the urlshortener API's base URL, which the shortlinks
	// are served under too.
	ShortlinkURL string
}

// NewConfigFromEnv reads the configuration from CLIENT_ID, CLIENT_SECRET,
// REDIRECT_URL, DASHBOARD_URL and SHORTLINK_URL.
func NewConfigFromEnv() *Config {
	return &Config{
		ClientID:     os.Getenv("CLIENT_ID"),
		ClientSecret: os.Getenv("CLIENT_SECRET"),
		RedirectURL:  os.Getenv("REDIRECT_URL"),
		DashboardURL: os.Getenv("DASHBOARD_URL"),
		ShortlinkURL: os.Getenv("SHORTLINK_URL"),
	}
}
