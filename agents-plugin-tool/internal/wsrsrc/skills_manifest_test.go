package wsrsrc

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// ---------------------------------------------------------------------------
// agents-plugin/skills/manifest.json — independent parallel manifest
// ---------------------------------------------------------------------------
//
// This is a separate, independent mechanism from the rsrc manifest: it hashes
// the agents-plugin/skills/ tree (SKILL.md entry-skill files with no
// kind:/delegates: frontmatter semantics, no harness overlays, no includes),
// reusing the same GenerateManifest/WriteManifest primitives. It is not an
// extension of the rsrc manifest schema and has no relationship to
// wsrsrc.Validate, which assumes rsrc-specific per-playbook structure
// (required <name>/<name>.md base file) that the skills tree does not follow.

func skillsRootForTest() string {
	return filepath.Join("..", "..", "..", "agents-plugin", "skills")
}

// TestSkillsManifestDriftIsVisible fails if agents-plugin/skills/manifest.json
// is stale relative to the committed skills tree. This is the CI drift gate,
// parallel to TestValidateRealTree for the rsrc tree.
func TestSkillsManifestDriftIsVisible(t *testing.T) {
	root := skillsRootForTest()

	want, err := GenerateManifest(root)
	if err != nil {
		t.Fatalf("GenerateManifest(%s): %v", root, err)
	}

	got, err := ReadManifest(root)
	if err != nil {
		t.Fatalf("ReadManifest(%s): %v", root, err)
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("agents-plugin/skills/manifest.json is stale; regenerate with:\n"+
			"  cd agents-plugin-tool && WSRSRC_REGEN_SKILLS=1 go test ./internal/wsrsrc/... -run TestGenerateRealSkillsManifest -v\n"+
			"got:  %+v\nwant: %+v", got, want)
	}
}

// TestGenerateRealSkillsManifest regenerates agents-plugin/skills/manifest.json
// from the current tree. Run with WSRSRC_REGEN_SKILLS=1 to update after editing
// skill files. Uses a distinct env var from the rsrc regen test (WSRSRC_REGEN)
// so a single flag never accidentally regenerates both manifests at once.
//
//	cd agents-plugin-tool && WSRSRC_REGEN_SKILLS=1 go test ./internal/wsrsrc/... -run TestGenerateRealSkillsManifest -v
func TestGenerateRealSkillsManifest(t *testing.T) {
	if os.Getenv("WSRSRC_REGEN_SKILLS") == "" {
		t.Skip("set WSRSRC_REGEN_SKILLS=1 to regenerate agents-plugin/skills/manifest.json")
	}
	root := skillsRootForTest()
	m, err := GenerateManifest(root)
	if err != nil {
		t.Fatalf("GenerateManifest: %v", err)
	}
	if err := WriteManifest(root, m); err != nil {
		t.Fatalf("WriteManifest: %v", err)
	}
	t.Logf("regenerated %s/manifest.json with %d files", root, len(m.Files))
}
