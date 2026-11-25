//go:build !windows

package tray

// Callbacks wires tray menu actions to daemon controls.
type Callbacks struct {
	DashboardURL string
	OnToggleLAN  func()
	OnQuit       func()
}

// Controller is a non-Windows no-op tray stub so cross-platform tests build cleanly.
type Controller struct{}

// Start initializes a no-op tray controller on non-Windows hosts.
func Start(_ Callbacks) *Controller { return &Controller{} }

// Stop is a no-op on non-Windows hosts.
func (c *Controller) Stop() {}

// OpenBrowser is a no-op on headless non-Windows test hosts.
func OpenBrowser(_ string) error { return nil }
