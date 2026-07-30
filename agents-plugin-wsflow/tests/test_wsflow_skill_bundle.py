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
    "lead-add-rule",
    "lead-backfill-docs",
    "lead-bootstrap",
    "lead-discuss",
    "lead-drain-ready-queue",
    "lead-goal-fan-out-step",
    "lead-forge-mental-model",
    "lead-forge-spec",
    "lead-implement",
    "lead-check-blockers",
    "lead-proceed",
    "lead-review",
    "lead-ship",
    "lead-tune",
    "lead-update-spec",
    "lead-verify-discussion",
    "lead-workflow-manual",
    "lead-write-spec",
    "lead-write-ticket",
    "lead-prefer-subagent",
    "lead-revive",
    "mcp-server-repair",
}

EXPECTED_WSFLOW_ONLY_SKILLS: set = set()
EXPECTED_INLINE_SKILLS = {
    "lead-revive",
    "lead-prefer-subagent",
    "lead-verify-discussion",
    "lead-drain-ready-queue",
    "mcp-server-repair",
}
EXPECTED_PARALLEL_INIT_SKILLS = {"lead-backfill-docs", "lead-discuss", "lead-goal-fan-out-step"}
PARALLEL_INIT_TITLES = {
    "lead-backfill-docs": "Backfill Docs",
    "lead-discuss": "Discuss",
    "lead-goal-fan-out-step": "Goal Fan-Out Step",
}

# Single-call shims that carry the mcp-server-repair pointer tail instead of
# the generic "stop and report that blocker" un-pointed form.
POINTER_TAIL_TITLES = {
    "lead-proceed": "Proceed",
    "lead-write-ticket": "Write Ticket",
    "lead-write-spec": "Write Spec",
    "lead-add-rule": "Add Rule",
    "lead-bootstrap": "Bootstrap",
    "lead-forge-mental-model": "Forge Mental Model",
    "lead-forge-spec": "Forge Spec",
    "lead-review": "Review",
    "lead-ship": "Ship",
    "lead-tune": "Workflow Tuning",
    "lead-implement": "Implement",
    "lead-check-blockers": "Check Blockers",
    "lead-update-spec": "Update Spec",
    "lead-workflow-manual": "Workflow Manual",
}

FORBIDDEN_PATTERNS = {
    "full ws MCP notation": re.compile(r"\bws/"),
    "full ws skill namespace": re.compile(r"\bws:"),
    "full ws dotted namespace": re.compile(r"\bws\."),
    "full ws query tool": re.compile(r"\bsubquery\b"),
    "full ws agent dotted tool": re.compile(r"\bagents\."),
    "excluded write-code skill": re.compile(r"\blead-write-code\b"),
    "excluded write-skeleton skill": re.compile(r"\blead-write-skeleton\b"),
    # lead-sprint and lead-salvage were retired outright rather than merely
    # excluded from wsflow, so these guard against a reintroduced reference to
    # a skill that no longer exists in either lineage.
    "retired sprint skill": re.compile(r"\blead-sprint\b"),
    "retired salvage skill": re.compile(r"\blead-salvage\b"),
    "excluded authoring skill": re.compile(r"\blead-skill-authoring\b"),
}


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
        self.assertEqual(sorted(EXPECTED_WSFLOW_ONLY_SKILLS), [])
        self.assertEqual(unexpected_wsflow_skills, [])

    def test_skill_files_do_not_reference_full_ws_agent_surface(self):
        offenders = []
        for path in sorted(SKILLS_DIR.rglob("*")):
            if not path.is_file():
                continue
            text = path.read_text(encoding="utf-8")
            for label, pattern in FORBIDDEN_PATTERNS.items():
                if pattern.search(text):
                    offenders.append(f"{path.relative_to(PLUGIN_DIR)}: {label}")
        self.assertEqual(offenders, [])

    def test_skill_files_are_thin_playbook_shims(self):
        # lead-proceed, lead-write-ticket, lead-write-spec, lead-add-rule,
        # lead-bootstrap, lead-forge-mental-model, lead-forge-spec,
        # lead-review, lead-ship, lead-tune, lead-implement,
        # lead-check-blockers, lead-update-spec, and lead-workflow-manual all
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
        ):
            path = SKILLS_DIR / skill / "SKILL.md"
            text = path.read_text(encoding="utf-8")
            match = re.fullmatch(
                r"---\n"
                rf"name: {re.escape(skill)}\n"
                r"description: .+\n"
                r"---\n\n"
                r"# .+\n\n"
                rf"Call `wsflow/playbook\.print\(name: \"{re.escape(skill)}\"\)` and execute the returned procedure\n"
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
                rf"Call `wsflow/playbook\.print\(name: \"{re.escape(skill)}\"\)` and execute the returned procedure\n"
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
            "lead-backfill-docs": r"\nIf this call fails to connect, run `/wsflow:mcp-server-repair`\.",
            "lead-discuss": r"\nIf this call fails to connect, run `/wsflow:mcp-server-repair`\.",
            "lead-goal-fan-out-step": r"\nIf this call fails to connect, run `/wsflow:mcp-server-repair`\.",
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
                rf"- `wsflow/playbook\.print\(name: \"{re.escape(skill)}\", session_key: <your key, omit if fresh>\)`\n"
                r'- `wsflow/workflow_manual\(session_key: <your key or "obsidian-latch" if fresh>, root: <absolute worktree path if fresh>\)`\n\n'
                r"After both return, execute the procedure returned by `wsflow/playbook\.print`\."
                + pointer_tail[skill]
                + r"\n",
                text,
            )
            if match is None:
                offenders.append(str(path.relative_to(PLUGIN_DIR)))
        self.assertEqual(offenders, [])

    def test_skill_shims_point_to_shared_playbooks(self):
        missing = []
        for skill in sorted(EXPECTED_SKILLS - EXPECTED_INLINE_SKILLS):
            subdir_playbook = FULL_PLUGIN_RSRC_DIR / skill / f"{skill}.md"
            flat_playbook = FULL_PLUGIN_RSRC_DIR / f"{skill}.md"
            if not subdir_playbook.exists() and not flat_playbook.exists():
                missing.append(skill)
        self.assertEqual(missing, [])

    def test_bootstrap_template_uses_wsflow_local_version_lineage(self):
        text = (SKILLS_DIR / "lead-bootstrap" / "AGENTS.template.md").read_text(encoding="utf-8")
        self.assertIn("<!-- Template Version: v0006 -->", text)
        self.assertIn("This template has package-local version history", text)

        # The forbidden markers are derived from the full-plugin lineage's
        # *current* state rather than pinned to a literal version, so this
        # guard keeps working as both lineages gain versions without a hand
        # edit. A hardcoded literal (e.g. "v0038") only catches contamination
        # from that one exact version and goes silent on every later one.
        full_text = (FULL_PLUGIN_SKILLS_DIR / "lead-bootstrap" / "AGENTS.template.md").read_text(
            encoding="utf-8"
        )

        full_tag_match = re.search(r"<!-- Template Version: v\d{4} -->", full_text)
        self.assertIsNotNone(
            full_tag_match, "full-plugin AGENTS.template.md is missing its version tag"
        )
        self.assertNotIn(full_tag_match.group(0), text)

        # Compare on bullet *content*, not bullet number: both lineages number
        # their changelogs independently starting at v0001, so wsflow's own
        # v0001-v0006 bullets legitimately share numbers with (differently
        # worded) full-plugin bullets. A number-only check would either miss
        # real contamination or false-positive on the wsflow copy's own
        # history. Matching full lines instead catches a leaked bullet from
        # any full-plugin version - including the current highest one - while
        # leaving wsflow's own same-numbered bullets untouched.
        full_bullet_lines = re.findall(r"(?m)^- v\d{4}:.*$", full_text)
        self.assertTrue(
            full_bullet_lines, "full-plugin AGENTS.template.md has no changelog bullets to compare"
        )
        leaked_bullets = [line for line in full_bullet_lines if line in text]
        self.assertEqual(leaked_bullets, [])


if __name__ == "__main__":
    unittest.main()
