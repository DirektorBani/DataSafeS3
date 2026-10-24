#!/usr/bin/env python3
"""Offline checks for the GitVerse publish size gate, prune selection, and dry-run."""

from __future__ import annotations

import os
import stat
import subprocess
import tempfile
import unittest
from pathlib import Path

import gitverse_publish as gv

ROOT = Path(__file__).resolve().parents[1]
TOKEN = "SUPER_SECRET_GITVERSE_TOKEN_VALUE"


def index_with_shared_layer() -> tuple[dict, dict[str, dict]]:
    shared = {"digest": "sha256:shared", "size": 100}
    amd64 = {
        "config": {"digest": "sha256:cfg-amd64", "size": 2},
        "layers": [shared, {"digest": "sha256:amd64", "size": 50}],
    }
    arm64 = {
        "config": {"digest": "sha256:cfg-arm64", "size": 2},
        "layers": [shared, {"digest": "sha256:arm64", "size": 70}],
    }
    attestation = {
        "config": {"digest": "sha256:cfg-att", "size": 1},
        "layers": [{"digest": "sha256:att", "size": 9}],
    }
    index = {
        "manifests": [
            {
                "digest": "sha256:m-amd64",
                "platform": {"os": "linux", "architecture": "amd64"},
            },
            {
                "digest": "sha256:m-arm64",
                "platform": {"os": "linux", "architecture": "arm64"},
            },
            {
                "digest": "sha256:m-att",
                "platform": {"os": "unknown", "architecture": "unknown"},
            },
        ]
    }
    store = {
        "sha256:m-amd64": amd64,
        "sha256:m-arm64": arm64,
        "sha256:m-att": attestation,
    }
    return index, store


class SizeTests(unittest.TestCase):
    def test_shared_layer_counted_once_and_attestations_excluded(self) -> None:
        index, store = index_with_shared_layer()
        image = gv.ImageSize(name="server")
        gv.add_manifest(index, store.get, image)
        self.assertEqual(image.platform_bytes("linux/amd64"), 152)
        self.assertEqual(image.platform_bytes("linux/arm64"), 172)
        report = gv.Report([image], [], "all", 500 * 1024 * 1024, 2)
        self.assertEqual(report.image_bytes(["linux/amd64", "linux/arm64"]), 224)
        self.assertEqual(report.attestation_bytes(), 10)
        self.assertNotIn("sha256:att", report.images[0].platforms["linux/amd64"])
        text = gv.render_markdown(report, True, "fits")
        self.assertIn(gv.fmt_bytes(224), text)
        self.assertNotIn(gv.fmt_bytes(324), text)

    def test_cross_image_dedup(self) -> None:
        shared = "sha256:base"
        server = gv.ImageSize(
            name="server",
            blobs={shared: 100, "sha256:server": 10},
            platforms={"linux/amd64": {shared, "sha256:server"}},
        )
        console = gv.ImageSize(
            name="console",
            blobs={shared: 100, "sha256:console": 5},
            platforms={"linux/amd64": {shared, "sha256:console"}},
        )
        report = gv.Report([server, console], [], "amd64", 500 * 1024 * 1024, 2)
        self.assertEqual(report.selected_bytes(), 115)

    def test_budget_suggests_amd64_when_only_that_fits(self) -> None:
        ok, message = gv.budget_decision(
            selected=600,
            amd64_only=300,
            limit=500,
            architectures="all",
        )
        self.assertFalse(ok)
        self.assertIn("GITVERSE_ARCHITECTURES=amd64", message)
        self.assertIn("Nothing was uploaded", message)

    def test_budget_rejects_when_amd64_also_exceeds(self) -> None:
        ok, message = gv.budget_decision(
            selected=800,
            amd64_only=700,
            limit=500,
            architectures="amd64",
        )
        self.assertFalse(ok)
        self.assertIn("also exceeds", message)

    def test_classify_ignores_bundles(self) -> None:
        self.assertEqual(gv.classify_filename("datasafes3_1.7.0_amd64.deb"), ("deb", "amd64"))
        self.assertEqual(gv.classify_filename("datasafes3-1.7.0-1.aarch64.rpm"), ("rpm", "arm64"))
        self.assertEqual(gv.classify_filename("datasafes3-1.7.0.tgz"), ("chart", "any"))
        self.assertIsNone(gv.classify_filename("datasafes3_1.7.0_amd64.deb.bundle"))
        self.assertIsNone(gv.classify_filename("SHA256SUMS"))

    def test_amd64_selection_skips_arm_packages(self) -> None:
        arm = gv.FileItem("deb", "arm64", Path("arm.deb"), 10)
        amd = gv.FileItem("deb", "amd64", Path("amd.deb"), 20)
        chart = gv.FileItem("chart", "any", Path("chart.tgz"), 3)
        report = gv.Report([], [arm, amd, chart], "amd64", 500, 2)
        self.assertEqual(report.file_bytes("amd64"), 23)
        self.assertEqual(report.file_bytes("all"), 33)


class PruneTests(unittest.TestCase):
    def test_keeps_latest_and_newest_n(self) -> None:
        versions = [
            {"id": 1, "name": "1.5.0", "created_at": "2024-01-01T00:00:00Z"},
            {"id": 2, "name": "1.6.0", "created_at": "2024-06-01T00:00:00Z"},
            {"id": 3, "name": "v1.7.0", "created_at": "2024-10-01T00:00:00Z"},
            {"id": 4, "name": "latest", "created_at": "2024-10-02T00:00:00Z"},
            {"id": 5, "name": "Latest", "created_at": "2024-10-03T00:00:00Z"},
        ]
        doomed = gv.select_deletions(versions, 2)
        self.assertEqual([item["name"] for item in doomed], ["1.5.0"])

    def test_keep_zero_deletes_every_numbered_version(self) -> None:
        versions = [
            {"id": 1, "name": "1.0.0", "created_at": "2024-01-01T00:00:00Z"},
            {"id": 2, "name": "latest", "created_at": "2024-02-01T00:00:00Z"},
        ]
        doomed = gv.select_deletions(versions, 0)
        self.assertEqual([item["id"] for item in doomed], [1])

    def test_versions_url_includes_packages_segment(self) -> None:
        url = gv.versions_url("https://api.gitverse.ru", "DirektorBani", "container", "datasafe-storage-server")
        self.assertEqual(
            url,
            "https://api.gitverse.ru/users/DirektorBani/packages/container/datasafe-storage-server/versions",
        )


class ManifestTests(unittest.TestCase):
    def test_parse_image_ref(self) -> None:
        self.assertEqual(
            gv.parse_image_ref("ghcr.io/direktorbani/datasafe-storage-server:v1.7.0"),
            ("ghcr.io", "direktorbani/datasafe-storage-server", "v1.7.0"),
        )
        self.assertEqual(
            gv.parse_image_ref("ghcr.io/direktorbani/datasafe-console@sha256:abc"),
            ("ghcr.io", "direktorbani/datasafe-console", "sha256:abc"),
        )

    def test_verify_multi_arch_index(self) -> None:
        index, _store = index_with_shared_layer()
        gv.verify_manifest(index, "all")
        with self.assertRaises(SystemExit):
            gv.verify_manifest(index, "amd64")

    def test_verify_single_arch_image(self) -> None:
        gv.verify_manifest({"config": {}, "layers": []}, "amd64")
        with self.assertRaises(SystemExit):
            gv.verify_manifest({"config": {}, "layers": []}, "all")

    def test_www_authenticate_parser(self) -> None:
        header = 'Bearer realm="https://ghcr.io/token",service="ghcr.io",scope="repository:direktorbani/datasafe-storage-server:pull"'
        parsed = gv.parse_www_authenticate(header)
        self.assertEqual(parsed["realm"], "https://ghcr.io/token")
        self.assertEqual(parsed["service"], "ghcr.io")


class DryRunTests(unittest.TestCase):
    def run_script(self, architectures: str) -> subprocess.CompletedProcess[str]:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            packages = root / "packages"
            chart = root / "chart"
            packages.mkdir()
            chart.mkdir()
            (chart / "Chart.yaml").write_text("version: 1.7.0\n", encoding="utf-8")
            for name in (
                "datasafes3_1.7.0_amd64.deb",
                "datasafes3_1.7.0_arm64.deb",
                "datasafes3-1.7.0-1.x86_64.rpm",
                "datasafes3-1.7.0-1.aarch64.rpm",
                "datasafes3-1.7.0.tgz",
                "datasafes3_1.7.0_amd64.deb.bundle",
            ):
                (packages / name).write_bytes(b"x")
            env = os.environ.copy()
            env.update(
                {
                    "TAG": "v1.7.0",
                    "GITVERSE_USER": "DirektorBani",
                    "GITVERSE_TOKEN": TOKEN,
                    "PKG_DIR": str(packages),
                    "CHART_SRC": str(chart),
                    "ARCHITECTURES": architectures,
                    "DELETE_OLD": "false",
                    "KEEP": "2",
                    "LIMIT_MB": "500",
                    "GITVERSE_DRY_RUN": "1",
                    "GITHUB_TOKEN": "SHOULD_NOT_PRINT_GITHUB_TOKEN",
                    "GITHUB_ACTOR": "direktorbani",
                }
            )
            return subprocess.run(
                ["bash", str(ROOT / "scripts" / "gitverse-publish.sh")],
                check=False,
                capture_output=True,
                text=True,
                env=env,
            )

    def test_all_architectures_dry_run_copies_both_and_hides_the_token(self) -> None:
        result = self.run_script("all")
        self.assertEqual(result.returncode, 0, result.stderr + result.stdout)
        output = result.stdout + result.stderr
        self.assertNotIn(TOKEN, output)
        self.assertNotIn("SHOULD_NOT_PRINT_GITHUB_TOKEN", output)
        self.assertIn("crane index filter", output)
        self.assertIn("--platform linux/amd64", output)
        self.assertIn("--platform linux/arm64", output)
        self.assertIn("gitverse.ru/direktorbani/datasafe-storage-server:v1.7.0", output)
        self.assertIn("gitverse.ru/direktorbani/datasafe-console:latest", output)
        self.assertIn(
            "https://gitverse.ru/api/packages/DirektorBani/debian/pool/stable/main/upload",
            output,
        )
        self.assertIn("https://gitverse.ru/api/packages/DirektorBani/rpm/stable/upload", output)
        self.assertIn("https://gitverse.ru/api/packages/DirektorBani/helm/api/charts", output)
        self.assertIn("datasafes3_1.7.0_arm64.deb", output)
        self.assertNotIn(".bundle", output)
        self.assertIn("-X POST", output)
        self.assertIn("-X PUT", output)

    def test_amd64_dry_run_skips_arm_packages(self) -> None:
        result = self.run_script("amd64")
        self.assertEqual(result.returncode, 0, result.stderr + result.stdout)
        output = result.stdout + result.stderr
        self.assertNotIn(TOKEN, output)
        self.assertIn("crane copy", output)
        self.assertIn("--platform linux/amd64", output)
        self.assertNotIn("crane index filter", output)
        self.assertNotIn("datasafes3_1.7.0_arm64.deb", output)
        self.assertNotIn("aarch64.rpm", output)
        self.assertIn("datasafes3_1.7.0_amd64.deb", output)
        self.assertIn("x86_64.rpm", output)


class SourceTests(unittest.TestCase):
    def test_publish_script_does_not_echo_the_token_or_trace(self) -> None:
        text = (ROOT / "scripts" / "gitverse-publish.sh").read_text(encoding="utf-8")
        self.assertNotIn("echo \"$GITVERSE_TOKEN\"", text)
        self.assertNotIn("echo ${GITVERSE_TOKEN}", text)
        self.assertNotIn("set -x", text)
        mode = (ROOT / "scripts" / "gitverse-publish.sh").stat().st_mode
        self.assertTrue(mode & stat.S_IXUSR)

    def test_workflow_triggers_and_permissions(self) -> None:
        text = (ROOT / ".github" / "workflows" / "publish-gitverse.yml").read_text(encoding="utf-8")
        self.assertIn('workflows: ["release"]', text)
        self.assertIn("workflow_dispatch:", text)
        self.assertIn("contents: read", text)
        self.assertIn("packages: read", text)
        self.assertIn("GITVERSE_TOKEN is not set", text)
        self.assertNotIn("packages: write", text)
        release = (ROOT / ".github" / "workflows" / "release.yml").read_text(encoding="utf-8")
        self.assertIn("name: release", release)


if __name__ == "__main__":
    unittest.main()
