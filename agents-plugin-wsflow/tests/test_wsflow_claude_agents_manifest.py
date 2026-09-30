"""The Claude manifest's curated `agents` list must name exactly the generated
effort agents under claude-agents/. The `agents` key replaces Claude's default
agents/ scan, so a generated file missing from the list never loads, and a
listed path with no file breaks plugin loading.
"""

import json
import unittest
from pathlib import Path

PLUGIN_ROOT = Path(__file__).resolve().parents[1]
MANIFEST_PATH = PLUGIN_ROOT / ".claude-plugin" / "plugin.json"
AGENTS_DIR = PLUGIN_ROOT / "claude-agents"


class WsflowClaudeAgentsManifestTest(unittest.TestCase):
    def test_manifest_agents_match_claude_agents_files(self):
        manifest = json.loads(MANIFEST_PATH.read_text(encoding="utf-8"))
        listed = manifest.get("agents")
        self.assertIsInstance(listed, list, "Claude manifest must list its agents as an array")
        self.assertEqual(len(listed), len(set(listed)), f"duplicate agents entries: {listed}")
        on_disk = {f"./claude-agents/{p.name}" for p in AGENTS_DIR.glob("*.md")}
        self.assertTrue(on_disk, f"no generated agents under {AGENTS_DIR}")
        self.assertEqual(set(listed), on_disk)


if __name__ == "__main__":
    unittest.main()
