#!/usr/bin/env python3
"""Generate the daemon's embedded packages from canonical checkout inputs.

Run before Go compilation. --check is a byte-for-byte drift guard; --inventory
prints the exact embedded inventory without writing (also used by desktop
assembly to check a PREBUILT binary, never to retrofit it).
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import sys

PACKAGES = {"ws": "agents-plugin", "wsflow": "agents-plugin-wsflow"}
TREES = ("rsrc", "skills")
FILES = ("runtime.json", "config-manifest.json", "agy-config-manifest.json", "dashboard-config-manifest.json",
         ".claude-plugin/plugin.json", "bin/ws-mcp-launcher.py")
# Retain this small generated index in Git: a placeholder-only Go build can
# still resolve an explicit dev checkout without embedding a second asset tree.
PLACEHOLDER = "PLACEHOLDER"


def clean_name(name):
    return (not name.startswith(".") and name not in {"__pycache__", "node_modules"}
            and not name.endswith((".pyc", ".pyo", "~", ".local", ".local.md",
                                   ".local.json", ".log", ".tmp", ".bak", ".swp", "Zone.Identifier")))


def tree_files(root):
    if root.is_symlink() or not root.is_dir():
        raise ValueError(f"missing regular input directory: {root}")
    for directory, dirs, files in os.walk(root, followlinks=False):
        dirs[:] = sorted(d for d in dirs if clean_name(d) and not (Path(directory) / d).is_symlink())
        for name in sorted(files):
            path = Path(directory) / name
            if clean_name(name) and not path.is_symlink() and path.is_file():
                yield path


def package_input_paths(repo):
    """List canonical inputs by embedded name using only directory/stat metadata."""
    # Checkout ancestors may be platform aliases (/var on macOS). Reject
    # symlinks within selected package paths, not outside the checkout.
    repo = repo.resolve()
    result = {}
    for product, folder in PACKAGES.items():
        root = repo / folder
        for name in FILES + (("bin/ws-pi-payload.py",) if product == "ws" else ()):
            path = root / name
            # Do not follow links even in explicitly selected paths.
            if any((repo / Path(*path.relative_to(repo).parts[:i])).is_symlink()
                   for i in range(1, len(path.relative_to(repo).parts) + 1)) or not path.is_file():
                raise ValueError(f"missing regular input file: {path}")
            result[f"{product}/{name}"] = path
        for tree in TREES + (("native-pi",) if product == "ws" else ()):
            for path in tree_files(root / tree):
                result[f"{product}/{path.relative_to(root).as_posix()}"] = path
    return result


def package_files(repo):
    result = {PLACEHOLDER: (json.dumps(PACKAGES, sort_keys=True, separators=(",", ":")) + "\n").encode()}
    result.update((name, path.read_bytes()) for name, path in package_input_paths(repo).items())
    return result


def inventory_bytes(files):
    payload = {"format": "ws-embedded-packages-v1", "files": {
        name: hashlib.sha256(data).hexdigest() for name, data in sorted(files.items())}}
    return (json.dumps(payload, sort_keys=True, separators=(",", ":")) + "\n").encode()


def generate(repo, output, check=False):
    expected = package_files(repo)
    expected["inventory.json"] = inventory_bytes(expected)
    actual = {}
    if output.is_symlink():
        raise ValueError(f"generated output must not be a symlink: {output}")
    if output.exists():
        for path in output.rglob("*"):
            if path.is_symlink():
                actual[path.relative_to(output).as_posix()] = None
            elif path.is_file():
                actual[path.relative_to(output).as_posix()] = path.read_bytes()
    drift = sorted(name for name in expected.keys() | actual.keys()
                   if name not in expected or name not in actual or expected[name] != actual[name])
    if check:
        if drift:
            raise ValueError("embedded package assets missing/stale/extra; run generate-package-assets.py:\n" + "\n".join(drift))
        return
    # Unchanged generation writes nothing, preserving source stamps and avoiding
    # antivirus scans of hundreds of unchanged assets on Windows.
    for name in drift:
        path = output / name
        if path.is_symlink():
            path.unlink()
        if name not in expected:
            path.unlink(missing_ok=True)
        else:
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_bytes(expected[name])
    # Empty directories can make go:embed fail after a source deletion.
    if output.exists():
        for path in sorted(output.rglob("*"), reverse=True):
            if path.is_dir() and not path.is_symlink() and not any(path.iterdir()):
                path.rmdir()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--checkout", type=Path, default=Path(__file__).resolve().parents[2])
    parser.add_argument("--output", type=Path)
    parser.add_argument("--check", action="store_true")
    parser.add_argument("--inventory", action="store_true")
    args = parser.parse_args()
    try:
        repo = args.checkout.resolve()
        if args.inventory:
            sys.stdout.buffer.write(inventory_bytes(package_files(repo)))
        else:
            generate(repo, args.output or repo / "agents-plugin-tool/internal/wspackage/generated", args.check)
    except (OSError, ValueError) as exc:
        print(f"generate-package-assets: {exc}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
