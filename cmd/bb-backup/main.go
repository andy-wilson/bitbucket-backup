// Package main provides the CLI entrypoint for bb-backup.
package main

import (
	"os"

	"github.com/andy-wilson/bb-backup/cmd/bb-backup/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
