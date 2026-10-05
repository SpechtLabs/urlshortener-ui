package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/spechtlabs/urlshortener-ui/pkg/config"
	"github.com/spechtlabs/urlshortener-ui/pkg/swagger"
)

// validToken is the GitHub token the fake GitHub and the fake API accept.
const validToken = "gho_valid"

func TestHandleRoot(t *testing.T) {
	tests := []struct {
		name         string
		cookie       string
		wantLocation string
	}{
		{name: "logged in", cookie: validToken, wantLocation: "/home"},
		{name: "a token with its scheme", cookie: "token " + validToken, wantLocation: "/home"},
		{name: "not logged in", wantLocation: "/login"},
		{name: "an empty token", cookie: "token ", wantLocation: "/login"},
		{name: "a token GitHub rejects", cookie: "gho_revoked", wantLocation: "/login"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router, _ := newTestUI(t)

			resp := serve(router, http.MethodGet, "/", tt.cookie, nil)
			checkRedirect(t, resp, tt.wantLocation)
		})
	}
}

func TestHandleLogin(t *testing.T) {
	router, _ := newTestUI(t)

	resp := serve(router, http.MethodGet, "/login", "", nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusOK)
	}

	if !strings.Contains(resp.Body.String(), "client_id=client-id") {
		t.Errorf("the login page doesn't send the client ID to GitHub:\n%s", resp.Body.String())
	}
}

func TestHandleLoginOauthRedirect(t *testing.T) {
	tests := []struct {
		name         string
		query        string
		wantStatus   int
		wantLocation string
		wantCookie   string
	}{
		{name: "a valid code", query: "?code=valid", wantStatus: http.StatusFound, wantLocation: "/home", wantCookie: validToken},
		{name: "GitHub refused", query: "?error=access_denied&error_description=denied", wantStatus: http.StatusFound, wantLocation: "/login"},
		{name: "a code GitHub doesn't know", query: "?code=unknown", wantStatus: http.StatusInternalServerError},
		{name: "GitHub answers garbage", query: "?code=garbage", wantStatus: http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router, _ := newTestUI(t)

			resp := serve(router, http.MethodGet, "/oauth/redirect"+tt.query, "", nil)
			if resp.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", resp.Code, tt.wantStatus)
			}

			if got := resp.Header().Get("Location"); got != tt.wantLocation {
				t.Errorf("Location = %q, want %q", got, tt.wantLocation)
			}

			var gotCookie string
			for _, cookie := range resp.Result().Cookies() {
				if cookie.Name == loginCookieName {
					gotCookie = cookie.Value
				}
			}

			if gotCookie != tt.wantCookie {
				t.Errorf("login cookie = %q, want %q", gotCookie, tt.wantCookie)
			}
		})
	}
}

func TestExchangeCodeUnreachable(t *testing.T) {
	c := NewUIClient(otel.Tracer("test"), &config.Config{}, nil)
	c.gitHubLoginURL = "http://127.0.0.1:0/login/oauth/access_token"

	if _, err := c.exchangeCode(context.Background(), "valid"); err == nil {
		t.Errorf("exchangeCode() with GitHub unreachable = nil, want an error")
	}

	c.gitHubLoginURL = "://not a URL"
	if _, err := c.exchangeCode(context.Background(), "valid"); err == nil {
		t.Errorf("exchangeCode() with an invalid URL = nil, want an error")
	}
}

func TestGetGhUser(t *testing.T) {
	_, api := newTestUI(t)

	tests := []struct {
		name      string
		apiURL    string
		token     string
		wantLogin string
		wantErr   bool
	}{
		{name: "a valid token", apiURL: api.gitHubAPIURL, token: validToken, wantLogin: "octocat"},
		{name: "a token GitHub rejects", apiURL: api.gitHubAPIURL, token: "gho_revoked", wantErr: true},
		{name: "GitHub answers garbage", apiURL: api.gitHubAPIURL + "/garbage", token: validToken, wantErr: true},
		{name: "GitHub unreachable", apiURL: "http://127.0.0.1:0", token: validToken, wantErr: true},
		{name: "an invalid URL", apiURL: "://not a URL", token: validToken, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewUIClient(otel.Tracer("test"), &config.Config{}, nil)
			c.gitHubAPIURL = tt.apiURL

			user, err := c.getGhUser(context.Background(), tt.token)
			if (err != nil) != tt.wantErr {
				t.Fatalf("getGhUser() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr && user.Login != tt.wantLogin {
				t.Errorf("getGhUser() = %q, want %q", user.Login, tt.wantLogin)
			}
		})
	}
}

func TestHandleHomePage(t *testing.T) {
	tests := []struct {
		name         string
		cookie       string
		failAPI      bool
		wantStatus   int
		wantLocation string
		wantInBody   []string
	}{
		{name: "lists the user's shortlinks, sorted", cookie: validToken, wantStatus: http.StatusOK, wantInBody: []string{"blog", "home", "https://example.com"}},
		{name: "not logged in", wantStatus: http.StatusFound, wantLocation: "/login"},
		{name: "an expired login", cookie: "gho_expired", wantStatus: http.StatusFound, wantLocation: "/login"},
		{name: "the API fails", cookie: validToken, failAPI: true, wantStatus: http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router, api := newTestUI(t)
			api.failAPI = tt.failAPI

			resp := serve(router, http.MethodGet, "/home", tt.cookie, nil)
			if resp.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d:\n%s", resp.Code, tt.wantStatus, resp.Body.String())
			}

			if got := resp.Header().Get("Location"); got != tt.wantLocation {
				t.Errorf("Location = %q, want %q", got, tt.wantLocation)
			}

			body := resp.Body.String()
			for _, want := range tt.wantInBody {
				if !strings.Contains(body, want) {
					t.Errorf("the page doesn't show %q", want)
				}
			}

			if len(tt.wantInBody) > 1 && strings.Index(body, `href="https://s.example.com/blog"`) > strings.Index(body, `href="https://s.example.com/home"`) {
				t.Errorf("the shortlinks aren't sorted by name")
			}
		})
	}
}

func TestHandleNew(t *testing.T) {
	router, _ := newTestUI(t)

	if resp := serve(router, http.MethodGet, "/new", validToken, nil); resp.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.Code, http.StatusOK)
	}
}

func TestHandleNewShortlink(t *testing.T) {
	form := url.Values{
		"name":               {"docs"},
		"url":                {"https://example.org"},
		"co-owners":          {"hubot, , monalisa"},
		"redirectTypeOption": {"html"},
		"redirectAfter":      {"3"},
		"httpStatusCode":     {"308"},
	}

	tests := []struct {
		name         string
		cookie       string
		failAPI      bool
		wantStatus   int
		wantLocation string
		wantCreated  bool
	}{
		{name: "creates the shortlink", cookie: validToken, wantStatus: http.StatusFound, wantLocation: "/home", wantCreated: true},
		{name: "not logged in", wantStatus: http.StatusFound, wantLocation: "/login"},
		{name: "a token GitHub rejects", cookie: "gho_revoked", wantStatus: http.StatusFound, wantLocation: "/login"},
		{name: "the API fails", cookie: validToken, failAPI: true, wantStatus: http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router, api := newTestUI(t)
			api.failAPI = tt.failAPI

			resp := serve(router, http.MethodPost, "/new", tt.cookie, form)
			if resp.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d:\n%s", resp.Code, tt.wantStatus, resp.Body.String())
			}

			if got := resp.Header().Get("Location"); got != tt.wantLocation {
				t.Errorf("Location = %q, want %q", got, tt.wantLocation)
			}

			if !tt.wantCreated {
				return
			}

			want := swagger.V1alpha1ShortLinkSpec{After: 3, Code: 200, Owner: "octocat", Owners: []string{"hubot", "monalisa"}, Target: "https://example.org"}
			if got := api.written["POST docs"]; !equalSpec(got, want) {
				t.Errorf("created %+v, want %+v", got, want)
			}
		})
	}
}

func TestHandleEdit(t *testing.T) {
	tests := []struct {
		name         string
		cookie       string
		query        string
		wantStatus   int
		wantLocation string
	}{
		{name: "the user's shortlink", cookie: validToken, query: "?name=home", wantStatus: http.StatusOK},
		{name: "not logged in", query: "?name=home", wantStatus: http.StatusFound, wantLocation: "/login"},
		{name: "a missing shortlink", cookie: validToken, query: "?name=missing", wantStatus: http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router, _ := newTestUI(t)

			resp := serve(router, http.MethodGet, "/edit"+tt.query, tt.cookie, nil)
			if resp.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d:\n%s", resp.Code, tt.wantStatus, resp.Body.String())
			}

			if got := resp.Header().Get("Location"); got != tt.wantLocation {
				t.Errorf("Location = %q, want %q", got, tt.wantLocation)
			}

			if tt.wantStatus == http.StatusOK && !strings.Contains(resp.Body.String(), "hubot, monalisa") {
				t.Errorf("the form doesn't show the co-owners")
			}
		})
	}
}

func TestHandleEditShortlink(t *testing.T) {
	form := url.Values{
		"name":               {"home"},
		"owner":              {"octocat"},
		"url":                {"https://example.net"},
		"redirectTypeOption": {"http"},
		"httpStatusCode":     {"301"},
	}

	tests := []struct {
		name         string
		cookie       string
		failAPI      bool
		wantStatus   int
		wantLocation string
		wantWritten  bool
	}{
		{name: "updates the shortlink", cookie: validToken, wantStatus: http.StatusFound, wantLocation: "/home", wantWritten: true},
		{name: "not logged in", wantStatus: http.StatusFound, wantLocation: "/login"},
		{name: "the API fails", cookie: validToken, failAPI: true, wantStatus: http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router, api := newTestUI(t)
			api.failAPI = tt.failAPI

			resp := serve(router, http.MethodPost, "/edit", tt.cookie, form)
			if resp.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d:\n%s", resp.Code, tt.wantStatus, resp.Body.String())
			}

			if got := resp.Header().Get("Location"); got != tt.wantLocation {
				t.Errorf("Location = %q, want %q", got, tt.wantLocation)
			}

			if !tt.wantWritten {
				return
			}

			want := swagger.V1alpha1ShortLinkSpec{Code: 301, Owner: "octocat", Target: "https://example.net"}
			if got := api.written["PUT home"]; !equalSpec(got, want) {
				t.Errorf("wrote %+v, want %+v", got, want)
			}
		})
	}
}

func TestHandleDeleteShortlink(t *testing.T) {
	tests := []struct {
		name         string
		cookie       string
		query        string
		wantStatus   int
		wantLocation string
	}{
		{name: "deletes the shortlink", cookie: validToken, query: "?name=home", wantStatus: http.StatusFound, wantLocation: "/home"},
		{name: "not logged in", query: "?name=home", wantStatus: http.StatusFound, wantLocation: "/login"},
		{name: "a missing shortlink", cookie: validToken, query: "?name=missing", wantStatus: http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router, api := newTestUI(t)

			resp := serve(router, http.MethodGet, "/delete"+tt.query, tt.cookie, nil)
			if resp.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d:\n%s", resp.Code, tt.wantStatus, resp.Body.String())
			}

			if got := resp.Header().Get("Location"); got != tt.wantLocation {
				t.Errorf("Location = %q, want %q", got, tt.wantLocation)
			}

			if tt.wantLocation == "/home" && !api.deleted["home"] {
				t.Errorf("the shortlink wasn't deleted")
			}
		})
	}
}

func TestHandleNotFound(t *testing.T) {
	router, _ := newTestUI(t)

	if resp := serve(router, http.MethodGet, "/no/such/page", "", nil); resp.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", resp.Code, http.StatusNotFound)
	}
}

// TestStartSpan checks that a handler ends the span it started, and leaves
// the request's own span to otelgin.
func TestStartSpan(t *testing.T) {
	provider := sdktrace.NewTracerProvider()
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })

	c := NewUIClient(provider.Tracer("test"), &config.Config{}, nil)

	gin.SetMode(gin.TestMode)
	ct, _ := gin.CreateTestContext(httptest.NewRecorder())

	requestCtx, requestSpan := provider.Tracer("test").Start(context.Background(), "request")
	ct.Request = httptest.NewRequestWithContext(requestCtx, http.MethodGet, "/", http.NoBody)

	_, span := c.startSpan(ct, "handler")
	span.End()

	if !requestSpan.IsRecording() {
		t.Errorf("startSpan() handed out the request's span, and its End ended it")
	}

	requestSpan.End()

	ct.Request = httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", http.NoBody)

	_, span = c.startSpan(ct, "handler")
	if !span.IsRecording() {
		t.Fatalf("startSpan() without a request span didn't start one")
	}

	span.End()

	if span.IsRecording() {
		t.Errorf("the span startSpan() started didn't end")
	}
}

func TestHTTPStatusText(t *testing.T) {
	if got := HTTPStatusText(http.StatusNotFound); got != "404 Not Found" {
		t.Errorf("HTTPStatusText(404) = %q", got)
	}
}

// fakeAPI is the urlshortener API and GitHub, as the UI sees them.
type fakeAPI struct {
	written      map[string]swagger.V1alpha1ShortLinkSpec
	deleted      map[string]bool
	gitHubAPIURL string
	failAPI      bool
}

// newTestUI returns a router serving the UI's pages against a fake API and a
// fake GitHub.
func newTestUI(t *testing.T) (*gin.Engine, *fakeAPI) {
	t.Helper()

	api := &fakeAPI{
		written: map[string]swagger.V1alpha1ShortLinkSpec{},
		deleted: map[string]bool{},
	}

	server := httptest.NewServer(http.HandlerFunc(api.serveHTTP))
	t.Cleanup(server.Close)

	api.gitHubAPIURL = server.URL + "/github"

	apiClient := swagger.NewAPIClient(&swagger.Configuration{
		BasePath:   server.URL,
		HTTPClient: server.Client(),
	})

	c := NewUIClient(otel.Tracer("test"), &config.Config{
		ClientID:     "client-id",
		ClientSecret: "client-secret",
		RedirectURL:  "https://dash.example.com/oauth/redirect",
		DashboardURL: "dash.example.com",
		ShortlinkURL: "https://s.example.com",
	}, apiClient)
	c.gitHubAPIURL = api.gitHubAPIURL
	c.gitHubLoginURL = server.URL + "/github/login/oauth/access_token"

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.LoadHTMLGlob("../../html/templates/*.html")

	router.NoRoute(c.HandleNotFound)
	router.GET("/", c.HandleRoot)
	router.GET("/login", c.HandleLogin)
	router.GET("/oauth/redirect", c.HandleLoginOauthRedirect)
	router.GET("/home", c.HandleHomePage)
	router.GET("/new", c.HandleNew)
	router.POST("/new", c.HandleNewShortlink)
	router.GET("/edit", c.HandleEdit)
	router.POST("/edit", c.HandleEditShortlink)
	router.GET("/delete", c.HandleDeleteShortlink)

	return router, api
}

func (a *fakeAPI) serveHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/github/user":
		if r.Header.Get("Authorization") != "token "+validToken {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{"login": "octocat", "id": 1})

	case r.URL.Path == "/github/garbage/user":
		_, _ = io.WriteString(w, "not json")

	case r.URL.Path == "/github/login/oauth/access_token":
		a.serveToken(w, r)

	case strings.HasPrefix(r.URL.Path, "/api/v1/shortlink/"):
		a.serveShortlinks(w, r)

	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (a *fakeAPI) serveToken(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil || r.PostForm.Get("client_secret") != "client-secret" || r.URL.Query().Has("client_secret") {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	switch r.PostForm.Get("code") {
	case "valid":
		writeJSON(w, http.StatusOK, map[string]string{"access_token": validToken})
	case "garbage":
		_, _ = io.WriteString(w, "not json")
	default:
		writeJSON(w, http.StatusOK, map[string]string{"error": "bad_verification_code"})
	}
}

func (a *fakeAPI) serveShortlinks(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Header.Get("Authorization") != "Bearer "+validToken:
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "bad credentials", "advice": []string{"log in"}})
		return
	case a.failAPI:
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "the cluster is down", "advice": []string{"wait"}})
		return
	}

	name := strings.TrimPrefix(r.URL.Path, "/api/v1/shortlink/")
	home := swagger.ControllerShortLink{
		Name:   "home",
		Spec:   &swagger.V1alpha1ShortLinkSpec{Owner: "octocat", Owners: []string{"hubot", "monalisa"}, Target: "https://example.com", Code: 307},
		Status: &swagger.V1alpha1ShortLinkStatus{Count: 42},
	}

	switch {
	case name == "" && r.Method == http.MethodGet:
		blog := home
		blog.Name = "blog"
		writeJSON(w, http.StatusOK, []swagger.ControllerShortLink{home, blog})

	case name != "home" && r.Method != http.MethodPost:
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found", "advice": []string{"check the name"}})

	case r.Method == http.MethodGet:
		writeJSON(w, http.StatusOK, home)

	case r.Method == http.MethodDelete:
		a.deleted[name] = true
		w.WriteHeader(http.StatusOK)

	default:
		var spec swagger.V1alpha1ShortLinkSpec
		if err := json.NewDecoder(r.Body).Decode(&spec); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		a.written[r.Method+" "+name] = spec
		writeJSON(w, http.StatusOK, swagger.ControllerShortLink{Name: name, Spec: &spec})
	}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func serve(router http.Handler, method, path, cookie string, form url.Values) *httptest.ResponseRecorder {
	body := io.Reader(http.NoBody)
	if form != nil {
		body = strings.NewReader(form.Encode())
	}

	req := httptest.NewRequestWithContext(context.Background(), method, path, body)
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}

	if cookie != "" {
		req.AddCookie(&http.Cookie{Name: loginCookieName, Value: cookie})
	}

	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	return resp
}

func checkRedirect(t *testing.T, resp *httptest.ResponseRecorder, location string) {
	t.Helper()

	if resp.Code != http.StatusFound || resp.Header().Get("Location") != location {
		t.Errorf("got %d to %q, want a redirect to %q", resp.Code, resp.Header().Get("Location"), location)
	}
}

func equalSpec(a, b swagger.V1alpha1ShortLinkSpec) bool {
	return a.After == b.After && a.Code == b.Code && a.Owner == b.Owner && a.Target == b.Target &&
		strings.Join(a.Owners, ",") == strings.Join(b.Owners, ",")
}
