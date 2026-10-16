# Desktop 1.0 installer baseline

E00 records the Windows installer size and the idle memory of the empty shell. Later releases compare against this table. A growth of more than 20% is a reason to look, not an automatic failure.

The Windows installer is built by the `windows-build` job in `.github/workflows/desktop-ci.yml` (`windows-latest`, MSVC, `--bundles nsis`). That job prints the byte size, writes it to the step summary, and uploads `nsis-size.txt` next to the `.exe` in the `datasafe-desktop-nsis` artifact. Copy that number into the table when cutting a baseline. Idle working set is not measurable in CI.

| | Windows NSIS | Idle working set |
|---|---|---|
| `1.0.0-dev` (E00) | see `desktop-ci` artifact `nsis-size.txt` | _not measured yet_ |

A Linux amd64 `.deb` built from this tree is 3,076,194 bytes (2.93 MiB). That is not the Windows baseline.

How to fill the row:

1. `npm ci` at the repository root.
2. `npm run tauri -w datasafe-desktop -- build --bundles nsis`
3. Write the byte size of the `.exe` under `clients/desktop/src-tauri/target/release/bundle/nsis/`.
4. Launch that build, leave it on the empty shell for a minute, and write the working set from Task Manager.
