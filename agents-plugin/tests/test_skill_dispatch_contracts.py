import unittest
from pathlib import Path


SKILLS_DIR = Path(__file__).resolve().parents[1] / "skills"


class SkillDispatchContractsTest(unittest.TestCase):
    def test_proceed_keeps_implementation_route_only(self):
        text = (SKILLS_DIR / "lead-proceed" / "SKILL.md").read_text(encoding="utf-8")

        self.assertIn("Always route code-editing work through `ws:lead-implement`", text)
        self.assertIn("## Routing Verdict", text)
        self.assertIn("NEXT: <ws:lead-discuss | ws:lead-write-ticket | ws:lead-implement | stop>", text)
        self.assertIn("If `NEXT: ws:lead-implement`, invoke `ws:lead-implement`", text)
        self.assertNotIn("**Implementation Route**", text)
        self.assertNotIn("**Implementation Verdict**", text)
        self.assertNotIn("**Verdict Basis**", text)
        self.assertNotIn("### judge: implementation-dispatch", text)

    def test_implement_keeps_execution_owner(self):
        text = (SKILLS_DIR / "lead-implement" / "SKILL.md").read_text(encoding="utf-8")

        self.assertIn("## Implementation Verdict", text)
        self.assertIn("- **Mode**: <direct edit | delegated>", text)
        self.assertIn("Do not use `NEXT:`", text)
        self.assertIn("### judge: needs-delegation", text)
        self.assertIn("| Direct-edit |", text)
        self.assertIn("If direct-edit: edit directly", text)
        self.assertNotIn("caller-provided `write-code` dispatch", text)
        self.assertNotIn("Confirm dispatch boundary", text)

    def test_workflow_manual_requires_english_agent_prompts(self):
        text = (SKILLS_DIR / "lead-workflow-manual" / "SKILL.md").read_text(encoding="utf-8")

        self.assertIn("Write prompts sent to `ws/subquery` and `ws/agents.call` in English.", text)


if __name__ == "__main__":
    unittest.main()
