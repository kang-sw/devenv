#!/usr/bin/env python3
import hashlib
import json
import os
import platform as platform_module
import shutil
import subprocess
import sys
import tempfile
import uuid
import urllib.request
from pathlib import Path


def fail(message: str) -> None:
    print(f"ws-mcp-launcher: {message}", file=sys.stderr)
    raise SystemExit(1)


def note(message: str) -> None:
    if os.environ.get("WS_MCP_LAUNCHER_DEBUG") == "1":
        print(f"ws-mcp-launcher: {message}", file=sys.stderr)


def read_runtime_contract(path: Path) -> dict:
    if not path.is_file():
        fail(f"missing runtime contract: {path}")
    try:
        return json.loads(path.read_text(encoding="utf-8"))
    except Exception as exc:
        fail(f"invalid runtime contract {path}: {exc}")


def host_os() -> str:
    system = platform_module.system().lower()
    if system == "darwin":
        return "darwin"
    if system == "linux":
        return "linux"
    if system == "windows":
        return "windows"
    fail(f"unsupported operating system: {system}")


def host_arch() -> str:
    machine = platform_module.machine().lower()
    if machine in {"arm64", "aarch64"}:
        return "arm64"
    if machine in {"x86_64", "amd64"}:
        return "amd64"
    fail(f"unsupported architecture: {machine}")


def version_tuple(value: str) -> tuple[int, int, int] | None:
    core = value.strip().split("-", 1)[0]
    parts = core.split(".")
    if len(parts) != 3:
        return None
    try:
        return tuple(int(part) for part in parts)  # type: ignore[return-value]
    except ValueError:
        return None


def version_compatible(version: str, contract: dict) -> bool:
    actual = version_tuple(version)
    plugin = version_tuple(str(contract.get("plugin_version", "")))
    if actual is None or plugin is None:
        return False
    return actual == plugin


def sha256_file(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as fh:
        for chunk in iter(lambda: fh.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def runtime_binary_name(contract: dict, contract_path: Path, os_name: str) -> str:
    version = str(contract.get("plugin_version", "")).strip()
    if not version:
        version = "unknown"
    safe_version = "".join(char if char.isalnum() or char in ".-" else "-" for char in version)
    contract_hash = sha256_file(contract_path)[:12]
    suffix = ".exe" if os_name == "windows" else ""
    return f"ws-mcp-{safe_version}-{contract_hash}{suffix}"


def download_file(url: str, destination: Path) -> None:
    try:
        with urllib.request.urlopen(url) as response, destination.open("wb") as out:
            shutil.copyfileobj(response, out)
    except Exception as exc:
        fail(f"failed to download {url}: {exc}")


def expected_checksum(sums_path: Path, asset: str) -> str:
    for line in sums_path.read_text(encoding="utf-8").splitlines():
        parts = line.split()
        if len(parts) >= 2 and parts[1] == asset:
            return parts[0].lower()
    fail(f"missing checksum entry for {asset}")


def runtime_tools(contract: dict) -> list[str]:
    return sorted(str(key) for key in contract.get("tools", {}))


def runtime_commands(contract: dict) -> list[str]:
    return sorted(str(key) for key in contract.get("commands", {}))


def run_binary(binary: Path, args: list[str], *, input_text: str | None = None, timeout: int = 30) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        [str(binary), *args],
        input=input_text,
        text=True,
        capture_output=True,
        timeout=timeout,
        check=False,
    )


def tools_compatible(binary: Path, contract: dict, runtime_dir: Path) -> bool:
    with tempfile.TemporaryDirectory(prefix="ws-mcp-compat.", dir=str(runtime_dir)) as temp_dir:
        root = Path(temp_dir) / "root"
        root.mkdir()
        git = shutil.which("git")
        if git:
            subprocess.run([git, "-C", str(root), "init"], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, check=False)
        payload = "\n".join(
            [
                '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}',
                '{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}',
                "",
            ]
        )
        env = os.environ.copy()
        env["WS_CACHE_HOME"] = temp_dir
        env["WS_MCP_TOOL_PROFILE"] = "lead"
        env["WS_MCP_ALLOWED_TOOLS"] = ""
        try:
            proc = subprocess.run(
                [str(binary), "serve", "--stdio", "--root", str(root)],
                input=payload,
                text=True,
                capture_output=True,
                timeout=30,
                check=False,
                env=env,
            )
        except Exception as exc:
            note(f"runtime tool compatibility check failed: {exc}")
            return False
    if not proc.stdout:
        return False
    for tool in runtime_tools(contract):
        if f'"name":"{tool}"' not in proc.stdout:
            note(f"runtime missing required MCP tool: {tool}")
            return False
    return True


def commands_compatible(binary: Path, contract: dict) -> bool:
    for command in runtime_commands(contract):
        parts = command.split(".")
        if len(parts) >= 3:
            args = [parts[0], parts[1]]
            expected = parts[2]
        elif len(parts) == 2:
            args = [parts[0]]
            expected = parts[1]
        else:
            args = []
            expected = parts[0]
        try:
            proc = run_binary(binary, args)
        except Exception:
            return False
        output = proc.stdout + proc.stderr
        if expected not in output:
            note(f"runtime missing required CLI command: {command}")
            return False
    return True


def prompt_bundle_compatible(binary: Path, contract: dict) -> bool:
    expected = contract.get("prompt_bundle", {}).get("content_sha256")
    if not expected:
        return True
    try:
        proc = run_binary(binary, ["runtime", "info"])
        if proc.returncode != 0 or not proc.stdout:
            return False
        info = json.loads(proc.stdout)
    except Exception:
        return False
    if info.get("prompt_bundle", {}).get("content_sha256") != expected:
        note("runtime prompt bundle hash mismatch")
        return False
    return True


def _capabilities_string_list(payload: dict, key: str) -> list[str] | None:
    values = payload.get(key)
    if not isinstance(values, list):
        return None
    result = []
    for value in values:
        if not isinstance(value, str):
            return None
        result.append(value)
    return result


def _capabilities_match_exact(contract: dict) -> bool:
    settings = contract.get("runtime_capabilities", {})
    if not isinstance(settings, dict):
        return False
    return str(settings.get("match", "")).lower() == "exact"


def _capabilities_match_contract(payload: dict, key: str, required: list[str], *, exact: bool) -> bool:
    actual = _capabilities_string_list(payload, key)
    if actual is None:
        return False
    actual_set = set(actual)
    required_set = set(required)
    for name in required_set:
        if name not in actual_set:
            note(f"runtime capabilities missing required {key[:-1]}: {name}")
            return False
    if exact:
        unexpected = actual_set - required_set
        if unexpected:
            note(f"runtime capabilities exposed unexpected {key[:-1]}: {sorted(unexpected)[0]}")
            return False
    return True


def compatibility_stamp_path(runtime_dir: Path) -> Path:
    return runtime_dir / ".compatibility.json"


def unique_runtime_temp_path(runtime_dir: Path, label: str) -> Path:
    return runtime_dir / f".{label}.{os.getpid()}.{uuid.uuid4().hex}.tmp"


def compatibility_stamp_payload(binary: Path, contract: dict, contract_path: Path) -> dict | None:
    try:
        stat = binary.stat()
    except OSError:
        return None
    return {
        "schema_version": 1,
        "contract_sha256": sha256_file(contract_path),
        "plugin_version": str(contract.get("plugin_version", "")),
        "required_mcp": str(contract.get("required_mcp", "")),
        "accepted_runtime_version": str(contract.get("plugin_version", "")),
        "binary_path": str(binary.resolve()),
        "binary_size": stat.st_size,
        "binary_mtime_ns": stat.st_mtime_ns,
    }


def compatibility_stamp_current(binary: Path, contract: dict, contract_path: Path, runtime_dir: Path) -> bool:
    expected = compatibility_stamp_payload(binary, contract, contract_path)
    if expected is None:
        return False
    try:
        actual = json.loads(compatibility_stamp_path(runtime_dir).read_text(encoding="utf-8"))
    except Exception:
        return False
    if actual == expected:
        note("using cached runtime compatibility stamp")
        return True
    return False


def write_compatibility_stamp(binary: Path, contract: dict, contract_path: Path, runtime_dir: Path) -> None:
    payload = compatibility_stamp_payload(binary, contract, contract_path)
    if payload is None:
        return
    try:
        runtime_dir.mkdir(parents=True, exist_ok=True)
        stamp = compatibility_stamp_path(runtime_dir)
        tmp = unique_runtime_temp_path(runtime_dir, "compatibility")
        tmp.write_text(json.dumps(payload, sort_keys=True, separators=(",", ":")) + "\n", encoding="utf-8")
        os.replace(tmp, stamp)
    except Exception as exc:
        note(f"failed to write runtime compatibility stamp: {exc}")


def clear_compatibility_stamp(runtime_dir: Path) -> None:
    try:
        compatibility_stamp_path(runtime_dir).unlink(missing_ok=True)
    except Exception as exc:
        note(f"failed to clear runtime compatibility stamp: {exc}")


def install_downloaded_runtime(binary: Path, runtime_dir: Path, asset: str, contract: dict) -> None:
    base_url = os.environ.get("WS_MCP_RELEASE_BASE_URL")
    repository = os.environ.get("WS_MCP_RELEASE_REPOSITORY") or contract.get("release_repository")
    tag = os.environ.get("WS_MCP_RELEASE_TAG") or contract.get("release_tag")
    if not base_url:
        if not repository or not tag:
            fail("missing runtime binary; set WS_MCP_BOOTSTRAP_BINARY, WS_MCP_BOOTSTRAP_URL, or release_repository/release_tag in runtime.json")
        base_url = f"https://github.com/{repository}/releases/download/{tag}"

    runtime_dir.mkdir(parents=True, exist_ok=True)
    tmp = unique_runtime_temp_path(runtime_dir, f"{binary.name}.download")
    sums_tmp = unique_runtime_temp_path(runtime_dir, "SHA256SUMS.download")

    try:
        download_file(f"{base_url}/{asset}", tmp)
        download_file(f"{base_url}/SHA256SUMS", sums_tmp)
        expected = expected_checksum(sums_tmp, asset)
        actual = sha256_file(tmp)
        if actual != expected:
            fail(f"checksum mismatch for downloaded {asset}")
        try:
            tmp.chmod(0o755)
        except OSError:
            pass
        installed = install_tmp_runtime(tmp, binary, contract, runtime_dir, f"downloaded runtime binary into {binary}")
    finally:
        tmp.unlink(missing_ok=True)
        sums_tmp.unlink(missing_ok=True)
    if installed:
        try:
            binary.chmod(0o755)
        except OSError:
            pass


def copy_runtime(source: Path, destination: Path) -> None:
    destination.parent.mkdir(parents=True, exist_ok=True)
    shutil.copy2(source, destination)
    try:
        destination.chmod(0o755)
    except OSError:
        pass


def install_tmp_runtime(tmp: Path, binary: Path, contract: dict, runtime_dir: Path, message: str) -> bool:
    try:
        os.replace(tmp, binary)
    except OSError as exc:
        if runtime_fully_compatible(binary, contract, runtime_dir):
            note(f"using compatible runtime already installed at {binary} after replace failed: {exc}")
            return False
        fail(f"failed to install runtime at {binary}: {exc}")
    note(message)
    return True


def local_devenv_cache_package(plugin_dir: Path) -> str | None:
    home = Path.home()
    try:
        rel = plugin_dir.relative_to(home / ".codex" / "plugins" / "cache" / "kang-sw-devenv")
    except ValueError:
        return None
    parts = rel.parts
    if len(parts) < 2 or parts[0] not in {"ws", "wsflow"}:
        return None
    return parts[0]


def read_local_devenv_contract(plugin_dir: Path, os_name: str) -> dict | None:
    if os_name == "windows" or local_devenv_cache_package(plugin_dir) is None:
        return None
    marker = plugin_dir / ".local-devenv-runtime"
    if not marker.is_file():
        return None
    try:
        payload = json.loads(marker.read_text(encoding="utf-8"))
    except Exception as exc:
        note(f"local devenv runtime contract is inactive: invalid JSON in {marker}: {exc}")
        return None
    if not isinstance(payload, dict):
        note(f"local devenv runtime contract is inactive: expected object in {marker}")
        return None
    if payload.get("schema_version") != 1:
        note("local devenv runtime contract is inactive: unsupported schema_version")
        return None
    resolved = {}
    for key in ("source_root", "tool_dir", "go"):
        value = payload.get(key)
        if not isinstance(value, str) or not value:
            note(f"local devenv runtime contract is inactive: missing {key}")
            return None
        path = Path(value)
        if not path.is_absolute():
            note(f"local devenv runtime contract is inactive: {key} is not absolute")
            return None
        resolved[key] = path
    if not resolved["source_root"].is_dir():
        note("local devenv runtime contract is inactive: source_root does not exist")
        return None
    if not resolved["tool_dir"].is_dir() or not (resolved["tool_dir"] / "cmd" / "ws-mcp").is_dir():
        note("local devenv runtime contract is inactive: tool_dir is not a ws-mcp module")
        return None
    if not resolved["go"].is_file() or not os.access(resolved["go"], os.X_OK):
        note("local devenv runtime contract is inactive: go is not executable")
        return None
    return resolved


def local_devenv_runtime_enabled(plugin_dir: Path, os_name: str) -> bool:
    return read_local_devenv_contract(plugin_dir, os_name) is not None


def build_local_devenv_runtime(runtime_dir: Path, binary: Path, contract: dict, local_contract: dict) -> bool:
    tool_dir = local_contract["tool_dir"]
    go_binary = local_contract["go"]
    tmp = unique_runtime_temp_path(runtime_dir, f"{binary.name}.local")
    proc = subprocess.run([str(go_binary), "build", "-o", str(tmp), "./cmd/ws-mcp"], cwd=str(tool_dir), check=False)
    if proc.returncode == 0 and runtime_fully_compatible(tmp, contract, runtime_dir):
        install_tmp_runtime(tmp, binary, contract, runtime_dir, f"built local devenv runtime from {tool_dir}")
        return True
    tmp.unlink(missing_ok=True)
    note(f"local devenv build failed or produced incompatible runtime: exit={proc.returncode}")
    return False


def install_local_devenv_runtime(plugin_dir: Path, runtime_dir: Path, binary: Path, asset: str, contract: dict, os_name: str, platform_name: str, *, prefer_build: bool = False) -> bool:
    local_contract = read_local_devenv_contract(plugin_dir, os_name)
    if local_contract is None:
        return False

    if prefer_build:
        return build_local_devenv_runtime(runtime_dir, binary, contract, local_contract)

    tmp = unique_runtime_temp_path(runtime_dir, f"{binary.name}.local")
    source_root = local_contract["source_root"]
    tool_dir = local_contract["tool_dir"]
    candidates = [
        tool_dir / "dist" / asset,
        source_root / "agents-plugin" / ".runtime" / platform_name / binary.name,
    ]
    for candidate in candidates:
        if candidate.is_file():
            copy_runtime(candidate, tmp)
            if runtime_fully_compatible(tmp, contract, runtime_dir):
                install_tmp_runtime(tmp, binary, contract, runtime_dir, f"installed local devenv runtime from {candidate}")
                return True
            tmp.unlink(missing_ok=True)
            note(f"local devenv runtime candidate is incompatible: {candidate}")

    return build_local_devenv_runtime(runtime_dir, binary, contract, local_contract)


def runtime_install_forced(plugin_dir: Path, os_name: str) -> bool:
    return bool(os.environ.get("WS_MCP_BOOTSTRAP_BINARY") or os.environ.get("WS_MCP_BOOTSTRAP_URL")) or local_devenv_runtime_enabled(plugin_dir, os_name)


def install_runtime(plugin_dir: Path, runtime_dir: Path, binary: Path, asset: str, contract: dict, os_name: str, platform_name: str, *, force_local: bool = False) -> None:
    bootstrap_binary = os.environ.get("WS_MCP_BOOTSTRAP_BINARY")
    bootstrap_url = os.environ.get("WS_MCP_BOOTSTRAP_URL")
    if bootstrap_binary:
        source = Path(bootstrap_binary)
        if not source.is_file():
            fail(f"bootstrap binary not found: {source}")
        tmp = unique_runtime_temp_path(runtime_dir, f"{binary.name}.bootstrap")
        copy_runtime(source, tmp)
        install_tmp_runtime(tmp, binary, contract, runtime_dir, f"installed bootstrap binary into {binary}")
    elif bootstrap_url:
        runtime_dir.mkdir(parents=True, exist_ok=True)
        tmp = unique_runtime_temp_path(runtime_dir, f"{binary.name}.download")
        try:
            download_file(bootstrap_url, tmp)
            expected = os.environ.get("WS_MCP_BOOTSTRAP_SHA256")
            if expected and sha256_file(tmp) != expected:
                fail("checksum mismatch for downloaded ws-mcp")
            try:
                tmp.chmod(0o755)
            except OSError:
                pass
            installed = install_tmp_runtime(tmp, binary, contract, runtime_dir, f"downloaded runtime binary into {binary}")
        finally:
            tmp.unlink(missing_ok=True)
        if installed:
            try:
                binary.chmod(0o755)
            except OSError:
                pass
    elif install_local_devenv_runtime(plugin_dir, runtime_dir, binary, asset, contract, os_name, platform_name, prefer_build=force_local):
        return
    elif force_local:
        fail("local devenv runtime was forced but no compatible local runtime could be installed")
    else:
        install_downloaded_runtime(binary, runtime_dir, asset, contract)


def runtime_capabilities_compatible(binary: Path, contract: dict) -> bool:
    try:
        proc = run_binary(binary, ["runtime", "capabilities"])
    except Exception as exc:
        note(f"runtime capabilities probe failed: {exc}")
        return False
    if proc.returncode != 0 or not proc.stdout:
        note("runtime capabilities probe unavailable")
        return False
    try:
        payload = json.loads(proc.stdout)
    except Exception:
        note("runtime capabilities probe returned invalid JSON")
        return False
    if not isinstance(payload, dict):
        return False

    if not version_compatible(str(payload.get("version", "")), contract):
        note("runtime capabilities version mismatch")
        return False

    expected_protocol = str(contract.get("mcp_protocol", ""))
    if not expected_protocol or payload.get("mcp_protocol") != expected_protocol:
        note("runtime capabilities MCP protocol mismatch")
        return False

    expected_prompt_hash = contract.get("prompt_bundle", {}).get("content_sha256")
    if expected_prompt_hash:
        prompt_bundle = payload.get("prompt_bundle")
        if not isinstance(prompt_bundle, dict) or prompt_bundle.get("content_sha256") != expected_prompt_hash:
            note("runtime capabilities prompt bundle hash mismatch")
            return False

    exact = _capabilities_match_exact(contract)
    if not _capabilities_match_contract(payload, "tools", runtime_tools(contract), exact=exact):
        return False
    if not _capabilities_match_contract(payload, "commands", runtime_commands(contract), exact=exact):
        return False
    return True

def runtime_fully_compatible(binary: Path, contract: dict, runtime_dir: Path) -> bool:
    if not binary.is_file():
        return False
    if runtime_capabilities_compatible(binary, contract):
        return True
    if _capabilities_match_exact(contract):
        return False
    try:
        proc = run_binary(binary, ["version"])
    except Exception:
        return False
    if proc.returncode != 0 or not version_compatible(proc.stdout.strip(), contract):
        return False
    return tools_compatible(binary, contract, runtime_dir) and commands_compatible(binary, contract) and prompt_bundle_compatible(binary, contract)


def parent_env_value(key: str) -> str:
    proc_environ = Path("/proc") / str(os.getppid()) / "environ"
    if proc_environ.is_file():
        try:
            for item in proc_environ.read_bytes().split(b"\0"):
                prefix = f"{key}=".encode()
                if item.startswith(prefix):
                    return item[len(prefix) :].decode(errors="ignore")
        except OSError:
            pass
    ps = shutil.which("ps")
    if ps:
        try:
            proc = subprocess.run([ps, "eww", "-p", str(os.getppid())], text=True, capture_output=True, timeout=5, check=False)
            for item in proc.stdout.split():
                if item.startswith(f"{key}="):
                    return item.split("=", 1)[1]
        except Exception:
            pass
    return ""


def git_root(candidate: str) -> str:
    if not candidate:
        return ""
    git = shutil.which("git")
    if not git:
        return ""
    try:
        proc = subprocess.run([git, "-C", candidate, "rev-parse", "--show-toplevel"], text=True, capture_output=True, timeout=5, check=False)
    except Exception:
        return ""
    if proc.returncode != 0:
        return ""
    return proc.stdout.strip()


def detect_project_root(plugin_dir: Path) -> None:
    if os.environ.get("WS_MCP_PROJECT_ROOT"):
        return
    candidates = [
        parent_env_value("PWD"),
        parent_env_value("OLDPWD"),
        os.environ.get("PWD", ""),
        os.environ.get("OLDPWD", ""),
        str(Path.cwd()),
    ]
    plugin_resolved = plugin_dir.resolve()
    for candidate in candidates:
        root = git_root(candidate)
        if not root:
            continue
        resolved = Path(root).resolve()
        try:
            resolved.relative_to(plugin_resolved)
            continue
        except ValueError:
            os.environ["WS_MCP_PROJECT_ROOT"] = str(resolved)
            note(f"detected project root: {resolved}")
            return


def main() -> int:
    launcher_path = Path(__file__).resolve()
    plugin_dir = launcher_path.parent.parent
    contract_path = plugin_dir / "runtime.json"
    contract = read_runtime_contract(contract_path)

    os_name = host_os()
    arch_name = host_arch()
    platform_name = f"{os_name}-{arch_name}"
    runtime_dir = Path(os.environ.get("WS_MCP_RUNTIME_DIR", str(plugin_dir / ".runtime" / platform_name)))
    binary_name = runtime_binary_name(contract, contract_path, os_name)
    binary = runtime_dir / binary_name
    asset = f"ws-mcp-{platform_name}{'.exe' if os_name == 'windows' else ''}"

    forced_install = runtime_install_forced(plugin_dir, os_name)
    compatible = False
    if forced_install:
        note("forcing runtime install from bootstrap or local devenv source")
        clear_compatibility_stamp(runtime_dir)
    else:
        compatible = compatibility_stamp_current(binary, contract, contract_path, runtime_dir)
    if not forced_install and not compatible:
        compatible = runtime_fully_compatible(binary, contract, runtime_dir)
        if compatible:
            write_compatibility_stamp(binary, contract, contract_path, runtime_dir)

    if forced_install or not compatible:
        note("installing or repairing incompatible runtime")
        clear_compatibility_stamp(runtime_dir)
        install_runtime(plugin_dir, runtime_dir, binary, asset, contract, os_name, platform_name, force_local=local_devenv_runtime_enabled(plugin_dir, os_name))
        if not runtime_fully_compatible(binary, contract, runtime_dir):
            fail("incompatible ws-mcp runtime after repair")
        write_compatibility_stamp(binary, contract, contract_path, runtime_dir)

    detect_project_root(plugin_dir)
    note(f"plugin_dir={plugin_dir}")
    note(f"runtime_dir={runtime_dir}")
    note(f"cwd={Path.cwd()}")
    note(f"project_root={os.environ.get('WS_MCP_PROJECT_ROOT', '')}")

    os.environ["WS_MCP_RUNTIME_BINARY"] = str(binary)
    args = [str(binary), *sys.argv[1:]]
    if os_name == "windows":
        return subprocess.call(args)
    os.execvpe(str(binary), args, os.environ)
    return 1


if __name__ == "__main__":
    raise SystemExit(main())
