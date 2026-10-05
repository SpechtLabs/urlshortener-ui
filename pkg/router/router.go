// Package router wires the UI's handlers into its HTTP server.
package router

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sierrasoftworks/humane-errors-go"
	ginprometheus "github.com/spechtlabs/go-gin-prometheus"
	"github.com/spechtlabs/go-otel-utils/otelzap"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"go.uber.org/zap"

	"github.com/spechtlabs/urlshortener-ui/pkg/client"
)

const (
	// readHeaderTimeout bounds how long a client may take to send its request
	// headers, so slow clients can't hold connections open (Slowloris).
	readHeaderTimeout = 10 * time.Second

	// shutdownTimeout is how long requests in flight get to finish on shutdown.
	shutdownTimeout = 5 * time.Second
)

// Server serves the UI.
type Server struct {
	srv *http.Server
}

// NewServer returns the UI's server, which serves uiClient's pages on
// bindAddr once it runs. It reads the templates and assets from html/ in the
// working directory.
func NewServer(bindAddr, serviceName string, uiClient *client.UIClient) *Server {
	router := gin.New()
	router.Use(
		gin.Recovery(),
		otelgin.Middleware(serviceName),
		ginprometheus.GinPrometheusMiddleware(router, "gin", ginprometheus.WithLowCardinalityUrl()),
	)

	router.LoadHTMLGlob("html/templates/*.html")
	router.Static("assets", "./html/assets")

	Load(router, uiClient)

	return &Server{
		srv: &http.Server{
			Addr:              bindAddr,
			Handler:           router,
			ReadHeaderTimeout: readHeaderTimeout,
		},
	}
}

// Load registers uiClient's pages with router.
func Load(router *gin.Engine, uiClient *client.UIClient) {
	router.NoRoute(uiClient.HandleNotFound) // 404 page

	router.GET("/", uiClient.HandleRoot)

	router.GET("/login", uiClient.HandleLogin)
	router.GET("/oauth/redirect", uiClient.HandleLoginOauthRedirect)

	router.GET("/home", uiClient.HandleHomePage)

	router.GET("/new", uiClient.HandleNew)
	router.POST("/new", uiClient.HandleNewShortlink)

	router.GET("/edit", uiClient.HandleEdit)
	router.POST("/edit", uiClient.HandleEditShortlink)

	router.GET("/delete", uiClient.HandleDeleteShortlink)
}

// Run serves until ctx is done, then shuts the server down, giving requests
// in flight shutdownTimeout to finish.
func (s *Server) Run(ctx context.Context) humane.Error {
	// The server goroutine ends when ListenAndServe fails or when Shutdown below
	// stops it; Run waits for it either way.
	var serving sync.WaitGroup
	defer serving.Wait()

	otelzap.L().InfoContext(ctx, "serving the UI", zap.String("address", s.srv.Addr))

	serveErr := make(chan error, 1)
	serving.Go(func() {
		serveErr <- s.srv.ListenAndServe()
	})

	select {
	case err := <-serveErr:
		return humane.Wrap(err, fmt.Sprintf("Unable to serve on %s", s.srv.Addr),
			"Make sure no other process listens on the --bind-address and try again.",
		)

	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
	defer cancel()

	if err := s.srv.Shutdown(shutdownCtx); err != nil {
		return humane.Wrap(err, "Unable to shut the server down gracefully",
			fmt.Sprintf("Requests still in flight after %s were cut off.", shutdownTimeout),
		)
	}

	return nil
}
