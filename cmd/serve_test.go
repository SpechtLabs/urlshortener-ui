package cmd

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel/log/noop"

	"github.com/spechtlabs/urlshortener-ui/pkg/config"
)

func TestNewRootCommand(t *testing.T) {
	root := NewRootCommand()

	serveCmd, _, err := root.Find([]string{"serve"})
	if err != nil || serveCmd.Name() != "serve" {
		t.Fatalf("root has no serve command: %v", err)
	}

	for flag, want := range map[string]string{"bind-address": ":8080", "debug": "false"} {
		if got := serveCmd.Flags().Lookup(flag); got == nil || got.DefValue != want {
			t.Errorf("serve --%s defaults to %v, want %q", flag, got, want)
		}
	}
}

func TestServe(t *testing.T) {
	// The server loads its templates from html/ in the working directory.
	t.Chdir("..")

	// A canceled context makes serve set everything up and shut the server
	// down again right away.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := serve(ctx, config.Config{ShortlinkURL: "http://127.0.0.1:0"}, "127.0.0.1:0", true); err != nil {
		t.Errorf("serve() = %v, want nil", err)
	}

	// The serve command does the same from its flags.
	root := NewRootCommand()
	root.SetArgs([]string{"serve", "--bind-address", "127.0.0.1:0"})

	if err := root.ExecuteContext(ctx); err != nil {
		t.Errorf("serve command = %v, want nil", err)
	}
}

func TestSetupLogging(t *testing.T) {
	for _, debug := range []bool{true, false} {
		undo, err := setupLogging(debug, noop.NewLoggerProvider())
		if err != nil {
			t.Fatalf("setupLogging(%v) error = %v", debug, err)
		}

		undo()
	}
}
