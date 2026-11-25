package cli

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Mostakim2005/cloud-manager/internal/config"
	"github.com/Mostakim2005/cloud-manager/internal/db"
	"github.com/Mostakim2005/cloud-manager/internal/doctor"
	"github.com/Mostakim2005/cloud-manager/internal/routing"
	"github.com/Mostakim2005/cloud-manager/internal/tray"
)

const (
	ExitOK         = 0
	ExitRuntime    = 1
	ExitInvalidArg = 2
	ExitOfflineDep = 3
)

// Run executes the deterministic --json CLI contract (stdout=JSON only, stderr=diagnostics).
func Run(args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) int {
	if len(args) == 0 {
		printHelp(stderr)
		return ExitInvalidArg
	}
	cmd := args[0]
	rest := args[1:]

	paths := config.DiscoverPaths("")
	settings, _ := config.LoadOrInitSettings(paths)
	secrets, _ := config.LoadOrInitSecrets(paths.SecretsPath)

	switch cmd {
	case "--help", "-h", "help":
		printHelp(stderr)
		return ExitOK
	case "health":
		return runHealth(rest, paths, settings, stdout, stderr)
	case "doctor":
		return runDoctor(rest, paths, settings, stdout, stderr)
	case "status":
		return runDaemonGET("/api/v1/status", rest, paths, secrets, stdout, stderr)
	case "pool":
		return runPool(rest, paths, secrets, stdout, stderr)
	case "route":
		return runRoute(rest, paths, settings, stdin, stdout, stderr)
	case "sync":
		return runSync(rest, paths, settings, secrets, stdin, stdout, stderr)
	case "lan":
		return runLAN(rest, paths, secrets, stdout, stderr)
	case "youtube":
		return runYouTube(rest, paths, settings, secrets, stdout, stderr)
	case "open-uri":
		return runOpenURI(rest, stdout, stderr)
	default:
		emitError(stderr, stdout, hasJSONFlag(rest), ExitInvalidArg, fmt.Sprintf("unknown command %q", cmd))
		return ExitInvalidArg
	}
}

func hasJSONFlag(args []string) bool {
	for i := 0; i < len(args); i++ {
		if args[i] == "--json" {
			return true
		}
		if args[i] == "--format" && i+1 < len(args) && args[i+1] == "json" {
			return true
		}
	}
	return false
}

func emitJSON(stdout io.Writer, v any) {
	enc := json.NewEncoder(stdout)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

func emitError(stderr io.Writer, stdout io.Writer, asJSON bool, code int, msg string) {
	fmt.Fprintln(stderr, msg)
	if asJSON {
		emitJSON(stdout, map[string]any{"ok": false, "exitCode": code, "error": msg})
	}
}

func runHealth(args []string, paths config.Paths, settings config.Settings, stdout, stderr io.Writer) int {
	asJSON := hasJSONFlag(args)
	daemonURL := fmt.Sprintf("http://%s:%d/api/v1/health", config.DefaultBindHost, config.DefaultPort)
	client := &http.Client{Timeout: 800 * time.Millisecond}
	daemonAlive := false
	if resp, err := client.Get(daemonURL); err == nil {
		daemonAlive = resp.StatusCode == http.StatusOK
		_ = resp.Body.Close()
	}
	rep := doctor.Run(paths, settings, nil)
	payload := map[string]any{
		"ok":           true,
		"service":      "cloud-manager",
		"daemonOnline": daemonAlive,
		"url":          fmt.Sprintf("http://%s:%d", config.DefaultBindHost, config.DefaultPort),
		"tools":        rep.Tools,
		"registryOk":   rep.RegistryParityOK,
	}
	if asJSON {
		emitJSON(stdout, payload)
	} else {
		fmt.Fprintf(stderr, "cloud-manager health: daemonOnline=%v registryParity=%v\n", daemonAlive, rep.RegistryParityOK)
	}
	return ExitOK
}

func runDoctor(args []string, paths config.Paths, settings config.Settings, stdout, stderr io.Writer) int {
	asJSON := hasJSONFlag(args)
	store, err := db.Open(paths.DatabasePath)
	if err != nil {
		emitError(stderr, stdout, asJSON, ExitRuntime, err.Error())
		return ExitRuntime
	}
	defer store.Close()
	rep := doctor.Run(paths, settings, store)
	if asJSON {
		emitJSON(stdout, rep)
	} else {
		fmt.Fprintf(stderr, "Doctor healthy=%v files=%v\n", rep.Healthy, rep.CatalogStats["totalFiles"])
	}
	return ExitOK
}

func runDaemonGET(endpoint string, args []string, _ config.Paths, secrets *config.EcosystemSecrets, stdout, stderr io.Writer) int {
	asJSON := hasJSONFlag(args)
	resp, body, code := callDaemon(http.MethodGet, endpoint, nil, secrets)
	if code != ExitOK {
		emitError(stderr, stdout, asJSON, code, string(body))
		return code
	}
	if resp.StatusCode >= 400 {
		emitError(stderr, stdout, asJSON, ExitRuntime, string(body))
		return ExitRuntime
	}
	if asJSON {
		_, _ = stdout.Write(append(bytes.TrimSpace(body), '\n'))
	} else {
		fmt.Fprintln(stderr, string(body))
	}
	return ExitOK
}

func runPool(args []string, paths config.Paths, secrets *config.EcosystemSecrets, stdout, stderr io.Writer) int {
	asJSON := hasJSONFlag(args)
	if len(args) == 0 {
		emitError(stderr, stdout, asJSON, ExitInvalidArg, "pool subcommand required: quota | duplicates | reconcile")
		return ExitInvalidArg
	}
	switch args[0] {
	case "quota":
		store, err := db.Open(paths.DatabasePath)
		if err != nil {
			emitError(stderr, stdout, asJSON, ExitRuntime, err.Error())
			return ExitRuntime
		}
		defer store.Close()
		accounts, _ := store.ListAccounts()
		if asJSON {
			emitJSON(stdout, map[string]any{"accounts": accounts})
		} else {
			for _, a := range accounts {
				fmt.Fprintf(stderr, "%s (%s): %d free bytes\n", a.Name, a.Provider, a.FreeBytes)
			}
		}
		return ExitOK
	case "duplicates":
		store, err := db.Open(paths.DatabasePath)
		if err != nil {
			emitError(stderr, stdout, asJSON, ExitRuntime, err.Error())
			return ExitRuntime
		}
		defer store.Close()
		dups, _ := store.ListDuplicates()
		emitJSON(stdout, map[string]any{"duplicates": dups})
		return ExitOK
	case "reconcile":
		return runDaemonGET("/api/v1/tombstones", args[1:], paths, secrets, stdout, stderr)
	default:
		emitError(stderr, stdout, asJSON, ExitInvalidArg, fmt.Sprintf("unknown pool subcommand %q", args[0]))
		return ExitInvalidArg
	}
}

func runRoute(args []string, paths config.Paths, settings config.Settings, stdin io.Reader, stdout, stderr io.Writer) int {
	asJSON := hasJSONFlag(args)
	if len(args) == 0 {
		emitError(stderr, stdout, asJSON, ExitInvalidArg, "route subcommand required: test | list | add")
		return ExitInvalidArg
	}
	store, err := db.Open(paths.DatabasePath)
	if err != nil {
		emitError(stderr, stdout, asJSON, ExitRuntime, err.Error())
		return ExitRuntime
	}
	defer store.Close()

	switch args[0] {
	case "list":
		rules, _ := store.ListRoutingRules()
		emitJSON(stdout, map[string]any{"rules": rules})
		return ExitOK
	case "test":
		fs := flag.NewFlagSet("route test", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		origin := fs.String("origin", "pc", "")
		size := fs.Int64("size", 1024*1024, "")
		_ = fs.Bool("json", false, "")
		if err := fs.Parse(stripGlobalFlags(args[1:])); err != nil || fs.NArg() == 0 {
			emitError(stderr, stdout, asJSON, ExitInvalidArg, "usage: route test <filename> [--origin pc] [--size N] [--json]")
			return ExitInvalidArg
		}
		rules, _ := store.ListRoutingRules()
		accounts, _ := store.ListAccounts()
		res := routing.Evaluate(rules, accounts, routing.NewQuotaReservationTracker(), settings, fs.Arg(0), *origin, *size, time.Now())
		emitJSON(stdout, res)
		return ExitOK
	case "add":
		fs := flag.NewFlagSet("route add", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		inFile := fs.String("input-file", "", "")
		_ = fs.Bool("json", false, "")
		if err := fs.Parse(stripGlobalFlags(args[1:])); err != nil {
			emitError(stderr, stdout, asJSON, ExitInvalidArg, "invalid route add flags")
			return ExitInvalidArg
		}
		raw, err := readInputOrStdin(*inFile, stdin)
		if err != nil {
			emitError(stderr, stdout, asJSON, ExitInvalidArg, err.Error())
			return ExitInvalidArg
		}
		var rule db.RoutingRule
		if err := json.Unmarshal(config.StripUTF8BOM(raw), &rule); err != nil || rule.Name == "" || rule.TargetProvider == "" {
			emitError(stderr, stdout, asJSON, ExitInvalidArg, "valid rule JSON with name and targetProvider required")
			return ExitInvalidArg
		}
		rule.Enabled = true
		rule.StopOnMatch = true
		id, err := store.InsertRoutingRule(rule)
		if err != nil {
			emitError(stderr, stdout, asJSON, ExitRuntime, err.Error())
			return ExitRuntime
		}
		rule.ID = id
		emitJSON(stdout, map[string]any{"ok": true, "rule": rule})
		return ExitOK
	default:
		emitError(stderr, stdout, asJSON, ExitInvalidArg, fmt.Sprintf("unknown route subcommand %q", args[0]))
		return ExitInvalidArg
	}
}

func runSync(args []string, paths config.Paths, settings config.Settings, secrets *config.EcosystemSecrets, stdin io.Reader, stdout, stderr io.Writer) int {
	asJSON := hasJSONFlag(args)
	if len(args) == 0 {
		emitError(stderr, stdout, asJSON, ExitInvalidArg, "sync subcommand required: run | app")
		return ExitInvalidArg
	}
	switch args[0] {
	case "run":
		dry := containsFlag(args[1:], "--dry-run")
		store, err := db.Open(paths.DatabasePath)
		if err != nil {
			emitError(stderr, stdout, asJSON, ExitRuntime, err.Error())
			return ExitRuntime
		}
		defer store.Close()
		jobs, _ := store.ListJobs(50)
		emitJSON(stdout, map[string]any{"dryRun": dry, "queuedJobs": len(jobs), "jobs": jobs})
		return ExitOK
	case "app":
		fs := flag.NewFlagSet("sync app", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		appSlug := fs.String("app", "", "")
		file := fs.String("file", "", "")
		sqlitePath := fs.String("sqlite-db", "", "")
		kopiaDir := fs.String("kopia-dir", "", "")
		inFile := fs.String("input-file", "", "")
		dry := fs.Bool("dry-run", false, "")
		_ = fs.Bool("json", false, "")
		if err := fs.Parse(stripGlobalFlags(args[1:])); err != nil || *appSlug == "" {
			emitError(stderr, stdout, asJSON, ExitInvalidArg, "usage: sync app --app <slug> [--file path | --sqlite-db path | --kopia-dir dir | --input-file json] [--dry-run] [--json]")
			return ExitInvalidArg
		}
		payload := map[string]any{
			"sourcePath":   *file,
			"sqliteDbPath": *sqlitePath,
			"kopiaDirPath": *kopiaDir,
			"autoQueue":    true,
		}
		if *inFile != "" {
			raw, err := readInputOrStdin(*inFile, stdin)
			if err != nil {
				emitError(stderr, stdout, asJSON, ExitInvalidArg, err.Error())
				return ExitInvalidArg
			}
			_ = json.Unmarshal(config.StripUTF8BOM(raw), &payload)
		}
		if *file != "" {
			payload["relPath"] = filepath.Base(*file)
		}
		if *dry {
			emitJSON(stdout, map[string]any{"dryRun": true, "app": *appSlug, "payload": payload})
			return ExitOK
		}
		rawBody, _ := json.Marshal(payload)
		resp, body, code := callDaemon(http.MethodPost, "/api/v1/sync?app="+url.QueryEscape(*appSlug), rawBody, secrets)
		if code != ExitOK {
			emitError(stderr, stdout, asJSON, code, string(body))
			return code
		}
		if resp.StatusCode >= 400 {
			emitError(stderr, stdout, asJSON, ExitRuntime, string(body))
			return ExitRuntime
		}
		_, _ = stdout.Write(append(bytes.TrimSpace(body), '\n'))
		return ExitOK
	default:
		emitError(stderr, stdout, asJSON, ExitInvalidArg, fmt.Sprintf("unknown sync subcommand %q", args[0]))
		return ExitInvalidArg
	}
}

func runLAN(args []string, _ config.Paths, secrets *config.EcosystemSecrets, stdout, stderr io.Writer) int {
	asJSON := hasJSONFlag(args)
	if len(args) == 0 {
		emitError(stderr, stdout, asJSON, ExitInvalidArg, "lan subcommand required: status | enable | disable")
		return ExitInvalidArg
	}
	var method string
	var body []byte
	switch args[0] {
	case "status":
		method = http.MethodGet
	case "enable":
		method = http.MethodPost
		body = []byte(`{"enabled":true}`)
	case "disable":
		method = http.MethodPost
		body = []byte(`{"enabled":false}`)
	default:
		emitError(stderr, stdout, asJSON, ExitInvalidArg, fmt.Sprintf("unknown lan subcommand %q", args[0]))
		return ExitInvalidArg
	}
	resp, respBody, code := callDaemon(method, "/api/v1/lan-qr", body, secrets)
	if code != ExitOK {
		emitError(stderr, stdout, asJSON, code, string(respBody))
		return code
	}
	if resp.StatusCode >= 400 {
		emitError(stderr, stdout, asJSON, ExitRuntime, string(respBody))
		return ExitRuntime
	}
	_, _ = stdout.Write(append(bytes.TrimSpace(respBody), '\n'))
	return ExitOK
}

func runYouTube(args []string, paths config.Paths, settings config.Settings, secrets *config.EcosystemSecrets, stdout, stderr io.Writer) int {
	asJSON := hasJSONFlag(args)
	if len(args) == 0 {
		emitError(stderr, stdout, asJSON, ExitInvalidArg, "youtube subcommand required: quota | queue")
		return ExitInvalidArg
	}
	switch args[0] {
	case "quota":
		return runDaemonGET("/api/v1/youtube/quota", args[1:], paths, secrets, stdout, stderr)
	case "queue":
		fs := flag.NewFlagSet("youtube queue", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		playlist := fs.String("playlist", settings.YouTubePlaylistID, "")
		dry := fs.Bool("dry-run", false, "")
		_ = fs.Bool("json", false, "")
		if err := fs.Parse(stripGlobalFlags(args[1:])); err != nil || fs.NArg() == 0 {
			emitError(stderr, stdout, asJSON, ExitInvalidArg, "usage: youtube queue <file> [--playlist id] [--dry-run] [--json]")
			return ExitInvalidArg
		}
		if *dry {
			emitJSON(stdout, map[string]any{
				"dryRun":        true,
				"file":          fs.Arg(0),
				"playlist":      *playlist,
				"privacyStatus": "private",
			})
			return ExitOK
		}
		body, _ := json.Marshal(map[string]any{"sourcePath": fs.Arg(0), "playlistId": *playlist, "dryRun": false})
		resp, respBody, code := callDaemon(http.MethodPost, "/api/v1/youtube/queue", body, secrets)
		if code != ExitOK {
			emitError(stderr, stdout, asJSON, code, string(respBody))
			return code
		}
		if resp.StatusCode >= 400 {
			emitError(stderr, stdout, asJSON, ExitRuntime, string(respBody))
			return ExitRuntime
		}
		_, _ = stdout.Write(append(bytes.TrimSpace(respBody), '\n'))
		return ExitOK
	default:
		emitError(stderr, stdout, asJSON, ExitInvalidArg, fmt.Sprintf("unknown youtube subcommand %q", args[0]))
		return ExitInvalidArg
	}
}

func runOpenURI(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "URI argument required")
		return ExitInvalidArg
	}
	u, err := url.Parse(args[0])
	if err != nil || !strings.EqualFold(u.Scheme, "cloud-manager") {
		fmt.Fprintf(stderr, "invalid cloud-manager URI: %q\n", args[0])
		return ExitInvalidArg
	}
	view := u.Query().Get("view")
	job := u.Query().Get("job")
	target := fmt.Sprintf("http://%s:%d/?view=%s&job=%s", config.DefaultBindHost, config.DefaultPort, url.QueryEscape(view), url.QueryEscape(job))
	_ = tray.OpenBrowser(target)
	emitJSON(stdout, map[string]any{"ok": true, "openedUrl": target})
	return ExitOK
}

func callDaemon(method, path string, body []byte, secrets *config.EcosystemSecrets) (*http.Response, []byte, int) {
	u := fmt.Sprintf("http://%s:%d%s", config.DefaultBindHost, config.DefaultPort, path)
	var r io.Reader
	if body != nil {
		r = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, u, r)
	if err != nil {
		return nil, []byte(err.Error()), ExitRuntime
	}
	if secrets != nil && secrets.Tokens["cloudManager"] != "" {
		req.Header.Set("Authorization", "Bearer "+secrets.Tokens["cloudManager"])
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, []byte("cloud-managerd daemon offline on 127.0.0.1:59432: " + err.Error()), ExitOfflineDep
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp, raw, ExitOK
}

func stripGlobalFlags(args []string) []string {
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		if args[i] == "--json" {
			continue
		}
		if args[i] == "--format" && i+1 < len(args) {
			i++
			continue
		}
		out = append(out, args[i])
	}
	return out
}

func containsFlag(args []string, target string) bool {
	for _, a := range args {
		if a == target {
			return true
		}
	}
	return false
}

func readInputOrStdin(path string, stdin io.Reader) ([]byte, error) {
	if path == "" || path == "-" {
		return io.ReadAll(stdin)
	}
	return os.ReadFile(path)
}

func printHelp(w io.Writer) {
	fmt.Fprintln(w, `Cloud Manager CLI (--json supported on all commands)
Exit codes: 0=ok, 1=runtime error, 2=invalid args, 3=daemon/dependency offline
Commands:
  health [--json]
  doctor [--json]
  status [--json]
  pool <quota|duplicates|reconcile> [--json]
  route <list|test <file>|add --input-file <json>> [--json]
  sync <run|app --app <slug>> [--dry-run] [--json]
  lan <status|enable|disable> [--json]
  youtube <quota|queue <file>> [--dry-run] [--json]
  open-uri <cloud-manager://...>`)
}
