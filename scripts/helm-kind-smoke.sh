#!/usr/bin/env bash
# Lint and render the datasafes3 chart. With KIND_SMOKE=1, install it into kind
# and put/get an object with the AWS CLI.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
CHART="$ROOT/deploy/helm/datasafes3"
VER="$(tr -d '[:space:]' < "$ROOT/VERSION")"
TAG="v${VER}"
IMAGE="ghcr.io/direktorbani/datasafe-storage-server:${TAG}"

helm lint "$CHART" -f "$CHART/values-ci.yaml"
helm template ds "$CHART" -f "$CHART/values-ci.yaml" >/dev/null
helm template ds "$CHART" -f "$CHART/values-production.yaml" >/dev/null
helm template ds "$CHART" -f "$CHART/values-ha.yaml" >/dev/null

# Rendered production output must not contain the historical default secrets.
rendered="$(helm template ds "$CHART" -f "$CHART/values-production.yaml")"
if printf '%s' "$rendered" | grep -E 'datasafesecret|datasafe-jwt-secret'; then
  echo "production render contains a default secret" >&2
  exit 1
fi
if ! printf '%s' "$rendered" | grep -q 'kind: StatefulSet'; then
  echo "production render has no StatefulSet" >&2
  exit 1
fi
if ! printf '%s' "$rendered" | grep -q 'readOnlyRootFilesystem: true'; then
  echo "production render is missing a read-only root filesystem" >&2
  exit 1
fi

if [ "${KIND_SMOKE:-}" != "1" ]; then
  echo "helm lint and template ok (set KIND_SMOKE=1 to install)"
  exit 0
fi

command -v kind >/dev/null
command -v kubectl >/dev/null
command -v aws >/dev/null
docker image inspect "$IMAGE" >/dev/null

kind delete cluster --name datasafes3 >/dev/null 2>&1 || true
kind create cluster --name datasafes3
kind load docker-image "$IMAGE" --name datasafes3

helm upgrade --install ds "$CHART" \
  --namespace datasafes3 --create-namespace \
  -f "$CHART/values-ci.yaml" \
  --set storageServer.image.repository=ghcr.io/direktorbani/datasafe-storage-server \
  --set storageServer.image.tag="$TAG" \
  --set storageServer.image.pullPolicy=Never \
  --wait --timeout 180s

kubectl -n datasafes3 rollout status statefulset/ds-datasafes3 --timeout=180s

kubectl -n datasafes3 port-forward svc/ds-datasafes3-s3 19000:9000 >/tmp/datasafes3-pf.log 2>&1 &
pf=$!
trap 'kill "$pf" >/dev/null 2>&1 || true; kind delete cluster --name datasafes3 >/dev/null 2>&1 || true' EXIT
for _ in $(seq 1 30); do
  if curl -sf http://127.0.0.1:19000/healthz >/dev/null; then
    break
  fi
  sleep 1
done
curl -sf http://127.0.0.1:19000/healthz >/dev/null

workdir="$(mktemp -d)"
printf 'kind-smoke' > "$workdir/obj.txt"
export AWS_ACCESS_KEY_ID=datasafe
export AWS_SECRET_ACCESS_KEY=ci-secret-not-default
export AWS_DEFAULT_REGION=us-east-1
export AWS_EC2_METADATA_DISABLED=true
aws --endpoint-url http://127.0.0.1:19000 s3 mb s3://kind-smoke
aws --endpoint-url http://127.0.0.1:19000 s3 cp "$workdir/obj.txt" s3://kind-smoke/obj.txt
aws --endpoint-url http://127.0.0.1:19000 s3 cp s3://kind-smoke/obj.txt "$workdir/got.txt"
cmp "$workdir/obj.txt" "$workdir/got.txt"
aws --endpoint-url http://127.0.0.1:19000 s3 rm s3://kind-smoke/obj.txt
echo "kind smoke put/get ok"
