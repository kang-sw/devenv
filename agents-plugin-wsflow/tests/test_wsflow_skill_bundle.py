import re
import unittest
from pathlib import Path


PLUGIN_DIR = Path(__file__).resolve().parents[1]
FULL_PLUGIN_SKILLS_DIR = PLUGIN_DIR.parent / "agents-plugin" / "skills"
# Full ws moves internal procedure bodies off the directly-invocable skill
# surface into rsrc playbooks (epic 260605 M2). A wsflow-mirrored skill may
# therefore have its full-ws counterpart as either a skill directory or an
# rsrc playbook directory; both count for drift detection.
FULL_PLUGIN_RSRC_DIR = PLUGIN_DIR.parent / "agents-plugin" / "rsrc"
SKILLS_DIR = PLUGIN_DIR / "skills"


EXPECTED_SKILLS = {
    "lead-audit-doc",
    "lead-bootstrap",
    "lead-discuss",
    "lead-check-blockers",
    "lead-proceed",
    "lead-review",
    "lead-run",
    "lead-ship",
    "lead-tune",
    "lead-workflow-manual",
    "lead-ticket",
    "lead-delegate",
    "lead-revive",
    "lead-scope-worktree",
    "mcp-server-repair",
}

# wsflow-only backward-compat aliases mapped to the shared playbook they route
# to. lead-proceed is a deprecation tombstone for the retired lead-proceed name:
# it has no full-ws counterpart (the flagship surface keeps that name
# unresolvable) and no rsrc body of its own — its body is lead-run's
# parallel-init shim pointing at the lead-run playbook. Each alias is therefore
# excused from the full-ws-counterpart, name-keyed shim-shape, and
# shared-playbook checks and asserted directly by
# test_wsflow_only_aliases_route_to_target.
WSFLOW_ALIAS_TARGET = {"lead-proceed": "lead-run"}

EXPECTED_WSFLOW_ONLY_SKILLS: set = set(WSFLOW_ALIAS_TARGET)
EXPECTED_INLINE_SKILLS = {
    "lead-revive",
    "mcp-server-repair",
}
EXPECTED_PARALLEL_INIT_SKILLS = {
    "lead-delegate",
    "lead-discuss",
    "lead-run",
}
PARALLEL_INIT_TITLES = {
    "lead-delegate": "Delegate",
    "lead-discuss": "Discuss",
    "lead-run": "Run",
}

# Single-call shims that carry the mcp-server-repair pointer tail instead of
# the generic "stop and report that blocker" un-pointed form.
POINTER_TAIL_TITLES = {
    "lead-ticket": "Ticket",
    "lead-audit-doc": "Audit Doc",
    "lead-bootstrap": "Bootstrap",
    "lead-review": "Review",
    "lead-ship": "Ship",
    "lead-tune": "Workflow Tuning",
    "lead-check-blockers": "Check Blockers",
    "lead-workflow-manual": "Workflow Manual",
    "lead-scope-worktree": "Scope Worktree",
}

FORBIDDEN_PATTERNS = {
    "retired add-rule skill": re.compile(r"\blead-add-rule\b"),
    "retired posture skill": re.compile(r"\blead-prefer-subagent\b"),
    "full ws MCP notation": re.compile(r"\bws/"),
    "full ws skill namespace": re.compile(r"\bws:"),
    "full ws dotted namespace": re.compile(r"\bws\."),
    "full ws query tool": re.compile(r"\bsubquery\b"),
    "full ws agent dotted tool": re.compile(r"\bagents\."),
    "excluded write-code skill": re.compile(r"\blead-write-code\b"),
    "excluded write-skeleton skill": re.compile(r"\blead-write-skeleton\b"),
    # lead-sprint, lead-salvage, lead-drain-ready-queue, lead-goal-fan-out-step,
    # lead-proceed, lead-implement and lead-verify-discussion were retired
    # outright rather than merely excluded from wsflow, and lead-write-ticket
    # was renamed to lead-ticket, so these guard against a reintroduced
    # reference to a skill that no longer exists in either lineage.
    "retired sprint skill": re.compile(r"\blead-sprint\b"),
    "retired salvage skill": re.compile(r"\blead-salvage\b"),
    "retired drain skill": re.compile(r"\blead-drain-ready-queue\b"),
    "retired fan-out skill": re.compile(r"\blead-goal-fan-out-step\b"),
    "retired proceed skill": re.compile(r"\blead-proceed\b"),
    "retired implement skill": re.compile(r"\blead-implement\b"),
    "retired verify-discussion skill": re.compile(r"\blead-verify-discussion\b"),
    "retired write-ticket skill": re.compile(r"\blead-write-ticket\b"),
    "excluded authoring skill": re.compile(r"\blead-skill-authoring\b"),
}

# Mirrors lead-bootstrap.md's `## On: fresh` step 1: strip only the two
# scaffold-only comment blocks before comparing emitted output across
# packages. The Inclusion-test comment is migration-v0010-permanent
# downstream content and must NOT be stripped.
_MIGRATION_SETUP_BLOCK = re.compile(r"<!-- MIGRATION:.*?-->\n*", re.DOTALL)
_MIGRATION_CHECKLIST_BLOCK = re.compile(r"<!-- MIGRATION CHECKLIST.*?-->\n*", re.DOTALL)


def _emit_fresh_body(raw_template_text: str) -> str:
    text = _MIGRATION_SETUP_BLOCK.sub("", raw_template_text)
    text = _MIGRATION_CHECKLIST_BLOCK.sub("", text)
    return text


class WsflowSkillBundleTest(unittest.TestCase):
    def test_shipped_skill_inventory_is_converged(self):
        actual = {path.name for path in SKILLS_DIR.iterdir() if path.is_dir()}
        self.assertEqual(actual, EXPECTED_SKILLS)

    def test_full_skill_inventory_drift_is_visible(self):
        full_skills = {path.name for path in FULL_PLUGIN_SKILLS_DIR.iterdir() if path.is_dir()}
        full_playbooks = (
            {path.name for path in FULL_PLUGIN_RSRC_DIR.iterdir() if path.is_dir()}
            if FULL_PLUGIN_RSRC_DIR.exists()
            else set()
        )
        # A wsflow skill's full-ws counterpart may be a skill directory or an
        # rsrc playbook directory (internal procedures migrated to playbooks).
        full_counterparts = full_skills | full_playbooks
        missing_full_counterparts = sorted(
            EXPECTED_SKILLS - EXPECTED_WSFLOW_ONLY_SKILLS - EXPECTED_INLINE_SKILLS - full_counterparts
        )
        unexpected_wsflow_skills = sorted(
            {path.name for path in SKILLS_DIR.iterdir() if path.is_dir()} - EXPECTED_SKILLS
        )

        self.assertEqual(missing_full_counterparts, [])
        # Exactly the declared wsflow-only aliases carry no full-ws counterpart;
        # a new divergence must be added to WSFLOW_ALIAS_TARGET deliberately.
        self.assertEqual(sorted(EXPECTED_WSFLOW_ONLY_SKILLS), sorted(WSFLOW_ALIAS_TARGET))
        self.assertEqual(unexpected_wsflow_skills, [])

    def test_skill_files_do_not_reference_full_ws_agent_surface(self):
        offenders = []
        for path in sorted(SKILLS_DIR.rglob("*")):
            if not path.is_file():
                continue
            # A wsflow-only alias legitimately names itself; its own directory
            # is exempt from the guard for its own retired name (and only that
            # guard) so the tombstone carve-out does not trip the full-ws sweep.
            # Every other forbidden pattern still applies to it.
            skill_dir = path.relative_to(SKILLS_DIR).parts[0]
            exempt = (
                {f"retired {skill_dir.removeprefix('lead-')} skill"}
                if skill_dir in WSFLOW_ALIAS_TARGET
                else set()
            )
            text = path.read_text(encoding="utf-8")
            for label, pattern in FORBIDDEN_PATTERNS.items():
                if label in exempt:
                    continue
                if pattern.search(text):
                    offenders.append(f"{path.relative_to(PLUGIN_DIR)}: {label}")
        self.assertEqual(offenders, [])

    def test_skill_files_are_thin_playbook_shims(self):
        # lead-ticket, lead-audit-doc, lead-bootstrap, lead-review,
        # lead-ship, lead-tune, lead-check-blockers, and
        # lead-workflow-manual all
        # carry the mcp-server-repair pointer in place of the generic "stop
        # and report that blocker" tail, so they are checked separately below
        # (see POINTER_TAIL_TITLES) with their own exact tail. That accounts
        # for every non-inline, non-parallel-init shim; none remain on the
        # un-pointed form.
        offenders = []
        for skill in sorted(
            EXPECTED_SKILLS
            - EXPECTED_INLINE_SKILLS
            - EXPECTED_PARALLEL_INIT_SKILLS
            - set(POINTER_TAIL_TITLES)
            - set(WSFLOW_ALIAS_TARGET)
        ):
            path = SKILLS_DIR / skill / "SKILL.md"
            text = path.read_text(encoding="utf-8")
            match = re.fullmatch(
                r"---\n"
                rf"name: {re.escape(skill)}\n"
                r"description: .+\n"
                r"---\n\n"
                r"# .+\n\n"
                rf"Call `wsflow/playbook\.read\(name: \"{re.escape(skill)}\"\)` and execute the returned procedure\n"
                r"inline against the current user request\. If the playbook cannot be loaded, stop\n"
                r"and report that blocker\.\n",
                text,
            )
            if match is None:
                offenders.append(str(path.relative_to(PLUGIN_DIR)))
        self.assertEqual(offenders, [])

    def test_single_call_shims_carry_repair_pointer(self):
        # All single-call shims in POINTER_TAIL_TITLES share the identical
        # joined-tail shape, differing only by skill name and title. A missing
        # pointer on any of them must fail loudly rather than silently
        # matching the generic un-pointed shim regex instead.
        offenders = []
        for skill, title in POINTER_TAIL_TITLES.items():
            path = SKILLS_DIR / skill / "SKILL.md"
            text = path.read_text(encoding="utf-8")
            match = re.fullmatch(
                r"---\n"
                rf"name: {re.escape(skill)}\n"
                r"description: .+\n"
                r"---\n\n"
                rf"# {re.escape(title)}\n\n"
                rf"Call `wsflow/playbook\.read\(name: \"{re.escape(skill)}\"\)` and execute the returned procedure\n"
                r"inline against the current user request\. "
                r"If this call fails to connect, run `/wsflow:mcp-server-repair`\.\n",
                text,
            )
            if match is None:
                offenders.append(str(path.relative_to(PLUGIN_DIR)))
        self.assertEqual(offenders, [])

    def test_parallel_init_skill_files_are_playbook_shims(self):
        # Every parallel-init skill gains the mcp-server-repair pointer
        # after the existing final line. Explicit per-skill tails (not an
        # optional regex group) so a missing pointer on any of them fails
        # loudly instead of silently passing.
        pointer_tail = {
            "lead-delegate": r"\nIf this call fails to connect, run `/wsflow:mcp-server-repair`\.",
            "lead-discuss": r"\nIf this call fails to connect, run `/wsflow:mcp-server-repair`\.",
            "lead-run": r"\nIf this call fails to connect, run `/wsflow:mcp-server-repair`\.",
        }
        offenders = []
        for skill in sorted(EXPECTED_PARALLEL_INIT_SKILLS):
            path = SKILLS_DIR / skill / "SKILL.md"
            text = path.read_text(encoding="utf-8")
            title = PARALLEL_INIT_TITLES[skill]
            match = re.fullmatch(
                r"---\n"
                rf"name: {re.escape(skill)}\n"
                r"description: .+\n"
                r"---\n\n"
                rf"# {re.escape(title)}\n\n"
                r"Call in parallel:\n"
                rf"- `wsflow/playbook\.read\(name: \"{re.escape(skill)}\", session_key: <your key, omit if fresh>\)`\n"
                r'- `wsflow/workflow_manual\(session_key: <your key or "obsidian-latch" if fresh>, root: <absolute worktree path if fresh>\)`\n\n'
                r"After both return, execute the procedure returned by `wsflow/playbook\.read`\."
                + pointer_tail[skill]
                + r"\n",
                text,
            )
            if match is None:
                offenders.append(str(path.relative_to(PLUGIN_DIR)))
        self.assertEqual(offenders, [])

    def test_skill_shims_point_to_shared_playbooks(self):
        # wsflow-only aliases point at a shared playbook under a different stem
        # than their own name (D5: no rsrc body of their own), so they are
        # covered by test_wsflow_only_aliases_route_to_target, which asserts the
        # target playbook exists.
        missing = []
        for skill in sorted(EXPECTED_SKILLS - EXPECTED_INLINE_SKILLS - set(WSFLOW_ALIAS_TARGET)):
            subdir_playbook = FULL_PLUGIN_RSRC_DIR / skill / f"{skill}.md"
            flat_playbook = FULL_PLUGIN_RSRC_DIR / f"{skill}.md"
            if not subdir_playbook.exists() and not flat_playbook.exists():
                missing.append(skill)
        self.assertEqual(missing, [])

    def test_wsflow_only_aliases_route_to_target(self):
        # A wsflow-only backward-compat alias (e.g. the retired lead-proceed
        # name) is a thin shim whose body is the target's parallel-init shim
        # pointing at the target playbook, so a by-name caller runs the current
        # target procedure. Its description does double duty as the tombstone:
        # it names both the retired alias and the canonical target so an
        # auto-selector is steered to the target while a by-name call still
        # runs. The alias has no rsrc body of its own; the target's shared
        # playbook must exist.
        offenders = []
        for alias, target in sorted(WSFLOW_ALIAS_TARGET.items()):
            path = SKILLS_DIR / alias / "SKILL.md"
            text = path.read_text(encoding="utf-8")
            description = re.search(r"^description: (.*)$", text, re.M)
            if description is None or alias not in description.group(1) or target not in description.group(1):
                offenders.append(f"{path.relative_to(PLUGIN_DIR)}: description must name both {alias} and {target}")
                continue
            subdir_playbook = FULL_PLUGIN_RSRC_DIR / target / f"{target}.md"
            flat_playbook = FULL_PLUGIN_RSRC_DIR / f"{target}.md"
            if not subdir_playbook.exists() and not flat_playbook.exists():
                offenders.append(f"{path.relative_to(PLUGIN_DIR)}: target playbook {target} missing")
                continue
            match = re.fullmatch(
                r"---\n"
                rf"name: {re.escape(alias)}\n"
                r"description: .+\n"
                r"---\n\n"
                r"# .+\n\n"
                r"Call in parallel:\n"
                rf'- `wsflow/playbook\.read\(name: "{re.escape(target)}", session_key: <your key, omit if fresh>\)`\n'
                r'- `wsflow/workflow_manual\(session_key: <your key or "obsidian-latch" if fresh>, root: <absolute worktree path if fresh>\)`\n\n'
                r"After both return, execute the procedure returned by `wsflow/playbook\.read`\."
                r"\nIf this call fails to connect, run `/wsflow:mcp-server-repair`\.\n",
                text,
            )
            if match is None:
                offenders.append(f"{path.relative_to(PLUGIN_DIR)}: body is not the {target} parallel-init shim")
        self.assertEqual(offenders, [])

    def test_bootstrap_scaffolds_emit_converged_output_across_packages(self):
        # Ticket 260825 Phase 4: assert positive convergence. Both packages'
        # AGENTS.template.md emit byte-identical fresh-mode bodies (stripping
        # only the scaffold-only MIGRATION blocks, mirroring lead-bootstrap.md's
        # `## On: fresh` step 1) and share one migration-ordinal tag; WORKFLOW.md
        # has no strip blocks and is compared raw.
        ws_agents_raw = (FULL_PLUGIN_SKILLS_DIR / "lead-bootstrap" / "AGENTS.template.md").read_text(encoding="utf-8")
        wsflow_agents_raw = (SKILLS_DIR / "lead-bootstrap" / "AGENTS.template.md").read_text(encoding="utf-8")
        ws_emitted = _emit_fresh_body(ws_agents_raw)
        wsflow_emitted = _emit_fresh_body(wsflow_agents_raw)
        self.assertEqual(ws_emitted, wsflow_emitted)

        tag_pattern = re.compile(r"<!-- Template Version: (v\d+) -->")
        ws_tag = tag_pattern.search(ws_emitted)
        wsflow_tag = tag_pattern.search(wsflow_emitted)
        self.assertIsNotNone(ws_tag)
        self.assertIsNotNone(wsflow_tag)
        self.assertEqual(ws_tag.group(1), wsflow_tag.group(1))

        ws_workflow = (FULL_PLUGIN_SKILLS_DIR / "lead-bootstrap" / "WORKFLOW.md").read_text(encoding="utf-8")
        wsflow_workflow = (SKILLS_DIR / "lead-bootstrap" / "WORKFLOW.md").read_text(encoding="utf-8")
        self.assertEqual(ws_workflow, wsflow_workflow)


if __name__ == "__main__":
    unittest.main()
