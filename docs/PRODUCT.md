# Product Intent & Feature Map (`PRODUCT.md`)
> **1-Line Summary:** The permanent source of truth for user intentions, `[PLANNED]` feature specs, non-goals, and roadmap for the Cloud Manager desktop system.

## 1. Core Vision & User Experience Intent
* **Blueprint Status:** Pre-implementation blueprint (`[PLANNED]` — zero application source code exists yet; final stack lock pending user confirmation).
* **AI-Friendly Language & Reuse Policy:** All custom-written code is restricted to **TypeScript/JavaScript, Go, or Python** so AI agents can reliably build, test, and maintain the codebase. Reused battle-tested open-source binaries and libraries (such as Rclone, LocalSend Android app, Syncthing, ADB, and SQLite) may be implemented in any language.
* **What This App Does (Plain English):** Cloud Manager aggregates multiple Google Drive and Mega.nz accounts into a unified virtual storage pool, deterministically routes incoming files by origin/extension/size, syncs new phone photos and videos over **both Same-Router Wi-Fi LAN P2P and USB/Wi-Fi ADB**, archives designated videos as private YouTube playlists, and exposes an authenticated `127.0.0.1:59432` backplane for custom Windows `.exe` and note-taking apps.
* **How It Must Feel to Use:** Responsive, instantaneous, and zero-freeze. Local catalog browsing and FTS5 search resolve in under 5 ms from SQLite WAL without blocking on cloud APIs, paired with live throttled (`<= 5 Hz`) transfer progress and full Light/Dark/System UI parity.

---

## 2. Planned Core Features (Blueprint — `[PLANNED]`)

### [FEATURE: Multi-Cloud-Storage-Pooling] | Aliases: account manager, drive pooling, quota aggregation, mega gdrive merge
> **1-Line Summary:** Combines multiple independent Google Drive and Mega.nz accounts into a cohesive virtual storage pool via Rclone `union` and `hasher`.
* **User Intention (Non-Negotiable):** Connect multiple free and paid Google Drive and Mega accounts simultaneously, visualize per-account and aggregated free/used quotas, and browse/manage files in one unified catalog.
* **Design Guardrails:** Uses Rclone `union` (`epmfs`/`mfs`) plus Rclone `hasher` overlays (`>= v1.66`) on Mega remotes so Mega's lack of native `ModTime` and standard remote hashes does not trigger re-upload loops.
* **Status:** `[PLANNED]` — code anchors will be added once scaffolding is created.

---

### [FEATURE: Smart-Routing-Rule-Engine] | Aliases: automated routing, file sorting, origin rules, extension dispatch
> **1-Line Summary:** Deterministically dispatches incoming files to designated cloud targets using priority-ordered rules and atomic in-flight quota reservation.
* **User Intention (Non-Negotiable):** Automatically route files (e.g., PDFs to Google Drive, raw media to Mega pools, recordings to YouTube) by origin directory, extension, or size without manual sorting.
* **Design Guardrails:** Evaluates rules strictly by `priority ASC` (`first-match-wins`, explicit `stop_on_match`) with a mandatory catch-all fallback rule, backed by an in-memory `QuotaReservationTracker` (`free - inflight - 500 MiB safety floor`) and automatic pool spillover.
* **Status:** `[PLANNED]` — code anchors will be added once scaffolding is created.

---

### [FEATURE: Phone-Media-Delta-Sync-WiFi-And-ADB] | Aliases: phone sync, wifi transfer, lan p2p, adb transfer, photo importer, dcim pull
> **1-Line Summary:** Pulls only newly captured phone photos, videos, and arbitrary files via Same-Router Wi-Fi LAN P2P or USB/Wi-Fi ADB into one SHA-256 deduplicated pipeline.
* **User Intention (Non-Negotiable):** Transfer only new media from Android to PC over home Wi-Fi (without requiring a USB cable) **and** support USB/Wi-Fi ADB bulk recovery, avoiding duplicate multi-gigabyte copies.
* **Design Guardrails:**
  * **Wi-Fi LAN P2P (Opt-In, OFF by default):** Isolated port (`53317`) supporting LocalSend v2 over HTTPS/TLS with cert fingerprint pinning + 6-digit PIN/accept prompt, plus optional Syncthing in LAN-only mode (Global Discovery/Relays disabled).
  * **ADB Fallback / Bulk Mode:** Queries `images/media` and `video/media` (`_id,relative_path,_display_name,_size,date_modified,datetaken`), normalizes timestamps to UTC seconds, and trims Windows `\r\r\n`.
  * **Canonical Ingestion:** Both paths feed one streaming SHA-256 deduplication and `/Photos/YYYY/MM/` filing pipeline.
* **Status:** `[PLANNED]` — code anchors will be added once scaffolding is created.

---

### [FEATURE: YouTube-Private-Video-Pipeline] | Aliases: youtube uploader, private video sync, playlist automation
> **1-Line Summary:** Uploads designated videos to YouTube as enforced `private` videos assigned to pre-configured playlists via resumable HTTP chunks.
* **User Intention (Non-Negotiable):** Archive screen recordings and large videos to private YouTube playlists instead of exhausting cloud drive quotas.
* **Design Guardrails:** Enforces `status.privacyStatus = "private"`, persists resumable `Location` session URIs across restarts, tracks daily quota units in a persistent `youtube_quota_ledger` (America/Los_Angeles reset, hard cap `9,600 / 10,000` units), and documents `In Production` OAuth consent to prevent 7-day token expiry.
* **Status:** `[PLANNED]` — code anchors will be added once scaffolding is created.

---

### [FEATURE: Localhost-App-Backplane-API] | Aliases: exe sync, custom app api, localhost backplane, cross-app state
> **1-Line Summary:** Exposes an authenticated loopback REST/MCP endpoint on `127.0.0.1:59432` so custom Windows `.exe` apps can sync state safely.
* **User Intention (Non-Negotiable):** Custom desktop and note-taking apps sync state through `127.0.0.1:59432` without bundling Google or Mega SDKs.
* **Design Guardrails:** Enforces `Authorization: Bearer <token>` (or `X-Local-Token`) from `%USERPROFILE%\.local\share\ecosystem-secrets.json`, `Origin` rejection (`403`), strict `Host` validation, `?app=` slug allowlist (`^[a-z0-9][a-z0-9_-]{0,63}$`), clean SQLite `VACUUM INTO` snapshot capture, and non-destructive `(sha256, parent_revision)` conflict preservation (`<name>.conflict-<host>-<utc>.<ext>`).
* **Status:** `[PLANNED]` — code anchors will be added once scaffolding is created.

---

### [FEATURE: Virtualized-File-Explorer-GUI] | Aliases: file grid, visual explorer, lazy loading, upload progress, uiux parity
> **1-Line Summary:** Renders 50,000+ cached files at 60 FPS with `TanStack Virtual`, FTS5 search, throttled transfer telemetry, and full `UIUX.md` parity.
* **User Intention (Non-Negotiable):** Instant navigation and filtering across pooled remotes showing sync states (`Synced`, `Uploading`, `Local-Only`, `Cloud-Only`, `Conflict`) and throttled transfer speeds.
* **Design Guardrails:** Built to `UIUX.md` standards from day one — Light/Dark/System WCAG AA semantic tokens (`>= 4.5:1`), unified `Lucide` SVG icons, `Ctrl+K` quick-switcher, `?` shortcut cheat-sheet, Empty/Loading/Error state triad, Show/Hide progressive disclosure toggles, and themed destructive confirmation dialogs.
* **Status:** `[PLANNED]` — code anchors will be added once scaffolding is created.

---

## 3. Non-Goals & External API Constraints
* **No Custom Android App-Data Sync:** Bidirectional syncing of private Android app-internal databases is explicitly out of scope (phone transfer covers photos, videos, and user-selected files).
* **Google Photos Library API (Archived / Blocked):** Google restricted `photoslibrary` / `photoslibrary.readonly` scopes on March 31, 2025 to app-created media only, blocking full-library automated photo sync/dedup. Phone photos stay routed to Mega/Google Drive pools.

---

## 4. Planned Enhancements & Wishlist (`[WISHLIST]`)

### [WISHLIST: Kopia-Block-Delta-App-Snapshots] | Aliases: kopia snapshots, deduplicated app backup, fastcdc app state
> **1-Line Summary:** Optional Kopia FastCDC block-delta snapshot bridge if custom `.exe` app workspaces grow too large for whole-file sync.
* **User Intention:** Deduplicate large multi-file app state directories before cloud upload.

### [WISHLIST: Virtual-Drive-Mount-WinFsp] | Aliases: winfsp mount, virtual windows drive, rclone mount
> **1-Line Summary:** Optional post-v1 Windows virtual drive letter mounting via WinFsp + Rclone VFS once core catalog and sync pipelines are stable.
* **User Intention:** Browse pooled cloud storage directly inside Windows File Explorer without premature kernel-driver complexity in v1.
