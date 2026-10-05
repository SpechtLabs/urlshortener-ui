package config

import "testing"

func TestNewConfigFromEnv(t *testing.T) {
	want := Config{
		ClientID:     "client-id",
		ClientSecret: "client-secret",
		RedirectURL:  "https://dash.example.com/oauth/redirect",
		DashboardURL: "dash.example.com",
		ShortlinkURL: "https://s.example.com",
	}

	t.Setenv("CLIENT_ID", want.ClientID)
	t.Setenv("CLIENT_SECRET", want.ClientSecret)
	t.Setenv("REDIRECT_URL", want.RedirectURL)
	t.Setenv("DASHBOARD_URL", want.DashboardURL)
	t.Setenv("SHORTLINK_URL", want.ShortlinkURL)

	if got := NewConfigFromEnv(); *got != want {
		t.Errorf("NewConfigFromEnv() = %+v, want %+v", *got, want)
	}
}
