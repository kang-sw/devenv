#!/usr/bin/env python3
"""Producer-owned, finite workflow packages for Pi-started shared daemons."""
import argparse
import hashlib
import io
import json
from pathlib import Path
import zipfile

TREES = (".claude-plugin", ".codex-plugin", "claude-agents", "hooks", "rsrc", "skills", "bin")
FILES = (".mcp.json", "runtime.json", "config-manifest.json", "dashboard-config-manifest.json")
PRODUCTS = {"ws": "agents-plugin", "wsflow": "agents-plugin-wsflow"}


def sha(data):
    return hashlib.sha256(data).hexdigest()


def package_files(root):
    files = {name: (root / name).read_bytes() for name in FILES}
    for tree in TREES:
        for path in sorted((root / tree).rglob("*")):
            if path.is_symlink():
                raise ValueError(f"workflow package contains a link: {path}")
            if path.is_file() and "__pycache__" not in path.parts and not path.name.endswith((".pyc", ".pyo", ".test.ts", ".test.tsx")):
                files[path.relative_to(root).as_posix()] = path.read_bytes()
    # Some workflow distributions carry curated skills without a manifest.
    # The complete-package consumer accepts the existing schema-1 inventory;
    # generate it in the bundle without modifying those source skill trees.
    if "skills/manifest.json" not in files:
        skills = {name.removeprefix("skills/"): sha(data.replace(b"\r\n", b"\n"))
                  for name, data in sorted(files.items()) if name.startswith("skills/")}
        if not skills:
            raise ValueError(f"incomplete workflow skills: {root}")
        files["skills/manifest.json"] = (json.dumps({"schema_version": 1, "files": skills}, sort_keys=True, indent=2) + "\n").encode()
    for required in (".claude-plugin/plugin.json", "hooks/hooks.json", "hooks/claude-mod/register.ts", "rsrc/manifest.json", "skills/manifest.json", "bin/ws-mcp-launcher.py"):
        if required not in files:
            raise ValueError(f"incomplete workflow package: {root}: {required}")
    return files


def archive(files):
    stream = io.BytesIO()
    with zipfile.ZipFile(stream, "w", compression=zipfile.ZIP_DEFLATED, compresslevel=9) as output:
        for name, data in sorted(files.items()):
            info = zipfile.ZipInfo(name, (2020, 1, 1, 0, 0, 0))
            info.create_system = 3
            info.external_attr = 0o100644 << 16
            output.writestr(info, data, compress_type=zipfile.ZIP_DEFLATED, compresslevel=9)
    return stream.getvalue()


def prepare(pi, check=False):
    dest = pi / "workflow-bundles"
    manifest_path = dest / "manifest.json"
    prior = json.loads(manifest_path.read_bytes()) if manifest_path.exists() else {}
    records = {}
    for identity, product in PRODUCTS.items():
        source = pi.parent / product
        target = dest / (identity + ".zip")
        existing = prior.get(identity, {})
        if source.is_dir():
            files = package_files(source)
            inventory = {name: sha(data) for name, data in sorted(files.items())}
        else:
            # A package-only install validates its supplied bytes; it never
            # looks for another installed workflow package or uses the network.
            inventory = existing["files"]
            files = None
        current = None
        if target.exists():
            with zipfile.ZipFile(target) as z:
                actual = {name: sha(z.read(name)) for name in z.namelist()}
                if len(z.infolist()) == len(inventory) and actual == inventory:
                    current = target.read_bytes()
        if current is None:
            if check or files is None:
                raise ValueError(f"workflow bundle stale or invalid: {target}")
            current = archive(files)
            dest.mkdir(parents=True, exist_ok=True)
            target.write_bytes(current)
        records[identity] = {"archive_sha256": sha(current), "files": inventory}
    encoded = (json.dumps(records, sort_keys=True, indent=2) + "\n").encode()
    if check or not all((pi.parent / product).is_dir() for product in PRODUCTS.values()):
        if prior != records:
            raise ValueError("workflow bundle manifest stale or invalid")
    elif not manifest_path.exists() or manifest_path.read_bytes() != encoded:
        manifest_path.write_bytes(encoded)
    return records


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--pi", type=Path, default=Path(__file__).resolve().parents[1])
    parser.add_argument("--check", action="store_true")
    args = parser.parse_args()
    prepare(args.pi.resolve(), args.check)
    print("workflow bundles: ws/wsflow complete and current")
