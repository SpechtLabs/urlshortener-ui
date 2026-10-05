package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sierrasoftworks/humane-errors-go"
	"github.com/spechtlabs/go-otel-utils/otelzap"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.uber.org/zap"

	"github.com/spechtlabs/urlshortener-ui/pkg/swagger"
)

// gitHubTimeout bounds a request to GitHub.
const gitHubTimeout = 10 * time.Second

// loginAdvice tells a user whose login is missing or expired what to do.
const loginAdvice = "Log in again with GitHub"

// githubUser is the part of GitHub's user object the UI uses.
type githubUser struct {
	Login     string `json:"login,omitempty"`
	AvatarURL string `json:"avatar_url,omitempty"`
	Type      string `json:"type,omitempty"`
	Name      string `json:"name,omitempty"`
	Email     string `json:"email,omitempty"`
	ID        int    `json:"id,omitempty"`
}

// HTTPStatusText returns code and its text, the way an HTTP status line
// spells them ("404 Not Found").
func HTTPStatusText(code int) string {
	return fmt.Sprintf("%d %s", code, http.StatusText(code))
}

// loginToken returns the GitHub token the user logged in with, from the auth
// cookie.
func loginToken(ct *gin.Context) (string, humane.Error) {
	token, err := ct.Cookie(loginCookieName)
	if err != nil {
		return "", humane.Wrap(err, "Not logged in", loginAdvice)
	}

	token = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(token, "Bearer"), "token"))
	if token == "" {
		return "", humane.New("The login cookie holds no token", loginAdvice)
	}

	return token, nil
}

// apiContext returns ctx with token, which the API client sends as the
// bearer token.
func apiContext(ctx context.Context, token string) context.Context {
	return context.WithValue(ctx, swagger.ContextAccessToken, token)
}

// apiError turns an error from the urlshortener API into the status the UI
// answers with and a humane error. The API's own error message and advice,
// when it sent them, become the error's.
func apiError(err error, resp *http.Response, action string) (int, humane.Error) {
	status := http.StatusBadGateway
	if resp != nil {
		status = resp.StatusCode
	}

	if swaggerErr, ok := errors.AsType[swagger.GenericSwaggerError](err); ok {
		var body struct {
			Error  string   `json:"error"`
			Advice []string `json:"advice"`
		}

		if json.Unmarshal(swaggerErr.Body(), &body) == nil && body.Error != "" {
			return status, humane.Wrap(err, fmt.Sprintf("Unable to %s: %s", action, body.Error), body.Advice...)
		}
	}

	return status, humane.Wrap(err, fmt.Sprintf("Unable to %s", action),
		"Check that the urlshortener API at SHORTLINK_URL is reachable, then try again",
	)
}

// writeFailed reports whether a write to the urlshortener API failed. The
// generated client expects the int body its OpenAPI document declares for
// writes, but the API answers a write with the shortlink, or with nothing, so
// the client returns a decoding error for every successful write; a 2xx
// status is success whatever the body.
func writeFailed(resp *http.Response, err error) bool {
	return err != nil && (resp == nil || resp.StatusCode >= http.StatusMultipleChoices)
}

// renderAPIError answers a request whose call to the urlshortener API failed,
// the way apiError and renderError describe.
func renderAPIError(ct *gin.Context, err error, resp *http.Response, action string) {
	status, herr := apiError(err, resp, action)
	renderError(ct, status, herr)
}

// redirectToLogin sends a user whose login is missing or invalid to the login
// page. That's routine, so it's logged at debug level.
func redirectToLogin(ct *gin.Context, err humane.Error) {
	otelzap.L().WithError(err).DebugContext(ct.Request.Context(), err.Error(),
		zap.String("path", ct.Request.URL.Path),
	)

	ct.Redirect(http.StatusFound, "/login")
}

// renderError logs err as the request's error event and answers with the
// error page for status: 404.html for 404, and 500.html for everything else.
// An expired login (401) goes back to the login page instead.
func renderError(ct *gin.Context, status int, err humane.Error) {
	if status == http.StatusUnauthorized {
		redirectToLogin(ct, err)
		return
	}

	otelzap.L().WithError(err).ErrorContext(ct.Request.Context(), err.Error(),
		zap.String("path", ct.Request.URL.Path),
		zap.Int("status", status),
		zap.Strings("advice", err.Advice()),
	)

	page := "500.html"
	if status == http.StatusNotFound {
		page = "404.html"
	}

	ct.HTML(status, page, gin.H{})
}

// getGhUser asks GitHub whose token bearerToken is.
func (c *UIClient) getGhUser(ctx context.Context, bearerToken string) (*githubUser, humane.Error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.gitHubAPIURL+"/user", http.NoBody)
	if err != nil {
		return nil, humane.Wrap(err, "Failed to build request to fetch GitHub API", "This is a bug in the GitHub user lookup; please report it")
	}

	req.Header.Add("Accept", "application/vnd.github.v3+json")
	req.Header.Add("Authorization", "token "+bearerToken)

	client := &http.Client{
		Transport: otelhttp.NewTransport(http.DefaultTransport),
		Timeout:   gitHubTimeout,
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, humane.Wrap(err, "Failed to fetch UserInfo from GitHub API", "Check that the UI can reach api.github.com, then try again")
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, humane.New(fmt.Sprintf("bad credentials: GitHub answered %s", resp.Status), loginAdvice)
	}

	user := &githubUser{}
	if err := json.NewDecoder(resp.Body).Decode(user); err != nil {
		return nil, humane.Wrap(err, "Failed to unmarshal GitHub UserInfo", "GitHub answered with something other than a user object; try again later")
	}

	return user, nil
}
