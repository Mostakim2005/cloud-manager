package server

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Mostakim2005/cloud-manager/internal/config"
	"github.com/Mostakim2005/cloud-manager/internal/db"
	"github.com/Mostakim2005/cloud-manager/internal/tray"
)

// ProbeRunningDaemon returns true if an active Cloud Manager daemon is already serving bindAddr.
func ProbeRunningDaemon(bindAddr string) bool {
	client := &http.Client{Timeout: 450 * time.Millisecond}
	resp, err := client.Get("http://" + bindAddr + "/api/v1/health")
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// RunForegroundDaemon starts the authenticated loopback daemon, optional system tray,
// and handles single-instance wake when already running.
func RunForegroundDaemon(bindAddr string, openUI, noTray bool, uiDir string) int {
	if ProbeRunningDaemon(bindAddr) {
		if openUI {
			_ = tray.OpenBrowser("http://" + bindAddr)
		}
		return 0
	}

	paths := config.DiscoverPaths("")
	settings, err := config.LoadOrInitSettings(paths)
	if err != nil {
		fmt.Fprintf(os.Stderr, "settings init error: %v\n", err)
		return 1
	}
	secrets, err := config.LoadOrInitSecrets(paths.SecretsPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "secrets init error: %v\n", err)
		return 1
	}
	store, err := db.Open(paths.DatabasePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sqlite open error: %v\n", err)
		return 1
	}
	logger, _ := config.NewLogger(paths.LogsDir)

	daemon, err := NewDaemon(paths, settings, secrets, store, logger)
	if err != nil {
		fmt.Fprintf(os.Stderr, "daemon init error: %v\n", err)
		_ = store.Close()
		return 1
	}
	daemon.UIOverride = uiDir

	quitCh := make(chan struct{}, 1)
	daemon.OnShutdownRequest = func() {
		select {
		case quitCh <- struct{}{}:
		default:
		}
	}

	url, err := daemon.Start(bindAddr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "daemon bind error: %v\n", err)
		_ = daemon.Shutdown(context.Background())
		return 1
	}

	fmt.Fprintf(os.Stderr, "Cloud Manager daemon listening on %s\n", url)
	if openUI {
		_ = tray.OpenBrowser(url)
	}

	var trayCtrl *tray.Controller
	if !noTray {
		trayCtrl = tray.Start(tray.Callbacks{
			DashboardURL: url,
			OnToggleLAN: func() {
				if daemon.LANQR.Status().Enabled {
					_ = daemon.LANQR.Disable(context.Background())
				} else {
					_, _ = daemon.LANQR.Enable()
				}
			},
			OnQuit: func() {
				select {
				case quitCh <- struct{}{}:
				default:
				}
			},
		})
	}

	sigCh := make(chan os.Signal, 2)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	select {
	case <-sigCh:
	case <-quitCh:
	}

	if trayCtrl != nil {
		trayCtrl.Stop()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = daemon.Shutdown(ctx)
	return 0
}
