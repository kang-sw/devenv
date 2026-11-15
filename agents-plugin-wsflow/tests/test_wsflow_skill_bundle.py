import re
import unittest
from pathlib import Path


PLUGIN_DIR = Path(__file__).resolve().parents[1]
FULL_PLUGIN_SKILLS_DIR = PLUGIN_DIR.parent / "agents-plugin" / "skills"
# A mirrored skill's full-ws counterpart may be a skill or an rsrc playbook.
FULL_PLUGIN_RSRC_DIR = PLUGIN_DIR.parent / "agents-plugin" / "rsrc"
SKILLS_DIR = PLUGIN_DIR / "skills"
RSRC_DIR = PLUGIN_DIR / "rsrc"


EXPECTED_SKILLS = {
    "lead-audit-doc",
    "lead-coauthor-paged-document",
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
    "lead-use-mailbox",
}

# wsflow-only backward-compat aliases mapped to their shared playbooks.
WSFLOW_ALIAS_TARGET = {"lead-proceed": "lead-run"}

EXPECTED_WSFLOW_ONLY_SKILLS: set = set(WSFLOW_ALIAS_TARGET)
EXPECTED_INLINE_SKILLS = {
    "lead-revive",
}
# Inline skills and aliases carry no own-playbook preload block.
EXPECTED_PRELOAD_EXCLUDED: set = set()

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

# Strip only scaffold comments before comparing emitted output across packages.
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

    def test_skill_files_load(self):
        for skill in sorted(EXPECTED_SKILLS):
            text = (SKILLS_DIR / skill / "SKILL.md").read_text(encoding="utf-8")
            self.assertTrue(text.strip(), skill)

    def test_preload_blocks_name_their_own_playbook(self):
        # The Claude mod expands a block only when its playbook attribute names
        # the skill being expanded, so a block naming another playbook would
        # silently stay a model read. Excluded skills (none today), the inline
        # revive, and the aliases carry no block at all.
        no_block = EXPECTED_PRELOAD_EXCLUDED | EXPECTED_INLINE_SKILLS | set(WSFLOW_ALIAS_TARGET)
        offenders = []
        for skill in sorted(EXPECTED_SKILLS):
            text = (SKILLS_DIR / skill / "SKILL.md").read_text(encoding="utf-8")
            names = re.findall(r'<ws-preload playbook="([^"]+)">', text)
            expected = [] if skill in no_block else [skill]
            if names != expected or text.count("</ws-preload>") != len(expected):
                offenders.append(f"{skill}: blocks {names}, expected {expected}")
        self.assertEqual(offenders, [])

    def test_preload_shim_headings_sit_inside_their_blocks(self):
        # Expansion replaces the shim heading with the playbook heading.
        offenders = []
        for skills_dir in (SKILLS_DIR, FULL_PLUGIN_SKILLS_DIR):
            for path in sorted(skills_dir.glob("*/SKILL.md")):
                text = path.read_text(encoding="utf-8")
                block = re.search(r'<ws-preload playbook="([^"]+)">\n(# [^\n]+)\n', text)
                if "<ws-preload" not in text:
                    continue
                label = f"{skills_dir.parent.name}/{path.parent.name}"
                body = re.sub(r"\A---\n.*?\n---\n\n", "", text, flags=re.DOTALL)
                if block is None or not body.startswith("<ws-preload "):
                    offenders.append(f"{label}: the heading does not open the block")
                    continue
                playbook = FULL_PLUGIN_RSRC_DIR / block.group(1) / f"{block.group(1)}.md"
                heading = re.search(r"^# .+$", playbook.read_text(encoding="utf-8"), re.M)
                if heading is None or heading.group(0) != block.group(2):
                    offenders.append(f"{label}: {block.group(2)!r} differs from the playbook heading")
        self.assertEqual(offenders, [])

    def test_skill_shims_point_to_shared_playbooks(self):
        # Aliases target a different shared stem, checked separately below.
        missing = []
        for skill in sorted(EXPECTED_SKILLS - EXPECTED_INLINE_SKILLS - set(WSFLOW_ALIAS_TARGET)):
            subdir_playbook = FULL_PLUGIN_RSRC_DIR / skill / f"{skill}.md"
            flat_playbook = FULL_PLUGIN_RSRC_DIR / f"{skill}.md"
            if not subdir_playbook.exists() and not flat_playbook.exists():
                missing.append(skill)
        self.assertEqual(missing, [])

    def test_wsflow_only_aliases_route_to_target(self):
        offenders = []
        for alias, target in sorted(WSFLOW_ALIAS_TARGET.items()):
            path = SKILLS_DIR / alias / "SKILL.md"
            text = path.read_text(encoding="utf-8")
            subdir_playbook = FULL_PLUGIN_RSRC_DIR / target / f"{target}.md"
            flat_playbook = FULL_PLUGIN_RSRC_DIR / f"{target}.md"
            if not subdir_playbook.exists() and not flat_playbook.exists():
                offenders.append(f"{path.relative_to(PLUGIN_DIR)}: target playbook {target} missing")
                continue
            if f'wsflow/playbook.read(name: "{target}"' not in text:
                offenders.append(f"{path.relative_to(PLUGIN_DIR)}: target playbook read missing")
        self.assertEqual(offenders, [])

    def test_wsflow_escalated_retired(self):
        self.assertFalse((RSRC_DIR / "ticket-worker-escalated").exists())

    def test_bootstrap_scaffolds_emit_converged_output_across_packages(self):
        # Compare emitted templates and their migration-ordinal tags, not prose.
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
