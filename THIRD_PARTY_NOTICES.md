# Third-Party Licenses and Open-Source Notices

This project builds upon and integrates mature open-source tools and libraries. While custom Cloud Manager code is written in TypeScript/JavaScript, Go, or Python, reused open-source dependencies below follow their respective upstream licenses. Exact versions will be pinned as scaffolding proceeds.

## 1. Ships Inside / Reused at Runtime (`[PLANNED]`)

| Component | License | Origin / Upstream Repository | Purpose in Cloud Manager |
| :--- | :--- | :--- | :--- |
| **Rclone** (`>= v1.66`) | MIT License | https://github.com/rclone/rclone | Multi-cloud transport daemon (`rcd`), `union`, `hasher`, Mega & Google Drive sync |
| **SQLite 3** | Public Domain | https://www.sqlite.org/ | Local metadata catalog, FTS5 search index, WAL-mode job queue & cursor ledger |
| **LocalSend v2 Protocol** *(+ Official Android App)* | Apache-2.0 | https://github.com/localsend/localsend | Same-router HTTPS/TLS Wi-Fi P2P transfer protocol (`:53317`) |
| **Syncthing** *(Optional LAN-Only Bridge)* | MPL-2.0 | https://github.com/syncthing/syncthing | Optional unattended same-router folder sync (`Global Discovery` / relays disabled) |
| **Android Debug Bridge (ADB)** *(Optional Bridge)* | Apache-2.0 | https://developer.android.com/tools/adb | Optional USB/Wi-Fi MediaStore inspection & bulk media recovery |
| **TanStack Virtual** | MIT License | https://github.com/TanStack/virtual | 60 FPS virtualized file explorer rendering |
| **Lucide Icons** | ISC License | https://github.com/lucide-icons/lucide | Unified minimalist SVG iconography across Light/Dark themes |
| **youtubeuploader** *(Optional CLI Candidate)* | Apache-2.0 | https://github.com/porjo/youtubeuploader | Candidate resumable private YouTube playlist upload adapter |

## 2. Build / Test Only — Not Shipped (`[PLANNED]`)

| Component | License | Origin / Upstream Repository | Purpose in Build Pipeline |
| :--- | :--- | :--- | :--- |
| **TypeScript** | Apache-2.0 | https://github.com/microsoft/TypeScript | Static type checking for frontend UI |
| **Vite** | MIT License | https://github.com/vitejs/vite | Frontend bundling and dev server |
| **Vitest** | MIT License | https://github.com/vitest-dev/vitest | Unit and UI component tests |
| **Playwright** *(Optional E2E)* | Apache-2.0 | https://github.com/microsoft/playwright | Headless UI, keyboard/a11y, and theme contrast verification |
