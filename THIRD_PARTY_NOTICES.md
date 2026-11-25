# Third-Party Licenses and Open-Source Notices

This project builds upon and supervises mature open-source tools and libraries. Custom Cloud Manager code is written strictly in Go and TypeScript under PolyForm Noncommercial 1.0.0, while third-party packages and external tools remain under their respective upstream licenses.

## 1. Ships Inside / Reused at Runtime

| Component | License | Origin / Upstream Repository | Purpose in Cloud Manager |
| :--- | :--- | :--- | :--- |
| **`modernc.org/sqlite`** | BSD-3-Clause | https://gitlab.com/cznic/sqlite | Pure-Go zero-CGO SQLite 3 driver (WAL, FTS5, `VACUUM INTO`, `user_version`) |
| **`github.com/skip2/go-qrcode`** | MIT License | https://github.com/skip2/go-qrcode | Pure-Go QR matrix generation for Syncthing, Wireless ADB, and LAN Drop-Zone SVG pairing |
| **`golang.org/x/sys`** | BSD-3-Clause | https://go.googlesource.com/sys | Windows Job Objects, DPAPI (`CryptProtectData`), disk headroom syscalls, Win32 tray APIs |
| **`golang.org/x/oauth2` & `google.golang.org/api`** | BSD-3-Clause | https://github.com/googleapis/google-api-go-client | Official Go YouTube Data API v3 SDK and OAuth2 client for private resumable uploads |
| **`@tanstack/virtual-core`** (`^3.13.0`) | MIT License | https://github.com/TanStack/virtual | 60 FPS virtualized file explorer list rendering |
| **`lucide`** (`^0.468.0`) | ISC License | https://github.com/lucide-icons/lucide | Unified minimalist SVG iconography (`currentColor`, `stroke-width="1.75"`) |
| **Rclone** (`>= v1.66`, external binary) | MIT License | https://github.com/rclone/rclone | Multi-cloud transport daemon (`rcd`), `union`, `hasher`, Mega & Google Drive sync |
| **Syncthing** *(Unmodified external binary)* | MPL-2.0 | https://github.com/syncthing/syncthing | Two-way LAN-only folder delta sync controlled via REST API |
| **Kopia** *(External binary)* | Apache-2.0 | https://github.com/kopia/kopia | FastCDC block-delta snapshots for large custom `.exe` app directories |
| **Android Debug Bridge (ADB)** *(External binary)* | Apache-2.0 | https://developer.android.com/tools/adb | Wireless QR & USB MediaStore / phone folder delta inspection and transfer |
| **QRCode Generator** *(SharedLibraries optional offline bundle)* | MIT License | https://github.com/kazuhikoarase/qrcode-generator | Shared offline QR matrix JS bundle installed under `%LOCALAPPDATA%\SharedLibraries\qrcode` |

## 2. Build / Test Only — Not Shipped

| Component | License | Origin / Upstream Repository | Purpose in Build Pipeline |
| :--- | :--- | :--- | :--- |
| **Go Toolchain** (`1.24+`) | BSD-3-Clause | https://github.com/golang/go | Compiler, `go vet`, `-race` detector, zero-CGO Windows `.exe` cross-compile |
| **TypeScript** (`^5.7.3`) | Apache-2.0 | https://github.com/microsoft/TypeScript | Static type checking (`tsc --noEmit`) |
| **Vite** (`^6.1.0`) | MIT License | https://github.com/vitejs/vite | Frontend bundling into `internal/webui/dist` for `go:embed` |
| **ESLint & `typescript-eslint`** | MIT License | https://github.com/eslint/eslint | Static linting for TypeScript UI sources |
| **Vitest & JSDOM** | MIT License | https://github.com/vitest-dev/vitest | Unit, contrast, keyboard ergonomics, and DOM tests |
