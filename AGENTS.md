# Project Operating Instructions (`AGENTS.md`)
> **Notice:** This project inherits user profile, confirmation gates, documentation blueprints, and master sync protocols from the global master `AGENTS.md`. Do not duplicate those rules here.

## 1. Project Tech Stack & Environment
* **Project Name:** Cloud Manager (`cloud-manager`)
* **Host / Desktop Framework:** Tauri v2 (Rust Core + Svelte / TypeScript / Tailwind CSS)
* **Storage & Transport Daemon:** Rclone Core (embedded headless daemon via JSON-RPC / VFS)
* **Local Database & Cache Engine:** SQLite 3 (WAL mode enabled, FTS5 full-text search)
* **Mobile Sync Engine:** Android Debug Bridge (ADB) CLI bridge via direct subprocess wrapper
* **Video Automation:** YouTube Data API v3 client (HTTP resumable video uploader)
* **Target OS / Build Output:** Windows 10/11 x64 standalone portable `.exe` (`CloudManager.exe`)

## 2. Mandatory Verification & Build Commands
* **Type Check (Frontend):** `npm run check`
* **Rust Core Check:** `cargo check --manifest-path src-tauri/Cargo.toml`
* **Linting:** `npm run lint`
* **Unit & Pipeline Tests:** `npm test && cargo test --manifest-path src-tauri/Cargo.toml`
* **Portable Desktop Packaging:** `npm run tauri build` (produces `src-tauri/target/release/bundle/nsis/CloudManager.exe`)

## 3. Repository Directory Structure & Conventions
* `src/` - Svelte & TypeScript frontend UI (virtualized file grid, routing rule editor, account meters, ADB transfer wizard).
* `src-tauri/` - Rust desktop host, IPC handlers, SQLite database manager, and child process lifecycle manager.
* `src-tauri/src/rclone/` - Rclone JSON-RPC client and daemon process supervisor.
* `src-tauri/src/adb/` - Android Debug Bridge device discovery, MediaStore inspector, and incremental transfer worker.
* `src-tauri/src/youtube/` - YouTube Data API v3 OAuth2 token manager and resumable video uploader.
* `src-tauri/src/backplane/` - Localhost REST/WebDAV server on port `59432` for third-party and custom `.exe` apps.
* `docs/` - Core documentation suite (`PRODUCT.md`, `ARCHITECTURE.md`, `DECISIONS_AND_TRAPS.md`, `VISUAL_GUIDE.html`).

## 4. Repository-Specific Engineering Traps
* **TRAP 1: Mega.nz Node Invalidation:** Never initiate concurrent raw upload sessions bypassing Rclone's session manager; Mega limits concurrent socket revisions per session and will return `ENOENT` / node desync errors.
* **TRAP 2: SQLite WAL Multi-Process Locking:** The background sync worker and the desktop GUI must connect to SQLite with `PRAGMA busy_timeout = 5000;` and WAL mode enabled, or write locks will cause UI freezing.
* **TRAP 3: ADB Shell Line-Ending Mangling:** When executing `adb shell` commands on Windows, strip trailing `\r\r\n` (CRLF doubling) before parsing file sizes and timestamps.
* **TRAP 4: YouTube API Daily Quota Ceiling:** YouTube Data API v3 grants 10,000 quota units per day by default; video upload costs 1,600 units. Never poll video status repeatedly; rely on the resumable upload completion response.
