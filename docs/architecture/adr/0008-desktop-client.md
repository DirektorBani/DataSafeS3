# ADR 0008: Desktop client

**Status:** Accepted  
**Date:** 2026-10-16  
**Release:** Desktop 1.0 (branch `desktop/1.0`)

## Context

Desktop 1.0 is a Windows and Linux application for profiles, a bucket browser, transfers, and share links. It is not the existing folder-sync shell. Folder sync stays a separate client (`cmd/datasafe-sync`) and is out of this release.

The server contract this client calls is the 1.6.1 `/api/v1` spec on `release/1.6.1`. This ADR does not change that server.

## Decision

### Stack

| Layer | Choice |
|---|---|
| Shell | Tauri 2 (Rust), system WebView |
| UI | React and Vite, shared components in `web/ui` |
| S3 and transfers | Go sidecar `datasafe-agent` (step E01), started by Rust |
| Local server | `storage-server` sidecar, later slice, loopback only |

Electron was not chosen. It ships a Node runtime in the window and a larger installer, and its default IPC does not give a per-command permission list. Wails was not chosen. The repository already has a Tauri shell, Tauri has a signed updater and a capability list for each command, and the Go client code can stay in a sidecar instead of moving into the UI process.

The window does not talk to the network. Rust starts `datasafe-agent` and speaks newline-delimited JSON-RPC on the child's stdin and stdout. The first methods are `profile.*`, `auth.login`, `auth.mfa`, `auth.logout`, and `server.info`. The WebView calls named Tauri commands. It does not receive passwords, JWTs, access-key secrets, or step-up tokens.

### IPC and the WebView

`tauri.conf.json` sets a CSP of `default-src 'self'` with `connect-src` limited to Tauri IPC (`ipc:` and `http://ipc.localhost`). Capabilities list each permission by name. The scaffold grants none: the window does not call a Tauri command. A later step adds the permission for each new command in `capabilities/default.json`. The core permission set and the shell plugin are not granted. `scripts/ci/desktop-s5.sh` fails the build if the CSP regresses, if a permission is a default set, or if the shell plugin is granted.

The current scaffold has no commands and no sidecar. E01 adds the agent and the named commands.

### Windows WebView and loopback port 80

On Windows, Tauri 2.12 (wry 0.57) does not navigate the WebView2 control to `tauri://localhost`. WebView2's custom-scheme support is incomplete, so wry rewrites the asset URL to `http://tauri.localhost` and the IPC URL to `http://ipc.localhost`, then answers those requests inside the `WebResourceRequested` handler. `app.windows[].useHttpsScheme` only changes that rewrite to `https://…localhost`. It does not switch the control to a scheme that never touches the network stack, and it would move the same connection from port 80 to port 443. This shell leaves the default http rewrite in place.

Names that end in `.localhost` are loopback names. WebView2's network process sometimes opens a TCP connection to the default HTTP port before, or beside, the in-process handler. Windows `http.sys` owns port 80, so that connection shows up as the WebView2 process holding `[::1]:80`. The peer is the local HTTP driver. The shell does not send the DataSafe API, credentials, or any other application byte on that socket. Linux uses `tauri://localhost` and does not do this.

### Secrets

Secrets (password, JWT, `ds_` token, access-key secret, local-server secrets, share password, TOTP, step-up token) live only in the OS store under `datasafe-desktop/<profile_id>/<kind>`: Windows Credential Manager or Linux Secret Service. There is no secret file. Secrets are not process arguments or environment variables. Without Secret Service, Linux asks for the password on each launch and does not save it. E00 removes the old sync shell, which passed the password to `datasafe-sync` as `--password`. The OS store itself is implemented in E01.

### Name

The product name is **DataSafeS3 Desktop**. The bundle id is `com.datasafe.desktop`. The scaffold version is `1.0.0-dev`.

### Platforms

Windows and Linux only. macOS is not built and has no CI in this release.

## Consequences

- `clients/desktop` on this branch is the Desktop 1.0 shell (React, RU/EN, light and dark theme). The Phase 3 sync window remains on `release/1.6.1`.
- `web/console` imports buttons, dialogs, tables, the step-up dialog, and the encryption cards from `@datasafe/ui`. Console behavior is unchanged.
- The agent, profiles, and transfers are later steps. This scaffold does not sign in and does not call the DataSafe API. On Windows the WebView2 process can still hold the loopback connection described above.
- A Windows release build is a GUI subsystem binary, so the installer executable does not open a console. A debug build keeps the console.
- The NSIS uninstaller removes `HKCU\Software\datasafe\DataSafeS3 Desktop` (and the same key under `HKLM` if a per-machine install wrote it). The stock "delete app data" checkbox still decides whether `%APPDATA%\com.datasafe.desktop` and `%LOCALAPPDATA%\com.datasafe.desktop` are removed.
