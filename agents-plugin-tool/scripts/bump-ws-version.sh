#!/bin/sh
set -eu

usage() {
  printf 'usage: %s <X.Y.Z>\n' "$(basename "$0")" >&2
}

version=${1:-}
if [ -z "$version" ]; then
  usage
  exit 2
fi
version=${version#v}

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd -P)
tool_dir=$(CDPATH= cd -- "$script_dir/.." && pwd -P)
repo_root=$(CDPATH= cd -- "$tool_dir/.." && pwd -P)

VERSION=$version REPO_ROOT=$repo_root python3 - <<'PY'
import json
import os
import re
import shutil
from pathlib import Path

version = os.environ["VERSION"]
repo = Path(os.environ["REPO_ROOT"])

match = re.fullmatch(r"(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)", version)
if not match:
    raise SystemExit(f"invalid version {version!r}; expected X.Y.Z")

major, minor, _patch = (int(part) for part in match.groups())
dev_version = f"{version}-dev"
release_tag = f"v{version}"
upper_bound = f"{major}.{minor + 1}.0"
contract_range = f">={dev_version} <{upper_bound}"
compatible_glob = f"{major}.{minor}.*"


def rel(path: str) -> Path:
    return repo / path


def read_text(path: str) -> str:
    return rel(path).read_text(encoding="utf-8")


def write_text(path: str, content: str) -> None:
    rel(path).write_text(content, encoding="utf-8")


def update_json(path: str, mutator) -> None:
    file_path = rel(path)
    data = json.loads(file_path.read_text(encoding="utf-8"))
    mutator(data)
    file_path.write_text(json.dumps(data, indent=2) + "\n", encoding="utf-8")


def update_plugin_manifest(data) -> None:
    data["version"] = version


def update_runtime(data) -> None:
    data["plugin_version"] = version
    data["required_mcp"] = contract_range
    data["release_tag"] = release_tag
    for section in ("tools", "commands"):
        for key in list(data.get(section, {})):
            data[section][key] = contract_range


update_json("agents-plugin/.codex-plugin/plugin.json", update_plugin_manifest)
update_json("agents-plugin/.claude-plugin/plugin.json", update_plugin_manifest)
update_json("agents-plugin/runtime.json", update_runtime)
update_json("agents-plugin-wsflow/.codex-plugin/plugin.json", update_plugin_manifest)
update_json("agents-plugin-wsflow/.claude-plugin/plugin.json", update_plugin_manifest)
update_json("agents-plugin-wsflow/runtime.json", update_runtime)


def sync_file(src: str, dst: str) -> None:
    shutil.copyfile(rel(src), rel(dst))


def sync_tree(src: str, dst: str) -> None:
    dst_path = rel(dst)
    tmp_path = dst_path.with_name(dst_path.name + ".sync-tmp")
    if tmp_path.exists():
        shutil.rmtree(tmp_path)
    # Copy into a sibling temp dir first so a failure here (missing src,
    # permission error) never leaves dst_path deleted or half-written; only
    # the final swap below touches dst_path, and it is a single rename.
    shutil.copytree(rel(src), tmp_path)
    if dst_path.exists():
        shutil.rmtree(dst_path)
    tmp_path.rename(dst_path)


def update_pi_bridge_package(data) -> None:
    data["version"] = version


# agents-plugin-pi/ mirrors agents-plugin/'s runtime.json, launcher, and rsrc/
# byte-for-byte; resync from the just-updated agents-plugin/ copies rather than
# re-deriving the version fields independently, so the two never drift.
sync_file("agents-plugin/runtime.json", "agents-plugin-pi/runtime.json")
sync_file("agents-plugin/bin/ws-mcp-launcher.py", "agents-plugin-pi/bin/ws-mcp-launcher.py")
sync_tree("agents-plugin/rsrc", "agents-plugin-pi/rsrc")
update_json("agents-plugin-pi/package.json", update_pi_bridge_package)
update_json("package.json", update_pi_bridge_package)

main_go = read_text("agents-plugin-tool/cmd/ws-mcp/main.go")
main_go = re.sub(
    r'var version = "[^"]+"',
    f'var version = "{dev_version}"',
    main_go,
    count=1,
)
write_text("agents-plugin-tool/cmd/ws-mcp/main.go", main_go)

build_script = read_text("agents-plugin-tool/scripts/build-release-assets.sh")
build_script = re.sub(
    r"(?m)^([ \t]*)version=[0-9]+\.[0-9]+\.[0-9]+-dev$",
    rf"\g<1>version={dev_version}",
    build_script,
    count=1,
)
write_text("agents-plugin-tool/scripts/build-release-assets.sh", build_script)

workflow = read_text(".github/workflows/ws-mcp-release.yml")
workflow = re.sub(r"[0-9]+\.[0-9]+\.[0-9]+-dev", dev_version, workflow)
write_text(".github/workflows/ws-mcp-release.yml", workflow)

agents_md = read_text("AGENTS.md")
agents_md = re.sub(r"agents-plugin/` \(`ws@[0-9]+\.[0-9]+\.[0-9]+`\)", f"agents-plugin/` (`ws@{version}`)", agents_md)
agents_md = re.sub(r"agents-plugin-wsflow/` \(`wsflow@[0-9]+\.[0-9]+\.[0-9]+`\)", f"agents-plugin-wsflow/` (`wsflow@{version}`)", agents_md)
write_text("AGENTS.md", agents_md)

ws_mcp_ref = read_text("ai-docs/manuals/ws-mcp.md")
ws_mcp_ref = re.sub(r"v[0-9]+\.[0-9]+\.[0-9]+", release_tag, ws_mcp_ref)
ws_mcp_ref = re.sub(r"[0-9]+\.[0-9]+\.[0-9]+-dev", dev_version, ws_mcp_ref)
ws_mcp_ref = re.sub(r"[0-9]+\.[0-9]+\.x", f"{major}.{minor}.x", ws_mcp_ref)
write_text("ai-docs/manuals/ws-mcp.md", ws_mcp_ref)

print(f"ws version set to {version}")
print(f"release tag: {release_tag}")
print(f"dev version: {dev_version}")
print(f"runtime range: {contract_range}")
print("agents-plugin-pi/ mirrors resynced from agents-plugin/")
PY
