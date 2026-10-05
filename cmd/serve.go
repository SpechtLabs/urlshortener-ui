package cmd

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sierrasoftworks/humane-errors-go"
	"github.com/spechtlabs/go-otel-utils/otelprovider"
	"github.com/spechtlabs/go-otel-utils/otelzap"
	"github.com/spf13/cobra"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/log"
	"go.uber.org/zap"

	"github.com/spechtlabs/urlshortener-ui/pkg/client"
	"github.com/spechtlabs/urlshortener-ui/pkg/config"
	"github.com/spechtlabs/urlshortener-ui/pkg/router"
	"github.com/spechtlabs/urlshortener-ui/pkg/swagger"
)

// apiTimeout bounds a request to the urlshortener API.
const apiTimeout = 30 * time.Second

func newServeCommand() *cobra.Command {
	var bindAddress string
	var debug bool

	serveCmd := &cobra.Command{
		Use:     "serve",
		Short:   "Serve the web UI",
		Example: "urlshortener-ui serve --bind-address :8080",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			return serve(ctx, *config.NewConfigFromEnv(), bindAddress, debug)
		},
	}

	serveCmd.Flags().StringVarP(&bindAddress, "bind-address", "p", ":8080", "Bind Address")
	serveCmd.Flags().BoolVarP(&debug, "debug", "d", false, "Enable Debug Mode")

	return serveCmd
}

// serve sets up logging and tracing, then serves the UI on bindAddress until
// ctx is done.
func serve(ctx context.Context, conf config.Config, bindAddress string, debug bool) humane.Error {
	logProvider := otelprovider.NewLogger(otelprovider.WithLogAutomaticEnv())
	traceProvider := otelprovider.NewTracer(otelprovider.WithTraceAutomaticEnv())

	undoLogging, err := setupLogging(debug, logProvider)
	if err != nil {
		return err
	}

	// Flush and stop the providers once the server has stopped. A failure
	// only loses telemetry, so it's logged rather than returned.
	defer func() {
		flushCtx := context.WithoutCancel(ctx)
		if err := errors.Join(
			traceProvider.ForceFlush(flushCtx),
			logProvider.ForceFlush(flushCtx),
			traceProvider.Shutdown(flushCtx),
			logProvider.Shutdown(flushCtx),
		); err != nil {
			otelzap.L().WithError(err).WarnContext(flushCtx, "failed to flush and shut down the telemetry providers",
				zap.String("component", "telemetry"),
			)
		}

		undoLogging()
	}()

	apiClient := swagger.NewAPIClient(&swagger.Configuration{
		BasePath:  conf.ShortlinkURL,
		UserAgent: serviceName,
		HTTPClient: &http.Client{
			Transport: otelhttp.NewTransport(http.DefaultTransport),
			Timeout:   apiTimeout,
		},
	})

	uiClient := client.NewUIClient(otel.Tracer(serviceName), &conf, apiClient)

	return router.NewServer(bindAddress, serviceName, uiClient).Run(ctx)
}

// setupLogging replaces the global zap and otelzap loggers and redirects the
// standard library's log to zap. It returns a function that undoes all of it.
func setupLogging(debug bool, logProvider log.LoggerProvider) (func(), humane.Error) {
	var zapLogger *zap.Logger
	var err error
	if debug {
		zapLogger, err = zap.NewDevelopment()
		gin.SetMode(gin.DebugMode)
	} else {
		zapLogger, err = zap.NewProduction()
		gin.SetMode(gin.ReleaseMode)
	}

	if err != nil {
		return nil, humane.Wrap(err, "Failed to initialize the logger", "This is a bug in the logger configuration; please report it")
	}

	undoZapGlobals := zap.ReplaceGlobals(zapLogger)
	undoStdLogRedirect := zap.RedirectStdLog(zapLogger)

	undoOtelZapGlobals := otelzap.ReplaceGlobals(otelzap.New(zapLogger,
		otelzap.WithCaller(true),
		otelzap.WithMinLevel(zap.InfoLevel),
		otelzap.WithAnnotateLevel(zap.WarnLevel),
		otelzap.WithErrorStatusLevel(zap.ErrorLevel),
		otelzap.WithStackTrace(false),
		otelzap.WithLoggerProvider(logProvider),
	))

	return func() {
		undoStdLogRedirect()
		undoOtelZapGlobals()
		undoZapGlobals()
	}, nil
}
