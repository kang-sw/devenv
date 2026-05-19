import unittest
from pathlib import Path


SKILLS_DIR = Path(__file__).resolve().parents[1] / "skills"


class SkillDispatchContractsTest(unittest.TestCase):
    def test_proceed_reads_implement_for_source_free_verdict(self):
        text = (SKILLS_DIR / "lead-proceed" / "SKILL.md").read_text(encoding="utf-8")

        self.assertIn("read `ws:lead-implement` skill text", text)
        self.assertIn("**Implementation Verdict**", text)
        self.assertIn("**Verdict Basis**", text)
        self.assertIn("unknown direct-edit predicate -> delegated", text)
        self.assertNotIn("### judge: implementation-dispatch", text)

    def test_implement_keeps_route_contract_owner(self):
        text = (SKILLS_DIR / "lead-implement" / "SKILL.md").read_text(encoding="utf-8")

        self.assertIn("### judge: execution-mode", text)
        self.assertIn("Direct edit -> `ws:lead-edit`", text)
        self.assertNotIn("caller-provided `write-code` dispatch", text)
        self.assertNotIn("Confirm dispatch boundary", text)


if __name__ == "__main__":
    unittest.main()
