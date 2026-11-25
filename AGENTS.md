# Project Operating Instructions (`AGENTS.md`)
> **Notice:** This project inherits user profile, confirmation gates, documentation blueprints, and master sync protocols from the global master `AGENTS.md`. Do not duplicate those rules here.

## 1. Project Classification, Stack Lock & Environment
* **Project Name:** Cloud Manager (`cloud-manager`)
* **Repository Classification:** **GitHub-Backed Public Repository** (`https://github.com/Mostakim2005/cloud-manager`, `PolyForm Noncommercial License 1.0.0`). GitHub Actions is the authoritative CI/CD runner.
* **Current State:** **Implemented (Phases 1–4 Complete)** — Go core daemon (`cmd/cloud-managerd`), `--json` CLI (`cmd/cloud-manager`), unified portable Windows entrypoint (`cmd/CloudManager`), TypeScript/Vite Material 3 Tonal UI (`web/`), and GitHub Actions CI/Release workflows (`.github/workflows/ci.yml`, `.github/workflows/release.yml`).
* **OSS Reuse Policy:** Custom code is strictly **Go** and **TypeScript/JavaScript**. External tools (`Rclone >= v1.66`, unmodified upstream `Syncthing`, `Kopia`, optional `ADB`, `SQLite 3`) run as Job-Object-supervised adapters; pure-Go `modernc.org/sqlite` provides zero-CGO SQLite WAL + FTS5 + `VACUUM INTO`.
* **Target Build Output:** Windows 10/11 x64 **portable `.exe`** (`CloudManager.exe`, `cloud-manager.exe`, `cloud-managerd.exe`) delivered via GitHub Actions CI artifacts and tag-triggered (`vX.Y.Z`) releases.

## 2. Ports, Ecosystem Integration & Shared Assets
* **Loopback Backplane & Control API:** `127.0.0.1:59432` (HTTP + `<= 5 Hz` SSE + `/mcp`, strictly loopback-only; `Authorization: Bearer` / `X-Local-Token` via `%USERPROFILE%\.local\share\ecosystem-secrets.json`, rejecting cross-origin/strict-`Origin` headers with `403` and validating `Host`).
* **Opt-In Browser QR Drop-Zone:** Port `53317` (HTTPS/TLS + 6-digit PIN, **OFF by default**, isolated from `:59432` and non-colliding with GaussBoard LAN `53318`/`53319`).
* **Supervised Adapter Ports:** Ephemeral `127.0.0.1:0` loopback ports for `rclone rcd` and unmodified upstream `Syncthing` (LAN-Only mode, Global Discovery/relays disabled).
* **Pentagram Integration:** Notifies `RAG Manager` (`POST http://127.0.0.1:27130/v1/ingest/event`) on synced `Obsidian`/`GaussBoard` changes; keeps both `%USERPROFILE%\.local\share\ecosystem-registry.json` and `%LOCALAPPDATA%\SharedLibraries\ecosystem-registry.json` synced identically (UTF-8 without BOM); registers `HKCU` `cloud-manager://` URI and `%USERPROFILE%\.local\bin\cloud-manager.exe`; mirrors shared Material 3 baseline tokens from `%LOCALAPPDATA%\SharedLibraries\m3-tokens\m3-tokens.css` (`lucide` and `qrcode-generator` also registered in `libraries.json`).

## 3. Repository Directory Structure
* `cmd/` — Go entrypoints (`CloudManager/`, `cloud-manager/`, `cloud-managerd/`).
* `internal/` — Modular Go backend (`security/`, `config/`, `db/`, `telemetry/`, `routing/`, `ingest/`, `adapters/{rclone,syncthing,adb,lanqr,youtube,kopia}/`, `qrsvg/`, `doctor/`, `tray/`, `webui/`, `server/`, `cli/`).
* `web/` — TypeScript + Vite Material 3 Tonal frontend (`src/`, `tests/`, `index.html`, `package.json`, `tsconfig.json`, `eslint.config.js`, `vite.config.ts`).
* `.github/workflows/` — Authoritative CI (`ci.yml` on Linux + Windows) and tag-triggered portable `.exe` release (`release.yml`).
* `docs/` — Living Markdown docs (`PRODUCT.md`, `ARCHITECTURE.md`, `DECISIONS_AND_TRAPS.md`, `modules/implementation-plan.md`). Scan with `grep -A 1 "^### \[" docs/`.
* `LICENSE` / `THIRD_PARTY_NOTICES.md` / `README.md` / `.gitignore`.

## 4. Verification & Build Commands (Authoritative in `.github/workflows/ci.yml`)
* **Backend Check & Tests:** `gofmt -l cmd internal`, `go vet ./...`, and `go test -race ./...`
* **Frontend Lint, Typecheck, Tests & Build:** `cd web && npm run lint && npx tsc --noEmit && npm test && npm run build`
* **Windows Portable Build:** `CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags="-s -w"`

## 5. Repository-Specific Trap Pointers (See `docs/DECISIONS_AND_TRAPS.md`)
* **Mega `ModTime` / Hash Trap:** Never run timestamp `bisync` on Mega; use Rclone `>= v1.66` with `hasher` overlay in Append/Archive mode (`rclone.ValidateSyncMode`).
* **Mega Node Concurrency Trap:** Never bypass `rclone rcd` with parallel raw Mega uploads; handle HTTP `509` per-IP limits with circuit breaker + backoff.
* **Loopback Security & Live SQLite Trap:** Never expose `:59432` without Bearer/`Origin`/`Host` checks or `?app=`/filename confinement, and never copy live `.sqlite` files without `VACUUM INTO` (`db.SnapshotSQLiteFile`).
* **Windows Job Object Trap:** Always attach `rclone rcd`, `syncthing`, `adb`, and `kopia` child processes to a Windows Job Object (`JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE`).
* **ADB MediaStore & CRLF Trap:** Execute ADB via Go `exec.Command` argv, strip `\r`, query `images/media` + `video/media` with `relative_path`, and normalize timestamps to UTC seconds.
* **YouTube OAuth & Quota Traps:** Set OAuth consent to `In Production` (avoid 7-day expiry), track daily PT quota (`9,600 / 10,000` cap, never poll `videos.list`), and persist resumable `Location` URIs.
* **SQLite WAL, Telemetry & Theme CSP Traps:** Always set `WAL` + `PRAGMA busy_timeout = 5000`, gate schemas via `PRAGMA user_version` + `.bak` snapshots, keep UI progress in memory (`<= 5 Hz` SSE), and keep inline FOUC theme script nonce-protected under strict CSP.
