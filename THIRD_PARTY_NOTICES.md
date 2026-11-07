# Third-Party Licenses and Open-Source Notices

This project incorporates and builds upon open-source libraries and engines. Below is the full attribution and license registry divided into runtime components and build-time dependencies.

## 1. Ships Inside the App (Runtime Binaries & Libraries)

| Component | License | Origin / Upstream Repository | Purpose in Cloud Manager |
| :--- | :--- | :--- | :--- |
| **Rclone Core** | MIT License | [https://github.com/rclone/rclone](https://github.com/rclone/rclone) | Multi-cloud transport daemon, Mega crypto & Google Drive sync engine |
| **Tauri v2** | MIT / Apache-2.0 | [https://github.com/tauri-apps/tauri](https://github.com/tauri-apps/tauri) | Lightweight native desktop shell, IPC bridge, and window management |
| **SQLite 3** | Public Domain | [https://www.sqlite.org/](https://www.sqlite.org/) | Local metadata catalog, FTS5 search index, WAL-mode state storage |
| **Rusqlite** | MIT License | [https://github.com/rusqlite/rusqlite](https://github.com/rusqlite/rusqlite) | Ergonomic Rust bindings for local SQLite operations |
| **reqwest** | MIT / Apache-2.0 | [https://github.com/seanmonstar/reqwest](https://github.com/seanmonstar/reqwest) | HTTP client for YouTube Data API v3 and Rclone JSON-RPC communication |
| **TanStack Virtual** | MIT License | [https://github.com/TanStack/virtual](https://github.com/TanStack/virtual) | 60 FPS virtualized list rendering for 50,000+ files in UI |
| **Svelte** | MIT License | [https://github.com/sveltejs/svelte](https://github.com/sveltejs/svelte) | Reactive frontend user interface layer |
| **Tailwind CSS** | MIT License | [https://github.com/tailwindlabs/tailwindcss](https://github.com/tailwindlabs/tailwindcss) | Utility-first dark-mode styling framework |
| **Lucide Icons** | ISC License | [https://github.com/lucide-icons/lucide](https://github.com/lucide-icons/lucide) | Visual icons for storage meters, cloud providers, and sync states |

## 2. Build / Test Only (Not Shipped in Binary)

| Component | License | Origin / Upstream Repository | Purpose in Build Pipeline |
| :--- | :--- | :--- | :--- |
| **TypeScript** | Apache-2.0 | [https://github.com/microsoft/TypeScript](https://github.com/microsoft/TypeScript) | Static type safety for frontend application code |
| **Vite** | MIT License | [https://github.com/vitejs/vite](https://github.com/vitejs/vite) | Frontend bundling and development hot-reloading |
| **Vitest** | MIT License | [https://github.com/vitest-dev/vitest](https://github.com/vitest-dev/vitest) | Fast unit test runner for frontend logic |
| **Cargo / Rustc** | MIT / Apache-2.0 | [https://github.com/rust-lang/rust](https://github.com/rust-lang/rust) | Compiler and package manager for Rust backend |
