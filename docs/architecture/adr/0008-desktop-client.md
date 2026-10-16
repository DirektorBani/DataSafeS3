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

`tauri.conf.json` sets a CSP of `default-src 'self'` with `connect-src` limited to Tauri IPC (`ipc:` and `http://ipc.localhost`). Capabilities list each permission by name. E01 grants one permission per command (`allow-profile-list`, `allow-auth-login`, and the other profile and auth commands). A later step adds the next command the same way. The core permission set and the shell plugin are not granted. `scripts/ci/desktop-s5.sh` fails the build if the CSP regresses, if a permission is a default set, if the WebView arguments drop the updater lock, or if the shell plugin is granted.

### Windows WebView and loopback port 80

On Windows, Tauri 2.12 (wry 0.57) does not navigate the WebView2 control to `tauri://localhost`. WebView2's custom-scheme support is incomplete, so wry rewrites the asset URL to `http://tauri.localhost` and the IPC URL to `http://ipc.localhost`, then answers those requests inside the `WebResourceRequested` handler. `app.windows[].useHttpsScheme` only changes that rewrite to `https://…localhost`. It does not switch the control to a scheme that never touches the network stack, and it would move the same connection from port 80 to port 443. This shell leaves the default http rewrite in place.

Names that end in `.localhost` are loopback names. WebView2's network process sometimes opens a TCP connection to the default HTTP port before, or beside, the in-process handler. Windows `http.sys` owns port 80, so that connection shows up as the WebView2 process holding `[::1]:80`. The peer is the local HTTP driver. The shell does not send the DataSafe API, credentials, or any other application byte on that socket. Linux uses `tauri://localhost` and does not do this.

A separate WebView2 process was observed calling `edge.microsoft.com:443` while the window was idle. That is the runtime's component updater and SmartScreen, not the page. Setting `additionalBrowserArgs` replaces wry's defaults, so the main window repeats `msWebOOUI`, `msPdfOOUI`, and `msSmartScreenProtection` and adds `--disable-background-networking`, `--disable-component-update`, `--disable-domain-reliability`, `--disable-sync`, `--disable-client-side-phishing-detection`, `--no-pings`, and `ComponentUpdater`. Those flags apply to the app's WebView2 browser process.

What remains: the Windows **Microsoft Edge Update** service can still contact `edge.microsoft.com` for the Evergreen runtime. It is not started by the page, it is not the DataSafe API, and browser arguments cannot turn that service off. The check on a Windows PC is the `msedgewebview2.exe` command line for this app: it should contain `--disable-component-update`.

### E01 agent

`cmd/datasafe-agent` speaks newline-delimited JSON-RPC on stdin and stdout. Rust starts it with only `--data-dir`. Passwords, tokens, and codes travel in that pipe and are not process arguments or environment variables. Rust stores them in the OS credential store under `datasafe-desktop/<profile_id>/<kind>` (`password`, `token`, `api_token`, `access_secret`). The `keyring` crate is built with `windows-native`, `sync-secret-service`, and `apple-native`. Without those features it falls back to an in-memory mock that reports a successful write and then loses the secret, including inside the same process, so Credential Manager stays empty. Startup and the shell test refuse that mock. Forget deletes every kind for the profile before it removes the profile record. The WebView receives status only. `profiles.json` has no secrets. Without a running Secret Service, Rust keeps the secret in memory for the process and says so (`secrets_persisted` is false).

The agent classifies 1.5.0 (stamped and `dev` without `kms`), 1.6.0, and 1.6.1. A public capabilities body without `features` falls through to `/healthz`. An authenticated body with `features` replaces that column. A semver below 1.5.0 is refused. Tag `v1.4.1` writes `/healthz` with no `version` field (`internal/api/ha_handlers.go`); `GET /api/v1/auth/login-options` already exists there and returns 200, so a 404 on that route is not a version signal. Tag `v1.5.0` always writes `version` and `commit`, and `TestHealthReportsVersionAndCommit` requires both to be non-empty. A reachable `/healthz` with the field missing or blank is refused before any credential is sent. Basic mode is only a version that is present but not a semver and not `dev`, or `/healthz` being down while `/api/v1/me` answers. An unauthenticated `GET /api/v1/events/delivery` is 404 on `v1.4.1` (the route is absent) and 401 on `v1.5.0` (`adminOnly` calls `requireAuth`). That matches the tags and is not used for the decision: a missing version is already older than 1.5.0, and a present but unparseable version stays basic. `1.0.0-dev` satisfies `min_supported` `1.0.0`; a higher floor still warns, and `min_required` with reason `security` blocks the profile while the list cache stays. OIDC in this step is the resource-owner password grant and the one-time exchange code. The system-browser redirect waits on the deep-link decision.

### Secrets

Secrets (password, JWT, `ds_` token, access-key secret, local-server secrets, share password, TOTP, step-up token) live only in the OS store under `datasafe-desktop/<profile_id>/<kind>`: Windows Credential Manager or Linux Secret Service. There is no secret file. Secrets are not process arguments or environment variables. Without Secret Service, Linux asks for the password on each launch and does not save it. E00 removes the old sync shell, which passed the password to `datasafe-sync` as `--password`. The OS store itself is implemented in E01.

### Name

The product name is **DataSafeS3 Desktop**. The bundle id is `com.datasafe.desktop`. The scaffold version is `1.0.0-dev`.

### Platforms

Windows and Linux only. macOS is not built and has no CI in this release.

## Consequences

- `clients/desktop` on this branch is the Desktop 1.0 shell (React, RU/EN, light and dark theme). The Phase 3 sync window remains on `release/1.6.1`.
- `web/console` imports buttons, dialogs, tables, the step-up dialog, and the encryption cards from `@datasafe/ui`. Console behavior is unchanged.
- The shell signs in through the agent. Object browsing is a later step. On Windows the WebView2 process can still hold the loopback connection described above.
- A Windows release build is a GUI subsystem binary, so the installer executable does not open a console. A debug build keeps the console.
- The NSIS uninstaller removes `HKCU\Software\datasafe\DataSafeS3 Desktop` (and the same key under `HKLM` if a per-machine install wrote it). The stock "delete app data" checkbox still decides whether `%APPDATA%\com.datasafe.desktop` and `%LOCALAPPDATA%\com.datasafe.desktop` are removed.
