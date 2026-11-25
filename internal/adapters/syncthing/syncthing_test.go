package syncthing

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSyncthingLANOnlyAndClickToSyncControls(t *testing.T) {
	var patchedLANOnly map[string]any
	var scannedFolder string
	var paused, resumed bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") != "st-key-123" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		switch {
		case r.Method == http.MethodPatch && r.URL.Path == "/rest/config/options":
			_ = json.NewDecoder(r.Body).Decode(&patchedLANOnly)
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodGet && r.URL.Path == "/rest/system/status":
			_ = json.NewEncoder(w).Encode(map[string]any{"myID": "ABCDEFG-1234567-SYNCTHING"})
		case r.Method == http.MethodGet && r.URL.Path == "/rest/db/status":
			_ = json.NewEncoder(w).Encode(map[string]any{"state": "idle", "globalFiles": 42, "needFiles": 3, "needBytes": 4096})
		case r.Method == http.MethodPost && r.URL.Path == "/rest/db/scan":
			scannedFolder = r.URL.Query().Get("folder")
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && r.URL.Path == "/rest/system/pause":
			paused = true
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && r.URL.Path == "/rest/system/resume":
			resumed = true
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	client := NewClient("", "", nil)
	client.BaseURL = srv.URL
	client.APIKey = "st-key-123"
	client.HTTPClient = srv.Client()

	if err := client.EnforceLANOnly(context.Background()); err != nil {
		t.Fatalf("EnforceLANOnly: %v", err)
	}
	if patchedLANOnly["globalAnnounceEnabled"] != false || patchedLANOnly["relaysEnabled"] != false || patchedLANOnly["localAnnounceEnabled"] != true {
		t.Fatalf("expected LAN-only flags enforced, got %#v", patchedLANOnly)
	}

	st, err := client.FolderDBStatus(context.Background(), "obsidian-nihon")
	if err != nil || st.NeedFiles != 3 {
		t.Fatalf("FolderDBStatus: %+v err=%v", st, err)
	}
	_ = client.TriggerScan(context.Background(), "obsidian-nihon")
	_ = client.PauseDevice(context.Background(), "")
	_ = client.ResumeDevice(context.Background(), "")
	if scannedFolder != "obsidian-nihon" || !paused || !resumed {
		t.Fatalf("expected scan/pause/resume REST calls")
	}

	id, svg, err := client.PairingQRSVG(context.Background())
	if err != nil || id != "ABCDEFG-1234567-SYNCTHING" || !strings.Contains(svg, "<svg") {
		t.Fatalf("expected valid QR SVG, id=%q err=%v", id, err)
	}
}
