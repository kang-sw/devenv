package wsrsrc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// composedSkills is the curated, bounded mapping of build-time skill-body
// splices. Like substitutionMirroredSkills this is not a blanket mechanism —
// adding an entry requires updating ai-docs/manuals/wsflow-mirroring.md in the
// same change.
// Currently empty: the sole entry (lead-prefer-subagent spliced into
// lead-drain-ready-queue) died with that skill, so the on-disk drift guard and
// regen entrypoint below are vacuous until the mapping gains an entry again.
// ComposeSkillBody's own behavior is therefore covered by the synthetic
// fixtures in TestComposeSkillBodyInsertsThenReplacesRegion and
// TestComposeSkillBodyRejectsMissingAnchor, which do not depend on the
// mapping.
var composedSkills = []SkillSplice{}

// composeSplice reads the on-disk source body for splice and returns the
// composed form of the given target text.
func composeSplice(t *testing.T, splice SkillSplice, target string) string {
	t.Helper()
	sourceBody, err := LoadSkillBody(fullSkillsRoot(), splice.Source)
	if err != nil {
		t.Fatalf("load splice source %s: %v", splice.Source, err)
	}
	composed, err := ComposeSkillBody(target, splice, sourceBody)
	if err != nil {
		t.Fatalf("compose %s: %v", splice.Target, err)
	}
	return composed
}

// TestComposedSkillsUpToDate is the drift guard for build-time skill
// composition: the committed full-ws SKILL.md of each splice target must equal
// the freshly composed output. It runs before the wsflow substitution mirror
// in the regen order, so a stale target here also means a stale wsflow copy.
func TestComposedSkillsUpToDate(t *testing.T) {
	for _, splice := range composedSkills {
		targetPath := filepath.Join(fullSkillsRoot(), splice.Target, "SKILL.md")
		got, err := os.ReadFile(targetPath)
		if err != nil {
			t.Fatalf("read splice target %s: %v", targetPath, err)
		}
		want := composeSplice(t, splice, string(got))
		if string(got) != want {
			t.Fatalf("composed skill %s has drifted from its splice sources.\n"+
				"Regenerate with: WS_REGEN_COMPOSED_SKILLS=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateComposedSkills",
				splice.Target)
		}
	}
}

// TestComposeSkillBodyInsertsThenReplacesRegion covers the whole happy path on
// a synthetic target, independent of the curated mapping: first generation
// inserts the region before the anchor heading, and a second generation locates
// it by its delimiter pair and replaces it in place rather than stacking
// another copy. Without the second half, every regeneration would append one
// more body to the target.
func TestComposeSkillBodyInsertsThenReplacesRegion(t *testing.T) {
	const target = "---\nname: fixture-target\n---\n\n# Fixture Target\n\n## Posture\n\nPosture body.\n\n## Select\n\nSelect body.\n"
	splice := SkillSplice{
		Target:        "fixture-target",
		Source:        "fixture-source",
		Title:         "Fixture Source",
		AnchorHeading: "## Select",
	}

	once, err := ComposeSkillBody(target, splice, "Source body.")
	if err != nil {
		t.Fatalf("first compose: %v", err)
	}
	if n := strings.Count(once, `<playbook name="fixture-source"`); n != 1 {
		t.Fatalf("expected exactly 1 spliced region after first compose, got %d:\n%s", n, once)
	}
	if !strings.Contains(once, "Source body.") {
		t.Fatalf("spliced region is missing the source body:\n%s", once)
	}
	regionIdx := strings.Index(once, `<playbook name="fixture-source"`)
	if anchorIdx := strings.Index(once, "\n## Select\n"); regionIdx < 0 || anchorIdx < 0 || regionIdx >= anchorIdx {
		t.Fatalf("region (index %d) must be inserted before the anchor heading (index %d):\n%s", regionIdx, anchorIdx, once)
	}
	if !strings.HasPrefix(once, "---\nname: fixture-target\n---\n") {
		t.Fatalf("frontmatter must be untouched:\n%s", once)
	}

	twice, err := ComposeSkillBody(once, splice, "Source body.")
	if err != nil {
		t.Fatalf("second compose: %v", err)
	}
	if once != twice {
		t.Fatalf("composing twice is not a no-op; region replacement is not idempotent")
	}

	updated, err := ComposeSkillBody(once, splice, "Replaced body.")
	if err != nil {
		t.Fatalf("compose with changed source: %v", err)
	}
	if n := strings.Count(updated, `<playbook name="fixture-source"`); n != 1 {
		t.Fatalf("expected the region to be replaced in place, got %d regions:\n%s", n, updated)
	}
	if strings.Contains(updated, "Source body.") || !strings.Contains(updated, "Replaced body.") {
		t.Fatalf("region was not replaced with the new source body:\n%s", updated)
	}
}

// TestComposeSkillBodyRejectsMissingAnchor covers the first-generation failure
// path: with no existing region and no anchor heading, composition must fail
// loudly rather than silently appending or dropping the region.
func TestComposeSkillBodyRejectsMissingAnchor(t *testing.T) {
	splice := SkillSplice{
		Target:        "fixture-target",
		Source:        "fixture-source",
		Title:         "Fixture Source",
		AnchorHeading: "## Absent",
	}
	if _, err := ComposeSkillBody("---\nname: fixture\n---\n\n# Fixture\n\nBody.\n", splice, "source body"); err == nil {
		t.Fatal("expected composition to fail when the anchor heading is absent")
	}
}

// TestRegenerateComposedSkills rewrites each splice target's committed
// SKILL.md from its splice sources. It is a no-op unless
// WS_REGEN_COMPOSED_SKILLS=1, so an ordinary test run never mutates the source
// tree. Uses a distinct env var from WS_REGEN_WSFLOW_SKILLS (wsflow mirror
// regen), WS_REGEN_WSFLOW_RSRC (rsrc mirror regen), and WSRSRC_REGEN_SKILLS
// (manifest regen) so a single flag never regenerates an unrelated surface.
//
// Run this BEFORE the wsflow skills mirror regen: the mirror derives from the
// composed full-ws source.
func TestRegenerateComposedSkills(t *testing.T) {
	if os.Getenv("WS_REGEN_COMPOSED_SKILLS") != "1" {
		t.Skip("set WS_REGEN_COMPOSED_SKILLS=1 to regenerate composed skill bodies")
	}
	for _, splice := range composedSkills {
		targetPath := filepath.Join(fullSkillsRoot(), splice.Target, "SKILL.md")
		raw, err := os.ReadFile(targetPath)
		if err != nil {
			t.Fatalf("read splice target %s: %v", targetPath, err)
		}
		out := composeSplice(t, splice, string(raw))
		if err := os.WriteFile(targetPath, []byte(out), 0o644); err != nil {
			t.Fatalf("write %s: %v", targetPath, err)
		}
	}
	t.Logf("regenerated %d composed skill(s)", len(composedSkills))
}
