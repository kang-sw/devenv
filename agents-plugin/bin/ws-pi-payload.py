#!/usr/bin/env python3
"""Validate and stage the prepared native Pi package, using only the stdlib.

The archive is producer-built. No package manager or lifecycle script runs here.
A content-addressed, fully verified directory is published only after extraction.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path, PurePosixPath
import shutil
import stat
import tempfile
import zipfile


def digest(data):
    return hashlib.sha256(data).hexdigest()


def tree(root):
    files = {}
    for path in sorted(root.rglob("*")):
        if path.is_symlink():
            raise ValueError(f"payload contains a link: {path}")
        if path.is_file():
            files[path.relative_to(root).as_posix()] = digest(path.read_bytes())
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
    for rel, expected in manifest["inputs"].items():
        path = repo / rel
        if not path.is_file() or digest(path.read_bytes()) != expected:
            raise ValueError(f"native Pi distribution is stale: {rel}")
    # Inventory changes (including newly added sources/skills) must invalidate it.
    roots = ["agents-plugin-pi/src", "agents-plugin-pi/scripts", "agents-plugin-pi/rsrc", "agents-plugin/skills", "agents-plugin-wsflow/skills"]
    for root in roots:
        for rel in tree(repo / root):
            full = f"{root}/{rel}"
            if full not in manifest["inputs"]:
                raise ValueError(f"native Pi distribution input added: {full}")
    return manifest


def materialize(plugin, cache):
    manifest = validate(plugin)
    # Runtime and skills follow the owner; native code/dependencies are identical.
    overlay = {}
    for root in ("rsrc", "skills"):
        overlay.update({f"{root}/{p}": h for p, h in tree(plugin / root).items()})
    for name in ("runtime.json", "dashboard-config-manifest.json", "bin/ws-mcp-launcher.py"):
        overlay[name] = digest((plugin / name).read_bytes())
    identity = json.loads((plugin / "runtime.json").read_text())["plugin"]
    if identity not in ("ws", "wsflow"):
        raise ValueError("unsupported native Pi workflow owner")
    key = digest(json.dumps([manifest["archive_sha256"], identity, overlay], sort_keys=True).encode())
    cache.mkdir(parents=True, exist_ok=True)
    dest = cache / key
    owner_bytes = (json.dumps({"plugin": identity}) + "\n").encode("utf-8")
    expected = {**manifest["files"], **overlay, "workflow-owner.json": digest(owner_bytes)}
    expected = {p: h for p, h in expected.items() if not p.startswith("skills/") or p in overlay}
    if dest.exists():
        if tree(dest) != expected:
            raise ValueError("staged native Pi payload is corrupt; remove this cache entry and retry: " + str(dest))
        return dest
    stage = Path(tempfile.mkdtemp(prefix=".native-pi-", dir=cache))
    try:
        with zipfile.ZipFile(plugin / "native-pi/payload.zip") as z:
            for info in z.infolist():
                path = stage / info.filename
                path.parent.mkdir(parents=True, exist_ok=True)
                path.write_bytes(z.read(info))
        for rel in overlay:
            path = stage / rel
            path.parent.mkdir(parents=True, exist_ok=True)
            shutil.copyfile(plugin / rel, path)
        # Remove canonical skills absent from an owning package's curated set.
        for path in (stage / "skills").rglob("*"):
            if path.is_file() and path.relative_to(stage).as_posix() not in overlay:
                path.unlink()
        expected = {p: h for p, h in expected.items() if not p.startswith("skills/") or p in overlay}
        (stage / "workflow-owner.json").write_bytes(owner_bytes)
        if tree(stage) != expected:
            raise ValueError("native Pi staging verification failed")
        try:
            stage.rename(dest)
        except OSError:
            if not dest.exists() or tree(dest) != expected:
                raise
        return dest
    finally:
        if stage.exists():
            shutil.rmtree(stage)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--plugin", type=Path, default=Path(__file__).resolve().parent.parent)
    parser.add_argument("--check-source", type=Path)
    parser.add_argument("--materialize", type=Path)
    args = parser.parse_args()
    plugin = args.plugin.resolve()
    try:
        if args.check_source:
            check_source(plugin, args.check_source.resolve())
        elif args.materialize:
            print(materialize(plugin, args.materialize.resolve()))
        else:
            validate(plugin)
    except (OSError, ValueError, KeyError, zipfile.BadZipFile) as exc:
        parser.exit(1, f"native Pi payload unavailable: {exc}\n")


if __name__ == "__main__":
    main()
