import re
import json
import unittest
from pathlib import Path


SKILLS_DIR = Path(__file__).resolve().parents[1] / "skills"
RSRC_DIR = Path(__file__).resolve().parents[1] / "rsrc"

# The lead surface is a working set plus housekeeping; everything else is a
# worker playbook or retired. A skill added or removed without updating this
# set is inventory drift, not a passing change.
EXPECTED_LEAD_SKILLS = {
    # working
    "lead-discuss",
    "lead-delegate",
    "lead-ticket",
    "lead-run",
    "lead-review",
    "lead-ship",
    # housekeeping
    "lead-bootstrap",
    "lead-tune",
    "lead-revive",
    "mcp-server-repair",
    # undecided disposition; survive unchanged
    "lead-scope-worktree",
    "lead-add-rule",
}

# Names that must not reappear anywhere on the shipped skill or playbook
# surface: each was retired into a surviving skill, or renamed.
RETIRED_SKILL_NAMES = (
    "lead-prefer-subagent",
    "lead-proceed",
    "lead-implement",
    "lead-verify-discussion",
    "lead-write-ticket",
    # retired with the spec and mental-model document layers
    "lead-backfill-docs",
    "lead-write-spec",
    "lead-update-spec",
    "lead-forge-spec",
    "lead-forge-mental-model",
    "mental-model-updater",
    "doc-gap-discovery",
)


class SkillDispatchContractsTest(unittest.TestCase):
    def test_delegate_and_sibling_exact_prose(self):
        contract = json.loads((Path(__file__).parent / "fixtures" / "lead_delegate_contract.json").read_text())
        for package in (SKILLS_DIR.parent, SKILLS_DIR.parent.parent / "agents-plugin-wsflow"):
            for name, expected in contract["descriptions"].items():
                shim = (package / "skills" / name / "SKILL.md").read_text()
                self.assertEqual(re.search(r"^description: (.*)$", shim, re.M).group(1), expected)
        delegate = (RSRC_DIR / "lead-delegate" / "lead-delegate.md").read_text().split("---", 2)[2].strip()
        self.assertEqual(delegate, contract["delegate"])
        discuss = (RSRC_DIR / "lead-discuss" / "lead-discuss.md").read_text()
        run = (RSRC_DIR / "lead-run" / "lead-run.md").read_text()
        self.assertIn(contract["discuss_opening"], discuss)
        self.assertIn(contract["run_opening"], run)
        self.assertIn(contract["run_ad_hoc"], run)
        self.assertIn("Do not read the file", run)
        self.assertNotIn("lead-delegate", run)
        shim = (SKILLS_DIR / "lead-delegate" / "SKILL.md").read_text()
        self.assertIn('ws/playbook.read(name: "lead-delegate", session_key:', shim)
        self.assertIn("ws/workflow_manual", shim)

    def test_lead_skill_surface_is_collapsed(self):
        actual = {path.name for path in SKILLS_DIR.iterdir() if path.is_dir()}
        self.assertEqual(actual, EXPECTED_LEAD_SKILLS)

    def test_retired_skill_names_are_gone_from_shipped_surfaces(self):
        offenders = []
        for root in (SKILLS_DIR, RSRC_DIR):
            for path in sorted(root.rglob("*")):
                if not path.is_file() or path.suffix not in {".md", ".json"}:
                    continue
                text = path.read_text(encoding="utf-8")
                for name in RETIRED_SKILL_NAMES:
                    if re.search(rf"\b{re.escape(name)}\b", text):
                        offenders.append(f"{path.relative_to(SKILLS_DIR.parent)}: {name}")
        self.assertEqual(offenders, [])

    def test_ticket_skill_dispatches_to_renamed_playbook(self):
        shim = (SKILLS_DIR / "lead-ticket" / "SKILL.md").read_text(encoding="utf-8")
        text = (RSRC_DIR / "lead-ticket" / "lead-ticket.md").read_text(encoding="utf-8")

        self.assertIn('ws/playbook.read(name: "lead-ticket", session_key:', shim)
        self.assertIn("## Open Decision Queue", text)
        self.assertIn('{{.McpNamespace}}/playbook.render(name: "ticket-fact-populator"', text)
        self.assertIn("{{.McpNamespace}}/tickets.sage_gate(stem, landing: \"ready\")", text)
        self.assertIn("{{.SkillNamespace}}:lead-run", text)

    def test_placed_lead_bodies_carry_no_unresolved_review_marker(self):
        for skill in ("lead-discuss", "lead-ticket", "lead-review", "lead-ship"):
            text = (RSRC_DIR / skill / f"{skill}.md").read_text(encoding="utf-8")
            self.assertNotIn("[design-review:", text, msg=skill)

    def test_review_and_ship_retain_their_config_schemas(self):
        review = (RSRC_DIR / "lead-review" / "lead-review.md").read_text(encoding="utf-8")
        ship = (RSRC_DIR / "lead-ship" / "lead-ship.md").read_text(encoding="utf-8")

        self.assertIn("### Review Config Template", review)
        self.assertIn("## Landing Lens", review)
        self.assertIn("## Deep Review", review)
        self.assertIn("### Ship Config Format", ship)
        self.assertIn("## Version Strategy", ship)

    def test_workflow_manual_requires_english_agent_prompts(self):
        text = (RSRC_DIR / "lead-workflow-manual" / "lead-workflow-manual.md").read_text(encoding="utf-8")

        self.assertIn("Write prompts sent to delegated subagents in English.", text)

    def test_run_dispatches_through_playbook_read(self):
        # lead-run is a playbook.read shim over an rsrc body, not an inline
        # SKILL.md: the body uses harness-idiom template variables, and those
        # are substituted only on the playbook.read/playbook.render path.
        shim = (SKILLS_DIR / "lead-run" / "SKILL.md").read_text(encoding="utf-8")
        text = (RSRC_DIR / "lead-run" / "lead-run.md").read_text(encoding="utf-8")

        self.assertIn('ws/playbook.read(name: "lead-run", session_key:', shim)
        self.assertIn("{{.ExploreAgent}}", text)
        self.assertIn("{{.SpawnIdiom}}", text)
        self.assertIn('{{.McpNamespace}}/playbook.render(name: <chosen worker playbook>', text)
        self.assertIn("{{.McpNamespace}}/session.note(session_key:", text)
        self.assertIn("One worker in flight per invocation.", text)
        self.assertIn("prerequisite", text)
        self.assertIn("do not list `ready/` or read", text)
        self.assertIn(
            "next cycle: {{.SkillNamespace}}:lead-run.",
            text,
        )
        self.assertNotIn("[design-review:", text)


if __name__ == "__main__":
    unittest.main()
