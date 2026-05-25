import importlib.util
import hashlib
import json
import subprocess
import tempfile
import unittest
from unittest import mock
from pathlib import Path


LAUNCHER_PATH = Path(__file__).resolve().parents[1] / "bin" / "ws-mcp-launcher.py"


def load_launcher():
    spec = importlib.util.spec_from_file_location("ws_mcp_launcher", LAUNCHER_PATH)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    spec.loader.exec_module(module)
    return module


class RuntimeCapabilitiesCompatibilityTest(unittest.TestCase):
    def write_local_contract(self, plugin_dir: Path, *, package_root: Path | None = None) -> dict:
        package_root = package_root or plugin_dir.parent.parent.parent.parent.parent.parent
        source_root = package_root / "devenv"
        tool_dir = source_root / "agents-plugin-tool"
        go_binary = source_root / "bin" / "go"
        (tool_dir / "cmd" / "ws-mcp").mkdir(parents=True)
        go_binary.parent.mkdir(parents=True)
        go_binary.write_text("#!/bin/sh\n", encoding="utf-8")
        go_binary.chmod(0o755)
        marker = plugin_dir / ".local-devenv-runtime"
        marker.write_text(
            json.dumps(
                {
                    "schema_version": 1,
                    "source_root": str(source_root),
                    "tool_dir": str(tool_dir),
                    "go": str(go_binary),
                }
            )
            + "\n",
            encoding="utf-8",
        )
        return {"source_root": source_root, "tool_dir": tool_dir, "go": go_binary}

    def capability_contract(self):
        return {
            "plugin_version": "0.18.1",
            "mcp_protocol": "2025-03-26",
            "prompt_bundle": {"content_sha256": "abc123"},
            "tools": {"runtime.info": ">=0.18.1-dev <0.19.0"},
            "commands": {"runtime.info": ">=0.18.1-dev <0.19.0"},
        }

    def capability_payload(self):
        return {
            "version": "0.18.1",
            "source_commit": "dev",
            "mcp_protocol": "2025-03-26",
            "prompt_bundle": {"content_sha256": "abc123", "prompts": ["delegate-orientation"]},
            "tools": ["runtime.info"],
            "commands": ["runtime.info"],
        }

    def test_version_compatibility_requires_exact_plugin_patch(self):
        launcher = load_launcher()
        contract = self.capability_contract()

        self.assertTrue(launcher.version_compatible("0.18.1", contract))
        self.assertTrue(launcher.version_compatible("0.18.1-dev", contract))
        self.assertFalse(launcher.version_compatible("0.18.0", contract))
        self.assertFalse(launcher.version_compatible("0.18.2", contract))
        self.assertFalse(launcher.version_compatible("0.19.0", contract))

    def test_capabilities_probe_validates_full_contract_from_one_response(self):
        launcher = load_launcher()
        binary = Path("/tmp/ws-mcp")
        calls = []
        contract = self.capability_contract()

        def fake_run_binary(got_binary, args, **kwargs):
            calls.append((got_binary, tuple(args)))
            return subprocess.CompletedProcess(
                [str(got_binary), *args], 0, stdout=json.dumps(self.capability_payload()), stderr=""
            )

        launcher.run_binary = fake_run_binary

        self.assertTrue(launcher.runtime_capabilities_compatible(binary, contract))
        self.assertEqual(calls, [(binary, ("runtime", "capabilities"))])

    def test_invalid_or_incomplete_capabilities_probe_is_not_compatible(self):
        launcher = load_launcher()
        binary = Path("/tmp/ws-mcp")
        contract = self.capability_contract()

        cases = {
            "version": lambda payload: payload.update({"version": "0.17.9"}),
            "patch_version": lambda payload: payload.update({"version": "0.18.0"}),
            "protocol": lambda payload: payload.update({"mcp_protocol": "2024-11-05"}),
            "prompt_bundle": lambda payload: payload.update({"prompt_bundle": {"content_sha256": "wrong"}}),
            "tools": lambda payload: payload.update({"tools": []}),
            "commands": lambda payload: payload.update({"commands": []}),
        }
        for name, mutate in cases.items():
            with self.subTest(name=name):
                payload = self.capability_payload()
                mutate(payload)
                launcher.run_binary = lambda got_binary, args, **kwargs: subprocess.CompletedProcess(
                    [str(got_binary), *args], 0, stdout=json.dumps(payload), stderr=""
                )

                self.assertFalse(launcher.runtime_capabilities_compatible(binary, contract))

    def test_exact_capabilities_contract_rejects_extra_surface(self):
        launcher = load_launcher()
        binary = Path("/tmp/ws-mcp")
        contract = self.capability_contract()
        contract["runtime_capabilities"] = {"match": "exact"}

        payload = self.capability_payload()
        payload["tools"] = ["runtime.info", "agents.call"]
        payload["commands"] = ["runtime.info"]
        launcher.run_binary = lambda got_binary, args, **kwargs: subprocess.CompletedProcess(
            [str(got_binary), *args], 0, stdout=json.dumps(payload), stderr=""
        )

        self.assertFalse(launcher.runtime_capabilities_compatible(binary, contract))

        payload["tools"] = ["runtime.info"]
        self.assertTrue(launcher.runtime_capabilities_compatible(binary, contract))

    def test_exact_capabilities_contract_disables_weaker_fallback(self):
        launcher = load_launcher()
        import tempfile

        with tempfile.TemporaryDirectory() as temp_dir:
            temp = Path(temp_dir)
            binary = temp / "ws-mcp"
            binary.write_text("stub", encoding="utf-8")
            contract = self.capability_contract()
            contract["runtime_capabilities"] = {"match": "exact"}

            launcher.run_binary = lambda got_binary, args, **kwargs: subprocess.CompletedProcess(
                [str(got_binary), *args], 0, stdout=json.dumps({"version": "0.18.1"}), stderr=""
            )

            def forbidden_fallback(*args, **kwargs):
                raise AssertionError("exact capability contracts must not use weaker fallback checks")

            launcher.tools_compatible = forbidden_fallback
            launcher.commands_compatible = forbidden_fallback
            launcher.prompt_bundle_compatible = forbidden_fallback

            self.assertFalse(launcher.runtime_fully_compatible(binary, contract, temp))

    def test_successful_capabilities_probe_skips_fanout(self):
        launcher = load_launcher()
        import tempfile

        with tempfile.TemporaryDirectory() as temp_dir:
            temp = Path(temp_dir)
            binary = temp / "ws-mcp"
            binary.write_text("stub", encoding="utf-8")

            launcher.runtime_capabilities_compatible = lambda got_binary, contract: got_binary == binary

            def forbidden_fanout(*args, **kwargs):
                raise AssertionError("fallback validation should not run after a successful capabilities probe")

            launcher.run_binary = forbidden_fanout
            launcher.tools_compatible = forbidden_fanout
            launcher.commands_compatible = forbidden_fanout
            launcher.prompt_bundle_compatible = forbidden_fanout

            self.assertTrue(launcher.runtime_fully_compatible(binary, {"plugin_version": "0.18.1"}, temp))

    def test_absent_or_invalid_capabilities_probe_falls_back(self):
        launcher = load_launcher()

        with tempfile.TemporaryDirectory() as temp_dir:
            temp = Path(temp_dir)
            binary = temp / "ws-mcp"
            binary.write_text("stub", encoding="utf-8")
            calls = []

            def fake_run_binary(got_binary, args, **kwargs):
                calls.append(tuple(args))
                if tuple(args) == ("runtime", "capabilities"):
                    return subprocess.CompletedProcess([str(got_binary), *args], 2, stdout="", stderr="unknown command")
                if tuple(args) != ("version",):
                    raise AssertionError(f"unexpected run_binary call: {args}")
                return subprocess.CompletedProcess([str(got_binary), *args], 0, stdout="0.18.1\n", stderr="")

            launcher.run_binary = fake_run_binary
            launcher.tools_compatible = lambda got_binary, contract, runtime_dir: True
            launcher.commands_compatible = lambda got_binary, contract: True
            launcher.prompt_bundle_compatible = lambda got_binary, contract: True

            self.assertTrue(launcher.runtime_fully_compatible(binary, {"plugin_version": "0.18.1"}, temp))
            self.assertEqual(calls, [("runtime", "capabilities"), ("version",)])

    def test_download_repair_uses_process_unique_temp_paths(self):
        launcher = load_launcher()

        with tempfile.TemporaryDirectory() as temp_dir:
            temp = Path(temp_dir)
            binary = temp / "ws-mcp"
            asset = "ws-mcp-darwin-arm64"
            payload = b"runtime"
            expected = hashlib.sha256(payload).hexdigest()
            destinations = []

            def fake_download_file(url, destination):
                destinations.append(destination.name)
                if url.endswith(asset):
                    destination.write_bytes(payload)
                elif url.endswith("SHA256SUMS"):
                    destination.write_text(f"{expected}  {asset}\n", encoding="utf-8")
                else:
                    raise AssertionError(f"unexpected URL: {url}")

            launcher.download_file = fake_download_file
            launcher.install_downloaded_runtime(
                binary,
                temp,
                asset,
                {"release_repository": "example/repo", "release_tag": "v0.18.1"},
            )

            self.assertEqual(binary.read_bytes(), payload)
            self.assertEqual(len(destinations), 2)
            self.assertEqual(len(set(destinations)), 2)
            self.assertNotIn(f"{binary.name}.download", destinations)
            self.assertNotIn("SHA256SUMS.download", destinations)

    def test_runtime_binary_name_is_contract_addressed(self):
        launcher = load_launcher()

        with tempfile.TemporaryDirectory() as temp_dir:
            contract_path = Path(temp_dir) / "runtime.json"
            contract_path.write_text('{"plugin_version":"0.18.1"}\n', encoding="utf-8")
            got = launcher.runtime_binary_name({"plugin_version": "0.18.1"}, contract_path, "windows")

            self.assertTrue(got.startswith("ws-mcp-0.18.1-"))
            self.assertTrue(got.endswith(".exe"))
            self.assertIn(hashlib.sha256(contract_path.read_bytes()).hexdigest()[:12], got)

    def test_install_replace_failure_reuses_existing_compatible_runtime(self):
        launcher = load_launcher()

        with tempfile.TemporaryDirectory() as temp_dir:
            temp = Path(temp_dir)
            tmp = temp / "candidate"
            binary = temp / "ws-mcp-0.18.1-test"
            tmp.write_text("candidate", encoding="utf-8")
            binary.write_text("existing", encoding="utf-8")
            calls = []

            def fake_replace(source, destination):
                calls.append((source, destination))
                raise PermissionError("target is busy")

            launcher.os.replace = fake_replace
            launcher.runtime_fully_compatible = lambda got_binary, contract, runtime_dir: got_binary == binary

            installed = launcher.install_tmp_runtime(tmp, binary, {"plugin_version": "0.18.1"}, temp, "installed")

            self.assertFalse(installed)
            self.assertEqual(calls, [(tmp, binary)])

    def test_bootstrap_or_local_devenv_marker_forces_runtime_install(self):
        launcher = load_launcher()

        with tempfile.TemporaryDirectory() as temp_dir:
            home = Path(temp_dir)
            plugin_dir = home / ".codex" / "plugins" / "cache" / "kang-sw-devenv" / "ws" / "0.29.2"
            plugin_dir.mkdir(parents=True)
            self.write_local_contract(plugin_dir, package_root=home)

            with mock.patch.object(launcher.Path, "home", return_value=home):
                self.assertTrue(launcher.local_devenv_runtime_enabled(plugin_dir, "darwin"))
                self.assertTrue(launcher.runtime_install_forced(plugin_dir, "darwin"))
                self.assertFalse(launcher.local_devenv_runtime_enabled(plugin_dir, "windows"))

            with mock.patch.dict(launcher.os.environ, {"WS_MCP_BOOTSTRAP_BINARY": "/tmp/ws-mcp"}, clear=False):
                self.assertTrue(launcher.runtime_install_forced(Path("/not/local/plugin"), "darwin"))

    def test_invalid_local_devenv_contract_falls_back_to_release_path(self):
        launcher = load_launcher()

        with tempfile.TemporaryDirectory() as temp_dir:
            home = Path(temp_dir)
            plugin_dir = home / ".codex" / "plugins" / "cache" / "kang-sw-devenv" / "ws" / "0.29.2"
            plugin_dir.mkdir(parents=True)
            (plugin_dir / ".local-devenv-runtime").write_text("", encoding="utf-8")

            with mock.patch.object(launcher.Path, "home", return_value=home):
                self.assertFalse(launcher.local_devenv_runtime_enabled(plugin_dir, "darwin"))
                self.assertFalse(launcher.runtime_install_forced(plugin_dir, "darwin"))

    def test_release_install_without_local_marker_uses_download_path(self):
        launcher = load_launcher()

        with tempfile.TemporaryDirectory() as temp_dir:
            home = Path(temp_dir)
            plugin_dir = home / ".codex" / "plugins" / "cache" / "kang-sw-devenv" / "ws" / "0.29.2"
            plugin_dir.mkdir(parents=True)
            runtime_dir = home / "runtime"
            binary = runtime_dir / "ws-mcp-0.18.1-test"
            calls = []

            def fake_download(got_binary, got_runtime_dir, got_asset, got_contract):
                calls.append((got_binary, got_runtime_dir, got_asset, got_contract))

            def forbidden_local_candidate(*args, **kwargs):
                raise AssertionError("release install without marker must not copy or build local runtime candidates")

            launcher.copy_runtime = forbidden_local_candidate
            launcher.build_local_devenv_runtime = forbidden_local_candidate
            launcher.install_downloaded_runtime = fake_download

            launcher.install_runtime(
                plugin_dir,
                runtime_dir,
                binary,
                "ws-mcp-darwin-arm64",
                {"plugin_version": "0.18.1"},
                "darwin",
                "darwin-arm64",
            )

            self.assertEqual(
                calls,
                [(binary, runtime_dir, "ws-mcp-darwin-arm64", {"plugin_version": "0.18.1"})],
            )

    def test_forced_local_runtime_does_not_fall_back_to_release_download(self):
        launcher = load_launcher()

        with tempfile.TemporaryDirectory() as temp_dir:
            temp = Path(temp_dir)
            binary = temp / "ws-mcp-0.18.1-test"

            launcher.install_local_devenv_runtime = lambda *args, **kwargs: False

            def forbidden_download(*args, **kwargs):
                raise AssertionError("forced local runtime must not fall back to release download")

            launcher.install_downloaded_runtime = forbidden_download

            with self.assertRaises(SystemExit):
                launcher.install_runtime(
                    temp,
                    temp,
                    binary,
                    "ws-mcp-darwin-arm64",
                    {"plugin_version": "0.18.1"},
                    "darwin",
                    "darwin-arm64",
                    force_local=True,
                )

    def test_forced_local_runtime_prefers_source_build_over_dist_candidate(self):
        launcher = load_launcher()

        with tempfile.TemporaryDirectory() as temp_dir:
            home = Path(temp_dir)
            plugin_dir = home / ".codex" / "plugins" / "cache" / "kang-sw-devenv" / "ws" / "0.29.2"
            plugin_dir.mkdir(parents=True)
            local_contract = self.write_local_contract(plugin_dir, package_root=home)
            runtime_dir = home / "runtime"
            binary = runtime_dir / "ws-mcp-0.18.1-test"
            asset = "ws-mcp-darwin-arm64"
            dist = local_contract["tool_dir"] / "dist"
            dist.mkdir(parents=True)
            (dist / asset).write_text("stale dist", encoding="utf-8")
            build_calls = []

            def fake_build(got_runtime_dir, got_binary, got_contract, got_local_contract):
                build_calls.append((got_runtime_dir, got_binary, got_contract, got_local_contract))
                return True

            def forbidden_copy(*args, **kwargs):
                raise AssertionError("forced local runtime should build source before reading dist candidates")

            launcher.build_local_devenv_runtime = fake_build
            launcher.copy_runtime = forbidden_copy

            with mock.patch.object(launcher.Path, "home", return_value=home):
                self.assertTrue(
                    launcher.install_local_devenv_runtime(
                        plugin_dir,
                        runtime_dir,
                        binary,
                        asset,
                        {"plugin_version": "0.18.1"},
                        "darwin",
                        "darwin-arm64",
                        prefer_build=True,
                    )
                )

            self.assertEqual(build_calls, [(runtime_dir, binary, {"plugin_version": "0.18.1"}, local_contract)])

    def test_local_runtime_ignores_legacy_fixed_name_source_candidate(self):
        launcher = load_launcher()

        with tempfile.TemporaryDirectory() as temp_dir:
            home = Path(temp_dir)
            plugin_dir = home / ".codex" / "plugins" / "cache" / "kang-sw-devenv" / "ws" / "0.29.2"
            plugin_dir.mkdir(parents=True)
            local_contract = self.write_local_contract(plugin_dir, package_root=home)
            runtime_dir = home / "runtime"
            binary = runtime_dir / "ws-mcp-0.18.1-test"
            asset = "ws-mcp-darwin-arm64"
            legacy = local_contract["source_root"] / "agents-plugin" / ".runtime" / "darwin-arm64" / "ws-mcp"
            legacy.parent.mkdir(parents=True)
            legacy.write_text("legacy fixed-name runtime", encoding="utf-8")
            build_calls = []

            def fake_copy(source, destination):
                if source == legacy:
                    raise AssertionError("legacy fixed-name source runtime must not be copied")
                destination.write_text("copied", encoding="utf-8")

            def fake_build(got_runtime_dir, got_binary, got_contract, got_local_contract):
                build_calls.append((got_runtime_dir, got_binary, got_contract, got_local_contract))
                return True

            launcher.copy_runtime = fake_copy
            launcher.build_local_devenv_runtime = fake_build
            launcher.runtime_fully_compatible = lambda *args, **kwargs: False

            with mock.patch.object(launcher.Path, "home", return_value=home):
                self.assertTrue(
                    launcher.install_local_devenv_runtime(
                        plugin_dir,
                        runtime_dir,
                        binary,
                        asset,
                        {"plugin_version": "0.18.1"},
                        "darwin",
                        "darwin-arm64",
                    )
                )

            self.assertEqual(build_calls, [(runtime_dir, binary, {"plugin_version": "0.18.1"}, local_contract)])


if __name__ == "__main__":
    unittest.main()
