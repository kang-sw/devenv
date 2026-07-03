import unittest
from pathlib import Path


SKILLS_DIR = Path(__file__).resolve().parents[1] / "skills"
RSRC_DIR = Path(__file__).resolve().parents[1] / "rsrc"


class SkillDispatchContractsTest(unittest.TestCase):
    def test_proceed_keeps_implementation_route_only(self):
        shim = (SKILLS_DIR / "lead-proceed" / "SKILL.md").read_text(encoding="utf-8")
        text = (RSRC_DIR / "lead-proceed" / "lead-proceed.md").read_text(encoding="utf-8")

        self.assertIn('ws/playbook.print(name: "lead-proceed")', shim)
        self.assertIn("Route only; do not implement or plan here.", text)
        self.assertIn("Always route code-editing work through `lead-implement`", text)
        self.assertIn("{{.McpNamespace}}/enter.proceed(session_key:", text)
        self.assertIn("Follow `Next:` from `enter.proceed` exactly", text)
        self.assertIn("scope_blocked=no-unfinished-phase", text)
        self.assertIn("scope_blocked=container-ticket", text)
        self.assertIn("scope_blocked=multiple-explicit-phases", text)
        self.assertNotIn("## Routing Verdict", text)
        self.assertNotIn("**Implementation Route**", text)
        self.assertNotIn("**Implementation Verdict**", text)
        self.assertNotIn("**Verdict Basis**", text)
        self.assertNotIn("### judge: implementation-dispatch", text)

    def test_implement_keeps_execution_owner(self):
        text = (RSRC_DIR / "lead-implement" / "lead-implement.md").read_text(encoding="utf-8")

        self.assertIn("{{.McpNamespace}}/enter.implement", text)
        self.assertIn("Gather `target`, `facts`, and explicit caller `policy`", text)
        self.assertIn("Follow the returned `raw` verdict and `next_instruction`; do not re-derive deterministic labels.", text)
        self.assertIn(
            "No Edit or Write tool call is permitted until `enter.implement` returns a `direct-edit` verdict.",
            text,
        )
        self.assertIn("Wait for user approval before merge or another implementation slice.", text)
        self.assertIn("### Implementer spawn prompt", text)
        self.assertIn("### Reviewer table", text)
        self.assertNotIn("## Implementation Verdict", text)
        self.assertNotIn("### judge: needs-delegation", text)
        self.assertNotIn("### judge: branch-mode", text)
        self.assertNotIn("\nNEXT:", text)

    def test_workflow_manual_requires_english_agent_prompts(self):
        text = (RSRC_DIR / "lead-workflow-manual" / "lead-workflow-manual.md").read_text(encoding="utf-8")

        self.assertIn("Write prompts sent to native Explore-style subagents in English.", text)
        self.assertIn("<!-- ws:full-only:start -->", text)
        self.assertIn("Write prompts sent to `mercenary.call` in English.", text)
        self.assertIn("<!-- ws:full-only:end -->", text)

    def test_verify_discussion_is_inlined_static_body(self):
        # lead-verify-discussion's body is inlined directly in SKILL.md (no
        # rsrc playbook, no playbook.print indirection) since the
        # substitution-mirrored inline-mirror work landed.
        text = (SKILLS_DIR / "lead-verify-discussion" / "SKILL.md").read_text(encoding="utf-8")

        self.assertNotIn('ws/playbook.print(name: "lead-verify-discussion")', text)
        self.assertIn("Treat user preference as input, not evidence.", text)
        self.assertIn("Build the strongest concise countercase", text)

    def test_drain_ready_queue_is_inlined_static_body(self):
        # lead-drain-ready-queue's body is inlined directly in SKILL.md (no
        # rsrc playbook, no playbook.print indirection), mirroring the
        # lead-verify-discussion inline-body shape.
        text = (SKILLS_DIR / "lead-drain-ready-queue" / "SKILL.md").read_text(encoding="utf-8")

        self.assertNotIn('ws/playbook.print(name: "lead-drain-ready-queue")', text)
        self.assertIn("otherwise take the oldest (FIFO)", text)
        self.assertIn("prerequisite", text)
        self.assertIn("lead-prefer-subagent", text)
        self.assertIn("run commands yourself", text)


if __name__ == "__main__":
    unittest.main()
