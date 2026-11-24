# Project Operating Instructions (`AGENTS.md`)
> **Notice:** This project inherits user profile, confirmation gates, documentation blueprints, and master sync protocols from the global master `AGENTS.md`. Do not duplicate those rules here.

## 1. Project Classification, Stack Policy & Environment
* **Project Name:** Cloud Manager (`cloud-manager`)
* **Repository Classification:** **GitHub-Backed Public Repository** (`https://github.com/Mostakim2005/cloud-manager`, `PolyForm Noncommercial License 1.0.0`). GitHub Actions is the authoritative CI/CD runner once scaffolded.
* **Current State:** **Pre-Implementation Blueprint** — zero application source code exists yet (`src/`, `go.mod`, `package.json`, `pyproject.toml`, and `.github/workflows/ci.yml` await final stack lock).
* **AI-Friendly Custom Code Policy:** Any custom-written code MUST be **TypeScript/JavaScript, Go, or Python**. Reused battle-tested open-source binaries/libraries (`Rclone`, `LocalSend v2` Android app, optional `Syncthing` LAN-only, optional `ADB`, `SQLite 3`) may be written in any language.
* **Target Build Output:** Windows 10/11 x64 **portable `.exe`** (`CloudManager.exe`) delivered via tag-triggered (`vX.Y.Z`) GitHub Actions releases (no NSIS installer).

## 2. Ports, Ecosystem Integration & Shared Assets
* **Loopback Backplane & Control API:** `127.0.0.1:59432` (HTTP + `/mcp`, strictly loopback-only; `Authorization: Bearer` / `X-Local-Token` via `%USERPROFILE%\.local\share\ecosystem-secrets.json`, rejecting any `Origin` header with `403` and validating `Host`).
* **Same-Router Wi-Fi P2P Transfer:** Port `53317` (LocalSend v2 HTTPS/TLS + PIN, **OFF by default**, isolated from `:59432` and non-colliding with GaussBoard LAN `53318`/`53319`).
* **Registry & Shared Assets:** Register `cloudManager` in `%USERPROFILE%\.local\share\ecosystem-registry.json` during Phase 1 scaffolding; load offline frontend vendor bundles from `%LOCALAPPDATA%\SharedLibraries` (never remote CDNs).

## 3. Repository Directory Structure
* `docs/PRODUCT.md` — Planned feature specs, non-goals, and wishlist (`[PLANNED]`).
* `docs/ARCHITECTURE.md` — Visual architecture, security boundaries, subsystem data flows, and CI plan.
* `docs/DECISIONS_AND_TRAPS.md` — Stack/sync pivots and critical engineering landmines.
* `LICENSE` / `THIRD_PARTY_NOTICES.md` / `.gitignore` — PolyForm NC 1.0.0 license, OSS notice ledger, and ignore rules.
* *(Source folders, dependency manifests, and `.github/workflows/ci.yml` will be added once the core stack is locked.)*

## 4. Verification & Build Commands
* **`[PENDING STACK LOCK]`** — Final lint, typecheck, unit/integration test, security regression, and portable `.exe` build commands will be wired into `.github/workflows/ci.yml` and recorded here when Phase 0/1 scaffolding begins.

## 5. Repository-Specific Trap Pointers (See `docs/DECISIONS_AND_TRAPS.md`)
* **Mega `ModTime` / Hash Trap:** Never run timestamp `bisync` on Mega; use Rclone `>= v1.66` with `hasher` overlay and append/archive pool semantics.
* **Mega Node Concurrency Trap:** Never bypass `rclone rcd` with parallel raw Mega uploads; handle HTTP `509` per-IP limits with backoff.
* **Loopback Security & Live SQLite Trap:** Never expose `:59432` without Bearer/`Origin`/`Host` checks or `?app=`/filename confinement, and never copy live `.sqlite` files without `VACUUM INTO`.
* **Windows Job Object Trap:** Always attach child `rclone rcd`/`adb` processes to a Windows Job Object (`JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE`).
* **ADB MediaStore & CRLF Trap:** Execute ADB via argv (`shell=False`), strip `\r` from `adb shell` stdout, query both `images/media` and `video/media` with `relative_path`, and normalize timestamps to UTC seconds.
* **YouTube OAuth & Quota Traps:** Set OAuth consent to `In Production` (avoid 7-day expiry), track daily PT quota (`9,600 / 10,000` cap, never poll `videos.list`), and persist resumable `Location` URIs.
* **SQLite WAL & Telemetry Trap:** Always set `WAL` + `PRAGMA busy_timeout = 5000`, gate schemas via `PRAGMA user_version` + `.bak` snapshots, and throttle UI telemetry (`<= 5 Hz` in memory).
