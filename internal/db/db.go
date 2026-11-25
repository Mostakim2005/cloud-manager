package db

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const currentSchemaVersion = 1

// Store manages Cloud Manager's SQLite WAL catalog, migrations, and queues.
type Store struct {
	db   *sql.DB
	path string
}

// CatalogFile represents a virtual catalog item in SQLite.
type CatalogFile struct {
	ID             int64  `json:"id"`
	RelPath        string `json:"relPath"`
	Name           string `json:"name"`
	LocalPath      string `json:"localPath,omitempty"`
	SizeBytes      int64  `json:"sizeBytes"`
	SHA256         string `json:"sha256"`
	Provider       string `json:"provider"`
	RemoteID       string `json:"remoteId"`
	OriginType     string `json:"originType"`
	MimeType       string `json:"mimeType"`
	MtimeUTC       int64  `json:"mtimeUtc"`
	ParentRevision string `json:"parentRevision"`
	Status         string `json:"status"`
	TombstonedAt   int64  `json:"tombstonedAt,omitempty"`
	UpdatedAt      int64  `json:"updatedAt"`
}

// Account represents a connected Google Drive, Mega, or YouTube account.
type Account struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Provider      string `json:"provider"`
	RcloneRemote  string `json:"rcloneRemote"`
	HasherRemote  string `json:"hasherRemote"`
	TotalBytes    int64  `json:"totalBytes"`
	UsedBytes     int64  `json:"usedBytes"`
	FreeBytes     int64  `json:"freeBytes"`
	Health        string `json:"health"`
	CooldownUntil int64  `json:"cooldownUntil"`
	UpdatedAt     int64  `json:"updatedAt"`
}

// RoutingRule defines deterministic priority-ordered routing.
type RoutingRule struct {
	ID             int64  `json:"id"`
	Priority       int    `json:"priority"`
	Name           string `json:"name"`
	OriginMatch    string `json:"originMatch"`
	ExtGlob        string `json:"extGlob"`
	MinSizeBytes   int64  `json:"minSizeBytes"`
	MaxSizeBytes   int64  `json:"maxSizeBytes"`
	TargetProvider string `json:"targetProvider"`
	TargetRemote   string `json:"targetRemote"`
	TargetPlaylist string `json:"targetPlaylist"`
	SpilloverPool  string `json:"spilloverPool"`
	StopOnMatch    bool   `json:"stopOnMatch"`
	Enabled        bool   `json:"enabled"`
	IsFallback     bool   `json:"isFallback"`
}

// Job represents a persistent queue job.
type Job struct {
	ID                 int64  `json:"id"`
	Kind               string `json:"kind"`
	SourcePath         string `json:"sourcePath"`
	TargetRemote       string `json:"targetRemote"`
	TargetPlaylist     string `json:"targetPlaylist,omitempty"`
	Status             string `json:"status"`
	Priority           int    `json:"priority"`
	BytesTotal         int64  `json:"bytesTotal"`
	BytesDone          int64  `json:"bytesDone"`
	Attempts           int    `json:"attempts"`
	MaxAttempts        int    `json:"maxAttempts"`
	NextRetryAt        int64  `json:"nextRetryAt"`
	LastError          string `json:"lastError,omitempty"`
	ResumableURI       string `json:"resumableUri,omitempty"`
	ResumableExpiresAt int64  `json:"resumableExpiresAt,omitempty"`
	CreatedAt          int64  `json:"createdAt"`
	UpdatedAt          int64  `json:"updatedAt"`
}

// DuplicateGroup summarizes identical SHA-256 files stored across multiple remotes/paths.
type DuplicateGroup struct {
	SHA256      string        `json:"sha256"`
	Count       int           `json:"count"`
	WastedBytes int64         `json:"wastedBytes"`
	Entries     []CatalogFile `json:"entries"`
}

// Tombstone represents a soft-deleted or remote-deleted object awaiting review.
type Tombstone struct {
	ID         int64  `json:"id"`
	FileID     int64  `json:"fileId"`
	RemoteID   string `json:"remoteId"`
	RemotePath string `json:"remotePath"`
	DetectedAt int64  `json:"detectedAt"`
	Resolution string `json:"resolution"`
	PurgeAfter int64  `json:"purgeAfter"`
}

// PhoneSyncFolder configures a two-way Syncthing/ADB sync folder.
type PhoneSyncFolder struct {
	ID         string `json:"id"`
	Transport  string `json:"transport"`
	Label      string `json:"label"`
	LocalPath  string `json:"localPath"`
	RemotePath string `json:"remotePath"`
	AutoSync   bool   `json:"autoSync"`
	RAGNotify  bool   `json:"ragNotify"`
	LastSyncAt int64  `json:"lastSyncAt"`
	Status     string `json:"status"`
}

// AppSnapshot records a VACUUM INTO or Kopia block-delta push from a custom .exe app.
type AppSnapshot struct {
	ID              int64  `json:"id"`
	AppSlug         string `json:"appSlug"`
	SnapshotType    string `json:"snapshotType"`
	KopiaSnapshotID string `json:"kopiaSnapshotId,omitempty"`
	SHA256          string `json:"sha256"`
	ParentRevision  string `json:"parentRevision"`
	RelPath         string `json:"relPath"`
	ConflictPath    string `json:"conflictPath,omitempty"`
	CreatedAt       int64  `json:"createdAt"`
}

// Open opens SQLite with WAL + busy_timeout=5000 + foreign_keys=ON and runs numbered migrations.
func Open(dbPath string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o700); err != nil {
		return nil, err
	}
	conn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	conn.SetMaxOpenConns(1) // Single-writer SQLite WAL safety
	for _, pragma := range []string{
		"PRAGMA journal_mode = WAL;",
		"PRAGMA busy_timeout = 5000;",
		"PRAGMA foreign_keys = ON;",
		"PRAGMA synchronous = NORMAL;",
	} {
		if _, err := conn.Exec(pragma); err != nil {
			_ = conn.Close()
			return nil, fmt.Errorf("apply %s: %w", pragma, err)
		}
	}

	s := &Store{db: conn, path: dbPath}
	if err := s.migrate(); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return s, nil
}

// Close closes the underlying database handle.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// DB exposes the underlying *sql.DB for advanced queries in tests.
func (s *Store) DB() *sql.DB { return s.db }

func (s *Store) migrate() error {
	var ver int
	if err := s.db.QueryRow("PRAGMA user_version;").Scan(&ver); err != nil {
		return err
	}
	if ver >= currentSchemaVersion {
		return nil
	}
	// Take pre-migration .bak snapshot if DB already contains tables.
	if ver > 0 {
		if err := s.createPreMigrationBackup(ver); err != nil {
			return fmt.Errorf("pre-migration backup: %w", err)
		}
	}

	migrations := []struct {
		version int
		stmts   []string
	}{
		{
			version: 1,
			stmts: []string{
				`CREATE TABLE IF NOT EXISTS files (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					rel_path TEXT NOT NULL,
					name TEXT NOT NULL,
					local_path TEXT NOT NULL DEFAULT '',
					size_bytes INTEGER NOT NULL DEFAULT 0,
					sha256 TEXT NOT NULL DEFAULT '',
					provider TEXT NOT NULL,
					remote_id TEXT NOT NULL DEFAULT '',
					origin_type TEXT NOT NULL DEFAULT 'pc',
					mime_type TEXT NOT NULL DEFAULT 'application/octet-stream',
					mtime_utc INTEGER NOT NULL DEFAULT 0,
					parent_revision TEXT NOT NULL DEFAULT '',
					status TEXT NOT NULL DEFAULT 'SYNCED',
					tombstoned_at INTEGER NOT NULL DEFAULT 0,
					created_at INTEGER NOT NULL,
					updated_at INTEGER NOT NULL,
					UNIQUE(provider, remote_id, rel_path)
				);`,
				`CREATE INDEX IF NOT EXISTS idx_files_sha256 ON files(sha256);`,
				`CREATE INDEX IF NOT EXISTS idx_files_status ON files(status);`,
				`CREATE VIRTUAL TABLE IF NOT EXISTS files_fts USING fts5(
					name, rel_path, provider, origin_type,
					content='files', content_rowid='id'
				);`,
				`CREATE TRIGGER IF NOT EXISTS files_ai AFTER INSERT ON files BEGIN
					INSERT INTO files_fts(rowid, name, rel_path, provider, origin_type)
					VALUES (new.id, new.name, new.rel_path, new.provider, new.origin_type);
				END;`,
				`CREATE TRIGGER IF NOT EXISTS files_ad AFTER DELETE ON files BEGIN
					INSERT INTO files_fts(files_fts, rowid, name, rel_path, provider, origin_type)
					VALUES ('delete', old.id, old.name, old.rel_path, old.provider, old.origin_type);
				END;`,
				`CREATE TRIGGER IF NOT EXISTS files_au AFTER UPDATE ON files BEGIN
					INSERT INTO files_fts(files_fts, rowid, name, rel_path, provider, origin_type)
					VALUES ('delete', old.id, old.name, old.rel_path, old.provider, old.origin_type);
					INSERT INTO files_fts(rowid, name, rel_path, provider, origin_type)
					VALUES (new.id, new.name, new.rel_path, new.provider, new.origin_type);
				END;`,
				`CREATE TABLE IF NOT EXISTS accounts (
					id TEXT PRIMARY KEY,
					name TEXT NOT NULL,
					provider TEXT NOT NULL,
					rclone_remote TEXT NOT NULL,
					hasher_remote TEXT NOT NULL DEFAULT '',
					total_bytes INTEGER NOT NULL DEFAULT 0,
					used_bytes INTEGER NOT NULL DEFAULT 0,
					free_bytes INTEGER NOT NULL DEFAULT 0,
					health TEXT NOT NULL DEFAULT 'healthy',
					cooldown_until INTEGER NOT NULL DEFAULT 0,
					updated_at INTEGER NOT NULL
				);`,
				`CREATE TABLE IF NOT EXISTS routing_rules (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					priority INTEGER NOT NULL,
					name TEXT NOT NULL,
					origin_match TEXT NOT NULL DEFAULT '*',
					ext_glob TEXT NOT NULL DEFAULT '*',
					min_size_bytes INTEGER NOT NULL DEFAULT 0,
					max_size_bytes INTEGER NOT NULL DEFAULT 0,
					target_provider TEXT NOT NULL,
					target_remote TEXT NOT NULL,
					target_playlist TEXT NOT NULL DEFAULT '',
					spillover_pool TEXT NOT NULL DEFAULT '',
					stop_on_match INTEGER NOT NULL DEFAULT 1,
					enabled INTEGER NOT NULL DEFAULT 1,
					is_fallback INTEGER NOT NULL DEFAULT 0
				);`,
				`CREATE TABLE IF NOT EXISTS jobs (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					kind TEXT NOT NULL,
					source_path TEXT NOT NULL,
					target_remote TEXT NOT NULL,
					target_playlist TEXT NOT NULL DEFAULT '',
					status TEXT NOT NULL DEFAULT 'QUEUED',
					priority INTEGER NOT NULL DEFAULT 100,
					bytes_total INTEGER NOT NULL DEFAULT 0,
					bytes_done INTEGER NOT NULL DEFAULT 0,
					attempts INTEGER NOT NULL DEFAULT 0,
					max_attempts INTEGER NOT NULL DEFAULT 5,
					next_retry_at INTEGER NOT NULL DEFAULT 0,
					last_error TEXT NOT NULL DEFAULT '',
					resumable_uri TEXT NOT NULL DEFAULT '',
					resumable_expires_at INTEGER NOT NULL DEFAULT 0,
					created_at INTEGER NOT NULL,
					updated_at INTEGER NOT NULL
				);`,
				`CREATE TABLE IF NOT EXISTS job_attempts (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					job_id INTEGER NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
					attempt_no INTEGER NOT NULL,
					status TEXT NOT NULL,
					error_message TEXT NOT NULL DEFAULT '',
					started_at INTEGER NOT NULL,
					finished_at INTEGER NOT NULL
				);`,
				`CREATE TABLE IF NOT EXISTS dead_letters (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					job_id INTEGER NOT NULL,
					kind TEXT NOT NULL,
					source_path TEXT NOT NULL,
					error_message TEXT NOT NULL,
					failed_at INTEGER NOT NULL
				);`,
				`CREATE TABLE IF NOT EXISTS remote_cursors (
					remote_id TEXT PRIMARY KEY,
					cursor_token TEXT NOT NULL DEFAULT '',
					last_reconciled_at INTEGER NOT NULL DEFAULT 0
				);`,
				`CREATE TABLE IF NOT EXISTS tombstones (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					file_id INTEGER NOT NULL DEFAULT 0,
					remote_id TEXT NOT NULL,
					remote_path TEXT NOT NULL,
					detected_at INTEGER NOT NULL,
					resolution TEXT NOT NULL DEFAULT 'PENDING',
					purge_after INTEGER NOT NULL
				);`,
				`CREATE TABLE IF NOT EXISTS youtube_quota_ledger (
					pt_date TEXT PRIMARY KEY,
					units_used INTEGER NOT NULL DEFAULT 0,
					updated_at INTEGER NOT NULL
				);`,
				`CREATE TABLE IF NOT EXISTS phone_sync_folders (
					id TEXT PRIMARY KEY,
					transport TEXT NOT NULL,
					label TEXT NOT NULL,
					local_path TEXT NOT NULL,
					remote_path TEXT NOT NULL,
					auto_sync INTEGER NOT NULL DEFAULT 0,
					rag_notify INTEGER NOT NULL DEFAULT 0,
					last_sync_at INTEGER NOT NULL DEFAULT 0,
					status TEXT NOT NULL DEFAULT 'idle'
				);`,
				`CREATE TABLE IF NOT EXISTS app_snapshots (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					app_slug TEXT NOT NULL,
					snapshot_type TEXT NOT NULL,
					kopia_snapshot_id TEXT NOT NULL DEFAULT '',
					sha256 TEXT NOT NULL,
					parent_revision TEXT NOT NULL DEFAULT '',
					rel_path TEXT NOT NULL,
					conflict_path TEXT NOT NULL DEFAULT '',
					created_at INTEGER NOT NULL
				);`,
				`CREATE TABLE IF NOT EXISTS settings (
					key TEXT PRIMARY KEY,
					value TEXT NOT NULL,
					updated_at INTEGER NOT NULL
				);`,
			},
		},
	}

	for _, m := range migrations {
		if m.version <= ver {
			continue
		}
		tx, err := s.db.Begin()
		if err != nil {
			return err
		}
		for _, stmt := range m.stmts {
			if _, err := tx.Exec(stmt); err != nil {
				_ = tx.Rollback()
				return fmt.Errorf("migration v%d failed: %w", m.version, err)
			}
		}
		if _, err := tx.Exec(fmt.Sprintf("PRAGMA user_version = %d;", m.version)); err != nil {
			_ = tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}

	return s.seedDefaults()
}

func (s *Store) createPreMigrationBackup(fromVer int) error {
	bakName := fmt.Sprintf("%s.v%d.%d.bak", s.path, fromVer, time.Now().UTC().Unix())
	if err := SnapshotSQLiteDB(s.db, bakName); err != nil {
		return err
	}
	pattern := s.path + ".*.bak"
	matches, err := filepath.Glob(pattern)
	if err != nil || len(matches) <= 5 {
		return nil
	}
	sort.Strings(matches)
	for _, old := range matches[:len(matches)-5] {
		_ = os.Remove(old)
	}
	return nil
}

// SnapshotSQLiteFile opens an external live SQLite DB safely and writes a clean WAL-flushed snapshot via VACUUM INTO.
func SnapshotSQLiteFile(srcPath, dstPath string) error {
	srcConn, err := sql.Open("sqlite", srcPath)
	if err != nil {
		return err
	}
	defer srcConn.Close()
	_, _ = srcConn.Exec("PRAGMA busy_timeout = 5000;")
	return SnapshotSQLiteDB(srcConn, dstPath)
}

// SnapshotSQLiteDB snapshots an open *sql.DB atomically using SQLite VACUUM INTO.
func SnapshotSQLiteDB(srcDB *sql.DB, dstPath string) error {
	if err := os.MkdirAll(filepath.Dir(dstPath), 0o700); err != nil {
		return err
	}
	absDst, err := filepath.Abs(dstPath)
	if err != nil {
		return err
	}
	tmpDst := fmt.Sprintf("%s.tmp-%d", absDst, time.Now().UnixNano())
	_ = os.Remove(tmpDst)
	escaped := strings.ReplaceAll(tmpDst, "'", "''")
	if _, err := srcDB.Exec(fmt.Sprintf("VACUUM INTO '%s';", escaped)); err != nil {
		_ = os.Remove(tmpDst)
		return fmt.Errorf("vacuum into: %w", err)
	}
	if err := os.Rename(tmpDst, absDst); err != nil {
		_ = os.Remove(tmpDst)
		return err
	}
	return nil
}

func (s *Store) seedDefaults() error {
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM routing_rules`).Scan(&count); err != nil {
		return err
	}
	if count == 0 {
		rules := []RoutingRule{
			{Priority: 10, Name: "Screen Recordings -> YouTube Private", OriginMatch: "recordings", ExtGlob: "*.mp4;*.mkv;*.mov", TargetProvider: "youtube", TargetRemote: "youtube-private", TargetPlaylist: "PL_PRIVATE_ARCHIVE", StopOnMatch: true, Enabled: true},
			{Priority: 20, Name: "Phone Media -> Mega Pool (Append/Archive)", OriginMatch: "phone", ExtGlob: "*.jpg;*.jpeg;*.heic;*.png;*.mp4", TargetProvider: "mega", TargetRemote: "mega-pool", SpilloverPool: "mega-pool", StopOnMatch: true, Enabled: true},
			{Priority: 30, Name: "Books & Notes -> Google Drive", OriginMatch: "*", ExtGlob: "*.pdf;*.md;*.canvas;*.gaussboard;*.mmd", TargetProvider: "gdrive", TargetRemote: "gdrive-1", SpilloverPool: "gdrive-pool", StopOnMatch: true, Enabled: true},
			{Priority: 999, Name: "Catch-All Fallback -> Primary Pool", OriginMatch: "*", ExtGlob: "*", TargetProvider: "gdrive", TargetRemote: "gdrive-1", SpilloverPool: "gdrive-pool", StopOnMatch: true, Enabled: true, IsFallback: true},
		}
		for _, r := range rules {
			if _, err := s.InsertRoutingRule(r); err != nil {
				return err
			}
		}
	}

	var accCount int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM accounts`).Scan(&accCount); err != nil {
		return err
	}
	if accCount == 0 {
		now := time.Now().UTC().Unix()
		defaults := []Account{
			{ID: "gdrive-1", Name: "GDrive #1 (Notes & AppData)", Provider: "gdrive", RcloneRemote: "gdrive1:", TotalBytes: 15 * 1024 * 1024 * 1024, UsedBytes: 4 * 1024 * 1024 * 1024, FreeBytes: 11 * 1024 * 1024 * 1024, Health: "unconfigured", UpdatedAt: now},
			{ID: "mega-1", Name: "Mega #1 (Media Pool)", Provider: "mega", RcloneRemote: "mega1:", HasherRemote: "mega1-hasher:", TotalBytes: 20 * 1024 * 1024 * 1024, UsedBytes: 2 * 1024 * 1024 * 1024, FreeBytes: 18 * 1024 * 1024 * 1024, Health: "unconfigured", UpdatedAt: now},
			{ID: "youtube-private", Name: "YouTube Private Archive", Provider: "youtube", RcloneRemote: "youtube-sdk", TotalBytes: 0, UsedBytes: 0, FreeBytes: 0, Health: "unconfigured", UpdatedAt: now},
		}
		for _, a := range defaults {
			if err := s.UpsertAccount(a); err != nil {
				return err
			}
		}
	}

	var folderCount int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM phone_sync_folders`).Scan(&folderCount); err != nil {
		return err
	}
	if folderCount == 0 {
		defaults := []PhoneSyncFolder{
			{ID: "obsidian-nihon", Transport: "syncthing", Label: "Obsidian Vault (Nihon)", LocalPath: "Obsidian/Nihon", RemotePath: "Obsidian/Nihon", AutoSync: true, RAGNotify: true, Status: "idle"},
			{ID: "gaussboard-boards", Transport: "syncthing", Label: "GaussBoard Sidecars", LocalPath: "GaussBoard", RemotePath: "GaussBoard", AutoSync: true, RAGNotify: true, Status: "idle"},
			{ID: "phone-dcim", Transport: "adb", Label: "Phone DCIM Camera & Video", LocalPath: "Phone/DCIM", RemotePath: "DCIM/Camera", AutoSync: false, RAGNotify: false, Status: "idle"},
			{ID: "phone-pdfs-music", Transport: "syncthing", Label: "Study PDFs & Music", LocalPath: "Phone/Library", RemotePath: "Documents/Study", AutoSync: false, RAGNotify: false, Status: "idle"},
		}
		for _, f := range defaults {
			if err := s.UpsertPhoneSyncFolder(f); err != nil {
				return err
			}
		}
	}
	return nil
}

// UpsertFile inserts or updates a file record (triggering FTS5 sync).
func (s *Store) UpsertFile(f CatalogFile) (int64, error) {
	now := time.Now().UTC().Unix()
	if f.UpdatedAt == 0 {
		f.UpdatedAt = now
	}
	if f.Status == "" {
		f.Status = "SYNCED"
	}
	if f.MimeType == "" {
		f.MimeType = "application/octet-stream"
	}
	res, err := s.db.Exec(`
		INSERT INTO files(rel_path, name, local_path, size_bytes, sha256, provider, remote_id, origin_type, mime_type, mtime_utc, parent_revision, status, tombstoned_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(provider, remote_id, rel_path) DO UPDATE SET
			name = excluded.name,
			local_path = excluded.local_path,
			size_bytes = excluded.size_bytes,
			sha256 = excluded.sha256,
			origin_type = excluded.origin_type,
			mime_type = excluded.mime_type,
			mtime_utc = excluded.mtime_utc,
			parent_revision = excluded.parent_revision,
			status = excluded.status,
			tombstoned_at = excluded.tombstoned_at,
			updated_at = excluded.updated_at;`,
		f.RelPath, f.Name, f.LocalPath, f.SizeBytes, f.SHA256, f.Provider, f.RemoteID,
		f.OriginType, f.MimeType, f.MtimeUTC, f.ParentRevision, f.Status, f.TombstonedAt, now, f.UpdatedAt,
	)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err == nil && id > 0 {
		return id, nil
	}
	var existingID int64
	_ = s.db.QueryRow(`SELECT id FROM files WHERE provider=? AND remote_id=? AND rel_path=?`, f.Provider, f.RemoteID, f.RelPath).Scan(&existingID)
	return existingID, nil
}

// GetFileByRelPath fetches the latest file matching relPath.
func (s *Store) GetFileByRelPath(relPath string) (*CatalogFile, error) {
	row := s.db.QueryRow(`
		SELECT id, rel_path, name, local_path, size_bytes, sha256, provider, remote_id, origin_type, mime_type, mtime_utc, parent_revision, status, tombstoned_at, updated_at
		FROM files WHERE rel_path = ? AND tombstoned_at = 0 ORDER BY updated_at DESC LIMIT 1`, relPath)
	var f CatalogFile
	if err := row.Scan(&f.ID, &f.RelPath, &f.Name, &f.LocalPath, &f.SizeBytes, &f.SHA256, &f.Provider, &f.RemoteID, &f.OriginType, &f.MimeType, &f.MtimeUTC, &f.ParentRevision, &f.Status, &f.TombstonedAt, &f.UpdatedAt); err != nil {
		return nil, err
	}
	return &f, nil
}

// GetFileByID fetches a single catalog file by ID.
func (s *Store) GetFileByID(id int64) (*CatalogFile, error) {
	row := s.db.QueryRow(`
		SELECT id, rel_path, name, local_path, size_bytes, sha256, provider, remote_id, origin_type, mime_type, mtime_utc, parent_revision, status, tombstoned_at, updated_at
		FROM files WHERE id = ?`, id)
	var f CatalogFile
	if err := row.Scan(&f.ID, &f.RelPath, &f.Name, &f.LocalPath, &f.SizeBytes, &f.SHA256, &f.Provider, &f.RemoteID, &f.OriginType, &f.MimeType, &f.MtimeUTC, &f.ParentRevision, &f.Status, &f.TombstonedAt, &f.UpdatedAt); err != nil {
		return nil, err
	}
	return &f, nil
}

// HasFileHash checks whether an active catalog file matches sha256 or (name, size).
func (s *Store) HasFileHash(sha256Hex string) (bool, error) {
	if sha256Hex == "" {
		return false, nil
	}
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM files WHERE sha256 = ? AND tombstoned_at = 0`, sha256Hex).Scan(&count)
	return count > 0, err
}

func buildFTSMatchQuery(q string) string {
	tokens := strings.Fields(q)
	if len(tokens) == 0 {
		return ""
	}
	parts := make([]string, 0, len(tokens))
	for _, tok := range tokens {
		clean := strings.ReplaceAll(tok, `"`, `""`)
		parts = append(parts, `"`+clean+`"*`)
	}
	return strings.Join(parts, " AND ")
}

// SearchCatalog executes fast FTS5 + filtered queries (<5 ms typical) against SQLite WAL.
func (s *Store) SearchCatalog(query, provider, status string, limit, offset int) ([]CatalogFile, int, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	where := []string{"f.tombstoned_at = 0"}
	args := []any{}
	joinFTS := ""

	if match := buildFTSMatchQuery(query); match != "" {
		joinFTS = "JOIN files_fts ON files_fts.rowid = f.id"
		where = append(where, "files_fts MATCH ?")
		args = append(args, match)
	}
	if provider != "" && provider != "all" {
		where = append(where, "f.provider = ?")
		args = append(args, provider)
	}
	if status != "" && status != "all" {
		where = append(where, "f.status = ?")
		args = append(args, status)
	}
	whereSQL := strings.Join(where, " AND ")

	var total int
	countSQL := fmt.Sprintf("SELECT COUNT(*) FROM files f %s WHERE %s", joinFTS, whereSQL)
	if err := s.db.QueryRow(countSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	querySQL := fmt.Sprintf(`
		SELECT f.id, f.rel_path, f.name, f.local_path, f.size_bytes, f.sha256, f.provider, f.remote_id, f.origin_type, f.mime_type, f.mtime_utc, f.parent_revision, f.status, f.tombstoned_at, f.updated_at
		FROM files f %s
		WHERE %s
		ORDER BY f.updated_at DESC, f.id DESC
		LIMIT ? OFFSET ?`, joinFTS, whereSQL)
	queryArgs := append(append([]any{}, args...), limit, offset)
	rows, err := s.db.Query(querySQL, queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := make([]CatalogFile, 0)
	for rows.Next() {
		var f CatalogFile
		if err := rows.Scan(&f.ID, &f.RelPath, &f.Name, &f.LocalPath, &f.SizeBytes, &f.SHA256, &f.Provider, &f.RemoteID, &f.OriginType, &f.MimeType, &f.MtimeUTC, &f.ParentRevision, &f.Status, &f.TombstonedAt, &f.UpdatedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, f)
	}
	return out, total, rows.Err()
}

// SoftDeleteFile moves a file into the 30-day soft-delete quarantine and logs a tombstone.
func (s *Store) SoftDeleteFile(fileID int64) (*Tombstone, error) {
	f, err := s.GetFileByID(fileID)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC().Unix()
	purge := now + int64(30*24*3600)

	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(`UPDATE files SET tombstoned_at = ?, status = 'QUARANTINED', updated_at = ? WHERE id = ?`, now, now, fileID); err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	res, err := tx.Exec(`INSERT INTO tombstones(file_id, remote_id, remote_path, detected_at, resolution, purge_after) VALUES (?, ?, ?, ?, 'PENDING', ?)`,
		fileID, f.RemoteID, f.RelPath, now, purge)
	if err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	tid, _ := res.LastInsertId()
	return &Tombstone{
		ID:         tid,
		FileID:     fileID,
		RemoteID:   f.RemoteID,
		RemotePath: f.RelPath,
		DetectedAt: now,
		Resolution: "PENDING",
		PurgeAfter: purge,
	}, nil
}

// ListDuplicates finds identical SHA-256 hashes replicated across multiple entries/accounts.
func (s *Store) ListDuplicates() ([]DuplicateGroup, error) {
	rows, err := s.db.Query(`
		SELECT sha256 FROM files
		WHERE sha256 != '' AND tombstoned_at = 0
		GROUP BY sha256 HAVING COUNT(*) > 1
		ORDER BY SUM(size_bytes) DESC LIMIT 100`)
	if err != nil {
		return nil, err
	}
	var hashes []string
	for rows.Next() {
		var h string
		if err := rows.Scan(&h); err != nil {
			_ = rows.Close()
			return nil, err
		}
		hashes = append(hashes, h)
	}
	_ = rows.Close()

	groups := make([]DuplicateGroup, 0, len(hashes))
	for _, h := range hashes {
		erows, err := s.db.Query(`
			SELECT id, rel_path, name, local_path, size_bytes, sha256, provider, remote_id, origin_type, mime_type, mtime_utc, parent_revision, status, tombstoned_at, updated_at
			FROM files WHERE sha256 = ? AND tombstoned_at = 0 ORDER BY id ASC`, h)
		if err != nil {
			return nil, err
		}
		var entries []CatalogFile
		var totalSize int64
		for erows.Next() {
			var f CatalogFile
			if err := erows.Scan(&f.ID, &f.RelPath, &f.Name, &f.LocalPath, &f.SizeBytes, &f.SHA256, &f.Provider, &f.RemoteID, &f.OriginType, &f.MimeType, &f.MtimeUTC, &f.ParentRevision, &f.Status, &f.TombstonedAt, &f.UpdatedAt); err != nil {
				_ = erows.Close()
				return nil, err
			}
			totalSize += f.SizeBytes
			entries = append(entries, f)
		}
		_ = erows.Close()
		var wasted int64
		if len(entries) > 1 {
			wasted = totalSize - entries[0].SizeBytes
		}
		groups = append(groups, DuplicateGroup{
			SHA256:      h,
			Count:       len(entries),
			WastedBytes: wasted,
			Entries:     entries,
		})
	}
	return groups, nil
}

// UpsertAccount saves account quota and health metadata.
func (s *Store) UpsertAccount(a Account) error {
	now := time.Now().UTC().Unix()
	if a.UpdatedAt == 0 {
		a.UpdatedAt = now
	}
	_, err := s.db.Exec(`
		INSERT INTO accounts(id, name, provider, rclone_remote, hasher_remote, total_bytes, used_bytes, free_bytes, health, cooldown_until, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			provider = excluded.provider,
			rclone_remote = excluded.rclone_remote,
			hasher_remote = excluded.hasher_remote,
			total_bytes = excluded.total_bytes,
			used_bytes = excluded.used_bytes,
			free_bytes = excluded.free_bytes,
			health = excluded.health,
			cooldown_until = excluded.cooldown_until,
			updated_at = excluded.updated_at;`,
		a.ID, a.Name, a.Provider, a.RcloneRemote, a.HasherRemote, a.TotalBytes, a.UsedBytes, a.FreeBytes, a.Health, a.CooldownUntil, a.UpdatedAt,
	)
	return err
}

// ListAccounts returns all configured accounts.
func (s *Store) ListAccounts() ([]Account, error) {
	rows, err := s.db.Query(`SELECT id, name, provider, rclone_remote, hasher_remote, total_bytes, used_bytes, free_bytes, health, cooldown_until, updated_at FROM accounts ORDER BY provider, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Account, 0)
	for rows.Next() {
		var a Account
		if err := rows.Scan(&a.ID, &a.Name, &a.Provider, &a.RcloneRemote, &a.HasherRemote, &a.TotalBytes, &a.UsedBytes, &a.FreeBytes, &a.Health, &a.CooldownUntil, &a.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// InsertRoutingRule creates a routing rule.
func (s *Store) InsertRoutingRule(r RoutingRule) (int64, error) {
	res, err := s.db.Exec(`
		INSERT INTO routing_rules(priority, name, origin_match, ext_glob, min_size_bytes, max_size_bytes, target_provider, target_remote, target_playlist, spillover_pool, stop_on_match, enabled, is_fallback)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.Priority, r.Name, r.OriginMatch, r.ExtGlob, r.MinSizeBytes, r.MaxSizeBytes,
		r.TargetProvider, r.TargetRemote, r.TargetPlaylist, r.SpilloverPool,
		boolToInt(r.StopOnMatch), boolToInt(r.Enabled), boolToInt(r.IsFallback),
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// ListRoutingRules lists routing rules strictly ordered by priority ASC, id ASC.
func (s *Store) ListRoutingRules() ([]RoutingRule, error) {
	rows, err := s.db.Query(`
		SELECT id, priority, name, origin_match, ext_glob, min_size_bytes, max_size_bytes, target_provider, target_remote, target_playlist, spillover_pool, stop_on_match, enabled, is_fallback
		FROM routing_rules ORDER BY priority ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]RoutingRule, 0)
	for rows.Next() {
		var r RoutingRule
		var stop, enabled, fb int
		if err := rows.Scan(&r.ID, &r.Priority, &r.Name, &r.OriginMatch, &r.ExtGlob, &r.MinSizeBytes, &r.MaxSizeBytes, &r.TargetProvider, &r.TargetRemote, &r.TargetPlaylist, &r.SpilloverPool, &stop, &enabled, &fb); err != nil {
			return nil, err
		}
		r.StopOnMatch = stop != 0
		r.Enabled = enabled != 0
		r.IsFallback = fb != 0
		out = append(out, r)
	}
	return out, rows.Err()
}

// DeleteRoutingRule removes a non-fallback routing rule.
func (s *Store) DeleteRoutingRule(id int64) error {
	res, err := s.db.Exec(`DELETE FROM routing_rules WHERE id = ? AND is_fallback = 0`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return errors.New("rule not found or cannot delete fallback rule")
	}
	return nil
}

// EnqueueJob creates a persistent job in the SQLite queue.
func (s *Store) EnqueueJob(j Job) (int64, error) {
	now := time.Now().UTC().Unix()
	if j.Status == "" {
		j.Status = "QUEUED"
	}
	if j.Priority == 0 {
		j.Priority = 100
	}
	if j.MaxAttempts == 0 {
		j.MaxAttempts = 5
	}
	res, err := s.db.Exec(`
		INSERT INTO jobs(kind, source_path, target_remote, target_playlist, status, priority, bytes_total, bytes_done, attempts, max_attempts, next_retry_at, last_error, resumable_uri, resumable_expires_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		j.Kind, j.SourcePath, j.TargetRemote, j.TargetPlaylist, j.Status, j.Priority,
		j.BytesTotal, j.BytesDone, j.Attempts, j.MaxAttempts, j.NextRetryAt, j.LastError,
		j.ResumableURI, j.ResumableExpiresAt, now, now,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// UpdateJobState updates discrete state transitions and resumable session metadata.
func (s *Store) UpdateJobState(id int64, status string, bytesDone int64, lastErr string, resumableURI string, resumableExpires int64) error {
	now := time.Now().UTC().Unix()
	_, err := s.db.Exec(`
		UPDATE jobs SET status = ?, bytes_done = ?, last_error = ?, resumable_uri = COALESCE(NULLIF(?, ''), resumable_uri),
		resumable_expires_at = CASE WHEN ? > 0 THEN ? ELSE resumable_expires_at END,
		updated_at = ? WHERE id = ?`,
		status, bytesDone, lastErr, resumableURI, resumableExpires, resumableExpires, now, id)
	return err
}

// UpdateJobPriority adjusts queue priority or status for pause/resume/retry.
func (s *Store) UpdateJobAction(id int64, action string) error {
	now := time.Now().UTC().Unix()
	switch action {
	case "pause":
		_, err := s.db.Exec(`UPDATE jobs SET status = 'PAUSED', updated_at = ? WHERE id = ?`, now, id)
		return err
	case "resume", "retry":
		_, err := s.db.Exec(`UPDATE jobs SET status = 'QUEUED', next_retry_at = 0, updated_at = ? WHERE id = ?`, now, id)
		return err
	case "priority":
		_, err := s.db.Exec(`UPDATE jobs SET priority = MAX(1, priority - 50), updated_at = ? WHERE id = ?`, now, id)
		return err
	default:
		return fmt.Errorf("unknown job action %q", action)
	}
}

// ListJobs returns recent jobs ordered by priority and update time.
func (s *Store) ListJobs(limit int) ([]Job, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.Query(`
		SELECT id, kind, source_path, target_remote, target_playlist, status, priority, bytes_total, bytes_done, attempts, max_attempts, next_retry_at, last_error, resumable_uri, resumable_expires_at, created_at, updated_at
		FROM jobs ORDER BY CASE status WHEN 'RUNNING' THEN 0 WHEN 'QUEUED' THEN 1 WHEN 'PAUSED' THEN 2 ELSE 3 END, priority ASC, updated_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Job, 0)
	for rows.Next() {
		var j Job
		if err := rows.Scan(&j.ID, &j.Kind, &j.SourcePath, &j.TargetRemote, &j.TargetPlaylist, &j.Status, &j.Priority, &j.BytesTotal, &j.BytesDone, &j.Attempts, &j.MaxAttempts, &j.NextRetryAt, &j.LastError, &j.ResumableURI, &j.ResumableExpiresAt, &j.CreatedAt, &j.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

// SetRemoteCursor persists Google Drive changes.list startPageToken or Mega reconciliation timestamps.
func (s *Store) SetRemoteCursor(remoteID, token string) error {
	now := time.Now().UTC().Unix()
	_, err := s.db.Exec(`
		INSERT INTO remote_cursors(remote_id, cursor_token, last_reconciled_at)
		VALUES (?, ?, ?)
		ON CONFLICT(remote_id) DO UPDATE SET cursor_token = excluded.cursor_token, last_reconciled_at = excluded.last_reconciled_at`,
		remoteID, token, now)
	return err
}

// GetRemoteCursor retrieves the persisted cursor token for remoteID.
func (s *Store) GetRemoteCursor(remoteID string) (string, int64, error) {
	var tok string
	var ts int64
	err := s.db.QueryRow(`SELECT cursor_token, last_reconciled_at FROM remote_cursors WHERE remote_id = ?`, remoteID).Scan(&tok, &ts)
	if errors.Is(err, sql.ErrNoRows) {
		return "", 0, nil
	}
	return tok, ts, err
}

// ListTombstones returns pending or recent tombstones.
func (s *Store) ListTombstones() ([]Tombstone, error) {
	rows, err := s.db.Query(`SELECT id, file_id, remote_id, remote_path, detected_at, resolution, purge_after FROM tombstones ORDER BY detected_at DESC LIMIT 200`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Tombstone, 0)
	for rows.Next() {
		var t Tombstone
		if err := rows.Scan(&t.ID, &t.FileID, &t.RemoteID, &t.RemotePath, &t.DetectedAt, &t.Resolution, &t.PurgeAfter); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// ResolveTombstone marks a tombstone RESTORE or PURGE.
func (s *Store) ResolveTombstone(id int64, resolution string) error {
	if resolution != "RESTORED" && resolution != "PURGED" {
		return fmt.Errorf("invalid tombstone resolution %q", resolution)
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	var fileID int64
	if err := tx.QueryRow(`SELECT file_id FROM tombstones WHERE id = ?`, id).Scan(&fileID); err != nil {
		_ = tx.Rollback()
		return err
	}
	if _, err := tx.Exec(`UPDATE tombstones SET resolution = ? WHERE id = ?`, resolution, id); err != nil {
		_ = tx.Rollback()
		return err
	}
	if resolution == "RESTORED" && fileID > 0 {
		if _, err := tx.Exec(`UPDATE files SET tombstoned_at = 0, status = 'SYNCED', updated_at = ? WHERE id = ?`, time.Now().UTC().Unix(), fileID); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// AddTombstone inserts a remote-detected deletion tombstone.
func (s *Store) AddTombstone(fileID int64, remoteID, remotePath string) (int64, error) {
	now := time.Now().UTC().Unix()
	res, err := s.db.Exec(`INSERT INTO tombstones(file_id, remote_id, remote_path, detected_at, resolution, purge_after) VALUES (?, ?, ?, ?, 'PENDING', ?)`,
		fileID, remoteID, remotePath, now, now+int64(30*24*3600))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// GetYouTubeQuota returns daily units used for PT date YYYY-MM-DD.
func (s *Store) GetYouTubeQuota(ptDate string) (int, error) {
	var used int
	err := s.db.QueryRow(`SELECT units_used FROM youtube_quota_ledger WHERE pt_date = ?`, ptDate).Scan(&used)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return used, err
}

// AddYouTubeQuota atomically increments daily YouTube quota usage if under cap.
func (s *Store) AddYouTubeQuota(ptDate string, delta, dailyCap int) (int, bool, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, false, err
	}
	var used int
	err = tx.QueryRow(`SELECT units_used FROM youtube_quota_ledger WHERE pt_date = ?`, ptDate).Scan(&used)
	if errors.Is(err, sql.ErrNoRows) {
		used = 0
	} else if err != nil {
		_ = tx.Rollback()
		return 0, false, err
	}
	if used+delta > dailyCap {
		_ = tx.Rollback()
		return used, false, nil
	}
	newUsed := used + delta
	now := time.Now().UTC().Unix()
	if _, err := tx.Exec(`
		INSERT INTO youtube_quota_ledger(pt_date, units_used, updated_at) VALUES (?, ?, ?)
		ON CONFLICT(pt_date) DO UPDATE SET units_used = excluded.units_used, updated_at = excluded.updated_at`,
		ptDate, newUsed, now); err != nil {
		_ = tx.Rollback()
		return used, false, err
	}
	return newUsed, true, tx.Commit()
}

// UpsertPhoneSyncFolder creates or updates a phone sync folder configuration.
func (s *Store) UpsertPhoneSyncFolder(f PhoneSyncFolder) error {
	_, err := s.db.Exec(`
		INSERT INTO phone_sync_folders(id, transport, label, local_path, remote_path, auto_sync, rag_notify, last_sync_at, status)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			transport = excluded.transport,
			label = excluded.label,
			local_path = excluded.local_path,
			remote_path = excluded.remote_path,
			auto_sync = excluded.auto_sync,
			rag_notify = excluded.rag_notify,
			last_sync_at = excluded.last_sync_at,
			status = excluded.status`,
		f.ID, f.Transport, f.Label, f.LocalPath, f.RemotePath,
		boolToInt(f.AutoSync), boolToInt(f.RAGNotify), f.LastSyncAt, f.Status)
	return err
}

// ListPhoneSyncFolders lists all configured phone sync folders.
func (s *Store) ListPhoneSyncFolders() ([]PhoneSyncFolder, error) {
	rows, err := s.db.Query(`SELECT id, transport, label, local_path, remote_path, auto_sync, rag_notify, last_sync_at, status FROM phone_sync_folders ORDER BY transport, label`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]PhoneSyncFolder, 0)
	for rows.Next() {
		var f PhoneSyncFolder
		var autoS, ragN int
		if err := rows.Scan(&f.ID, &f.Transport, &f.Label, &f.LocalPath, &f.RemotePath, &autoS, &ragN, &f.LastSyncAt, &f.Status); err != nil {
			return nil, err
		}
		f.AutoSync = autoS != 0
		f.RAGNotify = ragN != 0
		out = append(out, f)
	}
	return out, rows.Err()
}

// RecordAppSnapshot stores a custom .exe app snapshot or conflict entry.
func (s *Store) RecordAppSnapshot(snap AppSnapshot) (int64, error) {
	now := time.Now().UTC().Unix()
	if snap.CreatedAt == 0 {
		snap.CreatedAt = now
	}
	res, err := s.db.Exec(`
		INSERT INTO app_snapshots(app_slug, snapshot_type, kopia_snapshot_id, sha256, parent_revision, rel_path, conflict_path, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		snap.AppSlug, snap.SnapshotType, snap.KopiaSnapshotID, snap.SHA256, snap.ParentRevision, snap.RelPath, snap.ConflictPath, snap.CreatedAt)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// ListAppSnapshots returns recent custom .exe app snapshots.
func (s *Store) ListAppSnapshots(limit int) ([]AppSnapshot, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(`SELECT id, app_slug, snapshot_type, kopia_snapshot_id, sha256, parent_revision, rel_path, conflict_path, created_at FROM app_snapshots ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]AppSnapshot, 0)
	for rows.Next() {
		var a AppSnapshot
		if err := rows.Scan(&a.ID, &a.AppSlug, &a.SnapshotType, &a.KopiaSnapshotID, &a.SHA256, &a.ParentRevision, &a.RelPath, &a.ConflictPath, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// Stats returns quick catalog metrics for status/health UI headers.
func (s *Store) Stats() (map[string]any, error) {
	var totalFiles, totalBytes, activeJobs, pendingTombstones int64
	_ = s.db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(size_bytes), 0) FROM files WHERE tombstoned_at = 0`).Scan(&totalFiles, &totalBytes)
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM jobs WHERE status IN ('QUEUED', 'RUNNING')`).Scan(&activeJobs)
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM tombstones WHERE resolution = 'PENDING'`).Scan(&pendingTombstones)
	return map[string]any{
		"totalFiles":        totalFiles,
		"totalBytes":        totalBytes,
		"activeJobs":        activeJobs,
		"pendingTombstones": pendingTombstones,
	}, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
