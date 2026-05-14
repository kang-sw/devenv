import re
import unittest
from pathlib import Path


PLUGIN_DIR = Path(__file__).resolve().parents[1]
FULL_PLUGIN_SKILLS_DIR = PLUGIN_DIR.parent / "agents-plugin" / "skills"
SKILLS_DIR = PLUGIN_DIR / "skills"

EXPECTED_SKILLS = {
    "lead-add-rule",
    "lead-bootstrap",
    "lead-discuss",
    "lead-edit",
    "lead-forge-mental-model",
    "lead-forge-spec",
    "lead-implement",
    "lead-can-we-proceed",
    "lead-proceed",
    "lead-review",
    "lead-ship",
    "lead-sprint",
    "lead-update-spec",
    "lead-verify-discussion",
    "lead-workflow-manual",
    "lead-write-spec",
    "lead-write-ticket",
}

FORBIDDEN_PATTERNS = {
    "full ws MCP notation": re.compile(r"\bws/"),
    "full ws skill namespace": re.compile(r"\bws:"),
    "full ws dotted namespace": re.compile(r"\bws\."),
    "full ws query tool": re.compile(r"\bsubquery\b"),
    "full ws agent dotted tool": re.compile(r"\bagents\."),
    "full ws mental model updater": re.compile(r"\bmental-model-updater\b"),
    "excluded write-code skill": re.compile(r"\blead-write-code\b"),
    "excluded write-skeleton skill": re.compile(r"\blead-write-skeleton\b"),
    "excluded salvage skill": re.compile(r"\blead-salvage\b"),
    "excluded authoring skill": re.compile(r"\blead-skill-authoring\b"),
}


class WsflowSkillBundleTest(unittest.TestCase):
    def test_shipped_skill_inventory_is_curated(self):
        actual = {path.name for path in SKILLS_DIR.iterdir() if path.is_dir()}
        self.assertEqual(actual, EXPECTED_SKILLS)

    def test_full_skill_inventory_drift_is_visible(self):
        full_skills = {path.name for path in FULL_PLUGIN_SKILLS_DIR.iterdir() if path.is_dir()}
        missing_full_counterparts = sorted(EXPECTED_SKILLS - full_skills)
        unexpected_wsflow_skills = sorted(
            {path.name for path in SKILLS_DIR.iterdir() if path.is_dir()} - EXPECTED_SKILLS
        )

        self.assertEqual(missing_full_counterparts, [])
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

    def test_workflow_manual_documents_subagent_guidance(self):
        text = (SKILLS_DIR / "lead-workflow-manual" / "SKILL.md").read_text(encoding="utf-8")
        self.assertIn("Use subagents when a task benefits from scoped exploration", text)
        self.assertIn("The lead owns integration, verification, final judgment, and commits.", text)

    def test_bootstrap_template_uses_wsflow_local_version_lineage(self):
        text = (SKILLS_DIR / "lead-bootstrap" / "AGENTS.template.md").read_text(encoding="utf-8")
        self.assertIn("<!-- Template Version: v0002 -->", text)
        self.assertIn("This template has package-local version history", text)
        self.assertNotIn("<!-- Template Version: v0038 -->", text)
        self.assertNotIn("- v0038:", text)


if __name__ == "__main__":
    unittest.main()
