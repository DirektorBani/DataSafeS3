# DataSafeS3 Desktop 1.0

Tauri 2 shell for Windows and Linux, version `1.0.0-dev`. Profiles and sign-in go through the Go agent `datasafe-agent` on stdin. The window does not call the DataSafe API. Build the agent before the shell:

```bash
mkdir -p clients/desktop/src-tauri/binaries
go build -o clients/desktop/src-tauri/binaries/datasafe-agent ./cmd/datasafe-agent
```

On Windows, add `-ldflags "-H windowsgui"` and the `.exe` suffix. [ADR-0008](../../docs/architecture/adr/0008-desktop-client.md) records the WebView2 loopback connection, the browser arguments that stop component updates, and the Edge Update service those arguments cannot stop.

The bundle id is `com.datasafe.desktop`.

## Prerequisites

- Node.js 20+
- Rust and Cargo
- Windows: WebView2 and the Visual Studio C++ build tools
- Linux: `libwebkit2gtk-4.1-dev`, `libgtk-3-dev`, `librsvg2-dev`, `patchelf`, and a C compiler

From the repository root:

```bash
npm ci
```

## Run on Windows

```powershell
npm run tauri -w datasafe-desktop -- dev
```

The window title is **DataSafeS3 Desktop**. Switch EN/RU and Light/Dark. Close the window and open it again: the choices stay. The page does not ask for a password.

A release bundle:

```powershell
npm run tauri -w datasafe-desktop -- build --bundles nsis
```

The installer is under `clients/desktop/src-tauri/target/release/bundle/nsis/`. A release build does not open a console window. Uninstall removes `HKCU\Software\datasafe\DataSafeS3 Desktop`. App data is removed only if the uninstaller's "delete app data" checkbox is checked. CI uploads the installer as `datasafe-desktop-nsis` and writes the byte size and SHA256 into that artifact (`nsis-size.txt`) and the job summary. The E00 baseline is in [BASELINE.md](BASELINE.md).

## Linux

```bash
npm run tauri -w datasafe-desktop -- build --bundles deb
```

## Checks

```bash
bash scripts/ci/desktop-s5.sh
npm run build -w datasafe-console
npm run build -w datasafe-desktop
```
