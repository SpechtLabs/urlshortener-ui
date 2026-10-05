package main

import (
	"os"

	"github.com/sierrasoftworks/humane-errors-go"

	"github.com/spechtlabs/urlshortener-ui/cmd"
)

func main() {
	if err := cmd.NewRootCommand().Execute(); err != nil {
		humane.Eprint(err)
		os.Exit(1)
	}
}
