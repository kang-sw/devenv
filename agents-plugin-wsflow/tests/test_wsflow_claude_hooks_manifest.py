"""wsflow's Claude hooks/hooks.json carries the Claude mod, shared
byte-identically with the flagship package (every feature switched by a
claude.* config key whose default is this package's own), and the
SessionStart(compact) revive nudge, whose script is shared byte-identically
too and is passed this package's name. Every listed module and command script
exists next to the file, and the distributed text names this package only.
"""

import json
import os
import re
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

PLUGIN_ROOT = Path(__file__).resolve().parents[1]
HOOKS_PATH = PLUGIN_ROOT / "hooks" / "hooks.json"
NUDGE = PLUGIN_ROOT / "hooks" / "compact-revive-nudge.py"


class WsflowClaudeHooksManifestTest(unittest.TestCase):
    def load(self):
        return json.loads(HOOKS_PATH.read_text(encoding="utf-8"))

    def test_hooks_json_lists_only_existing_modules(self):
        doc = self.load()
        self.assertEqual(set(doc), {"description", "hooks", "modules"})
        self.assertEqual(doc["modules"], ["./claude-mod/register.ts"])
        for module in doc["modules"]:
            self.assertTrue(
                (HOOKS_PATH.parent / module).is_file(),
                f"hooks.json module {module} does not exist next to hooks.json",
            )

    def test_session_start_compact_runs_the_revive_nudge_with_this_package(self):
        doc = self.load()
        self.assertEqual(set(doc["hooks"]), {"SessionStart"})
        entries = [e for e in doc["hooks"]["SessionStart"] if e.get("matcher") == "compact"]
        commands = [hook["command"] for e in entries for hook in e.get("hooks", [])]
        self.assertEqual(
            commands,
            ['python3 "${CLAUDE_PLUGIN_ROOT}/hooks/compact-revive-nudge.py" wsflow || true'],
        )
        self.assertTrue(NUDGE.is_file())

    def test_nudge_names_wsflow_in_a_workflow_project(self):
        with tempfile.TemporaryDirectory() as tmp:
            repo = Path(tmp) / "repo"
            (repo / "ai-docs").mkdir(parents=True)
            (repo / "ai-docs" / "WORKFLOW.md").write_text("x", encoding="utf-8")
            subprocess.run(["git", "init", "-q", str(repo)], check=True, capture_output=True)
            res = subprocess.run(
                [sys.executable, str(NUDGE), "wsflow"],
                input=json.dumps({"source": "compact", "cwd": str(repo)}),
                capture_output=True,
                text=True,
                # Keeps git from finding a repository that encloses TMPDIR.
                env={**os.environ, "GIT_CEILING_DIRECTORIES": os.path.realpath(tmp)},
            )
            self.assertEqual(res.returncode, 0, res.stderr)
            self.assertIn("wsflow:lead-revive", res.stdout)
            self.assertIsNone(re.search(r"(?<![A-Za-z])ws(?![A-Za-z])", res.stdout))

    def test_description_does_not_name_the_flagship_package(self):
        text = self.load()["description"]
        self.assertIsNone(re.search(r"(?<![A-Za-z])ws(?![A-Za-z])", text), text)
        self.assertIn("conservative defaults", text)


if __name__ == "__main__":
    unittest.main()
