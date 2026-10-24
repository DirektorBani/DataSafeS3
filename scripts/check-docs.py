#!/usr/bin/env python3
"""Check that every document under docs/ has a Russian counterpart
and that relative links in docs/ and the root README files resolve.

A file is its own Russian counterpart when a path segment is ``ru``
or it lives in the legacy Russian guide ``docs/user-guide/``.
Other files must match one of:

* the same path with the first ``en``, ``de``, or ``fr`` segment
  replaced by ``ru``;
* ``<parent>/ru/<name>``;
* ``docs/ru/<path relative to docs>``;
* an entry in ALIASES (user-guide chapters whose Russian filenames differ).
"""

from __future__ import annotations

import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
DOCS = ROOT / "docs"

# English chapter -> existing Russian chapter (different slug).
ALIASES = {
    "docs/en/user-guide/01-introduction-and-login.md": "docs/ru/user-guide/01-vvedenie-i-vhod.md",
    "docs/en/user-guide/02-dashboard-and-buckets.md": "docs/ru/user-guide/02-dashbord-i-bakety.md",
    "docs/en/user-guide/03-keys-and-quotas.md": "docs/ru/user-guide/03-klyuchi-i-kvoty.md",
    "docs/en/user-guide/04-security-and-profile.md": "docs/ru/user-guide/04-bezopasnost-i-profil.md",
    "docs/en/user-guide/05-administration.md": "docs/ru/user-guide/05-administraciya.md",
    "docs/en/user-guide/06-gateway-and-minio.md": "docs/ru/user-guide/06-gateway-i-minio.md",
    "docs/en/user-guide/07-monitoring-and-databases.md": "docs/ru/user-guide/07-monitoring-i-bazy.md",
    "docs/en/user-guide/08-federation-and-cluster.md": "docs/ru/user-guide/08-federation-i-cluster.md",
}

LINK_RE = re.compile(r"!?\[[^\]]*\]\(([^)]+)\)")
HREF_RE = re.compile(r"""(?:href|src)\s*=\s*["']([^"']+)["']""", re.IGNORECASE)
FENCE_RE = re.compile(r"^```.*?^```", re.MULTILINE | re.DOTALL)

LINK_ROOTS = [DOCS, ROOT / "README.md", ROOT / "README.ru.md"]


def posix(path: Path) -> str:
    return path.as_posix()


def is_russian(path: Path) -> bool:
    rel = path.relative_to(DOCS)
    if "ru" in rel.parts:
        return True
    # Legacy end-user guide written in Russian (including its image index).
    if rel.parts and rel.parts[0] == "user-guide":
        return True
    return False


def counterparts(path: Path) -> list[Path]:
    rel = path.relative_to(DOCS)
    parts = list(rel.parts)
    found: list[Path] = []
    key = posix(path.relative_to(ROOT))
    alias = ALIASES.get(key)
    if alias:
        found.append(ROOT / alias)
    for lang in ("en", "de", "fr"):
        if lang in parts:
            swapped = parts.copy()
            swapped[parts.index(lang)] = "ru"
            found.append(DOCS.joinpath(*swapped))
            break
    found.append(path.parent / "ru" / path.name)
    found.append(DOCS / "ru" / rel)
    return found


def missing_russian() -> list[str]:
    missing: list[str] = []
    for path in sorted(DOCS.rglob("*.md")):
        if is_russian(path):
            continue
        if any(candidate.is_file() for candidate in counterparts(path)):
            continue
        missing.append(posix(path.relative_to(ROOT)))
    return missing


def link_files() -> list[Path]:
    files: list[Path] = []
    for entry in LINK_ROOTS:
        if entry.is_file():
            files.append(entry)
        elif entry.is_dir():
            files.extend(sorted(entry.rglob("*.md")))
    return files


INLINE_CODE_RE = re.compile(r"`[^`\n]*`")


def strip_fences(text: str) -> str:
    text = FENCE_RE.sub("", text)
    return INLINE_CODE_RE.sub("", text)


def link_target(raw: str) -> str | None:
    target = raw.strip()
    if not target or target.startswith("#"):
        return None
    if target.startswith("<") and target.endswith(">"):
        target = target[1:-1].strip()
    # Optional title: (url "title") or (url 'title').
    if " " in target:
        target = target.split()[0]
    lower = target.lower()
    if lower.startswith(("http://", "https://", "mailto:", "data:")):
        return None
    if target.startswith("//"):
        return None
    return target.split("#", 1)[0].split("?", 1)[0]


def resolve_link(source: Path, target: str) -> Path | None:
    if target.startswith("/"):
        # Site-root links are not repository paths.
        return None
    path = (source.parent / target).resolve()
    return path


def broken_links() -> list[str]:
    problems: list[str] = []
    for path in link_files():
        text = path.read_bytes().decode("utf-8", errors="replace")
        body = strip_fences(text)
        raw_links = LINK_RE.findall(body) + HREF_RE.findall(body)
        for raw in raw_links:
            target = link_target(raw)
            if target is None or target == "":
                continue
            resolved = resolve_link(path, target)
            if resolved is None:
                continue
            if resolved.is_file() or resolved.is_dir():
                continue
            rel = posix(path.relative_to(ROOT))
            problems.append(f"{rel}: {raw.strip()}")
    return problems


def main() -> int:
    missing = missing_russian()
    broken = broken_links()
    if not missing and not broken:
        print("docs check: russian counterparts and relative links ok")
        return 0
    if missing:
        print(f"docs check: {len(missing)} file(s) without a Russian counterpart:", file=sys.stderr)
        for item in missing:
            print(f"  {item}", file=sys.stderr)
    if broken:
        print(f"docs check: {len(broken)} broken relative link(s):", file=sys.stderr)
        for item in broken:
            print(f"  {item}", file=sys.stderr)
    return 1


if __name__ == "__main__":
    sys.exit(main())
