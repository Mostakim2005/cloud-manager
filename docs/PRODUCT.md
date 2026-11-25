# Product Intent & Feature Map (`PRODUCT.md`)
> **1-Line Summary:** Permanent source of truth for user intentions, implemented v1 features with verified symbol anchors, non-goals, and deferred wishlist for Cloud Manager.

## 1. Core Vision & User Experience Intent
* **Build Status:** **Implemented (Phases 1–4 Complete)** using **Go Core Daemon (`cmd/cloud-managerd` + `cmd/cloud-manager` + portable `cmd/CloudManager`) + TypeScript/Vite Material 3 Tonal Browser UI (`web/`) + Windows System Tray (`internal/tray`)**.
* **AI-Friendly Language & Reuse Policy:** Custom code stays strictly in **Go** and **TypeScript/JavaScript**. External OSS tools (`Rclone`, unmodified upstream `Syncthing`, `Kopia`, `ADB`, `SQLite 3`) run as Job-Object-supervised adapters.
* **What This App Does (Plain English):** Cloud Manager pools multiple Google Drive and Mega.nz accounts into one virtual catalog, routes incoming files deterministically, runs two-way delta sync between PC and phone over **Wi-Fi (Syncthing LAN-Only + Ephemeral Browser QR Drop-Zone) and Wireless/USB ADB**, archives videos to private YouTube playlists, and exposes an authenticated `127.0.0.1:59432` backplane + Kopia block-delta snapshots for custom Windows `.exe` apps and Pentagram tools.
* **How It Must Feel to Use:** Instant and zero-freeze. Local FTS5 browsing resolves in milliseconds from SQLite WAL (`internal/db`) without waiting on cloud APIs, paired with throttled (`<= 5 Hz`) SSE telemetry (`internal/telemetry`) and Light/Dark/System Material 3 UI parity.

---

## 2. Implemented v1 Features (`[IMPLEMENTED]`)

### [FEATURE: Multi-Cloud-Storage-Pooling] | Aliases: account manager, drive pooling, quota aggregation, mega gdrive merge, duplicate inspector
> **1-Line Summary:** Pools Google Drive (two-way `bisync`) and Mega.nz (Append/Archive via `hasher`) into one unified view with a Cross-Account Duplicate & Orphan Inspector.
* **User Intention (Non-Negotiable):** Connect multiple Google Drive and Mega accounts simultaneously, monitor individual and pooled gigabyte quotas, browse files in one catalog, and reclaim duplicate storage across accounts.
* **Implementation & Anchors:**
  * Rclone `rcd` supervisor (`>= v1.66`), `hasher` overlay config, HTTP `509` cooldowns, and `remote_cursors`/`tombstones` reconciliation: `internal/adapters/rclone/rclone.go` -> `Supervisor`, `ValidateSyncMode`, `EnsureMegaHasherConfig`, `ReconcileRemote`.
  * Cross-Account SHA-256 Duplicate Inspector and 30-day Quarantine: `internal/db/db.go` -> `Store.ListDuplicates`, `Store.SoftDeleteFile`, `Store.ResolveTombstone`.

---

### [FEATURE: Smart-Routing-Rule-Engine] | Aliases: automated routing, file sorting, origin rules, dry run simulator, quota spillover
> **1-Line Summary:** Dispatches files deterministically via priority-ordered rules with a drag-and-drop Rule Dry-Run Simulator, Quota Spillover, and Bandwidth Scheduler.
* **User Intention (Non-Negotiable):** Route files automatically by origin folder, extension, or size, test rule behavior safely beforehand, spill gracefully when an account nears capacity, and throttle transfers by schedule.
* **Implementation & Anchors:**
  * Strict `priority ASC` (`first-match-wins`), `QuotaReservationTracker` (`500 MiB` floor), and `BandwidthCapKBps`: `internal/routing/router.go` -> `Evaluate`, `QuotaReservationTracker`, `BandwidthCapKBps`.
  * Drag-and-drop + accessible file-picker Rule Dry-Run Bench: `web/src/views/rules.ts` -> `renderRulesView`, and `/api/v1/rules/simulate` in `internal/server/server.go` -> `Daemon.handleRuleSimulate`.

---

### [FEATURE: Phone-Media-Delta-Sync-WiFi-And-ADB] | Aliases: phone sync, wifi transfer, syncthing lan, wireless adb, browser qr dropzone, rag webhook
> **1-Line Summary:** Runs two-way PC <-> phone folder delta sync across images, videos, PDFs, Markdown, Obsidian, and GaussBoard via Syncthing LAN-Only, Wireless/USB ADB, and a Browser QR Drop-Zone, pinging `RAG Manager`.
* **User Intention (Non-Negotiable):** Click Sync over Wi-Fi or USB to inspect new/changed/conflicting media, PDFs, Markdown, `Obsidian`, `GaussBoard`, and `Music` folders without multicast discovery failures, and push files both ways.
* **Implementation & Anchors:**
  * Unmodified upstream Syncthing LAN-Only REST adapter (`GlobalDiscoveryEnabled=false`, `RelaysEnabled=false`): `internal/adapters/syncthing/syncthing.go` -> `Client.EnforceLANOnly`, `Client.TriggerScan`, `Client.PairingQRSVG`.
  * Wireless QR & USB ADB adapter (MediaStore `images/media` + `video/media`, `\r\r\n` cleanup, UTC second normalization): `internal/adapters/adb/adb.go` -> `Client.QueryMediaDelta`, `ParseMediaStoreOutput`, `WirelessPairingQR`.
  * Opt-In Browser QR Drop-Zone (`https://<lan-ip>:53317` + 6-digit PIN, OFF by default): `internal/adapters/lanqr/server.go` -> `Server.Enable`, `Server.handleUpload`.
  * Unified SHA-256 Ingestion + `RAG Manager :27130` webhook: `internal/ingest/pipeline.go` -> `Pipeline.Ingest`, `Pipeline.notifyRAGManager`.

---

### [FEATURE: YouTube-Private-Video-Pipeline] | Aliases: youtube uploader, private video sync, playlist automation
> **1-Line Summary:** Streams 16 MB video chunks using a custom Go YouTube v3 SDK uploader with forced `private` visibility, persisted resume URIs, and PT quota tracking.
* **User Intention (Non-Negotiable):** Archive screen recordings and videos directly to private YouTube playlists without exhausting cloud storage quotas.
* **Implementation & Anchors:**
  * `internal/adapters/youtube/uploader.go` -> `Uploader.ProcessJob`, `Uploader.QuotaStatus` (`16 MiB` chunks, forced `privacyStatus = "private"`, `9,600 / 10,000` Pacific Time `youtube_quota_ledger` ceiling, persisted `Location` URI).
  * `web/src/views/youtube.ts` -> `renderYouTubeView`.

---

### [FEATURE: Localhost-App-Backplane-API] | Aliases: exe sync, custom app api, localhost backplane, kopia snapshots, mcp cli
> **1-Line Summary:** Provides an authenticated `127.0.0.1:59432` HTTP + `/mcp` backplane, `--json` CLI, `cloud-manager://` URI, `VACUUM INTO` safety, and Kopia block-delta snapshots.
* **User Intention (Non-Negotiable):** Let custom `.exe` and Pentagram apps push/pull state over `127.0.0.1:59432` or CLI/MCP without bundling cloud SDKs, keeping large app directories deduplicated.
* **Implementation & Anchors:**
  * Loopback HTTP + `/mcp` + `VACUUM INTO` + Kopia routing: `internal/server/server.go` -> `Daemon.withGuard`, `Daemon.handleAppBackplaneSync`, `Daemon.handleMCP`.
  * Deterministic `--json` CLI & `cloud-manager://` URI handler: `internal/cli/cli.go` -> `Run`, `runOpenURI`; `internal/adapters/kopia/bridge.go` -> `Bridge.CreateSnapshot`.
  * SQLite safe snapshot & security path jail: `internal/db/db.go` -> `SnapshotSQLiteFile`; `internal/security/security.go` -> `ResolveConfinedPath`.

---

### [FEATURE: Virtualized-File-Explorer-GUI] | Aliases: file grid, visual explorer, material 3 ui, ctrl+k picker, quick look
> **1-Line Summary:** Renders 50,000+ SQLite-cached files at 60 FPS with Clean Minimalist + Material 3 Tonal Surfaces, `Ctrl+K` quick-picker, `?` cheat-sheet, density toggle, and `Space` Quick-Look.
* **User Intention (Non-Negotiable):** Sub-5 ms offline search and 60 FPS scrolling with instant status feedback, zero contrast issues across themes, and effortless keyboard-first navigation.
* **Implementation & Anchors:**
  * `web/src/views/explorer.ts` -> `renderExplorerView` (`@tanstack/virtual-core` `Virtualizer`, FTS5 query, filter chips + Reset, `Space` Quick-Look `<dialog>`).
  * `web/src/main.ts` -> `bootstrapApp` (`Ctrl+K` palette, `?` shortcuts, `Ctrl+1..6`, SSE `subscribeTelemetry`).
  * `web/src/theme.ts` -> `applyTheme`, `applyDensity`; `web/src/styles/tokens.css` (WCAG AA Material 3 Light/Dark/System tokens).

---

## 3. Non-Goals & External API Constraints
* **No Custom Android App-Data Sync:** Bidirectional syncing of private Android app-internal databases is out of scope.
* **Google Photos Library API (Archived / Blocked):** Restricted by Google on March 31, 2025 to app-created media only; phone photos route to Mega/Google Drive pools instead.

---

## 4. Deferred Enhancements & Wishlist (`[WISHLIST]`)

### [WISHLIST: Portable-Electron-Shell-Wrapper] | Aliases: electron shell, standalone desktop window, post-v1 wrapper
> **1-Line Summary:** Optional post-v1 Electron portable `.exe` shell wrapping the local TypeScript UI (`v1` runs via local browser + Windows System Tray).
* **User Intention:** Keep v1 lean via local browser UI + System Tray, with zero backend rewrite if a dedicated Electron `.exe` window is wrapped later.

### [WISHLIST: Virtual-Drive-Mount-WinFsp] | Aliases: winfsp mount, virtual windows drive, rclone mount
> **1-Line Summary:** Optional post-v1 Windows drive-letter mounting via WinFsp + Rclone VFS once core sync pipelines are battle-tested.
* **User Intention:** Browse pooled cloud storage in Windows File Explorer without premature kernel-driver complexity in v1.
