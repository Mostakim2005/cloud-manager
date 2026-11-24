# Product Intent & Feature Map (`PRODUCT.md`)
> **1-Line Summary:** The permanent source of truth for user intentions, locked `[PLANNED]` v1 feature specs, non-goals, and wishlist for Cloud Manager.

## 1. Core Vision & User Experience Intent
* **Blueprint Status:** **Stack & Scope Locked (`[PLANNED]` — awaiting user "go" to start Phase 1 scaffolding)** using **Go Core Daemon (`cloud-managerd.exe` + `cloud-manager.exe`) + TypeScript Local Browser UI + System Tray** (portable Electron `.exe` wrapper deferred post-v1).
* **AI-Friendly Language & Reuse Policy:** Custom code stays strictly in **Go** (core daemon, adapters, CLI/MCP) and **TypeScript/JavaScript** (frontend UI) for reliable AI development and automated CI checks. Reused battle-tested open-source tools (`Rclone`, unmodified upstream `Syncthing`, `Kopia`, `ADB`, `SQLite 3`) are integrated cleanly over local APIs/CLIs.
* **What This App Does (Plain English):** Cloud Manager pools multiple Google Drive and Mega.nz accounts into one virtual catalog, routes incoming files deterministically, runs two-way delta sync between PC and phone over **Wi-Fi (Syncthing LAN-Only + Ephemeral Browser QR Drop-Zone) and Wireless/USB ADB**, archives videos to private YouTube playlists, and exposes an authenticated `127.0.0.1:59432` backplane + Kopia block-delta snapshots for custom Windows `.exe` apps and Pentagram tools.
* **How It Must Feel to Use:** Instant and zero-freeze. Local FTS5 browsing resolves under 5 ms from SQLite WAL without waiting on cloud APIs, paired with throttled (`<= 5 Hz`) transfer telemetry and Light/Dark/System UI parity.

---

## 2. Locked v1 Features (`[PLANNED]`)

### [FEATURE: Multi-Cloud-Storage-Pooling] | Aliases: account manager, drive pooling, quota aggregation, mega gdrive merge, duplicate inspector
> **1-Line Summary:** Pools Google Drive (two-way `bisync`) and Mega.nz (Append/Archive via `hasher`) into one unified view with a Cross-Account Duplicate & Orphan Inspector.
* **User Intention (Non-Negotiable):** Connect multiple Google Drive and Mega accounts simultaneously, monitor individual and pooled gigabyte quotas, browse files in one catalog, and reclaim duplicate storage across accounts.
* **Locked Design (`Charlie 1` + `Delta 3`):** Uses Rclone (`>= v1.66`) `union` (`epmfs`/`mfs`) + `hasher` overlays on Mega remotes as Append/Archive tiers (eliminating Mega `ModTime` re-upload loops) while running two-way `bisync` (`Changes.list`) on Google Drive, plus a SHA-256 **Cross-Account Duplicate & Orphan Inspector**.
* **Status:** `[PLANNED]` — code anchors will be added during Phase 1/2 scaffolding.

---

### [FEATURE: Smart-Routing-Rule-Engine] | Aliases: automated routing, file sorting, origin rules, dry run simulator, quota spillover
> **1-Line Summary:** Dispatches files deterministically via priority-ordered rules with a drag-and-drop Rule Dry-Run Simulator, Quota Spillover, and Bandwidth Scheduler.
* **User Intention (Non-Negotiable):** Route files automatically by origin folder, extension, or size, test rule behavior safely beforehand, spill gracefully when an account nears capacity, and throttle transfers by schedule.
* **Locked Design (`Delta 1` + `Delta 2`):** Strict `priority ASC` (`first-match-wins`) dispatch with a catch-all fallback rule, in-memory `QuotaReservationTracker` (`500 MiB` safety floor) spillover, drag-and-drop **Rule Dry-Run Simulator**, and time-of-day **Bandwidth Scheduler**.
* **Status:** `[PLANNED]` — code anchors will be added during Phase 2 scaffolding.

---

### [FEATURE: Phone-Media-Delta-Sync-WiFi-And-ADB] | Aliases: phone sync, wifi transfer, syncthing lan, wireless adb, browser qr dropzone, rag webhook
> **1-Line Summary:** Runs two-way PC <-> phone folder delta sync across images, videos, PDFs, Markdown, Obsidian, and GaussBoard via Syncthing LAN-Only, Wireless/USB ADB, and a Browser QR Drop-Zone, pinging `RAG Manager`.
* **User Intention (Non-Negotiable):** Click Sync (or enable per-folder auto-sync) over Wi-Fi or USB to inspect exact new/changed/conflicting media, PDFs, Markdown, `Obsidian`, `GaussBoard`, and `Music` folders without multicast discovery failures, and push files both ways.
* **Locked Design (`Bravo 1` + `Bravo 4`):**
  * **Unmodified Upstream Syncthing (LAN-Only via REST API + Event Stream — NOT forked):** One-time QR pairing with Android `Syncthing-Fork` (`Global Discovery`/relays disabled) for two-way block-level folder delta sync, controlled via Syncthing's local REST API for click-to-sync pause/resume and per-file diff progress.
  * **Wireless ADB + USB ADB (Android 11+ QR Pairing):** In-UI Wireless ADB QR pairing + MediaStore (`images/media`, `video/media`) & folder diffing for two-way pull/push without any phone sync app.
  * **Ephemeral Browser QR Drop-Zone (`https://<lan-ip>:53317` + 6-digit PIN, OFF by default):** Instant bidirectional browser upload/download via QR scan.
  * **Pentagram Live Re-Index Hook (`Bravo 4`):** Automatically fires `POST http://127.0.0.1:27130/v1/ingest/event` to `RAG Manager` whenever synced `Obsidian` or `GaussBoard` files update.
* **Status:** `[PLANNED]` — code anchors will be added during Phase 2 scaffolding.

---

### [FEATURE: YouTube-Private-Video-Pipeline] | Aliases: youtube uploader, private video sync, playlist automation
> **1-Line Summary:** Streams 16 MB video chunks using a custom ~200 LOC Go YouTube v3 SDK uploader with forced `private` visibility, persisted resume URIs, and PT quota tracking.
* **User Intention (Non-Negotiable):** Archive screen recordings and videos directly to private YouTube playlists without exhausting cloud storage quotas.
* **Locked Design (`Charlie 3`):** Lightweight custom Go module using Google's official YouTube Data API v3 Go SDK enforcing `privacyStatus = "private"`, persisting resumable `Location` URIs in SQLite, capping daily Pacific Time quota (`9,600 / 10,000` units), and documenting `In Production` OAuth consent.
* **Status:** `[PLANNED]` — code anchors will be added during Phase 2 scaffolding.

---

### [FEATURE: Localhost-App-Backplane-API] | Aliases: exe sync, custom app api, localhost backplane, kopia snapshots, mcp cli
> **1-Line Summary:** Provides an authenticated `127.0.0.1:59432` HTTP + `/mcp` backplane, `--json` CLI, `cloud-manager://` URI, `VACUUM INTO` safety, and Kopia block-delta snapshots.
* **User Intention (Non-Negotiable):** Let custom `.exe` and Pentagram apps push/pull state over `127.0.0.1:59432` or CLI/MCP without bundling cloud SDKs, keeping large app directories deduplicated.
* **Locked Design (`Delta 4`, `Delta 5`, `Delta 6`):** Full `ECOSYSTEM_INTEGRATION_CONTRACT.md` v2.0.0 compliance (`Bearer`/`X-Local-Token` via `ecosystem-secrets.json`, `Origin` `403`, `Host` & `?app=` checks), dual `ecosystem-registry.json` + `HKCU` `cloud-manager://` registration, `%USERPROFILE%\.local\bin\cloud-manager.exe` `--json` CLI, SQLite `VACUUM INTO` snapshots, conflict preservation, and **Kopia (`Apache-2.0`)** block-delta snapshots for large `.exe` app directories.
* **Status:** `[PLANNED]` — code anchors will be added during Phase 1/2/4 scaffolding.

---

### [FEATURE: Virtualized-File-Explorer-GUI] | Aliases: file grid, visual explorer, material 3 ui, ctrl+k picker, quick look
> **1-Line Summary:** Renders 50,000+ SQLite-cached files at 60 FPS with Clean Minimalist + Material 3 Tonal Surfaces, `Ctrl+K` quick-picker, `?` cheat-sheet, density toggle, and `Space` Quick-Look.
* **User Intention (Non-Negotiable):** Sub-5 ms offline search and 60 FPS scrolling with instant status feedback, zero contrast issues across themes, and effortless keyboard-first navigation.
* **Locked Design (`Echo 1`, `Echo 3`, `Echo 4`, `Echo 5`):** `TanStack Virtual` + `Lucide` SVGs styled with **Clean Minimalist + Material 3 Tonal Surfaces** (Light/Dark/System WCAG AA `>= 4.5:1`), `Ctrl+K` quick-picker + `?` shortcut cheat-sheet, `Comfortable`/`Compact` density toggle, `Space` Quick-Look / Lightbox preview for staged media, Empty/Loading/Error states, Show/Hide toggles, and themed destructive confirmations.
* **Status:** `[PLANNED]` — code anchors will be added during Phase 3 scaffolding.

---

## 3. Non-Goals & External API Constraints
* **No Custom Android App-Data Sync:** Bidirectional syncing of private Android app-internal databases is out of scope.
* **Google Photos Library API (Archived / Blocked):** Restricted by Google on March 31, 2025 to app-created media only; phone photos route to Mega/Google Drive pools instead.

---

## 4. Deferred Enhancements & Wishlist (`[WISHLIST]`)

### [WISHLIST: Portable-Electron-Shell-Wrapper] | Aliases: electron shell, standalone desktop window, post-v1 wrapper
> **1-Line Summary:** Optional post-v1 Electron portable `.exe` shell wrapping the local TypeScript UI (`Alpha 4` runs via local browser + System Tray in v1).
* **User Intention:** Keep v1 lean via local browser UI + System Tray, with zero backend rewrite if a dedicated Electron `.exe` window is wrapped later.

### [WISHLIST: Virtual-Drive-Mount-WinFsp] | Aliases: winfsp mount, virtual windows drive, rclone mount
> **1-Line Summary:** Optional post-v1 Windows drive-letter mounting via WinFsp + Rclone VFS once core sync pipelines are battle-tested.
* **User Intention:** Browse pooled cloud storage in Windows File Explorer without premature kernel-driver complexity in v1.
