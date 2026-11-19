package wsrsrc

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"testing/fstest"
)

// TestPreloadCommandsFromShippedSkills: each shipped package's command list
// is its skill shims whose preload block reads their own playbook, so
// lead-run is there with its description and the inline recovery skill
// (lead-revive) and the retired alias (lead-proceed) are not.
func TestPreloadCommandsFromShippedSkills(t *testing.T) {
	for _, pkg := range []string{"agents-plugin", "agents-plugin-wsflow"} {
		t.Run(pkg, func(t *testing.T) {
			got, err := PreloadCommands(os.DirFS(filepath.Join("..", "..", "..", pkg, "skills")))
			if err != nil {
				t.Fatal(err)
			}
			var names []string
			for _, cmd := range got {
				names = append(names, cmd.Name)
				if cmd.Description == "" {
					t.Errorf("%s has no description", cmd.Name)
				}
			}
			for _, want := range []string{"lead-run", "lead-discuss", "lead-ticket", "lead-review"} {
				if !slices.Contains(names, want) {
					t.Errorf("%s is not a command: %q", want, names)
				}
			}
			for _, absent := range []string{"lead-revive", "lead-proceed"} {
				if slices.Contains(names, absent) {
					t.Errorf("%s is a command: %q", absent, names)
				}
			}
			if !slices.IsSorted(names) {
				t.Errorf("commands are not sorted by name: %q", names)
			}
		})
	}
}

// TestPreloadCommandsKeepsOnlySelfPreloadingShims: a directory without a
// SKILL.md, a skill whose procedure is inline, and an alias whose block
// reads another playbook are not commands.
func TestPreloadCommandsKeepsOnlySelfPreloadingShims(t *testing.T) {
	skills := fstest.MapFS{
		"zeta/SKILL.md":   {Data: []byte("---\nname: zeta\ndescription: Last one.\n---\n\n<ws-preload playbook=\"zeta\">\nread it\n</ws-preload>\n")},
		"alpha/SKILL.md":  {Data: []byte("---\nname: alpha\ndescription: \"First one.\"\n---\n\n<ws-preload playbook=\"alpha\">\n</ws-preload>\n")},
		"inline/SKILL.md": {Data: []byte("---\nname: inline\ndescription: Inline.\n---\n\nDo the steps here.\n")},
		"alias/SKILL.md":  {Data: []byte("---\nname: alias\ndescription: Old name.\n---\n\n<ws-preload playbook=\"alpha\">\n</ws-preload>\n")},
		"empty/notes.md":  {Data: []byte("no skill")},
		"manifest.json":   {Data: []byte("{}")},
	}
	got, err := PreloadCommands(skills)
	if err != nil {
		t.Fatal(err)
	}
	want := []PreloadCommand{{Name: "alpha", Description: "First one."}, {Name: "zeta", Description: "Last one."}}
	if !slices.Equal(got, want) {
		t.Fatalf("commands = %+v, want %+v", got, want)
	}
}
