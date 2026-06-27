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
    "lead-bootstrap",
    "lead-discuss",
    "lead-forge-mental-model",
    "lead-forge-spec",
    "lead-implement",
    "lead-check-blockers",
    "lead-proceed",
    "lead-review",
    "lead-ship",
    "lead-sprint",
    "lead-tune",
    "lead-update-spec",
    "lead-verify-design",
    "lead-verify-discussion",
    "lead-workflow-manual",
    "lead-write-spec",
    "lead-write-ticket",
    "lead-prefer-subagent",
    "lead-revive",
}

EXPECTED_WSFLOW_ONLY_SKILLS: set = set()
EXPECTED_INLINE_SKILLS = {"lead-revive"}

FORBIDDEN_PATTERNS = {
    "full ws MCP notation": re.compile(r"\bws/"),
    "full ws skill namespace": re.compile(r"\bws:"),
    "full ws dotted namespace": re.compile(r"\bws\."),
    "full ws query tool": re.compile(r"\bsubquery\b"),
    "full ws agent dotted tool": re.compile(r"\bagents\."),
    "excluded write-code skill": re.compile(r"\blead-write-code\b"),
    "excluded write-skeleton skill": re.compile(r"\blead-write-skeleton\b"),
    "excluded salvage skill": re.compile(r"\blead-salvage\b"),
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
        offenders = []
        for skill in sorted(EXPECTED_SKILLS - EXPECTED_INLINE_SKILLS):
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
        self.assertIn("<!-- Template Version: v0004 -->", text)
        self.assertIn("This template has package-local version history", text)
        self.assertNotIn("<!-- Template Version: v0038 -->", text)
        self.assertNotIn("- v0038:", text)


if __name__ == "__main__":
    unittest.main()
