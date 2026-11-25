package lanqr

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mostakim2005/cloud-manager/internal/config"
	"github.com/Mostakim2005/cloud-manager/internal/db"
	"github.com/Mostakim2005/cloud-manager/internal/ingest"
	"github.com/Mostakim2005/cloud-manager/internal/routing"
)

func TestLANQRDisabledByDefaultAndRoundTripUploadDownload(t *testing.T) {
	dir := t.TempDir()
	store, err := db.Open(filepath.Join(dir, "catalog.sqlite"))
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer store.Close()

	settings := config.DefaultSettings(filepath.Join(dir, "staging"))
	pipe := &ingest.Pipeline{
		Store:       store,
		Settings:    func() config.Settings { return settings },
		Paths:       config.DiscoverPaths(dir),
		Tracker:     routing.NewQuotaReservationTracker(),
		SkipDiskCap: true,
	}

	outbox := filepath.Join(dir, "outbox")
	_ = os.MkdirAll(outbox, 0o700)
	_ = os.WriteFile(filepath.Join(outbox, "study.pdf"), []byte("%PDF-1.4"), 0o600)

	// Port 0 picks ephemeral free TLS port for race-free parallel CI tests.
	srv := NewServer(0, outbox, pipe, func() config.Settings { return settings })
	if srv.Status().Enabled {
		t.Fatalf("expected LAN QR Drop-Zone OFF by default")
	}

	st, err := srv.Enable()
	if err != nil {
		t.Fatalf("Enable: %v", err)
	}
	defer srv.Disable(context.Background())

	if !st.Enabled || len(st.PIN) != 6 || !strings.Contains(st.QRSVG, "<svg") {
		t.Fatalf("unexpected enabled status: %+v", st)
	}

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	baseURL := fmt.Sprintf("https://127.0.0.1:%d", st.Port)

	// Wrong PIN must return 401.
	badResp, err := client.Get(baseURL + "/staged?pin=000000")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	_ = badResp.Body.Close()
	if badResp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 for bad PIN, got %d", badResp.StatusCode)
	}

	// Valid PIN multipart upload must stage file via unified SHA-256 pipeline.
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile("file", "phone_note.md")
	_, _ = fw.Write([]byte("# Note from phone"))
	_ = mw.Close()

	upReq, _ := http.NewRequest(http.MethodPost, baseURL+"/upload?pin="+st.PIN, &body)
	upReq.Header.Set("Content-Type", mw.FormDataContentType())
	upResp, err := client.Do(upReq)
	if err != nil || upResp.StatusCode != http.StatusOK {
		t.Fatalf("expected upload 200, got %v %v", upResp, err)
	}
	_ = upResp.Body.Close()

	// Traversal on download must fail 400.
	travResp, err := client.Get(baseURL + "/download?pin=" + st.PIN + "&file=../secret.txt")
	if err != nil {
		t.Fatalf("download trav failed: %v", err)
	}
	_ = travResp.Body.Close()
	if travResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 path traversal rejection, got %d", travResp.StatusCode)
	}
}
