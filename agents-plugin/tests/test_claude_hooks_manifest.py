"""Validation tests for the Claude hooks/hooks.json manifest
(260913-feat-cross-session-mailbox-wake Phase 3), mirroring
test_hooks_manifest.py's coverage of the Codex plugin-bundled hooks.json for
the Claude adapter's own file and schema.

Unlike Codex (which needs an explicit "hooks" pointer in plugin.json), Claude
Code auto-discovers a plugin's hooks by the fixed path
"<plugin-root>/hooks/hooks.json" alone, so this test targets that path
directly rather than resolving it through plugin.json.

Claude's hooks.json entry shape also differs from Codex's: each event's list
holds a matcher/hooks wrapper object (`{"hooks": [{"type": "command", ...}]}`),
not a bare hook object, and Claude's real-world plugin ecosystem has no
"commandWindows" counterpart field (confirmed by inspecting several installed
Claude plugins under ~/.claude/plugins) — this file's checks reflect that
different shape rather than reusing test_hooks_manifest.py's assertions
verbatim.

Covers:
  1. hooks/hooks.json exists at the plugin-root-relative path Claude
     auto-discovers.
  2. That file parses as JSON and matches Claude's hooks schema shape:
     {"hooks": {"<Event>": [{"hooks": [{"type": "command", "command": ...}]}]}}.
  3. Every POSIX "command" string is syntactically valid shell (via `sh -n`).
  4. The Stop event is present (the mailbox wake adapter's own event).
"""

import json
import shutil
import subprocess
import unittest
from pathlib import Path

PLUGIN_ROOT = Path(__file__).resolve().parents[1]
HOOKS_PATH = PLUGIN_ROOT / "hooks" / "hooks.json"


class ClaudeHooksManifestTest(unittest.TestCase):
    def test_hooks_path_exists(self):
        self.assertTrue(
            HOOKS_PATH.is_file(),
            f"Claude's auto-discovered hooks path {HOOKS_PATH} does not exist",
        )

    def test_hooks_json_parses_and_matches_expected_shape(self):
        doc = json.loads(HOOKS_PATH.read_text(encoding="utf-8"))
        self.assertIn("hooks", doc)
        events = doc["hooks"]
        self.assertIsInstance(events, dict)
        self.assertIn("Stop", events, "the mailbox wake adapter is wired on the Stop event")
        for event_name, entries in events.items():
            self.assertIsInstance(entries, list, f"hooks.{event_name} must be a list")
            for matcher_entry in entries:
                self.assertIsInstance(
                    matcher_entry, dict, f"hooks.{event_name} entry must be an object"
                )
                inner_hooks = matcher_entry.get("hooks")
                self.assertIsInstance(
                    inner_hooks, list, f"hooks.{event_name} entry must carry a 'hooks' list"
                )
                for entry in inner_hooks:
                    self.assertEqual(
                        entry.get("type"), "command", f"hooks.{event_name} entry must be type=command"
                    )
                    self.assertIsInstance(
                        entry.get("command"), str, f"hooks.{event_name} entry must carry a string 'command'"
                    )
                    self.assertTrue(
                        entry["command"].strip(), f"hooks.{event_name} 'command' must not be blank"
                    )

    def test_command_strings_are_syntactically_valid_posix_shell(self):
        sh = shutil.which("sh")
        if sh is None:
            self.skipTest("no 'sh' on PATH to syntax-check against")
        doc = json.loads(HOOKS_PATH.read_text(encoding="utf-8"))
        for event_name, entries in doc["hooks"].items():
            for matcher_entry in entries:
                for entry in matcher_entry.get("hooks", []):
                    command = entry["command"]
                    result = subprocess.run(
                        [sh, "-n", "-c", command],
                        capture_output=True,
                        text=True,
                    )
                    self.assertEqual(
                        result.returncode,
                        0,
                        f"hooks.{event_name} command failed sh -n syntax check: {result.stderr}\ncommand: {command}",
                    )

    def test_stop_command_invokes_the_claude_stop_hook_subcommand(self):
        doc = json.loads(HOOKS_PATH.read_text(encoding="utf-8"))
        commands = [
            entry["command"]
            for matcher_entry in doc["hooks"]["Stop"]
            for entry in matcher_entry.get("hooks", [])
        ]
        self.assertTrue(
            any("mailbox claude-stop-hook" in c for c in commands),
            f"no Stop hook command invokes the claude-stop-hook subcommand: {commands}",
        )


if __name__ == "__main__":
    unittest.main()
