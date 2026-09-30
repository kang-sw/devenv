package claudeagents

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

const regenHint = "Regenerate with: WS_REGEN_CLAUDE_AGENTS=1 go test ./internal/claudeagents -count=1 -run TestRegenerateClaudeAgents"

// agentDirs are the committed claude-agents/ directories, relative to this
// package dir (agents-plugin-tool/internal/claudeagents -> repo root). Both
// plugin packages ship the same five generated agents.
func agentDirs() []string {
	root := filepath.Join("..", "..", "..")
	return []string{
		filepath.Join(root, "agents-plugin", "claude-agents"),
		filepath.Join(root, "agents-plugin-wsflow", "claude-agents"),
	}
}

// renderAll returns FileName(level) -> Render(level) for every level.
func renderAll(t *testing.T) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, level := range Levels {
		text, err := Render(level)
		if err != nil {
			t.Fatalf("Render(%q): %v", level, err)
		}
		out[FileName(level)] = text
	}
	return out
}

// mdFiles lists the *.md file names directly under dir.
func mdFiles(t *testing.T, dir string) []string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, "*.md"))
	if err != nil {
		t.Fatalf("glob %s: %v", dir, err)
	}
	names := make([]string, 0, len(matches))
	for _, m := range matches {
		names = append(names, filepath.Base(m))
	}
	sort.Strings(names)
	return names
}

// frontmatter parses the leading `---`-delimited block into key -> value.
func frontmatter(t *testing.T, name, text string) map[string]string {
	t.Helper()
	lines := strings.Split(text, "\n")
	if len(lines) == 0 || lines[0] != "---" {
		t.Fatalf("%s: does not start with a frontmatter delimiter", name)
	}
	fields := map[string]string{}
	for _, line := range lines[1:] {
		if line == "---" {
			return fields
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			t.Fatalf("%s: malformed frontmatter line %q", name, line)
		}
		if _, dup := fields[key]; dup {
			t.Fatalf("%s: duplicate frontmatter key %q", name, key)
		}
		fields[key] = strings.TrimSpace(value)
	}
	t.Fatalf("%s: unterminated frontmatter", name)
	return nil
}

func TestRenderFrontmatterMatchesLevel(t *testing.T) {
	for _, level := range Levels {
		text, err := Render(level)
		if err != nil {
			t.Fatalf("Render(%q): %v", level, err)
		}
		fm := frontmatter(t, FileName(level), text)
		keys := make([]string, 0, len(fm))
		for k := range fm {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		if got, want := strings.Join(keys, ","), "description,effort,name"; got != want {
			t.Errorf("%s: frontmatter keys = %s, want exactly %s", level, got, want)
		}
		if fm["name"] != "effort-"+level {
			t.Errorf("%s: name = %q, want %q", level, fm["name"], "effort-"+level)
		}
		if fm["effort"] != level {
			t.Errorf("%s: effort = %q, want %q", level, fm["effort"], level)
		}
		if fm["description"] == "" {
			t.Errorf("%s: empty description", level)
		}
	}
}

func TestRenderRejectsUnknownLevel(t *testing.T) {
	for _, level := range []string{"", "High", "none", " high"} {
		if _, err := Render(level); err == nil {
			t.Errorf("Render(%q) returned no error", level)
		}
	}
}

// TestClaudeAgentsUpToDate is the drift guard for the committed generated
// agents in both packages: every level's file exists and equals Render output,
// and no other *.md file sits beside them.
func TestClaudeAgentsUpToDate(t *testing.T) {
	want := renderAll(t)
	var diffs []string
	for _, dir := range agentDirs() {
		for name, text := range want {
			got, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				diffs = append(diffs, "missing: "+filepath.Join(dir, name))
				continue
			}
			if string(got) != text {
				diffs = append(diffs, "differs from template output: "+filepath.Join(dir, name))
			}
		}
		for _, name := range mdFiles(t, dir) {
			if _, ok := want[name]; !ok {
				diffs = append(diffs, "extra (not generated): "+filepath.Join(dir, name))
			}
		}
	}
	if len(diffs) > 0 {
		sort.Strings(diffs)
		t.Fatalf("generated Claude effort agents have drifted from effort-agent.md.tmpl:\n%s\n\n%s",
			strings.Join(diffs, "\n"), regenHint)
	}
}

// TestRegenerateClaudeAgents rewrites the generated agents in both packages.
// It is a no-op unless WS_REGEN_CLAUDE_AGENTS=1, so an ordinary test run never
// mutates the source tree. Stale *.md files in the target dirs are removed;
// plugin.json manifests are curated by hand and never touched here.
func TestRegenerateClaudeAgents(t *testing.T) {
	if os.Getenv("WS_REGEN_CLAUDE_AGENTS") != "1" {
		t.Skip("set WS_REGEN_CLAUDE_AGENTS=1 to regenerate the Claude effort agents")
	}
	want := renderAll(t)
	for _, dir := range agentDirs() {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
		for _, name := range mdFiles(t, dir) {
			if _, ok := want[name]; ok {
				continue
			}
			if err := os.Remove(filepath.Join(dir, name)); err != nil {
				t.Fatalf("remove stale %s: %v", name, err)
			}
		}
		for name, text := range want {
			if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644); err != nil {
				t.Fatalf("write %s: %v", name, err)
			}
		}
		t.Logf("regenerated %d agents in %s", len(want), dir)
	}
}
