package ingest

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Mostakim2005/cloud-manager/internal/config"
	"github.com/Mostakim2005/cloud-manager/internal/db"
	"github.com/Mostakim2005/cloud-manager/internal/routing"
	"github.com/Mostakim2005/cloud-manager/internal/security"
	"github.com/Mostakim2005/cloud-manager/internal/telemetry"
)

// Pipeline funnels all local/mobile/backplane files through one SHA-256 + conflict + RAG hook pipeline.
type Pipeline struct {
	Store       *db.Store
	Settings    func() config.Settings
	Paths       config.Paths
	Tracker     *routing.QuotaReservationTracker
	Hub         *telemetry.Hub
	Logger      *config.Logger
	Secrets     *config.EcosystemSecrets
	HTTPClient  *http.Client
	SkipDiskCap bool
}

// Request describes an incoming file entering Cloud Manager.
type Request struct {
	RelPath        string
	SourceReader   io.Reader
	ExpectedSize   int64
	OriginType     string // syncthing | adb | lan-qr | app | recordings | pc
	ProviderHint   string
	RemoteHint     string
	ParentRevision string
	AppSlug        string
	AutoQueue      bool
}

// Result reports canonical ingestion outcome.
type Result struct {
	FileID          int64                    `json:"fileId"`
	SHA256          string                   `json:"sha256"`
	SizeBytes       int64                    `json:"sizeBytes"`
	StagedPath      string                   `json:"stagedPath"`
	RelPath         string                   `json:"relPath"`
	ConflictCreated bool                     `json:"conflictCreated"`
	ConflictPath    string                   `json:"conflictPath,omitempty"`
	DuplicateFound  bool                     `json:"duplicateFound"`
	QueuedJobID     int64                    `json:"queuedJobId,omitempty"`
	Route           routing.SimulationResult `json:"route"`
	RAGNotified     bool                     `json:"ragNotified"`
}

// Ingest streams src to staging using a fixed 128 KB buffer, hashes SHA-256, preserves conflicts,
// registers in SQLite FTS5, enqueues routing dispatch, and pings RAG Manager for Obsidian/GaussBoard.
func (p *Pipeline) Ingest(ctx context.Context, req Request) (*Result, error) {
	cleanRel, err := security.SanitizeRelPath(req.RelPath)
	if err != nil {
		return nil, err
	}
	settings := p.Settings()
	stagingRoot := settings.StagingDir
	if stagingRoot == "" {
		stagingRoot = p.Paths.StagingDir
	}
	if err := os.MkdirAll(stagingRoot, 0o700); err != nil {
		return nil, err
	}
	if !p.SkipDiskCap && req.ExpectedSize > 0 {
		ok, avail, derr := security.HasFreeSpaceHeadroom(stagingRoot, req.ExpectedSize)
		if derr == nil && !ok {
			return nil, fmt.Errorf("insufficient disk headroom in %s (available %d bytes)", stagingRoot, avail)
		}
	}

	// Check for conflict against an existing active catalog entry before overwriting.
	existing, _ := p.Store.GetFileByRelPath(cleanRel)

	// Write first to a unique temp file while computing streaming SHA-256.
	tmpFile, err := os.CreateTemp(stagingRoot, ".ingest-*")
	if err != nil {
		return nil, err
	}
	tmpPath := tmpFile.Name()
	cleanupTmp := true
	defer func() {
		if cleanupTmp {
			_ = tmpFile.Close()
			_ = os.Remove(tmpPath)
		}
	}()

	h := sha256.New()
	buf := make([]byte, 128*1024) // Fixed 128 KB streaming buffer
	written, err := io.CopyBuffer(io.MultiWriter(tmpFile, h), req.SourceReader, buf)
	if err != nil {
		return nil, err
	}
	if err := tmpFile.Sync(); err != nil {
		return nil, err
	}
	if err := tmpFile.Close(); err != nil {
		return nil, err
	}
	shaHex := hex.EncodeToString(h.Sum(nil))

	finalRel := cleanRel
	conflictCreated := false
	conflictPath := ""
	if existing != nil && existing.SHA256 != "" && existing.SHA256 != shaHex &&
		req.ParentRevision != "" && existing.ParentRevision != "" &&
		req.ParentRevision != existing.ParentRevision {
		host, _ := os.Hostname()
		if host == "" {
			host = "pc"
		}
		ext := filepath.Ext(cleanRel)
		stem := strings.TrimSuffix(cleanRel, ext)
		finalRel = fmt.Sprintf("%s.conflict-%s-%s%s", stem, sanitizeHost(host), time.Now().UTC().Format("20060102T150405Z"), ext)
		conflictCreated = true
		conflictPath = finalRel
	}

	destPath, err := security.ResolveConfinedPath(stagingRoot, finalRel)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(destPath), 0o700); err != nil {
		return nil, err
	}
	if err := os.Rename(tmpPath, destPath); err != nil {
		return nil, err
	}
	cleanupTmp = false

	dupFound, _ := p.Store.HasFileHash(shaHex)
	rules, _ := p.Store.ListRoutingRules()
	accounts, _ := p.Store.ListAccounts()
	sim := routing.Evaluate(rules, accounts, p.Tracker, settings, finalRel, req.OriginType, written, time.Now())

	provider := sim.TargetProvider
	if req.ProviderHint != "" {
		provider = req.ProviderHint
	}
	remoteID := sim.TargetRemote
	if req.RemoteHint != "" {
		remoteID = req.RemoteHint
	}
	mtype := mime.TypeByExtension(filepath.Ext(finalRel))
	if mtype == "" {
		mtype = "application/octet-stream"
	}
	rev := req.ParentRevision
	if rev == "" {
		rev = shaHex[:12]
	}

	fileID, err := p.Store.UpsertFile(db.CatalogFile{
		RelPath:        finalRel,
		Name:           filepath.Base(finalRel),
		LocalPath:      destPath,
		SizeBytes:      written,
		SHA256:         shaHex,
		Provider:       provider,
		RemoteID:       remoteID,
		OriginType:     req.OriginType,
		MimeType:       mtype,
		MtimeUTC:       time.Now().UTC().Unix(),
		ParentRevision: rev,
		Status:         "STAGED",
	})
	if err != nil {
		return nil, err
	}

	var jobID int64
	if req.AutoQueue {
		jobKind := "rclone_copy"
		if provider == "youtube" {
			jobKind = "youtube_upload"
		}
		jobID, _ = p.Store.EnqueueJob(db.Job{
			Kind:           jobKind,
			SourcePath:     destPath,
			TargetRemote:   remoteID,
			TargetPlaylist: sim.TargetPlaylist,
			Status:         "QUEUED",
			Priority:       50,
			BytesTotal:     written,
		})
	}

	ragNotified := false
	if isPentagramKnowledgeFile(finalRel, req.AppSlug) {
		ragNotified = p.notifyRAGManager(ctx, finalRel, destPath, shaHex, req.OriginType)
	}

	if p.Hub != nil {
		p.Hub.Emit(telemetry.Event{
			Kind:      "ingest",
			Component: req.OriginType,
			Message:   fmt.Sprintf("Staged %s (%d bytes, sha256=%s)", finalRel, written, shaHex[:8]),
		})
	}
	if p.Logger != nil {
		p.Logger.Log("info", "ingest", "file ingested", map[string]any{
			"relPath":  finalRel,
			"origin":   req.OriginType,
			"size":     written,
			"conflict": conflictCreated,
			"rag":      ragNotified,
		})
	}

	return &Result{
		FileID:          fileID,
		SHA256:          shaHex,
		SizeBytes:       written,
		StagedPath:      destPath,
		RelPath:         finalRel,
		ConflictCreated: conflictCreated,
		ConflictPath:    conflictPath,
		DuplicateFound:  dupFound,
		QueuedJobID:     jobID,
		Route:           sim,
		RAGNotified:     ragNotified,
	}, nil
}

func isPentagramKnowledgeFile(relPath, appSlug string) bool {
	lower := strings.ToLower(relPath)
	if strings.Contains(lower, "obsidian") || strings.Contains(lower, "nihon") ||
		strings.Contains(lower, "gaussboard") || strings.HasSuffix(lower, ".gaussboard") ||
		strings.HasSuffix(lower, ".canvas") {
		return true
	}
	return appSlug == "obsidian" || appSlug == "gaussboard"
}

func (p *Pipeline) notifyRAGManager(ctx context.Context, relPath, localPath, shaHex, origin string) bool {
	settings := p.Settings()
	baseURL := strings.TrimRight(settings.RAGManagerURL, "/")
	if baseURL == "" {
		return false
	}
	payload := map[string]any{
		"source":     "cloud-manager",
		"origin":     origin,
		"rel_path":   relPath,
		"local_path": localPath,
		"sha256":     shaHex,
		"event":      "upsert",
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/v1/ingest/event", bytes.NewReader(body))
	if err != nil {
		return false
	}
	req.Header.Set("Content-Type", "application/json")
	if p.Secrets != nil {
		if tok := p.Secrets.Tokens["ragManager"]; tok != "" {
			req.Header.Set("Authorization", "Bearer "+tok)
		}
	}
	client := p.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 2 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return resp.StatusCode >= 200 && resp.StatusCode < 300
}

func sanitizeHost(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "pc"
	}
	return b.String()
}
