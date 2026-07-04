#!/usr/bin/env python3
"""List buckets via Admin API (JWT login). Requires: pip install requests"""

import os
import sys

import requests

BASE = os.environ.get("DATASAFE_CONSOLE_URL", "http://127.0.0.1:9000").rstrip("/")
USER = os.environ.get("DATASAFE_ADMIN_USER", "admin")
PASSWORD = os.environ.get("DATASAFE_ADMIN_PASSWORD", "admin")


def main() -> int:
    login = requests.post(
        f"{BASE}/api/v1/admin/login",
        json={"username": USER, "password": PASSWORD},
        timeout=30,
    )
    login.raise_for_status()
    token = login.json()["token"]

    resp = requests.get(
        f"{BASE}/api/v1/buckets",
        headers={"Authorization": f"Bearer {token}"},
        timeout=30,
    )
    resp.raise_for_status()
    buckets = resp.json().get("buckets", [])
    for b in buckets:
        print(b.get("name", b))
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except Exception as exc:  # noqa: BLE001 — copy-paste example
        print(f"error: {exc}", file=sys.stderr)
        raise SystemExit(1) from exc
