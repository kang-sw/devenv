package mcp

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/kang-sw/devenv/internal/wsreview"
)

// htmlCommentRE strips template-internal comment blocks, approximating what
// the bootstrap fresh handler does when it copies AGENTS.template.md into a
// new project's AGENTS.md "stripping template-internal migration blocks".
var htmlCommentRE = regexp.MustCompile(`(?s)<!--.*?-->`)

// bootstrapTemplatePaths are the shipped AGENTS.template.md copies of both
// packages, relative to this package dir.
func bootstrapTemplatePaths() map[string]string {
	return map[string]string{
		"ws": filepath.Join("..", "..", "..", "agents-plugin",
			"skills", "lead-bootstrap", "AGENTS.template.md"),
		"wsflow": filepath.Join("..", "..", "..", "agents-plugin-wsflow",
			"skills", "lead-bootstrap", "AGENTS.template.md"),
	}
}

// TestBootstrapTemplateDeclaresNoBindingAnchor pins the contract between the
// shipped `### Binding Anchor` scaffold and the parser that reads it. The
// section is optional, and a project that has not filled it in must parse as
// undeclared: a declared anchor turns the binding-anchor proceed fact live and
// splices the anchor path into the worker Prep guardrail, so a scaffold that
// parses would give every freshly bootstrapped project a gate pointing at a
// placeholder path that resolves to nothing.
//
// ReadAgentsBindingAnchor matches `anchor:`/`topics:` at the start of any line
// in the section body and does not exclude code fences, so the scaffold must
// keep its example keys out of column 0 and ship no filled-in fence. Both the
// file as shipped and the comment-stripped fresh-mode body are checked.
func TestBootstrapTemplateDeclaresNoBindingAnchor(t *testing.T) {
	for pkg, path := range bootstrapTemplatePaths() {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("%s: read template: %v", pkg, err)
		}
		bodies := map[string]string{
			"as shipped":        string(raw),
			"fresh-mode output": htmlCommentRE.ReplaceAllString(string(raw), ""),
		}
		for label, body := range bodies {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte(body), 0o644); err != nil {
				t.Fatalf("%s: write AGENTS.md: %v", pkg, err)
			}
			if got := wsreview.ReadAgentsBindingAnchor(dir); got.Declared {
				t.Errorf("%s (%s): shipped binding-anchor scaffold parses as declared "+
					"(anchor=%q topics=%q); an unfilled section must parse as undeclared",
					pkg, label, got.Anchor, got.Topics)
			}
		}
	}
}
