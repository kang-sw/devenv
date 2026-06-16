import unittest
from pathlib import Path


SKILLS_DIR = Path(__file__).resolve().parents[1] / "skills"
RSRC_DIR = Path(__file__).resolve().parents[1] / "rsrc"


def fenced_template(text: str, heading: str) -> str:
    start = text.index(heading)
    fence_start = text.index("```text", start)
    fence_end = text.index("```", fence_start + len("```text"))
    return text[fence_start:fence_end]


def verdict_fields(template: str) -> list[str]:
    return [line.strip() for line in template.splitlines() if line.startswith("- **")]


class SkillDispatchContractsTest(unittest.TestCase):
    def test_proceed_keeps_implementation_route_only(self):
        shim = (SKILLS_DIR / "lead-proceed" / "SKILL.md").read_text(encoding="utf-8")
        text = (RSRC_DIR / "lead-proceed" / "lead-proceed.md").read_text(encoding="utf-8")

        self.assertIn('ws/playbook.print(name: "lead-proceed")', shim)
        self.assertIn("Always route code-editing work through the lead-implement procedure", text)
        self.assertIn("## Routing Verdict", text)
        self.assertIn("NEXT: <ws:lead-discuss | lead-write-ticket | lead-implement | stop>", text)
        self.assertIn('If `NEXT:` names `lead-implement`, call `ws/playbook.print(name: "lead-implement")`', text)
        self.assertIn("- **Migration Anchor**: <loaded | n/a | missing | conflict>", text)
        self.assertIn("especially Slice and Reason, as caller-provided scope", text)
        self.assertIn("before any source inspection, planning, or editing", text)
        self.assertNotIn("**Implementation Route**", text)
        self.assertNotIn("**Implementation Verdict**", text)
        self.assertNotIn("**Verdict Basis**", text)
        self.assertNotIn("### judge: implementation-dispatch", text)

    def test_implement_keeps_execution_owner(self):
        text = (RSRC_DIR / "lead-implement" / "lead-implement.md").read_text(encoding="utf-8")
        verdict = fenced_template(text, "### Implementation Verdict")

        self.assertIn("## Implementation Verdict", text)
        self.assertIn("Do not use `NEXT:`", text)
        self.assertIn("Record `<current-branch>`.", text)
        self.assertIn("Apply `judge: branch-mode` to `<current-branch>`.", text)
        self.assertIn("Delegated implementation has minimum plan-depth `brief`", text)
        self.assertIn("extra docs must be listed in brief References", text)
        self.assertEqual(
            verdict_fields(verdict),
            [
                "- **Target**: <ticket path/stem or inline target>",
                "- **Mode**: <direct edit | delegated>",
                "- **Branch Mode**: <continue implementation branch | create implementation branch>",
                "- **Plan Depth**: <none | brief | survey | research>",
                "- **Review Allocation**: <lead-only | single reviewer | partitioned: correctness[, fit][, test]>",
                "- **Scope**: <selected phase, whole target, or caller-provided slice>",
                "- **Reason**: <decisive route facts only>",
            ],
        )
        self.assertNotIn("\nNEXT:", verdict)
        self.assertIn("### judge: needs-delegation", text)
        self.assertIn("| Direct-edit |", text)
        self.assertIn("If direct-edit: edit directly", text)
        self.assertNotIn("caller-provided `write-code` dispatch", text)
        self.assertNotIn("Confirm dispatch boundary", text)

    def test_workflow_manual_requires_english_agent_prompts(self):
        text = (RSRC_DIR / "lead-workflow-manual" / "lead-workflow-manual.md").read_text(encoding="utf-8")

        self.assertIn("Write prompts sent to native Explore-style subagents and `ws.mercenary.call` in English.", text)

    def test_verify_discussion_is_entry_shim(self):
        shim = (SKILLS_DIR / "lead-verify-discussion" / "SKILL.md").read_text(encoding="utf-8")
        text = (RSRC_DIR / "lead-verify-discussion" / "lead-verify-discussion.md").read_text(encoding="utf-8")

        self.assertIn('ws/playbook.print(name: "lead-verify-discussion")', shim)
        self.assertIn("Treat user preference as input, not evidence.", text)
        self.assertIn("Build the strongest concise countercase", text)


if __name__ == "__main__":
    unittest.main()
