package rclone

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/Mostakim2005/cloud-manager/internal/db"
)

func TestRcloneVersionAndMegaGuard(t *testing.T) {
	if err := CheckVersion("rclone v1.65.2"); err == nil {
		t.Fatalf("expected rclone v1.65.2 to be rejected")
	}
	if err := CheckVersion("rclone v1.76.0"); err != nil {
		t.Fatalf("expected rclone v1.76.0 to pass, got %v", err)
	}
	if err := ValidateSyncMode("mega", "bisync", "mega1:"); !errors.Is(err, ErrMegaBisyncForbidden) {
		t.Fatalf("expected ErrMegaBisyncForbidden, got %v", err)
	}
}

func TestRcloneQuotaReconciliationAnd509Cooldown(t *testing.T) {
	dir := t.TempDir()
	store, err := db.Open(filepath.Join(dir, "catalog.sqlite"))
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer store.Close()

	_, _ = store.UpsertFile(db.CatalogFile{
		RelPath:  "deleted-remotely.jpg",
		Name:     "deleted-remotely.jpg",
		Provider: "mega",
		RemoteID: "mega-1",
		Status:   "SYNCED",
	})

	hit509 := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != "rcu" || pass != "rcp" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/operations/about":
			_ = json.NewEncoder(w).Encode(map[string]any{"total": 20000000000, "used": 5000000000, "free": 15000000000})
		case "/operations/list":
			_ = json.NewEncoder(w).Encode(map[string]any{"list": []map[string]any{{"Path": "still-there.jpg"}}})
		case "/operations/copyfile":
			hit509 = true
			w.WriteHeader(509)
		}
	}))
	defer srv.Close()

	sup := NewSupervisor("", "", "", store, nil)
	sup.RCAddr = srv.URL
	sup.RCUser = "rcu"
	sup.RCPass = "rcp"
	sup.HTTPClient = srv.Client()

	acc := db.Account{ID: "mega-1", Provider: "mega", RcloneRemote: "mega1:", HasherRemote: "mega1-hasher:"}
	updated, err := sup.RefreshQuota(context.Background(), acc)
	if err != nil || updated.FreeBytes != 15000000000 {
		t.Fatalf("RefreshQuota: %+v err=%v", updated, err)
	}

	tombs, err := sup.ReconcileRemote(context.Background(), acc)
	if err != nil || tombs != 1 {
		t.Fatalf("expected 1 tombstone from reconciliation, got %d err=%v", tombs, err)
	}

	err = sup.TransferFile(context.Background(), acc, "D:/Staging", "a.jpg", "a.jpg")
	if err == nil || !hit509 {
		t.Fatalf("expected HTTP 509 rate-limit error")
	}
	accounts, _ := store.ListAccounts()
	var found bool
	for _, a := range accounts {
		if a.ID == "mega-1" && a.Health == "rate_limited_509" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected mega-1 account cooldown to be recorded in SQLite")
	}
}
