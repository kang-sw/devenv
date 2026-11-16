#!/usr/bin/env python3
"""Producer-only: deterministic pruned dependencies plus small first-party files."""
import argparse
import importlib.util
import json
import os
from pathlib import Path, PurePosixPath
import posixpath
import re
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
production_recipe = payload.production_recipe
PRUNE_RULE_VERSION = 2
NON_RUNTIME_DIRS = {"docs", "doc", "examples", "example", "test", "tests", "__tests__", "benchmark", "benchmarks", ".github"}
MEDIA = {".mp4", ".mov", ".webm", ".png", ".gif", ".jpg", ".jpeg", ".svg", ".ico", ".webp"}


def legal_file(name):
    # Preserve legal filename families, including camel-case and suffixed names,
    # even inside otherwise excluded documentation/test trees.
    compact = re.sub(r"[^a-z]", "", name.casefold())
    if compact.startswith(("license", "licence", "licensing", "thirdpartynotice", "copyright", "patent",
                           "notice", "copying", "authors", "contributors", "acknowledgment", "acknowledgement")):
        return True
    return bool(re.search(r"(^|[^a-z])(licen[cs]es?|unlicense|notices?|copying|copyright|authors?|contributors?|patents?|legal|acknowledg[em]*ents?|third.party|[al]?gpl|gfdl|ofl|bsd|mit|apache)([^a-z]|$)", name.lower()))


def prune_rule(name, drop_src):
    """Reviewed exclusions, ONLY for a dependency package-relative filename.

    Every rule is limited further by the caller to files that are neither
    manifest targets nor literal relative runtime resources. Declared types
    exports therefore survive the declaration rule rather than being hidden
    package-specific exceptions. Legal files and package.json always win.
    """
    p = PurePosixPath(name)
    if p.name.lower() == "package.json" or any(legal_file(part) for part in p.parts):
        return None
    # Presentation media outside compiled runtime asset directories is not code.
    if p.suffix.lower() in MEDIA and not {"dist", "lib"}.intersection(p.parts[:-1]):
        return "media"
    # Human/development trees are unnecessary unless a runtime target uses them.
    if NON_RUNTIME_DIRS.intersection(part.lower() for part in p.parts[:-1]):
        return "documentation/test directories"
    # Keep redistribution notices, but not other Markdown prose.
    if p.suffix.lower() in {".md", ".markdown"}:
        return "markdown"
    # Debug maps do not participate in runtime loading.
    if p.name.endswith(".map"):
        return "source maps"
    # Unreferenced declarations are compile-time inputs, not JavaScript runtime.
    if p.name.endswith((".d.ts", ".d.mts", ".d.cts")):
        return "type declarations"
    # TypeScript incremental build caches have no consumer runtime role.
    if p.name.endswith(".tsbuildinfo"):
        return "build metadata"
    # Only packages with compiled output and no runtime path into src qualify.
    if drop_src and "src" in p.parts[:-1]:
        return "redundant src"
    return None


def prune_dependencies(stage, lock):
    files = payload.tree(stage / "node_modules", ignore_junk=True)
    files = {"node_modules/" + name: sha for name, sha in files.items()}
    roots = sorted((p for p in lock["packages"] if p), key=lambda p: (-len(p), p))
    manifests = {root: json.loads((stage / root / "package.json").read_text()) for root in roots}
    declared = set()
    for root, pkg in manifests.items():
        declared.update(payload.package_targets(pkg, root, files))
    protected = set(declared)
    source_candidates = {root for root in roots
                         if ((stage / root / "dist").is_dir() or (stage / root / "lib").is_dir())
                         and not any(p.startswith(root + "/src/") for p in declared)}
    def code(name):
        return PurePosixPath(name).suffix in {".js", ".mjs", ".cjs", ".ts", ".css"} and not name.endswith((".d.ts", ".d.mts", ".d.cts"))
    pending = []
    for name in files:
        root = next(r for r in roots if name.startswith(r + "/"))
        parts = PurePosixPath(name[len(root) + 1:]).parts[:-1]
        if code(name) and not NON_RUNTIME_DIRS.intersection(parts) and not (root in source_candidates and "src" in parts):
            pending.append(name)
    pending.extend(name for name in declared if code(name))
    # Follow runtime imports transitively, including into otherwise removable
    # source/docs trees. Dead source imports cannot keep their own tree alive.
    scanned = set()
    while pending:
        name = pending.pop()
        if name in scanned:
            continue
        scanned.add(name)
        text = (stage / name).read_text(errors="replace")
        for value in re.findall(r'''["'`]([^"'`\\\n]+)["'`]''', text):
            if not value.startswith(("./", "../")) and "." not in PurePosixPath(value).name:
                continue
            target = posixpath.normpath(posixpath.join(posixpath.dirname(name), value.split("?")[0].split("#")[0]))
            for candidate in (target, target + ".js", target + ".json", target + ".ts", target + "/index.js"):
                if candidate in files:
                    protected.add(candidate)
                    if code(candidate) and candidate not in scanned:
                        pending.append(candidate)
    drop_src = {root: root in source_candidates and not any(p.startswith(root + "/src/") for p in protected)
                for root in roots}
    removed = {rule: {"files": 0, "bytes": 0} for rule in ("media", "documentation/test directories", "markdown", "source maps", "type declarations", "build metadata", "redundant src")}
    narrowed = []
    for name in sorted(files):
        root = next(r for r in roots if name.startswith(r + "/"))
        rule = prune_rule(name[len(root) + 1:], drop_src[root])
        if rule is None:
            continue
        if name in protected:
            narrowed.append({"file": name, "rule": rule, "reason": "declared or literal runtime reference"})
            continue
        path = stage / name
        entry = removed.setdefault(rule, {"files": 0, "bytes": 0})
        entry["files"] += 1
        entry["bytes"] += path.stat().st_size
        path.unlink()
    remaining = {"node_modules/" + p: h for p, h in payload.tree(stage / "node_modules", ignore_junk=True).items()}
    # Every manifest target must still resolve AFTER pruning; fail before publish.
    for root, pkg in manifests.items():
        after = payload.package_targets(pkg, root, remaining)
        if not payload.package_targets(pkg, root, files).issubset(after):
            raise ValueError(f"pruning removed a runtime target: {root}")
    if not protected.issubset(remaining):
        raise ValueError("pruning removed a literal runtime resource")
    mandatory = {name for name in files if PurePosixPath(name).name.lower() == "package.json"
                 or any(legal_file(part) for part in PurePosixPath(name).parts)}
    if not mandatory.issubset(remaining):
        raise ValueError("pruning removed redistribution notices or a package manifest")
    return removed, narrowed


def check_bundle_paths(pi, repo):
    bundle = (pi / "dist/index.js").read_bytes()
    for path in (repo.resolve(), Path.home().resolve(), (pi / "node_modules").resolve()):
        if str(path).encode() in bundle:
            raise ValueError(f"native Pi bundle contains a producer path: {path}; run npm run bundle")


def bundle_hash(pi):
    files = payload.tree(pi / "src", ignore_junk=True)
    names = ["src/" + p for p in files if p.endswith((".ts", ".mjs"))]
    names += ["src/eff_large_wordlist.txt", "package-lock.json", "scripts/bundle.mjs"]
    value = payload.digest("".join(f"{p}\n{payload.digest((pi / p).read_bytes())}\n" for p in sorted(names)).encode())
    if (pi / "dist/index.js").read_bytes().splitlines()[0] != f"// ws-pi-bundle-inputs: sha256-{value}".encode():
        raise ValueError("FIRST-PARTY source bundle is stale; run npm run bundle")
    return value


def write_zip(path, stage, files):
    with zipfile.ZipFile(path, "w", compression=zipfile.ZIP_DEFLATED, compresslevel=9) as z:
        for rel in sorted(files):
            payload.safe_path(rel)
            info = zipfile.ZipInfo(rel, (2020, 1, 1, 0, 0, 0))
            info.create_system = 3  # Fixed Unix metadata, including Windows producers.
            info.external_attr = 0o100644 << 16
            z.writestr(info, (stage / rel).read_bytes(), compress_type=zipfile.ZIP_DEFLATED, compresslevel=9)


def retain_committed_closure(repo, native, candidate):
    """Reuse committed encoding for a same-key, byte-exact CONTENT inventory.

    Compression and ZIP creator metadata are not closure inputs. Do not make a
    Windows/zlib-ng producer mint a permanent blob for identical runtime files.
    Validate the chosen committed archive later, with the new first-party set.
    """
    selected = None
    for product in ("agents-plugin", "agents-plugin-wsflow"):
        prior = repo / product / "native-pi"
        manifest = prior / "closure-manifest.json"
        if not manifest.exists():
            continue
        existing = json.loads(manifest.read_text())
        if existing["key"] != candidate["key"]:
            continue
        if existing["files"] != candidate["files"]:
            raise ValueError("same closure key, different CONTENT: key-input bug or prune change without a version bump")
        if selected is not None:
            for name in ("closure.zip", "closure-manifest.json"):
                if (prior / name).read_bytes() != (selected / name).read_bytes():
                    raise ValueError("committed same-key closure copies disagree; reconcile them before generation")
        selected = prior
    if selected is None:
        return None
    for name in ("closure.zip", "closure-manifest.json"):
        shutil.copyfile(selected / name, native / name)
    return json.loads((native / "closure-manifest.json").read_text())


def generate(repo, offline=False, npm_cache=None, report=None):
    npm = shutil.which("npm")
    if npm is None:
        raise SystemExit("npm not found on PATH; install Node.js and npm to prepare the native payload")
    repo = repo.resolve()
    pi = repo / "agents-plugin-pi"
    check_bundle_paths(pi, repo)
    input_hash = bundle_hash(pi)
    pkg = json.loads((pi / "package.json").read_text())
    recipe, lock = production_recipe(pkg, json.loads((pi / "package-lock.json").read_text()))
    with tempfile.TemporaryDirectory(prefix="ws-native-pi-") as tmp:
        temp = Path(tmp).resolve()
        deps = temp / "dependencies"
        deps.mkdir()
        (deps / "package.json").write_text(json.dumps(recipe))
        (deps / "package-lock.json").write_text(json.dumps(lock))
        env = {k: v for k, v in os.environ.items() if not k.lower().startswith("npm_config_") and k not in ("NODE_OPTIONS", "NODE_PATH")}
        for name, folder in (("HOME", "home"), ("USERPROFILE", "home"), ("WS_CACHE_HOME", "cache"), ("WS_CONFIG_HOME", "config"), ("npm_config_cache", "npm-cache")):
            path = temp / folder
            path.mkdir(exist_ok=True)
            env[name] = str(path)
        if npm_cache is not None:
            env["npm_config_cache"] = str(npm_cache.resolve())
        env["npm_config_userconfig"] = os.devnull
        flags = ["--ignore-scripts", "--legacy-peer-deps", "--bin-links=false", "--no-audit", "--no-fund"] + (["--offline"] if offline else [])
        subprocess.run([npm, "install", "--package-lock-only", *flags], cwd=deps, env=env, check=True)
        normalized = json.loads((deps / "package-lock.json").read_text())
        if payload.normalized_lock(normalized, pkg["dependencies"]) != payload.normalized_lock(lock, pkg["dependencies"]):
            raise ValueError("npm changed the canonical production dependency graph")
        subprocess.run([npm, "ci", "--omit=dev", *flags], cwd=deps, env=env, check=True)
        (deps / "node_modules/.package-lock.json").unlink(missing_ok=True)
        removed, narrowed = prune_dependencies(deps, normalized)
        native = temp / "native-pi"
        native.mkdir()
        closure_files = {"node_modules/" + p: h for p, h in payload.tree(deps / "node_modules", ignore_junk=True).items()}
        candidate = {"schema_version": 1, "key": payload.closure_key(normalized, pkg["dependencies"], PRUNE_RULE_VERSION),
                     "prune_version": PRUNE_RULE_VERSION, "dependencies": pkg["dependencies"],
                     "production_lock": payload.normalized_lock(normalized, pkg["dependencies"]), "files": closure_files}
        closure = retain_committed_closure(repo, native, candidate)
        if closure is None:
            write_zip(native / "closure.zip", deps, closure_files)
            closure = {**candidate, "archive_sha256": payload.digest((native / "closure.zip").read_bytes())}
            (native / "closure-manifest.json").write_bytes(payload.canonical(closure) + b"\n")
        first = native / "first-party"
        for name in payload.FIRST_PARTY_FILES:
            target = first / name
            target.parent.mkdir(parents=True, exist_ok=True)
            if name == "package.json":
                runtime_pkg = {**pkg, "pi": {**pkg["pi"], "extensions": ["./dist/index.js"]}}
                target.write_bytes(payload.canonical(runtime_pkg) + b"\n")
            elif name == "bundle-inputs.json":
                target.write_bytes(payload.canonical({"schema_version": 1, "sha256": input_hash}) + b"\n")
            else:
                shutil.copyfile(pi / name, target)
        inputs = {}
        for root in ("src", "scripts", "dist"):
            for rel, sha in payload.tree(pi / root, ignore_junk=True).items():
                inputs[f"agents-plugin-pi/{root}/{rel}"] = sha
        for name in ("package.json", "package-lock.json", "config-manifest.json", "pi-lead-guide.md", "lead-compact-guide.md", "execute-worker-guide.md", "explore-guide.md"):
            inputs[f"agents-plugin-pi/{name}"] = payload.digest((pi / name).read_bytes())
        small = {"schema_version": 1, "closure_key": closure["key"], "bundle_input_hash": input_hash,
                 "files": payload.tree(first), "inputs": dict(sorted(inputs.items()))}
        (native / "first-party-manifest.json").write_text(json.dumps(small, sort_keys=True, indent=2) + "\n")
        fixture = temp / "fixture"
        fixture.mkdir()
        shutil.copytree(native, fixture / "native-pi")
        payload.validate(fixture)
        for product in ("agents-plugin", "agents-plugin-wsflow"):
            dest = repo / product / "native-pi"
            ready, old = repo / product / ".native-pi-prepared", repo / product / ".native-pi-previous"
            if old.exists():
                raise ValueError(f"previous native backup exists: {old}")
            shutil.rmtree(ready, ignore_errors=True)
            shutil.copytree(native, ready)
            if dest.exists():
                dest.rename(old)
            try:
                ready.rename(dest)
            except BaseException:
                if old.exists():
                    old.rename(dest)
                raise
            shutil.rmtree(old, ignore_errors=True)
        sizes = sorted(((name, (deps / name).stat().st_size) for name in closure_files), key=lambda x: (-x[1], x[0]))
        result = {"closure_key": closure["key"], "zip_bytes": (native / "closure.zip").stat().st_size,
                  "closure_manifest_bytes": (native / "closure-manifest.json").stat().st_size,
                  "first_party_bytes": sum(p.stat().st_size for p in first.rglob("*") if p.is_file()),
                  "first_party_manifest_bytes": (native / "first-party-manifest.json").stat().st_size,
                  "closure_files": len(closure_files), "closure_unpacked_bytes": sum(size for _, size in sizes),
                  "first_party_files": len(small["files"]), "removed": removed, "narrowed": narrowed, "largest": sizes[:15]}
        if report:
            report.write_text(json.dumps(result, indent=2) + "\n")
        print(json.dumps(result, sort_keys=True))


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--repo", type=Path, default=REPO)
    parser.add_argument("--offline", action="store_true")
    parser.add_argument("--npm-cache", type=Path)
    parser.add_argument("--report", type=Path)
    args = parser.parse_args()
    generate(args.repo, args.offline, args.npm_cache, args.report)
