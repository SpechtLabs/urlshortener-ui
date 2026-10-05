package cmd

import (
	"github.com/spf13/cobra"
)

// serviceName names the UI in traces, logs and metrics.
const serviceName = "urlshortener-ui"

// NewRootCommand returns the urlshortener-ui command line.
func NewRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   serviceName,
		Short: "The web UI for urlshortener's shortlinks",
		// main prints the error, with its advice.
		SilenceErrors: true,
		SilenceUsage:  true,
	}

	root.AddCommand(newServeCommand())

	return root
}
