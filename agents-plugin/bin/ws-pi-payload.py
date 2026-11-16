#!/usr/bin/env python3
"""Offline validation/assembly of the dependency closure and small native files."""
import argparse
import ast
import fnmatch
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

MATERIALIZER_SCHEMA = 3
STAGE_MAX_AGE = 3600
OS_JUNK = {".DS_Store", "Thumbs.db"}
OVERLAY_TREES = ("rsrc", "skills")
OVERLAY_FILES = ("runtime.json", "dashboard-config-manifest.json", "bin/ws-mcp-launcher.py")
FIRST_PARTY_FILES = ("dist/index.js", "dist/eff_large_wordlist.txt", "src/web-search-helper.mjs",
                     "src/helper-host.mjs", "package.json", "config-manifest.json",
                     "pi-lead-guide.md", "lead-compact-guide.md", "execute-worker-guide.md", "explore-guide.md",
                     "bundle-inputs.json")
REFERENCE_FIELDS = ("main", "module", "exports", "bin", "browser", "imports")


class StalePayload(ValueError):
    """Valid prepared files that differ from the producer checkout."""


def digest(data):
    return hashlib.sha256(data).hexdigest()


def canonical(value):
    return json.dumps(value, sort_keys=True, separators=(",", ":")).encode()


def ignored_path(name):
    return any(part in OS_JUNK or part == "__pycache__" or part.endswith((".pyc", ".pyo", ".swp", "~"))
               for part in PurePosixPath(name).parts)


def overlaid(name):
    return name in OVERLAY_FILES or any(name == r or name.startswith(r + "/") for r in OVERLAY_TREES)


def drift_input(name):
    prefix = "agents-plugin-pi/"
    return not ignored_path(name) and name.startswith(prefix) and not overlaid(name[len(prefix):])


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
    if {".runtime", ".local-devenv-runtime", ".git", ".cache", "__pycache__", ".bin", ".package-lock.json"}.intersection(p.parts) or name.endswith((".node", ".pyc", ".pyo")):
        raise ValueError(f"forbidden payload path: {name}")


def production_recipe(pkg, lock):
    records, selected = lock["packages"], {}
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
    return manifest, {"name": manifest["name"], "version": manifest["version"], "lockfileVersion": 3,
                      "packages": {"": manifest, **dict(sorted(selected.items()))}}


def normalized_lock(lock, dependencies):
    # npm's root name/private/requires bookkeeping is not dependency content.
    return {"lockfileVersion": 3, "packages": {"": {"dependencies": dependencies},
            **{p: v for p, v in sorted(lock["packages"].items()) if p}}}


def closure_key(lock, dependencies, version):
    return digest(canonical([normalized_lock(lock, dependencies), dependencies, version]))


def package_targets(pkg, root, files):
    """All local files declared by runtime manifest fields, including types conditions.

    Bare imports/browser mappings may name external packages or Node builtins;
    relative mappings and legacy main/module/bin paths must resolve locally.
    Wildcards protect every existing matching file, not a guessed single entry.
    """
    local = {name[len(root) + 1:] for name in files if name.startswith(root + "/")}
    targets = set()
    def resolve(value, field):
        if not isinstance(value, str) or not value or value.startswith(("node:", "#")):
            return
        relative = value.startswith("./")
        if field in ("imports", "browser") and not relative and value not in local:
            return
        value = value.removeprefix("./")
        if value.startswith("../") or value.startswith("/"):
            raise ValueError(f"invalid runtime target: {root}: {field}={value}")
        if "*" in value:
            matches = {name for name in local if fnmatch.fnmatchcase(name, value)}
            if not matches:
                raise ValueError(f"runtime target has no files: {root}: {field}={value}")
            targets.update(root + "/" + n for n in matches)
            return
        for name in (value, value + ".js", value + ".json", value + "/index.js"):
            if name in local:
                targets.add(root + "/" + name)
                return
        raise ValueError(f"runtime target missing: {root}: {field}={value}")
    def visit(value, field):
        if isinstance(value, dict):
            for key, child in value.items():
                if field == "browser" and (key.startswith("./") or key in local):
                    resolve(key, field)
                visit(child, field)
        elif isinstance(value, list):
            for child in value:
                visit(child, field)
        else:
            resolve(value, field)
    for field in REFERENCE_FIELDS:
        if field in pkg:
            visit(pkg[field], "browser-entry" if field == "browser" and isinstance(pkg[field], str) else field)
    return targets


def read_manifests(plugin):
    native = plugin / "native-pi"
    closure = json.loads((native / "closure-manifest.json").read_text())
    first = json.loads((native / "first-party-manifest.json").read_text())
    if closure.get("schema_version") != 1 or first.get("schema_version") != 1:
        raise ValueError("unsupported native Pi split manifest")
    return native, closure, first


def validate(plugin):
    native, closure, first = read_manifests(plugin)
    archive = native / "closure.zip"
    if digest(archive.read_bytes()) != closure["archive_sha256"]:
        raise ValueError("CLOSURE archive checksum mismatch")
    if closure_key(closure["production_lock"], closure["dependencies"], closure["prune_version"]) != closure["key"]:
        raise ValueError("CLOSURE key mismatch")
    files = closure["files"]
    with zipfile.ZipFile(archive) as z:
        if len(z.infolist()) != len(files) or set(z.namelist()) != set(files):
            raise ValueError("CLOSURE inventory mismatch")
        for info in z.infolist():
            safe_path(info.filename)
            if not info.filename.startswith("node_modules/") or stat.S_ISLNK(info.external_attr >> 16) or info.is_dir():
                raise ValueError(f"non-dependency/file CLOSURE member: {info.filename}")
            if digest(z.read(info)) != files[info.filename]:
                raise ValueError(f"CLOSURE file checksum mismatch: {info.filename}")
            if info.filename.endswith("package.json"):
                pkg = json.loads(z.read(info))
                if pkg.get("name", "").startswith(("@earendil-works/pi-", "@mariozechner/pi-")):
                    raise ValueError(f"CLOSURE contains host Pi package: {info.filename}")
        for root, record in closure["production_lock"]["packages"].items():
            if not root:
                continue
            pkg = json.loads(z.read(root + "/package.json"))
            if pkg["version"] != record["version"]:
                raise ValueError(f"CLOSURE production dependency wrong version: {root}")
            package_targets(pkg, root, files)
        web = json.loads(z.read("node_modules/pi-web-access/package.json"))
        if web.get("name") != "pi-web-access" or web["version"] != closure["dependencies"]["pi-web-access"]:
            raise ValueError("CLOSURE search package mismatch")
        for required in ("node_modules/pi-web-access/index.ts", "node_modules/pi-web-access/LICENSE", "node_modules/jiti/package.json"):
            if required not in files:
                raise ValueError(f"CLOSURE incomplete: {required}")
    if first["closure_key"] != closure["key"]:
        raise ValueError("FIRST-PARTY closure reference mismatch")
    if tree(native / "first-party", ignore_junk=True) != first["files"] or not set(FIRST_PARTY_FILES).issubset(first["files"]):
        raise ValueError("FIRST-PARTY file checksum/inventory mismatch")
    pkg = json.loads((native / "first-party/package.json").read_text())
    if pkg["dependencies"] != closure["dependencies"]:
        raise ValueError("FIRST-PARTY dependency declaration mismatch")
    header = (native / "first-party/dist/index.js").read_bytes().splitlines()[0]
    if header != f"// ws-pi-bundle-inputs: sha256-{first['bundle_input_hash']}".encode():
        raise ValueError("FIRST-PARTY bundle is stale")
    recorded = json.loads((native / "first-party/bundle-inputs.json").read_text())
    if recorded != {"schema_version": 1, "sha256": first["bundle_input_hash"]}:
        raise ValueError("FIRST-PARTY bundle input record mismatch")
    return {"closure": closure, "first": first}


def check_source(plugin, repo):
    manifests = validate(plugin)
    closure, first = manifests["closure"], manifests["first"]
    pi = repo / "agents-plugin-pi"
    hint = f"regenerate in {pi}: npm run bundle && python3 scripts/prepare-native-payload.py"
    pkg = json.loads((pi / "package.json").read_text())
    if pkg["dependencies"] != closure["dependencies"]:
        raise StalePayload(f"CLOSURE stale: agents-plugin-pi/package.json dependencies; {hint}")
    source_lock = json.loads((pi / "package-lock.json").read_text())
    try:
        _, lock = production_recipe(pkg, source_lock)
    except ValueError as exc:
        raise StalePayload(f"CLOSURE stale: agents-plugin-pi/package-lock.json production closure ({exc}); {hint}") from exc
    version = None
    for node in ast.parse((pi / "scripts/prepare-native-payload.py").read_text()).body:
        if isinstance(node, ast.Assign) and any(isinstance(t, ast.Name) and t.id == "PRUNE_RULE_VERSION" for t in node.targets):
            version = ast.literal_eval(node.value)
    changed = None
    if normalized_lock(lock, pkg["dependencies"]) != closure["production_lock"]:
        changed = "agents-plugin-pi/package-lock.json production closure"
    elif version != closure["prune_version"]:
        changed = "agents-plugin-pi/scripts/prepare-native-payload.py PRUNE_RULE_VERSION"
    if changed:
        raise StalePayload(f"CLOSURE stale: {changed}; {hint}")
    for rel, expected in first["inputs"].items():
        if drift_input(rel) and (not (repo / rel).is_file() or digest((repo / rel).read_bytes()) != expected):
            raise StalePayload(f"FIRST-PARTY stale: {rel}; {hint}")
    for root in ("src", "scripts", "dist"):
        for rel in tree(pi / root, ignore_junk=True):
            full = f"agents-plugin-pi/{root}/{rel}"
            if drift_input(full) and full not in first["inputs"]:
                raise StalePayload(f"FIRST-PARTY input added: {full}; {hint}")
    return manifests


def cleanup_stages(cache):
    pattern = re.compile(r"^\.native-pi-(?:stage-[0-9a-f]{32}|[a-z0-9_]{8})$")
    cutoff = time.time() - STAGE_MAX_AGE
    for path in cache.iterdir():
        if pattern.fullmatch(path.name) and not path.is_symlink() and path.is_dir():
            try:
                if path.stat().st_mtime < cutoff:
                    shutil.rmtree(path)
            except OSError:
                pass


def cache_matches(dest, expected):
    try:
        directories = {str(p) for name in expected for p in PurePosixPath(name).parents if str(p) != "."}
        return dest.is_dir() and tree(dest, ignore_junk=True, allow_runtime=True, expected_dirs=directories) == expected
    except (OSError, ValueError):
        return False


def materialize(plugin, cache, manifest=None):
    cache.mkdir(parents=True, exist_ok=True)
    cleanup_stages(cache)
    manifests = manifest if manifest is not None else validate(plugin)
    closure, first = manifests["closure"], manifests["first"]
    native = plugin / "native-pi"
    overlay = {}
    for root in OVERLAY_TREES:
        overlay.update({f"{root}/{p}": h for p, h in tree(plugin / root, ignore_junk=True).items()})
    for name in OVERLAY_FILES:
        overlay[name] = digest((plugin / name).read_bytes())
    identity = json.loads((plugin / "runtime.json").read_text())["plugin"]
    if identity not in ("ws", "wsflow"):
        raise ValueError("unsupported native Pi workflow owner")
    key = digest(canonical([MATERIALIZER_SCHEMA, closure["key"], digest(canonical(first)), identity, overlay]))
    dest = cache / key
    owner_bytes = (json.dumps({"plugin": identity}) + "\n").encode()
    expected = {**closure["files"], **first["files"], **overlay, "workflow-owner.json": digest(owner_bytes)}
    if cache_matches(dest, expected):
        return dest
    stage = cache / (".native-pi-stage-" + uuid.uuid4().hex)
    stage.mkdir(mode=0o700)
    ready, previous = stage / "ready", stage / "previous"
    ready.mkdir(mode=0o700)
    try:
        with zipfile.ZipFile(native / "closure.zip") as z:
            for info in z.infolist():
                path = ready / info.filename
                path.parent.mkdir(parents=True, exist_ok=True)
                path.write_bytes(z.read(info))
        for base, inventory in ((native / "first-party", first["files"]), (plugin, overlay)):
            for rel in inventory:
                path = ready / rel
                path.parent.mkdir(parents=True, exist_ok=True)
                shutil.copyfile(base / rel, path)
        (ready / "workflow-owner.json").write_bytes(owner_bytes)
        if tree(ready, ignore_junk=True) != expected:
            raise ValueError("native Pi staging verification failed")
        if cache_matches(dest, expected):
            return dest
        if dest.exists() or dest.is_symlink():
            try:
                dest.rename(previous)
            except FileNotFoundError:
                pass
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
                pass


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--plugin", type=Path, default=Path(__file__).resolve().parent.parent)
    parser.add_argument("--check-source", type=Path)
    parser.add_argument("--materialize", type=Path)
    parser.add_argument("--prefer-source", action="store_true")
    parser.add_argument("--allow-stale-source", action="store_true")
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
            print(f"WARNING: {exc}; continuing with validated prepared files", file=sys.stderr)
        else:
            parser.exit(2, f"native Pi payload unavailable: {exc}\n")
    except (OSError, ValueError, KeyError, zipfile.BadZipFile) as exc:
        parser.exit(1, f"native Pi payload unavailable: {exc}\n")


if __name__ == "__main__":
    main()
