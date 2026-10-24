#!/usr/bin/env python3
"""Check that every container image referenced by Compose files, Helm values
and Dockerfiles has a tag that exists in its registry (uses `crane digest`).

A wrong tag fails only when someone installs, so this runs before a release:
the Helm chart once shipped prom/prometheus:v1.2.0 (a 2016 release) and
grafana/grafana:v1.2.0 (never existed).

  python3 scripts/check-image-refs.py               # all refs
  python3 scripts/check-image-refs.py --skip-own    # skip gitverse.ru/direktorbani/* (not pushed yet)
  python3 scripts/check-image-refs.py --list        # print refs only, no network

Needs `crane` in PATH unless --list is given.
"""
import pathlib
import re
import shutil
import subprocess
import sys

ROOT = pathlib.Path(__file__).resolve().parent.parent
SKIP_DIRS = {".git", "node_modules", "dist", "docs", "_local"}
OWN = "gitverse.ru/direktorbani/"
# Images built locally by a script; they are not in a registry.
LOCAL = {"datasafe-cluster-node:lab", "datasafe-promote-drill", "datasafe-storage-server:ci"}

COMPOSE_IMAGE = re.compile(r"^\s*image:\s*[\"']?(?:\$\{[A-Z_]+:-)?([A-Za-z0-9._/@:-]+)\}?[\"']?\s*$")
FROM = re.compile(r"^\s*FROM\s+(?:--platform=\S+\s+)?(\S+)")
ARG = re.compile(r"^\s*ARG\s+(\w+)=(.*)$")


def files():
    for p in ROOT.rglob("*"):
        if not p.is_file() or any(part in SKIP_DIRS for part in p.relative_to(ROOT).parts):
            continue
        n = p.name
        if n.endswith((".yml", ".yaml")) or n.startswith("Dockerfile"):
            yield p


def refs():
    out = {}
    for p in files():
        text = p.read_text(encoding="utf-8", errors="replace").splitlines()
        rel = p.relative_to(ROOT).as_posix()
        if p.name.startswith("Dockerfile"):
            args = {}
            for line in text:
                m = ARG.match(line)
                if m:
                    args[m.group(1)] = m.group(2).strip()
                m = FROM.match(line)
                if m:
                    ref = re.sub(r"\$\{(\w+)\}", lambda x: args.get(x.group(1), ""), m.group(1))
                    if ref and ref != "build" and not ref.startswith("$"):
                        out.setdefault(ref, set()).add(rel)
            continue
        if "helm" in rel and p.name.startswith("values"):
            repo = None
            for line in text:
                m = re.match(r"^\s*repository:\s*[\"']?([^\s\"']+)", line)
                if m:
                    repo = m.group(1)
                    continue
                m = re.match(r"^\s*tag:\s*[\"']?([^\s\"']+)", line)
                if m and repo:
                    out.setdefault(f"{repo}:{m.group(1)}", set()).add(rel)
                    repo = None
            continue
        for line in text:
            m = COMPOSE_IMAGE.match(line)
            if m and (":" in m.group(1) or "@" in m.group(1)):
                out.setdefault(m.group(1), set()).add(rel)
    return out


def main():
    skip_own = "--skip-own" in sys.argv
    found = refs()
    if "--list" in sys.argv:
        for r in sorted(found):
            print(r, "  <-", ", ".join(sorted(found[r])))
        return 0
    if not shutil.which("crane"):
        print("crane is required (go install github.com/google/go-containerregistry/cmd/crane@v0.20.6)", file=sys.stderr)
        return 2
    bad = []
    for ref in sorted(found):
        if ref in LOCAL or ref.startswith("${") or (skip_own and ref.startswith(OWN)):
            continue
        r = subprocess.run(["crane", "digest", ref], capture_output=True, text=True, timeout=120)
        status = "ok" if r.returncode == 0 else "MISSING"
        print(f"{status:8} {ref}")
        if r.returncode != 0:
            bad.append((ref, r.stderr.strip().splitlines()[-1:] or [""]))
    if bad:
        print("\nimages that do not resolve:", file=sys.stderr)
        for ref, err in bad:
            print(f"  {ref}  ({', '.join(sorted(found[ref]))})  {err[0]}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
