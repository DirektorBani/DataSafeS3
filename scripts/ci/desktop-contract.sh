#!/bin/sh
# Desktop contract gate (spec 4.6.8). No Desktop release is published yet, so
# this job runs the suite in the repo and packs it as the release artifact.
# A fixture that renames a required response field fails this job.
set -eu
root=$(CDPATH= cd -- "$(dirname "$0")/../.." && pwd)
cd "$root"
echo "desktop-contract: in-repo suite (no published Desktop release is pinned yet)"
go test ./clients/desktop/contract -count=1 -timeout 15m
go test ./cmd/datasafe-agent -count=1 -timeout 15m -run 'TestFeatureColumns|TestParseCapsFixtures|TestDetectVersions|TestClientVersionPolicy'
tar -czf desktop-contract-1.0.0-dev.tar.gz -C clients/desktop contract
echo "wrote desktop-contract-1.0.0-dev.tar.gz"
