#!/usr/bin/env python3
"""Fail when the GitVerse release workflow would not publish the tagged images and packages.

Checks .gitverse/workflows/release.yaml, scripts/build-images.sh, the Dockerfile, link-version.sh,
the package script and nfpm config. The image must include the Kafka bridge binary and link
version/commit from VERSION and git. This script does not build or push.
"""

from __future__ import annotations

import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
RELEASE = ROOT / ".gitverse/workflows/release.yaml"
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

    w = "release.yaml"
    need(workflow, 'tags:\n      - "v*"', w)
    need(workflow, "bash scripts/check-release-pins.sh", w)
    need(workflow, "bash scripts/build-images.sh", w)
    need(workflow, "REGISTRY_TOKEN", w)
    need(workflow, "gitverse.ru/direktorbani", w)
    need(workflow, "scripts/gitverse-publish-packages.sh", w)
    need(workflow, "scripts/gitverse-release-assets.sh", w)
    need(workflow, "bash scripts/package-linux.sh", w)
    need(workflow, "bash scripts/sbom.sh", w)
    need(workflow, "https://go-mirror.gitverse.ru", w)
    need(workflow, "https://npm-mirror.gitverse.ru/", w)
    for foreign in ("ghcr.io", "github.com/", "docker.io", "registry-1.docker.io"):
        if foreign in workflow.replace("github.com/CycloneDX", "").replace("github.com/anchore", "").replace("github.com/google/go-containerregistry", ""):
            errors.append(f"{w}: depends on foreign host {foreign!r}")
    build = (ROOT / "scripts/build-images.sh").read_text(encoding="utf-8")
    need(build, 'PLATFORMS="linux/amd64,linux/arm64"', "build-images.sh")
    need(build, "tools/imgbuild", "build-images.sh")
    need(build, "datasafe-storage-server", "build-images.sh")
    need(build, "datasafe-console", "build-images.sh")
    script = (ROOT / "deploy/docker/link-version.sh").read_text(encoding="utf-8")
    need(dockerfile, "ARG VERSION=dev", "Dockerfile")
    need(dockerfile, "ARG GIT_COMMIT=unknown", "Dockerfile")
    need(dockerfile, 'VERSION="$VERSION" GIT_COMMIT="$GIT_COMMIT" ./deploy/docker/link-version.sh', "Dockerfile")
    need(dockerfile, "/usr/lib/datasafe/datasafe-kafka-bridge", "Dockerfile")
    need(script, "-tags datasafe_sinkexec", "link-version.sh")
    need(script, "internal/buildinfo.Version=${ver}", "link-version.sh")
    need(script, "internal/buildinfo.Commit=${commit}", "link-version.sh")
    need(script, "$out/datasafe-kafka-bridge", "link-version.sh")

    pkg = (ROOT / "scripts/package-linux.sh").read_text(encoding="utf-8")
    need(pkg, "NFPM_VERSION=", "package-linux.sh")
    need(pkg, "packaging/nfpm.yaml", "package-linux.sh")
    need(pkg, "-tags datasafe_sinkexec", "package-linux.sh")
    need(pkg, "internal/buildinfo.Version=${VERSION}", "package-linux.sh")
    need(pkg, "./cmd/storage-cli", "package-linux.sh")
    need(pkg, "./cmd/datasafe-kafka-bridge", "package-linux.sh")
    need(pkg, "build_arch amd64 amd64 x86_64", "package-linux.sh")
    need(pkg, "build_arch arm64 arm64 aarch64", "package-linux.sh")
    need(pkg, "SHA256SUMS.txt", "package-linux.sh")
    need(pkg, 'tr -d \'[:space:]\' < VERSION', "package-linux.sh")

    nfpm = (ROOT / "packaging/nfpm.yaml").read_text(encoding="utf-8")
    need(nfpm, "version: ${VERSION}", "nfpm.yaml")
    need(nfpm, "type: config|noreplace", "nfpm.yaml")
    need(nfpm, "/usr/bin/datasafe-storage-server", "nfpm.yaml")
    need(nfpm, "/usr/bin/storage-cli", "nfpm.yaml")
    need(nfpm, "/usr/lib/datasafe/datasafe-kafka-bridge", "nfpm.yaml")
    need(nfpm, "/etc/datasafes3/datasafes3.env", "nfpm.yaml")
    need(nfpm, "/var/lib/datasafes3", "nfpm.yaml")
    need(nfpm, "/usr/lib/systemd/system/datasafes3.service", "nfpm.yaml")

    postinstall = (ROOT / "packaging/scripts/postinstall.sh").read_text(encoding="utf-8")
    if "systemctl enable" in postinstall or "systemctl start" in postinstall:
        errors.append("postinstall.sh: must not enable or start the service")
    postremove = (ROOT / "packaging/scripts/postremove.sh").read_text(encoding="utf-8")
    if "rm " in postremove or "rmdir " in postremove:
        errors.append("postremove.sh: must not delete files")
    need(postremove, "/var/lib/datasafes3", "postremove.sh")

    unit = (ROOT / "packaging/datasafes3.service").read_text(encoding="utf-8")
    need(unit, "User=datasafes3", "datasafes3.service")
    need(unit, "Group=datasafes3", "datasafes3.service")
    need(unit, "ExecStart=/usr/bin/datasafe-storage-server", "datasafes3.service")
    need(unit, "EnvironmentFile=/etc/datasafes3/datasafes3.env", "datasafes3.service")

    ci = (ROOT / ".github/workflows/ci.yml").read_text(encoding="utf-8")
    need(ci, "linux-packages:", "ci.yml")
    need(ci, "bash scripts/package-linux.sh", "ci.yml")
    need(ci, "bash scripts/test-linux-packages.sh", "ci.yml")
    test_script = (ROOT / "scripts/test-linux-packages.sh").read_text(encoding="utf-8")
    need(test_script, "debian:12", "test-linux-packages.sh")
    need(test_script, "rockylinux:9", "test-linux-packages.sh")
    need(test_script, "linux/amd64", "test-linux-packages.sh")
    need(test_script, "linux/arm64", "test-linux-packages.sh")
    install_check = (ROOT / "scripts/package-install-check.sh").read_text(encoding="utf-8")
    need(install_check, "systemd-analyze verify", "package-install-check.sh")
    need(install_check, "/var/lib/datasafes3/metadata.db", "package-install-check.sh")
    need(install_check, "apt-get purge", "package-install-check.sh")
    need(install_check, "rpm -e datasafes3", "package-install-check.sh")

    if errors:
        for err in errors:
            print(f"release workflow: {err}", file=sys.stderr)
        sys.exit(1)
    print("release workflow publishes the tagged image, helm chart, and linux packages")


if __name__ == "__main__":
    main()
