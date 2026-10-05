// Package model holds the GitHub API responses the UI reads.
package model

// OAuthAccessResponse is GitHub's answer to the OAuth code exchange.
type OAuthAccessResponse struct {
	AccessToken string `json:"access_token"`
}
