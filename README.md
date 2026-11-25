# Cloud Manager

Unified multi-cloud storage pooler, deterministic smart router, two-way Wi-Fi/ADB phone sync hub, private YouTube archive pipeline, and authenticated local `.exe` app backplane for Windows 10/11.

* **License:** PolyForm Noncommercial 1.0.0 ([LICENSE](LICENSE)) — third-party attributions in [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md).
* **Architecture & Plans:** See [docs/PRODUCT.md](docs/PRODUCT.md), [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md), [docs/DECISIONS_AND_TRAPS.md](docs/DECISIONS_AND_TRAPS.md), and [docs/modules/implementation-plan.md](docs/modules/implementation-plan.md).

## Architecture Overview

* **Go Core Daemon & CLI:** `cmd/cloud-managerd`, `cmd/cloud-manager`, and single-binary portable `cmd/CloudManager` serving an embedded TypeScript UI, authenticated loopback HTTP + SSE (`<= 5 Hz`) + Streamable `/mcp` on `127.0.0.1:59432`, and Windows System Tray (`internal/tray`).
* **Supervised External Adapters:** Job-Object-bound wrappers (`JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE`) for `rclone rcd` (`>= v1.66` with `hasher` Mega Append/Archive), unmodified upstream `Syncthing` LAN-Only REST API, `ADB` Wireless QR / USB MediaStore (`\r\r\n` & UTC second normalization), `Kopia` FastCDC block-delta snapshots, opt-in Browser QR Drop-Zone (`https://<lan-ip>:53317` + 6-digit PIN, OFF by default), and Go YouTube Data API v3 private resumable uploader (`16 MiB` chunks, `9,600 / 10,000` PT quota ledger).

## UI Surface Inventory

| Surface | Entry Point / Shortcut | Capabilities |
| :--- | :--- | :--- |
| **Virtual File Explorer** | `Ctrl+1` | `@tanstack/virtual-core` 60 FPS virtual table, `< 5 ms` SQLite FTS5 search (`/` or `Ctrl+F`), pool filter chips + Reset, sortable columns, `Space` Quick-Look `<dialog>`, `Comfortable`/`Compact` density, soft-delete 30-day quarantine |
| **Smart Routing & Simulator** | `Ctrl+2` | Priority `ASC` (`first-match-wins`) rules, Show/Hide rule creator, drag-and-drop + accessible file picker Dry-Run Simulator (`500 MiB` spillover + bandwidth schedule) |
| **Phone Delta Sync** | `Ctrl+3` | Syncthing LAN-Only QR + per-folder Scan/Pause/Resume + `RAG Manager :27130` webhook indicator, Wireless/USB ADB QR + MediaStore delta scan, Opt-In Browser QR Drop-Zone `:53317` toggle & PIN |
| **YouTube Private Pipeline** | `Ctrl+4` | Forced `privacyStatus=private` queue, `16 MiB` resumable chunk progress, Pacific Time daily quota bar (`9,600 / 10,000`) |
| **`.EXE` Backplane & Duplicates** | `Ctrl+5` | `:59432` & `/mcp` status, `VACUUM INTO` / Kopia snapshot table, Cross-Account Duplicate Inspector, Tombstone Quarantine Review Queue, Show/Hide redacted live telemetry logs |
| **Settings & Diagnostics** | `Ctrl+6` | Tri-state `System`/`Light`/`Dark` Material 3 theme (`<meta name="color-scheme">` FOUC-safe), Density toggle, Staging/Bandwidth config, Show/Hide Advanced (`Copy Diagnostic Bundle`, redacted config export, guarded Reset to Defaults) |
| **Global Overlays** | `Ctrl+K`, `?`, `Esc` | Fuzzy command quick-picker (`Ctrl+K`), keyboard shortcut legend (`?`), native `<dialog>` destructive confirmation (`Cancel` autofocused), polite `role="status"` toasts with Undo |

## Headless `--json` CLI & `/mcp`

All CLI verbs emit single JSON objects on `stdout` (`--json`) and diagnostics on `stderr`, returning deterministic exit codes (`0` ok, `1` runtime error, `2` invalid arguments, `3` daemon/dependency offline):

```powershell
cloud-manager health --json
cloud-manager doctor --json
cloud-manager status --json
cloud-manager pool quota --json
cloud-manager pool duplicates --json
cloud-manager route test lecture.mp4 --origin recordings --size 52428800 --json
cloud-manager sync app --app custom-notes --sqlite-db "C:\path\notes.sqlite" --json
cloud-manager lan enable --json
cloud-manager youtube queue "D:\Videos\clip.mp4" --playlist PL_ID --dry-run --json
```

## CI & Portable Release Builds

Verification runs automatically in GitHub Actions (`.github/workflows/ci.yml` on both `ubuntu-latest` and `windows-latest`):
* **Go:** `gofmt`, `go vet ./...`, `go test -race ./...`, `CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build`
* **TypeScript UI:** `npm run lint`, `npx tsc --noEmit`, `npm test`, `npm run build`
* **Tag Releases (`vX.Y.Z`):** `.github/workflows/release.yml` publishes `CloudManager.exe`, `cloud-manager.exe`, `cloud-managerd.exe`, and `SHA256SUMS.txt`.
