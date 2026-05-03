package wsprompt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveEmbeddedPromptChain(t *testing.T) {
	resolved, err := Resolve([]string{"code-reviewer", "code-review-correctness", "code-review-fit"}, "", "", "")
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if resolved.Tier != "core" {
		t.Fatalf("tier = %q", resolved.Tier)
	}
	if strings.Contains(resolved.Text, "model: sonnet") {
		t.Fatalf("frontmatter was not stripped:\n%s", resolved.Text)
	}
	if !strings.Contains(resolved.Text, "You are a code reviewer.") {
		t.Fatalf("missing reviewer prompt:\n%s", resolved.Text)
	}
	if !strings.Contains(resolved.Text, "Correctness Partition") || !strings.Contains(resolved.Text, "Fit Partition") {
		t.Fatalf("missing partition prompts:\n%s", resolved.Text)
	}
	if got := strings.Count(resolved.Text, "\n\n---\n\n"); got != 2 {
		t.Fatalf("separator count = %d", got)
	}
	if len(resolved.Sources) != 3 || resolved.Sources[0].Path != "prompts/code-reviewer.md" {
		t.Fatalf("sources = %+v", resolved.Sources)
	}
}

func TestResolveAbsolutePathAndSystemText(t *testing.T) {
	path := filepath.Join(t.TempDir(), "custom.md")
	if err := os.WriteFile(path, []byte("---\nmodel: opus\n---\n\ncustom prompt\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	resolved, err := Resolve([]string{path}, "extra system", "", "")
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if resolved.Tier != "deep" {
		t.Fatalf("tier = %q", resolved.Tier)
	}
	if resolved.Text != "custom prompt\n\n---\n\nextra system" {
		t.Fatalf("text = %q", resolved.Text)
	}
}

func TestResolveRejectsRelativeAndTraversalSpecs(t *testing.T) {
	for _, spec := range []string{"prompts/code-reviewer", "../secret", "dir\\prompt"} {
		if _, err := Resolve([]string{spec}, "", "", ""); err == nil {
			t.Fatalf("expected error for %q", spec)
		}
	}
}

func TestResolveExplicitTierAndModelWin(t *testing.T) {
	resolved, err := Resolve([]string{"code-reviewer"}, "", "deep", "gpt-test")
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if resolved.Tier != "deep" || resolved.Model != "gpt-test" {
		t.Fatalf("tier/model = %q/%q", resolved.Tier, resolved.Model)
	}
}

func TestResolveSkeletonWriterPrompt(t *testing.T) {
	resolved, err := Resolve([]string{"skeleton-writer"}, "", "", "")
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if resolved.Tier != "deep" {
		t.Fatalf("tier = %q", resolved.Tier)
	}
	if strings.Contains(resolved.Text, "model: deep") {
		t.Fatalf("frontmatter was not stripped:\n%s", resolved.Text)
	}
	if !strings.Contains(resolved.Text, "You are the skeleton-writer delegate") {
		t.Fatalf("missing skeleton prompt:\n%s", resolved.Text)
	}
}

func TestResolveWriteCodePromptSet(t *testing.T) {
	cases := []struct {
		stem string
		tier string
		want string
	}{
		{"implementer", "core", "You are a code implementer."},
		{"project-survey", "light", "You are project-survey"},
		{"plan-populator-survey", "core", "You are conducting a codebase survey"},
		{"plan-populator-research", "deep", "You are drafting a step-by-step implementation plan"},
		{"code-review-test", "", "Test Partition"},
		{"delegate-orientation", "", "You are a delegated worker"},
		{"impl-playbook", "", "Implementation Playbook"},
	}
	for _, tc := range cases {
		t.Run(tc.stem, func(t *testing.T) {
			resolved, err := Resolve([]string{tc.stem}, "", "", "")
			if err != nil {
				t.Fatalf("Resolve returned error: %v", err)
			}
			if tc.tier != "" && resolved.Tier != tc.tier {
				t.Fatalf("tier = %q", resolved.Tier)
			}
			if strings.Contains(resolved.Text, "tools:") || strings.Contains(resolved.Text, "model:") {
				t.Fatalf("frontmatter was not stripped:\n%s", resolved.Text)
			}
			if !strings.Contains(resolved.Text, tc.want) {
				t.Fatalf("missing %q in prompt:\n%s", tc.want, resolved.Text)
			}
		})
	}
}

func TestResolveWriteCodeImplementerPolicyChain(t *testing.T) {
	resolved, err := Resolve([]string{"implementer", "impl-playbook"}, "", "", "")
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if !strings.Contains(resolved.Text, "You are a code implementer.") {
		t.Fatalf("missing implementer prompt:\n%s", resolved.Text)
	}
	if !strings.Contains(resolved.Text, "Implementation Playbook") {
		t.Fatalf("missing implementation playbook:\n%s", resolved.Text)
	}
	if got := strings.Count(resolved.Text, "\n\n---\n\n"); got != 1 {
		t.Fatalf("separator count = %d", got)
	}
}

func TestBundleMetadata(t *testing.T) {
	info, err := Bundle("dev")
	if err != nil {
		t.Fatalf("Bundle returned error: %v", err)
	}
	if info.SourceCommit != "dev" || len(info.ContentSHA256) != 64 {
		t.Fatalf("bundle info = %+v", info)
	}
	for _, prompt := range []string{
		"code-reviewer",
		"implementer",
		"plan-populator-research",
		"plan-populator-survey",
		"project-survey",
		"skeleton-writer",
		"code-review-correctness",
		"code-review-fit",
		"code-review-test",
		"delegate-orientation",
		"impl-playbook",
	} {
		found := false
		for _, item := range info.Prompts {
			if item == prompt {
				found = true
			}
		}
		if !found {
			t.Fatalf("missing prompt %q in %+v", prompt, info.Prompts)
		}
	}
}
