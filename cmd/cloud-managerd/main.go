package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/Mostakim2005/cloud-manager/internal/config"
	"github.com/Mostakim2005/cloud-manager/internal/server"
)

func main() {
	bindAddr := flag.String("bind", fmt.Sprintf("%s:%d", config.DefaultBindHost, config.DefaultPort), "Loopback listen address")
	openUI := flag.Bool("open", false, "Open dashboard in default browser on startup")
	background := flag.Bool("background", false, "Run silently in background tray without opening browser")
	noTray := flag.Bool("no-tray", false, "Disable Windows system tray icon")
	uiDir := flag.String("ui-dir", "", "Optional static UI override directory for development")
	flag.Parse()

	shouldOpen := *openUI && !*background
	os.Exit(server.RunForegroundDaemon(*bindAddr, shouldOpen, *noTray, *uiDir))
}
