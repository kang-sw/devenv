package wsrsrc

import (
	"strings"
	"testing"
)

// profileTree is a playbook with a profile variant, a harness overlay of an
// include, and a profile variant of another include.
func profileTree(t *testing.T, extra map[string]string) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"pb/pb.md":               "---\nkind: print\nincludes:\n  - core\n  - spawn\n---\nBase body.\n",
		"pb/pb.orchestrate.md":   "---\nkind: print\nincludes:\n  - core\n  - spawn\n---\nOrchestrate body.\n",
		"pb/core.md":             "Base core.\n",
		"pb/core.orchestrate.md": "Orchestrate core.\n",
		"pb/spawn.md":            "Base spawn.\n",
		"pb/spawn.codex.md":      "Codex spawn.\n",
		"other/other.md":         "---\nkind: print\n---\nOther base.\n",
		"other/other.codex.md":   "---\nkind: print\n---\nOther codex.\n",
		"flat.md":                "---\nkind: print\n---\nFlat base.\n",
		"flat.orchestrate.md":    "Flat orchestrate.\n",
	}
	for rel, body := range extra {
		files[rel] = body
	}
	for rel, body := range files {
		writeFile(t, root, rel, body)
	}
	m, err := GenerateManifest(root)
	if err != nil {
		t.Fatalf("GenerateManifest: %v", err)
	}
	if err := WriteManifest(root, m); err != nil {
		t.Fatalf("WriteManifest: %v", err)
	}
	return root
}

func TestLoadProfileLookupOrder(t *testing.T) {
	root := profileTree(t, nil)
	for _, tc := range []struct {
		name, playbook, harness, profile string
		want, wantProfile, wantHarness   string
		absent                           []string
	}{
		{"profile variant and its includes", "pb", "codex", "orchestrate", "Orchestrate body.\n\nOrchestrate core.\n\nCodex spawn.", "orchestrate", "", nil},
		{"direct reads base files", "pb", "codex", "direct", "Base body.\n\nBase core.\n\nCodex spawn.", "", "", nil},
		{"no profile reads base files", "pb", "", "", "Base body.\n\nBase core.\n\nBase spawn.", "", "", nil},
		{"harness overlay when no profile variant", "other", "codex", "orchestrate", "Other codex.", "", "codex", nil},
		{"flat playbook has no profile variant", "flat", "", "orchestrate", "Flat base.", "", "", []string{"Flat orchestrate."}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pb, err := Load(root, tc.playbook, tc.harness, tc.profile, nil)
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if pb.Body != tc.want {
				t.Errorf("Body = %q, want %q", pb.Body, tc.want)
			}
			if pb.Profile != tc.wantProfile || pb.Harness != tc.wantHarness {
				t.Errorf("loaded variant profile=%q harness=%q, want profile=%q harness=%q", pb.Profile, pb.Harness, tc.wantProfile, tc.wantHarness)
			}
		})
	}
}

func TestLoadRefusesUnknownProfile(t *testing.T) {
	root := profileTree(t, nil)
	if _, err := Load(root, "pb", "", "fancy", nil); err == nil || !strings.Contains(err.Error(), "invalid playbook profile") {
		t.Fatalf("Load with an unknown profile: err = %v, want an invalid-profile error", err)
	}
}

func TestValidateRefusesProfileAndHarnessVariantsOfOneStem(t *testing.T) {
	for _, tc := range []struct{ name, file string }{
		{"playbook", "pb/pb.codex.md"},
		{"include", "pb/core.claude.md"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := profileTree(t, map[string]string{tc.file: "---\nkind: print\n---\nOverlay.\n"})
			err := Validate(root)
			if err == nil || !strings.Contains(err.Error(), "both a profile variant") {
				t.Fatalf("Validate = %v, want a profile/harness conflict error", err)
			}
		})
	}
	if err := Validate(profileTree(t, nil)); err != nil {
		t.Fatalf("Validate of a tree with disjoint variants: %v", err)
	}
}
