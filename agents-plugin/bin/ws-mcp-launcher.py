#!/usr/bin/env python3
import hashlib
import json
import os
import platform as platform_module
import shutil
import subprocess
import sys
import tempfile
import time
import uuid
import urllib.request
from pathlib import Path


_BREADCRUMB_DIR: Path | None = None


def set_breadcrumb_dir(runtime_dir: Path) -> None:
    global _BREADCRUMB_DIR
    _BREADCRUMB_DIR = runtime_dir


def write_launch_breadcrumb(message: str) -> None:
    # Best-effort durable record of why startup failed, so a -32000 connect
    # failure leaves a readable reason in the runtime dir instead of vanishing
    # with the launcher's stderr. Never mask the original failure.
    if _BREADCRUMB_DIR is None:
        return
    try:
        _BREADCRUMB_DIR.mkdir(parents=True, exist_ok=True)
        (_BREADCRUMB_DIR / "last-launch-error").write_text(
            f"{time.strftime('%Y-%m-%dT%H:%M:%S')} ws-mcp-launcher: {message}\n",
            encoding="utf-8",
        )
    except Exception:
        pass


def clear_launch_breadcrumb() -> None:
    if _BREADCRUMB_DIR is None:
        return
    try:
        (_BREADCRUMB_DIR / "last-launch-error").unlink(missing_ok=True)
    except Exception:
        pass


def fail(message: str) -> None:
    print(f"ws-mcp-launcher: {message}", file=sys.stderr)
    write_launch_breadcrumb(message)
    raise SystemExit(1)


def note(message: str) -> None:
    if os.environ.get("WS_MCP_LAUNCHER_DEBUG") == "1":
        print(f"ws-mcp-launcher: {message}", file=sys.stderr)


def wait_for_runtime_contract(path: Path, *, timeout_seconds: float | None = None, interval_seconds: float = 0.05) -> None:
    # Use a longer default on Windows: AV scanners can delay file visibility on
    # cold installs (freshly-extracted packages are held open by the AV service).
    if timeout_seconds is None:
        timeout_seconds = 10.0 if os.name == "nt" else 2.0
    if path.is_file():
        return
    deadline = time.monotonic() + timeout_seconds
    while time.monotonic() < deadline:
        time.sleep(interval_seconds)
        if path.is_file():
            note(f"runtime contract appeared after package materialization wait: {path}")
            return
    fail(f"plugin package not fully materialized: missing runtime contract after {timeout_seconds:.1f}s: {path}")


def read_runtime_contract(path: Path) -> dict:
    wait_for_runtime_contract(path)
    # The file may exist but be momentarily unreadable (AV sharing hold on
    # Windows).  Retry a small number of times on OSError (covers PermissionError)
    # and ValueError (covers json.JSONDecodeError and UnicodeDecodeError — both
    # subclasses of ValueError) before giving up.  A partially-written or
    # byte-corrupt file in the cold-install window can raise UnicodeDecodeError,
    # which is NOT an OSError; catching (OSError, ValueError) ensures every
    # transient read/parse error reaches fail() rather than escaping as a
    # traceback.  A first-try success incurs no added latency.
    _read_attempts = 4
    _read_backoff = 0.05  # seconds; kept short — this is a transient window only
    last_exc: Exception | None = None
    for attempt in range(_read_attempts):
        try:
            return json.loads(path.read_text(encoding="utf-8"))
        except (OSError, ValueError) as exc:
            last_exc = exc
        if attempt < _read_attempts - 1:
            time.sleep(_read_backoff)
    fail(f"invalid runtime contract {path}: {last_exc}")


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


def compatibility_stamp_payload(binary: Path, contract: dict, contract_path: Path, source_fingerprint: str | None = None) -> dict | None:
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
        "local_devenv_source_fingerprint": source_fingerprint,
    }


def compatibility_stamp_current(binary: Path, contract: dict, contract_path: Path, runtime_dir: Path, source_fingerprint: str | None = None) -> bool:
    expected = compatibility_stamp_payload(binary, contract, contract_path, source_fingerprint)
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


def write_compatibility_stamp(binary: Path, contract: dict, contract_path: Path, runtime_dir: Path, source_fingerprint: str | None = None) -> None:
    payload = compatibility_stamp_payload(binary, contract, contract_path, source_fingerprint)
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
    # Bounded retry on OSError: AV scanners and concurrent installers can hold
    # the target file open briefly on Windows, causing a sharing-violation error.
    # Mirrors the Phase A Go-side MoveFileEx bounded retry (~5 attempts, ~10ms
    # exponential backoff).  On POSIX, os.replace is atomic and never raises this
    # error, so the retry loop never triggers and there is zero added latency.
    _replace_attempts = 5
    _replace_backoff = 0.01  # seconds; doubles per attempt
    last_exc: OSError | None = None
    for attempt in range(_replace_attempts):
        try:
            os.replace(tmp, binary)
            note(message)
            return True
        except OSError as exc:
            last_exc = exc
            if attempt < _replace_attempts - 1:
                time.sleep(_replace_backoff * (2 ** attempt))
    # Replace budget exhausted; fall through to the existing compatible-binary
    # fallback before failing hard.
    if runtime_fully_compatible(binary, contract, runtime_dir):
        note(f"using compatible runtime already installed at {binary} after replace failed: {last_exc}")
        return False
    fail(f"failed to install runtime at {binary}: {last_exc}")


def local_devenv_cache_package(plugin_dir: Path) -> str | None:
    # Recognize the repo-local plugin install regardless of on-disk layout.
    # Claude Code runs a directory-source marketplace plugin from the install.sh
    # snapshot (~/.claude/plugins/ws-plugin/<pkg>), while Codex/Claude package
    # installs live under <host>/plugins/cache/kang-sw-devenv/<pkg>/<version>.
    # Match the ws/wsflow package segment under any per-user (.codex/.claude)
    # plugin tree; the gitignored .local-devenv-runtime marker is the actual
    # dev opt-in, validated separately. This is HOME-independent.
    parts = plugin_dir.resolve().parts
    if "plugins" not in parts:
        return None
    if not any(host in parts for host in (".codex", ".claude")):
        return None
    for seg in parts[parts.index("plugins") + 1:]:
        if seg in {"ws", "wsflow"}:
            return seg
    return None


def read_local_devenv_contract(plugin_dir: Path, os_name: str) -> dict | None:
    if local_devenv_cache_package(plugin_dir) is None:
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
    # On Windows os.access(..., X_OK) is effectively meaningless (it returns True
    # for any existing file), so require the file to exist and trust the marker's
    # absolute go path; on POSIX keep the executable-bit check.
    go_path = resolved["go"]
    if not go_path.is_file() or (os_name != "windows" and not os.access(go_path, os.X_OK)):
        note("local devenv runtime contract is inactive: go is not executable")
        return None
    return resolved


def local_devenv_runtime_enabled(plugin_dir: Path, os_name: str) -> bool:
    return read_local_devenv_contract(plugin_dir, os_name) is not None


def bootstrap_runtime_forced() -> bool:
    return bool(os.environ.get("WS_MCP_BOOTSTRAP_BINARY") or os.environ.get("WS_MCP_BOOTSTRAP_URL"))


def runtime_install_forced(plugin_dir: Path, os_name: str, *, local_enabled: bool | None = None) -> bool:
    if bootstrap_runtime_forced():
        return True
    if local_enabled is None:
        local_enabled = local_devenv_runtime_enabled(plugin_dir, os_name)
    return local_enabled


def local_devenv_source_fingerprint(plugin_dir: Path, os_name: str) -> str | None:
    # Fingerprint the local ws-mcp source tree so an unchanged tree can reuse the
    # cached runtime binary instead of forcing a `go build` on every launch (the
    # forced rebuild blew past the MCP startup timeout on cold caches). Uses stat
    # metadata only (no file reads) to stay cheap on the startup path.
    contract = read_local_devenv_contract(plugin_dir, os_name)
    if contract is None:
        return None
    tool_dir = contract["tool_dir"]
    entries = []
    try:
        for path in tool_dir.rglob("*.go"):
            try:
                st = path.stat()
            except OSError:
                continue
            entries.append((str(path.relative_to(tool_dir)), st.st_size, st.st_mtime_ns))
    except OSError as exc:
        note(f"local devenv source fingerprint walk failed: {exc}")
        return None
    for name in ("go.mod", "go.sum"):
        candidate = tool_dir / name
        try:
            st = candidate.stat()
        except OSError:
            continue
        entries.append((name, st.st_size, st.st_mtime_ns))
    try:
        go_st = contract["go"].stat()
        entries.append(("::go-toolchain::", go_st.st_size, go_st.st_mtime_ns))
    except OSError:
        pass
    digest = hashlib.sha256()
    for rel, size, mtime in sorted(entries):
        digest.update(f"{rel}\x00{size}\x00{mtime}\x00".encode("utf-8"))
    return digest.hexdigest()


def local_devenv_build_env(os_name: str) -> dict:
    # The MCP host may launch the launcher with a sanitized environment that
    # lacks HOME (observed on Claude Code launches). `go build` then cannot
    # locate GOMODCACHE/GOCACHE (default under $HOME) and fails, aborting the
    # forced local repair. Recover HOME from the password database via
    # Path.home() so the source build finds the user's module/build cache.
    build_env = dict(os.environ)
    if not build_env.get("HOME"):
        try:
            build_env["HOME"] = str(Path.home())
        except Exception:
            pass
    if os_name == "windows":
        # Windows `go build` resolves GOMODCACHE under %USERPROFILE%\go and
        # GOCACHE under %LOCALAPPDATA%\go-build; recover them when the launch
        # environment was sanitized, mirroring the HOME recovery above.
        if not build_env.get("USERPROFILE"):
            try:
                build_env["USERPROFILE"] = str(Path.home())
            except Exception:
                pass
        if not build_env.get("LOCALAPPDATA"):
            profile = build_env.get("USERPROFILE")
            if profile:
                build_env["LOCALAPPDATA"] = str(Path(profile) / "AppData" / "Local")
    return build_env


def build_local_devenv_runtime(runtime_dir: Path, binary: Path, contract: dict, local_contract: dict, os_name: str) -> bool:
    tool_dir = local_contract["tool_dir"]
    go_binary = local_contract["go"]
    tmp = unique_runtime_temp_path(runtime_dir, f"{binary.name}.local")
    proc = subprocess.run(
        [str(go_binary), "build", "-o", str(tmp), "./cmd/ws-mcp"],
        cwd=str(tool_dir),
        env=local_devenv_build_env(os_name),
        check=False,
    )
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
        return build_local_devenv_runtime(runtime_dir, binary, contract, local_contract, os_name)

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

    return build_local_devenv_runtime(runtime_dir, binary, contract, local_contract, os_name)


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
    return tools_compatible(binary, contract, runtime_dir) and commands_compatible(binary, contract)


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


def wait_for_rsrc_tree(plugin_dir: Path, *, timeout_seconds: float = 5.0, interval_seconds: float = 0.05) -> None:
    """Wait for the rsrc tree to be materialized before the one-shot apply_rsrc_root_env check.

    Uses manifest.json as the presence sentinel: it is written last during rsrc
    tree extraction and proves the tree is populated, not merely an empty dir.
    On timeout, emits a note and returns — apply_rsrc_root_env already no-ops
    gracefully when the rsrc dir is absent, so this wait is best-effort.
    """
    sentinel = plugin_dir / "rsrc" / "manifest.json"
    if sentinel.is_file():
        return
    deadline = time.monotonic() + timeout_seconds
    while time.monotonic() < deadline:
        time.sleep(interval_seconds)
        if sentinel.is_file():
            note(f"rsrc tree materialized after wait: {sentinel}")
            return
    note(f"rsrc tree sentinel not found after {timeout_seconds:.1f}s; proceeding without rsrc env (best-effort): {sentinel}")


def apply_rsrc_root_env(plugin_dir: Path, env: dict) -> None:
    """Point the runtime at the staged rsrc tree through its WS_RSRC_ROOT seam.

    The runtime derives the rsrc tree as <dir(exe)>/../rsrc, but the launcher
    installs the binary under <plugin>/.runtime/<platform>/ -- two levels below
    the plugin root where the rsrc/ tree is staged -- so the derived path misses
    <plugin>/rsrc. Set the seam unless the caller already provided it.
    """
    rsrc_root = plugin_dir / "rsrc"
    if rsrc_root.is_dir() and not env.get("WS_RSRC_ROOT"):
        env["WS_RSRC_ROOT"] = str(rsrc_root)


def apply_skills_root_env(plugin_dir: Path, env: dict) -> None:
    """Point the runtime at the staged skills tree through WS_SKILLS_ROOT.

    The runtime binary lives under <plugin>/.runtime/<platform>/, so its
    executable-relative fallback resolves to <plugin>/.runtime/skills instead
    of the plugin-root <plugin>/skills tree. Set the seam unless the caller
    already provided it.
    """
    skills_root = plugin_dir / "skills"
    if skills_root.is_dir() and not env.get("WS_SKILLS_ROOT"):
        env["WS_SKILLS_ROOT"] = str(skills_root)


def main() -> int:
    launcher_path = Path(__file__).resolve()
    plugin_dir = launcher_path.parent.parent
    contract_path = plugin_dir / "runtime.json"
    contract = read_runtime_contract(contract_path)

    os_name = host_os()
    arch_name = host_arch()
    platform_name = f"{os_name}-{arch_name}"
    runtime_dir = Path(os.environ.get("WS_MCP_RUNTIME_DIR", str(plugin_dir / ".runtime" / platform_name)))
    set_breadcrumb_dir(runtime_dir)
    binary_name = runtime_binary_name(contract, contract_path, os_name)
    binary = runtime_dir / binary_name
    asset = f"ws-mcp-{platform_name}{'.exe' if os_name == 'windows' else ''}"

    bootstrap_forced = bootstrap_runtime_forced()
    local_enabled = local_devenv_runtime_enabled(plugin_dir, os_name)
    install_forced = runtime_install_forced(plugin_dir, os_name, local_enabled=local_enabled)
    # The stamp encodes the local source fingerprint, so a stamp hit under local
    # devenv proves the cached binary already matches the current source: skip the
    # rebuild. A miss means the source changed (or no runtime exists) -> rebuild.
    source_fingerprint = local_devenv_source_fingerprint(plugin_dir, os_name) if local_enabled else None

    need_install = False
    if bootstrap_forced:
        note("forcing runtime install from bootstrap binary or url")
        clear_compatibility_stamp(runtime_dir)
        need_install = True
    elif compatibility_stamp_current(binary, contract, contract_path, runtime_dir, source_fingerprint):
        pass
    elif install_forced and local_enabled:
        note("local devenv source changed or runtime missing; rebuilding from source")
        need_install = True
    elif runtime_fully_compatible(binary, contract, runtime_dir):
        write_compatibility_stamp(binary, contract, contract_path, runtime_dir, source_fingerprint)
    else:
        need_install = True

    if need_install:
        note("installing or repairing incompatible runtime")
        clear_compatibility_stamp(runtime_dir)
        install_runtime(plugin_dir, runtime_dir, binary, asset, contract, os_name, platform_name, force_local=local_enabled)
        if not runtime_fully_compatible(binary, contract, runtime_dir):
            fail("incompatible ws-mcp runtime after repair")
        write_compatibility_stamp(binary, contract, contract_path, runtime_dir, source_fingerprint)

    # Runtime is present and compatible; clear any stale failure breadcrumb so
    # last-launch-error only exists when the most recent launch actually failed.
    clear_launch_breadcrumb()
    detect_project_root(plugin_dir)
    note(f"plugin_dir={plugin_dir}")
    note(f"runtime_dir={runtime_dir}")
    note(f"cwd={Path.cwd()}")
    note(f"project_root={os.environ.get('WS_MCP_PROJECT_ROOT', '')}")

    os.environ["WS_MCP_RUNTIME_BINARY"] = str(binary)
    wait_for_rsrc_tree(plugin_dir)
    apply_rsrc_root_env(plugin_dir, os.environ)
    apply_skills_root_env(plugin_dir, os.environ)
    args = [str(binary), *sys.argv[1:]]
    if os_name == "windows":
        return subprocess.call(args)
    os.execvpe(str(binary), args, os.environ)
    return 1


if __name__ == "__main__":
    raise SystemExit(main())
