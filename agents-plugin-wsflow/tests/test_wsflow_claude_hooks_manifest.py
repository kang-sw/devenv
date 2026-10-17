"""wsflow's Claude hooks/hooks.json carries only the mailbox push mod: no
command hooks (the flagship package's SessionStart(compact) revive nudge is
not shipped here), and every listed module exists next to the file.
"""

import json
import unittest
from pathlib import Path

PLUGIN_ROOT = Path(__file__).resolve().parents[1]
HOOKS_PATH = PLUGIN_ROOT / "hooks" / "hooks.json"


class WsflowClaudeHooksManifestTest(unittest.TestCase):
    def test_hooks_json_lists_only_existing_modules(self):
        doc = json.loads(HOOKS_PATH.read_text(encoding="utf-8"))
        self.assertEqual(set(doc), {"modules"})
        self.assertEqual(doc["modules"], ["./mailbox-push/register.ts"])
        for module in doc["modules"]:
            self.assertTrue(
                (HOOKS_PATH.parent / module).is_file(),
                f"hooks.json module {module} does not exist next to hooks.json",
            )


if __name__ == "__main__":
    unittest.main()
