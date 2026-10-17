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

### E02 browser

The window lists buckets and one page of objects (delimiter `/`, `max_keys` 100) through the agent. It does not walk `next_marker` by itself. Sort and the name filter apply to the loaded page. Write buttons stay hidden unless `access.can_write` is true; a direct call still returns the server 403. Deleting a bucket that answers `401 step_up_required` shows the web-console 2FA message. Object details show size, type, ETag, dates, metadata, tags, retention (mode and date), and an encryption badge. `wrapped_dek` is removed before the result reaches the window. A folder created from the browser is inserted into the current page and selected, because the server sorts the whole listing and the new name can fall off page 1. Switching bucket or profile clears the trash panel. Home, End, Page Up, Page Down, and Backspace move the selection or go to the parent prefix.

### E03 transfers

Transfers run in the agent for the life of the process. The journal is bbolt `transfers.db` in the app data directory. A row stores the task, local path, size, mtime, bucket, key, upload id, confirmed part numbers and ETags, and the rename/skip report. It does not store a token or an access-key secret. Those stay in the agent process memory and are cleared when the task finishes.

Starting parameters, not yet measured on the owner's LAN or Wi-Fi: multipart above 64 MiB, part size 16 MiB, 4 transfers and 4 parts in flight. A later measurement can replace them; this step records the spec's starting values.

An upload continues from the first part that has no stored ETag. The last part that got no response is sent again. A size or mtime change aborts the old upload id and starts over, and the task says why. `404` / `multipart.upload_not_found` and a complete error that mentions an ETag do the same, unless object-meta shows the object is already there at the uploaded size and its ETag is the multipart ETag of the confirmed parts: then the upload is finished and is not started over. Size alone is not enough. Cancel sends `DELETE` for the upload id. ListParts is not called. Server 1.6.1 does not expose it on a path the client can use. An S3 GET that carries `uploadId` returns 405. The REST multipart routes are initiate, upload part, complete, and abort; there is no list-parts route. `objects.list_parts` is absent from capabilities and is reserved for E12. `ListParts` on the filesystem and erasure backends is in-process only. A part can finish on the server inside the journal batch window (32 file updates or 200 ms) and be missing from the journal after a kill. Resume uploads that part again. The same part number replaces `part.NNNNN`, so the object checksum still matches.

CompleteMultipartUpload uses a client with no response-header timeout. The ordinary transfer client still waits two minutes for headers, which is enough for a part and not enough for assembly: server 1.6.1 joins parts at about 16–22 seconds per GiB, so a complete of about 5 GiB or more used to fail and leave the multipart upload open. A timeout or a dropped connection during complete is retried. The upload id and the confirmed parts stay.

`bytes_done` and `size` advance while a transfer is running. Upload progress is counted as each part or chunk is read. Download progress is counted as each chunk is written, and a download learns its size from object-meta or S3 HEAD when the listing did not carry one. The panel shows that as done/total. The download path is the destination folder.

Each file in a directory transfer is its own journal row. The task row keeps the counters and the report, not the file list, so finishing one file does not rewrite the other files. Small objects use the same transfer concurrency as large ones (4 by default). File-row updates are queued and synced in batches of 32, or every 200ms, whichever comes first. A state change (finished, waiting, cancelled) syncs immediately. The list and the progress read overlay the queue, so the counters move before the sync. The transfer list stays a summary. Per-file state and the rename/skip report are loaded on demand with `transfer.files`.

### E04 logs, diagnostics, and TLS

The agent writes `logs/agent.log` and the window writes `logs/desktop.log`. Each rotates at 5 × 10 MiB. `DATASAFE_DESKTOP_LOG_LEVEL`, `DATASAFE_DESKTOP_LOG_MAX_MB`, and `DATASAFE_DESKTOP_LOG_MAX_FILES` change the level and the rotation. Secrets are masked at every level: `Authorization`, `X-DataSafe-MFA-Token`, `X-Amz-Signature`, `X-Amz-Credential`, `password`, `secret`, `token`, and the query string of a presigned URL. The log line is the method name and a masked error. RPC parameters are not logged.

Diagnostics show the log names, sizes, and version text first. Saving writes `datasafe-desktop-diagnostics.zip` containing those logs and `versions.txt` only. `profiles.json` and `transfers.db` are not in the archive. Windows `desktop-ci` runs `secrets-in-logs`.

TLS is verified, with a minimum of TLS 1.2. A profile `ca_file` is added to the system trust pool. `insecure_dev` skips verification and the window says so. HTTP is allowed only to loopback. A self-signed certificate is refused unless the flag is set or the certificate is the configured CA. A certificate failure is reported as “the server certificate could not be verified”. A host that does not answer stays “server is unreachable”.

Each log line is synced as it is written, in UTC ISO 8601 (`2026-10-17T03:34:00Z`) in both `agent.log` and `desktop.log`. A crash keeps the lines already written. `versions.txt` names the product version, the git commit, the OS name and version, and the connected profile’s server version. CI passes the commit with `-X main.DesktopCommit`. A local build leaves that unset, and the text then uses the revision Go recorded from git while compiling (`vcs.revision`, the same value as `git rev-parse`, first 12 characters). Windows 11 still reports version 10.0 from `RtlGetVersion`; build 22000 and later is written as `Windows 11 (10.0.26200)`.

### E05 Windows installer

`desktop-ci` builds Linux (`.deb`) and Windows (NSIS) and runs the agent tests plus the desktop library tests. macOS is not in this release. The NSIS installer is per-user (`currentUser`). If WebView2 is missing it downloads the silent bootstrapper. CI creates a self-signed Authenticode certificate, `CN=DataSafeS3 Desktop Test`, and signs the agent and the installer with it. The signature is only for checking that signing works. It is not a trusted publisher. `Get-AuthenticodeSignature` shows that subject; the status stays untrusted until the certificate is in the user’s trust store. A real publisher certificate waits on Q4-3. A Windows build without `DATASAFE_TEST_PFX` stays unsigned.

The owner’s Windows check, against a 1.6.x server, is still manual: install without an administrator prompt, confirm the test signer, sign in, browse, upload and download, then uninstall and confirm `HKCU\Software\datasafe\DataSafeS3 Desktop` is gone.

Silent uninstall is `uninstall.exe /S`. It does not delete `%APPDATA%\DataSafeS3`, which holds `profiles.json`, `transfers.db`, and `logs`. That is intentional: a reinstall keeps the profile list and the transfer journal. The NSIS template has no extra switch that combines `/S` with deleting that data directory. The graphical “delete app data” checkbox removes `%APPDATA%\com.datasafe.desktop` and `%LOCALAPPDATA%\com.datasafe.desktop`, which are the bundle-id directories, not `%APPDATA%\DataSafeS3`. The post-uninstall hook also removes `%LOCALAPPDATA%\com.datasafe.desktop` on uninstall, including `/S`, and skips that delete during an update. That directory is the WebView2 profile. It holds no user data. `%APPDATA%\DataSafeS3` stays either way. Keyring entries are separate and stay until the profile is forgotten inside the app.

A download writes `<name>.dspart` beside the target and renames it after the size and ETag agree. Resume uses `Range` on an S3 GET for an access-key profile, and a presigned GET for a JWT profile. REST `Range` is used only when the server advertises `objects.range`, which is not part of 1.6.1. Without a presign right and without `objects.range`, the download starts again and the panel says so. A network failure sets the task to waiting, with a pause of 1s that doubles up to 60s, and continues without restarting the app. Rows that were still running when the agent starts again are marked interrupted. The confirmed parts and the download offset stay in the journal.

### E06 resume after restart

The spike against the 1.6.1 server, which matches the 1.5.0 audit already recorded as A2, confirms 4.7.2. No server change is required.

Uploading the same part number again replaces `part.NNNNN` and returns a new MD5 ETag. The upload id is kept until abort or complete; it does not expire. Complete after the client restarts uses the part list in the journal. The server concatenates those part files and does not need ListParts for that. ListParts remains an E12 capability. If the upload id is gone, complete returns not found and the client starts a new upload.

Abort removes the multipart directory and the metadata row. A part that is still uploading can finish after that and leave an orphan file in a directory the metadata no longer names. The next complete then fails with upload not found, and the client starts over. Cancel still aborts every upload id the journal knows.

A presigned GET is an ordinary S3 GET, so `Range` works for a JWT profile on 1.5.0 and 1.6.1. REST GET still has no `Range` until `objects.range` is advertised.

After the app, or the agent, starts again, opening the journal marks queued, running, waiting, and conflict tasks as interrupted. As soon as a profile is signed in, including a remembered session restored at launch, the window calls `transfer.resume` for that profile. It does not wait until a bucket is opened. The transfer panel calls `transfer.resume` again when it mounts; only rows that are still interrupted are queued, so the second call does not resend parts that already have an ETag. That puts the tasks back in the queue and attaches the current token or access-key secret in memory only. An upload continues at the first part that has no ETag. A download continues from the saved offset, or from the size of the `.dspart` file if the offset was not flushed, and keeps the same local path so a partial file is not treated as a new name. A file that already exists and has no partial still asks replace, skip, or rename. A machine that sleeps does not restart the agent. The process stays up, a dead connection is a network wait, and the same upload id or offset continues when the network returns. Kill and restart use the journal path above. The panel says a transfer continues once the profile is signed in. That matches the window: resume runs at sign-in and at session restore. Until then the row stays interrupted and no secret is written to the journal.

Parts that the server stored but the journal had not synced are uploaded again. On one kill this was parts 64–67 (64 MiB), about 120 ms after they finished and still inside the 200 ms batch. Reconciling them would need ListParts for that upload id. 1.6.1 cannot do that, for the reasons in the E03 note above, so the client does not call it and the server code is unchanged. Repeating the part is safe: the same part number overwrites `part.NNNNN`.

Directory upload maps the tree to prefixes. Symlinks, and Windows reparse points, are skipped unless the user asks to follow them, and a followed link is not walked twice. Empty directories are not created. Download renames keys the target OS cannot store: Windows reserved names and illegal characters, Linux NUL, and a segment longer than 255 bytes. The macOS `:` and NFC rules are in the same function so a Linux test can check them; this release does not build macOS. A path that would leave the chosen directory is skipped. On Windows the agent opens files with the `\\?\` prefix. The executable has one application manifest: Tauri's Common Controls v6 manifest, with `longPathAware` added. A second manifest resource is not compiled. The linker rejects two `MANIFEST` resources with id 1 (`CVT1100`).

Dropping files on the window enqueues an upload. Tauri 2.12 can move the window with `startDragging`, and it can receive a drop, but it has no API that drags a storage object out to Explorer. Download into a chosen folder is the way out. The field is labeled as that destination folder.

### E07 step-up

Deleting a bucket and purging one trash item are the dangerous actions the window can perform today. When the server answers `401 step_up_required`, the window asks for a TOTP or a recovery code. The agent posts `{action, target, code}` to `POST /api/v1/auth/step-up`. `code` is the field the server accepts for both a TOTP and a recovery code. WebAuthn step-up is not called (open question Q2-1). The action token comes back to the agent only. It is sent once, on the retry, as `X-DataSafe-MFA-Token`, then discarded. It is not returned to the window, not written to the journal, and not reused for a second action. Trash purge's target is the original bucket from the 401 body, not the trash id. A second dangerous action therefore asks for a new code. A replayed token is `401 mfa_token_invalid`, and the dialog opens again. A wrong code stays on the same dialog. `403 mfa_setup_required` or `mfa_required` show a banner that points at the web console. The banner for a signed-in user who has no factor comes from `mfa_policy.banner` on `GET /api/v1/me`, which is what 1.6.1 sends, and it is shown when that profile is selected. `mfa_setup_required` on `/me` is a different flag and does not raise this banner. The desktop does not enroll a factor. A 1.5.0 server has no `auth.step_up` feature and does not return `step_up_required`, so delete stays the ordinary confirmation and the agent does not call step-up.

### E07 follow-up

A download that runs while multipart parts are being written can sit with no response headers for longer than the two-minute stream timeout. That timeout is Go's `ResponseHeaderTimeout`. Its error implements `Is(context.DeadlineExceeded)` and is wrapped in `url.Error`, so it used to be treated as a deadline instead of a network error and the task was marked failed. A header timeout, an i/o timeout, and a connection reset during the body now set the task to `waiting_network`, wait with the same backoff (1s, doubling up to 60s), and continue from the saved offset or the confirmed parts. Only a user cancel, or cancellation of the task context, is fatal. CompleteMultipartUpload still uses the client that has no response-header timeout.

The same `url.Error` text is `Get "<url>": ...`, and the URL is the presigned GET, query included. That text was stored on the task and returned by `transfer.list`, so `X-Amz-Credential` and `X-Amz-Signature` reached the window and `transfers.db`. Every error is stripped of http(s) userinfo, query, and fragment before it is stored or returned. Opening the journal rewrites rows that already contain them. Bolt may keep the old bytes in a freed page until that page is reused; the live row does not.

If the app is killed while the server is still concatenating a complete, the retried complete can return 400, and the object appears later (about 9s in one measurement). A 400 or 404 from that retry is treated as maybe done. The client polls object-meta for about 30s and accepts the object only when the size matches and the ETag equals the multipart ETag: the MD5 of the raw 16-byte part MD5s in part-number order, hex-encoded, then `-` and the part count. A same-size object with a different ETag stays failed, so an overwrite is not marked done. If a part ETag is not a 16-byte MD5, there is no size-only fallback. A network failure during complete still retries the complete and keeps the upload id. A plain 400 after the poll stays failed; the window has a Retry button, which queues that task again without clearing parts or the offset. A 404 after the poll still starts a new upload. The start screen lists tasks that are not done or cancelled, with byte progress, without opening a bucket.

WebView2 general autofill is off. The existing `--disable-features` list also names `Autofill`, `PasswordManager`, and `PasswordManagerOnboarding`, because the WebView setting does not disable password saving. The code inputs use `autocomplete="off"`.

### Server: GET stalls while parts are written (not changed)

This is a 1.6.1 server defect. No server code was changed.

Observed with an upload and a download at the same time: a GET received no response headers for more than two minutes while multipart parts were being written. The request was not rejected. Headers did not arrive. That is the stream client's `ResponseHeaderTimeout`, so the download failed until the client change above.

Hypothesis: an S3 GET does not write status or headers until `GetObject` returns a body reader (`internal/api/s3/handler.go`). `UploadPart` copies the part onto the same disk and commits metadata through bolt, which has a single writer (`internal/api/s3/service.go`). A metadata read for the GET waits behind those commits, or the process is busy enough on disk that the GET goroutine is not scheduled before the client gives up. This is not a missing route. The client retries. The server is unchanged.

### Server: REST GET ignores Range (not changed)

`handleDownloadObjectJSON` (`internal/api/bucket_handlers.go`) does not read `Range`. It always writes 200 and copies the whole object. S3 GET honors `Range` (`internal/api/s3/handler.go`). `objects.range` is not advertised, so a JWT download resumes through a presigned S3 GET. The server is unchanged.

### Versioning list bug (not changed)

This is a 1.6.1 server defect. It is recorded here so the proposed fix is next to the client that reproduced the symptom. No server code was changed in this step.

Objects uploaded while versioning was off are stored in the bolt `objects` bucket as a flat key (`bucket\x00key`) and are not given a version id. `SetBucketVersioning` only flips `BucketRecord.Versioning`. A later delete with an empty version id, which is also what rename does after `CopyObject`, takes the versioned branch in `Service.deleteObject` and writes a delete marker plus an `object_latest` pointer. `Store.ListObjects` walks `object_latest` first. When that pointer is a delete marker it skips the row without marking the key as seen, then the flat-key walk appends the pre-versioning object. The object stays in the listing. Postgres does not do this: a versioned insert clears `is_latest` on the previous row, and the list query requires `is_latest` and rejects delete markers.

Proposed fix, bolt only: in `ListObjects`, set `seen[objKey]` before continuing when the latest row is a delete marker, so the flat null version stays in history but is not a current object. In `GetObjectVersion`, when the latest pointer is a delete marker, return not found instead of the marker. Enabling versioning does not need to rewrite every object.

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
- The NSIS uninstaller removes `HKCU\Software\datasafe\DataSafeS3 Desktop` (and the same key under `HKLM` if a per-machine install wrote it) and `%LOCALAPPDATA%\com.datasafe.desktop`. `%APPDATA%\DataSafeS3` stays. The stock "delete app data" checkbox still decides `%APPDATA%\com.datasafe.desktop`.
