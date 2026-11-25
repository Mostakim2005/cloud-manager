package config

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestAtomicWriteHasNoUTF8BOM(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "test.json")

	if err := WriteJSONAtomic(target, map[string]string{"name": "Cloud Manager"}, 0o600); err != nil {
		t.Fatalf("WriteJSONAtomic: %v", err)
	}
	raw, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if bytes.HasPrefix(raw, utf8BOM) {
		t.Fatalf("output must not contain UTF-8 BOM")
	}

	// Ensure StripUTF8BOM handles legacy BOM inputs gracefully.
	bommed := append(append([]byte{}, utf8BOM...), []byte(`{"schemaVersion":1,"tokens":{}}`)...)
	secPath := filepath.Join(dir, "secrets.json")
	if err := os.WriteFile(secPath, bommed, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	sec, err := LoadOrInitSecrets(secPath)
	if err != nil {
		t.Fatalf("LoadOrInitSecrets: %v", err)
	}
	if sec.Tokens["cloudManager"] == "" || sec.Tokens["ragManager"] == "" {
		t.Fatalf("expected missing tokens to be generated")
	}
	rewritten, _ := os.ReadFile(secPath)
	if bytes.HasPrefix(rewritten, utf8BOM) {
		t.Fatalf("rewritten secrets must strip UTF-8 BOM")
	}
}

func TestDiscoverPathsAndLoggerRedaction(t *testing.T) {
	dir := t.TempDir()
	p := DiscoverPaths(dir)
	if p.DatabasePath != filepath.Join(dir, "catalog.sqlite") {
		t.Fatalf("unexpected DB path: %s", p.DatabasePath)
	}
	s, err := LoadOrInitSettings(p)
	if err != nil {
		t.Fatalf("LoadOrInitSettings: %v", err)
	}
	if s.LANEnabledDefault {
		t.Fatalf("LAN QR Drop-Zone must be OFF by default")
	}

	logger, err := NewLogger(p.LogsDir)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	logger.Log("info", "test", "Authorization: Bearer super-secret", map[string]any{
		"X-Local-Token": "hidden-123",
		"safe":          "visible",
	})
	logBytes, err := os.ReadFile(filepath.Join(p.LogsDir, "cloud-manager.jsonl"))
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if bytes.Contains(logBytes, []byte("super-secret")) || bytes.Contains(logBytes, []byte("hidden-123")) {
		t.Fatalf("secret leaked in log file: %s", string(logBytes))
	}
}
