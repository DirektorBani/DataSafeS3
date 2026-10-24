#!/usr/bin/env python3
"""Generate THIRD_PARTY_LICENSES from CycloneDX SBOMs.

  scripts/gen-third-party-licenses.py --go sbom-go.json --npm sbom-npm.json \
      --alpine sbom-alpine.json --lang ru|en --out FILE

Component lists come from the SBOMs (scripts/sbom.sh); the notices for optional
external components are maintained below.
"""
import argparse, json, re, sys

EXTERNAL = [
    # name, license, how DataSafeS3 uses it, note (ru, en)
    ("Grafana", "AGPL-3.0-only", "optional dashboards image/profile; not part of DataSafeS3 images",
     "Отдельный образ grafana/grafana, запускается как отдельный сервис. DataSafeS3 не включает и не изменяет код Grafana. Условия AGPL-3.0 относятся к вашей эксплуатации Grafana.",
     "Separate grafana/grafana image run as its own service. DataSafeS3 neither bundles nor modifies Grafana code. AGPL-3.0 terms apply to your operation of Grafana."),
    ("HashiCorp Vault", "BUSL-1.1", "optional KMS/transit (compose profile, Helm values)",
     "Отдельный образ hashicorp/vault. Лицензия BUSL-1.1 не является открытой; проверьте допустимость использования в вашей организации. Совместимая открытая альтернатива с тем же API — OpenBao (MPL-2.0), рекомендуется для установок, где BUSL неприемлема.",
     "Separate hashicorp/vault image. BUSL-1.1 is not an open-source licence; check that your use is permitted. OpenBao (MPL-2.0) is an API-compatible open alternative and is suggested where BUSL is not acceptable."),
    ("Prometheus", "Apache-2.0", "optional metrics scraping (compose, Helm)", "Отдельный образ prom/prometheus.", "Separate prom/prometheus image."),
    ("PostgreSQL", "PostgreSQL License", "optional metadata backend (HA mode)", "Отдельный образ postgres.", "Separate postgres image."),
    ("Apache Kafka", "Apache-2.0", "optional event stream", "Отдельный образ; клиент в бинарнике — Go-модуль из списка выше.", "Separate image; the client inside the binary is a Go module listed above."),
    ("NATS Server", "Apache-2.0", "optional event stream; used in tests only (go.mod)", "В релизный бинарник не входит; используется в тестах.", "Not part of the release binary; used in tests."),
    ("Keycloak", "Apache-2.0", "optional OIDC/LDAP lab integration", "Отдельный образ quay.io/keycloak/keycloak в примерах интеграции.", "Separate image used in integration examples."),
    ("Caddy", "Apache-2.0", "optional TLS reverse proxy (legacy chart, compose)", "Отдельный образ caddy.", "Separate caddy image."),
    ("OpenLDAP (osixia/openldap or similar)", "OpenLDAP Public License", "optional LDAP lab", "Только в лабораторных стендах.", "Lab stands only."),
]

def lic_of(c):
    out = []
    for src in (c.get("licenses") or [], (c.get("evidence") or {}).get("licenses") or []):
        for l in src:
            if "expression" in l:
                out.append(l["expression"])
            else:
                x = l.get("license", {})
                v = x.get("id") or x.get("name")
                if v: out.append(v)
        if out: break
    # de-dup keep order
    seen = []
    for v in out:
        if v not in seen: seen.append(v)
    return " OR ".join(seen) if seen else "NOASSERTION"

def comps(path, drop_files=True):
    d = json.load(open(path, encoding="utf-8"))
    rows = []
    for c in d.get("components", []):
        if c.get("type") not in ("library", "framework", "application"):
            continue
        rows.append((c.get("group","") + ("/" if c.get("group") else "") + c["name"], c.get("version",""), lic_of(c)))
    return sorted(set(rows), key=lambda r: r[0].lower())

def table(rows, head):
    s = "| %s | %s | %s |\n|---|---|---|\n" % head
    for n, v, l in rows:
        s += "| `%s` | %s | %s |\n" % (n.replace("|", "/"), v, l.replace("|", "/"))
    return s

def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--go", required=True); ap.add_argument("--npm", required=True)
    ap.add_argument("--alpine"); ap.add_argument("--lang", default="ru"); ap.add_argument("--out", required=True)
    ap.add_argument("--version", required=True); ap.add_argument("--root", default="")
    a = ap.parse_args()
    ru = a.lang == "ru"
    go, npm = comps(a.go), comps(a.npm)
    alp = [r for r in comps(a.alpine)] if a.alpine else []
    T = (lambda r, e: r if ru else e)
    o = []
    o.append(T("# Лицензии третьих сторон", "# Third-party licenses"))
    o.append("")
    o.append(T(
      f"Версия DataSafeS3: **{a.version}**. DataSafeS3 распространяется по Apache-2.0 (см. [LICENSE]({a.root}LICENSE)). Ниже — компоненты, которые входят в поставку или ставятся рядом по вашему выбору. Списки Go и npm сформированы автоматически из SBOM релиза (`scripts/sbom.sh`, `scripts/gen-third-party-licenses.py`); полные тексты лицензий лежат в исходных репозиториях модулей и в SBOM.",
      f"DataSafeS3 version: **{a.version}**. DataSafeS3 is distributed under Apache-2.0 (see [LICENSE]({a.root}LICENSE)). Below are the components shipped with the product or installed next to it at your choice. The Go and npm lists are generated from the release SBOMs (`scripts/sbom.sh`, `scripts/gen-third-party-licenses.py`); full licence texts are in the upstream repositories and in the SBOM."))
    o.append("")
    o.append(T("## 1. Go-модули в бинарнике storage-server", "## 1. Go modules compiled into storage-server"))
    o.append("")
    o.append(table(go, (T("Модуль","Module"), T("Версия","Version"), T("Лицензия","License"))))
    o.append(T("## 2. npm-зависимости консоли (production)", "## 2. Console npm dependencies (production)"))
    o.append("")
    o.append(table(npm, (T("Пакет","Package"), T("Версия","Version"), T("Лицензия","License"))))
    if alp:
        o.append(T("## 3. Пакеты Alpine Linux в образах", "## 3. Alpine Linux packages in the images"))
        o.append("")
        o.append(T("Образы `datasafe-storage-server` и `datasafe-console` собираются на базе `alpine:3.24` без дополнительных `apk add`; пакет `wget` отдельно не ставится (healthcheck использует busybox). Состав — по SBOM базового образа:",
                   "The `datasafe-storage-server` and `datasafe-console` images are built on `alpine:3.24` with no extra `apk add`; the `wget` package is not installed (the healthcheck uses busybox). Contents per the base image SBOM:"))
        o.append("")
        o.append(table(alp, (T("Пакет","Package"), T("Версия","Version"), T("Лицензия","License"))))
        o.append(T("Пакеты под GPL-2.0 (busybox, apk-tools и др.) — отдельные программы системы; их исходные тексты доступны в Alpine Linux (<https://git.alpinelinux.org/aports>). Бинарники DataSafeS3 с ними не слинкованы.",
                   "GPL-2.0 packages (busybox, apk-tools, etc.) are separate system programs; their sources are available from Alpine Linux (<https://git.alpinelinux.org/aports>). DataSafeS3 binaries are not linked with them."))
        o.append("")
    o.append(T("## 4. Необязательные внешние компоненты (не входят в образы DataSafeS3)", "## 4. Optional external components (not part of DataSafeS3 images)"))
    o.append("")
    o.append(T("Эти программы запускаются отдельными контейнерами из образов их авторов и скачиваются из реестров по вашему выбору. Лицензии и условия — их собственные.",
               "These programs run as separate containers from their authors' images, pulled from registries of your choice. Their own licences and terms apply."))
    o.append("")
    o.append("| %s | %s | %s | %s |\n|---|---|---|---|" % (T("Компонент","Component"), T("Лицензия","License"), T("Назначение","Use"), T("Примечание","Notice")))
    for n, l, use, ru_n, en_n in EXTERNAL:
        o.append("| %s | %s | %s | %s |" % (n, l, use, T(ru_n, en_n)))
    o.append("")
    o.append(T("Список не заменяет юридическую оценку. Если вам нужны полные тексты, берите их из SBOM релиза и репозиториев модулей.",
               "This list is not legal advice. For full texts use the release SBOM and the module repositories."))
    open(a.out, "w", encoding="utf-8", newline="\n").write("\n".join(o) + "\n")

main()
