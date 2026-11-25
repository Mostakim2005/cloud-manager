# Phased Build & Verification Plan (`implementation-plan.md`)

### [FLOW: Phased-Build-And-Verification-Plan] | Aliases: roadmap, phases, implementation plan, verification checklist
> **1-Line Summary:** Records Cloud Manager's executed Phases 0–4 implementation roadmap, user-path installations, and GitHub Actions verification gates.

## Phase Breakdown (Completed)

1. **Phase 0 & User-Path Setup (Complete):**
   * Shared Material 3 Tonal & Accessibility CSS baseline (`%LOCALAPPDATA%\SharedLibraries\m3-tokens\m3-tokens.css`), `lucide` (`0.468.0`), and `qrcode-generator` (`1.4.4`) installed and registered in `libraries.json` + `SharedLibraries/README.md`.
   * CSPRNG 256-bit `ecosystem-secrets.json` bootstrapped with user-only NTFS ACLs; `ecosystem-registry.json` mirrored identically (UTF-8 without BOM) between `%USERPROFILE%\.local\share` and `%LOCALAPPDATA%\SharedLibraries`.
   * `HKCU\Software\Classes\cloud-manager` URI protocol registered; `ECOSYSTEM_INTEGRATION_CONTRACT.md` updated with `:59432` and `:53317`.
2. **Phase 1 — Core Daemon, Security Boundaries & SQLite Safety (Complete):**
   * `internal/security/` (`ValidateLoopbackHost`, `ValidateOrigin`, `ConstantTimeTokenMatch`, `ValidateAppSlug`, `ResolveConfinedPath`, `RedactSecrets`, Windows Job Objects, Windows DPAPI, disk headroom).
   * `internal/config/` (zero-context discovery, atomic UTF-8-no-BOM writes, rotating redacted JSONL logs).
   * `internal/db/` (`modernc.org/sqlite` WAL + `busy_timeout=5000`, `PRAGMA user_version` + rolling 5 `.bak` snapshots, FTS5 triggers, `VACUUM INTO`, `remote_cursors`, `tombstones`, `youtube_quota_ledger`).
   * `internal/telemetry/` (in-memory ring buffer + `<= 5 Hz` SSE broadcaster).
3. **Phase 2 — Routing, Ingestion & Supervised Adapters (Complete):**
   * `internal/routing/` (`priority ASC`, `QuotaReservationTracker` `500 MiB` floor, Dry-Run Simulator, Bandwidth Scheduler).
   * `internal/ingest/` (128 KB streaming SHA-256, conflict preservation, `RAG Manager :27130` webhook).
   * `internal/adapters/{rclone,syncthing,adb,lanqr,youtube,kopia}/`.
4. **Phase 3 — TypeScript Material 3 Tonal Frontend (Complete):**
   * `web/` (Vite + TypeScript + `@tanstack/virtual-core` + `lucide` + native `<dialog>` modals + FOUC-safe Light/Dark/System theme + `Comfortable`/`Compact` density + Vitest a11y/contrast tests).
5. **Phase 4 — CLI, `/mcp`, Tray, CI & Portable Packaging (Complete):**
   * `internal/cli/` (`--json` CLI, exit codes `0/1/2/3`, `open-uri`), `internal/server/` (`/mcp` + HTTP + SSE), `internal/tray/` (Win32 NotifyIcon), `.github/workflows/ci.yml`, `.github/workflows/release.yml`.

## Verification Gates (Executed in GitHub Actions)
1. Linux CI (`ubuntu-latest`): `gofmt`, `go vet ./...`, `npm run lint`, `npx tsc --noEmit`, `npm test`, `npm run build`, `go test -race -v ./...`, `GOOS=windows GOARCH=amd64 go build`, and doc/BOM parity checks.
2. Windows CI (`windows-latest`): Native `go test -v ./...` (real Windows DPAPI, Job Objects, NTFS paths), `.exe` CLI smoke checks (`health --json`, `route test --json`, `upgrade --dry-run --json`), and Inno Setup `ISCC.exe` compilation of `packaging/CloudManager.iss` (`CloudManager-Setup.exe`).
3. Post-CI User-Path & Installer Setup: Run `CloudManager-Setup.exe` per-user (zero UAC) to install to `%LOCALAPPDATA%\Programs\CloudManager\` and `%USERPROFILE%\.local\bin\`, register Desktop + Start Menu shortcuts + Windows `Run` `--background` auto-start, and enable interactive UI account connection (`#connect-account-dialog`) and `#live-progress-card` transfer monitoring.
