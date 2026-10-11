#!/usr/bin/env python3
"""Check or bump the DataSafeS3 release pins against VERSION.

Release-time bump (writes VERSION and the pin files, then re-checks):

    scripts/bump-release-version.sh 1.4.2

CI (pins must match VERSION):

    scripts/check-release-pins.sh

Tag workflow (VERSION must also equal the pushed tag, v-prefix optional):

    scripts/check-release-pins.sh v1.4.1

Historical changelog and upgrade notes are not pins. Older installer and lab
image fallbacks stay. Third-party Helm images (MinIO, Prometheus, Grafana)
must not be rewritten to the DataSafeS3 release tag.
"""

from __future__ import annotations

import json
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
VERSION_FILE = ROOT / "VERSION"
SEMVER = re.compile(r"^[0-9]+\.[0-9]+\.[0-9]+$")

# (path, needle with {tag}=vX.Y.Z and {ver}=X.Y.Z, minimum occurrences)
PINS: list[tuple[str, str, int]] = [
    ("install.ps1", '[string]$Tag = "{tag}"', 1),
    ("install.ps1", r".\install.ps1 -Tag {tag}", 1),
    ("install.ps1", "ghcr.io/direktorbani/datasafe-storage-server:{tag}", 2),
    ("install.sh", 'TAG="{tag}"', 1),
    ("install.sh", "--tag {tag}", 1),
    ("install.sh", "ghcr.io/direktorbani/datasafe-storage-server:{tag}", 1),
    ("docker-compose.yml", "ghcr.io/direktorbani/datasafe-storage-server:{tag}", 1),
    ("deploy/compose/docker-compose.ha.yml", "ghcr.io/direktorbani/datasafe-storage-server:{tag}", 2),
    ("deploy/compose/docker-compose.ha-local.yml", "ghcr.io/direktorbani/datasafe-storage-server:{tag}", 2),
    ("README.md", "**Release / Релиз:** [{tag}]", 1),
    ("README.md", "ghcr.io/direktorbani/datasafe-storage-server:{tag}", 1),
    ("README.md", "ghcr.io/direktorbani/datasafe-console:{tag}", 1),
    ("docs/en/README.md", "**Current release:** [{tag}]", 1),
    ("docs/ru/README.md", "**Текущий релиз:** [{tag}]", 1),
    ("docs/README.md", "**Current Community release:** [{tag}]", 1),
    ("docs/getting-started/en/installer.md", "default `{tag}`", 1),
    ("docs/getting-started/ru/installer.md", "по умолчанию `{tag}`", 1),
    ("docs/en/context/project-status.md", "**Current release:** [{tag}]", 1),
    ("docs/ru/context/project-status.md", "**Текущий релиз:** [{tag}]", 1),
    (".env.example", "# DATASAFE_SERVER_IMAGE=ghcr.io/direktorbani/datasafe-storage-server:{tag}", 1),
    (".env.example", "# DATASAFE_CONSOLE_IMAGE=ghcr.io/direktorbani/datasafe-console:{tag}", 1),
    ("deploy/helm/datasafe/Chart.yaml", 'appVersion: "{ver}"', 1),
    ("deploy/helm/datasafe/values.yaml", "storageServer.image.tag: {tag}", 1),
    ("deploy/helm/datasafe/values.yaml", "console.image.tag: {tag}", 1),
    ("deploy/helm/datasafe/README.md", "ghcr.io/direktorbani/datasafe-storage-server:{tag}", 1),
    ("deploy/helm/datasafe/README.md", "ghcr.io/direktorbani/datasafe-console:{tag}", 1),
    ("deploy/helm/datasafe/README.md", "--set storageServer.image.tag={tag}", 1),
    ("deploy/helm/datasafe/README.md", "--set console.image.tag={tag}", 1),
    ("web/console/package.json", '"version": "{ver}"', 1),
    ("scripts/cluster/remote/deploy-storage-server.sh", "ghcr.io/direktorbani/datasafe-storage-server:{tag}", 2),
    ("scripts/cluster/lab/up-local.sh", "ghcr.io/direktorbani/datasafe-storage-server:{tag}", 1),
    ("deploy/cluster/lab/docker-compose.yml", "ghcr.io/direktorbani/datasafe-storage-server:{tag}", 1),
    ("deploy/cluster/lab/docker-compose.local.yml", "ghcr.io/direktorbani/datasafe-storage-server:{tag}", 1),
]

# repository line (exact, not a comment) and the tag key that must follow within 3 lines
HELM_IMAGES = (
    "repository: ghcr.io/direktorbani/datasafe-storage-server",
    "repository: ghcr.io/direktorbani/datasafe-console",
)
THIRD_PARTY_IMAGES = (
    "repository: minio/minio",
    "repository: prom/prometheus",
    "repository: grafana/grafana",
)
HELM_VALUES = "deploy/helm/datasafe/values.yaml"
PACKAGE_LOCK = "web/console/package-lock.json"


def fail(msg: str) -> None:
    print(f"release pins: {msg}", file=sys.stderr)
    sys.exit(1)


def read_text(path: Path) -> str:
    # Helm README still contains a legacy Windows-1252 dash; keep those bytes intact.
    return path.read_bytes().decode("utf-8", errors="surrogateescape")


def write_text(path: Path, text: str) -> None:
    path.write_bytes(text.encode("utf-8", errors="surrogateescape"))


def read_version() -> str:
    if not VERSION_FILE.is_file():
        fail("VERSION is missing")
    ver = VERSION_FILE.read_text(encoding="utf-8").strip()
    if not SEMVER.match(ver):
        fail(f"VERSION must be X.Y.Z, got {ver!r}")
    return ver


def normalize_tag(arg: str) -> str:
    ver = arg[1:] if arg.startswith("v") else arg
    if not SEMVER.match(ver):
        fail(f"tag must be vX.Y.Z or X.Y.Z, got {arg!r}")
    return ver


def fmt(needle: str, ver: str) -> str:
    return needle.format(tag=f"v{ver}", ver=ver)


def pin_files() -> list[str]:
    seen: list[str] = []
    for path, _, _ in PINS:
        if path not in seen:
            seen.append(path)
    if PACKAGE_LOCK not in seen:
        seen.append(PACKAGE_LOCK)
    return seen


def following_tag(text: str, repository: str) -> str | None:
    lines = text.splitlines()
    for i, line in enumerate(lines):
        if line.strip() != repository:
            continue
        window = lines[i + 1 : i + 4]
        for nxt in window:
            stripped = nxt.strip()
            if stripped.startswith("tag:"):
                return stripped.split(":", 1)[1].strip()
        return ""
    return None


def check_helm(ver: str, text: str) -> list[str]:
    errors: list[str] = []
    want = f"v{ver}"
    for repo in HELM_IMAGES:
        got = following_tag(text, repo)
        if got is None:
            errors.append(f"{HELM_VALUES}: missing {repo}")
        elif got != want:
            errors.append(f"{HELM_VALUES}: {repo} tag is {got!r}, want {want}")
    for repo in THIRD_PARTY_IMAGES:
        got = following_tag(text, repo)
        if got is None:
            errors.append(f"{HELM_VALUES}: missing third-party image {repo}")
        elif got == want:
            errors.append(f"{HELM_VALUES}: {repo} must not use the DataSafeS3 release tag {want}")
    return errors


def check_package_lock(ver: str) -> list[str]:
    path = ROOT / PACKAGE_LOCK
    try:
        data = json.loads(path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as exc:
        return [f"{PACKAGE_LOCK}: {exc}"]
    errors: list[str] = []
    if data.get("version") != ver:
        errors.append(f"{PACKAGE_LOCK}: root version is {data.get('version')!r}, want {ver}")
    pkg = (data.get("packages") or {}).get("") or {}
    if pkg.get("version") != ver:
        errors.append(f"{PACKAGE_LOCK}: packages[''].version is {pkg.get('version')!r}, want {ver}")
    return errors


def check(tag_arg: str | None) -> None:
    ver = read_version()
    if tag_arg is not None:
        tagged = normalize_tag(tag_arg)
        if tagged != ver:
            fail(f"pushed tag {tagged} does not match VERSION {ver}")
    errors: list[str] = []
    for path, needle, minimum in PINS:
        text = read_text(ROOT / path)
        want = fmt(needle, ver)
        count = text.count(want)
        if count < minimum:
            errors.append(f"{path}: {want!r} found {count} time(s), want at least {minimum}")
    errors.extend(check_helm(ver, read_text(ROOT / HELM_VALUES)))
    errors.extend(check_package_lock(ver))
    if errors:
        for err in errors:
            print(f"release pins: {err}", file=sys.stderr)
        sys.exit(1)
    print(f"release pins match {ver}")


def replace_file(path: Path, old: str, new: str) -> None:
    text = read_text(path)
    updated = text.replace(f"v{old}", f"v{new}").replace(f'"{old}"', f'"{new}"')
    if updated == text:
        fail(f"{path.relative_to(ROOT)}: no {old} pin to bump")
    write_text(path, updated)


def bump_package_lock(old: str, new: str) -> None:
    path = ROOT / PACKAGE_LOCK
    text = read_text(path)
    needle = f'"version": "{old}"'
    repl = f'"version": "{new}"'
    out = text
    for n in range(2):
        idx = out.find(needle)
        if idx < 0:
            fail(f"{PACKAGE_LOCK}: occurrence {n + 1} of {needle} not found")
        out = out[:idx] + repl + out[idx + len(needle) :]
    write_text(path, out)


def bump(raw: str) -> None:
    new = normalize_tag(raw)
    old = read_version()
    if new == old:
        fail(f"VERSION is already {old}")
    VERSION_FILE.write_text(new + "\n", encoding="utf-8")
    for rel in pin_files():
        if rel == PACKAGE_LOCK:
            bump_package_lock(old, new)
            continue
        replace_file(ROOT / rel, old, new)
    check(None)
    print(f"bumped {old} -> {new}")


def main(argv: list[str]) -> None:
    if len(argv) < 2 or argv[1] not in {"check", "bump"}:
        fail("usage: release_pins.py check [vX.Y.Z] | bump X.Y.Z")
    if argv[1] == "check":
        if len(argv) > 3:
            fail("usage: release_pins.py check [vX.Y.Z]")
        check(argv[2] if len(argv) == 3 else None)
        return
    if len(argv) != 3:
        fail("usage: release_pins.py bump X.Y.Z")
    bump(argv[2])


if __name__ == "__main__":
    main(sys.argv)
