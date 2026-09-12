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

# Names that must not reappear on this flagship package's shipped skill or
# playbook surface (SKILLS_DIR/RSRC_DIR below are agents-plugin/): each was
# retired into a surviving skill, or renamed. The flagship keeps lead-proceed
# unresolvable; the conservative wsflow derivative deliberately re-adds it as a
# deprecation alias routing to lead-run, asserted in that package's
# test_wsflow_only_aliases_route_to_target. Do not extend this sweep to the
# wsflow surface — that would break the intended one-package carve-out.
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
        self.assertIn("Do not read the file", run)
        self.assertNotIn("lead-delegate", run.split("## Handle the report")[0])
        self.assertIn("{{.SkillNamespace}}:lead-delegate", run.split("## Handle the report")[1])
        # lead-run is ticket-only: the ad-hoc implementation-contract route is
        # gone from the shipped body (retired 260911).
        self.assertNotIn("Contract:", run)
        self.assertNotIn("ad-hoc", run)
        shim = (SKILLS_DIR / "lead-delegate" / "SKILL.md").read_text()
        self.assertIn('ws/playbook.read(name: "lead-delegate", session_key:', shim)
        self.assertIn("ws/workflow_manual", shim)

    def test_review_local_fix_uses_delegate_gate_and_returns_to_review(self):
        for package in (RSRC_DIR, SKILLS_DIR.parent.parent / "agents-plugin-wsflow" / "rsrc"):
            review = (package / "lead-review" / "lead-review.md").read_text()
            handoff = review.split("- **NEEDS FIX**:")[1].split("- **OPEN**:")[0]
            self.assertIn("{{.SkillNamespace}}:lead-delegate` with the findings path", handoff)
            self.assertIn("If its routing gate requires a ticket", handoff)
            self.assertIn("{{.SkillNamespace}}:lead-ticket` with those inputs, then", handoff)
            self.assertIn("{{.SkillNamespace}}:lead-run` with the ready ticket", handoff)
            self.assertIn("After either local\n  repair route completes", handoff)
            self.assertIn("{{.SkillNamespace}}:lead-review` again", handoff)
            self.assertIn("retain the original base and include the\n  repair commits", handoff)
            self.assertIn("Contributor → the config's Comment Method, else hand over the path", handoff)
            self.assertNotIn("with that path as the contract", handoff)
            delegate = (package / "lead-delegate" / "lead-delegate.md").read_text()
            gate = delegate.split("## Routing")[1].split("## Assignment")[0]
            self.assertIn("public behavior, an API, protocol, schema, template", gate)
            self.assertIn("canonical flow, or architecture", gate)
            self.assertIn("unresolved product or workflow decision", gate)
            self.assertIn("independent review is needed, unless the task is a local NEEDS FIX repair", gate)
            self.assertIn("whose follow-up review supplies\n  that verification", gate)

    def test_delegate_implementer_is_a_tier_unaware_reviewer_free_floor(self):
        # The delegate-side implementer floor is a render-only rsrc playbook with
        # no lead-skill entry; lead-delegate renders it when the assignment writes
        # code (260911). It must stay tier-unaware (delegate resolves the tier via
        # config.resolve_agent) and reviewer-free (that is the delegate/worker line).
        for package in (RSRC_DIR, SKILLS_DIR.parent.parent / "agents-plugin-wsflow" / "rsrc"):
            body = (package / "delegate-implementer" / "delegate-implementer.md").read_text(encoding="utf-8")
            head = body.split("---", 2)[1]
            self.assertNotIn("tier:", head)
            self.assertNotRegex(body, r"(?i)\breview(er)?\b")
        self.assertFalse((SKILLS_DIR / "delegate-implementer").exists())
        delegate = (RSRC_DIR / "lead-delegate" / "lead-delegate.md").read_text(encoding="utf-8")
        self.assertIn('playbook.render(name:\n"delegate-implementer"', delegate)

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

        self.assertIn("Give it an English\nprompt", text)

    def test_workers_report_and_lead_owns_impl_merge(self):
        for name in ("ticket-worker", "ticket-worker-elevated", "ticket-worker-escalated"):
            text = (RSRC_DIR / name / (name + ".md")).read_text(encoding="utf-8")
            self.assertIn("The lead owns merging after your report; do not merge.", text)
            self.assertNotIn("Merge per the route verdict", text)
        protocol = (RSRC_DIR / "worker-stop-protocol.md").read_text(encoding="utf-8")
        self.assertIn("all merges, including impl into goal, belong to the lead", protocol)
        self.assertIn("merge_confirm: skip | ask", protocol)
        self.assertIn("completion: phase | ticket | ad_hoc | none", protocol)
        self.assertIn("Valid terminal pairs are `[ok]` with `stop: none`", protocol)
        text = (RSRC_DIR / "lead-run" / "lead-run.md").read_text(encoding="utf-8")
        self.assertIn("`merge_confirm: skip` auto-calls", text)
        self.assertIn("`ask` (including absent)", text)
        self.assertIn("{{.McpNamespace}}/git.merge", text)
        self.assertIn("call `{{.McpNamespace}}/git.merge` with the goal branch", text)
        self.assertNotIn("goal-to-PARENT terminal uses raw Git", text)
        self.assertIn("`completion: phase`, leave the ticket active", text)
        self.assertIn("With `completion: ticket`", text)
        self.assertIn("incompatible values are a protocol mismatch", text)
        self.assertIn("do not query the ticket, infer a\npath, merge", text)

    def test_run_dispatches_through_playbook_read(self):
        # lead-run is a playbook.read shim over an rsrc body, not an inline
        # SKILL.md: the body uses harness-idiom template variables, and those
        # are substituted only on the playbook.read/playbook.render path.
        shim = (SKILLS_DIR / "lead-run" / "SKILL.md").read_text(encoding="utf-8")
        text = (RSRC_DIR / "lead-run" / "lead-run.md").read_text(encoding="utf-8")

        self.assertIn('ws/playbook.read(name: "lead-run", session_key:', shim)
        self.assertIn("ticket-selector", text)
        self.assertNotIn("{{.ExploreAgent}}", text)
        self.assertIn("{{.SpawnIdiom}}", text)
        self.assertIn('{{.McpNamespace}}/playbook.render(name: <chosen worker playbook>', text)
        self.assertIn("{{.McpNamespace}}/session.note(session_key:", text)
        self.assertIn("One worker in flight per invocation.", text)
        selector = (RSRC_DIR / "ticket-selector" / "ticket-selector.md").read_text(encoding="utf-8")
        self.assertIn("prerequisite", selector)
        self.assertIn("impl_ticket", selector)
        self.assertIn('statuses: ["todo", "idea"]', selector)
        self.assertIn("backlog_omitted", selector)
        self.assertIn("Do not list\n`ready/` or read", text)
        self.assertIn(
            "A goal run is the current branch `goal/*` or an active goal reminder.",
            text,
        )
        self.assertIn(
            "a goal branch only when an active goal reminder is present",
            text,
        )
        self.assertNotIn("/goal", text)
        self.assertIn(
            "next cycle: {{.SkillNamespace}}:lead-run.",
            text,
        )
        self.assertIn(
            "Ready queue is empty — prepare a todo or idea ticket with",
            text,
        )
        self.assertNotIn("[design-review:", text)


if __name__ == "__main__":
    unittest.main()
