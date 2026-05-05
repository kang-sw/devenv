#!/usr/bin/env python3
import hashlib
import json
import os
import platform as platform_module
import shutil
import subprocess
import sys
import tempfile
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
    return actual[:2] == plugin[:2]


def sha256_file(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as fh:
        for chunk in iter(lambda: fh.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


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


def install_downloaded_runtime(binary: Path, runtime_dir: Path, asset: str, contract: dict) -> None:
    base_url = os.environ.get("WS_MCP_RELEASE_BASE_URL")
    repository = os.environ.get("WS_MCP_RELEASE_REPOSITORY") or contract.get("release_repository")
    tag = os.environ.get("WS_MCP_RELEASE_TAG") or contract.get("release_tag")
    if not base_url:
        if not repository or not tag:
            fail("missing runtime binary; set WS_MCP_BOOTSTRAP_BINARY, WS_MCP_BOOTSTRAP_URL, or release_repository/release_tag in runtime.json")
        base_url = f"https://github.com/{repository}/releases/download/{tag}"

    runtime_dir.mkdir(parents=True, exist_ok=True)
    tmp = runtime_dir / f"{binary.name}.download"
    sums_tmp = runtime_dir / "SHA256SUMS.download"
    for path in (tmp, sums_tmp):
        path.unlink(missing_ok=True)

    download_file(f"{base_url}/{asset}", tmp)
    download_file(f"{base_url}/SHA256SUMS", sums_tmp)
    expected = expected_checksum(sums_tmp, asset)
    actual = sha256_file(tmp)
    if actual != expected:
        fail(f"checksum mismatch for downloaded {asset}")
    os.replace(tmp, binary)
    sums_tmp.unlink(missing_ok=True)
    try:
        binary.chmod(0o755)
    except OSError:
        pass
    note(f"downloaded runtime binary into {binary}")


def copy_runtime(source: Path, destination: Path) -> None:
    destination.parent.mkdir(parents=True, exist_ok=True)
    shutil.copy2(source, destination)
    try:
        destination.chmod(0o755)
    except OSError:
        pass


def install_local_devenv_runtime(plugin_dir: Path, runtime_dir: Path, binary: Path, asset: str, contract: dict, os_name: str, platform_name: str) -> bool:
    home = Path.home()
    try:
        plugin_dir.relative_to(home / ".codex" / "plugins" / "cache" / "kang-sw-devenv" / "ws")
    except ValueError:
        return False
    if not (plugin_dir / ".local-devenv-runtime").is_file() or os_name == "windows":
        return False

    tmp = runtime_dir / f"{binary.name}.local"
    candidates = [
        home / "devenv" / "agents-plugin-tool" / "dist" / asset,
        home / "devenv" / "agents-plugin" / ".runtime" / platform_name / binary.name,
    ]
    for candidate in candidates:
        if candidate.is_file():
            copy_runtime(candidate, tmp)
            if runtime_fully_compatible(tmp, contract, runtime_dir):
                os.replace(tmp, binary)
                note(f"installed local devenv runtime from {candidate}")
                return True
            tmp.unlink(missing_ok=True)
            note(f"local devenv runtime candidate is incompatible: {candidate}")

    tool_dir = home / "devenv" / "agents-plugin-tool"
    if tool_dir.is_dir() and shutil.which("go"):
        proc = subprocess.run(["go", "build", "-o", str(tmp), "./cmd/ws-mcp"], cwd=str(tool_dir), check=False)
        if proc.returncode == 0 and runtime_fully_compatible(tmp, contract, runtime_dir):
            os.replace(tmp, binary)
            note(f"built local devenv runtime from {tool_dir}")
            return True
        tmp.unlink(missing_ok=True)
        note("local devenv build produced incompatible runtime")
    return False


def install_runtime(plugin_dir: Path, runtime_dir: Path, binary: Path, asset: str, contract: dict, os_name: str, platform_name: str) -> None:
    bootstrap_binary = os.environ.get("WS_MCP_BOOTSTRAP_BINARY")
    bootstrap_url = os.environ.get("WS_MCP_BOOTSTRAP_URL")
    if bootstrap_binary:
        source = Path(bootstrap_binary)
        if not source.is_file():
            fail(f"bootstrap binary not found: {source}")
        copy_runtime(source, binary)
        note(f"installed bootstrap binary into {binary}")
    elif bootstrap_url:
        runtime_dir.mkdir(parents=True, exist_ok=True)
        tmp = runtime_dir / f"{binary.name}.download"
        download_file(bootstrap_url, tmp)
        expected = os.environ.get("WS_MCP_BOOTSTRAP_SHA256")
        if expected and sha256_file(tmp) != expected:
            fail("checksum mismatch for downloaded ws-mcp")
        os.replace(tmp, binary)
        try:
            binary.chmod(0o755)
        except OSError:
            pass
        note(f"downloaded runtime binary into {binary}")
    elif install_local_devenv_runtime(plugin_dir, runtime_dir, binary, asset, contract, os_name, platform_name):
        return
    else:
        install_downloaded_runtime(binary, runtime_dir, asset, contract)


def runtime_fully_compatible(binary: Path, contract: dict, runtime_dir: Path) -> bool:
    if not binary.is_file():
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
    contract = read_runtime_contract(plugin_dir / "runtime.json")

    os_name = host_os()
    arch_name = host_arch()
    platform_name = f"{os_name}-{arch_name}"
    runtime_dir = Path(os.environ.get("WS_MCP_RUNTIME_DIR", str(plugin_dir / ".runtime" / platform_name)))
    binary_name = "ws-mcp.exe" if os_name == "windows" else "ws-mcp"
    binary = runtime_dir / binary_name
    asset = f"ws-mcp-{platform_name}{'.exe' if os_name == 'windows' else ''}"

    if not runtime_fully_compatible(binary, contract, runtime_dir):
        note("installing or repairing incompatible runtime")
        install_runtime(plugin_dir, runtime_dir, binary, asset, contract, os_name, platform_name)
    if not runtime_fully_compatible(binary, contract, runtime_dir):
        fail("incompatible ws-mcp runtime after repair")

    detect_project_root(plugin_dir)
    note(f"plugin_dir={plugin_dir}")
    note(f"runtime_dir={runtime_dir}")
    note(f"cwd={Path.cwd()}")
    note(f"project_root={os.environ.get('WS_MCP_PROJECT_ROOT', '')}")

    args = [str(binary), *sys.argv[1:]]
    if os_name == "windows":
        return subprocess.call(args)
    os.execvpe(str(binary), args, os.environ)
    return 1


if __name__ == "__main__":
    raise SystemExit(main())
