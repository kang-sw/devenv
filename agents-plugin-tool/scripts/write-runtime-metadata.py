#!/usr/bin/env python3
"""Stamp a just-built runtime without executing it; overrides match build ldflags."""
import argparse
import importlib.util
import sys
from pathlib import Path

sys.dont_write_bytecode = True
TOOL_DIR = Path(__file__).resolve().parents[1]
LAUNCHER = TOOL_DIR.parent / "agents-plugin" / "bin" / "ws-mcp-launcher.py"


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("binary", type=Path)
    parser.add_argument("--version")
    parser.add_argument("--source-commit")
    args = parser.parse_args()
    spec = importlib.util.spec_from_file_location("ws_mcp_launcher", LAUNCHER)
    launcher = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(launcher)
    identity = launcher.source_runtime_metadata(TOOL_DIR)
    if args.version is not None:
        identity["version"] = args.version
    if args.source_commit is not None:
        identity["source_commit"] = args.source_commit
    launcher.write_runtime_metadata(args.binary, identity)


if __name__ == "__main__":
    main()
