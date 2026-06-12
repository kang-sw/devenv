package mcp

import (
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsrsrc"
)

// TestShippedAPIDocPromptsRenderFromRsrc verifies the api.ask prompts moved off
// the embedded wsprompt bundle (Phase 6, 260611) are loadable from the shipped
// rsrc tree and carry the ported body content. These are var-free `kind: print`
// playbooks, so a nil-vars Load returns the verbatim body (what renderAPIPrompt
// hands to RegisterOptions.SystemPromptText).
func TestShippedAPIDocPromptsRenderFromRsrc(t *testing.T) {
	rsrcRoot := shippedRsrcRootForTest()
	cases := map[string]string{
		"pre-router":          "API documentation pre-router",
		"api-doc-manager":     "API documentation manager",
		"api-doc-cargo-brief": "cargo-brief",
	}
	for stem, want := range cases {
		t.Run(stem, func(t *testing.T) {
			pb, err := wsrsrc.Load(rsrcRoot, stem, "claude", nil)
			if err != nil {
				t.Fatalf("Load(%s): %v", stem, err)
			}
			if !strings.Contains(pb.Body, want) {
				t.Fatalf("rsrc %s body missing %q:\n%s", stem, want, pb.Body)
			}
		})
	}
}
