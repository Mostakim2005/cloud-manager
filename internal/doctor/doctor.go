package doctor

import (
	"bytes"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/Mostakim2005/cloud-manager/internal/config"
	"github.com/Mostakim2005/cloud-manager/internal/db"
	"github.com/Mostakim2005/cloud-manager/internal/security"
)

// ToolCheck reports availability of an external OSS binary.
type ToolCheck struct {
	Name      string `json:"name"`
	Found     bool   `json:"found"`
	Path      string `json:"path,omitempty"`
	Required  bool   `json:"required"`
	Condition string `json:"condition"`
}

// Report contains a redacted system diagnostic summary.
type Report struct {
	Timestamp         string          `json:"timestamp"`
	Service           string          `json:"service"`
	OS                string          `json:"os"`
	Arch              string          `json:"arch"`
	GoVersion         string          `json:"goVersion"`
	Healthy           bool            `json:"healthy"`
	Tools             []ToolCheck     `json:"tools"`
	RegistryPrimaryOK bool            `json:"registryPrimaryOk"`
	RegistryMirrorOK  bool            `json:"registryMirrorOk"`
	RegistryParityOK  bool            `json:"registryParityOk"`
	SecretsFileOK     bool            `json:"secretsFileOk"`
	SharedLibrariesOK bool            `json:"sharedLibrariesOk"`
	CatalogStats      map[string]any  `json:"catalogStats,omitempty"`
	RedactedSettings  config.Settings `json:"settings"`
	Paths             config.Paths    `json:"paths"`
}

// Run inspects local paths, external binaries, registry parity, and catalog health (zero secrets leaked).
func Run(paths config.Paths, settings config.Settings, store *db.Store) Report {
	reg, _ := config.LoadRegistry(paths.RegistryPrimaryPath, paths.RegistryMirrorPath)

	checkBin := func(name string, regKey string, required bool) ToolCheck {
		p, err := exec.LookPath(name)
		if err != nil && reg != nil && reg.Toolchains != nil {
			if candidate := reg.Toolchains[regKey]; candidate != "" {
				if _, serr := os.Stat(candidate); serr == nil {
					p = candidate
					err = nil
				}
			}
		}
		cond := "ready"
		if err != nil {
			cond = "missing"
		}
		return ToolCheck{Name: name, Found: err == nil, Path: p, Required: required, Condition: cond}
	}

	tools := []ToolCheck{
		checkBin("rclone", "rclone", true),
		checkBin("syncthing", "syncthing", false),
		checkBin("adb", "adb", false),
		checkBin("kopia", "kopia", false),
		checkBin("sqlite3", "sqlite3", false),
	}

	pRaw, pErr := os.ReadFile(paths.RegistryPrimaryPath)
	mRaw, mErr := os.ReadFile(paths.RegistryMirrorPath)
	regParity := pErr == nil && mErr == nil && bytes.Equal(config.StripUTF8BOM(pRaw), config.StripUTF8BOM(mRaw))

	_, secErr := os.Stat(paths.SecretsPath)
	_, sharedErr := os.Stat(paths.SharedLibrariesDir)

	var stats map[string]any
	if store != nil {
		stats, _ = store.Stats()
	}

	redactedPaths := paths
	redactedPaths.RclonePassBlobPath = "[DPAPI_SEALED]"
	redactedPaths.YouTubeTokenBlob = "[DPAPI_SEALED]"
	redactedPaths.SecretsPath = security.RedactSecrets(paths.SecretsPath)

	healthy := store != nil
	return Report{
		Timestamp:         time.Now().UTC().Format(time.RFC3339),
		Service:           "cloud-manager",
		OS:                runtime.GOOS,
		Arch:              runtime.GOARCH,
		GoVersion:         runtime.Version(),
		Healthy:           healthy,
		Tools:             tools,
		RegistryPrimaryOK: pErr == nil,
		RegistryMirrorOK:  mErr == nil,
		RegistryParityOK:  regParity,
		SecretsFileOK:     secErr == nil,
		SharedLibrariesOK: sharedErr == nil,
		CatalogStats:      stats,
		RedactedSettings:  settings,
		Paths:             redactedPaths,
	}
}
