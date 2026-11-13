import importlib.util
import os
import subprocess
import sys
import unittest
from pathlib import Path
from unittest import mock


LAUNCHER_PATH = Path(__file__).resolve().parents[1] / "bin" / "ws-mcp-launcher.py"


def load_launcher():
    spec = importlib.util.spec_from_file_location("ws_mcp_launcher_harness_pid", LAUNCHER_PATH)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    spec.loader.exec_module(module)
    return module


class HarnessPidTest(unittest.TestCase):
    def setUp(self):
        self.launcher = load_launcher()
        patcher = mock.patch.object(self.launcher.os, "getppid", return_value=100)
        patcher.start()
        self.addCleanup(patcher.stop)

    def test_windows_cmd_parent_reports_grandparent(self):
        lookup = mock.Mock(return_value=(50, "cmd.exe"))
        self.assertEqual(self.launcher.harness_pid("windows", lookup), 50)
        lookup.assert_called_once_with(100)

    def test_windows_cmd_match_ignores_case(self):
        self.assertEqual(self.launcher.harness_pid("windows", lambda pid: (50, "CMD.EXE")), 50)

    def test_windows_non_cmd_parent_stays_parent(self):
        self.assertEqual(self.launcher.harness_pid("windows", lambda pid: (50, "claude.exe")), 100)

    def test_windows_lookup_failure_falls_back_to_parent(self):
        def boom(pid):
            raise OSError("snapshot failed")

        self.assertEqual(self.launcher.harness_pid("windows", boom), 100)
        self.assertEqual(self.launcher.harness_pid("windows", lambda pid: None), 100)

    def test_windows_cmd_without_live_grandparent_stays_parent(self):
        self.assertEqual(self.launcher.harness_pid("windows", lambda pid: (0, "cmd.exe")), 100)

    def test_only_one_level_is_skipped(self):
        table = {100: (50, "cmd.exe"), 50: (10, "cmd.exe")}
        self.assertEqual(self.launcher.harness_pid("windows", table.get), 50)

    def test_non_windows_never_consults_the_process_table(self):
        lookup = mock.Mock(return_value=(50, "cmd.exe"))
        self.assertEqual(self.launcher.harness_pid("linux", lookup), 100)
        self.assertEqual(self.launcher.harness_pid("darwin", lookup), 100)
        lookup.assert_not_called()

    @unittest.skipUnless(os.name == "nt", "Windows process table")
    def test_windows_process_entry_reads_own_parent(self):
        entry = self.launcher.windows_process_entry(os.getpid())
        self.assertIsNotNone(entry)
        self.assertGreater(entry[0], 0)
        self.assertTrue(entry[1].lower().startswith("python"))
        self.assertIsNone(self.launcher.windows_process_entry(0x7FFFFFF0))

    @unittest.skipUnless(os.name == "nt", "Windows cmd.exe wrapper")
    def test_windows_live_cmd_wrapper_reports_its_parent(self):
        script = (
            "import importlib.util,sys;"
            f"s=importlib.util.spec_from_file_location('l',{str(LAUNCHER_PATH)!r});"
            "m=importlib.util.module_from_spec(s);s.loader.exec_module(m);"
            "print(m.harness_pid('windows'))"
        )
        out = subprocess.run(
            ["cmd.exe", "/d", "/c", sys.executable, "-c", script],
            capture_output=True,
            text=True,
            check=True,
        ).stdout.strip()
        self.assertEqual(int(out), os.getpid())


if __name__ == "__main__":
    unittest.main()
