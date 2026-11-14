"""Static runner regression contracts; execution waits for the allocated slot."""

import importlib.util
from pathlib import Path
import signal
from types import SimpleNamespace
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


if __name__ == "__main__":
    unittest.main()
