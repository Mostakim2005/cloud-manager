package adb

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mostakim2005/cloud-manager/internal/db"
)

type fakeRunner struct {
	responses map[string]string
}

func (f fakeRunner) Run(_ context.Context, args ...string) (string, error) {
	key := strings.Join(args, " ")
	for k, v := range f.responses {
		if strings.Contains(key, k) {
			return SanitizeOutput(v), nil
		}
	}
	return "", nil
}

func TestADBCRLFMediaStoreDeltaAndQR(t *testing.T) {
	dir := t.TempDir()
	store, err := db.Open(filepath.Join(dir, "catalog.sqlite"))
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer store.Close()

	// Seed one already-synced photo so delta excludes it.
	_, _ = store.UpsertFile(db.CatalogFile{
		RelPath:   "DCIM/Camera/IMG_ALREADY.jpg",
		Name:      "IMG_ALREADY.jpg",
		SizeBytes: 2048,
		Provider:  "mega",
		RemoteID:  "mega-1",
	})

	// Simulate raw Windows ADB \r\r\n output with both seconds (date_modified) and ms (datetaken).
	imgOut := "Row: 0 _id=10, relative_path=DCIM/Camera/, _display_name=IMG_ALREADY.jpg, _size=2048, date_modified=1732500000, datetaken=1732500000123\r\r\n" +
		"Row: 1 _id=11, relative_path=DCIM/Camera/, _display_name=IMG_NEW.heic, _size=4096, date_modified=NULL, datetaken=1732501000456\r\r\n"
	vidOut := "Row: 0 _id=22, relative_path=Movies/Screen/, _display_name=CLIP_01.mp4, _size=1048576, date_modified=1732502000, datetaken=NULL\r\r\n"

	client := NewClientWithRunner(fakeRunner{responses: map[string]string{
		"devices -l":   "List of devices attached\r\r\nR5CT12345 device product:dm1q model:SM_S911B\r\r\n",
		"images/media": imgOut,
		"video/media":  vidOut,
	}}, store)

	devs, err := client.ListDevices(context.Background())
	if err != nil || len(devs) != 1 || devs[0].Model != "SM_S911B" {
		t.Fatalf("unexpected devices: %+v err=%v", devs, err)
	}

	items, err := client.QueryMediaDelta(context.Background(), "R5CT12345")
	if err != nil || len(items) != 2 {
		t.Fatalf("expected 2 media items, got %d (%v)", len(items), err)
	}
	if items[0].NeedsPull || !items[1].NeedsPull {
		t.Fatalf("expected IMG_ALREADY NeedsPull=false and CLIP_01 NeedsPull=true, got %+v", items)
	}
	if items[0].ModifiedUTC != 1732500000 {
		t.Fatalf("expected UTC seconds 1732500000, got %d", items[0].ModifiedUTC)
	}

	// Also verify datetaken millisecond normalization separately.
	parsed := ParseMediaStoreOutput(imgOut, "image")
	if len(parsed) != 2 || parsed[1].ModifiedUTC != 1732501000 {
		t.Fatalf("expected ms datetaken normalized to 1732501000 seconds, got %+v", parsed)
	}

	payload, svg, err := WirelessPairingQR("adb-CM-4821", "893412")
	if err != nil || !strings.HasPrefix(payload, "WIFI:T:ADB;") || !strings.Contains(svg, "<svg") {
		t.Fatalf("unexpected wireless ADB QR: %q err=%v", payload, err)
	}
}
