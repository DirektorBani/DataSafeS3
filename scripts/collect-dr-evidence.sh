#!/usr/bin/env bash
# Collect a DR evidence folder and zip after a scripted promote.
# Modelled on scripts/collect-evidence-pack.ps1: a directory, summary.md (EN),
# evidence.json, and a zip. This is an operator report, not a certification.
#
#   --lab     record healthz from --base-url and exit 0 when status is ok.
#             Used by the SSH lab gate. Missing promote timing is recorded,
#             not treated as a skip.
#   (default) post-promote checks. Exit 0 when they pass, 5 when they do not.
#
# Exit codes: 0 pass, 1 usage, 5 a required check failed.
set -euo pipefail

OUT=""
NEW_URL=""
OLD_URL=""
BASE_URL=""
PG_NEW=""
PG_OLD=""
TIMING=""
RERUN=""
KEYS=""
WEBHOOK=""
BUCKET="dr-drill"
VERSION_FILE=""
LAB=0
PGUSER="${PGUSER:-datasafe}"
PGDATABASE="${PGDATABASE:-datasafe}"

usage() {
  cat <<'EOF'
usage: collect-dr-evidence.sh --out DIR [options]

  --lab --base-url URL     healthz pack for the SSH lab gate (exit 0 when ok)
  --new-url URL            promoted storage-server (S3 and /healthz)
  --old-url URL            fenced storage-server
  --pg-new NAME            new Postgres container
  --pg-old NAME            old Postgres container
  --timing-file PATH       JSON from the promote that changed state
  --rerun-file PATH        JSON {ran, exit_code, skipped} for the idempotent re-run
  --keys-file PATH         one object key per line that returned HTTP 200
  --webhook-log PATH       JSON lines posted by the sink
  --bucket NAME            bucket for the post-promote write/read (default dr-drill)
  --version-file PATH      repo VERSION (default ./VERSION when present)
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --out) OUT="${2:-}"; shift 2 ;;
    --new-url) NEW_URL="${2:-}"; shift 2 ;;
    --old-url) OLD_URL="${2:-}"; shift 2 ;;
    --base-url) BASE_URL="${2:-}"; shift 2 ;;
    --pg-new) PG_NEW="${2:-}"; shift 2 ;;
    --pg-old) PG_OLD="${2:-}"; shift 2 ;;
    --timing-file) TIMING="${2:-}"; shift 2 ;;
    --rerun-file) RERUN="${2:-}"; shift 2 ;;
    --keys-file) KEYS="${2:-}"; shift 2 ;;
    --webhook-log) WEBHOOK="${2:-}"; shift 2 ;;
    --bucket) BUCKET="${2:-}"; shift 2 ;;
    --version-file) VERSION_FILE="${2:-}"; shift 2 ;;
    --lab) LAB=1; shift ;;
    -h|--help) usage; exit 0 ;;
    *) echo "collect-dr-evidence: unknown argument: $1" >&2; usage >&2; exit 1 ;;
  esac
done

if [[ -z "$OUT" ]]; then
  echo "collect-dr-evidence: --out is required" >&2
  exit 1
fi
if [[ "$LAB" -eq 1 && -z "$BASE_URL" ]]; then
  echo "collect-dr-evidence: --lab requires --base-url" >&2
  exit 1
fi
if [[ "$LAB" -eq 0 && -z "$NEW_URL" ]]; then
  echo "collect-dr-evidence: --new-url is required" >&2
  exit 1
fi

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
if [[ -z "$VERSION_FILE" && -f "$ROOT/VERSION" ]]; then
  VERSION_FILE="$ROOT/VERSION"
fi
mkdir -p "$OUT"
S3="$ROOT/scripts/ha/drill_s3.py"

export DR_OUT="$OUT" DR_NEW="$NEW_URL" DR_OLD="$OLD_URL" DR_BASE="$BASE_URL" \
  DR_PG_NEW="$PG_NEW" DR_PG_OLD="$PG_OLD" DR_TIMING="$TIMING" DR_RERUN="$RERUN" DR_KEYS="$KEYS" \
  DR_WEBHOOK="$WEBHOOK" DR_BUCKET="$BUCKET" DR_VERSION="$VERSION_FILE" \
  DR_LAB="$LAB" DR_PGUSER="$PGUSER" DR_PGDATABASE="$PGDATABASE" DR_S3="$S3" DR_ROOT="$ROOT"

# DR_PY_CONTAINER (set by the promote drill) runs this helper inside a container
# that has Python and a Docker client. The SSH lab leaves it unset and uses host python3.
map_drill_path() {
  local p="$1"
  if [[ -z "${DR_PY_CONTAINER:-}" || -z "$p" ]]; then
    printf '%s' "$p"
    return
  fi
  local state="${DR_HOST_STATE:-}"
  local root="${DR_HOST_ROOT:-}"
  if [[ -n "$state" && "$p" == "$state"* ]]; then
    printf '/state%s' "${p#"$state"}"
    return
  fi
  if [[ -n "$root" && "$p" == "$root"* ]]; then
    printf '/workspace%s' "${p#"$root"}"
    return
  fi
  printf '%s' "$p"
}

if [[ -n "${DR_PY_CONTAINER:-}" ]]; then
  DR_OUT="$(map_drill_path "$DR_OUT")"
  DR_TIMING="$(map_drill_path "$DR_TIMING")"
  DR_RERUN="$(map_drill_path "$DR_RERUN")"
  DR_KEYS="$(map_drill_path "$DR_KEYS")"
  DR_WEBHOOK="$(map_drill_path "$DR_WEBHOOK")"
  DR_VERSION="$(map_drill_path "$DR_VERSION")"
  DR_S3="$(map_drill_path "$DR_S3")"
  DR_ROOT="$(map_drill_path "$DR_ROOT")"
fi

run_evidence_py() {
  if [[ -n "${DR_PY_CONTAINER:-}" ]]; then
    docker exec -i -w /workspace \
      -e DR_OUT -e DR_NEW -e DR_OLD -e DR_BASE \
      -e DR_PG_NEW -e DR_PG_OLD -e DR_TIMING -e DR_RERUN -e DR_KEYS \
      -e DR_WEBHOOK -e DR_BUCKET -e DR_VERSION \
      -e DR_LAB -e DR_PGUSER -e DR_PGDATABASE -e DR_S3 -e DR_ROOT \
      "$DR_PY_CONTAINER" python -
  else
    python3 -
  fi
}

run_evidence_py <<'PY'
import json, os, subprocess, urllib.request, datetime, hashlib, pathlib, zipfile

out = pathlib.Path(os.environ["DR_OUT"])
lab = os.environ["DR_LAB"] == "1"
bucket = os.environ["DR_BUCKET"]
checks = []
fail = []

def add(name, ok, detail):
    verdict = "PASS" if ok else "FAIL"
    checks.append({"verdict": verdict, "name": name, "ok": bool(ok), "detail": detail})
    if not ok:
        fail.append(name)

def lag_after_value(raw):
    if raw in (None, -1, "-1"):
        return "n/a (old primary fenced, no replication)"
    return raw

def load_json(path):
    if not path or not os.path.isfile(path):
        return {}
    try:
        return json.loads(pathlib.Path(path).read_text(encoding="utf-8"))
    except Exception as exc:
        return {"error": str(exc)}

def curl_json(url):
    with urllib.request.urlopen(url, timeout=8) as resp:
        return json.load(resp)

def health(url):
    try:
        body = curl_json(url.rstrip("/") + "/healthz")
        return body, ""
    except Exception as exc:
        return None, str(exc)

def psql(container, sql):
    if not container:
        return "", "container unset"
    proc = subprocess.run(
        ["docker", "exec", container, "psql", "-U", os.environ["DR_PGUSER"], "-d", os.environ["DR_PGDATABASE"], "-Atqc", sql],
        capture_output=True, text=True,
    )
    if proc.returncode != 0:
        return "", (proc.stderr or proc.stdout or "psql failed").strip()
    return proc.stdout.strip(), ""

def read_text(path):
    if not path or not os.path.isfile(path):
        return ""
    return pathlib.Path(path).read_text(encoding="utf-8", errors="replace").strip()

now = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
version = read_text(os.environ["DR_VERSION"]) or "unknown"
timing = load_json(os.environ["DR_TIMING"])
rerun_raw = load_json(os.environ.get("DR_RERUN", ""))
if rerun_raw:
    rerun = {
        "ran": bool(rerun_raw.get("ran")),
        "exit_code": rerun_raw.get("exit_code"),
        "skipped": rerun_raw.get("skipped") or [],
    }
else:
    rerun = {"ran": False, "exit_code": None, "skipped": []}

if lab:
    url = os.environ["DR_BASE"]
    body, err = health(url)
    add("healthz", body is not None and body.get("status") == "ok", err or json.dumps(body))
    evidence = {
        "generated_at": now,
        "mode": "lab",
        "result": "pass" if not fail else "fail",
        "verdicts": {c["name"]: c["verdict"] for c in checks},
        "base_url": url,
        "healthz": body,
        "promote": timing or None,
        "versions": {"version_file": version},
        "checks": checks,
        "notes": [
            "SSH lab pack records healthz and the repo VERSION.",
            "Promote timing is included only when --timing-file was passed.",
            "Scripted promote is not automatic multi-AZ failover.",
        ],
    }
else:
    new_url = os.environ["DR_NEW"]
    old_url = os.environ["DR_OLD"]
    new_h, new_err = health(new_url)
    old_h, old_err = health(old_url) if old_url else (None, "unset")
    add("new healthz", new_h is not None and new_h.get("status") == "ok", new_err or json.dumps(new_h))
    leader_ok = bool(new_h) and new_h.get("is_leader") is True
    add("dispatcher on new leader", leader_ok, "is_leader must be true so the outbox dispatcher runs here")
    if old_url:
        old_leader = bool(old_h) and old_h.get("is_leader") is True
        add("old storage is not leader", not old_leader, old_err or json.dumps(old_h))

    proc = None
    if os.environ["DR_PG_OLD"]:
        subprocess.run(
            ["docker", "exec", os.environ["DR_PG_OLD"], "psql", "-U", os.environ["DR_PGUSER"], "-d", os.environ["DR_PGDATABASE"], "-c", "DROP TABLE IF EXISTS dr_fence_probe;"],
            capture_output=True, text=True,
        )
        proc = subprocess.run(
            ["docker", "exec", os.environ["DR_PG_OLD"], "psql", "-U", os.environ["DR_PGUSER"], "-d", os.environ["DR_PGDATABASE"], "-v", "ON_ERROR_STOP=1", "-c", "CREATE TABLE dr_fence_probe(id int);"],
            capture_output=True, text=True,
        )
    if proc is None:
        add("old primary refuses SQL writes", False, "pg container unset")
    elif proc.returncode != 0:
        add("old primary refuses SQL writes", True, (proc.stderr or proc.stdout or "write failed").strip().splitlines()[-1][:240])
    else:
        subprocess.run(["docker", "exec", os.environ["DR_PG_OLD"], "psql", "-U", os.environ["DR_PGUSER"], "-d", os.environ["DR_PGDATABASE"], "-c", "DROP TABLE IF EXISTS dr_fence_probe;"], capture_output=True)
        add("old primary refuses SQL writes", False, "CREATE TABLE succeeded")

    body_path = out / "post-promote-object.bin"
    body_path.write_bytes(b"dr-post-promote\n")
    key = "dr-post-promote"
    put = subprocess.run(["python3", os.environ["DR_S3"], "put", new_url, bucket, key, str(body_path)], capture_output=True, text=True)
    got = out / "post-promote-got.bin"
    get = subprocess.run(["python3", os.environ["DR_S3"], "get", new_url, bucket, key, str(got)], capture_output=True, text=True)
    match = got.is_file() and got.read_bytes() == body_path.read_bytes()
    add("object write/read on new leader", put.returncode == 0 and get.returncode == 0 and match, f"put={put.stdout.strip()} get={get.stdout.strip()}")

    if old_url:
        old_put = subprocess.run(["python3", os.environ["DR_S3"], "put", old_url, bucket, "dr-old-must-fail", str(body_path)], capture_output=True, text=True)
        add("old storage refuses object write", old_put.returncode != 0, f"http {old_put.stdout.strip()}")

    keys = []
    if os.environ["DR_KEYS"] and os.path.isfile(os.environ["DR_KEYS"]):
        keys = [ln.strip() for ln in pathlib.Path(os.environ["DR_KEYS"]).read_text(encoding="utf-8").splitlines() if ln.strip()]
    counts = {}
    hook_err = ""
    if os.environ["DR_WEBHOOK"] and os.path.isfile(os.environ["DR_WEBHOOK"]):
        for line in pathlib.Path(os.environ["DR_WEBHOOK"]).read_text(encoding="utf-8", errors="replace").splitlines():
            line = line.strip()
            if not line:
                continue
            try:
                ev = json.loads(line)
            except Exception:
                continue
            if ev.get("event") != "ObjectCreated":
                continue
            data = ev.get("data")
            if isinstance(data, str):
                try:
                    data = json.loads(data)
                except Exception:
                    data = {}
            if not isinstance(data, dict):
                continue
            k = data.get("key") or ""
            if k:
                counts[k] = counts.get(k, 0) + 1
    elif keys:
        hook_err = "webhook log missing"
    missing = [k for k in keys if counts.get(k, 0) < 1]
    dupes = {k: n for k, n in counts.items() if n > 1 and k in set(keys)}
    extra_by_key = {k: n - 1 for k, n in dupes.items()}
    extra_total = sum(extra_by_key.values())
    extra_max = max(extra_by_key.values()) if extra_by_key else 0
    add("no lost objects", not missing and not hook_err, hook_err or ("missing " + ", ".join(missing) if missing else f"{len(keys)} keys delivered"))
    add(
        "duplicates within at-least-once",
        extra_max <= 1 and not hook_err,
        "none" if extra_total == 0 else f"{extra_total} extra deliveries across {len(extra_by_key)} keys (max {extra_max} per key)",
    )

    outbox = {}
    text, err = psql(os.environ["DR_PG_NEW"], "SELECT status || ' ' || count(*) FROM event_outbox GROUP BY status ORDER BY status;")
    if err:
        add("outbox readable on new primary", False, err)
    else:
        for line in text.splitlines():
            parts = line.split()
            if len(parts) == 2:
                outbox[parts[0]] = int(parts[1])
        add("outbox readable on new primary", True, json.dumps(outbox))
    old_keys, _ = psql(os.environ["DR_PG_OLD"], "SELECT key FROM event_outbox WHERE event_type='ObjectCreated';")
    new_keys, new_err = psql(os.environ["DR_PG_NEW"], "SELECT key FROM event_outbox WHERE event_type='ObjectCreated';")
    old_set = {k for k in old_keys.splitlines() if k}
    new_set = {k for k in new_keys.splitlines() if k}
    # Delivered rows stay until retention, so a key on the old primary must still
    # be on the new primary. A key only in the webhook was delivered and may
    # still be in the table. Loss is an old key that is on neither side.
    lost_sql = sorted(k for k in old_set if k not in new_set and counts.get(k, 0) < 1)
    if os.environ["DR_PG_OLD"] and os.environ["DR_PG_NEW"] and not new_err:
        add("no outbox loss across promote", not lost_sql, "lost " + ", ".join(lost_sql) if lost_sql else f"old={len(old_set)} new={len(new_set)}")

    pg_new, _ = psql(os.environ["DR_PG_NEW"], "SHOW server_version;")
    pg_old, _ = psql(os.environ["DR_PG_OLD"], "SHOW server_version;")
    image = ""
    proc = subprocess.run(["docker", "inspect", "-f", "{{.Config.Image}}", "ds-promote-b"], capture_output=True, text=True)
    if proc.returncode == 0:
        image = proc.stdout.strip()

    evidence = {
        "generated_at": now,
        "mode": "promote",
        "result": "pass" if not fail else "fail",
        "verdicts": {c["name"]: c["verdict"] for c in checks},
        "promote": {
            "started_at": timing.get("started_at"),
            "finished_at": timing.get("finished_at"),
            "seconds": timing.get("seconds"),
            "lag_bytes_before": timing.get("lag_bytes_before"),
            "lag_bytes_after": lag_after_value(timing.get("lag_bytes_after")),
        },
        "idempotent_rerun": rerun,
        "healthz": {"new": new_h, "old": old_h},
        "object_write_read": {"bucket": bucket, "key": key, "ok": put.returncode == 0 and match},
        "outbox": outbox,
        "delivery": {
            "committed_keys": len(keys),
            "webhook_keys": len(counts),
            "missing": missing,
            "duplicate_extra": extra_total,
            "duplicate_max_per_key": extra_max,
        },
        "dispatcher_node": (new_h or {}).get("node_id"),
        "versions": {
            "version_file": version,
            "postgres_new": pg_new,
            "postgres_old": pg_old,
            "storage_image": image,
        },
        "checks": checks,
        "notes": [
            "Scripted promote of one Postgres standby. Not automatic multi-AZ failover.",
            "Delivery is at-least-once. One extra post per in-flight row is allowed. A second extra of the same key fails the pack.",
            "A missing key is loss and fails the pack.",
            "Object bytes are local to each storage node. The post-promote read is a new object on the new leader.",
            "The old primary is fenced with default_transaction_read_only (ALTER SYSTEM, not replicated). An outbox write on that database stops the dispatcher and drops the lease without another publish.",
        ],
    }

def md_line(title, value):
    return f"- {title}: {value}"

lines = [
    "# DR evidence",
    "",
    f"Generated: {now}",
    f"Result: {evidence['result'].upper()}",
    "",
    "## Promote",
    "",
]
prom = evidence.get("promote") or {}
if lab:
    lines += [
        md_line("mode", "SSH lab health check"),
        md_line("base url", evidence.get("base_url")),
        md_line("healthz", "ok" if not fail else "failed"),
        "",
    ]
else:
    lines += [
        md_line("started", prom.get("started_at")),
        md_line("finished", prom.get("finished_at")),
        md_line("seconds", prom.get("seconds")),
        md_line("replication lag bytes before", prom.get("lag_bytes_before")),
        md_line("replication lag bytes after", prom.get("lag_bytes_after")),
        "",
        "## Idempotent re-run",
        "",
        md_line("ran", "yes" if (evidence.get("idempotent_rerun") or {}).get("ran") else "no"),
        md_line("exit code", (evidence.get("idempotent_rerun") or {}).get("exit_code")),
        md_line("skipped", ", ".join((evidence.get("idempotent_rerun") or {}).get("skipped") or []) or "nothing"),
        "",
        "## Post-promote checks",
        "",
    ]
    for c in checks:
        lines.append(f"- {'PASS' if c['ok'] else 'FAIL'} {c['name']}: {c['detail']}")
    lines += [
        "",
        "## Versions",
        "",
        md_line("VERSION file", evidence["versions"].get("version_file")),
        md_line("Postgres new", evidence["versions"].get("postgres_new")),
        md_line("Postgres old", evidence["versions"].get("postgres_old")),
        md_line("storage image", evidence["versions"].get("storage_image")),
        "",
    ]
lines += [
    "## What this is",
    "",
    "Scripted promote of a Postgres standby, with the old primary fenced so it refuses writes.",
    "It is not automatic multi-AZ failover.",
    "",
]
(out / "summary.md").write_text("\n".join(lines) + "\n", encoding="utf-8")
(out / "evidence.json").write_text(json.dumps(evidence, indent=2) + "\n", encoding="utf-8")
zip_path = out.parent / (out.name + ".zip")
with zipfile.ZipFile(zip_path, "w", zipfile.ZIP_DEFLATED) as zf:
    for p in out.rglob("*"):
        if p.is_file():
            zf.write(p, p.relative_to(out.parent))
print(f"evidence: {out}")
print(f"zip: {zip_path}")
print(f"result: {evidence['result']}")
raise SystemExit(0 if not fail else 5)
PY
