package ingest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mostakim2005/cloud-manager/internal/config"
	"github.com/Mostakim2005/cloud-manager/internal/db"
	"github.com/Mostakim2005/cloud-manager/internal/routing"
)

func TestConflictPreservationAndRAGManagerWebhook(t *testing.T) {
	dir := t.TempDir()
	store, err := db.Open(filepath.Join(dir, "catalog.sqlite"))
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer store.Close()

	var ragAuth string
	var ragEvent map[string]any
	ragSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/ingest/event" || r.Method != http.MethodPost {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		ragAuth = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&ragEvent)
		w.WriteHeader(http.StatusOK)
	}))
	defer ragSrv.Close()

	settings := config.DefaultSettings(filepath.Join(dir, "staging"))
	settings.RAGManagerURL = ragSrv.URL

	pipe := &Pipeline{
		Store:       store,
		Settings:    func() config.Settings { return settings },
		Paths:       config.DiscoverPaths(dir),
		Tracker:     routing.NewQuotaReservationTracker(),
		Secrets:     &config.EcosystemSecrets{Tokens: map[string]string{"ragManager": "rag-tok-123"}},
		HTTPClient:  ragSrv.Client(),
		SkipDiskCap: true,
	}

	res1, err := pipe.Ingest(context.Background(), Request{
		RelPath:        "Obsidian/Nihon/circuits.md",
		SourceReader:   strings.NewReader("# Version 1"),
		OriginType:     "syncthing",
		ParentRevision: "rev-1",
		AutoQueue:      true,
	})
	if err != nil {
		t.Fatalf("Ingest 1: %v", err)
	}
	if !res1.RAGNotified || ragAuth != "Bearer rag-tok-123" || ragEvent["rel_path"] != "Obsidian/Nihon/circuits.md" {
		t.Fatalf("expected RAG webhook notification, got notified=%v auth=%q event=%#v", res1.RAGNotified, ragAuth, ragEvent)
	}

	// Conflicting edit with different parent_revision must preserve conflict copy instead of overwriting.
	res2, err := pipe.Ingest(context.Background(), Request{
		RelPath:        "Obsidian/Nihon/circuits.md",
		SourceReader:   strings.NewReader("# Version 2 from phone"),
		OriginType:     "syncthing",
		ParentRevision: "rev-stale",
	})
	if err != nil {
		t.Fatalf("Ingest 2: %v", err)
	}
	if !res2.ConflictCreated || !strings.Contains(res2.ConflictPath, ".conflict-") {
		t.Fatalf("expected conflict preservation file, got %+v", res2)
	}
}
