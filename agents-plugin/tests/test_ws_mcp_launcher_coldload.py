"""Cold-load robustness tests for ws-mcp-launcher (Phase B hardening).

Covers:
  1. wait_for_rsrc_tree — sentinel-based best-effort wait.
  2. read_runtime_contract — bounded retry on transient OSError / JSON errors.
  3. install_tmp_runtime — bounded os.replace retry with existing fallback intact.

All tests use the same load_launcher() + unittest.mock style as
test_ws_mcp_launcher_capabilities.py.
"""

import importlib.util
import json
import os
import tempfile
import unittest
from pathlib import Path
from unittest import mock


LAUNCHER_PATH = Path(__file__).resolve().parents[1] / "bin" / "ws-mcp-launcher.py"


def load_launcher():
    spec = importlib.util.spec_from_file_location("ws_mcp_launcher", LAUNCHER_PATH)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    spec.loader.exec_module(module)
    return module


class WaitForRsrcTreeTest(unittest.TestCase):
    """Tests for wait_for_rsrc_tree."""

    def test_returns_immediately_when_sentinel_already_exists(self):
        # Happy-path: zero added latency when the tree is already materialized.
        launcher = load_launcher()
        sleep_calls = []
        launcher.time.sleep = lambda s: sleep_calls.append(s)

        with tempfile.TemporaryDirectory() as temp_dir:
            plugin_dir = Path(temp_dir)
            rsrc_dir = plugin_dir / "rsrc"
            rsrc_dir.mkdir()
            (rsrc_dir / "manifest.json").write_text("{}", encoding="utf-8")

            launcher.wait_for_rsrc_tree(plugin_dir)

        self.assertEqual(sleep_calls, [], "no sleep when sentinel is already present")

    def test_returns_on_timeout_without_raising(self):
        # Best-effort: must return (not raise) when the sentinel never appears.
        launcher = load_launcher()
        # Patch sleep so the loop terminates immediately.
        launcher.time.sleep = lambda _: None

        with tempfile.TemporaryDirectory() as temp_dir:
            plugin_dir = Path(temp_dir)
            # sentinel never created
            # Should not raise SystemExit.
            launcher.wait_for_rsrc_tree(plugin_dir, timeout_seconds=0.0)

    def test_returns_once_sentinel_appears_mid_wait(self):
        # Sentinel appears on the second polling iteration.
        launcher = load_launcher()
        sleep_calls = []
        sentinel_path_holder = []

        def fake_sleep(interval):
            sleep_calls.append(interval)
            # Create the sentinel on the first sleep so the next is_file() hits.
            if len(sleep_calls) == 1 and sentinel_path_holder:
                sentinel_path_holder[0].write_text("{}", encoding="utf-8")

        with tempfile.TemporaryDirectory() as temp_dir:
            plugin_dir = Path(temp_dir)
            rsrc_dir = plugin_dir / "rsrc"
            rsrc_dir.mkdir()
            sentinel_path_holder.append(rsrc_dir / "manifest.json")

            launcher.time.sleep = fake_sleep
            launcher.wait_for_rsrc_tree(plugin_dir, timeout_seconds=5.0)

        self.assertEqual(len(sleep_calls), 1, "only one sleep before sentinel appeared")


class ReadRuntimeContractRetryTest(unittest.TestCase):
    """Tests for read_runtime_contract bounded retry on transient errors.

    Strategy: patch Path.read_text via mock.patch.object with the bound-method
    signature (self_path, encoding="utf-8") so the patch is restored cleanly
    after each test.
    """

    def test_transient_permission_error_retries_and_succeeds(self):
        # First read raises PermissionError; second succeeds.
        launcher = load_launcher()
        launcher.time.sleep = lambda _: None
        launcher.wait_for_runtime_contract = lambda path, **kw: None

        with tempfile.TemporaryDirectory() as temp_dir:
            contract_path = Path(temp_dir) / "runtime.json"
            contract_path.write_text('{"plugin_version":"0.18.1"}\n', encoding="utf-8")
            attempt = [0]
            _orig = Path.read_text

            def fake_read_text(self_path, encoding="utf-8"):
                attempt[0] += 1
                if attempt[0] < 2:
                    raise PermissionError("AV hold")
                return _orig(self_path, encoding=encoding)

            with mock.patch.object(Path, "read_text", fake_read_text):
                result = launcher.read_runtime_contract(contract_path)

            self.assertEqual(result["plugin_version"], "0.18.1")

    def test_two_transient_errors_then_success(self):
        # First two reads raise OSError; third succeeds.
        launcher = load_launcher()
        launcher.time.sleep = lambda _: None
        launcher.wait_for_runtime_contract = lambda path, **kw: None

        with tempfile.TemporaryDirectory() as temp_dir:
            contract_path = Path(temp_dir) / "runtime.json"
            contract_path.write_text('{"plugin_version":"0.19.0"}\n', encoding="utf-8")
            attempt = [0]
            _orig = Path.read_text

            def fake_read_text(self_path, encoding="utf-8"):
                attempt[0] += 1
                if attempt[0] <= 2:
                    raise OSError("sharing violation")
                return _orig(self_path, encoding=encoding)

            with mock.patch.object(Path, "read_text", fake_read_text):
                result = launcher.read_runtime_contract(contract_path)

            self.assertEqual(result["plugin_version"], "0.19.0")

    def test_persistently_unreadable_file_raises_system_exit(self):
        # All retry attempts fail → fail() → SystemExit.
        launcher = load_launcher()
        launcher.time.sleep = lambda _: None
        launcher.wait_for_runtime_contract = lambda path, **kw: None

        with tempfile.TemporaryDirectory() as temp_dir:
            contract_path = Path(temp_dir) / "runtime.json"

            def always_raises(self_path, encoding="utf-8"):
                raise PermissionError("still locked")

            with mock.patch.object(Path, "read_text", always_raises):
                with self.assertRaises(SystemExit):
                    launcher.read_runtime_contract(contract_path)


class InstallTmpRuntimeReplaceRetryTest(unittest.TestCase):
    """Tests for install_tmp_runtime bounded os.replace retry.

    Strategy: use mock.patch("os.replace", ...) so the patch is scoped and
    restored cleanly; the launcher module references os.replace through the
    shared os module object, so patching os.replace here affects the launcher.
    """

    def _make_replace(self, fail_times: int, *, real_replace=os.replace):
        """Return a fake os.replace that fails `fail_times` times then succeeds."""
        attempt = [0]

        def fake_replace(source, destination):
            attempt[0] += 1
            if attempt[0] <= fail_times:
                raise OSError("transient error")
            real_replace(source, destination)

        return fake_replace

    def test_transient_os_error_retries_and_succeeds(self):
        # os.replace raises OSError once then succeeds → returns True.
        launcher = load_launcher()
        launcher.time.sleep = lambda _: None

        with tempfile.TemporaryDirectory() as temp_dir:
            temp = Path(temp_dir)
            tmp = temp / "candidate"
            binary = temp / "ws-mcp-0.18.1-test"
            tmp.write_text("candidate", encoding="utf-8")

            with mock.patch("os.replace", self._make_replace(1)):
                installed = launcher.install_tmp_runtime(tmp, binary, {"plugin_version": "0.18.1"}, temp, "installed")

            self.assertTrue(installed)
            self.assertEqual(binary.read_text(encoding="utf-8"), "candidate")

    def test_two_transient_errors_then_success(self):
        # os.replace raises OSError twice then succeeds → returns True.
        launcher = load_launcher()
        launcher.time.sleep = lambda _: None

        with tempfile.TemporaryDirectory() as temp_dir:
            temp = Path(temp_dir)
            tmp = temp / "candidate"
            binary = temp / "ws-mcp-0.18.1-test"
            tmp.write_text("binary-content", encoding="utf-8")

            with mock.patch("os.replace", self._make_replace(2)):
                installed = launcher.install_tmp_runtime(tmp, binary, {"plugin_version": "0.18.1"}, temp, "done")

            self.assertTrue(installed)
            self.assertEqual(binary.read_text(encoding="utf-8"), "binary-content")

    def test_persistent_os_error_compatible_binary_reuses_it(self):
        # All retry attempts fail; existing binary is compatible → return False.
        launcher = load_launcher()
        launcher.time.sleep = lambda _: None

        with tempfile.TemporaryDirectory() as temp_dir:
            temp = Path(temp_dir)
            tmp = temp / "candidate"
            binary = temp / "ws-mcp-0.18.1-test"
            tmp.write_text("candidate", encoding="utf-8")
            binary.write_text("existing", encoding="utf-8")
            replace_calls = []

            def always_fails(source, destination):
                replace_calls.append((source, destination))
                raise PermissionError("target is busy")

            launcher.runtime_fully_compatible = lambda got_binary, contract, runtime_dir: got_binary == binary

            with mock.patch("os.replace", always_fails):
                installed = launcher.install_tmp_runtime(tmp, binary, {"plugin_version": "0.18.1"}, temp, "installed")

            self.assertFalse(installed)
            self.assertTrue(len(replace_calls) > 0)
            self.assertTrue(all(c == (tmp, binary) for c in replace_calls))

    def test_persistent_os_error_incompatible_binary_raises_system_exit(self):
        # All retry attempts fail; existing binary is NOT compatible → fail().
        launcher = load_launcher()
        launcher.time.sleep = lambda _: None

        with tempfile.TemporaryDirectory() as temp_dir:
            temp = Path(temp_dir)
            tmp = temp / "candidate"
            binary = temp / "ws-mcp-0.18.1-test"
            tmp.write_text("candidate", encoding="utf-8")
            # binary is absent → runtime_fully_compatible returns False

            def always_fails(source, destination):
                raise PermissionError("target is busy")

            launcher.runtime_fully_compatible = lambda got_binary, contract, runtime_dir: False

            with mock.patch("os.replace", always_fails):
                with self.assertRaises(SystemExit):
                    launcher.install_tmp_runtime(tmp, binary, {"plugin_version": "0.18.1"}, temp, "installed")


if __name__ == "__main__":
    unittest.main()
