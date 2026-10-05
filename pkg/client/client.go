// Package client serves the UI's pages. It reads and writes the user's
// shortlinks through the urlshortener API, with the GitHub token they logged
// in with.
package client

import (
	"go.opentelemetry.io/otel/trace"

	"github.com/spechtlabs/urlshortener-ui/pkg/config"
	"github.com/spechtlabs/urlshortener-ui/pkg/swagger"
)

const (
	// loginCookieName is the cookie that holds the user's GitHub token.
	loginCookieName = "auth"

	// loginCookieMaxAge is how long the login lasts, in seconds.
	loginCookieMaxAge = 3600

	// gitHubAPIURL and gitHubLoginURL are GitHub's REST API and its OAuth token
	// endpoint.
	gitHubAPIURL   = "https://api.github.com"
	gitHubLoginURL = "https://github.com/login/oauth/access_token"
)

// UIClient serves the UI's pages.
type UIClient struct {
	tracer    trace.Tracer
	config    *config.Config
	apiClient *swagger.APIClient

	// gitHubAPIURL and gitHubLoginURL are the GitHub endpoints the UI talks
	// to; the tests point them at fakes.
	gitHubAPIURL   string
	gitHubLoginURL string
}

// NewUIClient returns a UIClient that reads and writes shortlinks with
// apiClient.
func NewUIClient(tracer trace.Tracer, conf *config.Config, apiClient *swagger.APIClient) *UIClient {
	return &UIClient{
		tracer:         tracer,
		config:         conf,
		apiClient:      apiClient,
		gitHubAPIURL:   gitHubAPIURL,
		gitHubLoginURL: gitHubLoginURL,
	}
}
