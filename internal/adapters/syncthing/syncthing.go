package syncthing

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Mostakim2005/cloud-manager/internal/adapters"
	"github.com/Mostakim2005/cloud-manager/internal/qrsvg"
	"github.com/Mostakim2005/cloud-manager/internal/security"
)

// FolderStatus represents Syncthing REST /rest/db/status summary.
type FolderStatus struct {
	FolderID    string `json:"folderId"`
	State       string `json:"state"`
	GlobalFiles int64  `json:"globalFiles"`
	GlobalBytes int64  `json:"globalBytes"`
	LocalFiles  int64  `json:"localFiles"`
	NeedFiles   int64  `json:"needFiles"`
	NeedBytes   int64  `json:"needBytes"`
	Errors      int64  `json:"errors"`
}

// Client supervises unmodified upstream Syncthing over its LAN-Only local REST API.
type Client struct {
	mu         sync.Mutex
	BinaryPath string
	HomeDir    string
	BaseURL    string
	APIKey     string
	DeviceID   string
	JobSup     *security.JobSupervisor
	Breaker    *adapters.CircuitBreaker
	HTTPClient *http.Client
	cmd        *exec.Cmd
}

// NewClient creates a Syncthing LAN-Only REST adapter.
func NewClient(binaryPath, homeDir string, jobSup *security.JobSupervisor) *Client {
	return &Client{
		BinaryPath: binaryPath,
		HomeDir:    homeDir,
		JobSup:     jobSup,
		Breaker:    adapters.NewCircuitBreaker(3, 20*time.Second),
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// Start launches unmodified syncthing bound strictly to loopback GUI/API and enforces LAN-Only options.
func (c *Client) Start(ctx context.Context) error {
	bin := c.BinaryPath
	if bin == "" {
		found, err := exec.LookPath("syncthing")
		if err != nil {
			return fmt.Errorf("syncthing binary not found: %w", err)
		}
		bin = found
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	key, err := security.GenerateToken()
	if err != nil {
		return err
	}

	args := []string{
		"serve",
		"--no-browser",
		"--no-restart",
		"--gui-address=http://" + addr,
		"--gui-apikey=" + key,
	}
	if c.HomeDir != "" {
		args = append(args, "--home="+c.HomeDir)
	}
	cmd := exec.Command(bin, args...)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start syncthing: %w", err)
	}
	if c.JobSup != nil {
		_ = c.JobSup.AssignPID(cmd.Process.Pid)
	}

	c.mu.Lock()
	c.BaseURL = "http://" + addr
	c.APIKey = key
	c.cmd = cmd
	c.mu.Unlock()

	return c.EnforceLANOnly(ctx)
}

// Stop triggers graceful /rest/system/shutdown.
func (c *Client) Stop(ctx context.Context) error {
	c.mu.Lock()
	cmd := c.cmd
	baseURL := c.BaseURL
	c.cmd = nil
	c.mu.Unlock()

	if baseURL != "" {
		_, _ = c.doJSON(ctx, http.MethodPost, "/rest/system/shutdown", nil)
	}
	if cmd != nil && cmd.Process != nil {
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		select {
		case <-time.After(3 * time.Second):
			_ = cmd.Process.Kill()
		case <-done:
		}
	}
	return nil
}

// EnforceLANOnly disables Global Discovery, Relays, and NAT traversal via /rest/config/options.
func (c *Client) EnforceLANOnly(ctx context.Context) error {
	payload := map[string]any{
		"globalAnnounceEnabled": false,
		"relaysEnabled":         false,
		"natEnabled":            false,
		"localAnnounceEnabled":  true,
		"urAccepted":            -1,
	}
	_, err := c.doJSON(ctx, http.MethodPatch, "/rest/config/options", payload)
	return err
}

// SystemStatus queries /rest/system/status and caches DeviceID.
func (c *Client) SystemStatus(ctx context.Context) (map[string]any, error) {
	res, err := c.doJSON(ctx, http.MethodGet, "/rest/system/status", nil)
	if err != nil {
		return nil, err
	}
	if id, ok := res["myID"].(string); ok && id != "" {
		c.mu.Lock()
		c.DeviceID = id
		c.mu.Unlock()
	}
	return res, nil
}

// FolderDBStatus queries /rest/db/status?folder=<id>.
func (c *Client) FolderDBStatus(ctx context.Context, folderID string) (FolderStatus, error) {
	res, err := c.doJSON(ctx, http.MethodGet, "/rest/db/status?folder="+folderID, nil)
	if err != nil {
		return FolderStatus{}, err
	}
	out := FolderStatus{FolderID: folderID}
	if s, ok := res["state"].(string); ok {
		out.State = s
	}
	out.GlobalFiles = toInt64(res["globalFiles"])
	out.GlobalBytes = toInt64(res["globalBytes"])
	out.LocalFiles = toInt64(res["localFiles"])
	out.NeedFiles = toInt64(res["needFiles"])
	out.NeedBytes = toInt64(res["needBytes"])
	out.Errors = toInt64(res["errors"])
	return out, nil
}

// TriggerScan runs /rest/db/scan?folder=<id>.
func (c *Client) TriggerScan(ctx context.Context, folderID string) error {
	path := "/rest/db/scan"
	if folderID != "" {
		path += "?folder=" + folderID
	}
	_, err := c.doJSON(ctx, http.MethodPost, path, nil)
	return err
}

// PauseDevice pauses synchronization for deviceID (or all devices if empty).
func (c *Client) PauseDevice(ctx context.Context, deviceID string) error {
	path := "/rest/system/pause"
	if deviceID != "" {
		path += "?device=" + deviceID
	}
	_, err := c.doJSON(ctx, http.MethodPost, path, nil)
	return err
}

// ResumeDevice resumes synchronization for deviceID (or all devices if empty).
func (c *Client) ResumeDevice(ctx context.Context, deviceID string) error {
	path := "/rest/system/resume"
	if deviceID != "" {
		path += "?device=" + deviceID
	}
	_, err := c.doJSON(ctx, http.MethodPost, path, nil)
	return err
}

// PairingQRSVG renders a Syncthing Device ID QR code for Android Syncthing-Fork scanning.
func (c *Client) PairingQRSVG(ctx context.Context) (string, string, error) {
	c.mu.Lock()
	id := c.DeviceID
	c.mu.Unlock()
	if id == "" && c.BaseURL != "" {
		if st, err := c.SystemStatus(ctx); err == nil {
			if myID, ok := st["myID"].(string); ok {
				id = myID
			}
		}
	}
	if id == "" {
		id = "UNPAIRED-SYNCTHING-OFFLINE"
	}
	svg, err := qrsvg.RenderSVG(id, 200)
	return id, svg, err
}

func (c *Client) doJSON(ctx context.Context, method, path string, body any) (map[string]any, error) {
	if !c.Breaker.Allow(time.Now()) {
		return nil, errors.New("syncthing circuit breaker open")
	}
	c.mu.Lock()
	baseURL := strings.TrimRight(c.BaseURL, "/")
	apiKey := c.APIKey
	c.mu.Unlock()
	if baseURL == "" {
		return nil, errors.New("syncthing REST endpoint offline")
	}

	var rbody *bytes.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rbody = bytes.NewReader(raw)
	} else {
		rbody = bytes.NewReader(nil)
	}

	req, err := http.NewRequestWithContext(ctx, method, baseURL+path, rbody)
	if err != nil {
		return nil, err
	}
	if apiKey != "" {
		req.Header.Set("X-API-Key", apiKey)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		c.Breaker.RecordFailure(time.Now(), 0)
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		c.Breaker.RecordFailure(time.Now(), 0)
		return nil, fmt.Errorf("syncthing %s %s HTTP %d", method, path, resp.StatusCode)
	}
	c.Breaker.RecordSuccess()

	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return out, nil
}

// ResolveExecutable inspects PATH or standard Windows Syncthing install path.
func ResolveExecutable() string {
	if p, err := exec.LookPath("syncthing"); err == nil {
		return p
	}
	if localApp := filepath.Clean(filepath.Join(filepath.VolumeName(filepath.Dir(".")), "Users")); localApp != "" {
		_ = localApp
	}
	return ""
}

func toInt64(v any) int64 {
	if f, ok := v.(float64); ok {
		return int64(f)
	}
	return 0
}
