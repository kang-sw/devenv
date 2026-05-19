import unittest
from pathlib import Path


SKILLS_DIR = Path(__file__).resolve().parents[1] / "skills"


class SkillDispatchContractsTest(unittest.TestCase):
    def test_proceed_announces_source_free_implementation_dispatch(self):
        text = (SKILLS_DIR / "lead-proceed" / "SKILL.md").read_text(encoding="utf-8")

        self.assertIn("**Implementation Dispatch**", text)
        self.assertIn("**Dispatch Reason**", text)
        self.assertIn("**Branch Mode**", text)
        self.assertIn("Do not inspect source, source", text)
        self.assertIn("Any direct-edit predicate is false or unknown", text)
        self.assertIn("Ready tickets, spec-linked changes", text)

    def test_implement_preserves_write_code_lower_bound(self):
        text = (SKILLS_DIR / "lead-implement" / "SKILL.md").read_text(encoding="utf-8")

        self.assertIn("Honor caller-provided `write-code` dispatch as a hard lower bound.", text)
        self.assertIn("Caller-provided implementation dispatch is `write-code`", text)
        self.assertIn("Confirm dispatch boundary", text)


if __name__ == "__main__":
    unittest.main()
