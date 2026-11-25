package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/Mostakim2005/cloud-manager/internal/security"
)

const (
	DefaultBindHost = "127.0.0.1"
	DefaultPort     = 59432
	DefaultLANPort  = 53317
)

var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// Paths resolves all user-path directories and ecosystem JSON files without hardcoding user names.
type Paths struct {
	DataDir             string `json:"dataDir"`
	LogsDir             string `json:"logsDir"`
	StagingDir          string `json:"stagingDir"`
	SharedLibrariesDir  string `json:"sharedLibrariesDir"`
	RegistryPrimaryPath string `json:"registryPrimaryPath"`
	RegistryMirrorPath  string `json:"registryMirrorPath"`
	SecretsPath         string `json:"secretsPath"`
	DatabasePath        string `json:"databasePath"`
	SettingsPath        string `json:"settingsPath"`
	RcloneConfPath      string `json:"rcloneConfPath"`
	RclonePassBlobPath  string `json:"rclonePassBlobPath"`
	YouTubeTokenBlob    string `json:"youtubeTokenBlob"`
}

// Settings holds persisted user configuration for Cloud Manager.
type Settings struct {
	SchemaVersion        int    `json:"schemaVersion"`
	Theme                string `json:"theme"`
	Density              string `json:"density"`
	StagingDir           string `json:"stagingDir"`
	LANPort              int    `json:"lanPort"`
	LANEnabledDefault    bool   `json:"lanEnabledDefault"`
	AutoRouteLAN         bool   `json:"autoRouteLan"`
	BandwidthDayKBps     int    `json:"bandwidthDayKbps"`
	BandwidthNightKBps   int    `json:"bandwidthNightKbps"`
	BandwidthDayStart    int    `json:"bandwidthDayStartHour"`
	BandwidthDayEnd      int    `json:"bandwidthDayEndHour"`
	RAGManagerURL        string `json:"ragManagerUrl"`
	YouTubePlaylistID    string `json:"youtubePlaylistId"`
	ShowAdvancedSettings bool   `json:"showAdvancedSettings"`
}

// DefaultSettings returns conservative, offline-first defaults.
func DefaultSettings(stagingDir string) Settings {
	return Settings{
		SchemaVersion:        1,
		Theme:                "system",
		Density:              "comfortable",
		StagingDir:           stagingDir,
		LANPort:              DefaultLANPort,
		LANEnabledDefault:    false,
		AutoRouteLAN:         true,
		BandwidthDayKBps:     4096,
		BandwidthNightKBps:   0,
		BandwidthDayStart:    9,
		BandwidthDayEnd:      23,
		RAGManagerURL:        "http://127.0.0.1:27130",
		YouTubePlaylistID:    "",
		ShowAdvancedSettings: false,
	}
}

// EcosystemSecrets matches %USERPROFILE%\.local\share\ecosystem-secrets.json.
type EcosystemSecrets struct {
	SchemaVersion int               `json:"schemaVersion"`
	Tokens        map[string]string `json:"tokens"`
}

// EcosystemRegistry models relevant discovery fields in ecosystem-registry.json.
type EcosystemRegistry struct {
	SchemaVersion int                       `json:"schemaVersion"`
	UpdatedAt     string                    `json:"updatedAt"`
	Contract      string                    `json:"contract,omitempty"`
	Paths         map[string]string         `json:"paths,omitempty"`
	Services      map[string]map[string]any `json:"services,omitempty"`
	Apps          map[string]map[string]any `json:"apps,omitempty"`
	Toolchains    map[string]string         `json:"toolchains,omitempty"`
}

// DiscoverPaths locates SharedLibraries, ecosystem registry/secrets, and app data dirs.
func DiscoverPaths(overrideDataDir string) Paths {
	home, _ := os.UserHomeDir()
	localAppData := os.Getenv("LOCALAPPDATA")
	if localAppData == "" {
		if runtime.GOOS == "windows" {
			localAppData = filepath.Join(home, "AppData", "Local")
		} else {
			localAppData = filepath.Join(home, ".local", "share")
		}
	}

	dataDir := overrideDataDir
	if dataDir == "" {
		if env := os.Getenv("CLOUD_MANAGER_DATA_DIR"); env != "" {
			dataDir = env
		} else {
			dataDir = filepath.Join(localAppData, "CloudManager")
		}
	}

	sharedDir := firstNonEmpty(
		os.Getenv("LOCAL_COMMON_LIBRARIES"),
		os.Getenv("SHARED_LIBRARIES_DIR"),
		filepath.Join(localAppData, "SharedLibraries"),
	)

	regPrimary := firstNonEmpty(
		os.Getenv("LOCAL_ECOSYSTEM_REGISTRY"),
		filepath.Join(home, ".local", "share", "ecosystem-registry.json"),
	)
	regMirror := filepath.Join(sharedDir, "ecosystem-registry.json")

	secretsPath := firstNonEmpty(
		os.Getenv("LOCAL_ECOSYSTEM_SECRETS"),
		filepath.Join(home, ".local", "share", "ecosystem-secrets.json"),
	)

	// Allow registry paths.secretsFile override when primary registry exists.
	if reg, err := LoadRegistry(regPrimary, regMirror); err == nil {
		if s := reg.Paths["secretsFile"]; s != "" && os.Getenv("LOCAL_ECOSYSTEM_SECRETS") == "" {
			secretsPath = s
		}
	}

	return Paths{
		DataDir:             dataDir,
		LogsDir:             filepath.Join(dataDir, "logs"),
		StagingDir:          filepath.Join(dataDir, "staging"),
		SharedLibrariesDir:  sharedDir,
		RegistryPrimaryPath: regPrimary,
		RegistryMirrorPath:  regMirror,
		SecretsPath:         secretsPath,
		DatabasePath:        filepath.Join(dataDir, "catalog.sqlite"),
		SettingsPath:        filepath.Join(dataDir, "settings.json"),
		RcloneConfPath:      filepath.Join(dataDir, "rclone.conf"),
		RclonePassBlobPath:  filepath.Join(dataDir, "rclone-pass.dpapi"),
		YouTubeTokenBlob:    filepath.Join(dataDir, "youtube-oauth.dpapi"),
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// StripUTF8BOM removes an accidental UTF-8 BOM prefix if present.
func StripUTF8BOM(b []byte) []byte {
	return bytes.TrimPrefix(b, utf8BOM)
}

// WriteAtomicNoBOM writes data to path via temp file -> fsync -> atomic rename with zero BOM.
func WriteAtomicNoBOM(path string, data []byte, perm os.FileMode) error {
	clean := StripUTF8BOM(data)
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = tmp.Close()
			_ = os.Remove(tmpName)
		}
	}()
	if _, err := tmp.Write(clean); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, perm); err != nil && runtime.GOOS != "windows" {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	cleanup = false
	return nil
}

// WriteJSONAtomic writes pretty-printed UTF-8 JSON without BOM atomically.
func WriteJSONAtomic(path string, v any, perm os.FileMode) error {
	buf := &bytes.Buffer{}
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return err
	}
	return WriteAtomicNoBOM(path, buf.Bytes(), perm)
}

// LoadRegistry reads the first valid ecosystem registry JSON.
func LoadRegistry(candidates ...string) (*EcosystemRegistry, error) {
	for _, p := range candidates {
		if p == "" {
			continue
		}
		raw, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var reg EcosystemRegistry
		if err := json.Unmarshal(StripUTF8BOM(raw), &reg); err != nil {
			return nil, fmt.Errorf("parse registry %s: %w", p, err)
		}
		return &reg, nil
	}
	return nil, os.ErrNotExist
}

// LoadOrInitSecrets reads ecosystem-secrets.json or generates tokens.cloudManager if missing.
func LoadOrInitSecrets(path string) (*EcosystemSecrets, error) {
	var sec EcosystemSecrets
	if raw, err := os.ReadFile(path); err == nil {
		if uerr := json.Unmarshal(StripUTF8BOM(raw), &sec); uerr != nil {
			return nil, fmt.Errorf("parse secrets %s: %w", path, uerr)
		}
	}
	if sec.SchemaVersion == 0 {
		sec.SchemaVersion = 1
	}
	if sec.Tokens == nil {
		sec.Tokens = make(map[string]string)
	}
	changed := false
	for _, key := range []string{"cloudManager", "ragManager", "obsidianAgentManager", "deepTutor"} {
		if sec.Tokens[key] == "" {
			tok, err := security.GenerateToken()
			if err != nil {
				return nil, err
			}
			sec.Tokens[key] = tok
			changed = true
		}
	}
	if changed {
		if err := WriteJSONAtomic(path, &sec, 0o600); err != nil {
			return nil, err
		}
	}
	return &sec, nil
}

// LoadOrInitSettings loads settings.json or writes clean defaults atomically.
func LoadOrInitSettings(p Paths) (Settings, error) {
	def := DefaultSettings(p.StagingDir)
	raw, err := os.ReadFile(p.SettingsPath)
	if err != nil {
		if os.IsNotExist(err) {
			if werr := WriteJSONAtomic(p.SettingsPath, def, 0o600); werr != nil {
				return def, werr
			}
			return def, nil
		}
		return def, err
	}
	var s Settings
	if err := json.Unmarshal(StripUTF8BOM(raw), &s); err != nil {
		return def, err
	}
	if s.SchemaVersion == 0 {
		s.SchemaVersion = 1
	}
	if s.StagingDir == "" {
		s.StagingDir = p.StagingDir
	}
	if s.LANPort == 0 {
		s.LANPort = DefaultLANPort
	}
	if s.Theme == "" {
		s.Theme = "system"
	}
	if s.Density == "" {
		s.Density = "comfortable"
	}
	if s.RAGManagerURL == "" {
		s.RAGManagerURL = def.RAGManagerURL
	}
	return s, nil
}

// Logger emits rotating, secret-redacted JSONL records to disk and optionally stderr.
type Logger struct {
	mu      sync.Mutex
	path    string
	maxSize int64
}

// NewLogger creates a rotating JSONL logger under logsDir.
func NewLogger(logsDir string) (*Logger, error) {
	if err := os.MkdirAll(logsDir, 0o700); err != nil {
		return nil, err
	}
	return &Logger{
		path:    filepath.Join(logsDir, "cloud-manager.jsonl"),
		maxSize: 5 * 1024 * 1024,
	}, nil
}

// Log writes a structured, redacted log event.
func (l *Logger) Log(level, component, msg string, fields map[string]any) {
	if l == nil {
		return
	}
	entry := map[string]any{
		"ts":        time.Now().UTC().Format(time.RFC3339Nano),
		"level":     level,
		"component": component,
		"message":   security.RedactSecrets(msg),
	}
	if len(fields) > 0 {
		entry["fields"] = security.RedactMap(fields)
	}
	line, err := json.Marshal(entry)
	if err != nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if st, err := os.Stat(l.path); err == nil && st.Size() >= l.maxSize {
		_ = os.Remove(l.path + ".3")
		_ = os.Rename(l.path+".2", l.path+".3")
		_ = os.Rename(l.path+".1", l.path+".2")
		_ = os.Rename(l.path, l.path+".1")
	}
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(append(line, '\n'))
}
