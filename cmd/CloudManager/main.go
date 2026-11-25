package main

import (
	"fmt"
	"os"

	"github.com/Mostakim2005/cloud-manager/internal/cli"
	"github.com/Mostakim2005/cloud-manager/internal/config"
	"github.com/Mostakim2005/cloud-manager/internal/server"
)

// CloudManager.exe serves as the windowless Windows GUI tray & dashboard launcher:
// - No args: starts daemon + tray (if offline) and opens http://127.0.0.1:59432 (or wakes existing instance).
// - --background / --tray: starts silently in System Tray with 0% idle CPU and no browser popup.
// - Any CLI verb: delegates cleanly to headless CLI logic.
func main() {
	openUI := true
	if len(os.Args) > 1 {
		arg := os.Args[1]
		if arg == "--background" || arg == "--tray" {
			openUI = false
		} else if arg != "--daemon" {
			os.Exit(cli.Run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
		}
	}
	addr := fmt.Sprintf("%s:%d", config.DefaultBindHost, config.DefaultPort)
	os.Exit(server.RunForegroundDaemon(addr, openUI, false, ""))
}
