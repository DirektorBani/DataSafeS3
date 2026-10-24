#!/usr/bin/env python3
"""Fail when a Compose file bind-mounts a repository path that does not exist.

Compose resolves relative paths against the project directory, which is the
directory of the FIRST -f file. Every overlay in deploy/compose is documented
as `-f docker-compose.yml -f deploy/compose/<overlay>`, so its relative paths
must start from the repository root (`./deploy/...`). The standalone files
listed below are run alone with `-f deploy/compose/<file>` and resolve from
their own directory.
"""
import pathlib
import re
import sys

ROOT = pathlib.Path(__file__).resolve().parent.parent
STANDALONE = {  # run as the only/first -f file: paths relative to deploy/compose
    "docker-compose.kms-cluster.yml",
}
# Produced by `go build` before the overlay is used (see install.ps1 / docs), not in git.
BUILD_ARTIFACTS = {"deploy/docker/storage-server-linux"}
SRC = re.compile(r"^\s*-\s*[\"']?(\.{1,2}/[^:\"']+):")
bad = []
files = [ROOT / "docker-compose.yml", ROOT / "docker-compose.local-data.yml"]
files += sorted((ROOT / "deploy/compose").glob("docker-compose*.yml"))
for f in files:
    if not f.exists():
        continue
    base = f.parent if f.name in STANDALONE else ROOT
    for n, line in enumerate(f.read_text(encoding="utf-8", errors="replace").splitlines(), 1):
        m = SRC.match(line)
        if not m:
            continue
        src = m.group(1)
        if re.search(r"\$\{", src):
            continue
        target = (base / src).resolve()
        if str(target).startswith(str(ROOT)) and target.relative_to(ROOT).as_posix() in BUILD_ARTIFACTS:
            continue
        if not target.exists():
            bad.append(f"{f.relative_to(ROOT)}:{n}: {src} does not exist relative to {base.relative_to(ROOT) if base != ROOT else '.'}")
if bad:
    print("\n".join(bad))
    sys.exit(1)
print("compose bind-mount paths: OK")
