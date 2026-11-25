package server

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mostakim2005/cloud-manager/internal/config"
	"github.com/Mostakim2005/cloud-manager/internal/db"
	_ "modernc.org/sqlite"
)

func setupTestDaemon(t *testing.T) (*Daemon, string) {
	t.Helper()
	dir := t.TempDir()
	paths := config.DiscoverPaths(dir)
	settings := config.DefaultSettings(paths.StagingDir)
	secrets := &config.EcosystemSecrets{
		SchemaVersion: 1,
		Tokens:        map[string]string{"cloudManager": "cm-secret-token-xyz"},
	}
	store, err := db.Open(paths.DatabasePath)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	logger, _ := config.NewLogger(paths.LogsDir)
	d, err := NewDaemon(paths, settings, secrets, store, logger)
	if err != nil {
		t.Fatalf("NewDaemon: %v", err)
	}
	d.Pipeline.SkipDiskCap = true
	d.Port = 59432
	return d, dir
}

func TestSecurityBoundariesHostOriginBearerAndAppBackplane(t *testing.T) {
	d, dir := setupTestDaemon(t)
	h := d.Handler()

	// 1. Missing auth must fail 401.
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:59432/api/v1/status", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 missing auth, got %d", rec.Code)
	}

	// 2. Evil Origin header must fail 403.
	req = httptest.NewRequest(http.MethodPost, "http://127.0.0.1:59432/api/v1/rules/simulate", strings.NewReader(`{"fileName":"a.pdf"}`))
	req.Header.Set("Authorization", "Bearer cm-secret-token-xyz")
	req.Header.Set("Origin", "https://evil.example")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for cross-origin header, got %d", rec.Code)
	}

	// 3. Machine /mcp endpoint must reject ANY Origin header with 403.
	req = httptest.NewRequest(http.MethodPost, "http://127.0.0.1:59432/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	req.Header.Set("Authorization", "Bearer cm-secret-token-xyz")
	req.Header.Set("Origin", "http://127.0.0.1:59432")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for Origin on /mcp, got %d", rec.Code)
	}

	// 4. Invalid Host (DNS rebinding) must fail 403.
	req = httptest.NewRequest(http.MethodGet, "http://evil.rebind:59432/api/v1/health", nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for invalid Host header, got %d", rec.Code)
	}

	// 5. Non-loopback Daemon bind must fail closed.
	if _, err := d.Start("0.0.0.0:59432"); err == nil {
		t.Fatalf("expected Start on 0.0.0.0 to fail closed")
	}

	// 6. Invalid ?app= slug must fail 400.
	req = httptest.NewRequest(http.MethodPost, "http://127.0.0.1:59432/api/v1/sync?app=../evil", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer cm-secret-token-xyz")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid ?app= slug, got %d", rec.Code)
	}

	// 7. Live SQLite VACUUM INTO sync via Backplane succeeds cleanly.
	liveDBPath := filepath.Join(dir, "app_notes.sqlite")
	liveDB, _ := sql.Open("sqlite", liveDBPath)
	_, _ = liveDB.Exec("PRAGMA journal_mode=WAL; CREATE TABLE k(v TEXT); INSERT INTO k(v) VALUES ('safe');")
	defer liveDB.Close()

	body, _ := json.Marshal(map[string]any{
		"relPath":      "notes_workspace.sqlite",
		"sqliteDbPath": liveDBPath,
		"autoQueue":    true,
	})
	req = httptest.NewRequest(http.MethodPost, "http://127.0.0.1:59432/api/v1/sync?app=custom-notes", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer cm-secret-token-xyz")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 VACUUM INTO sync, got %d: %s", rec.Code, rec.Body.String())
	}

	// 8. Authenticated /mcp tools/call succeeds without Origin.
	mcpReq := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:59432/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"cloud_route_dry_run","arguments":{"fileName":"lecture.mp4","originType":"recordings","sizeBytes":1000}}}`))
	mcpReq.Header.Set("X-Local-Token", "cm-secret-token-xyz")
	mcpRec := httptest.NewRecorder()
	h.ServeHTTP(mcpRec, mcpReq)
	if mcpRec.Code != http.StatusOK || !strings.Contains(mcpRec.Body.String(), "youtube") {
		t.Fatalf("expected MCP dry-run youtube result, got %d: %s", mcpRec.Code, mcpRec.Body.String())
	}
}
