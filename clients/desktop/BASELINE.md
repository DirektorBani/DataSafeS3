# Desktop 1.0 installer baseline

E00 records the Windows installer size and the idle memory of the empty shell. Later releases compare against this table. A growth of more than 20% is a reason to look, not an automatic failure.

This environment cannot build the Windows installer. The owner measures it on the Windows PC.

| | Windows NSIS | Idle working set |
|---|---|---|
| `1.0.0-dev` (E00) | _not measured yet_ | _not measured yet_ |

A Linux amd64 `.deb` built from this commit is 3,108,156 bytes (2.96 MiB). That is not the Windows baseline.

How to fill the row:

1. `npm ci` at the repository root.
2. `npm run tauri -w datasafe-desktop -- build --bundles nsis`
3. Write the byte size of the `.exe` under `clients/desktop/src-tauri/target/release/bundle/nsis/`.
4. Launch that build, leave it on the empty shell for a minute, and write the working set from Task Manager.
