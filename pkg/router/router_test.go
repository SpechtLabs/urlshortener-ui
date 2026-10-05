package router

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"go.opentelemetry.io/otel"

	"github.com/spechtlabs/urlshortener-ui/pkg/client"
	"github.com/spechtlabs/urlshortener-ui/pkg/config"
)

// TestServer runs the server, checks that it serves the pages Load
// registers, and stops it by canceling its context.
func TestServer(t *testing.T) {
	// The server loads its templates from html/ in the working directory.
	t.Chdir("../..")

	addr := freeAddr(t)
	uiClient := client.NewUIClient(otel.Tracer("test"), &config.Config{ClientID: "client-id"}, nil)
	srv := NewServer(addr, "urlshortener-ui-test", uiClient)

	ctx, cancel := context.WithCancel(context.Background())
	stopped := make(chan error, 1)
	go func() {
		if err := srv.Run(ctx); err != nil {
			stopped <- err
			return
		}

		stopped <- nil
	}()

	for path, want := range map[string]int{
		"/login":              http.StatusOK,
		"/":                   http.StatusFound,
		"/assets/css/404.css": http.StatusOK,
		"/no/such/page":       http.StatusNotFound,
	} {
		if status := getStatus(t, "http://"+addr+path); status != want {
			t.Errorf("GET %s = %d, want %d", path, status, want)
		}
	}

	cancel()

	select {
	case err := <-stopped:
		if err != nil {
			t.Errorf("Run() after cancel = %v, want nil", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Run() didn't return after its context was canceled")
	}

	// A second server can't bind the address while one listens there.
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()

	if err := NewServer(addr, "urlshortener-ui-test", uiClient).Run(context.Background()); err == nil {
		t.Errorf("Run() on a busy address = nil, want an error")
	}
}

// freeAddr returns a local address no one listens on.
func freeAddr(t *testing.T) string {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	addr := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}

	return addr
}

// getStatus GETs url until the server answers, and returns the status
// without following a redirect.
func getStatus(t *testing.T, url string) int {
	t.Helper()

	// Without keep-alives, no connection outlives its request, so none holds
	// up the server's graceful shutdown.
	httpClient := &http.Client{
		Transport: &http.Transport{DisableKeepAlives: true},
		Timeout:   5 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	deadline := time.Now().Add(10 * time.Second)
	for {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, http.NoBody)
		if err != nil {
			t.Fatal(err)
		}

		resp, err := httpClient.Do(req)
		if err == nil {
			_ = resp.Body.Close()
			return resp.StatusCode
		}

		if time.Now().After(deadline) {
			t.Fatalf("GET %s: %v", url, err)
		}

		time.Sleep(50 * time.Millisecond)
	}
}
