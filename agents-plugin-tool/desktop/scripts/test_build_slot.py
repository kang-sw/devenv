"""Mocked runner contracts: no real subprocesses or process-group signals."""

from contextlib import redirect_stdout
import importlib.util
import io
import json
from pathlib import Path
import signal
from types import SimpleNamespace
import tempfile
import unittest
from unittest.mock import Mock, call, patch

spec = importlib.util.spec_from_file_location("build_slot", Path(__file__).with_name("build-slot.py"))
runner = importlib.util.module_from_spec(spec)
spec.loader.exec_module(runner)


class OwnedGroupsTests(unittest.TestCase):
    def process(self, pid):
        return SimpleNamespace(pid=pid, poll=Mock(return_value=0))

    def test_absent_foreground_group_is_never_probed_or_signaled_again(self):
        groups = runner.OwnedGroups()
        groups.track(self.process(123))
        with patch.object(runner.os, "killpg", side_effect=ProcessLookupError) as killpg:
            self.assertFalse(groups.present(123))
            killpg.assert_called_once_with(123, 0)
        # Simulate the number becoming valid again, now belonging to someone else.
        with patch.object(runner.os, "killpg") as killpg:
            self.assertFalse(groups.present(123))
            groups.send(123, signal.SIGTERM)
            groups.send(123, signal.SIGKILL)
            killpg.assert_not_called()
        self.assertEqual(groups.ids, [123])
        self.assertEqual(groups.retired, [123])
        self.assertEqual(groups.active, set())

    def test_evidence_keeps_retired_ids_but_cleanup_uses_only_active_groups(self):
        groups = runner.OwnedGroups()
        groups.track(self.process(123))
        with patch.object(runner.os, "killpg", side_effect=ProcessLookupError):
            groups.present(123)
        groups.track(self.process(456))
        with patch.object(runner.os, "killpg") as killpg:
            for group in tuple(groups.active):
                groups.send(group, signal.SIGTERM)
            self.assertEqual(killpg.call_args_list, [call(456, 0), call(456, signal.SIGTERM)])
        self.assertEqual(groups.ids, [123, 456])
        self.assertEqual(groups.retired, [123])

    def test_disappearance_between_probe_and_signal_retires_the_group(self):
        groups = runner.OwnedGroups()
        groups.track(self.process(123))
        with patch.object(runner.os, "killpg", side_effect=[None, ProcessLookupError]) as killpg:
            groups.send(123, signal.SIGTERM)
            self.assertEqual(killpg.call_args_list, [call(123, 0), call(123, signal.SIGTERM)])
        with patch.object(runner.os, "killpg") as killpg:
            groups.send(123, signal.SIGKILL)
            killpg.assert_not_called()
        self.assertEqual(groups.retired, [123])


class RunnerStepBoundaryTests(unittest.TestCase):
    def run_main(self, processes, probe):
        groups = runner.OwnedGroups()
        with tempfile.TemporaryDirectory(prefix="werkschaft-runner-contract-") as directory:
            with (
                patch.object(runner.sys, "argv", ["build-slot.py", "--allocated"]),
                patch.object(runner.tempfile, "mkdtemp", return_value=directory),
                patch.object(runner, "OwnedGroups", return_value=groups),
                patch.object(runner.subprocess, "Popen", side_effect=processes) as popen,
                patch.object(runner.os, "killpg", side_effect=probe) as killpg,
                patch.object(runner.signal, "signal"),
                redirect_stdout(io.StringIO()),
            ):
                result = runner.main()
                summary = json.loads((Path(directory) / "summary.json").read_text())
                spawn_count = popen.call_count
                probes = killpg.call_args_list
        return result, summary, spawn_count, probes, groups

    def process(self, pid):
        return SimpleNamespace(pid=pid, wait=Mock(return_value=0), poll=Mock(return_value=0))

    def test_present_after_step_stops_before_next_build_and_reused_id_is_not_signaled(self):
        # The parent exited, but a helper is present at the step boundary. It
        # disappears before immediate cleanup. Any later probe would see an
        # unrelated process group that reused the number during a long build.
        probes = iter([None, ProcessLookupError()])

        def probe(group, sig):
            self.assertEqual((group, sig), (123, 0))
            value = next(probes, None)  # Reused number would now appear present.
            if isinstance(value, Exception):
                raise value

        result, summary, spawns, calls, groups = self.run_main(
            [self.process(123), self.process(456)], probe
        )
        self.assertEqual(result, 125)
        self.assertEqual(spawns, 1, "The next long build must never start")
        self.assertEqual(calls, [call(123, 0), call(123, 0)])
        self.assertEqual(summary["records"][0]["exit"], 0)
        self.assertTrue(summary["records"][0]["group_present_after_step"])
        self.assertTrue(summary["records"][0]["stopped_for_present_group"])
        self.assertEqual(summary["retired_groups"], [123])
        self.assertEqual(summary["lingering_groups"], [])
        with patch.object(runner.os, "killpg") as killpg:
            groups.send(123, signal.SIGTERM)
            groups.send(123, signal.SIGKILL)
            killpg.assert_not_called()

    def test_absent_groups_allow_the_normal_two_step_build(self):
        result, summary, spawns, calls, _ = self.run_main(
            [self.process(123), self.process(456)], Mock(side_effect=ProcessLookupError)
        )
        self.assertEqual(result, 0)
        self.assertEqual(spawns, 2)
        self.assertEqual(calls, [call(123, 0), call(456, 0)])
        self.assertEqual([record["step"] for record in summary["records"]], ["unit-tests", "debug-app"])
        self.assertEqual(summary["retired_groups"], [123, 456])
        self.assertEqual(summary["lingering_groups"], [])


if __name__ == "__main__":
    unittest.main()
