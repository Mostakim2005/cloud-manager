package adb

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/Mostakim2005/cloud-manager/internal/db"
	"github.com/Mostakim2005/cloud-manager/internal/qrsvg"
	"github.com/Mostakim2005/cloud-manager/internal/security"
)

// Device describes a USB or Wireless ADB device.
type Device struct {
	Serial string `json:"serial"`
	State  string `json:"state"`
	Model  string `json:"model,omitempty"`
}

// MediaItem describes an Android MediaStore entry normalized to UTC seconds.
type MediaItem struct {
	ID           string `json:"id"`
	MediaType    string `json:"mediaType"` // image | video
	RelativePath string `json:"relativePath"`
	DisplayName  string `json:"displayName"`
	SizeBytes    int64  `json:"sizeBytes"`
	ModifiedUTC  int64  `json:"modifiedUtc"`
	NeedsPull    bool   `json:"needsPull"`
}

// Runner abstracts argv-only subprocess execution for unit testability.
type Runner interface {
	Run(ctx context.Context, args ...string) (string, error)
}

type execRunner struct {
	bin string
}

func (e execRunner) Run(ctx context.Context, args ...string) (string, error) {
	bin := e.bin
	if bin == "" {
		found, err := exec.LookPath("adb")
		if err != nil {
			return "", fmt.Errorf("adb binary not found: %w", err)
		}
		bin = found
	}
	cmd := exec.CommandContext(ctx, bin, args...) // Argv-only execution, never shell
	out, err := cmd.CombinedOutput()
	return SanitizeOutput(string(out)), err
}

// Client manages USB & Wireless ADB QR pairing, MediaStore queries, and delta pulls.
type Client struct {
	runner Runner
	Store  *db.Store
}

// NewClient constructs an ADB adapter.
func NewClient(binaryPath string, store *db.Store) *Client {
	return &Client{runner: execRunner{bin: binaryPath}, Store: store}
}

// NewClientWithRunner injects a mock Runner for testing.
func NewClientWithRunner(r Runner, store *db.Store) *Client {
	return &Client{runner: r, Store: store}
}

// SanitizeOutput strips Windows \r\r\n / \r carriage returns from ADB stdout.
func SanitizeOutput(raw string) string {
	s := strings.ReplaceAll(raw, "\r\n", "\n")
	return strings.ReplaceAll(s, "\r", "")
}

// WirelessPairingQR builds an Android 11+ ADB QR pairing payload and renders it as SVG.
func WirelessPairingQR(serviceName, password string) (string, string, error) {
	if serviceName == "" || password == "" {
		return "", "", fmt.Errorf("service name and password required")
	}
	payload := fmt.Sprintf("WIFI:T:ADB;S:%s;P:%s;;", serviceName, password)
	svg, err := qrsvg.RenderSVG(payload, 200)
	return payload, svg, err
}

// Pair executes `adb pair <host:port> <code>` safely via argv.
func (c *Client) Pair(ctx context.Context, addr, code string) (string, error) {
	if addr == "" || code == "" {
		return "", fmt.Errorf("pair addr and code required")
	}
	return c.runner.Run(ctx, "pair", addr, code)
}

// Connect executes `adb connect <host:port>` safely via argv.
func (c *Client) Connect(ctx context.Context, addr string) (string, error) {
	if addr == "" {
		return "", fmt.Errorf("connect addr required")
	}
	return c.runner.Run(ctx, "connect", addr)
}

// ListDevices parses `adb devices -l` into structured devices.
func (c *Client) ListDevices(ctx context.Context) ([]Device, error) {
	out, err := c.runner.Run(ctx, "devices", "-l")
	if err != nil {
		return nil, err
	}
	return ParseDevices(out), nil
}

// ParseDevices parses sanitized `adb devices -l` lines.
func ParseDevices(out string) []Device {
	var devices []Device
	for _, line := range strings.Split(SanitizeOutput(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "List of devices") || strings.HasPrefix(line, "*") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		dev := Device{Serial: fields[0], State: fields[1]}
		for _, f := range fields[2:] {
			if strings.HasPrefix(f, "model:") {
				dev.Model = strings.TrimPrefix(f, "model:")
			}
		}
		devices = append(devices, dev)
	}
	return devices
}

// QueryMediaDelta queries both images/media and video/media with relative_path and flags unsynced items.
func (c *Client) QueryMediaDelta(ctx context.Context, serial string) ([]MediaItem, error) {
	var all []MediaItem
	queries := []struct {
		kind string
		uri  string
	}{
		{kind: "image", uri: "content://media/external/images/media"},
		{kind: "video", uri: "content://media/external/video/media"},
	}
	for _, q := range queries {
		args := []string{}
		if serial != "" {
			args = append(args, "-s", serial)
		}
		args = append(args, "shell", "content", "query",
			"--uri", q.uri,
			"--projection", "_id:relative_path:_display_name:_size:date_modified:datetaken")
		out, err := c.runner.Run(ctx, args...)
		if err != nil {
			return nil, fmt.Errorf("query %s: %w", q.kind, err)
		}
		items := ParseMediaStoreOutput(out, q.kind)
		for i := range items {
			rel := strings.TrimRight(items[i].RelativePath, "/") + "/" + items[i].DisplayName
			if _, serr := security.SanitizeRelPath(strings.TrimLeft(rel, "/")); serr != nil {
				continue
			}
			items[i].NeedsPull = c.needsPull(items[i])
			all = append(all, items[i])
		}
	}
	return all, nil
}

func (c *Client) needsPull(item MediaItem) bool {
	if c.Store == nil {
		return true
	}
	matches, _, err := c.Store.SearchCatalog(item.DisplayName, "", "", 20, 0)
	if err != nil {
		return true
	}
	for _, m := range matches {
		if m.Name == item.DisplayName && m.SizeBytes == item.SizeBytes {
			return false
		}
	}
	return true
}

var kvRegex = regexp.MustCompile(`([a-zA-Z0-9_]+)=([^,\n]*)`)

// ParseMediaStoreOutput parses `adb shell content query` rows and normalizes timestamps to UTC seconds.
func ParseMediaStoreOutput(raw string, mediaType string) []MediaItem {
	clean := SanitizeOutput(raw)
	var items []MediaItem
	for _, line := range strings.Split(clean, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "Row:") {
			continue
		}
		vals := make(map[string]string)
		for _, m := range kvRegex.FindAllStringSubmatch(line, -1) {
			vals[m[1]] = strings.TrimSpace(m[2])
		}
		name := vals["_display_name"]
		if name == "" || vals["_size"] == "" || vals["_size"] == "NULL" {
			continue
		}
		sz, err := strconv.ParseInt(vals["_size"], 10, 64)
		if err != nil || sz <= 0 {
			continue
		}
		// Normalize date_modified (seconds) vs datetaken (milliseconds) strictly to UTC seconds.
		var modSec int64
		if dm, err := strconv.ParseInt(vals["date_modified"], 10, 64); err == nil && dm > 0 {
			modSec = dm
		} else if dt, err := strconv.ParseInt(vals["datetaken"], 10, 64); err == nil && dt > 0 {
			if dt > 1_000_000_000_000 { // milliseconds
				modSec = dt / 1000
			} else {
				modSec = dt
			}
		}
		items = append(items, MediaItem{
			ID:           vals["_id"],
			MediaType:    mediaType,
			RelativePath: strings.ReplaceAll(vals["relative_path"], `\`, "/"),
			DisplayName:  filepath.Base(name),
			SizeBytes:    sz,
			ModifiedUTC:  modSec,
		})
	}
	return items
}

// PullFile executes `adb pull` safely into a confined local destination path.
func (c *Client) PullFile(ctx context.Context, serial, remotePath, localRoot, relDest string) (string, error) {
	if _, err := security.SanitizeRelPath(strings.TrimLeft(remotePath, "/")); err != nil {
		return "", err
	}
	dest, err := security.ResolveConfinedPath(localRoot, relDest)
	if err != nil {
		return "", err
	}
	args := []string{}
	if serial != "" {
		args = append(args, "-s", serial)
	}
	args = append(args, "pull", "/sdcard/"+strings.TrimLeft(remotePath, "/"), dest)
	if _, err := c.runner.Run(ctx, args...); err != nil {
		return "", err
	}
	return dest, nil
}
