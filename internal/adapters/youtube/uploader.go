package youtube

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Mostakim2005/cloud-manager/internal/adapters"
	"github.com/Mostakim2005/cloud-manager/internal/db"
	"github.com/Mostakim2005/cloud-manager/internal/telemetry"
	"google.golang.org/api/option"
	ytv3 "google.golang.org/api/youtube/v3"
)

const (
	DefaultChunkSizeBytes = 16 * 1024 * 1024 // 16 MiB chunks
	UploadCostUnits       = 1650             // videos.insert (1600) + playlistItems.insert (50)
	DailyQuotaCapUnits    = 9600             // Safety cap out of 10,000 PT units/day
)

var pacificTZ = loadPacificTZ()

func loadPacificTZ() *time.Location {
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		return time.FixedZone("PST", -8*3600)
	}
	return loc
}

// QuotaStatus summarizes daily Pacific Time YouTube API quota usage.
type QuotaStatus struct {
	PTDate        string `json:"ptDate"`
	UnitsUsed     int    `json:"unitsUsed"`
	DailyCap      int    `json:"dailyCap"`
	DailyCeiling  int    `json:"dailyCeiling"`
	Remaining     int    `json:"remaining"`
	NextResetPT   string `json:"nextResetPt"`
	CanUploadNext bool   `json:"canUploadNext"`
}

// Uploader streams 16 MB private resumable video uploads using the official Go YouTube v3 SDK.
type Uploader struct {
	Store         *db.Store
	Hub           *telemetry.Hub
	HTTPClient    *http.Client
	UploadInitURL string // Default https://www.googleapis.com/upload/youtube/v3/videos?uploadType=resumable&part=snippet,status
	APIEndpoint   string // Optional override for YouTube Data API v3 Service (playlistItems.insert)
	ChunkSize     int64
	Breaker       *adapters.CircuitBreaker
}

// NewUploader initializes the YouTube private video uploader.
func NewUploader(store *db.Store, hub *telemetry.Hub, httpClient *http.Client) *Uploader {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 60 * time.Second}
	}
	return &Uploader{
		Store:         store,
		Hub:           hub,
		HTTPClient:    httpClient,
		UploadInitURL: "https://www.googleapis.com/upload/youtube/v3/videos?uploadType=resumable&part=snippet,status",
		ChunkSize:     DefaultChunkSizeBytes,
		Breaker:       adapters.NewCircuitBreaker(3, 60*time.Second),
	}
}

// TodayPT returns YYYY-MM-DD in America/Los_Angeles.
func TodayPT(now time.Time) string {
	return now.In(pacificTZ).Format("2006-01-02")
}

// QuotaStatus returns current Pacific Time quota usage without polling videos.list.
func (u *Uploader) QuotaStatus(now time.Time) (QuotaStatus, error) {
	ptDate := TodayPT(now)
	used, err := u.Store.GetYouTubeQuota(ptDate)
	if err != nil {
		return QuotaStatus{}, err
	}
	rem := DailyQuotaCapUnits - used
	if rem < 0 {
		rem = 0
	}
	ptNow := now.In(pacificTZ)
	nextMid := time.Date(ptNow.Year(), ptNow.Month(), ptNow.Day()+1, 0, 0, 0, 0, pacificTZ)
	return QuotaStatus{
		PTDate:        ptDate,
		UnitsUsed:     used,
		DailyCap:      DailyQuotaCapUnits,
		DailyCeiling:  10000,
		Remaining:     rem,
		NextResetPT:   nextMid.Format(time.RFC3339),
		CanUploadNext: rem >= UploadCostUnits,
	}, nil
}

// ProcessJob executes or resumes a private resumable YouTube upload job and appends it to playlistID.
func (u *Uploader) ProcessJob(ctx context.Context, job db.Job, title string) (string, error) {
	if !u.Breaker.Allow(time.Now()) {
		return "", errors.New("youtube circuit breaker open")
	}
	f, err := os.Open(job.SourcePath)
	if err != nil {
		return "", err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return "", err
	}
	totalSize := st.Size()
	if totalSize <= 0 {
		return "", errors.New("cannot upload empty video file")
	}
	if title == "" {
		title = strings.TrimSuffix(filepath.Base(job.SourcePath), filepath.Ext(job.SourcePath))
	}

	chunkSize := u.ChunkSize
	if chunkSize <= 0 {
		chunkSize = DefaultChunkSizeBytes
	}

	sessionURI := job.ResumableURI
	var offset int64
	if sessionURI == "" || (job.ResumableExpiresAt > 0 && time.Now().UTC().Unix() >= job.ResumableExpiresAt) {
		// Check and reserve daily PT quota before starting a new resumable session.
		if _, ok, err := u.Store.AddYouTubeQuota(TodayPT(time.Now()), UploadCostUnits, DailyQuotaCapUnits); err != nil {
			return "", err
		} else if !ok {
			_ = u.Store.UpdateJobState(job.ID, "DEFERRED_QUOTA", job.BytesDone, "Daily PT quota cap reached (9,600/10,000); deferred to PT midnight", "", 0)
			return "", errors.New("youtube daily PT quota cap reached")
		}
		sessionURI, err = u.initResumableSession(ctx, title, totalSize)
		if err != nil {
			u.Breaker.RecordFailure(time.Now(), 0)
			return "", err
		}
		expires := time.Now().UTC().Add(6 * 24 * time.Hour).Unix()
		_ = u.Store.UpdateJobState(job.ID, "RUNNING", 0, "", sessionURI, expires)
	} else {
		// Query server for confirmed byte offset before resuming chunks.
		offset, err = u.queryResumeOffset(ctx, sessionURI, totalSize)
		if err != nil {
			offset = job.BytesDone
		}
	}

	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return "", err
	}

	buf := make([]byte, chunkSize)
	var videoID string
	for offset < totalSize {
		n, rerr := io.ReadFull(f, buf)
		if rerr != nil && !errors.Is(rerr, io.EOF) && !errors.Is(rerr, io.ErrUnexpectedEOF) {
			return "", rerr
		}
		if n == 0 {
			break
		}
		end := offset + int64(n) - 1
		req, err := http.NewRequestWithContext(ctx, http.MethodPut, sessionURI, bytes.NewReader(buf[:n]))
		if err != nil {
			return "", err
		}
		req.Header.Set("Content-Type", "video/*")
		req.Header.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", offset, end, totalSize))
		req.ContentLength = int64(n)

		resp, err := u.HTTPClient.Do(req)
		if err != nil {
			u.Breaker.RecordFailure(time.Now(), 0)
			_ = u.Store.UpdateJobState(job.ID, "PAUSED", offset, err.Error(), sessionURI, 0)
			return "", err
		}

		if resp.StatusCode == 308 {
			_ = resp.Body.Close()
			offset = end + 1
			_ = u.Store.UpdateJobState(job.ID, "RUNNING", offset, "", sessionURI, 0)
			if u.Hub != nil {
				u.Hub.UpdateTransfer(telemetry.TransferTick{
					JobID: job.ID, Label: title, BytesDone: offset, BytesTotal: totalSize, Status: "RUNNING",
				})
			}
			continue
		}
		if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated {
			var vidResp ytv3.Video
			_ = json.NewDecoder(resp.Body).Decode(&vidResp)
			_ = resp.Body.Close()
			videoID = vidResp.Id
			offset = totalSize
			break
		}
		wait := adapters.ComputeBackoff(resp, 2)
		_ = resp.Body.Close()
		u.Breaker.RecordFailure(time.Now(), wait)
		return "", fmt.Errorf("youtube chunk upload HTTP %d", resp.StatusCode)
	}

	if videoID != "" && job.TargetPlaylist != "" {
		if err := u.addToPlaylist(ctx, videoID, job.TargetPlaylist); err != nil {
			return videoID, fmt.Errorf("uploaded private video %s, playlist insert failed: %w", videoID, err)
		}
	}

	u.Breaker.RecordSuccess()
	_ = u.Store.UpdateJobState(job.ID, "SYNCED", totalSize, "", sessionURI, 0)
	if u.Hub != nil {
		u.Hub.UpdateTransfer(telemetry.TransferTick{
			JobID: job.ID, Label: title, BytesDone: totalSize, BytesTotal: totalSize, Status: "SYNCED",
		})
	}
	return videoID, nil
}

func (u *Uploader) initResumableSession(ctx context.Context, title string, totalSize int64) (string, error) {
	// Strictly force private visibility on all automated archive uploads.
	meta := map[string]any{
		"snippet": map[string]any{
			"title":       title,
			"description": "Archived automatically as private by Cloud Manager",
		},
		"status": map[string]any{
			"privacyStatus": "private",
		},
	}
	body, _ := json.Marshal(meta)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.UploadInitURL, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json; charset=UTF-8")
	req.Header.Set("X-Upload-Content-Length", strconv.FormatInt(totalSize, 10))
	req.Header.Set("X-Upload-Content-Type", "video/*")

	resp, err := u.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("youtube resumable init HTTP %d", resp.StatusCode)
	}
	loc := resp.Header.Get("Location")
	if loc == "" {
		return "", errors.New("youtube resumable init missing Location header")
	}
	return loc, nil
}

func (u *Uploader) queryResumeOffset(ctx context.Context, sessionURI string, totalSize int64) (int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, sessionURI, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Range", fmt.Sprintf("bytes */%d", totalSize))
	resp, err := u.HTTPClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 308 {
		return 0, fmt.Errorf("unexpected resume probe status %d", resp.StatusCode)
	}
	rng := resp.Header.Get("Range")
	if idx := strings.LastIndexByte(rng, '-'); idx >= 0 {
		lastByte, perr := strconv.ParseInt(rng[idx+1:], 10, 64)
		if perr == nil {
			return lastByte + 1, nil
		}
	}
	return 0, nil
}

func (u *Uploader) addToPlaylist(ctx context.Context, videoID, playlistID string) error {
	opts := []option.ClientOption{option.WithHTTPClient(u.HTTPClient)}
	if u.APIEndpoint != "" {
		opts = append(opts, option.WithEndpoint(u.APIEndpoint), option.WithoutAuthentication())
	}
	svc, err := ytv3.NewService(ctx, opts...)
	if err != nil {
		return err
	}
	_, err = svc.PlaylistItems.Insert([]string{"snippet"}, &ytv3.PlaylistItem{
		Snippet: &ytv3.PlaylistItemSnippet{
			PlaylistId: playlistID,
			ResourceId: &ytv3.ResourceId{
				Kind:    "youtube#video",
				VideoId: videoID,
			},
		},
	}).Do()
	return err
}
