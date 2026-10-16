# DataSafeS3 Desktop 1.0

Tauri 2 shell for Windows and Linux. This scaffold is version `1.0.0-dev`: Russian and English, light and dark theme, no sign-in yet. The window does not open a network connection. Folder sync remains the `datasafe-sync` CLI; it is not this app.

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

The installer is under `clients/desktop/src-tauri/target/release/bundle/nsis/`. Record its size and the idle memory of the running app in [BASELINE.md](BASELINE.md).

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
