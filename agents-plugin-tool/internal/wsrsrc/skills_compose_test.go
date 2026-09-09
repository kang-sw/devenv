package wsrsrc

import (
	"os"
	"path/filepath"
	"testing"
)

// composedSkills is the curated, bounded mapping of build-time skill-body
// splices. Like substitutionMirroredSkills this is not a blanket mechanism —
// adding an entry requires updating ai-docs/manuals/wsflow-mirroring.md in the
// same change.
// Currently empty: the sole entry (lead-prefer-subagent spliced into
// lead-drain-ready-queue) died with that skill. The mechanism is retained
// because the mapping is expected to gain entries again; ComposeSkillBody's
// own behavior stays covered by the fixture-driven tests below.
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

// TestComposeSkillBodyIsIdempotent proves the region is located and replaced by
// its delimiter pair rather than appended again: composing an already-composed
// target must be a byte-for-byte no-op. Without this, every regeneration would
// stack another copy of the source body into the target.
func TestComposeSkillBodyIsIdempotent(t *testing.T) {
	for _, splice := range composedSkills {
		targetPath := filepath.Join(fullSkillsRoot(), splice.Target, "SKILL.md")
		raw, err := os.ReadFile(targetPath)
		if err != nil {
			t.Fatalf("read splice target %s: %v", targetPath, err)
		}
		once := composeSplice(t, splice, string(raw))
		twice := composeSplice(t, splice, once)
		if once != twice {
			t.Fatalf("composing %s twice is not a no-op; region replacement is not idempotent", splice.Target)
		}
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
