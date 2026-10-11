#!/usr/bin/env bash
# promtool check rules, then provision the Compose Grafana image and read the
# overview dashboard back from the HTTP API.
set -eu

root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
cd "$root"

prom_image=$(grep -E 'image: prom/prometheus:' docker-compose.yml | head -1 | awk '{print $2}')
grafana_image=$(grep -E 'image: grafana/grafana:' docker-compose.yml | head -1 | awk '{print $2}')
if [ -z "$prom_image" ] || [ -z "$grafana_image" ]; then
  echo "compose image pin missing" >&2
  exit 1
fi
echo "promtool image ${prom_image}"
echo "grafana image ${grafana_image}"

docker run --rm --entrypoint promtool \
  -v "$root/deploy/docker/prometheus/alerts.yml:/alerts.yml:ro" \
  "$prom_image" check rules /alerts.yml

python3 - <<'PY'
import json
from pathlib import Path
need = [
    "Event delivery",
    "datasafe_events_outbox_pending",
    "rate(datasafe_events_delivered_total[5m])",
    "rate(datasafe_events_failed_total[5m])",
    "rate(datasafe_events_dead_total[5m])",
    "rate(datasafe_events_dropped_total[5m])",
    "histogram_quantile(0.50, sum by (le, sink) (rate(datasafe_events_delivery_lag_seconds_bucket[5m])))",
    "histogram_quantile(0.95, sum by (le, sink) (rate(datasafe_events_delivery_lag_seconds_bucket[5m])))",
    "histogram_quantile(0.99, sum by (le, sink) (rate(datasafe_events_delivery_lag_seconds_bucket[5m])))",
    "datasafe_events_bridge_restarts_total",
    "datasafe_events_sink_degraded",
]
for path in (
    "deploy/docker/grafana/dashboards/datasafe-overview.json",
    "deploy/helm/datasafe/dashboards/datasafe-overview.json",
):
    text = Path(path).read_text()
    json.loads(text)
    missing = [item for item in need if item not in text]
    if missing:
        raise SystemExit(f"{path} missing {missing}")
print("dashboard JSON contains the Event delivery queries")
PY

name=datasafe-grafana-check
port=${GRAFANA_CHECK_PORT:-13091}
docker rm -f "$name" >/dev/null 2>&1 || true
docker run -d --name "$name" \
  -p "127.0.0.1:${port}:3000" \
  -e GF_SECURITY_ADMIN_USER=admin \
  -e GF_SECURITY_ADMIN_PASSWORD=admin \
  -e GF_USERS_ALLOW_SIGN_UP=false \
  -v "$root/deploy/docker/grafana/provisioning:/etc/grafana/provisioning:ro" \
  -v "$root/deploy/docker/grafana/dashboards:/var/lib/grafana/dashboards:ro" \
  "$grafana_image" >/dev/null
trap 'docker rm -f "$name" >/dev/null 2>&1 || true' EXIT

ok=0
for _ in $(seq 1 60); do
  if curl -sf -u admin:admin "http://127.0.0.1:${port}/api/health" >/dev/null; then
    body=$(curl -sf -u admin:admin "http://127.0.0.1:${port}/api/dashboards/uid/datasafe-overview" || true)
    if printf '%s' "$body" | python3 -c 'import json,sys
raw=sys.stdin.read()
if not raw.strip():
    raise SystemExit(1)
doc=json.loads(raw)
dash=doc.get("dashboard") or {}
titles=[]
def walk(panels):
    for p in panels or []:
        titles.append(p.get("title") or "")
        walk(p.get("panels"))
walk(dash.get("panels"))
need=["Event delivery","Outbox pending","Delivery rates","Delivery lag","Sink degraded","Event bridge restarts"]
missing=[t for t in need if t not in titles]
if missing:
    raise SystemExit(1)
print("grafana loaded", dash.get("uid"), "panels", len(titles))
'; then
      ok=1
      break
    fi
  fi
  sleep 2
done
if [ "$ok" != 1 ]; then
  echo "Grafana did not serve the Event delivery row" >&2
  docker logs "$name" >&2 || true
  exit 1
fi
