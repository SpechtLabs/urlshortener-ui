package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/sierrasoftworks/humane-errors-go"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/spechtlabs/urlshortener-ui/pkg/model"
)

// HandleRoot sends a logged-in user to their shortlinks and everyone else to
// the login page.
func (c *UIClient) HandleRoot(ct *gin.Context) {
	ctx, span := c.startSpan(ct, "UIClient.HandleRoot")
	defer span.End()

	token, err := loginToken(ct)
	if err != nil {
		redirectToLogin(ct, err)
		return
	}

	if _, err := c.getGhUser(ctx, token); err != nil {
		span.RecordError(err)
		redirectToLogin(ct, err)
		return
	}

	ct.Redirect(http.StatusFound, "/home")
}

// HandleLogin renders the login page, which sends the user to GitHub.
func (c *UIClient) HandleLogin(ct *gin.Context) {
	_, span := c.startSpan(ct, "UIClient.HandleLogin")
	defer span.End()

	span.SetAttributes(attribute.String("redirect_uri", c.config.RedirectURL))

	ct.HTML(http.StatusOK, "login.html", gin.H{
		"clientID":     c.config.ClientID,
		"redirect_uri": c.config.RedirectURL,
	})
}

// HandleLoginOauthRedirect is where GitHub sends the user back to: it trades
// the code GitHub passed for the user's token, keeps the token in the auth
// cookie and sends the user to their shortlinks.
func (c *UIClient) HandleLoginOauthRedirect(ct *gin.Context) {
	ctx, span := c.startSpan(ct, "UIClient.HandleLoginOauthRedirect")
	defer span.End()

	code := ct.Query("code")
	if code == "" {
		err := humane.New(fmt.Sprintf("GitHub login failed: %s: %s", ct.Query("error"), ct.Query("error_description")), loginAdvice)
		span.RecordError(err)
		redirectToLogin(ct, err)
		return
	}

	token, err := c.exchangeCode(ctx, code)
	if err != nil {
		span.RecordError(err)
		renderError(ct, http.StatusInternalServerError, err)
		return
	}

	ct.SetCookie(loginCookieName, token, loginCookieMaxAge, "/", c.config.DashboardURL, true, true)
	ct.Redirect(http.StatusFound, "/home")
}

// exchangeCode trades the code GitHub passed to the redirect for the user's
// token. The client secret travels in the request body, so it stays out of
// the request URL that traces record.
func (c *UIClient) exchangeCode(ctx context.Context, code string) (string, humane.Error) {
	form := url.Values{
		"client_id":     {c.config.ClientID},
		"client_secret": {c.config.ClientSecret},
		"code":          {code},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.gitHubLoginURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", humane.Wrap(err, "could not create HTTP request", "This is a bug in the GitHub login; please report it")
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	client := &http.Client{
		Transport: otelhttp.NewTransport(http.DefaultTransport),
		Timeout:   gitHubTimeout,
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", humane.Wrap(err, "could not send HTTP request", "Check that the UI can reach github.com, then try again")
	}
	defer func() { _ = resp.Body.Close() }()

	var access model.OAuthAccessResponse
	if err := json.NewDecoder(resp.Body).Decode(&access); err != nil {
		return "", humane.Wrap(err, "could not parse JSON response", "GitHub answered with something other than a token; try again later")
	}

	if access.AccessToken == "" {
		return "", humane.New("GitHub answered without an access token",
			"Check that CLIENT_ID and CLIENT_SECRET belong to the GitHub OAuth app, then log in again",
		)
	}

	return access.AccessToken, nil
}

// startSpan returns the request's span, or starts one named name when the
// request's span isn't recording. The caller ends it.
func (c *UIClient) startSpan(ct *gin.Context, name string) (context.Context, trace.Span) {
	ctx := ct.Request.Context()
	if span := trace.SpanFromContext(ctx); span.IsRecording() {
		// Ending the request's span is otelgin's job.
		return ctx, nonEndingSpan{span}
	}

	return c.tracer.Start(ctx, name)
}

// nonEndingSpan is a span whose End does nothing, for handing out a span
// someone else ends.
type nonEndingSpan struct {
	trace.Span
}

// End does nothing; the span's owner ends it.
func (nonEndingSpan) End(...trace.SpanEndOption) {}
