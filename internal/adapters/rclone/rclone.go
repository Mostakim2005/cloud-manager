package rclone

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Mostakim2005/cloud-manager/internal/adapters"
	"github.com/Mostakim2005/cloud-manager/internal/config"
	"github.com/Mostakim2005/cloud-manager/internal/db"
	"github.com/Mostakim2005/cloud-manager/internal/security"
)

var (
	// ErrMegaBisyncForbidden enforces the Mega ModTime/hash trap rule.
	ErrMegaBisyncForbidden = errors.New("timestamp-based bisync is forbidden on Mega remotes; use Append/Archive via hasher")
	versionRegex           = regexp.MustCompile(`v?(\d+)\.(\d+)`)
)

// Supervisor manages the Job-Object-bound rclone rcd process and JSON-RPC pool operations.
type Supervisor struct {
	mu         sync.Mutex
	BinaryPath string
	ConfPath   string
	ConfigPass string
	RCAddr     string
	RCUser     string
	RCPass     string
	JobSup     *security.JobSupervisor
	Store      *db.Store
	Breaker    *adapters.CircuitBreaker
	HTTPClient *http.Client
	cmd        *exec.Cmd
}

// NewSupervisor initializes an Rclone supervisor (without spawning until Start).
func NewSupervisor(binaryPath, confPath, configPass string, store *db.Store, jobSup *security.JobSupervisor) *Supervisor {
	return &Supervisor{
		BinaryPath: binaryPath,
		ConfPath:   confPath,
		ConfigPass: configPass,
		JobSup:     jobSup,
		Store:      store,
		Breaker:    adapters.NewCircuitBreaker(3, 60*time.Second),
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// CheckVersion verifies rclone >= v1.66 for reliable hasher + union behavior.
func CheckVersion(versionOutput string) error {
	m := versionRegex.FindStringSubmatch(versionOutput)
	if len(m) < 3 {
		return fmt.Errorf("unable to parse rclone version from %q", versionOutput)
	}
	maj, _ := strconv.Atoi(m[1])
	min, _ := strconv.Atoi(m[2])
	if maj < 1 || (maj == 1 && min < 66) {
		return fmt.Errorf("rclone v%d.%d is below required >= v1.66", maj, min)
	}
	return nil
}

// Start launches rclone rcd on 127.0.0.1:<ephemeral> with 256-bit auth and env-only RCLONE_CONFIG_PASS.
func (s *Supervisor) Start(ctx context.Context) error {
	bin := s.BinaryPath
	if bin == "" {
		found, err := exec.LookPath("rclone")
		if err != nil {
			return fmt.Errorf("rclone binary not found: %w", err)
		}
		bin = found
	}

	out, err := exec.CommandContext(ctx, bin, "version").CombinedOutput()
	if err != nil {
		return fmt.Errorf("rclone version check: %w", err)
	}
	if err := CheckVersion(string(out)); err != nil {
		return err
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	user, err := security.GenerateToken()
	if err != nil {
		return err
	}
	pass, err := security.GenerateToken()
	if err != nil {
		return err
	}

	args := []string{
		"rcd",
		"--rc-addr=" + addr,
		"--rc-user=" + user,
		"--rc-pass=" + pass,
		"--rc-allow-origin=",
		"--rc-no-auth=false",
	}
	if s.ConfPath != "" {
		args = append(args, "--config="+s.ConfPath)
	}

	cmd := exec.Command(bin, args...)
	env := os.Environ()
	if s.ConfigPass != "" {
		env = append(env, "RCLONE_CONFIG_PASS="+s.ConfigPass)
	}
	cmd.Env = env
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start rclone rcd: %w", err)
	}
	if s.JobSup != nil {
		_ = s.JobSup.AssignPID(cmd.Process.Pid)
	}

	s.mu.Lock()
	s.RCAddr = "http://" + addr
	s.RCUser = user
	s.RCPass = pass
	s.cmd = cmd
	s.mu.Unlock()
	return nil
}

// Stop triggers graceful core/quit and kills if unresponsive.
func (s *Supervisor) Stop(ctx context.Context) error {
	s.mu.Lock()
	cmd := s.cmd
	addr := s.RCAddr
	s.cmd = nil
	s.mu.Unlock()

	if addr != "" {
		_, _ = s.CallRPC(ctx, "core/quit", map[string]any{})
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

// CallRPC executes an authenticated Rclone JSON-RPC call against rclone rcd.
func (s *Supervisor) CallRPC(ctx context.Context, method string, params map[string]any) (map[string]any, error) {
	if !s.Breaker.Allow(time.Now()) {
		return nil, errors.New("rclone circuit breaker open (cooldown active)")
	}
	s.mu.Lock()
	baseURL := s.RCAddr
	user := s.RCUser
	pass := s.RCPass
	s.mu.Unlock()
	if baseURL == "" {
		return nil, errors.New("rclone rcd endpoint not initialized")
	}

	body, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(baseURL, "/")+"/"+strings.TrimLeft(method, "/"), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if user != "" || pass != "" {
		req.SetBasicAuth(user, pass)
	}

	resp, err := s.HTTPClient.Do(req)
	if err != nil {
		s.Breaker.RecordFailure(time.Now(), 0)
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 509 || resp.StatusCode == http.StatusTooManyRequests {
		wait := adapters.ComputeBackoff(resp, 3)
		s.Breaker.RecordFailure(time.Now(), wait)
		return nil, fmt.Errorf("rclone rate limit HTTP %d (cooldown %s)", resp.StatusCode, wait)
	}
	if resp.StatusCode >= 400 {
		s.Breaker.RecordFailure(time.Now(), 0)
		var errBody map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&errBody)
		return nil, fmt.Errorf("rclone rpc %s HTTP %d: %v", method, resp.StatusCode, errBody["error"])
	}

	s.Breaker.RecordSuccess()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return out, nil
}

// ValidateSyncMode blocks timestamp bisync on Mega remotes while permitting Append/Archive + hasher.
func ValidateSyncMode(provider, mode, remoteName string) error {
	if strings.EqualFold(provider, "mega") {
		if strings.EqualFold(mode, "bisync") {
			return ErrMegaBisyncForbidden
		}
	}
	if remoteName == "" {
		return errors.New("remote name required")
	}
	return nil
}

// TransferFile dispatches a file to an account enforcing Mega Append/Archive + hasher overlays.
func (s *Supervisor) TransferFile(ctx context.Context, acc db.Account, srcFS, srcRel, dstRel string) error {
	mode := "copy"
	if acc.Provider == "gdrive" {
		mode = "bisync"
	}
	if err := ValidateSyncMode(acc.Provider, mode, acc.RcloneRemote); err != nil {
		return err
	}
	dstFS := acc.RcloneRemote
	if acc.Provider == "mega" && acc.HasherRemote != "" {
		dstFS = acc.HasherRemote
	}
	_, err := s.CallRPC(ctx, "operations/copyfile", map[string]any{
		"srcFs":     srcFS,
		"srcRemote": srcRel,
		"dstFs":     dstFS,
		"dstRemote": dstRel,
	})
	if err != nil && strings.Contains(err.Error(), "HTTP 509") && s.Store != nil {
		acc.CooldownUntil = time.Now().UTC().Add(15 * time.Minute).Unix()
		acc.Health = "rate_limited_509"
		_ = s.Store.UpsertAccount(acc)
	}
	return err
}

// RefreshQuota queries operations/about on an account and persists free/used/total bytes to SQLite.
func (s *Supervisor) RefreshQuota(ctx context.Context, acc db.Account) (db.Account, error) {
	res, err := s.CallRPC(ctx, "operations/about", map[string]any{"fs": acc.RcloneRemote})
	if err != nil {
		return acc, err
	}
	if total, ok := toInt64(res["total"]); ok {
		acc.TotalBytes = total
	}
	if used, ok := toInt64(res["used"]); ok {
		acc.UsedBytes = used
	}
	if free, ok := toInt64(res["free"]); ok {
		acc.FreeBytes = free
	} else if acc.TotalBytes > 0 {
		acc.FreeBytes = acc.TotalBytes - acc.UsedBytes
	}
	acc.Health = "healthy"
	acc.UpdatedAt = time.Now().UTC().Unix()
	if s.Store != nil {
		_ = s.Store.UpsertAccount(acc)
	}
	return acc, nil
}

// ReconcileRemote lists remote objects and marks missing tracked files as reviewable Tombstones.
func (s *Supervisor) ReconcileRemote(ctx context.Context, acc db.Account) (int, error) {
	targetFS := acc.RcloneRemote
	if acc.Provider == "mega" && acc.HasherRemote != "" {
		targetFS = acc.HasherRemote
	}
	res, err := s.CallRPC(ctx, "operations/list", map[string]any{
		"fs":     targetFS,
		"remote": "",
		"opt":    map[string]any{"recurse": true},
	})
	if err != nil {
		return 0, err
	}
	present := make(map[string]bool)
	if items, ok := res["list"].([]any); ok {
		for _, item := range items {
			if m, ok := item.(map[string]any); ok {
				if p, ok := m["Path"].(string); ok {
					present[p] = true
				}
			}
		}
	}

	tombstonesCreated := 0
	if s.Store != nil {
		files, _, _ := s.Store.SearchCatalog("", acc.Provider, "SYNCED", 500, 0)
		for _, f := range files {
			if f.RemoteID != acc.ID {
				continue
			}
			if !present[f.RelPath] {
				if _, err := s.Store.AddTombstone(f.ID, acc.ID, f.RelPath); err == nil {
					tombstonesCreated++
				}
			}
		}
		_ = s.Store.SetRemoteCursor(acc.ID, fmt.Sprintf("reconciled-%d", time.Now().UTC().Unix()))
	}
	return tombstonesCreated, nil
}

// EnsureMegaHasherConfig writes an encrypted-ready Rclone union+hasher section into confPath atomically.
func EnsureMegaHasherConfig(confPath, remoteName, hasherName string) error {
	if confPath == "" {
		return nil
	}
	existingBytes, _ := os.ReadFile(confPath)
	content := string(config.StripUTF8BOM(existingBytes))
	section := fmt.Sprintf("[%s]\n", hasherName)
	if strings.Contains(content, section) {
		return nil
	}
	addition := fmt.Sprintf("\n[%s]\ntype = hasher\nremote = %s:\nhashes = sha256\n", hasherName, remoteName)
	return config.WriteAtomicNoBOM(confPath, []byte(content+addition), 0o600)
}

func toInt64(v any) (int64, bool) {
	switch n := v.(type) {
	case float64:
		return int64(n), true
	case int64:
		return n, true
	case json.Number:
		i, err := n.Int64()
		return i, err == nil
	default:
		return 0, false
	}
}
