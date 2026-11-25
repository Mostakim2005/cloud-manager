package youtube

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Mostakim2005/cloud-manager/internal/db"
)

func TestPrivateResumableUploadChunkResumeAndPTQuota(t *testing.T) {
	dir := t.TempDir()
	store, err := db.Open(filepath.Join(dir, "catalog.sqlite"))
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer store.Close()

	videoPath := filepath.Join(dir, "demo.mp4")
	payload := strings.Repeat("A", 100) + strings.Repeat("B", 100) + strings.Repeat("C", 50)
	_ = os.WriteFile(videoPath, []byte(payload), 0o600)

	var privacySeen string
	var playlistInserted bool
	var uploaded bytes.Buffer
	var chunksReceived int

	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/upload"):
			var meta map[string]any
			_ = json.NewDecoder(r.Body).Decode(&meta)
			privacySeen = meta["status"].(map[string]any)["privacyStatus"].(string)
			w.Header().Set("Location", srv.URL+"/session/abc")
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPut && r.URL.Path == "/session/abc":
			cr := r.Header.Get("Content-Range")
			if strings.HasPrefix(cr, "bytes */") {
				w.Header().Set("Range", "bytes=0-99")
				w.WriteHeader(308)
				return
			}
			chunk, _ := io.ReadAll(r.Body)
			uploaded.Write(chunk)
			chunksReceived++
			if uploaded.Len() < len(payload) {
				w.WriteHeader(308)
				return
			}
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "yt_vid_999"})
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "playlistItems"):
			playlistInserted = true
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "pl_item_1"})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	uploader := NewUploader(store, nil, srv.Client())
	uploader.UploadInitURL = srv.URL + "/upload"
	uploader.APIEndpoint = srv.URL + "/"
	uploader.ChunkSize = 100

	jobID, _ := store.EnqueueJob(db.Job{
		Kind:           "youtube_upload",
		SourcePath:     videoPath,
		TargetRemote:   "youtube-private",
		TargetPlaylist: "PL_PRIVATE",
		BytesTotal:     int64(len(payload)),
	})
	jobs, _ := store.ListJobs(10)
	vid, err := uploader.ProcessJob(context.Background(), jobs[0], "Demo Recording")
	if err != nil {
		t.Fatalf("ProcessJob: %v", err)
	}
	if vid != "yt_vid_999" || privacySeen != "private" || !playlistInserted || chunksReceived != 3 {
		t.Fatalf("unexpected upload outcome: vid=%q privacy=%q playlist=%v chunks=%d", vid, privacySeen, playlistInserted, chunksReceived)
	}
	if uploaded.String() != payload {
		t.Fatalf("reassembled stream mismatch")
	}

	qs, err := uploader.QuotaStatus(time.Now())
	if err != nil || qs.UnitsUsed != UploadCostUnits {
		t.Fatalf("unexpected quota status: %+v err=%v", qs, err)
	}

	// Verify job transitioned to SYNCED with persisted resumable URI.
	jobsAfter, _ := store.ListJobs(10)
	if jobsAfter[0].ID != jobID || jobsAfter[0].Status != "SYNCED" || jobsAfter[0].ResumableURI == "" {
		t.Fatalf("unexpected persisted job state: %+v", jobsAfter[0])
	}
}
