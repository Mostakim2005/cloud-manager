package cli

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIJSONContractAndExitCodes(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("CLOUD_MANAGER_DATA_DIR", tmp)
	t.Setenv("LOCAL_ECOSYSTEM_SECRETS", filepath.Join(tmp, "secrets.json"))

	// Exit 2 on unknown command.
	var out, errBuf bytes.Buffer
	code := Run([]string{"bogus-verb", "--json"}, strings.NewReader(""), &out, &errBuf)
	if code != ExitInvalidArg {
		t.Fatalf("expected ExitInvalidArg (2), got %d", code)
	}

	// Exit 0 and valid single JSON on stdout for health --json.
	out.Reset()
	errBuf.Reset()
	code = Run([]string{"health", "--json"}, strings.NewReader(""), &out, &errBuf)
	if code != ExitOK {
		t.Fatalf("expected ExitOK (0) for health --json, got %d (stderr=%s)", code, errBuf.String())
	}
	var healthPayload map[string]any
	if err := json.Unmarshal(out.Bytes(), &healthPayload); err != nil {
		t.Fatalf("stdout must be pure JSON, got %q (%v)", out.String(), err)
	}

	// Exit 0 and deterministic routing dry-run via route test.
	out.Reset()
	errBuf.Reset()
	code = Run([]string{"route", "test", "lecture.mp4", "--origin", "recordings", "--size", "5000000", "--json"}, strings.NewReader(""), &out, &errBuf)
	if code != ExitOK {
		t.Fatalf("expected ExitOK for route test, got %d (%s)", code, errBuf.String())
	}
	var routePayload map[string]any
	if err := json.Unmarshal(out.Bytes(), &routePayload); err != nil || routePayload["targetProvider"] != "youtube" {
		t.Fatalf("unexpected route test output: %q (%v)", out.String(), err)
	}

	// Exit 3 when daemon is offline for status --json.
	out.Reset()
	errBuf.Reset()
	code = Run([]string{"status", "--json"}, strings.NewReader(""), &out, &errBuf)
	if code != ExitOfflineDep {
		t.Fatalf("expected ExitOfflineDep (3) when daemon offline, got %d", code)
	}
}
