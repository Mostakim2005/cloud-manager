# Product Intent & Feature Map (`PRODUCT.md`)
> **1-Line Summary:** The permanent source of truth for user intentions, active feature capabilities, and future roadmap for the Cloud Manager desktop system.

## 1. Core Vision & User Experience Intent
* **What This App Does (Plain English):** Cloud Manager aggregates multiple personal Google Drive accounts and Mega.nz accounts into a unified, high-speed virtual filesystem. It intelligently routes files to specific cloud targets based on origin directory, file extension, or size, automatically manages phone-to-PC photo sync via Android Debug Bridge (ADB), automates private video uploads to YouTube playlists, and provides a local background sync API for custom Windows `.exe` desktop apps.
* **How It Must Feel to Use:** Responsive, instantaneous, and zero-freeze. The UI loads thousands of virtual files in under 5 milliseconds from a local SQLite cache without waiting for slow cloud network requests. Live transfer meters provide real-time feedback on upload speeds and remaining bytes.

---

## 2. Active Features (Do Not Break)

### [FEATURE: Multi-Cloud-Storage-Pooling] | Aliases: account manager, drive pooling, quota aggregation, mega gdrive merge
> **1-Line Summary:** Combines multiple independent Google Drive and Mega.nz accounts into a cohesive virtual storage pool displayed in a single unified view.
* **User Intention (Non-Negotiable):** The user has multiple free and paid cloud accounts across Google and Mega. The app must allow logging into all of them simultaneously, visualize individual and aggregated quota meters, and present files as if they exist on a single local drive.
* **Visual / UI Behavior:** The left navigation sidebar displays each connected account with a colored progress bar showing used vs. total gigabytes (e.g., `GDrive #1: 10.2/15 GB`, `Mega #1: 18.4/20 GB`, `Pool Total: 175 GB`).
* **Code Anchors (Grep Targets):**
  * `src-tauri/src/rclone/account_manager.rs` -> `AccountManager`, `list_connected_remotes`
  * `src/lib/components/StorageMeter.svelte` -> `storagePoolTotal`, `accountQuotas`

---

### [FEATURE: Smart-Routing-Rule-Engine] | Aliases: automated routing, file sorting, origin rules, extension dispatch
> **1-Line Summary:** Dynamically dispatches incoming and modified files to designated cloud accounts based on user-defined matching criteria.
* **User Intention (Non-Negotiable):** Avoid manual drag-and-drop sorting. Files must automatically flow to their intended storage tier (e.g., PDFs to Google Drive #2, large raw media to Mega pools, note data to Google Drive #1) based on origin directory, extension patterns, or file size thresholds.
* **Visual / UI Behavior:** Dedicated "Smart Routing" panel allowing the user to create, toggle, and prioritize rules with match chips (e.g., `*.pdf` $\to$ GDrive Books; `> 100MB` $\to$ Mega Pool).
* **Code Anchors (Grep Targets):**
  * `src-tauri/src/rules/engine.rs` -> `RuleMatcher`, `evaluate_target_account`
  * `src/lib/components/RuleEditorModal.svelte` -> `saveRuleConfig`, `activeRulesList`

---

### [FEATURE: ADB-Phone-Photo-Delta-Sync] | Aliases: phone sync, adb transfer, photo importer, dcim pull
> **1-Line Summary:** Connects to an Android phone over ADB, compares photo/video metadata against the local database, and pulls only newly captured files.
* **User Intention (Non-Negotiable):** The user connects their phone periodically via USB/WiFi ADB. The app must instantly identify which photos and videos on the phone have already been copied to PC and which are new, avoiding redundant multi-gigabyte copying.
* **Visual / UI Behavior:** Dedicated "Phone Ingestion Stream" view displaying connected device serial, total phone media count, count of already-synced items, and a one-click button: `Import Unsynced Photos (N items, X MB)`.
* **Code Anchors (Grep Targets):**
  * `src-tauri/src/adb/client.rs` -> `AdbDeviceManager`, `fetch_phone_mediastore`
  * `src-tauri/src/adb/sync_worker.rs` -> `pull_delta_photos`, `verify_imported_hash`
  * `src/lib/components/PhoneSyncModal.svelte` -> `deviceConnectionState`, `executeAdbPull`

---

### [FEATURE: YouTube-Private-Video-Pipeline] | Aliases: youtube uploader, private video sync, playlist automation
> **1-Line Summary:** Automatically uploads designated video files to a linked YouTube channel as private videos assigned to pre-configured playlists.
* **User Intention (Non-Negotiable):** Eliminate manual browser uploads for screen recordings and archives. When a video is placed into a designated folder or marked in the UI, it uploads via the YouTube Data API v3 with privacy forced to `private` and is placed into a user-selected playlist.
* **Visual / UI Behavior:** A specialized pipeline card showing YouTube OAuth status, selected default playlist, privacy badge (`🔒 Private (Enforced)`), and live resumable upload progress bar with upload speed (MB/s).
* **Code Anchors (Grep Targets):**
  * `src-tauri/src/youtube/uploader.rs` -> `YouTubeUploader`, `upload_resumable_video`
  * `src/lib/components/YouTubePipelineCard.svelte` -> `playlistSelector`, `uploadProgressBar`

---

### [FEATURE: Localhost-App-Backplane-API] | Aliases: exe sync, custom app api, localhost backplane, cross-app state
> **1-Line Summary:** Exposes a lightweight local HTTP REST endpoint on port `59432` allowing custom desktop `.exe` and note-taking apps to sync state effortlessly.
* **User Intention (Non-Negotiable):** Custom Windows desktop applications and note-taking tools need cross-device data persistence without bundling complex Google or Mega cloud SDKs into every binary. They simply ping `localhost:59432` to push or pull changes.
* **Visual / UI Behavior:** "Local Backplane" status card displaying the listener status (`127.0.0.1:59432 LISTENING`), active connected clients, and real-time request log.
* **Code Anchors (Grep Targets):**
  * `src-tauri/src/backplane/server.rs` -> `start_local_backplane`, `handle_app_sync`
  * `src/lib/components/BackplaneStatus.svelte` -> `activeClientList`, `recentSyncEvents`

---

### [FEATURE: Virtualized-File-Explorer-GUI] | Aliases: file grid, visual explorer, lazy loading, upload progress
> **1-Line Summary:** Displays all aggregated files in a 60 FPS virtualized table cached in SQLite, with real-time transfer progress bars.
* **User Intention (Non-Negotiable):** Fast navigation across 50,000+ files. The user must see exactly where each file is stored, its sync status (`Synced`, `Uploading`, `Local-Only`, `Cloud-Only`), and instant full-text search results.
* **Visual / UI Behavior:** Modern dark-theme file grid with sticky column headers, search bar with debounce, status chips, and per-row upload speed indicators.
* **Code Anchors (Grep Targets):**
  * `src/lib/components/VirtualFileExplorer.svelte` -> `virtualListConfig`, `searchQuery`
  * `src-tauri/src/db/catalog.rs` -> `query_virtual_catalog`, `update_sync_state`

---

## 3. Planned Features & App-Merge Wishlist

### [WISHLIST: Google-Photos-API-Target] | Aliases: gphotos backup, google photos upload, mobile photo album
> **1-Line Summary:** Direct album-based upload to Google Photos as an optional alternate target for phone photo streams.
* **User Intention:** Allow routing compressed or high-value family photos directly to Google Photos albums alongside the raw file backup in Mega.
* **Target Integration Point:** `src-tauri/src/rules/engine.rs` -> `evaluate_target_account`

### [WISHLIST: P2P-WiFi-Direct-Phone-Sync] | Aliases: wireless phone sync, syncthing receiver, local wifi photo transfer
> **1-Line Summary:** Wirelessly sync photos and app state from phone to PC over local Wi-Fi without requiring a USB cable.
* **Target Integration Point:** `src-tauri/src/adb/client.rs` -> `AdbDeviceManager::connect_wifi`
