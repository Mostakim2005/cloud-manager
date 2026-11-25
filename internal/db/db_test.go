package db

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func TestOpenWALPragmasAndFTSSearch(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(filepath.Join(dir, "catalog.sqlite"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer store.Close()

	var mode string
	var timeout int
	_ = store.DB().QueryRow("PRAGMA journal_mode;").Scan(&mode)
	_ = store.DB().QueryRow("PRAGMA busy_timeout;").Scan(&timeout)
	if mode != "wal" {
		t.Fatalf("expected WAL journal_mode, got %q", mode)
	}
	if timeout != 5000 {
		t.Fatalf("expected busy_timeout=5000, got %d", timeout)
	}

	_, err = store.UpsertFile(CatalogFile{
		RelPath:    "notes/notes_workspace.sqlite",
		Name:       "notes_workspace.sqlite",
		SizeBytes:  1024,
		SHA256:     "abc123",
		Provider:   "gdrive",
		RemoteID:   "gdrive-1",
		OriginType: "app",
	})
	if err != nil {
		t.Fatalf("UpsertFile 1: %v", err)
	}
	_, err = store.UpsertFile(CatalogFile{
		RelPath:    "backup/notes_workspace.sqlite",
		Name:       "notes_workspace.sqlite",
		SizeBytes:  1024,
		SHA256:     "abc123",
		Provider:   "mega",
		RemoteID:   "mega-1",
		OriginType: "app",
	})
	if err != nil {
		t.Fatalf("UpsertFile 2: %v", err)
	}

	results, total, err := store.SearchCatalog("notes_workspace", "", "", 50, 0)
	if err != nil {
		t.Fatalf("SearchCatalog: %v", err)
	}
	if total != 2 || len(results) != 2 {
		t.Fatalf("expected 2 FTS results, got total=%d len=%d", total, len(results))
	}

	dups, err := store.ListDuplicates()
	if err != nil {
		t.Fatalf("ListDuplicates: %v", err)
	}
	if len(dups) != 1 || dups[0].WastedBytes != 1024 {
		t.Fatalf("expected 1 duplicate group wasting 1024 bytes, got %#v", dups)
	}
}

func TestLiveSQLiteVacuumIntoSnapshot(t *testing.T) {
	dir := t.TempDir()
	livePath := filepath.Join(dir, "live_app.sqlite")
	liveDB, err := sql.Open("sqlite", livePath)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	_, _ = liveDB.Exec("PRAGMA journal_mode = WAL; CREATE TABLE notes(id INTEGER PRIMARY KEY, body TEXT); INSERT INTO notes(body) VALUES ('hello wal');")
	// Intentionally keep liveDB open with WAL active.
	snapPath := filepath.Join(dir, "snapshot.sqlite")
	if err := SnapshotSQLiteFile(livePath, snapPath); err != nil {
		t.Fatalf("SnapshotSQLiteFile: %v", err)
	}
	_ = liveDB.Close()

	snapDB, err := sql.Open("sqlite", snapPath)
	if err != nil {
		t.Fatalf("open snapshot: %v", err)
	}
	defer snapDB.Close()
	var body string
	if err := snapDB.QueryRow("SELECT body FROM notes WHERE id=1").Scan(&body); err != nil || body != "hello wal" {
		t.Fatalf("expected snapshot row 'hello wal', got %q (%v)", body, err)
	}
}

func TestYouTubeQuotaAndSoftDeleteTombstone(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(filepath.Join(dir, "catalog.sqlite"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer store.Close()

	used, ok, err := store.AddYouTubeQuota("2026-11-25", 1650, 9600)
	if err != nil || !ok || used != 1650 {
		t.Fatalf("unexpected quota result: used=%d ok=%v err=%v", used, ok, err)
	}
	_, ok, _ = store.AddYouTubeQuota("2026-11-25", 8500, 9600)
	if ok {
		t.Fatalf("expected quota cap 9600 to reject overflow")
	}

	id, _ := store.UpsertFile(CatalogFile{RelPath: "a.pdf", Name: "a.pdf", Provider: "gdrive", RemoteID: "gdrive-1"})
	tomb, err := store.SoftDeleteFile(id)
	if err != nil || tomb.Resolution != "PENDING" {
		t.Fatalf("expected PENDING tombstone, got %#v (%v)", tomb, err)
	}
	if err := store.ResolveTombstone(tomb.ID, "RESTORED"); err != nil {
		t.Fatalf("ResolveTombstone: %v", err)
	}
	restored, err := store.GetFileByID(id)
	if err != nil || restored.TombstonedAt != 0 || restored.Status != "SYNCED" {
		t.Fatalf("expected restored file, got %#v (%v)", restored, err)
	}
}
