import re
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
    "lead-audit-doc",
    "lead-use-mailbox",
    "lead-coauthor-paged-document",
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
    def test_document_audit_preserves_dispatch_boundary(self):
        for package in (SKILLS_DIR.parent, SKILLS_DIR.parent.parent / "agents-plugin-wsflow"):
            shim = (package / "skills" / "lead-audit-doc" / "SKILL.md").read_text(encoding="utf-8")
            self.assertNotIn("allow_implicit_invocation: false", shim)
            for root in (package / "skills", package / "rsrc"):
                self.assertFalse((root / "lead-add-rule").exists())
                for path in root.rglob("*.md"):
                    self.assertNotIn("lead-add-rule", path.read_text(encoding="utf-8"), str(path))
            self.assertFalse((package / "skills" / "fresh-read-doc-auditor").exists())

    def test_delegate_implementer_is_tier_unaware(self):
        for package in (RSRC_DIR, SKILLS_DIR.parent.parent / "agents-plugin-wsflow" / "rsrc"):
            body = (package / "delegate-implementer" / "delegate-implementer.md").read_text(encoding="utf-8")
            head = body.split("---", 2)[1]
            self.assertNotIn("tier:", head)
        self.assertFalse((SKILLS_DIR / "delegate-implementer").exists())

    def test_lead_skill_surface_is_collapsed(self):
        actual = {path.name for path in SKILLS_DIR.iterdir() if path.is_dir()}
        self.assertEqual(actual, EXPECTED_LEAD_SKILLS)

    def test_lead_shims_wrap_their_own_playbook_read_in_a_preload_block(self):
        # The Claude mod replaces a `<ws-preload playbook="...">` block with the
        # playbook body only when the attribute names the skill being expanded,
        # so a block naming another playbook would silently stay a model read.
        # lead-revive reads no playbook, so it has no block. lead-use-mailbox
        # has one like every other shim: the mod replaces the key in its
        # key-specific wait command with a placeholder before inlining it.
        no_block = {"lead-revive"}
        offenders = []
        for skill in sorted(EXPECTED_LEAD_SKILLS):
            text = (SKILLS_DIR / skill / "SKILL.md").read_text(encoding="utf-8")
            names = re.findall(r'<ws-preload playbook="([^"]+)">', text)
            expected = [] if skill in no_block else [skill]
            if names != expected or text.count("</ws-preload>") != len(expected):
                offenders.append(f"{skill}: blocks {names}, expected {expected}")
                continue
            if expected:
                block = text.split(f'<ws-preload playbook="{skill}">', 1)[1].split("</ws-preload>", 1)[0]
                if f'ws/playbook.read(name: "{skill}"' not in block:
                    offenders.append(f"{skill}: the block does not wrap its own playbook read")
        self.assertEqual(offenders, [])

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

    def test_placed_lead_bodies_carry_no_unresolved_review_marker(self):
        for skill in ("lead-discuss", "lead-ticket", "lead-review", "lead-ship"):
            text = (RSRC_DIR / skill / f"{skill}.md").read_text(encoding="utf-8")
            self.assertNotIn("[design-review:", text, msg=skill)

    def test_worker_roles_and_retired_surface(self):
        for rsrc in (RSRC_DIR, RSRC_DIR.parent.parent / "agents-plugin-wsflow" / "rsrc"):
            self.assertFalse((rsrc / "ticket-worker-escalated").exists(), str(rsrc))
            for name in ("ticket-worker", "ticket-worker-elevated"):
                text = (rsrc / name / f"{name}.md").read_text(encoding="utf-8")
                head = text.split("---", 2)[1]
                self.assertIn("role: worker", head)


if __name__ == "__main__":
    unittest.main()
