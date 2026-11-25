package security

import (
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestLoopbackHostAndOriginValidation(t *testing.T) {
	if !IsLoopbackBind("127.0.0.1:59432") || !IsLoopbackBind("localhost:59432") || !IsLoopbackBind("[::1]:59432") {
		t.Fatalf("expected loopback addresses to pass")
	}
	for _, bad := range []string{"", "0.0.0.0:59432", "[::]:59432", "192.168.1.10:59432"} {
		if IsLoopbackBind(bad) {
			t.Fatalf("expected non-loopback address %q to fail closed", bad)
		}
	}

	if !ValidateLoopbackHost("127.0.0.1:59432", 59432) || !ValidateLoopbackHost("localhost:59432", 59432) {
		t.Fatalf("expected valid loopback Host headers to pass")
	}
	for _, badHost := range []string{"", "evil.example:59432", "127.0.0.1:9999", "192.168.1.5:59432"} {
		if ValidateLoopbackHost(badHost, 59432) {
			t.Fatalf("expected invalid Host %q to be rejected", badHost)
		}
	}

	// Strict machine endpoints (/mcp, /api/v1/sync) must reject ANY Origin header.
	if !ValidateOrigin("", 59432, true) {
		t.Fatalf("expected empty Origin to pass strict check")
	}
	for _, origin := range []string{"https://evil.example", "http://127.0.0.1:59432", "null"} {
		if ValidateOrigin(origin, 59432, true) {
			t.Fatalf("expected strictNoOrigin to reject %q", origin)
		}
	}

	// UI endpoints allow only exact same-origin http://127.0.0.1:59432 or http://localhost:59432.
	if !ValidateOrigin("http://127.0.0.1:59432", 59432, false) || !ValidateOrigin("http://localhost:59432", 59432, false) {
		t.Fatalf("expected same-origin UI header to pass")
	}
	if ValidateOrigin("https://evil.example", 59432, false) || ValidateOrigin("http://127.0.0.1:3000", 59432, false) {
		t.Fatalf("expected cross-origin UI header to be rejected")
	}
}

func TestAppSlugAndTokenComparison(t *testing.T) {
	for _, ok := range []string{"gaussboard", "rag-manager", "app_01", "a"} {
		if !ValidateAppSlug(ok) {
			t.Fatalf("expected slug %q to be valid", ok)
		}
	}
	for _, bad := range []string{"", "-bad", "_bad", "BadCase", "../escape", "app/sub", strings.Repeat("a", 65)} {
		if ValidateAppSlug(bad) {
			t.Fatalf("expected slug %q to be rejected", bad)
		}
	}

	req, _ := http.NewRequest(http.MethodPost, "http://127.0.0.1:59432/api/v1/sync", nil)
	req.Header.Set("Authorization", "Bearer top-secret-token")
	if got := ExtractBearerToken(req); got != "top-secret-token" {
		t.Fatalf("got %q want top-secret-token", got)
	}
	if !ConstantTimeTokenMatch(gotToken(req), "other", "top-secret-token") {
		t.Fatalf("expected constant time match to succeed")
	}
	if ConstantTimeTokenMatch("wrong", "top-secret-token") {
		t.Fatalf("expected mismatch to fail")
	}
}

func gotToken(r *http.Request) string { return ExtractBearerToken(r) }

func TestPathTraversalAndNTFSConfinement(t *testing.T) {
	badPaths := []string{
		"",
		"../secret.txt",
		`..\..\Startup\evil.exe`,
		`C:\Windows\System32\cmd.exe`,
		`\\server\share\file.txt`,
		"/etc/passwd",
		"folder/\x00evil.txt",
		"CON",
		"sub/con.txt",
		"LPT1.log",
		"NUL",
		"trailingdot.",
		"trailingspace ",
		"bad<char>.txt",
	}
	for _, p := range badPaths {
		if _, err := SanitizeRelPath(p); err == nil {
			t.Fatalf("expected SanitizeRelPath(%q) to fail", p)
		}
	}

	root := t.TempDir()
	good, err := ResolveConfinedPath(root, "inbox/photo_01.heic")
	if err != nil {
		t.Fatalf("expected clean path to succeed, got %v", err)
	}
	if !strings.HasPrefix(good, root) {
		t.Fatalf("resolved path %q outside %q", good, root)
	}

	outside := t.TempDir()
	escapeLink := filepath.Join(root, "escape")
	if err := os.Symlink(outside, escapeLink); err == nil || runtime.GOOS != "windows" {
		if err == nil {
			if _, rerr := ResolveConfinedPath(root, "escape/payload.txt"); rerr == nil {
				t.Fatalf("expected symlink escape to be blocked")
			}
		}
	}
}

func TestSecretProtectionAndRedaction(t *testing.T) {
	plain := []byte("super-secret-rclone-master-key")
	sealed, err := ProtectSecret(plain)
	if err != nil {
		t.Fatalf("ProtectSecret failed: %v", err)
	}
	opened, err := UnprotectSecret(sealed)
	if err != nil {
		t.Fatalf("UnprotectSecret failed: %v", err)
	}
	if string(opened) != string(plain) {
		t.Fatalf("roundtrip mismatch: got %q", string(opened))
	}

	raw := `Authorization: Bearer abc123xyz RCLONE_CONFIG_PASS=mysecret123 refresh_token="1//0gSecret"`
	redacted := RedactSecrets(raw)
	for _, leak := range []string{"abc123xyz", "mysecret123", "1//0gSecret"} {
		if strings.Contains(redacted, leak) {
			t.Fatalf("secret %q leaked in %q", leak, redacted)
		}
	}

	m := RedactMap(map[string]any{
		"X-Local-Token": "tok-123",
		"nested": map[string]any{
			"RCLONE_PASS": "pw-456",
			"safeField":   "ok",
		},
	})
	if m["X-Local-Token"] != "[REDACTED]" {
		t.Fatalf("expected X-Local-Token redacted")
	}
	nested := m["nested"].(map[string]any)
	if nested["RCLONE_PASS"] != "[REDACTED]" || nested["safeField"] != "ok" {
		t.Fatalf("unexpected nested redaction result: %#v", nested)
	}
}
