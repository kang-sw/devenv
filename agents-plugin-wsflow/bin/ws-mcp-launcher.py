#!/usr/bin/env python3
import hashlib
import json
import os
import platform as platform_module
import re
import shlex
import shutil
import stat as stat_module
import subprocess
import sys
import threading
import time
import uuid
from pathlib import Path


_BREADCRUMB_DIR: Path | None = None
RUNTIME_DOWNLOAD_TIMEOUT = 10.0
RUNTIME_BUILD_TIMEOUT = 120.0


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
        with urllib.request.urlopen(url, timeout=RUNTIME_DOWNLOAD_TIMEOUT) as response, destination.open("wb") as out:
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


def runtime_metadata_path(binary: Path) -> Path:
    return binary.with_name(binary.name + ".metadata.json")


def read_runtime_metadata(binary: Path) -> dict | None:
    """Read hash-verified identity, with version and tools/commands keys.

    Inventories are lists when recorded by the producer, otherwise None (unknown,
    not an empty surface). Never fill them from the connecting runtime contract.
    """
    try:
        payload = json.loads(runtime_metadata_path(binary).read_text(encoding="utf-8"))
        if (not isinstance(payload, dict) or payload.get("schema_version") != 1
                or not isinstance(payload.get("version"), str)
                or version_tuple(payload["version"]) is None
                or payload.get("binary_sha256") != sha256_file(binary)):
            return None
        payload.setdefault("tools", None)
        payload.setdefault("commands", None)
        return payload
    except (OSError, ValueError):
        return None


def write_runtime_metadata(binary: Path, identity: dict) -> None:
    """Bind build/source or checksum-verified release identity to a binary.

    Callers must supply the actual identity, never the desired runtime contract.
    Optional capability fields, when supplied by a producer, are checked too.
    """
    if version_tuple(str(identity.get("version", ""))) is None:
        raise ValueError("runtime metadata requires an actual build/release version")
    digest = sha256_file(binary)
    if "binary_sha256" in identity and identity["binary_sha256"] != digest:
        raise ValueError("runtime metadata does not identify the copied binary")
    payload = dict(identity, schema_version=1, binary_sha256=digest)
    path = runtime_metadata_path(binary)
    tmp = unique_runtime_temp_path(path.parent, path.name)
    try:
        tmp.write_text(json.dumps(payload, sort_keys=True) + "\n", encoding="utf-8")
        os.replace(tmp, path)
    finally:
        tmp.unlink(missing_ok=True)


def source_runtime_metadata(tool_dir: Path) -> dict:
    """Identity for a default (no version ldflags) source build, without running it."""
    main = (tool_dir / "cmd" / "ws-mcp" / "main.go").read_text(encoding="utf-8")
    server = (tool_dir / "internal" / "mcp" / "server.go").read_text(encoding="utf-8")
    version = re.search(r'^var version = "([^"]+)"', main, re.MULTILINE)
    commit = re.search(r'^var sourceCommit = "([^"]+)"', main, re.MULTILINE)
    protocol = re.search(r'^const ProtocolVersion = "([^"]+)"', server, re.MULTILINE)
    if version is None or commit is None or protocol is None:
        raise ValueError("cannot read runtime identity from build source")
    return {"version": version[1], "source_commit": commit[1], "mcp_protocol": protocol[1]}


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
    metadata = read_runtime_metadata(binary)
    if metadata is None or not runtime_capabilities_compatible(binary, contract, allow_same_minor=source_fingerprint is not None):
        return None
    try:
        stat = binary.stat()
    except OSError:
        return None
    return {
        "schema_version": 2,
        "binary_sha256": metadata["binary_sha256"],
        "contract_sha256": sha256_file(contract_path),
        "plugin_version": str(contract.get("plugin_version", "")),
        "required_mcp": str(contract.get("required_mcp", "")),
        "accepted_runtime_version": metadata["version"],
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
        # A versioned release plus its checksum identifies legacy releases too;
        # no new mandatory release asset and no executable probe is needed.
        release_version = str(tag or "").removeprefix("v")
        if version_tuple(release_version) is None:
            fail("release tag must identify a version for runtime metadata")
        write_runtime_metadata(tmp, {"version": release_version, "release_tag": tag, "asset": asset})
        try:
            tmp.chmod(0o755)
        except OSError:
            pass
        installed = install_tmp_runtime(tmp, binary, contract, runtime_dir, f"downloaded runtime binary into {binary}")
    finally:
        tmp.unlink(missing_ok=True)
        runtime_metadata_path(tmp).unlink(missing_ok=True)
        sums_tmp.unlink(missing_ok=True)
    if installed:
        try:
            binary.chmod(0o755)
        except OSError:
            pass


def copy_runtime(source: Path, destination: Path) -> None:
    destination.parent.mkdir(parents=True, exist_ok=True)
    metadata = read_runtime_metadata(source)
    shutil.copy2(source, destination)
    if metadata is not None:
        write_runtime_metadata(destination, metadata)
    else:
        runtime_metadata_path(destination).unlink(missing_ok=True)
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
    metadata = read_runtime_metadata(tmp)
    if metadata is not None and binary.is_file() and sha256_file(binary) == metadata["binary_sha256"]:
        # Repair an unstamped legacy release in place, even when Windows holds
        # its executable open. The verified candidate identifies the same bytes.
        write_runtime_metadata(binary, metadata)
        note(f"using identical runtime already installed at {binary}")
        return False
    _replace_attempts = 5
    _replace_backoff = 0.01  # seconds; doubles per attempt
    last_exc: OSError | None = None
    for attempt in range(_replace_attempts):
        try:
            os.replace(tmp, binary)
            break
        except OSError as exc:
            last_exc = exc
            if attempt < _replace_attempts - 1:
                time.sleep(_replace_backoff * (2 ** attempt))
    else:
        if runtime_fully_compatible(binary, contract, runtime_dir, allow_same_minor=allow_same_minor):
            note(f"using compatible runtime already installed at {binary} after replace failed: {last_exc}")
            return False
        fail(f"failed to install runtime at {binary}: {last_exc}")
    if metadata is not None:
        write_runtime_metadata(binary, metadata)
    else:
        runtime_metadata_path(binary).unlink(missing_ok=True)
    runtime_metadata_path(tmp).unlink(missing_ok=True)
    note(message)
    return True


def local_devenv_cache_package(plugin_dir: Path) -> str | None:
    # Recognize a plugin copy installed under a per-user host plugin tree:
    # the install.sh snapshot (~/.claude/plugins/ws-plugin/<pkg>) or a package
    # install under <host>/plugins/cache/kang-sw-devenv/<pkg>/<version>. Match
    # the ws/wsflow package segment under any per-user (.codex/.claude) plugin
    # tree. This is HOME-independent. A plugin running straight from its source
    # checkout has no such segment; read_local_devenv_contract accepts that case
    # through the marker's own source_root instead. Either way the gitignored
    # .local-devenv-runtime marker is the only dev opt-in.
    parts = plugin_dir.resolve().parts
    if "plugins" not in parts:
        return None
    if not any(host in parts for host in (".codex", ".claude")):
        return None
    for seg in parts[parts.index("plugins") + 1:]:
        if seg in {"ws", "wsflow"}:
            return seg
    return None


def path_within(child: Path, root: Path, *, case_insensitive: bool) -> bool:
    # Resolved-path containment by path components; Windows paths compare
    # case-insensitively.
    child_parts, root_parts = child.resolve().parts, root.resolve().parts
    if case_insensitive:
        child_parts = tuple(part.lower() for part in child_parts)
        root_parts = tuple(part.lower() for part in root_parts)
    return child_parts[:len(root_parts)] == root_parts


def read_local_devenv_contract(plugin_dir: Path, os_name: str) -> dict | None:
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
    # The marker counts for a per-user plugin install, or for a plugin running
    # from inside the marker's own source checkout; anywhere else it is stale.
    if local_devenv_cache_package(plugin_dir) is None and not path_within(
        plugin_dir, resolved["source_root"], case_insensitive=os_name == "windows"
    ):
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
    # metadata only (no file reads) to stay cheap on the startup path. Every
    # regular file under tool_dir counts, so //go:embed inputs (assets, i18n
    # tables, embedded rsrc) are covered without parsing embed patterns; test
    # files, testdata, dist* output, and dot-directories cannot change the
    # binary and are skipped.
    contract = read_local_devenv_contract(plugin_dir, os_name)
    if contract is None:
        return None
    tool_dir = contract["tool_dir"]
    entries = []
    walk_errors = []
    for dirpath, dirnames, filenames in os.walk(tool_dir, onerror=walk_errors.append):
        dirnames[:] = [d for d in dirnames if d != "testdata" and not d.startswith(("dist", "."))]
        for filename in filenames:
            if filename.endswith("_test.go"):
                continue
            path = Path(dirpath) / filename
            try:
                st = path.stat()
            except OSError:
                continue
            if not stat_module.S_ISREG(st.st_mode):
                continue
            entries.append((str(path.relative_to(tool_dir)), st.st_size, st.st_mtime_ns))
    if walk_errors:
        note(f"local devenv source fingerprint walk failed: {walk_errors[0]}")
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
    for rel, size, mtime in sorted(set(entries)):
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
    runtime_dir.mkdir(parents=True, exist_ok=True)
    tmp = unique_runtime_temp_path(runtime_dir, f"{binary.name}.local")
    identity = source_runtime_metadata(tool_dir)
    try:
        proc = subprocess.run(
            [str(go_binary), "build", "-o", str(tmp), "./cmd/ws-mcp"],
            cwd=str(tool_dir),
            env=local_devenv_build_env(os_name),
            timeout=RUNTIME_BUILD_TIMEOUT,
            check=False,
        )
        if proc.returncode == 0:
            write_runtime_metadata(tmp, identity)
        if proc.returncode == 0 and runtime_fully_compatible(tmp, contract, runtime_dir, allow_same_minor=True):
            install_tmp_runtime(tmp, binary, contract, runtime_dir, f"built local devenv runtime from {tool_dir}", allow_same_minor=True)
            return True
        note(f"local devenv build failed or produced incompatible runtime: exit={proc.returncode}")
        return False
    finally:
        tmp.unlink(missing_ok=True)
        runtime_metadata_path(tmp).unlink(missing_ok=True)


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
            try:
                copy_runtime(candidate, tmp)
                if runtime_fully_compatible(tmp, contract, runtime_dir, allow_same_minor=True):
                    install_tmp_runtime(tmp, binary, contract, runtime_dir, f"installed local devenv runtime from {candidate}", allow_same_minor=True)
                    return True
                note(f"local devenv runtime candidate is incompatible: {candidate}")
            finally:
                tmp.unlink(missing_ok=True)
                runtime_metadata_path(tmp).unlink(missing_ok=True)

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
            if read_runtime_metadata(tmp) is None:
                fail("bootstrap binary needs hash-bound build/release metadata")
            install_tmp_runtime(tmp, binary, contract, runtime_dir, f"installed bootstrap binary into {binary}")
        finally:
            # A compatible running binary may survive a failed Windows replace;
            # do not retain one full bootstrap copy per mailbox wait re-arm.
            tmp.unlink(missing_ok=True)
            runtime_metadata_path(tmp).unlink(missing_ok=True)
    elif bootstrap_url:
        runtime_dir.mkdir(parents=True, exist_ok=True)
        tmp = unique_runtime_temp_path(runtime_dir, f"{binary.name}.download")
        try:
            download_file(bootstrap_url, tmp)
            download_file(bootstrap_url + ".metadata.json", runtime_metadata_path(tmp))
            if read_runtime_metadata(tmp) is None:
                fail("bootstrap URL needs hash-bound build/release metadata")
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
            runtime_metadata_path(tmp).unlink(missing_ok=True)
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
    payload = read_runtime_metadata(binary)
    if payload is None:
        note("runtime metadata missing or content hash mismatch; repair required")
        return False

    if not version_compatible(str(payload.get("version", "")), contract, allow_same_minor=allow_same_minor):
        note("runtime capabilities version mismatch")
        return False

    expected_protocol = str(contract.get("mcp_protocol", ""))
    if "mcp_protocol" in payload and payload["mcp_protocol"] != expected_protocol:
        note("runtime capabilities MCP protocol mismatch")
        return False

    exact = _capabilities_match_exact(contract)
    # Versioned build/release identity is sufficient for older producers that
    # predate optional surface inventories. Live capabilities belong to the daemon.
    if payload.get("tools") is not None and not _capabilities_match_contract(payload, "tools", runtime_tools(contract), exact=exact):
        return False
    if payload.get("commands") is not None and not _capabilities_match_contract(payload, "commands", runtime_commands(contract), exact=exact):
        return False
    return True

def runtime_fully_compatible(binary: Path, contract: dict, runtime_dir: Path, *, allow_same_minor: bool = False) -> bool:
    return runtime_capabilities_compatible(binary, contract, allow_same_minor=allow_same_minor)


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
# Product commands use the authenticated daemon transport or read metadata.
# Only a necessary daemon start/replacement executes the staged ws binary;
# missing, incompatible or hung peers never select a second server.
# Wire format: internal/wsdashboard/proxy.go (hello, then MCP or CLI frames).
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
# One bounded wait for requests not yet sent, shared by MCP and CLI forwarding.
DAEMON_RETRY_SECONDS = 75.0
DAEMON_RETRY_POLL_SECONDS = 3.0
# Repeated quick closes back off rather than starting a second MCP server.
PROXY_QUICK_CLOSE_SECONDS = 2.0
# After the client closed its stdin, how long the proxy keeps reading the
# answers to the requests still in flight before closing the connection.
PROXY_DRAIN_SECONDS = 10.0


class DaemonUnavailable(Exception):
    """The daemon connection failed; executable fallback is never permitted."""


class DaemonRequestSent(DaemonUnavailable):
    """A CLI hello may have executed its command; never retry it."""


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


def go_ext(name: str) -> str:
    """Go's filepath.Ext: from the last dot of the base name to its end.

    Path.suffix differs on the edges (a trailing dot, a dot-file), and the
    staged name must match the Go side byte for byte.
    """
    base = os.path.basename(name)
    dot = base.rfind(".")
    return base[dot:] if dot >= 0 else ""


def staged_daemon_name(binary: Path, digest: str | None = None) -> str:
    """The name wsdashboard.StageExecutable gives binary's staged copy."""
    if digest is None:
        digest = sha256_file(binary)
    return f"ws-mcp-dashboard-{digest[:16]}{go_ext(binary.name).lower()}"


def stage_daemon_binary(binary: Path, bin_dir: Path) -> Path:
    """Copy binary into bin_dir under its staged name, as StageExecutable does.

    The copy is written beside its target and renamed into place, so a
    concurrent starter never runs a half-written file. Other entries are left
    alone: one of them is the executable the running daemon was started from
    (it respawns itself from that path on a LAN mode change), and the daemon
    that holds the lock prunes the directory down to its own executable.
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
    metadata = read_runtime_metadata(binary)
    if metadata is not None:
        write_runtime_metadata(target, metadata)
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


def same_core(a: str, b: str) -> bool:
    pa = parse_daemon_version(a)
    pb = parse_daemon_version(b)
    return pa is not None and pb is not None and pa[0] == pb[0]


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


class DaemonHung(Exception):
    """The daemon's port accepted but nothing answered within the timeout."""


def daemon_health(port: int, timeout: float) -> dict | None:
    """GET /health on the daemon's port; None when nothing compatible answers.

    Raises DaemonHung when the port accepts the connection but the answer
    does not arrive within timeout: a daemon that is alive but wedged, which
    is the one case the launcher gives up on instead of starting another.
    """
    import http.client
    import socket as socket_module

    conn = http.client.HTTPConnection("127.0.0.1", port, timeout=timeout)
    try:
        conn.request("GET", "/health")
        resp = conn.getresponse()
        body = resp.read(64 * 1024)
        if resp.status != 200:
            return None
        payload = json.loads(body)
    except socket_module.timeout as exc:
        raise DaemonHung(f"daemon on port {port} did not answer within {timeout:.0f}s") from exc
    except (OSError, ValueError, http.client.HTTPException):
        return None
    finally:
        conn.close()
    return payload if isinstance(payload, dict) else None


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
    root = env.get("WS_MCP_PROJECT_ROOT", "")
    if mode == "mcp":
        explicit_root = stdio_root(argv)
        if explicit_root and explicit_root != ".":
            root = os.path.abspath(explicit_root)
    return {
        "protocol": PROXY_PROTOCOL,
        "mode": mode,
        "argv": list(argv),
        "cwd": cwd,
        "project_root": root,
        "env": env,
        "harness": env.get("WS_MCP_HARNESS", ""),
        "owner_pid": os.getpid(),
        "version": runtime_version,
    }


class SocketStream:
    """A newline-delimited stream over the upgraded daemon connection."""

    def __init__(self, sock, reader, daemon_version: str = "") -> None:
        self.sock = sock
        self.reader = reader
        self.daemon_version = daemon_version

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

    def close_write(self) -> None:
        """Tell the daemon the client is done; its answers still arrive."""
        import socket as socket_module

        try:
            self.sock.shutdown(socket_module.SHUT_WR)
        except OSError:
            pass

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


def daemon_upgrade(endpoint: dict, hello: dict, timeout: float) -> SocketStream:
    """Open the MCP route: HTTP Upgrade, the hello line, the accept line."""
    import socket as socket_module

    port = int(endpoint["port"])
    request_sent = False
    try:
        sock = socket_module.create_connection(("127.0.0.1", port), timeout=timeout)
    except OSError as exc:
        raise DaemonUnavailable(f"daemon connection failed: {exc}") from exc
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
            raise DaemonUnavailable(f"daemon refused the MCP route: {status or 'no answer'}")
        # CLI commands are carried by the hello itself. Even a failed write
        # can have sent bytes, so everything after this point is non-retryable.
        request_sent = hello.get("mode") == "cli"
        sock.sendall(json.dumps(hello, separators=(",", ":")).encode("utf-8") + b"\n")
        raw = reader.readline()
        try:
            accept = json.loads(raw)
        except ValueError:
            accept = None
        if not isinstance(accept, dict):
            raise DaemonUnavailable("daemon closed the MCP route before answering the hello")
        if not accept.get("ok"):
            raise DaemonUnavailable(f"daemon refused this session: {accept.get('reason') or 'no reason given'}")
        sock.settimeout(None)
        return SocketStream(sock, reader, str(accept.get("version", "")))
    except (OSError, ValueError) as exc:
        sock.close()
        error = DaemonRequestSent if request_sent else DaemonUnavailable
        raise error(f"daemon connection failed: {exc}") from exc
    except DaemonUnavailable as exc:
        sock.close()
        if request_sent:
            raise DaemonRequestSent(str(exc)) from exc
        raise


def wait_daemon_connection(link, hello: dict):
    """Retry only before a command was sent; rediscover the endpoint each time.

    Keep the existing first connection's start/replacement behavior. Subsequent
    attempts only connect to the successor, never repeatedly start a daemon.
    """
    deadline = time.monotonic() + DAEMON_RETRY_SECONDS
    try:
        return link.connect(hello)
    except DaemonRequestSent:
        raise
    except (DaemonUnavailable, DaemonHung) as exc:
        failure = exc
    while True:
        remaining = deadline - time.monotonic()
        if remaining <= 0:
            raise DaemonUnavailable(f"{failure} (waited {DAEMON_RETRY_SECONDS:g}s for the dashboard daemon)")
        time.sleep(min(DAEMON_RETRY_POLL_SECONDS, remaining))
        remaining = deadline - time.monotonic()
        if remaining <= 0:
            continue
        endpoint = read_daemon_endpoint(link.daemon_dir)
        if endpoint is None:
            continue
        try:
            return daemon_upgrade(endpoint, hello, min(DAEMON_RETRY_POLL_SECONDS, remaining))
        except DaemonRequestSent:
            raise
        except (DaemonUnavailable, DaemonHung) as exc:
            failure = exc


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
        allow_start: bool = True,
        repair_runtime=None,
    ) -> None:
        self.binary = binary
        self.runtime_version = runtime_version
        self.os_name = os_name
        self.env = env
        self.allow_start = allow_start
        self.repair_runtime = repair_runtime
        self.daemon_dir = proxy_daemon_dir(env)
        extra = 10.0 if os_name == "windows" else 5.0
        # Each connection/start attempt is bounded; failed attempts never
        # select another executable server.
        self.answer_budget = extra if answer_budget is None else answer_budget
        self.start_budget = DAEMON_DEFAULT_START_WAIT + extra if start_budget is None else start_budget
        try:
            self.build_time = binary.stat().st_mtime_ns // 1_000_000
        except OSError:
            self.build_time = 0
        self._content_hash: str | None = None
        self.started = False
        self.start_error = ""
        metadata = read_runtime_metadata(binary)
        self.binary_version = metadata["version"] if metadata is not None else runtime_version
        if metadata is None:
            self.build_time = 0

    def content_hash(self) -> str:
        """The runtime binary's sha256, computed once and only when needed."""
        if self._content_hash is None:
            try:
                self._content_hash = sha256_file(self.binary)
            except OSError:
                self._content_hash = ""
        return self._content_hash

    def probe(self, timeout: float) -> tuple[dict | None, dict | None]:
        """The endpoint file and the daemon's health; DaemonHung when it accepts but never answers."""
        endpoint = read_daemon_endpoint(self.daemon_dir)
        if endpoint is None:
            return None, None
        health = daemon_health(int(endpoint["port"]), timeout)
        if health is None or not health.get("version"):
            # Older compatible daemons need no new version endpoint or stamp.
            # Their existing hello already returns the actual daemon version.
            hello = proxy_hello("mcp", ["serve", "--stdio"], self.env, self.runtime_version)
            try:
                stream = daemon_upgrade(endpoint, hello, timeout)
            except DaemonUnavailable:
                if health is not None:
                    raise
                return endpoint, None
            try:
                if not stream.daemon_version:
                    raise DaemonUnavailable("daemon answered but supplied no version information")
                health = dict(health or {}, version=stream.daemon_version)
            finally:
                stream.close()
        return endpoint, health

    def wait_healthy(self, budget: float) -> tuple[dict | None, dict | None]:
        deadline = time.monotonic() + budget
        while True:
            try:
                endpoint, health = self.probe(min(1.0, max(0.1, deadline - time.monotonic())))
            except DaemonHung:
                endpoint, health = None, None
            if health is not None:
                return endpoint, health
            if time.monotonic() >= deadline:
                return endpoint, None
            time.sleep(0.2)

    def needs_start(self, health: dict) -> bool:
        """Older daemon, or the same version from an older build of other content.

        Same version and same content hash never replace: Claude, Codex, and
        Pi each install their own copy of one binary, whose mtimes differ,
        and a replacement drains every session on the machine. The build time
        breaks the tie only when the hashes differ (or the daemon reports
        none).

        An equal numeric core counts as the same version whatever the
        suffixes: runtime_version is the contract's plugin_version
        ("0.48.17"), while a local-devenv build reports "0.48.17-dev", so
        the suffix order would hide every rebuild at that core.
        """
        version = str(health.get("version", ""))
        order = compare_daemon_versions(version, self.binary_version)
        if order != 0 and not same_core(version, self.binary_version):
            return order < 0
        build_time = health.get("build_time")
        if not (isinstance(build_time, int) and 0 < self.build_time and build_time < self.build_time):
            return False
        content_hash = health.get("content_hash")
        if isinstance(content_hash, str) and content_hash and content_hash == self.content_hash():
            return False
        return True

    def start_daemon(self) -> bool:
        """Run the staged binary's daemon-start entry; True when it reports a daemon."""
        self.started = True
        # Healthy peers do not need a local candidate. Repair only when a
        # start/replacement is actually required, including after reconnect.
        if self.repair_runtime is not None and not self.repair_runtime():
            self.start_error = "runtime repair failed; no verified runtime available to start the daemon"
            return False
        metadata = read_runtime_metadata(self.binary)
        if metadata is None:
            self.start_error = "no verified runtime metadata available to start the daemon"
            return False
        self.binary_version = metadata["version"]
        self.build_time = self.binary.stat().st_mtime_ns // 1_000_000
        self._content_hash = None
        try:
            staged = stage_daemon_binary(self.binary, self.daemon_dir / DAEMON_BIN_DIR_NAME)
        except OSError as exc:
            self.start_error = f"staging the daemon binary failed: {exc}"
            note(self.start_error)
            return False
        argv = [str(staged), "daemon-start", "--build-time", str(self.build_time)]
        note(f"starting the daemon: {' '.join(argv)}")
        # The daemon starts session holders with the interpreter this launcher
        # runs under; PATH python3 may be missing or, on Windows, a Store alias.
        env = dict(self.env)
        if not env.get("WS_MCP_HOLDER_PYTHON") and sys.executable:
            env["WS_MCP_HOLDER_PYTHON"] = sys.executable
        try:
            proc = subprocess.run(
                argv,
                env=env,
                stdin=subprocess.DEVNULL,
                capture_output=True,
                text=True,
                timeout=2 * DAEMON_DEFAULT_START_WAIT + self.start_budget,
                check=False,
            )
        except (OSError, subprocess.SubprocessError) as exc:
            self.start_error = f"daemon-start failed: {exc}"
            note(self.start_error)
            return False
        for line in proc.stderr.splitlines():
            note(f"daemon-start: {line}")
        if proc.returncode != 0:
            self.start_error = f"daemon-start exited {proc.returncode}: {proc.stderr.strip() or proc.stdout.strip()}"
            note(self.start_error)
            return False
        return True

    def connect(self, hello: dict, *, grace: float = 0.0) -> SocketStream:
        """Connect to a compatible daemon, starting or replacing one when needed.

        grace is how long a daemon that answers nothing is given before one is
        started: a reconnect under a replacement gives the successor a moment.
        A wedged daemon is diagnosed without trying to start a second one.
        """
        if grace > 0:
            endpoint, health = self.wait_healthy(grace)
        else:
            try:
                endpoint, health = self.probe(self.answer_budget)
            except DaemonHung as exc:
                raise DaemonUnavailable(str(exc)) from exc
        if health is None and not self.allow_start:
            raise DaemonUnavailable("no running daemon answered; inspection never starts one")
        if self.allow_start and (health is None or self.needs_start(health)):
            if not self.start_daemon():
                raise DaemonUnavailable(self.start_error or "no daemon could be started")
            endpoint, health = self.wait_healthy(self.start_budget)
            if health is None or endpoint is None:
                raise DaemonUnavailable(f"no daemon answered within {self.start_budget:.0f}s of starting one")
            if self.needs_start(health) and compare_daemon_versions(str(health.get("version", "")), self.binary_version) <= 0:
                # daemon-start reported a daemon, but the one answering is
                # still the older one (it refused to stop, or won the lock
                # race): never settle on it. A same-core daemon whose suffix
                # sorts after runtime_version is one daemon-start keeps by
                # design (wsdashboard.keepRunning: a release binary never
                # replaces a "-dev" daemon of its core), so it is used.
                raise DaemonUnavailable(f"daemon {health.get('version', '')} (build {health.get('build_time', 0)}) is older than this plugin's runtime and was not replaced")
        version = str(health.get("version", ""))
        if not same_minor(version, self.runtime_version):
            raise DaemonUnavailable(f"daemon {version} is outside this plugin's minor ({self.runtime_version})")
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
    with an error. While disconnected, unsent requests wait up to 75 seconds
    for recovery. Requests already sent are never replayed.
    """

    def __init__(self, link: DaemonLink, hello: dict, stdin, stdout, stderr) -> None:
        self.link = link
        self.hello = hello
        self.stdin = stdin
        self.stdout = stdout
        self.stderr = stderr
        self.lock = threading.Lock()
        self.pending_done = threading.Condition(self.lock)
        self.pending: dict[str, tuple[bytes, dict, threading.Timer]] = {}
        self.output_lock = threading.Lock()
        self.stdin_done = threading.Event()
        self.stream = None
        self.initialize: dict | None = None
        self.initialized: bytes | None = None
        self.inflight: dict[str, object] = {}
        self.replay_key: str | None = None
        self.replays = 0
        self.stdin_closed = False
        # connected_at is when the current stream was made; quick_closes
        # counts the streams in a row that ended within
        # PROXY_QUICK_CLOSE_SECONDS of being made (a daemon in a crash loop).
        self.connected_at = time.monotonic()
        self.quick_closes = 0

    def run(self, stream) -> int:
        self.stream = stream
        self.connected_at = time.monotonic()
        pump = threading.Thread(target=self.pump_client, name="ws-mcp-launcher-stdin", daemon=True)
        pump.start()
        while True:
            if stream is None:
                if self.stdin_closed:
                    break
                stream = self.recover()
                continue
            line = stream.read_line()
            if not line:
                if self.stdin_closed:
                    self.fail_inflight("dashboard daemon not reachable: connection closed")
                    break
                stream = self.recover()
                continue
            if self.deliver(line) and not self.output(line):
                break
        with self.lock:
            final = self.stream
        if final is not None:
            final.close()
            return final.wait()
        return 1

    def output(self, line: bytes) -> bool:
        try:
            with self.output_lock:
                self.stdout.write(line if line.endswith(b"\n") else line + b"\n")
                self.stdout.flush()
            return True
        except (OSError, ValueError):
            return False

    def error(self, request_id, message: str) -> None:
        self.output(json.dumps({"jsonrpc": "2.0", "id": request_id,
                               "error": {"code": PROXY_LOST_REQUEST_CODE, "message": message}},
                              separators=(",", ":")).encode("utf-8"))

    def fail_inflight(self, message: str) -> None:
        with self.lock:
            lost = list(self.inflight.values())
            self.inflight.clear()
        for request_id in lost:
            self.error(request_id, message)

    def pump_client(self) -> None:
        while True:
            try:
                line = self.stdin.readline()
            except (OSError, ValueError):
                line = b""
            if not line:
                break
            message = parse_jsonrpc_line(line)
            request = message is not None and message.get("method") is not None and "id" in message
            with self.lock:
                if request and self.stream is None:
                    # Each request has its own budget, even when a client sends
                    # several concurrently. None has attempted a socket write.
                    key = json_id_key(message["id"])
                    timer = threading.Timer(DAEMON_RETRY_SECONDS, self.expire_pending, args=(key,))
                    timer.daemon = True
                    self.pending[key] = (line, message, timer)
                    timer.start()
                else:
                    self.forward(line, message)
        with self.pending_done:
            # EOF still lets unsent requests finish within their budgets, just
            # as half-close below lets already-sent requests finish.
            while self.pending:
                self.pending_done.wait()
            self.stdin_closed = True
            self.stdin_done.set()
            stream = self.stream
        if stream is None:
            return
        # Half-close: the server sees EOF, finishes the requests in flight,
        # and closes when it is done; run() delivers those answers meanwhile
        # and ends at the server's close. A server that never closes is cut
        # off after PROXY_DRAIN_SECONDS.
        stream.close_write()
        cutoff = threading.Timer(PROXY_DRAIN_SECONDS, stream.close)
        cutoff.daemon = True
        cutoff.start()

    def forward(self, line: bytes, message: dict | None) -> None:
        """Attempt one write while holding lock; never retry a failed write."""
        if message is not None:
            method = message.get("method")
            if method == "initialize" and "id" in message and self.stream is not None:
                # Capture only sent initialize requests, so recovery cannot
                # replay an unsent one AND forward its queued original.
                self.initialize = message
            elif method == "notifications/initialized":
                self.initialized = line
        if self.stream is not None:
            if message is not None and message.get("method") is not None and "id" in message:
                self.inflight[json_id_key(message["id"])] = message["id"]
            if not self.stream.write(line):
                self.stream.close()

    def expire_pending(self, key: str) -> None:
        with self.pending_done:
            pending = self.pending.pop(key, None)
            if pending is not None:
                self.error(pending[1]["id"], f"dashboard daemon not reachable; request was not sent (waited {DAEMON_RETRY_SECONDS:g}s for the dashboard daemon)")
            self.pending_done.notify_all()

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
            self.stream = None
        if old is not None:
            old.close()
            if time.monotonic() - self.connected_at < PROXY_QUICK_CLOSE_SECONDS:
                self.quick_closes += 1
            else:
                self.quick_closes = 0
        # Sent requests fail before reconnecting: their outcome is unknown.
        # Unsent requests wait independently in pump_client.
        self.fail_inflight("dashboard daemon not reachable; the request did not complete")
        if self.stdin_done.wait(max(DAEMON_RETRY_POLL_SECONDS, min(5.0, 0.5 * self.quick_closes))):
            return None
        try:
            stream = self.link.connect(self.hello, grace=1.5)
        except (DaemonUnavailable, DaemonHung) as exc:
            print(f"ws-mcp-launcher: dashboard daemon not reachable: {exc}", file=self.stderr, flush=True)
            return None
        self.replay_into(stream)
        self.connected_at = time.monotonic()
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
            pending = list(self.pending.values())
            self.pending.clear()
            for line, message, timer in pending:
                timer.cancel()
                self.forward(line, message)
            self.pending_done.notify_all()
        for request_id in lost:
            self.error(request_id, "dashboard daemon not reachable; the request did not complete")


def watch_parent_exit(on_exit, parent_pid: int | None = None) -> None:
    """Close the daemon connection when the launcher's parent ends.

    POSIX does not forward a parent's signals to this process. Reparenting
    therefore needs a watch too, especially for waits with ignored stdin.
    """
    ppid = os.getppid() if parent_pid is None else parent_pid
    if ppid <= 0:
        return
    if os.name != "nt":
        def run_posix() -> None:
            while os.getppid() == ppid:
                time.sleep(1.0)
            on_exit()

        threading.Thread(target=run_posix, name="ws-mcp-launcher-parent-watch", daemon=True).start()
        return
    try:
        import _winapi
    except ImportError:
        return

    def run() -> None:
        try:
            handle = _winapi.OpenProcess(getattr(_winapi, "SYNCHRONIZE", 0x00100000), False, ppid)
        except OSError as exc:
            if exc.winerror == 87:  # ERROR_INVALID_PARAMETER: parent already gone.
                on_exit()
            return
        try:
            if _winapi.WaitForSingleObject(handle, _winapi.INFINITE) == _winapi.WAIT_OBJECT_0:
                on_exit()
        except OSError:
            return
        finally:
            _winapi.CloseHandle(handle)

    threading.Thread(target=run, name="ws-mcp-launcher-parent-watch", daemon=True).start()


def stdio_root(argv: list[str]) -> str | None:
    """Parse serve's Go flag shapes; None means not a valid stdio invocation."""
    if argv[:1] != ["serve"]:
        return None
    stdio, root, index = False, "", 1
    while index < len(argv):
        arg = argv[index]
        if arg == "--" or not arg.startswith("-"):
            break  # Go flag parsing stops at the first positional argument.
        name, assignment, value = arg.partition("=")
        if name in {"--stdio", "-stdio"}:
            if not assignment:
                stdio = True
            elif value in {"1", "t", "T", "TRUE", "true", "True"}:
                stdio = True
            elif value in {"0", "f", "F", "FALSE", "false", "False"}:
                stdio = False
            else:
                return None
        elif name in {"--root", "-root"}:
            if not assignment:
                index += 1
                if index >= len(argv):
                    return None
                value = argv[index]
            root = value
        else:
            return None
        index += 1
    return root if stdio else None


def proxy_mode(argv: list[str]) -> str:
    """All product commands stay in the daemon; only its starter executes."""
    if stdio_root(argv) is not None:
        return "mcp"
    if argv[:1] == ["daemon-start"]:
        return "lifecycle"
    return "cli"


def proxy_serve(link: DaemonLink, argv: list[str], stdin, stdout, stderr) -> int:
    hello = proxy_hello("mcp", argv, link.env, link.runtime_version)
    try:
        stream = link.connect(hello)
    except (DaemonUnavailable, DaemonHung) as exc:
        print(f"ws-mcp-launcher: dashboard daemon not reachable: {exc}", file=stderr, flush=True)
        stream = None
    return McpProxy(link, hello, stdin, stdout, stderr).run(stream)


def write_bytes(stream, text: str) -> None:
    """Write text to a binary stream as UTF-8, unchanged: the daemon's output
    goes out byte for byte, with no console code page and no newline
    translation (Windows text-mode stdout would apply both)."""
    stream.write(text.encode("utf-8"))
    stream.flush()


def proxy_cli(link: DaemonLink, argv: list[str], stdout, stderr, input_text: str | None = None) -> int:
    """Run once through authenticated CLI frames. Never replay a command."""
    parent_pid = os.getppid()
    hello = proxy_hello("cli", argv, link.env, link.runtime_version)
    if input_text is not None:
        hello["stdin"] = input_text
    failure_code = 0 if argv[:2] == ["mailbox", "codex-stop-hook"] else 1
    try:
        stream = wait_daemon_connection(link, hello)
    except (DaemonUnavailable, DaemonHung) as exc:
        write_bytes(stderr, f"ws-mcp-launcher: dashboard daemon not reachable: {exc}\n")
        return failure_code
    watch_parent_exit(stream.close, parent_pid)
    while True:
        line = stream.read_line()
        if not line:
            write_bytes(stderr, "ws-mcp-launcher: dashboard daemon not reachable: connection closed before the command finished\n")
            stream.close()
            return failure_code
        frame = parse_jsonrpc_line(line)
        if frame is None:
            continue
        data = frame.get("data")
        if not isinstance(data, str):
            data = ""
        if frame.get("stream") == "stderr" and data:
            write_bytes(stderr, data)
        elif frame.get("stream") == "stdout" and data:
            write_bytes(stdout, data)
        if frame.get("final"):
            write_bytes(stdout, data)
            stream.close()
            # The daemon always sends the code; a final frame without one
            # can only come from a daemon that omitted a zero, so it reads
            # as success rather than turning a delivered mail into an error.
            code = frame.get("exit_code", 0)
            return code if isinstance(code, int) and not isinstance(code, bool) else 1


def inspect_runtime(link: DaemonLink, argv: list[str], health: dict | None) -> int:
    """Read live identity or content-bound metadata, without starting a daemon."""
    metadata = read_runtime_metadata(link.binary)
    identity = health or metadata
    if identity is None:
        fail("dashboard daemon not reachable and no hash-bound runtime metadata available")
    if argv == ["version"]:
        print(identity["version"])
        return 0
    if health is not None:
        return proxy_cli(link, argv, sys.stdout.buffer, sys.stderr.buffer)
    if argv == ["runtime", "info"]:
        print(json.dumps({"version": identity["version"], "source_commit": identity.get("source_commit", "")}))
        return 0
    if identity.get("tools") is None or identity.get("commands") is None:
        fail("dashboard daemon not reachable; live capabilities unavailable in runtime metadata")
    print(json.dumps({key: identity.get(key) for key in ("version", "source_commit", "mcp_protocol", "tools", "commands")}))
    return 0


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

    argv = sys.argv[1:]
    mode = proxy_mode(argv)
    inspection = argv in (["version"], ["runtime", "info"], ["runtime", "capabilities"])
    bootstrap_forced = bootstrap_runtime_forced()
    local_enabled = local_devenv_runtime_enabled(plugin_dir, os_name)
    install_forced = runtime_install_forced(plugin_dir, os_name, local_enabled=local_enabled)
    source_fingerprint = local_devenv_source_fingerprint(plugin_dir, os_name) if local_enabled else None

    detect_project_root(plugin_dir)
    os.environ["WS_MCP_RUNTIME_BINARY"] = str(binary)
    os.environ["WS_MCP_HARNESS_PID"] = str(os.getppid())
    # Printed mailbox waits must lead back to Python too (Codex has no shim on PATH).
    os.environ.setdefault("WS_MCP_CLI_NAME", shlex.join([sys.executable, str(launcher_path)]))
    apply_rsrc_root_env(plugin_dir, os.environ)
    apply_skills_root_env(plugin_dir, os.environ)
    link = DaemonLink(binary, str(contract.get("plugin_version", "")), os_name, os.environ, allow_start=not inspection)
    try:
        _, health = link.probe(link.answer_budget)
    except (DaemonHung, DaemonUnavailable) as exc:
        note(str(exc))
        health = None

    if inspection and not bootstrap_forced:
        return inspect_runtime(link, argv, health)
    usable_daemon = health is not None and same_minor(str(health.get("version", "")), link.runtime_version) and (
        compare_daemon_versions(str(health["version"]), link.runtime_version) >= 0 or same_core(str(health["version"]), link.runtime_version))
    cli_uses_daemon = mode == "cli" and not inspection and usable_daemon
    need_install = False
    if cli_uses_daemon:
        # Waits and hooks consume the running daemon, even with a forced local install.
        pass
    elif bootstrap_forced:
        note("forcing runtime install from bootstrap binary or url")
        clear_compatibility_stamp(runtime_dir)
        need_install = True
    elif usable_daemon and not install_forced:
        # An already compatible daemon never requires a local metadata upgrade.
        pass
    elif compatibility_stamp_current(binary, contract, contract_path, runtime_dir, source_fingerprint):
        pass
    elif install_forced and local_enabled:
        note("local devenv source changed or runtime missing; rebuilding from source")
        need_install = True
    elif runtime_fully_compatible(binary, contract, runtime_dir, allow_same_minor=local_enabled):
        write_compatibility_stamp(binary, contract, contract_path, runtime_dir, source_fingerprint)
    else:
        need_install = True

    def repair_runtime(force: bool = False) -> bool:
        if not force and runtime_fully_compatible(binary, contract, runtime_dir, allow_same_minor=local_enabled):
            return True
        note("installing or repairing incompatible runtime")
        clear_compatibility_stamp(runtime_dir)
        try:
            install_runtime(plugin_dir, runtime_dir, binary, asset, contract, os_name, platform_name, force_local=local_enabled)
            if not runtime_fully_compatible(binary, contract, runtime_dir, allow_same_minor=local_enabled):
                fail("incompatible ws-mcp runtime after repair")
            write_compatibility_stamp(binary, contract, contract_path, runtime_dir, source_fingerprint)
            return True
        except (SystemExit, OSError, ValueError, subprocess.SubprocessError) as exc:
            print(f"ws-mcp-launcher: runtime repair failed: {exc}", file=sys.stderr, flush=True)
            if inspection or mode == "lifecycle":
                fail(f"dashboard daemon not reachable: runtime repair failed: {exc}")
            return False

    if need_install:
        repair_runtime(force=True)
    # A CLI consumer must not replace a usable daemon just because its local binary is newer.
    link = DaemonLink(binary, str(contract.get("plugin_version", "")), os_name, os.environ,
                      allow_start=not inspection and not cli_uses_daemon, repair_runtime=repair_runtime)
    if inspection:
        # An explicit bootstrap+version repairs a child runtime but never starts it.
        return inspect_runtime(link, argv, None)
    if mode == "lifecycle":
        if health is not None and not link.needs_start(health):
            print(json.dumps(dict(health, outcome="kept")))
            return 0
        if read_runtime_metadata(binary) is None:
            fail("dashboard daemon not reachable: no verified runtime to start")
        staged = stage_daemon_binary(binary, link.daemon_dir / DAEMON_BIN_DIR_NAME)
        return subprocess.call([str(staged), *argv], env=os.environ)
    wait_for_rsrc_tree(plugin_dir)
    apply_rsrc_root_env(plugin_dir, os.environ)
    apply_skills_root_env(plugin_dir, os.environ)
    clear_launch_breadcrumb()
    if mode == "mcp":
        return proxy_serve(link, argv, sys.stdin.buffer, sys.stdout.buffer, sys.stderr)
    reads_stdin = argv[:2] == ["mailbox", "codex-stop-hook"] or (
        argv[:2] == ["git", "commit"] and ("--description-file=-" in argv or "-description-file=-" in argv
        or any(arg in {"--description-file", "-description-file"} and index + 1 < len(argv) and argv[index + 1] == "-"
               for index, arg in enumerate(argv))))
    # The pipe is UTF-8 regardless of Python's Windows console/locale encoding.
    # Replacement matches Go JSON decoding for malformed UTF-8 hook payloads.
    input_text = sys.stdin.buffer.read().decode("utf-8", errors="replace") if reads_stdin else None
    return proxy_cli(link, argv, sys.stdout.buffer, sys.stderr.buffer, input_text)


if __name__ == "__main__":
    raise SystemExit(main())
