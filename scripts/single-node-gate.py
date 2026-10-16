#!/usr/bin/env python3
"""Assertions for the single-node regression gate.

The shell driver (scripts/single-node-gate.sh) starts containers. This file
checks budgets, broker logs, outbound TCP, and smoke payloads. `self-test`
needs no Docker.
"""

from __future__ import annotations

import argparse
import json
import re
import sys
from pathlib import Path

BROKER_PORTS = {5432, 4222, 9092, 5672, 9094, 6650}
LISTEN_PORT = 9000

LOG_PATTERNS = (
    re.compile(r"nats://", re.I),
    re.compile(r"\bkafka\b", re.I),
    re.compile(r"amqp://", re.I),
    re.compile(r"dial tcp", re.I),
    re.compile(r"lookup postgres", re.I),
    re.compile(r"no such host", re.I),
    re.compile(r"connection refused", re.I),
)


def median(values: list[int]) -> int:
    if not values:
        raise SystemExit("no samples")
    ordered = sorted(values)
    # Odd counts: the middle. Even counts: the upper middle, so a budget
    # check does not round a high sample down.
    return ordered[len(ordered) // 2]


def _ipv4(hex_ip: str) -> str:
    raw = bytes.fromhex(hex_ip)
    return ".".join(str(b) for b in raw[::-1])


def _is_loopback(hex_ip: str) -> bool:
    if len(hex_ip) == 8:
        ip = _ipv4(hex_ip)
        return ip.startswith("127.")
    # ::1 in /proc/net/tcp6 is 32 hex digits, last word little-endian 1.
    if hex_ip == "00000000000000000000000001000000":
        return True
    return False


def unexpected_tcp(text: str) -> list[str]:
    """Return human-readable sockets that are not inbound or loopback."""
    bad: list[str] = []
    for line in text.splitlines():
        line = line.strip()
        if not line or line.startswith("sl ") or "local_address" in line:
            continue
        parts = line.split()
        if len(parts) < 4 or ":" not in parts[1] or ":" not in parts[2]:
            continue
        local, remote, state = parts[1], parts[2], parts[3]
        if state.upper() == "0A":  # LISTEN
            continue
        local_ip, local_port_hex = local.split(":")
        remote_ip, remote_port_hex = remote.split(":")
        local_port = int(local_port_hex, 16)
        remote_port = int(remote_port_hex, 16)
        if remote_port == 0 and remote_ip.strip("0") == "":
            continue
        if local_port == LISTEN_PORT:
            continue
        if _is_loopback(remote_ip):
            continue
        kind = "broker" if remote_port in BROKER_PORTS else "outbound"
        shown = remote
        if len(remote_ip) == 8:
            shown = f"{_ipv4(remote_ip)}:{remote_port}"
        bad.append(f"{kind} {shown} state={state} local_port={local_port}")
    return bad


def log_hits(text: str) -> list[str]:
    hits: list[str] = []
    for line in text.splitlines():
        if any(p.search(line) for p in LOG_PATTERNS):
            hits.append(line.strip())
            if len(hits) >= 20:
                break
    return hits


def metrics_problems(text: str) -> list[str]:
    pending = None
    problems: list[str] = []
    for line in text.splitlines():
        if not line or line.startswith("#"):
            continue
        name, _, rest = line.partition(" ")
        # Counter series look like name{labels} value [timestamp].
        base = name.split("{", 1)[0]
        try:
            value = float(rest.split()[0])
        except (IndexError, ValueError):
            continue
        if base == "datasafe_events_outbox_pending":
            pending = value
        elif base in (
            "datasafe_events_delivered_total",
            "datasafe_events_dropped_total",
            "datasafe_events_dead_total",
        ) and value > 0:
            problems.append(f"{name}={value}")
    if pending is None:
        problems.append("datasafe_events_outbox_pending is missing")
    elif pending != 0:
        problems.append(f"datasafe_events_outbox_pending={pending}")
    return problems


def budget_result(samples: dict) -> tuple[bool, str]:
    base_startup = [int(x) for x in samples["baseline"]["startup_ms"]]
    pr_startup = [int(x) for x in samples["pr"]["startup_ms"]]
    base_rss = [int(x) for x in samples["baseline"]["rss_kib"]]
    pr_rss = [int(x) for x in samples["pr"]["rss_kib"]]
    if not base_startup or not pr_startup or not base_rss or not pr_rss:
        raise SystemExit("budget samples are empty")
    b_start, p_start = median(base_startup), median(pr_startup)
    b_rss, p_rss = median(base_rss), median(pr_rss)
    start_limit = b_start + 1000
    rss_limit = (b_rss * 110) // 100
    start_ok = p_start <= start_limit
    rss_ok = p_rss <= rss_limit
    ok = start_ok and rss_ok
    title = samples.get("title") or "Single-node resource budget"

    def fmt_s(ms: int) -> str:
        return f"{ms / 1000:.3f}"

    rows = [
        "| image | runs | startup median | startup min | startup max | idle RSS median | RSS min | RSS max |",
        "|---|---:|---:|---:|---:|---:|---:|---:|",
        _row(samples.get("baseline_image", "v1.5.0"), base_startup, base_rss),
        _row(samples.get("pr_image", "pr"), pr_startup, pr_rss),
        f"| limit |  | {fmt_s(start_limit)} s |  |  | {rss_limit} KiB |  |  |",
        f"| result |  | {'pass' if start_ok else 'FAIL'} (median {fmt_s(p_start)} s, baseline {fmt_s(b_start)} s) |  |  | {'pass' if rss_ok else 'FAIL'} (median {p_rss} KiB, baseline {b_rss} KiB) |  |  |",
    ]
    note = (
        "Startup is container create until HTTP /healthz. "
        "Idle RSS is the median of three VmRSS samples of PID 1 after warm-up, "
        "then the median across interleaved runs of the same metadata backend. "
        "Fail when PR startup median > baseline median + 1s, or PR RSS median > baseline + 10%."
    )
    ids = ""
    if samples.get("baseline_id") or samples.get("pr_id"):
        ids = (
            f"\n\nBaseline id `{samples.get('baseline_id', '')}`  \n"
            f"PR id `{samples.get('pr_id', '')}`\n"
        )
    body = f"### {title}\n\n" + "\n".join(rows) + "\n\n" + note + ids
    return ok, body


def _row(label: str, startup: list[int], rss: list[int]) -> str:
    return (
        f"| `{label}` | {len(startup)} | {median(startup) / 1000:.3f} s | "
        f"{min(startup) / 1000:.3f} s | {max(startup) / 1000:.3f} s | "
        f"{median(rss)} KiB | {min(rss)} KiB | {max(rss)} KiB |"
    )


def _load_json(path: str) -> dict:
    return json.loads(Path(path).read_text(encoding="utf-8"))


def cmd_activity(path: str) -> None:
    data = _load_json(path)
    events = data.get("events") or []
    total = int(data.get("total") or 0)
    if total < 1 and len(events) < 1:
        raise SystemExit(f"activity has no entries: {data!r}"[:500])


def cmd_inventory(created_path: str, listed_path: str) -> None:
    created = _load_json(created_path)
    if created.get("status") != "completed":
        raise SystemExit(f"inventory job not completed: {created!r}"[:500])
    job_id = created.get("id")
    if not job_id:
        raise SystemExit("inventory create response has no id")
    listed = _load_json(listed_path)
    ids = [j.get("id") for j in (listed.get("jobs") or [])]
    if job_id not in ids:
        raise SystemExit(f"inventory list missing {job_id}: {ids}")


def cmd_s3_list(path: str, key: str) -> None:
    data = _load_json(path)
    keys = [c.get("Key") for c in (data.get("Contents") or [])]
    if key not in keys:
        raise SystemExit(f"s3 list missing {key}: {keys}")


def self_test() -> None:
    assert median([10, 1, 5, 8, 3]) == 5
    assert median([1, 2, 3, 4]) == 3  # upper middle

    listen = "0: 00000000:2328 00000000:0000 0A 00000000:00000000 00:00000000 00000000 0 0 1"
    inbound = "1: 020011AC:2328 010011AC:C3A2 01 00000000:00000000 00:00000000 00000000 0 0 2"
    loopback = "2: 0100007F:C351 0100007F:2328 01 00000000:00000000 00:00000000 00000000 0 0 3"
    header = "sl local_address rem_address st"
    quiet = "\n".join([header, listen, inbound, loopback])
    assert unexpected_tcp(quiet) == [], unexpected_tcp(quiet)

    # 4222 = 0x107E. Remote 10.0.0.1, local ephemeral.
    broker = "3: 020011AC:C350 0100000A:107E 01 00000000:00000000 00:00000000 00000000 0 0 4"
    found = unexpected_tcp(quiet + "\n" + broker)
    assert found and found[0].startswith("broker 10.0.0.1:4222"), found

    other = "4: 020011AC:C352 08080808:01BB 01 00000000:00000000 00:00000000 00000000 0 0 5"
    outbound = unexpected_tcp(other)
    assert outbound and outbound[0].startswith("outbound 8.8.8.8:443"), outbound

    assert log_hits('{"msg":"http server listening"}') == []
    assert log_hits("dial tcp 10.0.0.1:4222: connect: connection refused")
    assert not log_hits("metadata backend bolt")

    metrics = "\n".join(
        [
            "# HELP datasafe_events_outbox_pending x",
            "datasafe_events_outbox_pending 0",
            "datasafe_events_delivered_total 0",
        ]
    )
    assert metrics_problems(metrics) == []
    assert any("missing" in p for p in metrics_problems("up 1\n"))
    assert any("pending=2" in p for p in metrics_problems("datasafe_events_outbox_pending 2\n"))
    assert any(
        "delivered" in p
        for p in metrics_problems(
            'datasafe_events_outbox_pending 0\ndatasafe_events_delivered_total{sink="webhook"} 1\n'
        )
    )

    ok, text = budget_result(
        {
            "baseline_image": "base",
            "pr_image": "pr",
            "baseline": {"startup_ms": [100, 200, 300, 400, 500], "rss_kib": [1000] * 5},
            "pr": {"startup_ms": [400, 500, 600, 700, 800], "rss_kib": [1100] * 5},
        }
    )
    assert ok, text
    assert "pass" in text

    over_rss, text = budget_result(
        {
            "baseline_image": "base",
            "pr_image": "pr",
            "baseline": {"startup_ms": [300] * 5, "rss_kib": [1000] * 5},
            "pr": {"startup_ms": [300] * 5, "rss_kib": [1101] * 5},
        }
    )
    assert not over_rss and "FAIL" in text

    over_start, _ = budget_result(
        {
            "baseline_image": "base",
            "pr_image": "pr",
            "baseline": {"startup_ms": [300] * 5, "rss_kib": [1000] * 5},
            "pr": {"startup_ms": [1301] * 5, "rss_kib": [1000] * 5},
        }
    )
    assert not over_start

    edge, _ = budget_result(
        {
            "baseline_image": "base",
            "pr_image": "pr",
            "baseline": {"startup_ms": [300] * 5, "rss_kib": [1000] * 5},
            "pr": {"startup_ms": [1300] * 5, "rss_kib": [1100] * 5},
        }
    )
    assert edge  # exactly +1s and exactly +10% still pass

    pg_slow, pg_text = budget_result(
        {
            "title": "Postgres resource budget",
            "baseline": {"startup_ms": [300] * 5, "rss_kib": [1000] * 5},
            "pr": {"startup_ms": [9000] * 5, "rss_kib": [1060] * 5},
        }
    )
    assert not pg_slow and "Postgres resource budget" in pg_text, pg_text
    pg_bad, _ = budget_result(
        {
            "title": "Postgres resource budget",
            "baseline": {"startup_ms": [300] * 5, "rss_kib": [1000] * 5},
            "pr": {"startup_ms": [300] * 5, "rss_kib": [1101] * 5},
        }
    )
    assert not pg_bad

    print("single-node-gate self-test ok")


def main(argv: list[str] | None = None) -> None:
    parser = argparse.ArgumentParser(description="single-node gate checks")
    sub = parser.add_subparsers(dest="cmd", required=True)

    sub.add_parser("self-test")
    med = sub.add_parser("median")
    med.add_argument("values", nargs="+")

    tcp = sub.add_parser("tcp-check")
    tcp.add_argument("path")
    logs = sub.add_parser("log-check")
    logs.add_argument("path")
    met = sub.add_parser("metrics-check")
    met.add_argument("path")

    bud = sub.add_parser("budget")
    bud.add_argument("samples")
    bud.add_argument("--summary", default="")

    act = sub.add_parser("activity-check")
    act.add_argument("path")
    inv = sub.add_parser("inventory-check")
    inv.add_argument("created")
    inv.add_argument("listed")
    s3 = sub.add_parser("s3-list-check")
    s3.add_argument("path")
    s3.add_argument("key")

    args = parser.parse_args(argv)
    if args.cmd == "self-test":
        self_test()
        return
    if args.cmd == "median":
        print(median([int(v) for v in args.values]))
        return
    if args.cmd == "tcp-check":
        bad = unexpected_tcp(Path(args.path).read_text(encoding="utf-8", errors="replace"))
        if bad:
            print("unexpected TCP sockets:", file=sys.stderr)
            print("\n".join(bad), file=sys.stderr)
            raise SystemExit(1)
        print("tcp-check ok")
        return
    if args.cmd == "log-check":
        hits = log_hits(Path(args.path).read_text(encoding="utf-8", errors="replace"))
        if hits:
            print("broker or dial lines in logs:", file=sys.stderr)
            print("\n".join(hits), file=sys.stderr)
            raise SystemExit(1)
        print("log-check ok")
        return
    if args.cmd == "metrics-check":
        problems = metrics_problems(Path(args.path).read_text(encoding="utf-8", errors="replace"))
        if problems:
            print("outbox metrics:", file=sys.stderr)
            print("\n".join(problems), file=sys.stderr)
            raise SystemExit(1)
        print("metrics-check ok (outbox pending 0, no deliveries)")
        return
    if args.cmd == "budget":
        ok, body = budget_result(_load_json(args.samples))
        print(body)
        if args.summary:
            with open(args.summary, "a", encoding="utf-8") as fh:
                fh.write("\n" + body + "\n")
        if not ok:
            raise SystemExit(1)
        return
    if args.cmd == "activity-check":
        cmd_activity(args.path)
        print("activity-check ok")
        return
    if args.cmd == "inventory-check":
        cmd_inventory(args.created, args.listed)
        print("inventory-check ok")
        return
    if args.cmd == "s3-list-check":
        cmd_s3_list(args.path, args.key)
        print("s3-list-check ok")
        return
    raise SystemExit(f"unknown command {args.cmd}")


if __name__ == "__main__":
    main()
