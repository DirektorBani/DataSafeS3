#!/usr/bin/env python3
"""Measure a DataSafeS3 release and prune old GitVerse package versions.

The size walk sums unique OCI config and layer blobs. BuildKit attestation
manifests (platform unknown/unknown) are reported and are not part of the
upload: the publish script copies only linux/amd64 and linux/arm64.

GitVerse documents a 500 MB account cap:
https://gitverse.ru/docs/limits-lfs-cicd/limits-packages

Package cleanup uses the Public API, not the GitVerse CI action
``actions/delete-old-packages`` (that action runs on GitVerse, not GitHub):

- GET    /users/{username}/packages/{type}/{name}/versions
- DELETE /users/{username}/packages/{type}/{name}/versions/{id}

https://gitverse.ru/docs/developers/public-api/packages/
The curl samples on those pages omit the ``/packages/`` segment; the path
template includes it.
"""

from __future__ import annotations

import argparse
import base64
import json
import os
import re
import sys
import urllib.error
import urllib.parse
import urllib.request
from dataclasses import dataclass, field
from pathlib import Path

ACCEPT = ", ".join(
    [
        "application/vnd.oci.image.index.v1+json",
        "application/vnd.docker.distribution.manifest.list.v2+json",
        "application/vnd.oci.image.manifest.v1+json",
        "application/vnd.docker.distribution.manifest.v2+json",
    ]
)
GITVERSE_ACCEPT = "application/vnd.gitverse.object+json;version=latest"
ALIASES = frozenset({"latest"})
API_BASE = "https://api.gitverse.ru"


@dataclass
class ImageSize:
    name: str
    blobs: dict[str, int] = field(default_factory=dict)
    platforms: dict[str, set[str]] = field(default_factory=dict)

    def platform_bytes(self, key: str) -> int:
        return sum(self.blobs[digest] for digest in self.platforms.get(key, ()))


@dataclass
class FileItem:
    kind: str
    arch: str
    path: Path
    size: int


@dataclass
class Report:
    images: list[ImageSize]
    files: list[FileItem]
    architectures: str
    limit_bytes: int
    keep: int

    def image_bytes(self, platforms: list[str]) -> int:
        blobs: dict[str, int] = {}
        digests: set[str] = set()
        for image in self.images:
            blobs.update(image.blobs)
            for key in platforms:
                digests.update(image.platforms.get(key, ()))
        return sum(blobs[digest] for digest in digests)

    def file_bytes(self, architectures: str) -> int:
        return sum(item.size for item in self.files if file_included(item, architectures))

    def selected_bytes(self, architectures: str | None = None) -> int:
        architectures = architectures or self.architectures
        return self.image_bytes(platform_keys(architectures)) + self.file_bytes(architectures)

    def attestation_bytes(self) -> int:
        blobs: dict[str, int] = {}
        digests: set[str] = set()
        for image in self.images:
            blobs.update(image.blobs)
            digests.update(image.platforms.get("attestation", ()))
        return sum(blobs[digest] for digest in digests)


def platform_keys(architectures: str) -> list[str]:
    if architectures == "amd64":
        return ["linux/amd64"]
    if architectures == "all":
        return ["linux/amd64", "linux/arm64"]
    raise ValueError(f"architectures must be all or amd64, got {architectures!r}")


def file_included(item: FileItem, architectures: str) -> bool:
    if item.arch == "any":
        return True
    if architectures == "all":
        return True
    return item.arch == "amd64"


def platform_key(descriptor: dict) -> str:
    platform = descriptor.get("platform") or {}
    os_name = platform.get("os") or "unknown"
    arch = platform.get("architecture") or "unknown"
    if os_name == "unknown" or arch == "unknown":
        return "attestation"
    return f"{os_name}/{arch}"


def is_index(manifest: dict) -> bool:
    return "manifests" in manifest and "layers" not in manifest


def add_image(manifest: dict, into: ImageSize, key: str) -> None:
    descriptors: list[dict] = []
    config = manifest.get("config")
    if isinstance(config, dict) and config.get("digest"):
        descriptors.append(config)
    descriptors.extend(manifest.get("layers") or [])
    bucket = into.platforms.setdefault(key, set())
    for descriptor in descriptors:
        digest = descriptor.get("digest")
        if not digest:
            continue
        into.blobs[digest] = int(descriptor.get("size") or 0)
        bucket.add(digest)


def add_manifest(manifest: dict, fetch, into: ImageSize) -> None:
    if is_index(manifest):
        for descriptor in manifest.get("manifests") or []:
            child = fetch(descriptor["digest"])
            if is_index(child):
                add_manifest(child, fetch, into)
            else:
                add_image(child, into, platform_key(descriptor))
        return
    add_image(manifest, into, "unknown")


def classify_filename(name: str) -> tuple[str, str] | None:
    if name.endswith("_amd64.deb"):
        return "deb", "amd64"
    if name.endswith("_arm64.deb"):
        return "deb", "arm64"
    if name.endswith(".x86_64.rpm"):
        return "rpm", "amd64"
    if name.endswith(".aarch64.rpm"):
        return "rpm", "arm64"
    if name.endswith(".tgz"):
        return "chart", "any"
    return None


def files_in(directory: Path) -> list[FileItem]:
    items: list[FileItem] = []
    for path in sorted(directory.iterdir()):
        if not path.is_file():
            continue
        classified = classify_filename(path.name)
        if classified is None:
            continue
        kind, arch = classified
        items.append(FileItem(kind, arch, path, path.stat().st_size))
    return items


def require_files(items: list[FileItem], architectures: str) -> None:
    wanted = [("deb", "amd64"), ("rpm", "amd64"), ("chart", "any")]
    if architectures == "all":
        wanted.extend([("deb", "arm64"), ("rpm", "arm64")])
    for kind, arch in wanted:
        matches = [item for item in items if item.kind == kind and item.arch == arch]
        if len(matches) != 1:
            names = ", ".join(item.path.name for item in matches) or "none"
            raise SystemExit(f"expected one {kind} ({arch}) package, found: {names}")


def require_platforms(images: list[ImageSize], architectures: str) -> None:
    for image in images:
        for key in platform_keys(architectures):
            if not image.platforms.get(key):
                found = ", ".join(sorted(image.platforms)) or "none"
                raise SystemExit(f"{image.name} has no {key} manifest (found: {found})")


def selected_image_bytes(image: ImageSize, keys: list[str]) -> int:
    digests: set[str] = set()
    for key in keys:
        digests.update(image.platforms.get(key, ()))
    return sum(image.blobs[digest] for digest in digests)


def fmt_bytes(size: int) -> str:
    return f"{size / 1024 / 1024:.2f} MiB ({size} B)"


def budget_decision(selected: int, amd64_only: int, limit: int, architectures: str) -> tuple[bool, str]:
    if selected <= limit:
        return True, (
            f"Selected set is {fmt_bytes(selected)}, within the limit of {fmt_bytes(limit)}. "
            "Upload will continue."
        )
    if architectures == "all" and amd64_only <= limit:
        return False, (
            f"Multi-arch set is {fmt_bytes(selected)}, above the limit of {fmt_bytes(limit)}. "
            f"amd64-only is {fmt_bytes(amd64_only)} and fits. "
            "Re-run workflow_dispatch with architectures=amd64, or set the repository variable "
            "GITVERSE_ARCHITECTURES=amd64 for runs that follow a release. Nothing was uploaded."
        )
    return False, (
        f"Selected set is {fmt_bytes(selected)}, above the limit of {fmt_bytes(limit)}. "
        f"amd64-only is {fmt_bytes(amd64_only)} and also exceeds the limit. Nothing was uploaded."
    )


def render_markdown(report: Report, ok: bool, decision: str) -> str:
    keys = platform_keys(report.architectures)
    lines = [
        "## GitVerse artifact sizes",
        "",
        (
            f"Architectures `{report.architectures}`, limit {fmt_bytes(report.limit_bytes)}. "
            "GitVerse caps every package of the account together at 500 Мбайт "
            "(https://gitverse.ru/docs/limits-lfs-cicd/limits-packages)."
        ),
        "",
        "| Artifact | linux/amd64 | linux/arm64 | Selected |",
        "| --- | ---: | ---: | ---: |",
    ]
    for image in report.images:
        selected = selected_image_bytes(image, keys)
        lines.append(
            "| {name} | {amd} | {arm} | {selected} |".format(
                name=image.name,
                amd=fmt_bytes(image.platform_bytes("linux/amd64")),
                arm=fmt_bytes(image.platform_bytes("linux/arm64")),
                selected=fmt_bytes(selected),
            )
        )
    for item in report.files:
        included = file_included(item, report.architectures)
        amd = fmt_bytes(item.size) if item.arch == "amd64" else ""
        arm = fmt_bytes(item.size) if item.arch == "arm64" else ""
        selected = fmt_bytes(item.size) if included else "skipped"
        lines.append(f"| {item.path.name} | {amd} | {arm} | {selected} |")
    lines.extend(
        [
            "",
            (
                f"Total unique blobs in the selected set: **{fmt_bytes(report.selected_bytes())}**. "
                "Identical layer digests are counted once across images. "
                "`:latest` is the same digest as the version tag and is not counted again."
            ),
            "",
            (
                f"GHCR attestation manifests are not copied "
                f"({fmt_bytes(report.attestation_bytes())})."
            ),
            "",
            (
                f"`keep={report.keep}` times this release is "
                f"{fmt_bytes(report.keep * report.selected_bytes())} if versions share no blobs. "
                "Packages already stored on the account count toward the same cap."
            ),
            "",
            decision,
            "",
        ]
    )
    if not ok:
        lines.append("Upload was not started.")
        lines.append("")
    return "\n".join(lines)


def write_summary(text: str, path: str) -> None:
    sys.stdout.write(text)
    if not text.endswith("\n"):
        sys.stdout.write("\n")
    if path:
        with open(path, "a", encoding="utf-8") as handle:
            handle.write(text)
            if not text.endswith("\n"):
                handle.write("\n")


def parse_image_ref(ref: str) -> tuple[str, str, str]:
    value = ref.strip()
    if "://" in value:
        value = value.split("://", 1)[1]
    if "@" in value:
        left, reference = value.rsplit("@", 1)
    else:
        left, reference = value, "latest"
        slash = left.split("/")
        if len(slash) < 2:
            raise ValueError(f"image ref needs a repository: {ref}")
        repo = "/".join(slash[1:])
        if ":" in repo:
            repo, reference = repo.rsplit(":", 1)
            left = slash[0] + "/" + repo
    host, repo = left.split("/", 1)
    if not host or not repo or not reference:
        raise ValueError(f"cannot parse image ref: {ref}")
    return host, repo, reference


def image_name(ref: str) -> str:
    _host, repo, _reference = parse_image_ref(ref)
    return repo.rsplit("/", 1)[-1]


_AUTH_PARAM = re.compile(r'(\w+)="([^"]*)"')


def parse_www_authenticate(header: str) -> dict[str, str]:
    return {key: value for key, value in _AUTH_PARAM.findall(header)}


class Registry:
    """Read OCI manifests. Tokens stay in memory and out of error text."""

    def __init__(self, username: str = "", password: str = "") -> None:
        self.username = username
        self.password = password
        self._bearers: dict[tuple[str, str], str] = {}

    def manifest(self, ref: str) -> dict:
        host, repo, reference = parse_image_ref(ref)
        return self._manifest(host, repo, reference)

    def _manifest(self, host: str, repo: str, reference: str) -> dict:
        quoted = urllib.parse.quote(reference, safe=":")
        url = f"https://{host}/v2/{repo}/manifests/{quoted}"
        scope = f"repository:{repo}:pull"
        return self._get_json(host, url, scope)

    def _get_json(self, host: str, url: str, scope: str) -> dict:
        token = self._bearers.get((host, scope))
        if token:
            return self._request(url, token)
        try:
            return self._request(url, None)
        except urllib.error.HTTPError as err:
            if err.code != 401:
                raise SystemExit(_http_error(url, err)) from None
            header = err.headers.get("WWW-Authenticate", "") if err.headers else ""
            token = self._bearer(host, scope, header)
            self._bearers[(host, scope)] = token
            try:
                return self._request(url, token)
            except urllib.error.HTTPError as retry_err:
                raise SystemExit(_http_error(url, retry_err)) from None

    def _bearer(self, host: str, scope: str, www: str) -> str:
        params = parse_www_authenticate(www)
        realm = params.get("realm") or f"https://{host}/token"
        query = urllib.parse.urlencode(
            {key: params[key] for key in ("service", "scope") if params.get(key)}
            or {"scope": scope}
        )
        joiner = "&" if "?" in realm else "?"
        request = urllib.request.Request(realm + joiner + query)
        if self.username and self.password:
            raw = f"{self.username}:{self.password}".encode()
            request.add_header("Authorization", "Basic " + base64.b64encode(raw).decode("ascii"))
        try:
            with urllib.request.urlopen(request, timeout=60) as response:
                payload = json.load(response)
        except urllib.error.HTTPError as err:
            raise SystemExit(_http_error(realm, err)) from None
        token = payload.get("token") or payload.get("access_token")
        if not token:
            raise SystemExit(f"registry token response from {realm} has no token")
        return str(token)

    def _request(self, url: str, token: str | None) -> dict:
        headers = {"Accept": ACCEPT}
        if token:
            headers["Authorization"] = f"Bearer {token}"
        request = urllib.request.Request(url, headers=headers)
        with urllib.request.urlopen(request, timeout=60) as response:
            payload = json.load(response)
        if not isinstance(payload, dict):
            raise SystemExit(f"manifest from {url} is not a JSON object")
        return payload


def _http_error(url: str, err: urllib.error.HTTPError) -> str:
    try:
        body = err.read().decode("utf-8", "replace")
    except Exception:
        body = ""
    body = body.replace("\n", " ")[:300]
    return f"HTTP {err.code} for {url}: {body}"


def measure_ref(registry: Registry, ref: str) -> ImageSize:
    image = ImageSize(name=image_name(ref))

    def fetch(digest: str) -> dict:
        host, repo, _reference = parse_image_ref(ref)
        return registry._manifest(host, repo, digest)

    add_manifest(registry.manifest(ref), fetch, image)
    return image


def select_deletions(versions: list[dict], keep_non_alias: int) -> list[dict]:
    """Return versions to delete. Alias tags such as ``latest`` are kept."""
    if keep_non_alias < 0:
        raise ValueError("keep_non_alias must be >= 0")
    numbered: list[dict] = []
    for version in versions:
        name = str(version.get("name") or "")
        if name.lower() in ALIASES:
            continue
        numbered.append(version)

    def sort_key(version: dict) -> tuple[str, int]:
        raw_id = version.get("id") or 0
        try:
            version_id = int(raw_id)
        except (TypeError, ValueError):
            version_id = 0
        return str(version.get("created_at") or ""), version_id

    numbered.sort(key=sort_key, reverse=True)
    return numbered[keep_non_alias:]


def versions_url(api_base: str, user: str, package_type: str, name: str) -> str:
    quoted_user = urllib.parse.quote(user, safe="")
    quoted_type = urllib.parse.quote(package_type, safe="")
    quoted_name = urllib.parse.quote(name, safe="")
    return (
        f"{api_base}/users/{quoted_user}/packages/{quoted_type}/{quoted_name}/versions"
    )


class GitVerseAPI:
    def __init__(self, token: str, user: str, api_base: str = API_BASE) -> None:
        if not token:
            raise SystemExit("GITVERSE_TOKEN is not set")
        self.token = token
        self.user = user
        self.api_base = api_base.rstrip("/")

    def list_versions(self, package_type: str, name: str) -> list[dict] | None:
        """Return versions, or None when the package does not exist yet."""
        found: list[dict] = []
        seen: set[int] = set()
        for page in range(1, 21):
            url = versions_url(self.api_base, self.user, package_type, name)
            url += "?" + urllib.parse.urlencode({"page": page, "per_page": 100})
            status, payload = self._json("GET", url)
            if status == 404:
                return None if page == 1 else found
            if status != 200:
                raise SystemExit(f"list {package_type}/{name} failed: HTTP {status}: {_clip(payload)}")
            if not isinstance(payload, list):
                raise SystemExit(
                    f"list {package_type}/{name} returned {type(payload).__name__}, expected a list"
                )
            fresh = []
            for version in payload:
                if not isinstance(version, dict) or "id" not in version:
                    raise SystemExit(f"list {package_type}/{name} returned a version without id")
                version_id = int(version["id"])
                if version_id in seen:
                    continue
                seen.add(version_id)
                fresh.append(version)
            if not fresh:
                break
            found.extend(fresh)
            if len(payload) < 100:
                break
        return found

    def delete_version(self, package_type: str, name: str, version_id: int) -> None:
        url = versions_url(self.api_base, self.user, package_type, name) + f"/{int(version_id)}"
        status, payload = self._json("DELETE", url)
        if status in (200, 202, 204, 404):
            return
        raise SystemExit(
            f"delete {package_type}/{name} version {version_id} failed: HTTP {status}: {_clip(payload)}"
        )

    def _json(self, method: str, url: str) -> tuple[int, object]:
        request = urllib.request.Request(
            url,
            method=method,
            headers={
                "Accept": GITVERSE_ACCEPT,
                "Authorization": f"Bearer {self.token}",
            },
        )
        try:
            with urllib.request.urlopen(request, timeout=60) as response:
                raw = response.read()
                status = response.status
        except urllib.error.HTTPError as err:
            raw = err.read()
            status = err.code
        if not raw:
            return status, None
        try:
            return status, json.loads(raw.decode("utf-8"))
        except json.JSONDecodeError:
            return status, raw.decode("utf-8", "replace")[:300]


def _clip(payload: object) -> str:
    text = payload if isinstance(payload, str) else json.dumps(payload, ensure_ascii=False)
    return text.replace("\n", " ")[:300]


def parse_package(value: str) -> tuple[str, str]:
    if "/" not in value:
        raise SystemExit(f"package must be type/name, got {value!r}")
    package_type, name = value.split("/", 1)
    if not package_type or not name or "/" in name:
        raise SystemExit(f"package must be type/name, got {value!r}")
    return package_type, name


def verify_manifest(manifest: dict, architectures: str) -> None:
    if architectures == "all":
        if not is_index(manifest):
            raise SystemExit("expected a multi-arch index, got a single image manifest")
        archs = {
            (descriptor.get("platform") or {}).get("architecture")
            for descriptor in manifest.get("manifests") or []
        }
        missing = [arch for arch in ("amd64", "arm64") if arch not in archs]
        if missing:
            raise SystemExit(f"index missing {missing}; found {sorted(archs)}")
        return
    if architectures != "amd64":
        raise SystemExit(f"architectures must be all or amd64, got {architectures!r}")
    if is_index(manifest):
        archs = {
            (descriptor.get("platform") or {}).get("architecture")
            for descriptor in manifest.get("manifests") or []
        }
        if "arm64" in archs:
            raise SystemExit(f"amd64-only publish still contains arm64: {sorted(archs)}")
        if "amd64" not in archs:
            raise SystemExit(f"amd64-only publish has no amd64 manifest: {sorted(archs)}")


def _load_manifest(path: str) -> dict:
    if path == "-":
        payload = json.load(sys.stdin)
    else:
        payload = json.loads(Path(path).read_text(encoding="utf-8"))
    if not isinstance(payload, dict):
        raise SystemExit("manifest is not a JSON object")
    return payload


def cmd_measure(args: argparse.Namespace) -> int:
    registry = Registry(
        username=os.environ.get("GH_USERNAME", ""),
        password=os.environ.get("GH_TOKEN", ""),
    )
    images = [measure_ref(registry, ref) for ref in args.image]
    files = files_in(Path(args.packages_dir))
    try:
        require_files(files, args.architectures)
        require_platforms(images, args.architectures)
    except SystemExit as exc:
        write_summary(f"## GitVerse size check failed\n\n{exc}\n", args.summary)
        raise
    limit = args.limit_mb * 1024 * 1024
    report = Report(images, files, args.architectures, limit, args.keep)
    selected = report.selected_bytes()
    amd64_only = report.selected_bytes("amd64")
    ok, decision = budget_decision(selected, amd64_only, limit, args.architectures)
    write_summary(render_markdown(report, ok, decision), args.summary)
    return 0 if ok else 2


def cmd_prune(args: argparse.Namespace) -> int:
    token = os.environ.get("GITVERSE_TOKEN", "")
    api = GitVerseAPI(token, args.user, args.api_base)
    for value in args.package:
        package_type, name = parse_package(value)
        versions = api.list_versions(package_type, name)
        if versions is None:
            print(f"prune {package_type}/{name}: package not found, nothing to delete")
            continue
        doomed = select_deletions(versions, args.keep)
        doomed_ids = {id(version) for version in doomed}
        kept = [version for version in versions if id(version) not in doomed_ids]
        print(
            f"prune {package_type}/{name}: keep {[version.get('name') for version in kept]}, "
            f"delete {[version.get('name') for version in doomed]}"
        )
        if args.dry_run:
            continue
        for version in doomed:
            api.delete_version(package_type, name, int(version["id"]))
            print(f"deleted {package_type}/{name} {version.get('name')} id {version['id']}")
    return 0


def cmd_verify(args: argparse.Namespace) -> int:
    verify_manifest(_load_manifest(args.manifest_file), args.architectures)
    print(f"manifest ok for {args.architectures}")
    return 0


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description=__doc__)
    sub = parser.add_subparsers(dest="cmd", required=True)

    measure = sub.add_parser("measure", help="sum blob and package bytes and enforce the cap")
    measure.add_argument("--image", action="append", required=True)
    measure.add_argument("--packages-dir", required=True)
    measure.add_argument("--architectures", required=True, choices=("all", "amd64"))
    measure.add_argument("--limit-mb", type=int, required=True)
    measure.add_argument("--keep", type=int, required=True)
    measure.add_argument("--summary", default="")
    measure.set_defaults(func=cmd_measure)

    prune = sub.add_parser("prune", help="delete GitVerse package versions beyond the newest N")
    prune.add_argument("--user", required=True)
    prune.add_argument("--keep", type=int, required=True)
    prune.add_argument("--package", action="append", required=True)
    prune.add_argument("--dry-run", action="store_true")
    prune.add_argument("--api-base", default=API_BASE)
    prune.set_defaults(func=cmd_prune)

    verify = sub.add_parser("verify", help="check a pushed manifest has the requested architectures")
    verify.add_argument("--manifest-file", required=True)
    verify.add_argument("--architectures", required=True, choices=("all", "amd64"))
    verify.set_defaults(func=cmd_verify)
    return parser


def main(argv: list[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    if args.cmd == "measure" and args.limit_mb < 1:
        raise SystemExit("--limit-mb must be >= 1")
    if args.cmd == "measure" and args.keep < 1:
        raise SystemExit("--keep must be >= 1")
    if args.cmd == "prune" and args.keep < 0:
        raise SystemExit("--keep must be >= 0")
    return int(args.func(args))


if __name__ == "__main__":
    sys.exit(main())
