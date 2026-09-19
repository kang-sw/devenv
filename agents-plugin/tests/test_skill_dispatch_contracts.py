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
    "lead-audit-doc",
    "lead-use-mailbox",
    # undecided disposition; survive unchanged
    "lead-scope-worktree",
}

# Names that must not reappear on this flagship package's shipped skill or
# playbook surface (SKILLS_DIR/RSRC_DIR below are agents-plugin/): each was
# retired into a surviving skill, or renamed. The flagship keeps lead-proceed
# unresolvable; the conservative wsflow derivative deliberately re-adds it as a
# deprecation alias routing to lead-run, asserted in that package's
# test_wsflow_only_aliases_route_to_target. Do not extend this sweep to the
# wsflow surface — that would break the intended one-package carve-out.
RETIRED_SKILL_NAMES = (
    "lead-add-rule",
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
    def test_document_audit_preserves_approved_prose_and_dispatch_boundary(self):
        contract = json.loads((Path(__file__).parent / "fixtures" / "lead_audit_doc_contract.json").read_text())
        for package in (SKILLS_DIR.parent, SKILLS_DIR.parent.parent / "agents-plugin-wsflow"):
            shim = (package / "skills" / "lead-audit-doc" / "SKILL.md").read_text()
            self.assertEqual(re.search(r"^description: (.*)$", shim, re.M).group(1), contract["description"])
            self.assertNotIn("allow_implicit_invocation: false", shim)
            for name, key in (("lead-audit-doc", "lead"), ("fresh-read-doc-auditor", "auditor")):
                body = (package / "rsrc" / name / f"{name}.md").read_text().split("---", 2)[2].strip()
                self.assertEqual(body, contract[key])
            for root in (package / "skills", package / "rsrc"):
                self.assertFalse((root / "lead-add-rule").exists())
                for path in root.rglob("*.md"):
                    self.assertNotIn("lead-add-rule", path.read_text(), str(path))
            self.assertFalse((package / "skills" / "fresh-read-doc-auditor").exists())

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

    def test_selector_and_run_consume_body_blocked_marker(self):
        # 260913: the body-level `## Blocked` marker is now advisory inventory,
        # so the selector reads the section and judges currency (rather than
        # skipping on any marker), and lead-run rechecks the selected ticket
        # before dispatch. Assert on both mirror packages that ship the rsrc
        # bodies; the pi mirror's byte-identity is guarded by TestPiMirrorUpToDate.
        for rsrc in (RSRC_DIR, RSRC_DIR.parent.parent / "agents-plugin-wsflow" / "rsrc"):
            selector = " ".join(
                (rsrc / "ticket-selector" / "ticket-selector.md").read_text(encoding="utf-8").split()
            )
            self.assertIn("blocked_marker", selector)
            self.assertIn("read that section and judge whether the blocker is current", selector)
            self.assertIn("skipping only a current one", selector)

            run = " ".join((rsrc / "lead-run" / "lead-run.md").read_text(encoding="utf-8").split())
            self.assertIn("blocked_headings", run)
            self.assertIn("read the referenced `## Blocked` section and judge whether the blocker is current", run)
            self.assertIn("return to Select for the next candidate", run)
            self.assertIn("never dispatch a worker into it", run)
            self.assertIn("named directly in the invocation is not re-selected by queue ordering", run)

    def test_lead_ticket_mentions_git_commit_expected_branch(self):
        # 260916: git.commit now requires expected_branch (the branch guard
        # against a parallel session switching the shared worktree). Unlike the
        # parallel blocked_marker change (test_selector_and_run_consume_body_
        # blocked_marker above), this argument was previously mentioned only in
        # prose with no pinning test. Assert it on all three mirror packages
        # directly, rather than leaning on the pi mirror's separate byte-identity
        # guard (TestPiMirrorUpToDate), so a drift here fails at the source.
        for rsrc in (
            RSRC_DIR,
            RSRC_DIR.parent.parent / "agents-plugin-wsflow" / "rsrc",
            RSRC_DIR.parent.parent / "agents-plugin-pi" / "rsrc",
        ):
            text = " ".join((rsrc / "lead-ticket" / "lead-ticket.md").read_text(encoding="utf-8").split())
            self.assertIn("git.commit(paths, title, ai_context, expected_branch)", text)
            self.assertIn("expected_branch` is the branch", text)

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
        for name in ("ticket-worker", "ticket-worker-elevated"):
            text = (RSRC_DIR / name / (name + ".md")).read_text(encoding="utf-8")
            self.assertIn("The lead owns merging after your report; do not merge.", text)
            self.assertNotIn("Merge per the route verdict", text)
        protocol = (RSRC_DIR / "worker-stop-protocol.md").read_text(encoding="utf-8")
        self.assertIn("all merges, including impl into goal, belong to the lead", protocol)
        self.assertIn("merge_confirm: skip | ask", protocol)
        self.assertIn("completion: phase | ticket | ad_hoc | none", protocol)
        self.assertIn("Valid terminal pairs are `[ok]` with `stop: none`", protocol)
        # Whitespace-normalized so a prose reflow cannot break a pinned phrase
        # across a newline (the rendered-policy Go test normalizes the same way).
        text = " ".join((RSRC_DIR / "lead-run" / "lead-run.md").read_text(encoding="utf-8").split())
        self.assertIn("`merge_confirm: skip` auto-calls", text)
        self.assertIn("`ask` (including absent)", text)
        self.assertIn("{{.McpNamespace}}/git.merge", text)
        self.assertIn("call `{{.McpNamespace}}/git.merge` with the goal branch", text)
        self.assertNotIn("goal-to-PARENT terminal uses raw Git", text)
        self.assertIn("`completion: phase` — do not merge", text)
        self.assertIn("`completion: ticket` — merge the retained impl branch", text)
        self.assertIn("incompatible values are a protocol mismatch", text)
        self.assertIn("call `{{.McpNamespace}}/git.status` and decide", text)
        self.assertIn("Branch-explicit calls need no check.", text)
        self.assertIn(
            "Your terminal report does not restore the checkout: the shared worktree's",
            protocol,
        )

    def test_two_worker_bodies_diverge_in_behavior_not_just_tier(self):
        # The three byte-identical ticket-worker* bodies collapsed to two that
        # differ in behavior: default `ticket-worker` implements directly and is
        # scoped to one phase; `elevated` orchestrates as a mini-lead owning the
        # whole ticket. `ticket-worker-escalated` is retired (xlarge reuses the
        # elevated body via a render-time tier override, not a third body). A
        # future edit that re-collapses them to a tier-only difference, or drops
        # the orchestration prose, fails here. Assert on both mirror packages
        # shipping the rsrc bodies.
        for rsrc in (RSRC_DIR, RSRC_DIR.parent.parent / "agents-plugin-wsflow" / "rsrc"):
            self.assertFalse((rsrc / "ticket-worker-escalated").exists(), str(rsrc))
            default = " ".join(
                (rsrc / "ticket-worker" / "ticket-worker.md").read_text(encoding="utf-8").split()
            )
            elevated = " ".join(
                (rsrc / "ticket-worker-elevated" / "ticket-worker-elevated.md")
                .read_text(encoding="utf-8")
                .split()
            )
            # Default body: unchanged direct implementer, scoped to one phase,
            # with no orchestration prose.
            self.assertIn(
                "The earliest phase without a `### Result` is the phase you execute",
                default,
            )
            self.assertNotIn("orchestrating mini-lead", default)
            self.assertNotIn("delegate-implementer", default)
            self.assertNotIn("warm worktree", default)
            # Elevated body: orchestrating mini-lead owning the whole ticket,
            # sequential leaves on one warm worktree, difficulty-calibrated
            # delegation to the implementer floor.
            self.assertIn("You are an orchestrating mini-lead", elevated)
            self.assertIn("you own the whole ticket in one invocation", elevated)
            self.assertIn("You own every phase without a `### Result`", elevated)
            self.assertIn(
                "Run leaves one at a time on your single warm worktree, never in parallel",
                elevated,
            )
            self.assertIn('playbook.render(name: "delegate-implementer"', elevated)
            self.assertIn("Calibrate delegation by difficulty, not a quota", elevated)
            # Both keep role: worker (lead scope) — the difference is prose, not
            # permission — and the lead-owns-merge obligation.
            for body in (default, elevated):
                self.assertIn("role: worker", body)
                self.assertIn("The lead owns merging after your report; do not merge.", body)

    def test_run_defers_phase_merge_and_continues_active_assignment(self):
        # 260915: the phase-completion default is "continue on the persistent
        # impl branch," not "merge." Per-phase merge deleted the deterministic
        # impl branch and forced the next phase to re-create the same name.
        run = (RSRC_DIR / "lead-run" / "lead-run.md").read_text(encoding="utf-8")
        # Assert against whitespace-normalized sections so line wrapping in the
        # prose does not break a phrase across a newline (the rendered-policy Go
        # test normalizes the same way).
        norm = lambda s: " ".join(s.split())
        report = norm(run.split("## Handle the report")[1].split("## Parallel route")[0])
        # (a) phase completion does not merge; the impl branch is retained AND
        # stays checked out, which is the condition under which the worker's
        # route returns `continue` for the next phase.
        self.assertIn("`completion: phase` — do not merge", report)
        self.assertIn("a per-phase merge deletes the impl branch the next phase stacks on", report)
        self.assertIn("Leave the checkout on that impl branch", report)
        self.assertIn("the worker's route returns `continue` only when HEAD is that branch", report)
        # (b) ticket completion still merges through the user-approval gate; a
        # mid-ticket landing uses the same gate, and no auto-merge is added.
        self.assertIn("`completion: ticket` — merge the retained impl branch", report)
        self.assertIn(
            "Merge mid-ticket only when a dependent ticket needs the landing, through the same gate",
            report,
        )
        self.assertIn("`merge_confirm: skip` auto-calls", report)
        self.assertIn("`ask` (including absent)", report)
        # (c) Select is one selection and nothing else: the invocation's ticket,
        # or the selector's single result. Continue detection for an active
        # multi-phase ticket is the selector's own git.status `impl_ticket`
        # read, so the lead keeps no assignment record of its own.
        select = norm(run.split("## Select")[1].split("## Spawn")[0])
        self.assertIn("A ticket named in the invocation wins.", select)
        self.assertIn("Otherwise render `ticket-selector`", select)
        self.assertIn("use its one `selection:` result", select)
        self.assertNotIn("session.note", run)
        self.assertNotIn("session.children", run)
        # (d) the continue verdict variant is emitted, distinct from the generic
        # ready-queue line, and the finished/complete/done ban stays intact.
        end = norm(run.split("## End the turn")[1])
        self.assertIn("has phases remaining on its retained impl branch", end)
        self.assertIn("next cycle: {{.SkillNamespace}}:lead-run continues it.", end)
        self.assertIn("next cycle: {{.SkillNamespace}}:lead-run.", end)
        self.assertIn("keep `finished`, `complete`, and `done` out", end)

    def test_run_pins_branch_awareness_reasoning(self):
        # The merge/stop test pins the mechanical ws/git.status call; this pins
        # the branch-awareness REASONING the lead must apply to it, so a rewrite
        # that keeps the call but drops why it exists, the stack-vs-return
        # decision, or the dirty-tree judgment fails here. Whitespace-normalized
        # so a reflow of the compressed paragraph does not break the pins.
        run = " ".join((RSRC_DIR / "lead-run" / "lead-run.md").read_text(encoding="utf-8").split())
        # why the check exists at all.
        self.assertIn("The worker's checkout is shared and outlives its turn", run)
        # stack-vs-return: the explicit either/or the lead must decide.
        self.assertIn(
            "stack on the impl branch when the write belongs to a ticket that continues;"
            " check out the branch you were invoked on when the write is unrelated or you"
            " are leaving the ticket blocked",
            run,
        )
        # dirty-tree: the worker's shared checkout may be dirty; commit or stash
        # rather than forcing a checkout through it.
        self.assertIn("committing or stashing a dirty tree first", run)

    def test_run_dispatches_through_playbook_read(self):
        # lead-run is a playbook.read shim over an rsrc body, not an inline
        # SKILL.md: the body uses harness-idiom template variables, and those
        # are substituted only on the playbook.read/playbook.render path.
        shim = (SKILLS_DIR / "lead-run" / "SKILL.md").read_text(encoding="utf-8")
        text = (RSRC_DIR / "lead-run" / "lead-run.md").read_text(encoding="utf-8")

        # Phrase pins run against the whitespace-normalized body so a reflow
        # does not break one across a newline; the template-variable and
        # forbidden-string pins run against the raw text.
        flat = " ".join(text.split())

        self.assertIn('ws/playbook.read(name: "lead-run", session_key:', shim)
        self.assertIn("ticket-selector", text)
        # Batch selection for the parallel route is its own delegate playbook,
        # not inline lead prose.
        self.assertIn("ticket-batch-selector", text)
        self.assertNotIn("{{.ExploreAgent}}", text)
        self.assertIn("{{.SpawnIdiom}}", text)
        self.assertIn('{{.McpNamespace}}/playbook.render(name: <chosen worker playbook>', flat)
        self.assertIn(
            "One worker in flight per invocation, unless the opt-in parallel route below"
            " is approved for this run.",
            flat,
        )
        selector = (RSRC_DIR / "ticket-selector" / "ticket-selector.md").read_text(encoding="utf-8")
        self.assertIn("prerequisite", selector)
        self.assertIn("impl_ticket", selector)
        self.assertIn('statuses: ["todo", "idea"]', selector)
        self.assertIn("backlog_omitted", selector)
        batch = (
            RSRC_DIR / "ticket-batch-selector" / "ticket-batch-selector.md"
        ).read_text(encoding="utf-8")
        self.assertIn("impl_ticket", batch)
        self.assertIn('statuses: ["ready"]', batch)
        # The output block is the contract the lead reads back: one line per
        # selected ticket, a reason on every exclusion, and the three terminal
        # lines that stand in place of `batch:`.
        self.assertIn("batch: <ticket path>", batch)
        self.assertIn("excluded: <ticket path> — <reason>", batch)
        self.assertIn("omitted: none", batch)
        self.assertIn(
            "Terminal lines in place of `batch:`: `ready/ empty`, `every remaining ticket"
            " blocked`, or `stop: <reason>`.",
            " ".join(batch.split()),
        )
        self.assertIn(
            "A goal run is the current branch `goal/*` or an active goal reminder.",
            flat,
        )
        self.assertIn(
            "When a goal reminder is active and the branch is not yet `goal/*`",
            flat,
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
