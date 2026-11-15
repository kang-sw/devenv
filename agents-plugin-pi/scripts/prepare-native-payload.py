#!/usr/bin/env python3
"""Producer-only: build a clean production closure and commit-ready native archives.

Run after npm run bundle. Consumers and release checks use ws-pi-payload.py,
which requires neither Node/npm nor network. Versions come only from the Pi
package manifest and its lock; development/peer packages never enter the recipe.
"""
import argparse
import importlib.util
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import zipfile

sys.dont_write_bytecode = True
REPO = Path(__file__).resolve().parents[2]
SPEC = importlib.util.spec_from_file_location("payload", REPO / "agents-plugin/bin/ws-pi-payload.py")
payload = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(payload)


def production_recipe(pkg, lock):
    records = lock["packages"]
    selected = {}
    pending = [("", name) for name in pkg["dependencies"]]
    while pending:
        parent, name = pending.pop()
        scope = parent
        while True:
            candidate = (scope + "/" if scope else "") + "node_modules/" + name
            if candidate in records:
                break
            if not scope:
                raise ValueError(f"dependency absent from canonical lock: {parent}: {name}")
            scope = scope.rsplit("/node_modules/", 1)[0] if "/node_modules/" in scope else ""
        if candidate in selected:
            continue
        record = dict(records[candidate])
        for field in ("inBundle", "dev", "devOptional"):
            record.pop(field, None)
        if record.get("link") or not record.get("resolved", "").startswith("https://registry.npmjs.org/"):
            raise ValueError(f"non-registry dependency: {candidate}")
        if name.startswith(("@earendil-works/pi-", "@mariozechner/pi-")):
            raise ValueError(f"production graph contains host Pi package: {name}")
        selected[candidate] = record
        pending.extend((candidate, dep) for dep in {**record.get("dependencies", {}), **record.get("optionalDependencies", {})})
    manifest = {"name": "ws-native-pi-production", "version": "1.0.0", "private": True, "dependencies": pkg["dependencies"]}
    return manifest, {"name": manifest["name"], "version": manifest["version"], "lockfileVersion": 3, "packages": {"": manifest, **dict(sorted(selected.items()))}}


def check_bundle_paths(pi, repo):
    bundle = (pi / "dist/index.js").read_bytes()
    # esbuild labels must not capture a producer checkout/home or a temporary
    # dependency installation. Check before any download or artifact replacement.
    for path in (repo.resolve(), Path.home().resolve(), (pi / "node_modules").resolve()):
        if str(path).encode() in bundle:
            raise ValueError(f"native Pi bundle contains a producer path: {path}; run npm run bundle")


def generate(repo, offline=False, npm_cache=None):
    repo = repo.resolve()
    pi = repo / "agents-plugin-pi"
    check_bundle_paths(pi, repo)
    pkg = json.loads((pi / "package.json").read_text())
    recipe, lock = production_recipe(pkg, json.loads((pi / "package-lock.json").read_text()))
    with tempfile.TemporaryDirectory(prefix="ws-native-pi-") as tmp:
        temp = Path(tmp).resolve()
        deps = temp / "dependencies"
        deps.mkdir()
        (deps / "package.json").write_text(json.dumps(recipe))
        (deps / "package-lock.json").write_text(json.dumps(lock))
        env = dict(os.environ)
        for name in list(env):
            if name.lower().startswith("npm_config_") or name in ("NODE_OPTIONS", "NODE_PATH"):
                env.pop(name)
        for name, folder in (("HOME", "home"), ("USERPROFILE", "home"), ("WS_CACHE_HOME", "cache"), ("WS_CONFIG_HOME", "config"), ("npm_config_cache", "npm-cache")):
            path = temp / folder
            path.mkdir(exist_ok=True)
            env[name] = str(path)
        if npm_cache is not None:
            env["npm_config_cache"] = str(npm_cache.resolve())
        env["npm_config_userconfig"] = os.devnull
        flags = ["--ignore-scripts", "--legacy-peer-deps", "--bin-links=false", "--no-audit", "--no-fund"] + (["--offline"] if offline else [])
        # Normalize npm's virtual-root bookkeeping in the real cwd (not an
        # aliased /tmp --prefix). Reject any resulting version/graph drift.
        subprocess.run(["npm", "install", "--package-lock-only", *flags], cwd=deps, env=env, check=True)
        normalized = json.loads((deps / "package-lock.json").read_text())
        if {p: v.get("version") for p, v in normalized["packages"].items() if p} != {p: v.get("version") for p, v in lock["packages"].items() if p}:
            raise ValueError("npm changed the canonical production dependency graph")
        subprocess.run(["npm", "ci", "--omit=dev", *flags], cwd=deps, env=env, check=True)
        stage = temp / "package"
        stage.mkdir()
        inputs = {}
        def copy(source, destination):
            if source.is_dir():
                for rel in payload.tree(source):
                    copy(source / rel, destination / rel)
            else:
                destination.parent.mkdir(parents=True, exist_ok=True)
                shutil.copyfile(source, destination)
                inputs[source.relative_to(repo).as_posix()] = payload.digest(source.read_bytes())
        for name in ("src", "dist", "scripts", "bin", "rsrc", "package.json", "package-lock.json", "runtime.json", "config-manifest.json", "dashboard-config-manifest.json", "pi-lead-guide.md", "lead-compact-guide.md", "execute-worker-guide.md", "explore-guide.md"):
            copy(pi / name, stage / name)
        copy(repo / "agents-plugin/skills", stage / "skills")
        for rel in payload.tree(repo / "agents-plugin-wsflow/skills"):
            inputs["agents-plugin-wsflow/skills/" + rel] = payload.digest((repo / "agents-plugin-wsflow/skills" / rel).read_bytes())
        for rel in ("agents-plugin/bin/ws-pi-payload.py",):
            inputs[rel] = payload.digest((repo / rel).read_bytes())
        shutil.copytree(deps / "node_modules", stage / "node_modules")
        (stage / "node_modules/.package-lock.json").unlink(missing_ok=True)
        (stage / "production-lock.json").write_text(json.dumps(normalized, sort_keys=True, indent=2) + "\n")
        files = payload.tree(stage)
        native = temp / "native-pi"
        native.mkdir()
        with zipfile.ZipFile(native / "payload.zip", "w", compression=zipfile.ZIP_DEFLATED, compresslevel=9) as z:
            for rel in files:
                payload.safe_path(rel)
                info = zipfile.ZipInfo(rel, (2020, 1, 1, 0, 0, 0))
                info.compress_type = zipfile.ZIP_DEFLATED
                info.external_attr = 0o100644 << 16
                z.writestr(info, (stage / rel).read_bytes())
        manifest = {"schema_version": 1, "archive_sha256": payload.digest((native / "payload.zip").read_bytes()), "dependencies": pkg["dependencies"], "inputs": dict(sorted(inputs.items())), "files": files}
        (native / "manifest.json").write_text(json.dumps(manifest, sort_keys=True, indent=2) + "\n")
        fixture = temp / "fixture"
        fixture.mkdir()
        shutil.copytree(native, fixture / "native-pi")
        payload.validate(fixture)
        # Validate everything before replacing either previously usable artifact.
        for product in ("agents-plugin", "agents-plugin-wsflow"):
            dest = repo / product / "native-pi"
            ready = repo / product / ".native-pi-prepared"
            if ready.exists():
                shutil.rmtree(ready)
            shutil.copytree(native, ready)
            old = repo / product / ".native-pi-previous"
            if old.exists():
                raise ValueError(f"previous native payload backup exists: {old}")
            if dest.exists():
                dest.rename(old)
            try:
                ready.rename(dest)
            except BaseException:
                if old.exists():
                    old.rename(dest)
                raise
            if old.exists():
                shutil.rmtree(old)
        shutil.copyfile(repo / "agents-plugin/bin/ws-pi-payload.py", repo / "agents-plugin-wsflow/bin/ws-pi-payload.py")
        print(f"Prepared identical native Pi archives: {len(files)} files, {len(lock['packages']) - 1} production dependencies, {(native / 'payload.zip').stat().st_size} compressed bytes")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--repo", type=Path, default=REPO)
    parser.add_argument("--offline", action="store_true")
    parser.add_argument("--npm-cache", type=Path, help="Producer-owned isolated npm cache (also enables warmed --offline generation)")
    args = parser.parse_args()
    generate(args.repo, args.offline, args.npm_cache)
