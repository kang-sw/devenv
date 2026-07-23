package wsrsrc

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// substitutionMirroredSkills is the curated, bounded list of skills eligible
// for substitution-mirrored generation: only skills explicitly and
// deliberately migrated out of playbook.print. This is not a blanket
// auto-mirror mechanism — see ai-docs/ref/wsflow-mirroring.md.
var substitutionMirroredSkills = []string{
	"lead-goal-step",
	"lead-prefer-subagent",
	"lead-verify-discussion",
}

// fullSkillsRoot is the canonical full-ws skills tree relative to this
// package dir (agents-plugin-tool/internal/wsrsrc -> repo root ->
// agents-plugin/skills).
func fullSkillsRoot() string {
	return filepath.Join("..", "..", "..", "agents-plugin", "skills")
}

// wsflowSkillsRoot is the generated wsflow skills copy relative to this
// package dir.
func wsflowSkillsRoot() string {
	return filepath.Join("..", "..", "..", "agents-plugin-wsflow", "skills")
}

// TestWsflowSkillsMirrorUpToDate is the drift guard for substitution-mirrored
// wsflow skills. Unlike TestWsflowRsrcMirrorUpToDate, this is a
// substitution-aware comparison, not bytes.Equal: the wsflow file is expected
// to be the ws:->wsflow:/ws/->wsflow/-substituted form of the full-ws source.
func TestWsflowSkillsMirrorUpToDate(t *testing.T) {
	var diffs []string
	for _, name := range substitutionMirroredSkills {
		srcPath := filepath.Join(fullSkillsRoot(), name, "SKILL.md")
		dstPath := filepath.Join(wsflowSkillsRoot(), name, "SKILL.md")

		src, err := os.ReadFile(srcPath)
		if err != nil {
			t.Fatalf("read source %s: %v", srcPath, err)
		}
		want, err := GenerateWsflowSkillBody(string(src))
		if err != nil {
			t.Fatalf("generate wsflow body for %s: %v", name, err)
		}
		got, err := os.ReadFile(dstPath)
		if err != nil {
			diffs = append(diffs, "missing wsflow mirror: "+name)
			continue
		}
		if string(got) != want {
			diffs = append(diffs, "byte-differs after substitution: "+name)
		}
	}

	// Synthetic namespace-token fixture: none of the curated skills above
	// currently contain ws:/ws/ tokens, so without this case the loop above
	// never actually exercises substitution — it only detects added, removed,
	// or byte-edited mirrors. This pins substitution correctness through the
	// exact same want-vs-got comparison path.
	syntheticSrc := "---\nname: synthetic-drift-fixture\n---\n\n" +
		"Use ws:lead-example and call ws/example.tool for details.\n"
	syntheticWant, err := GenerateWsflowSkillBody(syntheticSrc)
	if err != nil {
		t.Fatalf("generate wsflow body for synthetic drift fixture: %v", err)
	}
	syntheticGot := "---\nname: synthetic-drift-fixture\n---\n\n" +
		"Use wsflow:lead-example and call wsflow/example.tool for details.\n"
	if syntheticGot != syntheticWant {
		diffs = append(diffs, "synthetic namespace-token fixture: substitution mismatch")
	}

	if len(diffs) > 0 {
		sort.Strings(diffs)
		t.Fatalf("wsflow skills mirror has drifted from substitution-mirrored full-ws source:\n%s\n\n"+
			"Regenerate with: WS_REGEN_WSFLOW_SKILLS=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateWsflowSkillsMirror",
			strings.Join(diffs, "\n"))
	}
}

// TestRegenerateWsflowSkillsMirror rewrites the wsflow skill mirrors from the
// substitution-mirrored full-ws sources. It is a no-op unless
// WS_REGEN_WSFLOW_SKILLS=1, so an ordinary test run never mutates the source
// tree. Uses a distinct env var from WS_REGEN_WSFLOW_RSRC (rsrc mirror regen)
// and WSRSRC_REGEN_SKILLS (manifest regen) so a single flag never
// accidentally regenerates unrelated generation surfaces.
func TestRegenerateWsflowSkillsMirror(t *testing.T) {
	if os.Getenv("WS_REGEN_WSFLOW_SKILLS") != "1" {
		t.Skip("set WS_REGEN_WSFLOW_SKILLS=1 to regenerate the wsflow skills mirror")
	}
	for _, name := range substitutionMirroredSkills {
		srcPath := filepath.Join(fullSkillsRoot(), name, "SKILL.md")
		dstPath := filepath.Join(wsflowSkillsRoot(), name, "SKILL.md")

		src, err := os.ReadFile(srcPath)
		if err != nil {
			t.Fatalf("read source %s: %v", srcPath, err)
		}
		out, err := GenerateWsflowSkillBody(string(src))
		if err != nil {
			t.Fatalf("generate wsflow body for %s: %v", name, err)
		}
		if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
			t.Fatalf("mkdir for %s: %v", name, err)
		}
		if err := os.WriteFile(dstPath, []byte(out), 0o644); err != nil {
			t.Fatalf("write %s: %v", dstPath, err)
		}
	}
	t.Logf("regenerated %d wsflow skill mirror(s)", len(substitutionMirroredSkills))
}

// TestSubstitutionGuardRejectsDisqualifyingContent exercises the negative
// path of the eligibility guard: fixtures that must make generation fail.
func TestSubstitutionGuardRejectsDisqualifyingContent(t *testing.T) {
	cases := []struct {
		name   string
		source string
	}{
		{
			name: "mercenary word anywhere",
			source: "---\nname: fixture\n---\n\n" +
				"A ws-managed external subprocess agent (mercenary) is reachable.\n",
		},
		{
			name: "ws:full-only marker",
			source: "---\nname: fixture\n---\n\n" +
				"<!-- ws:full-only:start -->\nfull-only content\n<!-- ws:full-only:end -->\n",
		},
		{
			name: "ws:wsflow-only marker",
			source: "---\nname: fixture\n---\n\n" +
				"<!-- ws:wsflow-only:start -->\nwsflow-only content\n<!-- ws:wsflow-only:end -->\n",
		},
		{
			name: "excluded skill name literal",
			source: "---\nname: fixture\n---\n\n" +
				"See lead-write-code for implementation details.\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := GenerateWsflowSkillBody(tc.source); err == nil {
				t.Fatalf("expected guard to reject fixture %q, but generation succeeded", tc.name)
			}
		})
	}
}

// TestSubstitutionMirrorRespectsWordBoundaries proves the ws:/ws/ substitution
// only matches standalone namespace tokens, not substrings inside unrelated
// words like "shows:" or "draws/". Without a left-side word-boundary anchor, a
// blind strings.ReplaceAll would mangle "shows:" into "showsflow:" and
// "draws/" into "drawsflow/".
func TestSubstitutionMirrorRespectsWordBoundaries(t *testing.T) {
	source := "---\nname: fixture\n---\n\n" +
		"This shows: the result and draws/ conclusions alongside " +
		"ws:playbook.print and ws/enter_implement plus `ws:tickets_create`.\n"
	out, err := GenerateWsflowSkillBody(source)
	if err != nil {
		t.Fatalf("expected guard to accept fixture, got error: %v", err)
	}
	want := "---\nname: fixture\n---\n\n" +
		"This shows: the result and draws/ conclusions alongside " +
		"wsflow:playbook.print and wsflow/enter_implement plus `wsflow:tickets_create`.\n"
	if out != want {
		t.Fatalf("word-boundary substitution mismatch:\ngot:  %q\nwant: %q", out, want)
	}
}

// TestSubstitutionGuardAcceptsNamespaceOnlyContent is the positive-path
// sibling: a source containing only ws:/ws/ namespace tokens must pass the
// guard and substitute cleanly.
func TestSubstitutionGuardAcceptsNamespaceOnlyContent(t *testing.T) {
	source := "---\nname: fixture\n---\n\n" +
		"Use ws:lead-example and call ws/example.tool for details.\n"
	out, err := GenerateWsflowSkillBody(source)
	if err != nil {
		t.Fatalf("expected guard to accept namespace-only fixture, got error: %v", err)
	}
	want := "---\nname: fixture\n---\n\n" +
		"Use wsflow:lead-example and call wsflow/example.tool for details.\n"
	if out != want {
		t.Fatalf("substitution mismatch:\ngot:  %q\nwant: %q", out, want)
	}
}
