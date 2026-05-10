package wsprompt

import (
	"encoding/json"
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
	if strings.Contains(resolved.Text, "model: core") {
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
	if !strings.Contains(resolved.Text, "skeleton-writer compatibility delegate") {
		t.Fatalf("missing skeleton prompt:\n%s", resolved.Text)
	}
	if !strings.Contains(resolved.Text, "CONTRACT:") || !strings.Contains(resolved.Text, "HINT:") || !strings.Contains(resolved.Text, "HOLE:") {
		t.Fatalf("missing marker guidance:\n%s", resolved.Text)
	}
}

func TestResolveSkeletonPopulatorPrompt(t *testing.T) {
	resolved, err := Resolve([]string{"skeleton-populator"}, "", "", "")
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if resolved.Tier != "deep" {
		t.Fatalf("tier = %q", resolved.Tier)
	}
	if strings.Contains(resolved.Text, "model: deep") {
		t.Fatalf("frontmatter was not stripped:\n%s", resolved.Text)
	}
	for _, want := range []string{
		"You are the skeleton-populator delegate",
		"Treat lead-authored source draft markers as authoritative input",
		"CONTRACT:",
		"HINT:",
		"HOLE:",
	} {
		if !strings.Contains(resolved.Text, want) {
			t.Fatalf("missing %q in prompt:\n%s", want, resolved.Text)
		}
	}
}

func TestResolveWriteCodePromptSet(t *testing.T) {
	cases := []struct {
		stem string
		tier string
		want string
	}{
		{"api-doc-manager", "core", "You are an API documentation manager"},
		{"api-doc-cargo-brief", "", "`cargo-brief` is available"},
		{"pre-router", "light", "You are an API documentation pre-router"},
		{"implementer", "core", "You are a code implementer."},
		{"mental-model-updater", "core", "You are updating mental-model documents"},
		{"project-survey", "light", "You are project-survey"},
		{"plan-populator-survey", "core", "You are conducting a codebase survey"},
		{"plan-populator-research", "deep", "You are drafting a step-by-step implementation plan"},
		{"sprint-survey", "core", "You are a sprint-context survey agent"},
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

func TestAPIDocsPromptContracts(t *testing.T) {
	preRouter, err := Resolve([]string{"pre-router"}, "", "", "")
	if err != nil {
		t.Fatalf("Resolve pre-router returned error: %v", err)
	}
	for _, want := range []string{
		"you never answer the question",
		"Return only canonical API documentation domain slugs",
		"one per non-empty line",
		"Do not include prose",
		"letters, digits, dot, underscore, or hyphen",
	} {
		if !strings.Contains(preRouter.Text, want) {
			t.Fatalf("pre-router prompt missing %q:\n%s", want, preRouter.Text)
		}
	}

	manager, err := Resolve([]string{"api-doc-manager"}, "", "", "")
	if err != nil {
		t.Fatalf("Resolve api-doc-manager returned error: %v", err)
	}
	for _, want := range []string{
		"At the start of each session",
		"scripts/check-stale",
		"Run the staleness check",
		"before answering",
		"Cite the cached file paths and/or official source URLs",
	} {
		if !strings.Contains(manager.Text, want) {
			t.Fatalf("api-doc-manager prompt missing %q:\n%s", want, manager.Text)
		}
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
	for i := 1; i < len(info.Prompts); i++ {
		if info.Prompts[i-1] >= info.Prompts[i] {
			t.Fatalf("prompts are not sorted and unique: %+v", info.Prompts)
		}
	}
	required := []string{
		"api-doc-manager",
		"pre-router",
		"code-reviewer",
		"implementer",
		"mental-model-updater",
		"plan-populator-research",
		"plan-populator-survey",
		"project-survey",
		"skeleton-populator",
		"skeleton-writer",
		"sprint-survey",
		"code-review-correctness",
		"code-review-fit",
		"code-review-test",
		"delegate-orientation",
		"impl-playbook",
	}
	for _, prompt := range required {
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

func TestEmbeddedPromptDiscoveryUsesTopLevelMarkdownOnly(t *testing.T) {
	paths, err := embeddedPromptPaths()
	if err != nil {
		t.Fatalf("embeddedPromptPaths returned error: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("no embedded prompt paths discovered")
	}
	for _, path := range paths {
		if !strings.HasSuffix(path, ".md") {
			t.Fatalf("non-markdown prompt path discovered: %s", path)
		}
		dir := filepath.ToSlash(filepath.Dir(path))
		if dir != "prompts" && dir != "infra" {
			t.Fatalf("prompt path outside top-level embed directories: %s", path)
		}
	}
}

func TestRuntimeContractPromptBundleHash(t *testing.T) {
	info, err := Bundle("dev")
	if err != nil {
		t.Fatalf("Bundle returned error: %v", err)
	}
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "agents-plugin", "runtime.json"))
	if err != nil {
		t.Fatalf("read runtime contract: %v", err)
	}
	var contract struct {
		PromptBundle struct {
			ContentSHA256 string `json:"content_sha256"`
		} `json:"prompt_bundle"`
	}
	if err := json.Unmarshal(data, &contract); err != nil {
		t.Fatalf("parse runtime contract: %v", err)
	}
	if contract.PromptBundle.ContentSHA256 != info.ContentSHA256 {
		t.Fatalf("runtime.json prompt bundle hash = %q, want %q", contract.PromptBundle.ContentSHA256, info.ContentSHA256)
	}
}

func TestNormalizePromptHashContent(t *testing.T) {
	got := string(normalizePromptHashContent([]byte("one\r\ntwo\nthree\r\n")))
	want := "one\ntwo\nthree\n"
	if got != want {
		t.Fatalf("normalized prompt hash content = %q, want %q", got, want)
	}
}
