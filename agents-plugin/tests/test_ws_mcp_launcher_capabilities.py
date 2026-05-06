import importlib.util
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

            launcher.runtime_capabilities_compatible = lambda got_binary, contract: False

            def fake_run_binary(got_binary, args, **kwargs):
                calls.append(tuple(args))
                return subprocess.CompletedProcess([str(got_binary), *args], 0, stdout="0.18.1\n", stderr="")

            launcher.run_binary = fake_run_binary
            launcher.tools_compatible = lambda got_binary, contract, runtime_dir: True
            launcher.commands_compatible = lambda got_binary, contract: True
            launcher.prompt_bundle_compatible = lambda got_binary, contract: True

            self.assertTrue(launcher.runtime_fully_compatible(binary, {"plugin_version": "0.18.1"}, temp))
            self.assertEqual(calls, [("version",)])


if __name__ == "__main__":
    unittest.main()
