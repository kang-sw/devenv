"""Validation tests for the Codex hooks.json manifest
(260913-feat-cross-session-mailbox-wake Phase 2, round-1 review Minor-3
finding: nothing previously asserted the shipped hooks.json actually parses,
that plugin.json's "hooks" pointer resolves to a real file, or that its
command strings are syntactically valid shell — a typo in any of these would
only surface at live Codex install time.

Covers:
  1. plugin.json's "hooks" field resolves (plugin-root-relative, per the
     ".mcp.json lives at plugin root" convention already used for
     mcpServers) to a file that exists.
  2. That file parses as JSON and matches the shape Codex's hooks schema
     expects: {"hooks": {"<Event>": [{"type": "command", "command": ...}]}}.
  3. Every POSIX "command" string is syntactically valid shell (via
     `sh -n`), and every "commandWindows" string is present when "command"
     is (Windows parity, round-1 Important-E).
"""

import json
import shutil
import subprocess
import unittest
from pathlib import Path

PLUGIN_ROOT = Path(__file__).resolve().parents[1]
PLUGIN_MANIFEST_PATH = PLUGIN_ROOT / ".codex-plugin" / "plugin.json"


class HooksManifestTest(unittest.TestCase):
    def setUp(self):
        manifest = json.loads(PLUGIN_MANIFEST_PATH.read_text(encoding="utf-8"))
        hooks_ref = manifest.get("hooks")
        self.assertIsInstance(
            hooks_ref, str, "plugin.json must declare a string 'hooks' path when hooks are shipped"
        )
        # Manifest string paths are plugin-root-relative, not
        # manifest-file-relative (confirmed by ".mcp.json" living at plugin
        # root despite being referenced from .codex-plugin/plugin.json).
        # NB: str.lstrip("./") strips characters, not the "./" prefix as a
        # unit — it would also eat the leading dot of ".codex-plugin",
        # collapsing the path to the wrong "codex-plugin/hooks.json". Use an
        # explicit prefix removal instead.
        relative = hooks_ref[2:] if hooks_ref.startswith("./") else hooks_ref
        self.hooks_path = (PLUGIN_ROOT / relative).resolve()

    def test_hooks_path_resolves_to_an_existing_file(self):
        self.assertTrue(
            self.hooks_path.is_file(),
            f"plugin.json's hooks pointer resolves to {self.hooks_path}, which does not exist",
        )

    def test_hooks_json_parses_and_matches_expected_shape(self):
        doc = json.loads(self.hooks_path.read_text(encoding="utf-8"))
        self.assertIn("hooks", doc)
        events = doc["hooks"]
        self.assertIsInstance(events, dict)
        self.assertIn("Stop", events, "the mailbox wake adapter is wired on the Stop event")
        for event_name, entries in events.items():
            self.assertIsInstance(entries, list, f"hooks.{event_name} must be a list")
            for entry in entries:
                self.assertEqual(entry.get("type"), "command", f"hooks.{event_name} entry must be type=command")
                self.assertIsInstance(
                    entry.get("command"), str, f"hooks.{event_name} entry must carry a string 'command'"
                )
                self.assertTrue(entry["command"].strip(), f"hooks.{event_name} 'command' must not be blank")

    def test_command_strings_are_syntactically_valid_posix_shell(self):
        sh = shutil.which("sh")
        if sh is None:
            self.skipTest("no 'sh' on PATH to syntax-check against")
        doc = json.loads(self.hooks_path.read_text(encoding="utf-8"))
        for event_name, entries in doc["hooks"].items():
            for entry in entries:
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

    def test_every_command_has_a_windows_counterpart(self):
        doc = json.loads(self.hooks_path.read_text(encoding="utf-8"))
        for event_name, entries in doc["hooks"].items():
            for entry in entries:
                self.assertIn(
                    "commandWindows",
                    entry,
                    f"hooks.{event_name} entry has a POSIX 'command' but no 'commandWindows' counterpart",
                )
                self.assertTrue(
                    str(entry["commandWindows"]).strip(),
                    f"hooks.{event_name} 'commandWindows' must not be blank",
                )


if __name__ == "__main__":
    unittest.main()
