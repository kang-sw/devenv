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
  4. No Stop event: the Claude mailbox wake is the mailbox push mod, not a
     turn-end hook.
  5. Every "modules" entry names a mod source file that exists.
"""

import json
import os
import shutil
import subprocess
import sys
import tempfile
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
        self.assertNotIn("Stop", events, "the Claude mailbox wake is the push mod, not a Stop hook")
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

    def test_modules_name_existing_mod_sources(self):
        doc = json.loads(HOOKS_PATH.read_text(encoding="utf-8"))
        modules = doc.get("modules")
        self.assertEqual(modules, ["./claude-mod/register.ts"])
        for module in modules:
            self.assertTrue(
                (HOOKS_PATH.parent / module).is_file(),
                f"hooks.json module {module} does not exist next to hooks.json",
            )

    def test_session_start_compact_runs_the_revive_nudge(self):
        doc = json.loads(HOOKS_PATH.read_text(encoding="utf-8"))
        entries = [e for e in doc["hooks"].get("SessionStart", []) if e.get("matcher") == "compact"]
        commands = [hook["command"] for e in entries for hook in e.get("hooks", [])]
        self.assertTrue(
            any('hooks/compact-revive-nudge.py" ws ' in c for c in commands),
            f"no SessionStart(compact) hook runs compact-revive-nudge.py with this package's name: {commands}",
        )

    def _run_nudge(self, payload, ceiling, package="ws"):
        # The ceiling keeps git from finding a repository that encloses the
        # temporary directory (TMPDIR inside a checkout), whose root would
        # decide the nudge in place of the layout the test builds.
        script = PLUGIN_ROOT / "hooks" / "compact-revive-nudge.py"
        return subprocess.run(
            [sys.executable, str(script), *([package] if package else [])],
            input=json.dumps(payload),
            capture_output=True,
            text=True,
            env={**os.environ, "GIT_CEILING_DIRECTORIES": os.path.realpath(ceiling)},
        )

    def _git_init(self, path):
        subprocess.run(["git", "init", "-q", str(path)], check=True, capture_output=True)

    def test_revive_nudge_prints_only_on_compact_in_a_workflow_project(self):
        with tempfile.TemporaryDirectory() as tmp:
            repo = Path(tmp) / "repo"
            (repo / "ai-docs").mkdir(parents=True)
            (repo / "ai-docs" / "WORKFLOW.md").write_text("x", encoding="utf-8")
            self._git_init(repo)

            compact = self._run_nudge({"source": "compact", "cwd": str(repo)}, tmp)
            self.assertEqual(compact.returncode, 0, compact.stderr)
            self.assertIn("ws:lead-revive", compact.stdout)
            self.assertIn("[message from ws plugin]", compact.stdout)
            self.assertIn("not a spawned worker or subagent", compact.stdout)

            startup = self._run_nudge({"source": "startup", "cwd": str(repo)}, tmp)
            self.assertEqual(startup.returncode, 0, startup.stderr)
            self.assertEqual(startup.stdout, "")

            # From a subdirectory, the repository root decides.
            sub = repo / "a" / "b"
            sub.mkdir(parents=True)
            from_sub = self._run_nudge({"source": "compact", "cwd": str(sub)}, tmp)
            self.assertIn("ws:lead-revive", from_sub.stdout)

            # The text names the package passed; without one nothing prints.
            other = self._run_nudge({"source": "compact", "cwd": str(repo)}, tmp, package="pkgx")
            self.assertIn("pkgx:lead-revive", other.stdout)
            self.assertNotIn("ws:", other.stdout)
            bare = self._run_nudge({"source": "compact", "cwd": str(repo)}, tmp, package="")
            self.assertEqual(bare.stdout, "")

    def test_revive_nudge_outside_a_repository_walks_ancestors(self):
        with tempfile.TemporaryDirectory() as tmp:
            top = Path(tmp) / "top"
            (top / "ai-docs").mkdir(parents=True)
            (top / "ai-docs" / "WORKFLOW.md").write_text("x", encoding="utf-8")
            deep = top / "x" / "y"
            deep.mkdir(parents=True)
            res = self._run_nudge({"source": "compact", "cwd": str(deep)}, tmp)
            self.assertEqual(res.returncode, 0, res.stderr)
            self.assertIn("ws:lead-revive", res.stdout)

    def test_revive_nudge_prints_nothing_without_workflow_md(self):
        with tempfile.TemporaryDirectory() as tmp:
            plain = Path(tmp) / "plain"
            plain.mkdir()
            self.assertEqual(self._run_nudge({"source": "compact", "cwd": str(plain)}, tmp).stdout, "")
            # A repository whose root lacks it stays silent, even under an
            # ancestor that holds one: the repository root decides.
            (Path(tmp) / "ai-docs").mkdir()
            (Path(tmp) / "ai-docs" / "WORKFLOW.md").write_text("x", encoding="utf-8")
            repo = Path(tmp) / "repo"
            repo.mkdir()
            self._git_init(repo)
            self.assertEqual(self._run_nudge({"source": "compact", "cwd": str(repo)}, tmp).stdout, "")


if __name__ == "__main__":
    unittest.main()
