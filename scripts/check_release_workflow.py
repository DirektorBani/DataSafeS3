#!/usr/bin/env python3
"""Fail when release.yml would not publish the tagged storage-server image.

The image must include the Kafka bridge binary and link version/commit from
VERSION and git. This script does not build or push.
"""

from __future__ import annotations

import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
RELEASE = ROOT / ".github/workflows/release.yml"
DOCKERFILE = ROOT / "deploy/docker/Dockerfile"


def fail(msg: str) -> None:
    print(f"release workflow: {msg}", file=sys.stderr)
    sys.exit(1)


def main() -> None:
    workflow = RELEASE.read_text(encoding="utf-8")
    dockerfile = DOCKERFILE.read_text(encoding="utf-8")
    errors: list[str] = []

    def need(text: str, needle: str, where: str) -> None:
        if needle not in text:
            errors.append(f"{where}: missing {needle!r}")

    need(workflow, 'tags:\n      - "v*"', "release.yml")
    need(workflow, "bash scripts/check-release-pins.sh", "release.yml")
    need(workflow, "deploy/docker/Dockerfile", "release.yml")
    need(workflow, '--build-arg VERSION="${{ steps.meta.outputs.version }}"', "release.yml")
    need(workflow, '--build-arg GIT_COMMIT="${{ github.sha }}"', "release.yml")
    need(workflow, "linux/amd64,linux/arm64", "release.yml")
    need(workflow, "docker buildx build --push", "release.yml")
    need(workflow, '"${SERVER_IMAGE}:${GITHUB_REF_NAME}"', "release.yml")
    need(workflow, '"${SERVER_IMAGE}:latest"', "release.yml")
    need(workflow, '"${CONSOLE_IMAGE}:${GITHUB_REF_NAME}"', "release.yml")
    need(workflow, '"${CONSOLE_IMAGE}:latest"', "release.yml")
    need(workflow, "ghcr.io/direktorbani/datasafe-storage-server", "release.yml")
    need(workflow, "cosign sign --yes", "release.yml")
    need(workflow, "oci://ghcr.io/direktorbani/charts", "release.yml")
    need(workflow, "helm push", "release.yml")
    need(workflow, "--provenance=mode=max", "release.yml")
    need(workflow, "--sbom=true", "release.yml")
    script = (ROOT / "deploy/docker/link-version.sh").read_text(encoding="utf-8")
    need(dockerfile, "ARG VERSION=dev", "Dockerfile")
    need(dockerfile, "ARG GIT_COMMIT=unknown", "Dockerfile")
    need(dockerfile, 'VERSION="$VERSION" GIT_COMMIT="$GIT_COMMIT" ./deploy/docker/link-version.sh', "Dockerfile")
    need(dockerfile, "/usr/lib/datasafe/datasafe-kafka-bridge", "Dockerfile")
    need(script, "-tags datasafe_sinkexec", "link-version.sh")
    need(script, "internal/buildinfo.Version=${ver}", "link-version.sh")
    need(script, "internal/buildinfo.Commit=${commit}", "link-version.sh")
    need(script, "/out/datasafe-kafka-bridge", "link-version.sh")

    if errors:
        for err in errors:
            print(f"release workflow: {err}", file=sys.stderr)
        sys.exit(1)
    print("release workflow publishes the tagged image with the kafka bridge")


if __name__ == "__main__":
    main()
