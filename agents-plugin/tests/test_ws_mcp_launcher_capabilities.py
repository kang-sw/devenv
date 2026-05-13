import importlib.util
import json
import subprocess
import unittest
from pathlib import Path


LAUNCHER_PATH = Path(__file__).resolve().parents[1] / "bin" / "ws-mcp-launcher.py"


def load_launcher():
    spec = importlib.util.spec_from_file_location("ws_mcp_launcher", LAUNCHER_PATH)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    spec.loader.exec_module(module)
    return module


class RuntimeCapabilitiesCompatibilityTest(unittest.TestCase):
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
        import tempfile

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


if __name__ == "__main__":
    unittest.main()
