#!/usr/bin/env python3
"""Validate and stage the prepared native Pi package, using only the stdlib.

The archive is producer-built. No package manager or lifecycle script runs here.
A content-addressed, fully verified directory is published only after extraction.
"""
import argparse
import hashlib
import json
from pathlib import Path, PurePosixPath
import re
import shutil
import stat
import sys
import time
import uuid
import zipfile

MATERIALIZER_SCHEMA = 2
STAGE_MAX_AGE = 3600
OS_JUNK = {".DS_Store", "Thumbs.db"}
OVERLAY_TREES = ("rsrc", "skills")
OVERLAY_FILES = ("runtime.json", "dashboard-config-manifest.json", "bin/ws-mcp-launcher.py")


class StalePayload(ValueError):
    """A valid archive no longer matches its producer source inputs."""


def overlaid(name):
    return name in OVERLAY_FILES or any(name == root or name.startswith(root + "/") for root in OVERLAY_TREES)


def ignored_path(name):
    # One policy for source scanning, producer copying and owner overlays.
    return any(part in OS_JUNK or part == "__pycache__" or part.endswith((".pyc", ".pyo", ".swp", "~"))
               for part in PurePosixPath(name).parts)


def drift_input(name):
    if ignored_path(name):
        return False
    # The consumer is outside the archive. Its recorded hash is provenance,
    # not native-closure freshness; consumer fixes need no payload regeneration.
    prefix = "agents-plugin-pi/"
    return name.startswith(prefix) and not overlaid(name[len(prefix):])


def digest(data):
    return hashlib.sha256(data).hexdigest()


def tree(root, ignore_junk=False, allow_runtime=False, expected_dirs=None):
    files = {}
    def walk(directory):
        for path in sorted(directory.iterdir()):
            rel = path.relative_to(root).as_posix()
            if ignore_junk and ignored_path(rel):
                continue
            if path.is_symlink():
                raise ValueError(f"payload contains a link: {path}")
            if allow_runtime and rel == ".runtime" and path.is_dir():
                continue
            if path.is_dir():
                if expected_dirs is not None and rel not in expected_dirs:
                    raise ValueError(f"payload contains an extra directory: {path}")
                walk(path)
            elif path.is_file():
                files[rel] = digest(path.read_bytes())
            else:
                raise ValueError(f"payload contains a non-file: {path}")
    if root.is_symlink():
        raise ValueError(f"payload contains a link: {root}")
    if root.exists():
        walk(root)
    return files


def safe_path(name):
    p = PurePosixPath(name)
    if not name or p.is_absolute() or p.as_posix() != name or ".." in p.parts or "\\" in name or ":" in name:
        raise ValueError(f"invalid payload path: {name!r}")
    forbidden = {".runtime", ".local-devenv-runtime", ".git", ".cache", "__pycache__", ".bin", ".package-lock.json"}
    if forbidden.intersection(p.parts) or name.endswith((".node", ".pyc")):
        raise ValueError(f"forbidden payload path: {name}")
    return p


def validate(plugin):
    native = plugin / "native-pi"
    manifest = json.loads((native / "manifest.json").read_text())
    if manifest.get("schema_version") != 1:
        raise ValueError("unsupported native Pi payload manifest")
    archive = native / "payload.zip"
    if digest(archive.read_bytes()) != manifest["archive_sha256"]:
        raise ValueError("native Pi payload archive checksum mismatch")
    files = manifest["files"]
    required = {"package.json", "package-lock.json", "production-lock.json", "dist/index.js", "dist/eff_large_wordlist.txt", "src/index.ts", "src/web-search-helper.mjs", "scripts/bundle.mjs", "runtime.json", "config-manifest.json", "dashboard-config-manifest.json", "bin/ws-mcp-launcher.py", "rsrc/manifest.json", "pi-lead-guide.md", "lead-compact-guide.md", "execute-worker-guide.md", "explore-guide.md", "node_modules/pi-web-access/index.ts", "node_modules/pi-web-access/LICENSE", "node_modules/jiti/package.json"}
    if not required.issubset(files) or not any(p.startswith("skills/") and p.endswith("SKILL.md") for p in files):
        raise ValueError("native Pi payload is incomplete")
    with zipfile.ZipFile(archive) as z:
        if len(z.infolist()) != len(files) or set(z.namelist()) != set(files):
            raise ValueError("native Pi payload inventory mismatch")
        for info in z.infolist():
            safe_path(info.filename)
            if stat.S_ISLNK(info.external_attr >> 16) or info.is_dir():
                raise ValueError(f"non-file payload member: {info.filename}")
            if digest(z.read(info)) != files[info.filename]:
                raise ValueError(f"native Pi payload file checksum mismatch: {info.filename}")
            if info.filename.endswith("package.json"):
                pkg = json.loads(z.read(info))
                if pkg.get("name", "").startswith(("@earendil-works/pi-", "@mariozechner/pi-")):
                    raise ValueError(f"payload contains host Pi package: {info.filename}")
        pkg = json.loads(z.read("package.json"))
        if pkg["dependencies"] != manifest["dependencies"]:
            raise ValueError("native Pi dependency declaration mismatch")
        lock = json.loads(z.read("production-lock.json"))
        for rel, record in lock["packages"].items():
            if not rel:
                continue
            path = rel + "/package.json"
            if path not in files or json.loads(z.read(path))["version"] != record["version"]:
                raise ValueError(f"native Pi production dependency missing or wrong version: {rel}")
        web = json.loads(z.read("node_modules/pi-web-access/package.json"))
        if web.get("name") != "pi-web-access" or web["version"] != pkg["dependencies"]["pi-web-access"]:
            raise ValueError("native Pi search package mismatch")
        inputs = [p for p in files if p.startswith("src/") and p.endswith((".ts", ".mjs"))]
        inputs += ["src/eff_large_wordlist.txt", "package-lock.json", "scripts/bundle.mjs"]
        current = digest("".join(f"{p}\n{files[p]}\n" for p in sorted(inputs)).encode())
        if z.read("dist/index.js").splitlines()[0] != f"// ws-pi-bundle-inputs: sha256-{current}".encode():
            raise ValueError("native Pi bundle is stale")
    return manifest


def check_source(plugin, repo):
    manifest = validate(plugin)
    hint = f"regenerate in {repo / 'agents-plugin-pi'}: npm run bundle && python3 scripts/prepare-native-payload.py"
    for rel, expected in manifest["inputs"].items():
        if not drift_input(rel):
            continue
        path = repo / rel
        if not path.is_file() or digest(path.read_bytes()) != expected:
            raise StalePayload(f"native Pi distribution is stale: {rel}; {hint}")
    # piBundleInputHash covers src TS/MJS, the wordlist, lock and bundle script.
    # Also gate added distribution/build inputs, but never owner overlays/junk.
    # bin contains only the owner-overlaid launcher, so it has no added inputs.
    roots = ["agents-plugin-pi/src", "agents-plugin-pi/scripts", "agents-plugin-pi/dist"]
    for root in roots:
        for rel in tree(repo / root, ignore_junk=True):
            full = f"{root}/{rel}"
            if drift_input(full) and full not in manifest["inputs"]:
                raise StalePayload(f"native Pi distribution input added: {full}; {hint}")
    return manifest


def cleanup_stages(cache):
    # Only our UUID stages and the previous mkdtemp naming scheme are eligible.
    # One hour exceeds the 300s extraction budget, protecting active producers.
    pattern = re.compile(r"^\.native-pi-(?:stage-[0-9a-f]{32}|[a-z0-9_]{8})$")
    cutoff = time.time() - STAGE_MAX_AGE
    for path in cache.iterdir():
        if pattern.fullmatch(path.name) and not path.is_symlink() and path.is_dir():
            try:
                if path.stat().st_mtime < cutoff:
                    shutil.rmtree(path)
            except OSError:
                pass  # Removed concurrently or temporarily locked; retry later.


def cache_matches(dest, expected):
    try:
        directories = {str(parent) for name in expected for parent in PurePosixPath(name).parents if str(parent) != "."}
        return dest.is_dir() and tree(dest, allow_runtime=True, expected_dirs=directories) == expected
    except (OSError, ValueError):
        return False


def materialize(plugin, cache, manifest=None):
    cache.mkdir(parents=True, exist_ok=True)
    cleanup_stages(cache)
    manifest = manifest if manifest is not None else validate(plugin)
    # Replace whole owner trees, including deletions, rather than merging stale
    # archive resources into a package's current curated inventory.
    overlay = {}
    for root in OVERLAY_TREES:
        overlay.update({f"{root}/{p}": h for p, h in tree(plugin / root, ignore_junk=True).items()})
    for name in OVERLAY_FILES:
        overlay[name] = digest((plugin / name).read_bytes())
    identity = json.loads((plugin / "runtime.json").read_text())["plugin"]
    if identity not in ("ws", "wsflow"):
        raise ValueError("unsupported native Pi workflow owner")
    key = digest(json.dumps([MATERIALIZER_SCHEMA, manifest["archive_sha256"], identity, overlay], sort_keys=True).encode())
    dest = cache / key
    owner_bytes = (json.dumps({"plugin": identity}) + "\n").encode("utf-8")
    expected = {p: h for p, h in manifest["files"].items() if not overlaid(p)}
    expected.update(overlay)
    expected["workflow-owner.json"] = digest(owner_bytes)
    if cache_matches(dest, expected):
        return dest
    stage = cache / (".native-pi-stage-" + uuid.uuid4().hex)
    stage.mkdir(mode=0o700)
    ready, previous = stage / "ready", stage / "previous"
    ready.mkdir()
    try:
        with zipfile.ZipFile(plugin / "native-pi/payload.zip") as z:
            for info in z.infolist():
                if overlaid(info.filename):
                    continue
                path = ready / info.filename
                path.parent.mkdir(parents=True, exist_ok=True)
                path.write_bytes(z.read(info))
        for rel in overlay:
            path = ready / rel
            path.parent.mkdir(parents=True, exist_ok=True)
            shutil.copyfile(plugin / rel, path)
        (ready / "workflow-owner.json").write_bytes(owner_bytes)
        if tree(ready) != expected:
            raise ValueError("native Pi staging verification failed")
        if cache_matches(dest, expected):
            return dest  # Another materializer published the same valid entry.
        if dest.exists() or dest.is_symlink():
            try:
                dest.rename(previous)
            except FileNotFoundError:
                pass  # A concurrent repair already moved the corrupt entry.
        try:
            ready.rename(dest)
        except OSError:
            if not cache_matches(dest, expected):
                if previous.exists() and not dest.exists():
                    previous.rename(dest)
                raise
        return dest
    finally:
        if previous.exists() and not dest.exists():
            print(f"native Pi cache rollback needs recovery from {previous}", file=sys.stderr)
        else:
            try:
                shutil.rmtree(stage)
            except OSError:
                # A published entry remains usable even if antivirus/open handles
                # delay cleanup. cleanup_stages retries abandoned stages later.
                pass


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--plugin", type=Path, default=Path(__file__).resolve().parent.parent)
    parser.add_argument("--check-source", type=Path)
    parser.add_argument("--materialize", type=Path)
    parser.add_argument("--prefer-source", action="store_true", help="Select a sibling only when this valid payload is stale against its source")
    parser.add_argument("--allow-stale-source", action="store_true", help="Installer warning instead of failure for source drift only")
    args = parser.parse_args()
    plugin = args.plugin.resolve()
    try:
        if args.check_source:
            check_source(plugin, args.check_source.resolve())
        elif args.materialize:
            manifest = None
            source = plugin.parent / "agents-plugin-pi"
            if args.prefer_source and source.is_dir():
                try:
                    manifest = check_source(plugin, plugin.parent)
                except StalePayload as exc:
                    print(f"native Pi payload: using repository sibling: {exc}", file=sys.stderr)
                    print(source)
                    return
            print(materialize(plugin, args.materialize.resolve(), manifest))
        else:
            validate(plugin)
    except StalePayload as exc:
        if args.check_source and args.allow_stale_source:
            print(f"WARNING: {exc}; continuing with the validated prepared payload", file=sys.stderr)
        else:
            parser.exit(2, f"native Pi payload unavailable: {exc}\n")
    except (OSError, ValueError, KeyError, zipfile.BadZipFile) as exc:
        parser.exit(1, f"native Pi payload unavailable: {exc}\n")


if __name__ == "__main__":
    main()
