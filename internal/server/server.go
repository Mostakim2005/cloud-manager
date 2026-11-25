package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Mostakim2005/cloud-manager/internal/adapters/adb"
	"github.com/Mostakim2005/cloud-manager/internal/adapters/kopia"
	"github.com/Mostakim2005/cloud-manager/internal/adapters/lanqr"
	"github.com/Mostakim2005/cloud-manager/internal/adapters/rclone"
	"github.com/Mostakim2005/cloud-manager/internal/adapters/syncthing"
	"github.com/Mostakim2005/cloud-manager/internal/adapters/youtube"
	"github.com/Mostakim2005/cloud-manager/internal/config"
	"github.com/Mostakim2005/cloud-manager/internal/db"
	"github.com/Mostakim2005/cloud-manager/internal/doctor"
	"github.com/Mostakim2005/cloud-manager/internal/ingest"
	"github.com/Mostakim2005/cloud-manager/internal/routing"
	"github.com/Mostakim2005/cloud-manager/internal/security"
	"github.com/Mostakim2005/cloud-manager/internal/telemetry"
	"github.com/Mostakim2005/cloud-manager/internal/webui"
)

// Daemon aggregates all Cloud Manager subsystems behind the authenticated 127.0.0.1:59432 server.
type Daemon struct {
	mu         sync.RWMutex
	Paths      config.Paths
	Settings   config.Settings
	Secrets    *config.EcosystemSecrets
	Store      *db.Store
	Hub        *telemetry.Hub
	Logger     *config.Logger
	Tracker    *routing.QuotaReservationTracker
	Pipeline   *ingest.Pipeline
	Rclone     *rclone.Supervisor
	Syncthing  *syncthing.Client
	ADB        *adb.Client
	LANQR      *lanqr.Server
	YouTube    *youtube.Uploader
	Kopia      *kopia.Bridge
	JobSup     *security.JobSupervisor
	UIToken    string
	Port       int
	UIOverride string
	srv        *http.Server
	ln         net.Listener
}

// NewDaemon initializes all components cleanly (ready for HTTP ListenAndServe or httptest).
func NewDaemon(paths config.Paths, settings config.Settings, secrets *config.EcosystemSecrets, store *db.Store, logger *config.Logger) (*Daemon, error) {
	uiToken, err := security.GenerateToken()
	if err != nil {
		return nil, err
	}
	jobSup, _ := security.NewJobSupervisor()
	hub := telemetry.NewHub()
	tracker := routing.NewQuotaReservationTracker()

	d := &Daemon{
		Paths:    paths,
		Settings: settings,
		Secrets:  secrets,
		Store:    store,
		Hub:      hub,
		Logger:   logger,
		Tracker:  tracker,
		JobSup:   jobSup,
		UIToken:  uiToken,
		Port:     config.DefaultPort,
	}

	d.Pipeline = &ingest.Pipeline{
		Store:    store,
		Settings: d.GetSettings,
		Paths:    paths,
		Tracker:  tracker,
		Hub:      hub,
		Logger:   logger,
		Secrets:  secrets,
	}
	d.Rclone = rclone.NewSupervisor("", paths.RcloneConfPath, "", store, jobSup)
	d.Syncthing = syncthing.NewClient("", filepath.Join(paths.DataDir, "syncthing"), jobSup)
	d.ADB = adb.NewClient("", store)
	d.LANQR = lanqr.NewServer(settings.LANPort, paths.StagingDir, d.Pipeline, d.GetSettings)
	d.YouTube = youtube.NewUploader(store, hub, nil)
	d.Kopia = kopia.NewBridge("")
	return d, nil
}

// GetSettings returns a concurrency-safe copy of active settings.
func (d *Daemon) GetSettings() config.Settings {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.Settings
}

// UpdateSettings updates and persists settings atomically without UTF-8 BOM.
func (d *Daemon) UpdateSettings(next config.Settings) error {
	next.SchemaVersion = 1
	if next.StagingDir == "" {
		next.StagingDir = d.Paths.StagingDir
	}
	if err := config.WriteJSONAtomic(d.Paths.SettingsPath, next, 0o600); err != nil {
		return err
	}
	d.mu.Lock()
	d.Settings = next
	d.mu.Unlock()
	d.LANQR.SetAutoRoute(next.AutoRouteLAN)
	return nil
}

// Start binds strictly to 127.0.0.1:<port> (fail closed) and serves HTTP + SSE + /mcp.
func (d *Daemon) Start(bindAddr string) (string, error) {
	if !security.IsLoopbackBind(bindAddr) {
		return "", fmt.Errorf("refusing non-loopback bind address %q (fail closed)", bindAddr)
	}
	ln, err := net.Listen("tcp", bindAddr)
	if err != nil {
		return "", err
	}
	if tcp, ok := ln.Addr().(*net.TCPAddr); ok {
		d.Port = tcp.Port
	}
	d.ln = ln
	d.srv = &http.Server{
		Handler:           d.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		_ = d.srv.Serve(ln)
	}()
	return "http://" + ln.Addr().String(), nil
}

// Shutdown gracefully terminates HTTP servers, supervised children, and Job Object.
func (d *Daemon) Shutdown(ctx context.Context) error {
	if d.srv != nil {
		_ = d.srv.Shutdown(ctx)
	}
	if d.LANQR != nil {
		_ = d.LANQR.Disable(ctx)
	}
	if d.Rclone != nil {
		_ = d.Rclone.Stop(ctx)
	}
	if d.Syncthing != nil {
		_ = d.Syncthing.Stop(ctx)
	}
	if d.JobSup != nil {
		_ = d.JobSup.Close()
	}
	if d.Hub != nil {
		d.Hub.Close()
	}
	if d.Store != nil {
		_ = d.Store.Close()
	}
	return nil
}

// Handler returns the HTTP router with security boundaries enforced.
func (d *Daemon) Handler() http.Handler {
	mux := http.NewServeMux()

	// Static UI shell and SharedLibraries mount (strict Host & cross-Origin rejection).
	uiHandler := webui.FileServer(d.UIOverride)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if !security.ValidateLoopbackHost(r.Host, d.Port) || !security.ValidateOrigin(r.Header.Get("Origin"), d.Port, false) {
			http.Error(w, "Forbidden loopback host/origin", http.StatusForbidden)
			return
		}
		if r.URL.Path == "/" || r.URL.Path == "/index.html" {
			d.serveIndexWithSession(w, r)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/shared-libs/") {
			d.serveSharedLib(w, r)
			return
		}
		uiHandler.ServeHTTP(w, r)
	})

	// Minimal unauthenticated readiness probe (still checks Host + no cross-Origin).
	mux.HandleFunc("/api/v1/health", d.withGuard(false, false, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"status":  "ok",
			"service": "cloud-manager",
			"port":    d.Port,
		})
	}))

	// Machine-to-machine endpoints: strict no-Origin (ANY Origin -> 403) + Bearer auth required.
	mux.HandleFunc("/mcp", d.withGuard(true, true, d.handleMCP))
	mux.HandleFunc("/api/v1/sync", d.withGuard(true, true, d.handleAppBackplaneSync))

	// Authenticated API routes (UI + CLI + scripts).
	mux.HandleFunc("/api/v1/status", d.withGuard(false, true, d.handleStatus))
	mux.HandleFunc("/api/v1/doctor", d.withGuard(false, true, d.handleDoctor))
	mux.HandleFunc("/api/v1/events", d.withGuard(false, true, d.handleSSE))
	mux.HandleFunc("/api/v1/catalog", d.withGuard(false, true, d.handleCatalog))
	mux.HandleFunc("/api/v1/catalog/preview", d.withGuard(false, true, d.handleCatalogPreview))
	mux.HandleFunc("/api/v1/catalog/delete", d.withGuard(false, true, d.handleCatalogDelete))
	mux.HandleFunc("/api/v1/accounts", d.withGuard(false, true, d.handleAccounts))
	mux.HandleFunc("/api/v1/duplicates", d.withGuard(false, true, d.handleDuplicates))
	mux.HandleFunc("/api/v1/tombstones", d.withGuard(false, true, d.handleTombstones))
	mux.HandleFunc("/api/v1/tombstones/resolve", d.withGuard(false, true, d.handleTombstoneResolve))
	mux.HandleFunc("/api/v1/rules", d.withGuard(false, true, d.handleRules))
	mux.HandleFunc("/api/v1/rules/simulate", d.withGuard(false, true, d.handleRuleSimulate))
	mux.HandleFunc("/api/v1/jobs", d.withGuard(false, true, d.handleJobs))
	mux.HandleFunc("/api/v1/jobs/action", d.withGuard(false, true, d.handleJobAction))
	mux.HandleFunc("/api/v1/phone/folders", d.withGuard(false, true, d.handlePhoneFolders))
	mux.HandleFunc("/api/v1/phone/syncthing/action", d.withGuard(false, true, d.handleSyncthingAction))
	mux.HandleFunc("/api/v1/phone/adb/action", d.withGuard(false, true, d.handleADBAction))
	mux.HandleFunc("/api/v1/lan-qr", d.withGuard(false, true, d.handleLANQR))
	mux.HandleFunc("/api/v1/youtube/quota", d.withGuard(false, true, d.handleYouTubeQuota))
	mux.HandleFunc("/api/v1/youtube/queue", d.withGuard(false, true, d.handleYouTubeQueue))
	mux.HandleFunc("/api/v1/app-snapshots", d.withGuard(false, true, d.handleAppSnapshots))
	mux.HandleFunc("/api/v1/settings", d.withGuard(false, true, d.handleSettings))
	mux.HandleFunc("/api/v1/settings/reset", d.withGuard(false, true, d.handleSettingsReset))
	mux.HandleFunc("/api/v1/settings/export", d.withGuard(false, true, d.handleSettingsExport))

	return mux
}

func (d *Daemon) withGuard(strictNoOrigin bool, requireAuth bool, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !security.ValidateLoopbackHost(r.Host, d.Port) {
			http.Error(w, "Invalid loopback Host header", http.StatusForbidden)
			return
		}
		if !security.ValidateOrigin(r.Header.Get("Origin"), d.Port, strictNoOrigin) {
			http.Error(w, "Cross-origin or disallowed Origin header blocked", http.StatusForbidden)
			return
		}
		if requireAuth {
			cand := security.ExtractBearerToken(r)
			var persistent string
			if d.Secrets != nil {
				persistent = d.Secrets.Tokens["cloudManager"]
			}
			if !security.ConstantTimeTokenMatch(cand, persistent, d.UIToken) {
				http.Error(w, "Unauthorized: Bearer token required", http.StatusUnauthorized)
				return
			}
		}
		next(w, r)
	}
}

func (d *Daemon) serveIndexWithSession(w http.ResponseWriter, r *http.Request) {
	var raw []byte
	var err error
	if d.UIOverride != "" {
		raw, err = os.ReadFile(filepath.Join(d.UIOverride, "index.html"))
	} else {
		sub, aerr := webui.Assets()
		if aerr == nil {
			if f, ferr := sub.Open("index.html"); ferr == nil {
				raw, err = io.ReadAll(f)
				_ = f.Close()
			} else {
				err = ferr
			}
		} else {
			err = aerr
		}
	}
	if err != nil {
		raw = []byte(`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="color-scheme" content="light dark"><title>Cloud Manager</title></head><body><h1>Cloud Manager Daemon Active</h1><p>UI assets not built yet. Run Vite build inside web/.</p></body></html>`)
	}

	nonce, _ := security.GenerateToken()
	htmlStr := string(raw)
	metaTag := fmt.Sprintf(`<meta name="cm-session" content="%s">`, d.UIToken)
	if strings.Contains(htmlStr, "</head>") {
		htmlStr = strings.Replace(htmlStr, "<script data-cm-fouc>", fmt.Sprintf(`<script nonce="%s" data-cm-fouc>`, nonce), 1)
		htmlStr = strings.Replace(htmlStr, "</head>", "  "+metaTag+"\n</head>", 1)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", fmt.Sprintf("default-src 'self'; script-src 'self' 'nonce-%s'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; media-src 'self' blob:; connect-src 'self'; frame-ancestors 'none';", nonce))
	_, _ = io.WriteString(w, htmlStr)
}

func (d *Daemon) serveSharedLib(w http.ResponseWriter, r *http.Request) {
	rel := strings.TrimPrefix(r.URL.Path, "/shared-libs/")
	target, err := security.ResolveConfinedPath(d.Paths.SharedLibrariesDir, rel)
	if err != nil {
		http.Error(w, "Forbidden shared asset path", http.StatusForbidden)
		return
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeFile(w, r, target)
}

func (d *Daemon) handleStatus(w http.ResponseWriter, _ *http.Request) {
	stats, _ := d.Store.Stats()
	accounts, _ := d.Store.ListAccounts()
	snap := d.Hub.Snapshot()
	writeJSON(w, http.StatusOK, map[string]any{
		"stats":     stats,
		"accounts":  accounts,
		"telemetry": snap,
		"lanQr":     d.LANQR.Status(),
		"bwCapKbps": routing.BandwidthCapKBps(d.GetSettings(), time.Now()),
	})
}

func (d *Daemon) handleDoctor(w http.ResponseWriter, _ *http.Request) {
	rep := doctor.Run(d.Paths, d.GetSettings(), d.Store)
	writeJSON(w, http.StatusOK, rep)
}

func (d *Daemon) handleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch, unsub := d.Hub.Subscribe()
	defer unsub()

	for {
		select {
		case <-r.Context().Done():
			return
		case snap := <-ch:
			data, err := json.Marshal(snap)
			if err != nil {
				continue
			}
			_, _ = fmt.Fprintf(w, "event: snapshot\ndata: %s\n\n", data)
			flusher.Flush()
		}
	}
}

func (d *Daemon) handleCatalog(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	provider := r.URL.Query().Get("provider")
	status := r.URL.Query().Get("status")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	start := time.Now()
	items, total, err := d.Store.SearchCatalog(q, provider, status, limit, offset)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items":   items,
		"total":   total,
		"queryMs": float64(time.Since(start).Microseconds()) / 1000.0,
	})
}

func (d *Daemon) handleCatalogPreview(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
	if err != nil || id <= 0 {
		writeErr(w, http.StatusBadRequest, "valid id required")
		return
	}
	item, err := d.Store.GetFileByID(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "file not found")
		return
	}
	stagingRoot := d.GetSettings().StagingDir
	if stagingRoot == "" {
		stagingRoot = d.Paths.StagingDir
	}
	safeTarget, err := security.ResolveConfinedPath(stagingRoot, item.RelPath)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"file":        item,
			"previewable": false,
			"reason":      "remote metadata only (not staged locally)",
		})
		return
	}
	if _, serr := os.Stat(safeTarget); serr != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"file":        item,
			"previewable": false,
			"reason":      "staged file not present on disk",
		})
		return
	}
	if r.URL.Query().Get("raw") == "1" {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Disposition", "inline")
		http.ServeFile(w, r, safeTarget)
		return
	}
	var snippet string
	if strings.HasPrefix(item.MimeType, "text/") || strings.HasSuffix(item.Name, ".md") || strings.HasSuffix(item.Name, ".json") {
		if f, oerr := os.Open(safeTarget); oerr == nil {
			buf := make([]byte, 16*1024)
			n, _ := f.Read(buf)
			snippet = string(buf[:n])
			_ = f.Close()
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"file":        item,
		"previewable": true,
		"rawUrl":      fmt.Sprintf("/api/v1/catalog/preview?id=%d&raw=1", id),
		"textSnippet": snippet,
	})
}

func (d *Daemon) handleCatalogDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var body struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ID <= 0 {
		writeErr(w, http.StatusBadRequest, "valid id required")
		return
	}
	tomb, err := d.Store.SoftDeleteFile(body.ID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tombstone": tomb})
}

func (d *Daemon) handleAccounts(w http.ResponseWriter, _ *http.Request) {
	accounts, err := d.Store.ListAccounts()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"accounts": accounts})
}

func (d *Daemon) handleDuplicates(w http.ResponseWriter, _ *http.Request) {
	dups, err := d.Store.ListDuplicates()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"duplicates": dups})
}

func (d *Daemon) handleTombstones(w http.ResponseWriter, _ *http.Request) {
	tombs, err := d.Store.ListTombstones()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tombstones": tombs})
}

func (d *Daemon) handleTombstoneResolve(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var body struct {
		ID         int64  `json:"id"`
		Resolution string `json:"resolution"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := d.Store.ResolveTombstone(body.ID, body.Resolution); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (d *Daemon) handleRules(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		rules, err := d.Store.ListRoutingRules()
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"rules": rules})
	case http.MethodPost:
		var rule db.RoutingRule
		if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid rule json")
			return
		}
		if rule.Name == "" || rule.TargetProvider == "" {
			writeErr(w, http.StatusBadRequest, "name and targetProvider required")
			return
		}
		rule.Enabled = true
		rule.StopOnMatch = true
		id, err := d.Store.InsertRoutingRule(rule)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		rule.ID = id
		writeJSON(w, http.StatusCreated, map[string]any{"rule": rule})
	case http.MethodDelete:
		id, err := strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "valid id required")
			return
		}
		if err := d.Store.DeleteRoutingRule(id); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"deleted": id})
	default:
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (d *Daemon) handleRuleSimulate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var req struct {
		FileName   string `json:"fileName"`
		OriginType string `json:"originType"`
		SizeBytes  int64  `json:"sizeBytes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.FileName == "" {
		writeErr(w, http.StatusBadRequest, "fileName required")
		return
	}
	if req.OriginType == "" {
		req.OriginType = "pc"
	}
	rules, _ := d.Store.ListRoutingRules()
	accounts, _ := d.Store.ListAccounts()
	res := routing.Evaluate(rules, accounts, d.Tracker, d.GetSettings(), req.FileName, req.OriginType, req.SizeBytes, time.Now())
	writeJSON(w, http.StatusOK, res)
}

func (d *Daemon) handleJobs(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		var req db.Job
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.SourcePath == "" {
			writeErr(w, http.StatusBadRequest, "sourcePath required")
			return
		}
		id, err := d.Store.EnqueueJob(req)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		req.ID = id
		writeJSON(w, http.StatusCreated, map[string]any{"job": req})
		return
	}
	jobs, err := d.Store.ListJobs(100)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"jobs": jobs})
}

func (d *Daemon) handleJobAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var req struct {
		ID     int64  `json:"id"`
		Action string `json:"action"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := d.Store.UpdateJobAction(req.ID, req.Action); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (d *Daemon) handlePhoneFolders(w http.ResponseWriter, _ *http.Request) {
	folders, err := d.Store.ListPhoneSyncFolders()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	id, qrSVG, _ := d.Syncthing.PairingQRSVG(context.Background())
	writeJSON(w, http.StatusOK, map[string]any{
		"folders":        folders,
		"syncthingId":    id,
		"syncthingQrSvg": qrSVG,
	})
}

func (d *Daemon) handleSyncthingAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var req struct {
		Action   string `json:"action"` // scan | pause | resume
		FolderID string `json:"folderId"`
		DeviceID string `json:"deviceId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	var err error
	switch req.Action {
	case "scan":
		err = d.Syncthing.TriggerScan(r.Context(), req.FolderID)
	case "pause":
		err = d.Syncthing.PauseDevice(r.Context(), req.DeviceID)
	case "resume":
		err = d.Syncthing.ResumeDevice(r.Context(), req.DeviceID)
	default:
		writeErr(w, http.StatusBadRequest, "unknown syncthing action")
		return
	}
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "offline": true, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (d *Daemon) handleADBAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var req struct {
		Action   string `json:"action"` // devices | delta | pair | qr
		Serial   string `json:"serial"`
		Addr     string `json:"addr"`
		Code     string `json:"code"`
		Service  string `json:"service"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	switch req.Action {
	case "qr":
		svc := req.Service
		if svc == "" {
			svc = "adb-cloud-manager"
		}
		pass := req.Password
		if pass == "" {
			pass = "cm-pair"
		}
		payload, svg, err := adb.WirelessPairingQR(svc, pass)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"payload": payload, "qrSvg": svg})
	case "devices":
		devs, err := d.ADB.ListDevices(r.Context())
		if err != nil {
			writeJSON(w, http.StatusOK, map[string]any{"devices": []adb.Device{}, "offline": true, "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"devices": devs})
	case "delta":
		items, err := d.ADB.QueryMediaDelta(r.Context(), req.Serial)
		if err != nil {
			writeJSON(w, http.StatusOK, map[string]any{"items": []adb.MediaItem{}, "offline": true, "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items})
	case "pair":
		out, err := d.ADB.Pair(r.Context(), req.Addr, req.Code)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"output": out})
	default:
		writeErr(w, http.StatusBadRequest, "unknown adb action")
	}
}

func (d *Daemon) handleLANQR(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		writeJSON(w, http.StatusOK, d.LANQR.Status())
		return
	}
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "GET or POST required")
		return
	}
	var req struct {
		Enabled   *bool `json:"enabled"`
		AutoRoute *bool `json:"autoRoute"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if req.AutoRoute != nil {
		d.LANQR.SetAutoRoute(*req.AutoRoute)
	}
	if req.Enabled != nil {
		if *req.Enabled {
			st, err := d.LANQR.Enable()
			if err != nil {
				writeErr(w, http.StatusInternalServerError, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, st)
			return
		}
		_ = d.LANQR.Disable(r.Context())
	}
	writeJSON(w, http.StatusOK, d.LANQR.Status())
}

func (d *Daemon) handleYouTubeQuota(w http.ResponseWriter, _ *http.Request) {
	qs, err := d.YouTube.QuotaStatus(time.Now())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, qs)
}

func (d *Daemon) handleYouTubeQueue(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var req struct {
		SourcePath string `json:"sourcePath"`
		PlaylistID string `json:"playlistId"`
		DryRun     bool   `json:"dryRun"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.SourcePath == "" {
		writeErr(w, http.StatusBadRequest, "sourcePath required")
		return
	}
	qs, _ := d.YouTube.QuotaStatus(time.Now())
	if req.DryRun {
		writeJSON(w, http.StatusOK, map[string]any{
			"dryRun":        true,
			"privacyStatus": "private",
			"quota":         qs,
			"willDefer":     !qs.CanUploadNext,
		})
		return
	}
	var sz int64
	if st, err := os.Stat(req.SourcePath); err == nil {
		sz = st.Size()
	}
	id, err := d.Store.EnqueueJob(db.Job{
		Kind:           "youtube_upload",
		SourcePath:     req.SourcePath,
		TargetRemote:   "youtube-private",
		TargetPlaylist: req.PlaylistID,
		Status:         "QUEUED",
		Priority:       40,
		BytesTotal:     sz,
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"jobId": id, "privacyStatus": "private", "quota": qs})
}

// handleAppBackplaneSync handles POST /api/v1/sync?app=<slug> supporting raw files,
// live SQLite VACUUM INTO snapshots, and Kopia block-delta app directory snapshots.
func (d *Daemon) handleAppBackplaneSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	appSlug := r.URL.Query().Get("app")
	if !security.ValidateAppSlug(appSlug) {
		writeErr(w, http.StatusBadRequest, "invalid ?app= slug (expected ^[a-z0-9][a-z0-9_-]{0,63}$)")
		return
	}
	var req struct {
		RelPath        string `json:"relPath"`
		SourcePath     string `json:"sourcePath"`
		SQLiteDBPath   string `json:"sqliteDbPath"`
		KopiaDirPath   string `json:"kopiaDirPath"`
		ParentRevision string `json:"parentRevision"`
		Content        string `json:"content"`
		ContentBase64  string `json:"contentBase64"`
		AutoQueue      bool   `json:"autoQueue"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json payload")
		return
	}

	if req.KopiaDirPath != "" {
		snapID, err := d.Kopia.CreateSnapshot(r.Context(), req.KopiaDirPath, map[string]string{"app": appSlug})
		if err != nil {
			writeErr(w, http.StatusServiceUnavailable, err.Error())
			return
		}
		id, _ := d.Store.RecordAppSnapshot(db.AppSnapshot{
			AppSlug:         appSlug,
			SnapshotType:    "kopia",
			KopiaSnapshotID: snapID,
			ParentRevision:  req.ParentRevision,
			RelPath:         req.KopiaDirPath,
		})
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "snapshotId": id, "kopiaSnapshotId": snapID})
		return
	}

	rel := req.RelPath
	if rel == "" {
		rel = "apps/" + appSlug + "/state.bin"
	}
	if _, err := security.SanitizeRelPath(rel); err != nil {
		writeErr(w, http.StatusBadRequest, "unsafe relPath")
		return
	}

	var reader io.Reader
	snapType := "payload"
	if req.SQLiteDBPath != "" {
		snapType = "vacuum_into"
		tmpSnap := filepath.Join(d.Paths.StagingDir, fmt.Sprintf(".vac-%s-%d.sqlite", appSlug, time.Now().UnixNano()))
		if err := db.SnapshotSQLiteFile(req.SQLiteDBPath, tmpSnap); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		defer os.Remove(tmpSnap)
		f, err := os.Open(tmpSnap)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		defer f.Close()
		reader = f
	} else if req.SourcePath != "" {
		f, err := os.Open(req.SourcePath)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		defer f.Close()
		reader = f
	} else if req.ContentBase64 != "" {
		raw, err := base64.StdEncoding.DecodeString(req.ContentBase64)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "invalid base64 content")
			return
		}
		reader = bytes.NewReader(raw)
	} else {
		reader = strings.NewReader(req.Content)
	}

	res, err := d.Pipeline.Ingest(r.Context(), ingest.Request{
		RelPath:        filepath.ToSlash(filepath.Join("apps", appSlug, filepath.Base(rel))),
		SourceReader:   reader,
		OriginType:     "app",
		AppSlug:        appSlug,
		ParentRevision: req.ParentRevision,
		AutoQueue:      req.AutoQueue,
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	_, _ = d.Store.RecordAppSnapshot(db.AppSnapshot{
		AppSlug:        appSlug,
		SnapshotType:   snapType,
		SHA256:         res.SHA256,
		ParentRevision: req.ParentRevision,
		RelPath:        res.RelPath,
		ConflictPath:   res.ConflictPath,
	})
	writeJSON(w, http.StatusOK, res)
}

func (d *Daemon) handleAppSnapshots(w http.ResponseWriter, _ *http.Request) {
	snaps, err := d.Store.ListAppSnapshots(50)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"snapshots": snaps})
}

func (d *Daemon) handleSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, d.GetSettings())
	case http.MethodPut:
		var next config.Settings
		if err := json.NewDecoder(r.Body).Decode(&next); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid settings json")
			return
		}
		if err := d.UpdateSettings(next); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, next)
	default:
		writeErr(w, http.StatusMethodNotAllowed, "GET or PUT required")
	}
}

func (d *Daemon) handleSettingsReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	def := config.DefaultSettings(d.Paths.StagingDir)
	if err := d.UpdateSettings(def); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, def)
}

func (d *Daemon) handleSettingsExport(w http.ResponseWriter, _ *http.Request) {
	rep := doctor.Run(d.Paths, d.GetSettings(), d.Store)
	rules, _ := d.Store.ListRoutingRules()
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="cloud-manager-config-redacted.json"`)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"settings": d.GetSettings(),
		"rules":    rules,
		"doctor":   rep,
	})
}

// handleMCP exposes Streamable HTTP JSON-RPC 2.0 MCP tools on 127.0.0.1:59432/mcp.
func (d *Daemon) handleMCP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var rpc struct {
		JSONRPC string         `json:"jsonrpc"`
		ID      any            `json:"id"`
		Method  string         `json:"method"`
		Params  map[string]any `json:"params"`
	}
	if err := json.NewDecoder(r.Body).Decode(&rpc); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"jsonrpc": "2.0", "id": nil, "error": map[string]any{"code": -32700, "message": "parse error"}})
		return
	}
	reply := func(res any) {
		writeJSON(w, http.StatusOK, map[string]any{"jsonrpc": "2.0", "id": rpc.ID, "result": res})
	}
	switch rpc.Method {
	case "initialize":
		reply(map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "cloud-manager", "version": "0.1.0"},
		})
	case "tools/list":
		reply(map[string]any{"tools": mcpToolDefinitions()})
	case "tools/call":
		name, _ := rpc.Params["name"].(string)
		args, _ := rpc.Params["arguments"].(map[string]any)
		out, err := d.dispatchMCPTool(r.Context(), name, args)
		if err != nil {
			writeJSON(w, http.StatusOK, map[string]any{
				"jsonrpc": "2.0",
				"id":      rpc.ID,
				"result":  map[string]any{"isError": true, "content": []map[string]any{{"type": "text", "text": err.Error()}}},
			})
			return
		}
		txt, _ := json.MarshalIndent(out, "", "  ")
		reply(map[string]any{"content": []map[string]any{{"type": "text", "text": string(txt)}}})
	default:
		writeJSON(w, http.StatusOK, map[string]any{"jsonrpc": "2.0", "id": rpc.ID, "error": map[string]any{"code": -32601, "message": "method not found"}})
	}
}

func mcpToolDefinitions() []map[string]any {
	return []map[string]any{
		{"name": "cloud_status", "description": "Get Cloud Manager accounts, catalog stats, and adapter states"},
		{"name": "cloud_search", "description": "Search the local SQLite FTS5 multi-cloud catalog"},
		{"name": "cloud_route_dry_run", "description": "Simulate priority routing and quota spillover for a file"},
		{"name": "cloud_pool_duplicates", "description": "List identical SHA-256 duplicates across pooled accounts"},
		{"name": "cloud_phone_status", "description": "List Syncthing/ADB sync folders and LAN QR status"},
		{"name": "cloud_youtube_quota", "description": "Get YouTube daily PT quota usage (9,600/10,000 cap)"},
		{"name": "cloud_doctor", "description": "Run redacted system, binary, and registry diagnostics"},
	}
}

func (d *Daemon) dispatchMCPTool(_ context.Context, name string, args map[string]any) (any, error) {
	switch name {
	case "cloud_status":
		stats, _ := d.Store.Stats()
		accounts, _ := d.Store.ListAccounts()
		return map[string]any{"stats": stats, "accounts": accounts}, nil
	case "cloud_search":
		q, _ := args["query"].(string)
		items, total, err := d.Store.SearchCatalog(q, "", "", 50, 0)
		return map[string]any{"items": items, "total": total}, err
	case "cloud_route_dry_run":
		fn, _ := args["fileName"].(string)
		orig, _ := args["originType"].(string)
		szF, _ := args["sizeBytes"].(float64)
		if fn == "" {
			return nil, errors.New("fileName required")
		}
		rules, _ := d.Store.ListRoutingRules()
		accounts, _ := d.Store.ListAccounts()
		return routing.Evaluate(rules, accounts, d.Tracker, d.GetSettings(), fn, orig, int64(szF), time.Now()), nil
	case "cloud_pool_duplicates":
		dups, err := d.Store.ListDuplicates()
		return map[string]any{"duplicates": dups}, err
	case "cloud_phone_status":
		folders, err := d.Store.ListPhoneSyncFolders()
		return map[string]any{"folders": folders, "lanQr": d.LANQR.Status()}, err
	case "cloud_youtube_quota":
		return d.YouTube.QuotaStatus(time.Now())
	case "cloud_doctor":
		return doctor.Run(d.Paths, d.GetSettings(), d.Store), nil
	default:
		return nil, fmt.Errorf("unknown MCP tool %q", name)
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]any{"error": msg})
}

// Unused import safety helper for SHA256 if needed externally.
var _ = sha256.Size
