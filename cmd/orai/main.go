// Command orai runs Codex and Claude Code as role sessions with exact resume,
// notifications, a project wiki and diagnostics.
package main

import (
	"os"

	"github.com/oXpace/orai/internal/cli"
)

func main() {
	os.Exit(cli.Main(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
