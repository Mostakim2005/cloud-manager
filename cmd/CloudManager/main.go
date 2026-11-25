package main

import (
	"fmt"
	"os"

	"github.com/Mostakim2005/cloud-manager/internal/cli"
	"github.com/Mostakim2005/cloud-manager/internal/config"
	"github.com/Mostakim2005/cloud-manager/internal/server"
)

// CloudManager.exe serves as the single portable Windows entrypoint:
// - With CLI verbs (health, doctor, route, sync, open-uri, ...): executes headless --json CLI.
// - Launched directly (no subcommand or --daemon): starts daemon + tray + opens browser UI.
func main() {
	if len(os.Args) > 1 && os.Args[1] != "--daemon" {
		os.Exit(cli.Run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
	}
	addr := fmt.Sprintf("%s:%d", config.DefaultBindHost, config.DefaultPort)
	os.Exit(server.RunForegroundDaemon(addr, true, false, ""))
}
