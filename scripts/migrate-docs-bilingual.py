#!/usr/bin/env python3
"""Migrate docs to docs/ru/ with path fixes and language headers."""
from __future__ import annotations

import re
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
DOCS = ROOT / "docs"

FILES = [
    ("user-guide/README.md", "user-guide/README.md", "user-guide/README.md"),
    ("user-guide/01-vvedenie-i-vhod.md", "user-guide/01-vvedenie-i-vhod.md", "user-guide/01-introduction-and-login.md"),
    ("user-guide/02-dashbord-i-bakety.md", "user-guide/02-dashbord-i-bakety.md", "user-guide/02-dashboard-and-buckets.md"),
    ("user-guide/03-klyuchi-i-kvoty.md", "user-guide/03-klyuchi-i-kvoty.md", "user-guide/03-keys-and-quotas.md"),
    ("user-guide/04-bezopasnost-i-profil.md", "user-guide/04-bezopasnost-i-profil.md", "user-guide/04-security-and-profile.md"),
    ("user-guide/05-administraciya.md", "user-guide/05-administraciya.md", "user-guide/05-administration.md"),
    ("user-guide/06-gateway-i-minio.md", "user-guide/06-gateway-i-minio.md", "user-guide/06-gateway-and-minio.md"),
    ("user-guide/07-monitoring-i-bazy.md", "user-guide/07-monitoring-i-bazy.md", "user-guide/07-monitoring-and-databases.md"),
    ("user-guide/08-federation-i-cluster.md", "user-guide/08-federation-i-cluster.md", "user-guide/08-federation-and-cluster.md"),
    ("context/roadmap.md", "context/roadmap.md", "context/roadmap.md"),
    ("context/architecture.md", "context/architecture.md", "context/architecture.md"),
    ("context/gateway.md", "context/gateway.md", "context/gateway.md"),
    ("integrations/ldap-keycloak-standalone.md", "integrations/ldap-keycloak-standalone.md", "integrations/ldap-keycloak-standalone.md"),
    ("specs/tenant-groups-tz.md", "specs/tenant-groups-tz.md", "specs/tenant-groups-tz.md"),
    ("specs/tenant-bucket-isolation-tz.md", "specs/tenant-bucket-isolation-tz.md", "specs/tenant-bucket-isolation-tz.md"),
    ("specs/bucket-settings-multi-select-tz.md", "specs/bucket-settings-multi-select-tz.md", "specs/bucket-settings-multi-select-tz.md"),
    ("specs/settings-ui-split-tz.md", "specs/settings-ui-split-tz.md", "specs/settings-ui-split-tz.md"),
]

RU_HEADER = "**[English](../en/{en_rel})** | Русский\n\n"
EN_HEADER = "English | **[Русский](../ru/{ru_rel})**\n\n"


def fix_paths(content: str) -> str:
    content = content.replace("../images/", "../../images/")
    content = re.sub(r"\]\(images/([^)]+)\)", r"](../../user-guide/images/\1)", content)
    content = re.sub(r"!\[([^\]]*)\]\(images/([^)]+)\)", r"![\1](../../user-guide/images/\2)", content)
    content = content.replace("../testing/", "../../testing/")
    content = content.replace("../../README.md", "../../../README.md")
    content = content.replace("../../docker-compose.local-binary.yml", "../../../docker-compose.local-binary.yml")
    content = content.replace("../diagrams/", "../../diagrams/")
    return content


def migrate_ru():
    for src_rel, ru_rel, en_rel in FILES:
        src = DOCS / src_rel
        if not src.exists():
            print(f"SKIP: {src}")
            continue
        dst = DOCS / "ru" / ru_rel
        dst.parent.mkdir(parents=True, exist_ok=True)
        text = fix_paths(src.read_text(encoding="utf-8"))
        text = RU_HEADER.format(en_rel=en_rel) + text
        dst.write_text(text, encoding="utf-8")
        print(f"OK ru/{ru_rel}")


if __name__ == "__main__":
    migrate_ru()
