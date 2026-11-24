# Third-Party Licenses and Open-Source Notices

This project builds upon and supervises mature open-source tools and libraries. Custom Cloud Manager code is written strictly in Go and TypeScript, while reused external dependencies follow their respective upstream licenses. Exact versions will be pinned as Phase 1+ scaffolding proceeds.

## 1. Ships Inside / Reused at Runtime (`[PLANNED]`)

| Component | License | Origin / Upstream Repository | Purpose in Cloud Manager |
| :--- | :--- | :--- | :--- |
| **Rclone** (`>= v1.66`) | MIT License | https://github.com/rclone/rclone | Multi-cloud transport daemon (`rcd`), `union`, `hasher`, Mega & Google Drive sync |
| **Syncthing** *(Unmodified Upstream)* | MPL-2.0 | https://github.com/syncthing/syncthing | Two-way LAN-only folder delta sync controlled via REST API + Event Stream |
| **Kopia** | Apache-2.0 | https://github.com/kopia/kopia | FastCDC block-delta snapshots for large custom `.exe` app directories |
| **Android Debug Bridge (ADB)** | Apache-2.0 | https://developer.android.com/tools/adb | Wireless QR & USB MediaStore / phone folder delta inspection and transfer |
| **SQLite 3** | Public Domain | https://www.sqlite.org/ | Local metadata catalog, FTS5 search index, WAL job queue, and cursor/quota ledgers |
| **Google APIs Go Client** | BSD-3-Clause | https://github.com/googleapis/google-api-go-client | Official Go YouTube Data API v3 SDK for private resumable uploads |
| **TanStack Virtual** | MIT License | https://github.com/TanStack/virtual | 60 FPS virtualized file explorer rendering |
| **Lucide Icons** | ISC License | https://github.com/lucide-icons/lucide | Unified minimalist SVG iconography across Light/Dark/System themes |

## 2. Build / Test Only — Not Shipped (`[PLANNED]`)

| Component | License | Origin / Upstream Repository | Purpose in Build Pipeline |
| :--- | :--- | :--- | :--- |
| **Go Toolchain** | BSD-3-Clause | https://github.com/golang/go | Compiler, static checker (`go vet`), race detector (`go test -race`), portable `.exe` build |
| **TypeScript** | Apache-2.0 | https://github.com/microsoft/TypeScript | Static type checking for frontend UI |
| **Vite** | MIT License | https://github.com/vitejs/vite | Frontend bundling and asset generation |
| **Vitest** | MIT License | https://github.com/vitest-dev/vitest | Unit and UI component tests |
| **Playwright** *(Optional E2E)* | Apache-2.0 | https://github.com/microsoft/playwright | Headless UI, keyboard/a11y, and WCAG AA contrast verification |
