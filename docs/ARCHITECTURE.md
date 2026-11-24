# System Architecture (`ARCHITECTURE.md`)
> **1-Line Summary:** Plain-English architectural blueprint of Cloud Manager (`[PLANNED]`), covering AI-friendly stack policy, security boundaries, subsystem data flows, and verification plans.

## 1. High-Level Visual System Map

```mermaid
flowchart TB
    classDef uiNode fill:#1d4ed8,stroke:#1e3a8a,color:#ffffff
    classDef coreNode fill:#0f766e,stroke:#115e59,color:#ffffff
    classDef secNode fill:#b45309,stroke:#78350f,color:#ffffff
    classDef adapterNode fill:#6d28d9,stroke:#4c1d95,color:#ffffff
    classDef targetNode fill:#334155,stroke:#0f172a,color:#ffffff

    subgraph UI["TypeScript Frontend (WCAG AA + TanStack Virtual)"]
        EXP["Virtual Explorer + FTS5 Search"]:::uiNode
        RULES_UI["Smart Routing + Dry-Run Bench"]:::uiNode
        MEDIA_UI["Wi-Fi P2P + ADB Media Center"]:::uiNode
        YT_UI["YouTube Private Archive Monitor"]:::uiNode
    end

    subgraph Core["Hardened Core Daemon (cloud-managerd: Go or Python)"]
        AUTH["Security Guard (Bearer, Origin 403, Host, Path Jail)"]:::secNode
        BACKPLANE["Loopback API / MCP (127.0.0.1:59432)"]:::coreNode
        SQL[("SQLite WAL + FTS5 (user_version, cursors, jobs)")]:::coreNode
        RULE_ENG["Rule & Quota Engine (priority ASC + In-Flight Lock)"]:::coreNode
        INGEST["Unified SHA-256 Ingestion & Dedup Pipeline"]:::coreNode
    end

    subgraph Adapters["Isolated Adapters (Job Object Supervised)"]
        LAN_P2P["Same-Router Wi-Fi P2P (HTTPS TLS :53317, OFF default)"]:::secNode
        ADB["Optional ADB Worker (Argv-Only + CRLF Trim)"]:::adapterNode
        YT_CLIENT["YouTube Data API v3 (Resumable + Quota Ledger)"]:::adapterNode
        RCLONE["rclone rcd (127.0.0.1:0, 256-bit RC Auth, DPAPI Pass)"]:::adapterNode
    end

    subgraph Targets["External Devices & Cloud Targets"]
        PHONE["Android Phone (LocalSend / Syncthing LAN / ADB)"]:::targetNode
        EXE_CLIENTS["Custom Windows .EXE Apps"]:::targetNode
        YT_CLOUD["YouTube Channel (Private Playlists)"]:::targetNode
        GDRIVE["Google Drive Remotes (Changes.list + bisync)"]:::targetNode
        MEGA["Mega.nz Remotes (hasher Overlay + Append Tiers)"]:::targetNode
    end

    UI <-->|Bearer Auth + <=5Hz SSE| AUTH
    EXE_CLIENTS <-->|HTTP POST :59432 + Bearer| AUTH
    AUTH --> BACKPLANE
    BACKPLANE <--> SQL
    BACKPLANE --> RULE_ENG

    PHONE <-->|HTTPS TLS + PIN :53317| LAN_P2P
    PHONE <-->|USB / Wi-Fi ADB| ADB
    LAN_P2P --> INGEST
    ADB --> INGEST
    INGEST --> SQL
    INGEST --> RULE_ENG

    RULE_ENG -->|Videos| YT_CLIENT
    RULE_ENG -->|Pooled Cloud Files| RCLONE
    YT_CLIENT -->|OAuth2 Resumable| YT_CLOUD
    RCLONE <-->|Google REST API| GDRIVE
    RCLONE <-->|hasher + Pool Crypto| MEGA

    subgraph Legend["Legend (Color Guide)"]
        L1["Blue = TypeScript UI & UX Layer"]:::uiNode
        L2["Teal = Core Daemon, SQLite & Routing"]:::coreNode
        L3["Amber = Security Guards & Opt-In TLS Boundary"]:::secNode
        L4["Purple = Isolated Process & Cloud Adapters"]:::adapterNode
        L5["Slate = External Devices & Cloud Destinations"]:::targetNode
    end
```

* **How It Connects (Plain English):** All UI traffic and local `.exe` sync requests pass through the authenticated `127.0.0.1:59432` guard before touching SQLite or routing rules. Phone transfers over opt-in Wi-Fi P2P (`:53317`) or ADB merge into one SHA-256 deduplication pipeline, while `rclone rcd` and YouTube run as isolated worker adapters supervised by Windows Job Objects.

---

## 2. Tech Stack & AI-Friendly Language Policy

* **Custom-Written Code Policy:** Custom code is restricted strictly to **TypeScript/JavaScript, Go, or Python** so AI coding agents can maintain, refactor, and verify every layer cleanly. Reused open-source binaries and libraries may use any language.

| Layer | Planned Stack / OSS Reuse | Plain-English Rationale |
| :--- | :--- | :--- |
| **Core Daemon (`[PLANNED]`)** | **Go (`cloud-managerd.exe`)** or **Python 3.12+ (`uv`, FastAPI)** *(pending stack lock)* | Single-language clarity, strong HTTP/SQLite/process tooling, zero Rust borrow-checker friction. |
| **Frontend UI (`[PLANNED]`)** | **TypeScript + Vite + `TanStack Virtual` + `Lucide`** | 60 FPS virtual scrolling, strict typing, Light/Dark/System WCAG AA tokens, offline-safe bundles. |
| **Local Catalog & Queue** | **SQLite 3** (`WAL`, `busy_timeout = 5000`, `FTS5`, `user_version`) | Sub-5 ms offline search, durable job queue, remote change cursors, and rolling `.bak` safety. |
| **Multi-Cloud Transport** | **Rclone Core** (`>= v1.66`, `rcd`, `union`, `hasher`) *(Go OSS)* | Battle-tested Mega encryption, Google Drive `Changes.list`, and pool balancing. |
| **Phone Sync (Wi-Fi + ADB)** | **LocalSend v2 Protocol** (`:53317`) + optional **Syncthing LAN-Only** + **ADB** | Zero custom Android app code required; works over home Wi-Fi with USB ADB fallback. |
| **Video Archive** | **YouTube Data API v3** (official Go/Python SDK or `porjo/youtubeuploader`) | Enforces `privacyStatus = "private"`, playlist routing, and resumable chunk recovery. |

---

## 3. Core Subsystems & Security Boundaries (`[PLANNED]`)

### [SUBSYSTEM: Security-Boundaries-And-Hardening] | Aliases: security boundaries, loopback auth, rclone encryption, job object, data integrity
> **1-Line Summary:** Enforces fail-closed loopback authentication, encrypted secrets, TLS LAN isolation, argv-only subprocesses, and atomic SQLite/file integrity across every boundary.

* **Loopback Backplane (`127.0.0.1:59432`):** Binds strictly to `127.0.0.1` (fail closed if overridden); requires `Authorization: Bearer <token>` (or `X-Local-Token`) from `%USERPROFILE%\.local\share\ecosystem-secrets.json` (`tokens.cloudManager`); rejects any `Origin` header with `403 Forbidden`; enforces `Host` matching `^127\.0\.0\.1:59432$|^localhost:59432$`; validates `?app=` against `^[a-z0-9][a-z0-9_-]{0,63}$`.
* **Path & Subprocess Isolation:** Executes all external binaries (`rclone`, `adb`, `syncthing`) strictly via argv arrays (`shell=False` / `exec.Command` / `execFile`). Canonicalizes incoming filenames and rejects `..`, drive letters, UNC paths, symlinks/junctions, null bytes, and NTFS reserved names (`CON`, `PRN`, `AUX`, `NUL`, `COM1`–`COM9`, `LPT1`–`LPT9`).
* **Rclone & Secret Protection:** Encrypts `rclone.conf` using a master key protected by Windows DPAPI and injected via `RCLONE_CONFIG_PASS` environment variable (never CLI argv or plaintext disk); binds `rclone rcd` to `127.0.0.1:0` with ephemeral 256-bit `--rc-user`/`--rc-pass` and `--rc-allow-origin ""`; attaches child processes to a Windows **Job Object** (`JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE`).
* **Log & Telemetry Redaction:** Passes all JSONL logs and UI SSE events (`<= 5 Hz`) through middleware stripping `Authorization`, `X-Local-Token`, OAuth tokens, passwords, and `RCLONE_*` fields.
* **Data Integrity & Backups:** Gates schema changes behind `PRAGMA user_version` migrations with pre-migration `.bak` copies (retaining last 5); writes config files atomically (`temp -> fsync -> rename`, UTF-8 without BOM); captures live `.exe` app SQLite DBs via `VACUUM INTO`; preserves conflicting edits via `(sha256, parent_revision)` as `<name>.conflict-<host>-<utc>.<ext>`.

---

### [SUBSYSTEM: Unified-Phone-Ingestion-WiFi-And-ADB] | Aliases: wifi phone sync, lan p2p, adb sync, mediastore compare, sha256 dedup
> **1-Line Summary:** Combines opt-in Same-Router Wi-Fi P2P (`:53317`) and ADB MediaStore queries into one streaming SHA-256 deduplication and filing pipeline.

```mermaid
flowchart LR
    classDef wifiNode fill:#0284c7,stroke:#0369a1,color:#ffffff
    classDef adbNode fill:#7c3aed,stroke:#5b21b6,color:#ffffff
    classDef pipeNode fill:#0f766e,stroke:#115e59,color:#ffffff
    classDef destNode fill:#334155,stroke:#0f172a,color:#ffffff

    WIFI_SRC["Phone over Home Wi-Fi (LocalSend v2 / Syncthing LAN)"]:::wifiNode
    TLS_GUARD["TLS :53317 + PIN/Cert Pin + Filename Jail"]:::wifiNode
    ADB_SRC["Phone over USB/Wi-Fi ADB"]:::adbNode
    ADB_QUERY["MediaStore Query (images+video, UTC sec, CRLF Trim)"]:::adbNode
    SHA_PIPE["Streaming SHA-256 Check + Free-Space Guard"]:::pipeNode
    SQL_DEDUP[("SQLite Catalog Dedup + YYYY/MM Move")]:::pipeNode
    POOL_Q["Smart Router & Rclone Queue"]:::destNode

    WIFI_SRC -->|Opt-In ON| TLS_GUARD --> SHA_PIPE
    ADB_SRC --> ADB_QUERY -->|New Items Only| SHA_PIPE
    SHA_PIPE --> SQL_DEDUP --> POOL_Q

    subgraph Legend["Legend (Color Guide)"]
        LW["Blue = Same-Router Wi-Fi P2P Path"]:::wifiNode
        LA["Purple = ADB MediaStore Delta Path"]:::adbNode
        LP["Teal = Canonical SHA-256 & Disk Safety Pipeline"]:::pipeNode
        LD["Slate = Smart Router & Cloud Upload Queue"]:::destNode
    end
```

* **How It Works (Plain English):** Both Wi-Fi transfers (`:53317` LocalSend v2 HTTPS/PIN or optional Syncthing LAN-only folder) and ADB pulls (`images/media` + `video/media` with UTC seconds normalization) pass free-disk pre-flight checks (`free > incoming * 1.2 + 2 GiB`) and streaming SHA-256 deduplication before routing to cloud storage.
* **Status:** `[PLANNED]` — `staging/` path configurable to secondary drives (`D:`/`E:`) with failed-artifact TTL cleanup.

---

### [SUBSYSTEM: YouTube-Resumable-Video-Pipeline] | Aliases: youtube uploader, private video sync, quota ledger, video chunker
> **1-Line Summary:** Streams 16 MB video chunks to private YouTube playlists with persistent resumable URIs and daily PT quota accounting.

```mermaid
flowchart LR
    classDef qNode fill:#b45309,stroke:#78350f,color:#ffffff
    classDef apiNode fill:#be123c,stroke:#881337,color:#ffffff
    classDef dbNode fill:#0f766e,stroke:#115e59,color:#ffffff

    QUEUE["Queued Video (Smart Router / UI)"]:::dbNode
    LEDGER["Check youtube_quota_ledger (Cap 9,600 PT/day)"]:::qNode
    SESSION["POST Resumable Init (privacyStatus = private)"]:::apiNode
    CHUNK["Stream 16 MB Chunks (Save Location URI in SQLite)"]:::apiNode
    PLAYLIST["POST playlistItems.insert + Record Video ID"]:::dbNode

    QUEUE --> LEDGER -->|Units Available| SESSION --> CHUNK -->|200 OK| PLAYLIST
    LEDGER -->|Over Cap| DEFER["Defer to Next PT Midnight"]:::qNode

    subgraph Legend["Legend (Color Guide)"]
        LQ["Amber = Quota Guard & PT Midnight Deferral"]:::qNode
        LA["Rose = YouTube Resumable HTTP Session"]:::apiNode
        LD["Teal = Queue, Playlist & SQLite Persistence"]:::dbNode
    end
```

* **How It Works (Plain English):** Checks `youtube_quota_ledger` (1,650 units per video + playlist insert) before opening a `private` resumable session, saves the `Location` URI in SQLite so crashes resume without re-burning 1,600 units, and skips polling `videos.list` loops.

---

### [SUBSYSTEM: Localhost-App-Backplane] | Aliases: app sync api, custom exe persistence, port 59432, vacuum into
> **1-Line Summary:** Authenticated `127.0.0.1:59432` HTTP + `/mcp` server accepting `VACUUM INTO` app snapshots and queueing cloud sync offline-first.

* **How It Works (Plain English):** Custom Windows `.exe` apps authenticate with Bearer tokens from `ecosystem-secrets.json`, export clean SQLite snapshots via `VACUUM INTO` (avoiding `-wal`/`-shm` tears), and post to `127.0.0.1:59432/api/v1/sync?app=<slug>`. State saves immediately to local storage/SQLite offline and syncs to Google Drive asynchronously with `Retry-After` / exponential backoff.

---

### [SUBSYSTEM: Rclone-Pool-And-Catalog-Reconciliation] | Aliases: rclone hasher, gdrive changes, mega append, sqlite cursors
> **1-Line Summary:** Reconciles out-of-band cloud changes using Google Drive `changes.list` tokens and Mega `hasher` recursive tree diffs into SQLite tombstones.

* **How It Works (Plain English):** Tracks `remote_cursors (remote_id, cursor_token, last_reconciled_at)` and `tombstones` in SQLite. Google Drive syncs via `changes.list` page tokens; Mega remotes run under Rclone `hasher` with append/archive semantics and `509` bandwidth backoff so missing native `ModTime` never causes re-upload loops.

---

## 4. Verification & CI Pipeline (`[PLANNED]`)

### [FLOW: Automated-Verification-And-CI-Pipeline] | Aliases: ci workflow, security regression, test suite, portable build
> **1-Line Summary:** Defines GitHub Actions CI checks, security regression gates, and tag-triggered portable `.exe` builds once the core stack is locked.

* **Pipeline Target:** `.github/workflows/ci.yml` (bootstrapped from `agent-templates/templates/ci.yml` after stack lock).
* **Required Gates:** Static lint/type checks (`tsc` + `go vet` or `ruff`/`mypy`), mocked adapter unit/integration suites (Rclone JSON-RPC, ADB `\r\r\n` fixtures, YouTube quota/resumable chunks, LocalSend LAN loopback), and security regression checks (`401` missing auth, `403` `Origin`, bad `Host`, path traversal, secret redaction).
* **Release Packaging:** Tag-triggered (`vX.Y.Z`) GitHub Actions release producing a Windows 10/11 x64 portable `.exe` (`CloudManager.exe`).

---

## 5. Extension Recipes (`[PLANNED]`)
1. **Add a Cloud Provider:** Add an Rclone remote capability descriptor (hash support, `ModTime` reliability, rate-limit policy) in the Rclone adapter config + UI quota meter component.
2. **Add a Routing Predicate:** Extend the priority-ordered rule matcher schema, dry-run simulator test suite, and Rule Editor form controls.
