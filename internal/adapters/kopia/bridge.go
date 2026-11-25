package kopia

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// SnapshotSummary captures a Kopia FastCDC block-delta snapshot record.
type SnapshotSummary struct {
	ID        string `json:"id"`
	RootEntry struct {
		Summary struct {
			Size int64 `json:"size"`
		} `json:"summ"`
	} `json:"rootEntry"`
}

// Bridge executes Kopia CLI commands via argv arrays without spawning shells.
type Bridge struct {
	BinaryPath string
}

// NewBridge initializes a Kopia CLI bridge.
func NewBridge(binaryPath string) *Bridge {
	return &Bridge{BinaryPath: binaryPath}
}

func (b *Bridge) resolveBin() (string, error) {
	if b.BinaryPath != "" {
		return b.BinaryPath, nil
	}
	bin, err := exec.LookPath("kopia")
	if err != nil {
		return "", fmt.Errorf("kopia binary not found: %w", err)
	}
	return bin, nil
}

// CreateSnapshot creates a FastCDC deduplicated snapshot of dirPath and returns the snapshot ID.
func (b *Bridge) CreateSnapshot(ctx context.Context, dirPath string, tags map[string]string) (string, error) {
	bin, err := b.resolveBin()
	if err != nil {
		return "", err
	}
	args := []string{"snapshot", "create", dirPath, "--json"}
	for k, v := range tags {
		args = append(args, "--tags", fmt.Sprintf("%s:%s", k, v))
	}
	out, err := exec.CommandContext(ctx, bin, args...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("kopia snapshot create: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	var res map[string]any
	if err := json.Unmarshal(out, &res); err == nil {
		if id, ok := res["id"].(string); ok && id != "" {
			return id, nil
		}
	}
	return strings.TrimSpace(string(out)), nil
}

// ListSnapshots lists Kopia snapshots for sourceDir.
func (b *Bridge) ListSnapshots(ctx context.Context, sourceDir string) ([]SnapshotSummary, error) {
	bin, err := b.resolveBin()
	if err != nil {
		return nil, err
	}
	args := []string{"snapshot", "list", "--json"}
	if sourceDir != "" {
		args = append(args, sourceDir)
	}
	out, err := exec.CommandContext(ctx, bin, args...).Output()
	if err != nil {
		return nil, err
	}
	var list []SnapshotSummary
	if err := json.Unmarshal(out, &list); err != nil {
		return nil, err
	}
	return list, nil
}
