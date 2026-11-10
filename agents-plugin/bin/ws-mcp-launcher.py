#!/usr/bin/env python3
import hashlib
import json
import os
import platform as platform_module
import shutil
import subprocess
import sys
import tempfile
import threading
import time
import uuid
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


def write_exit_breadcrumb(exit_code: int) -> None:
    # Best-effort durable record of a mid-session abnormal child exit, so a
    # Windows-side crash leaves a readable reason in the runtime dir. This
    # complements (never overwrites) last-launch-error, which only reflects
    # startup failures.
    if _BREADCRUMB_DIR is None:
        return
    try:
        _BREADCRUMB_DIR.mkdir(parents=True, exist_ok=True)
        (_BREADCRUMB_DIR / "last-abnormal-exit").write_text(
            f"{time.strftime('%Y-%m-%dT%H:%M:%S')} ws-mcp-launcher: child exited abnormally with code {exit_code}\n",
            encoding="utf-8",
        )
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


def version_compatible(version: str, contract: dict, *, allow_same_minor: bool = False) -> bool:
    actual = version_tuple(version)
    plugin = version_tuple(str(contract.get("plugin_version", "")))
    if actual is None or plugin is None:
        return False
    if actual == plugin:
        return True
    # Local-devenv builds run the live source, whose version routinely drifts a
    # few patches ahead of the installed plugin snapshot's contract between
    # `install.sh update` runs. Requiring an exact patch match there breaks the
    # MCP server on every local bump; accept same-minor drift so the dev loop
    # survives a bump without a re-snapshot. Released/downloaded runtimes keep
    # the strict exact-match check (allow_same_minor stays False for them).
    if allow_same_minor and actual[0] == plugin[0] and actual[1] == plugin[1]:
        return True
    return False


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
    import urllib.request

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


def install_tmp_runtime(tmp: Path, binary: Path, contract: dict, runtime_dir: Path, message: str, *, allow_same_minor: bool = False) -> bool:
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
    if runtime_fully_compatible(binary, contract, runtime_dir, allow_same_minor=allow_same_minor):
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
    if proc.returncode == 0 and runtime_fully_compatible(tmp, contract, runtime_dir, allow_same_minor=True):
        install_tmp_runtime(tmp, binary, contract, runtime_dir, f"built local devenv runtime from {tool_dir}", allow_same_minor=True)
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
            if runtime_fully_compatible(tmp, contract, runtime_dir, allow_same_minor=True):
                install_tmp_runtime(tmp, binary, contract, runtime_dir, f"installed local devenv runtime from {candidate}", allow_same_minor=True)
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
        try:
            copy_runtime(source, tmp)
            install_tmp_runtime(tmp, binary, contract, runtime_dir, f"installed bootstrap binary into {binary}")
        finally:
            # A compatible running binary may survive a failed Windows replace;
            # do not retain one full bootstrap copy per mailbox wait re-arm.
            tmp.unlink(missing_ok=True)
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


def runtime_capabilities_compatible(binary: Path, contract: dict, *, allow_same_minor: bool = False) -> bool:
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

    if not version_compatible(str(payload.get("version", "")), contract, allow_same_minor=allow_same_minor):
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

def runtime_fully_compatible(binary: Path, contract: dict, runtime_dir: Path, *, allow_same_minor: bool = False) -> bool:
    if not binary.is_file():
        return False
    if runtime_capabilities_compatible(binary, contract, allow_same_minor=allow_same_minor):
        return True
    if _capabilities_match_exact(contract):
        return False
    try:
        proc = run_binary(binary, ["version"])
    except Exception:
        return False
    if proc.returncode != 0 or not version_compatible(proc.stdout.strip(), contract, allow_same_minor=allow_same_minor):
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


# ---------------------------------------------------------------------------
# Daemon-hosted MCP: the launcher as a proxy.
#
# `serve --stdio` and `mailbox wait` do not run the runtime binary. They
# connect to the machine's dashboard daemon (one per cache root), which hosts
# one MCP server per connection built from the hello line below, and copy
# bytes between stdin/stdout and that connection. The daemon runs from a
# content-hash-named copy of the runtime binary under the daemon's bin/
# directory (the name wsdashboard.StageExecutable produces), so on this path
# the only ws-mcp executable that runs is that staged copy, once per version.
# When no daemon can be reached within the budgets below, or the daemon is in
# another minor, the launcher runs the direct path (the runtime binary itself)
# and says so once on stderr. Wire format: ai-docs/manuals/ws-mcp.md.
# ---------------------------------------------------------------------------

DEFAULT_CACHE_DIR_NAME = "ws@kang-sw-devenv"  # wsstate.defaultCacheDirName
DAEMON_DIR_NAME = "dashboard"  # wsdashboard.dirName
DAEMON_ENDPOINT_FILE = "endpoint.json"
DAEMON_BIN_DIR_NAME = "bin"
PROXY_PATH = "/mcp"
PROXY_UPGRADE = "ws-mcp"
PROXY_PROTOCOL = 1
PROXY_REPLAY_ID_PREFIX = "ws-mcp-launcher-replay-"
DAEMON_DEFAULT_START_WAIT = 10.0  # wsdashboard.DefaultStartWait, seconds
PROXY_LOST_REQUEST_CODE = -32000


class ProxyFallback(Exception):
    """The daemon path is not available; the caller runs the direct path."""


def proxy_cache_root(env: dict) -> Path:
    """The cache root wsstate.CacheRoot resolves for env.

    An override is made absolute and symlink-resolved when it exists, as Go's
    filepath.Abs + EvalSymlinks does; the default is the home cache dir as is.
    """
    override = env.get("WS_CACHE_HOME", "")
    if override:
        absolute = os.path.abspath(override)
        try:
            return Path(os.path.realpath(absolute, strict=True))
        except OSError:
            return Path(os.path.normpath(absolute))
    return Path(os.path.expanduser("~")) / ".cache" / DEFAULT_CACHE_DIR_NAME


def proxy_daemon_dir(env: dict) -> Path:
    return proxy_cache_root(env) / DAEMON_DIR_NAME


def staged_daemon_name(binary: Path, digest: str | None = None) -> str:
    """The name wsdashboard.StageExecutable gives binary's staged copy."""
    if digest is None:
        digest = sha256_file(binary)
    return f"ws-mcp-dashboard-{digest[:16]}{binary.suffix.lower()}"


def stage_daemon_binary(binary: Path, bin_dir: Path) -> Path:
    """Copy binary into bin_dir under its staged name, as StageExecutable does.

    The copy is written beside its target and renamed into place, so a
    concurrent starter never runs a half-written file; every other entry of
    bin_dir is removed best-effort (a running older daemon keeps its unlinked
    executable on POSIX; on Windows the removal of a running one fails and is
    ignored).
    """
    digest = sha256_file(binary)
    name = staged_daemon_name(binary, digest)
    bin_dir.mkdir(parents=True, exist_ok=True)
    target = bin_dir / name
    current = False
    try:
        current = target.is_file() and sha256_file(target) == digest
    except OSError:
        current = False
    if not current:
        tmp = bin_dir / f".{name}.{os.getpid()}.{uuid.uuid4().hex}.tmp"
        shutil.copyfile(binary, tmp)
        os.chmod(tmp, 0o755)
        os.replace(tmp, target)
    try:
        for entry in bin_dir.iterdir():
            if entry.name == name:
                continue
            try:
                if entry.is_dir() and not entry.is_symlink():
                    shutil.rmtree(entry, ignore_errors=True)
                else:
                    entry.unlink()
            except OSError:
                pass
    except OSError:
        pass
    return target


def parse_daemon_version(value: str) -> tuple[tuple[int, int, int], str] | None:
    """wsdashboard.parseVersion: a core triple and a pre-release suffix."""
    core = value.strip()
    if core.startswith("v"):
        core = core[1:]
    pre = ""
    if "-" in core:
        core, pre = core.split("-", 1)
    parts = core.split(".")
    if len(parts) != 3:
        return None
    try:
        numbers = tuple(int(part) for part in parts)
    except ValueError:
        return None
    if any(number < 0 for number in numbers):
        return None
    return numbers, pre  # type: ignore[return-value]


def compare_daemon_versions(a: str, b: str) -> int:
    """wsdashboard.CompareVersions: a pre-release suffix sorts after none."""
    pa = parse_daemon_version(a)
    pb = parse_daemon_version(b)
    if pa is None and pb is None:
        return 0
    if pa is None:
        return -1
    if pb is None:
        return 1
    if pa[0] != pb[0]:
        return -1 if pa[0] < pb[0] else 1
    if pa[1] == pb[1]:
        return 0
    if pa[1] == "":
        return -1
    if pb[1] == "":
        return 1
    return -1 if pa[1] < pb[1] else 1


def same_minor(a: str, b: str) -> bool:
    pa = parse_daemon_version(a)
    pb = parse_daemon_version(b)
    return pa is not None and pb is not None and pa[0][:2] == pb[0][:2]


def read_daemon_endpoint(daemon_dir: Path) -> dict | None:
    try:
        payload = json.loads((daemon_dir / DAEMON_ENDPOINT_FILE).read_text(encoding="utf-8"))
    except (OSError, ValueError):
        return None
    if not isinstance(payload, dict):
        return None
    port = payload.get("port")
    token = payload.get("token")
    if not isinstance(port, int) or port <= 0 or not isinstance(token, str) or not token:
        return None
    return payload


def daemon_health(port: int, timeout: float) -> dict | None:
    """GET /health on the daemon's port; None when nothing compatible answers."""
    import http.client

    conn = http.client.HTTPConnection("127.0.0.1", port, timeout=timeout)
    try:
        conn.request("GET", "/health")
        resp = conn.getresponse()
        body = resp.read(64 * 1024)
        if resp.status != 200:
            return None
        payload = json.loads(body)
    except (OSError, ValueError, http.client.HTTPException):
        return None
    finally:
        conn.close()
    if not isinstance(payload, dict) or not isinstance(payload.get("version"), str):
        return None
    return payload


# Environment variables whose values are paths the hosted server reads. A
# harness may set them relative to the launcher's cwd (Codex's .mcp.json
# names "./config-manifest.json" with cwd "."); the daemon resolves relative
# paths against its own cwd, so the hello carries them absolute.
HELLO_PATH_ENV_KEYS = (
    "WS_CACHE_HOME",
    "WS_CONFIG_HOME",
    "WS_RSRC_ROOT",
    "WS_SKILLS_ROOT",
    "WS_MCP_PROJECT_ROOT",
    "WS_WORKTREE_DIR",
    "WS_MCP_DEBUG_LOG",
    "WS_MCP_RUNTIME_BINARY",
    "WS_MCP_RUNTIME_DIR",
)
# A list of manifest paths, split on ";" and the platform separator as the
# server splits it, rejoined with ";" (accepted everywhere).
HELLO_PATH_LIST_ENV_KEYS = ("WS_MCP_CONFIG_MANIFESTS",)


def hello_env(env: dict, cwd: str) -> dict:
    """env for the hello, path-valued entries made absolute against cwd."""
    out = dict(env)
    for key in HELLO_PATH_ENV_KEYS:
        value = out.get(key, "")
        if value.strip():
            out[key] = os.path.normpath(os.path.join(cwd, value.strip()))
    for key in HELLO_PATH_LIST_ENV_KEYS:
        value = out.get(key, "")
        if not value.strip():
            continue
        parts = []
        for part in value.split(";"):
            for item in (part.split(os.pathsep) if os.pathsep != ";" else [part]):
                if item.strip():
                    parts.append(os.path.normpath(os.path.join(cwd, item.strip())))
        out[key] = ";".join(parts)
    return out


def proxy_hello(mode: str, argv: list[str], env: dict, runtime_version: str) -> dict:
    """The first line on the stream: this connection's process context."""
    cwd = os.getcwd()
    env = hello_env(env, cwd)
    return {
        "protocol": PROXY_PROTOCOL,
        "mode": mode,
        "argv": list(argv),
        "cwd": cwd,
        "project_root": env.get("WS_MCP_PROJECT_ROOT", ""),
        "env": env,
        "harness": env.get("WS_MCP_HARNESS", ""),
        "owner_pid": os.getpid(),
        "version": runtime_version,
    }


class SocketStream:
    """A newline-delimited stream over the upgraded daemon connection."""

    def __init__(self, sock, reader) -> None:
        self.sock = sock
        self.reader = reader

    def read_line(self) -> bytes:
        try:
            return self.reader.readline()
        except (OSError, ValueError):
            return b""

    def write(self, data: bytes) -> bool:
        try:
            self.sock.sendall(data)
            return True
        except (OSError, ValueError):
            return False

    def close(self) -> None:
        import socket as socket_module

        try:
            self.sock.shutdown(socket_module.SHUT_RDWR)
        except OSError:
            pass
        for closer in (self.reader.close, self.sock.close):
            try:
                closer()
            except OSError:
                pass

    def wait(self) -> int:
        return 0


class ProcessStream:
    """The same stream shape over a direct `serve --stdio` child."""

    def __init__(self, argv: list[str], env: dict) -> None:
        self.proc = subprocess.Popen(argv, stdin=subprocess.PIPE, stdout=subprocess.PIPE, env=env)

    def read_line(self) -> bytes:
        try:
            return self.proc.stdout.readline()
        except (OSError, ValueError):
            return b""

    def write(self, data: bytes) -> bool:
        try:
            self.proc.stdin.write(data)
            self.proc.stdin.flush()
            return True
        except (OSError, ValueError):
            return False

    def close(self) -> None:
        try:
            self.proc.stdin.close()
        except OSError:
            pass

    def wait(self) -> int:
        try:
            return self.proc.wait(timeout=10)
        except subprocess.TimeoutExpired:
            self.proc.kill()
            return self.proc.wait()


def daemon_upgrade(endpoint: dict, hello: dict, timeout: float) -> SocketStream:
    """Open the MCP route: HTTP Upgrade, the hello line, the accept line."""
    import socket as socket_module

    port = int(endpoint["port"])
    try:
        sock = socket_module.create_connection(("127.0.0.1", port), timeout=timeout)
    except OSError as exc:
        raise ProxyFallback(f"daemon connection failed: {exc}") from exc
    try:
        request = (
            f"GET {PROXY_PATH} HTTP/1.1\r\n"
            f"Host: 127.0.0.1:{port}\r\n"
            f"Upgrade: {PROXY_UPGRADE}\r\n"
            "Connection: Upgrade\r\n"
            f"Authorization: Bearer {endpoint['token']}\r\n"
            "\r\n"
        ).encode("ascii")
        sock.sendall(request)
        reader = sock.makefile("rb")
        status = reader.readline().decode("latin-1").strip()
        while True:
            header = reader.readline()
            if not header or header == b"\r\n":
                break
        if " 101 " not in f" {status} ":
            raise ProxyFallback(f"daemon refused the MCP route: {status or 'no answer'}")
        sock.sendall(json.dumps(hello, separators=(",", ":")).encode("utf-8") + b"\n")
        raw = reader.readline()
        try:
            accept = json.loads(raw)
        except ValueError:
            accept = None
        if not isinstance(accept, dict):
            raise ProxyFallback("daemon closed the MCP route before answering the hello")
        if not accept.get("ok"):
            raise ProxyFallback(f"daemon refused this session: {accept.get('reason') or 'no reason given'}")
        sock.settimeout(None)
        return SocketStream(sock, reader)
    except (OSError, ValueError) as exc:
        sock.close()
        raise ProxyFallback(f"daemon connection failed: {exc}") from exc
    except ProxyFallback:
        sock.close()
        raise


class DaemonLink:
    """Finds, starts, and connects to the daemon for one cache root."""

    def __init__(
        self,
        binary: Path,
        runtime_version: str,
        os_name: str,
        env: dict,
        *,
        answer_budget: float | None = None,
        start_budget: float | None = None,
    ) -> None:
        self.binary = binary
        self.runtime_version = runtime_version
        self.os_name = os_name
        self.env = env
        self.daemon_dir = proxy_daemon_dir(env)
        extra = 10.0 if os_name == "windows" else 5.0
        # A running daemon that does not answer sends the launcher to the
        # direct path after this long; a start, replacement, or reconnect
        # waits at least DefaultStartWait plus this long, so a draining
        # predecessor never pushes the launcher onto the direct path.
        self.answer_budget = extra if answer_budget is None else answer_budget
        self.start_budget = DAEMON_DEFAULT_START_WAIT + extra if start_budget is None else start_budget
        try:
            self.build_time = binary.stat().st_mtime_ns // 1_000_000
        except OSError:
            self.build_time = 0
        self.started = False

    def probe(self, timeout: float) -> tuple[dict | None, dict | None]:
        endpoint = read_daemon_endpoint(self.daemon_dir)
        if endpoint is None:
            return None, None
        return endpoint, daemon_health(int(endpoint["port"]), timeout)

    def wait_healthy(self, budget: float) -> tuple[dict | None, dict | None]:
        deadline = time.monotonic() + budget
        while True:
            endpoint, health = self.probe(min(1.0, max(0.1, deadline - time.monotonic())))
            if health is not None:
                return endpoint, health
            if time.monotonic() >= deadline:
                return endpoint, None
            time.sleep(0.2)

    def needs_start(self, health: dict) -> bool:
        """Older daemon, or the same version from an older build."""
        order = compare_daemon_versions(str(health.get("version", "")), self.runtime_version)
        if order != 0:
            return order < 0
        build_time = health.get("build_time")
        return isinstance(build_time, int) and 0 < self.build_time and build_time < self.build_time

    def start_daemon(self) -> bool:
        """Run the staged binary's daemon-start entry; True when it reports a daemon."""
        self.started = True
        try:
            staged = stage_daemon_binary(self.binary, self.daemon_dir / DAEMON_BIN_DIR_NAME)
        except OSError as exc:
            note(f"staging the daemon binary failed: {exc}")
            return False
        argv = [str(staged), "daemon-start", "--build-time", str(self.build_time)]
        note(f"starting the daemon: {' '.join(argv)}")
        try:
            proc = subprocess.run(
                argv,
                env=self.env,
                stdin=subprocess.DEVNULL,
                capture_output=True,
                text=True,
                timeout=2 * DAEMON_DEFAULT_START_WAIT + self.start_budget,
                check=False,
            )
        except (OSError, subprocess.SubprocessError) as exc:
            note(f"daemon-start failed: {exc}")
            return False
        for line in proc.stderr.splitlines():
            note(f"daemon-start: {line}")
        if proc.returncode != 0:
            note(f"daemon-start exited {proc.returncode}: {proc.stdout.strip()}")
            return False
        return True

    def connect(self, hello: dict, *, grace: float = 0.0) -> SocketStream:
        """Connect to a compatible daemon, starting or replacing one when needed.

        grace is how long a daemon that answers nothing is given before one is
        started: a reconnect under a replacement gives the successor a moment.
        """
        endpoint, health = self.wait_healthy(grace) if grace > 0 else self.probe(self.answer_budget)
        if health is None or self.needs_start(health):
            if not self.start_daemon():
                raise ProxyFallback("no daemon could be started")
            endpoint, health = self.wait_healthy(self.start_budget)
            if health is None or endpoint is None:
                raise ProxyFallback(f"no daemon answered within {self.start_budget:.0f}s of starting one")
        version = str(health.get("version", ""))
        if not same_minor(version, self.runtime_version):
            raise ProxyFallback(f"daemon {version} is outside this plugin's minor ({self.runtime_version})")
        return daemon_upgrade(endpoint, hello, self.answer_budget)


def json_id_key(value) -> str:
    return json.dumps(value, sort_keys=True, separators=(",", ":"))


def parse_jsonrpc_line(line: bytes) -> dict | None:
    try:
        message = json.loads(line)
    except ValueError:
        return None
    return message if isinstance(message, dict) else None


class McpProxy:
    """Copies MCP traffic between the client (stdin/stdout) and the stream.

    When the stream ends while the client is still connected (the daemon was
    replaced or died), the proxy reconnects, replays the client's `initialize`
    under an id of its own (dropping that answer) and
    `notifications/initialized`, and answers every request still in flight
    with an error, so the client never sees the server go away. When no
    daemon comes back, a direct `serve --stdio` child takes over the same way.
    """

    def __init__(self, link: DaemonLink, hello: dict, direct_argv: list[str], stdin, stdout, stderr) -> None:
        self.link = link
        self.hello = hello
        self.direct_argv = direct_argv
        self.stdin = stdin
        self.stdout = stdout
        self.stderr = stderr
        self.lock = threading.Lock()
        self.stream = None
        self.initialize: dict | None = None
        self.initialized: bytes | None = None
        self.inflight: dict[str, object] = {}
        self.replay_key: str | None = None
        self.replays = 0
        self.stdin_closed = False

    def run(self, stream) -> int:
        self.stream = stream
        pump = threading.Thread(target=self.pump_client, name="ws-mcp-launcher-stdin", daemon=True)
        pump.start()
        exit_code = 0
        while True:
            line = stream.read_line()
            if not line:
                if self.stdin_closed:
                    break
                successor = self.recover()
                if successor is None:
                    exit_code = 1
                    break
                stream = successor
                continue
            if self.deliver(line):
                try:
                    self.stdout.write(line if line.endswith(b"\n") else line + b"\n")
                    self.stdout.flush()
                except (OSError, ValueError):
                    break
        with self.lock:
            final = self.stream
        final.close()
        code = final.wait()
        return code if exit_code == 0 else exit_code

    def pump_client(self) -> None:
        while True:
            try:
                line = self.stdin.readline()
            except (OSError, ValueError):
                line = b""
            if not line:
                break
            message = parse_jsonrpc_line(line)
            with self.lock:
                if message is not None:
                    method = message.get("method")
                    if method == "initialize" and "id" in message:
                        self.initialize = message
                    elif method == "notifications/initialized":
                        self.initialized = line
                    if method is not None and "id" in message:
                        self.inflight[json_id_key(message["id"])] = message["id"]
                stream = self.stream
            stream.write(line)
        with self.lock:
            self.stdin_closed = True
            stream = self.stream
        stream.close()

    def deliver(self, line: bytes) -> bool:
        """Bookkeeping for a server line; False drops it."""
        message = parse_jsonrpc_line(line)
        if message is None or "id" not in message or "method" in message:
            return True
        key = json_id_key(message["id"])
        with self.lock:
            if key == self.replay_key:
                self.replay_key = None
                return False
            self.inflight.pop(key, None)
        return True

    def recover(self):
        with self.lock:
            old = self.stream
        if isinstance(old, ProcessStream):
            return None
        old.close()
        try:
            stream = self.link.connect(self.hello, grace=1.5)
            note("daemon connection lost; reconnected")
        except ProxyFallback as exc:
            print(f"ws-mcp-launcher: daemon connection lost and not recovered ({exc}); continuing on the direct path", file=self.stderr, flush=True)
            try:
                stream = ProcessStream(self.direct_argv, self.link.env)
            except OSError as exc2:
                print(f"ws-mcp-launcher: direct path failed: {exc2}", file=self.stderr, flush=True)
                return None
        self.replay_into(stream)
        return stream

    def replay_into(self, stream) -> None:
        with self.lock:
            self.stream = stream
            if self.stdin_closed:
                # The client left while we reconnected: nobody will close
                # this stream for us, so end the session now.
                self.inflight.clear()
                stream.close()
                return
            lost = list(self.inflight.values())
            self.inflight.clear()
            if self.initialize is not None:
                self.replays += 1
                replay = dict(self.initialize)
                replay["id"] = f"{PROXY_REPLAY_ID_PREFIX}{self.replays}"
                self.replay_key = json_id_key(replay["id"])
                stream.write(json.dumps(replay, separators=(",", ":")).encode("utf-8") + b"\n")
                if self.initialized is not None:
                    stream.write(self.initialized)
        for request_id in lost:
            error = {
                "jsonrpc": "2.0",
                "id": request_id,
                "error": {"code": PROXY_LOST_REQUEST_CODE, "message": "ws-mcp daemon connection was replaced; the request did not complete"},
            }
            try:
                self.stdout.write(json.dumps(error, separators=(",", ":")).encode("utf-8") + b"\n")
                self.stdout.flush()
            except (OSError, ValueError):
                return


def watch_parent_exit(on_exit) -> None:
    """Call on_exit when the parent process ends (Windows; POSIX relies on signals).

    Mirrors the runtime's parent-death watch for `mailbox wait`: without it a
    wait whose caller is gone would hold its daemon session until its timeout.
    """
    if os.name != "nt":
        return
    try:
        import _winapi
    except ImportError:
        return
    ppid = os.getppid()
    if ppid <= 0:
        return

    def run() -> None:
        try:
            handle = _winapi.OpenProcess(getattr(_winapi, "SYNCHRONIZE", 0x00100000), False, ppid)
        except OSError:
            return
        try:
            if _winapi.WaitForSingleObject(handle, 0) == _winapi.WAIT_OBJECT_0:
                return
            if _winapi.WaitForSingleObject(handle, _winapi.INFINITE) == _winapi.WAIT_OBJECT_0:
                on_exit()
        except OSError:
            return
        finally:
            _winapi.CloseHandle(handle)

    threading.Thread(target=run, name="ws-mcp-launcher-parent-watch", daemon=True).start()


def proxy_mode(argv: list[str]) -> str | None:
    """Which argv the daemon hosts: `serve --stdio` as MCP, `mailbox wait` as CLI."""
    if argv == ["serve", "--stdio"]:
        return "mcp"
    if argv[:2] == ["mailbox", "wait"]:
        return "cli"
    return None


def proxy_serve(link: DaemonLink, argv: list[str], stdin, stdout, stderr) -> int | None:
    """Serve argv through the daemon; None means run the direct path instead."""
    hello = proxy_hello("mcp", argv, link.env, link.runtime_version)
    try:
        stream = link.connect(hello)
    except ProxyFallback as exc:
        print(f"ws-mcp-launcher: {exc}; running the direct path", file=stderr, flush=True)
        return None
    note("serving through the daemon")
    return McpProxy(link, hello, [str(link.binary), *argv], stdin, stdout, stderr).run(stream)


def proxy_cli(link: DaemonLink, argv: list[str], stdout, stderr) -> int | None:
    """Run argv (a `mailbox wait`) in the daemon; None means run it directly."""
    hello = proxy_hello("cli", argv, link.env, link.runtime_version)
    try:
        stream = link.connect(hello)
    except ProxyFallback as exc:
        print(f"ws-mcp-launcher: {exc}; running the direct path", file=stderr, flush=True)
        return None
    watch_parent_exit(stream.close)
    while True:
        line = stream.read_line()
        if not line:
            print("ws-mcp-launcher: daemon connection closed before the command finished", file=stderr, flush=True)
            stream.close()
            return 1
        frame = parse_jsonrpc_line(line)
        if frame is None:
            continue
        data = frame.get("data")
        if not isinstance(data, str):
            data = ""
        if frame.get("stream") == "stderr" and data:
            stderr.write(data)
            stderr.flush()
        if frame.get("final"):
            stdout.write(data)
            stdout.flush()
            stream.close()
            code = frame.get("exit_code")
            return code if isinstance(code, int) else 1


def compat_probe_binary(binary: Path, env: dict, mode) -> Path:
    """The executable the compatibility probe runs.

    On the daemon path the plugin-dir copy never executes: the probe runs the
    staged daemon copy (byte-identical, so the verdict is the same), which keeps
    one executable path per build for the endpoint protector. Off that path,
    or when staging fails, the runtime binary itself.
    """
    if mode is None or proxy_disabled(env) or not binary.is_file():
        return binary
    try:
        return stage_daemon_binary(binary, proxy_daemon_dir(env) / DAEMON_BIN_DIR_NAME)
    except OSError as exc:
        note(f"staging the runtime for the compatibility probe failed: {exc}")
        return binary


def proxy_disabled(env: dict) -> bool:
    return env.get("WS_MCP_LAUNCHER_DIRECT", "").strip().lower() in {"1", "true", "yes", "on"}


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

    mode = proxy_mode(sys.argv[1:])
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
    elif runtime_fully_compatible(compat_probe_binary(binary, os.environ, mode), contract, runtime_dir, allow_same_minor=local_enabled):
        write_compatibility_stamp(binary, contract, contract_path, runtime_dir, source_fingerprint)
    else:
        need_install = True

    if need_install:
        note("installing or repairing incompatible runtime")
        clear_compatibility_stamp(runtime_dir)
        install_runtime(plugin_dir, runtime_dir, binary, asset, contract, os_name, platform_name, force_local=local_enabled)
        if not runtime_fully_compatible(compat_probe_binary(binary, os.environ, mode), contract, runtime_dir, allow_same_minor=local_enabled):
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
    if mode is not None and not proxy_disabled(os.environ):
        # The runtime's version is the one the contract accepted: the daemon's
        # build time, not a second run of the binary, tells a newer build apart.
        link = DaemonLink(binary, str(contract.get("plugin_version", "")), os_name, os.environ)
        if mode == "mcp":
            code = proxy_serve(link, sys.argv[1:], sys.stdin.buffer, sys.stdout.buffer, sys.stderr)
        else:
            code = proxy_cli(link, sys.argv[1:], sys.stdout, sys.stderr)
        if code is not None:
            return code
    args = [str(binary), *sys.argv[1:]]
    if os_name == "windows":
        exit_code = subprocess.call(args)
        if exit_code != 0:
            write_exit_breadcrumb(exit_code)
        return exit_code
    os.execvpe(str(binary), args, os.environ)
    return 1


if __name__ == "__main__":
    raise SystemExit(main())
