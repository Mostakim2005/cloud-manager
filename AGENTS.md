# Project Operating Instructions (`AGENTS.md`)
> **Notice:** This project inherits user profile, confirmation gates, documentation blueprints, and master sync protocols from the global master `AGENTS.md`. Do not duplicate those rules here.

## 1. Project Classification, Stack Lock & Environment
* **Project Name:** Cloud Manager (`cloud-manager`)
* **Repository Classification:** **GitHub-Backed Public Repository** (`https://github.com/Mostakim2005/cloud-manager`, `PolyForm Noncommercial License 1.0.0`). GitHub Actions is the authoritative CI/CD runner.
* **Current State:** **Stack Locked (Pre-Implementation — awaiting user "go" for Phase 1 scaffolding)**.
* **Locked Architecture (`Alpha 1 + Alpha 4`):** **Go** core daemon (`cloud-managerd.exe` + `cloud-manager.exe --json` CLI + System Tray) + **TypeScript/Vite** local browser UI (`Material 3 Tonal` Light/Dark/System; portable Electron `.exe` wrapper deferred post-v1).
* **OSS Reuse Policy:** Custom code is strictly **Go** and **TypeScript/JavaScript**. Reused external tools (`Rclone >= v1.66`, unmodified upstream `Syncthing` LAN-only via REST API, `Kopia`, optional `ADB`, `SQLite 3`) run as Job-Object-supervised adapters.
* **Target Build Output:** Windows 10/11 x64 **portable `.exe`** (`CloudManager.exe`) delivered via tag-triggered (`vX.Y.Z`) GitHub Actions releases.

## 2. Ports, Ecosystem Integration & Shared Assets
* **Loopback Backplane & Control API:** `127.0.0.1:59432` (HTTP + `/mcp`, strictly loopback-only; `Authorization: Bearer` / `X-Local-Token` via `%USERPROFILE%\.local\share\ecosystem-secrets.json`, rejecting any `Origin` header with `403` and validating `Host`).
* **Opt-In Browser QR Drop-Zone:** Port `53317` (HTTPS/TLS + 6-digit PIN, **OFF by default**, isolated from `:59432` and non-colliding with GaussBoard LAN `53318`/`53319`).
* **Supervised Adapter Ports:** Ephemeral `127.0.0.1:0` loopback ports for `rclone rcd` and unmodified upstream `Syncthing` (LAN-Only mode, Global Discovery/relays disabled).
* **Pentagram Integration (`Delta 4–5`, `Bravo 4`):** Notify `RAG Manager` (`POST http://127.0.0.1:27130/v1/ingest/event`) on synced `Obsidian`/`GaussBoard` changes; keep both `%USERPROFILE%\.local\share\ecosystem-registry.json` and `%LOCALAPPDATA%\SharedLibraries\ecosystem-registry.json` synced identically (UTF-8 without BOM); register `HKCU` `cloud-manager://` URI and `%USERPROFILE%\.local\bin\cloud-manager.exe` (`--json` CLI); load offline UI assets from `%LOCALAPPDATA%\SharedLibraries`.

## 3. Repository Directory Structure
* `docs/PRODUCT.md` — Locked v1 feature map, non-goals, and deferred wishlist (`[PLANNED]`).
* `docs/ARCHITECTURE.md` — Visual architecture, security boundaries, subsystem flows, and CI specs.
* `docs/DECISIONS_AND_TRAPS.md` — Locked stack/sync pivots and critical runtime landmines.
* `LICENSE` / `THIRD_PARTY_NOTICES.md` / `.gitignore` — PolyForm NC 1.0.0 license, OSS notice ledger, and ignore rules.
* *(Source trees and `.github/workflows/ci.yml` will be added at the start of Phase 1 once the user says "go".)*

## 4. Verification & Build Commands (Locked for `.github/workflows/ci.yml` in Phase 1)
* **Backend Check & Tests:** `go vet ./...` and `go test -race ./...`
* **Frontend Lint, Typecheck & Tests:** `npm run lint`, `npx tsc --noEmit`, `npm test`
* **Windows Portable Build:** `GOOS=windows GOARCH=amd64 go build` (plus tag-triggered `vX.Y.Z` GitHub release packaging)

## 5. Repository-Specific Trap Pointers (See `docs/DECISIONS_AND_TRAPS.md`)
* **Mega `ModTime` / Hash Trap:** Never run timestamp `bisync` on Mega; use Rclone `>= v1.66` with `hasher` overlay in Append/Archive mode.
* **Mega Node Concurrency Trap:** Never bypass `rclone rcd` with parallel raw Mega uploads; handle HTTP `509` per-IP limits with backoff.
* **Loopback Security & Live SQLite Trap:** Never expose `:59432` without Bearer/`Origin`/`Host` checks or `?app=`/filename confinement, and never copy live `.sqlite` files without `VACUUM INTO`.
* **Windows Job Object Trap:** Always attach `rclone rcd`, `syncthing`, `adb`, and `kopia` child processes to a Windows Job Object (`JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE`).
* **ADB MediaStore & CRLF Trap:** Execute ADB via Go `exec.Command` argv, strip `\r`, query `images/media` + `video/media` with `relative_path`, and normalize timestamps to UTC seconds.
* **YouTube OAuth & Quota Traps:** Set OAuth consent to `In Production` (avoid 7-day expiry), track daily PT quota (`9,600 / 10,000` cap, never poll `videos.list`), and persist resumable `Location` URIs.
* **SQLite WAL & Telemetry Trap:** Always set `WAL` + `PRAGMA busy_timeout = 5000`, gate schemas via `PRAGMA user_version` + `.bak` snapshots, and keep UI progress in memory (`<= 5 Hz` SSE).
