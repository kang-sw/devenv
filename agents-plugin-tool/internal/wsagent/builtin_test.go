package wsagent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsconfig"
)

// TestBuiltinSupportProfilesResolve: each harness has a builtin support
// profile that resolves through any root with no config file, allows the
// tracker tools, the wiki tools, git.search, and playbook.read (and, on
// Claude, the Agent tool alone), and names the embedded instruction text,
// never a file under the root.
func TestBuiltinSupportProfilesResolve(t *testing.T) {
	opts := testConfig(t)
	root := t.TempDir()
	for name, backend := range map[string]string{SupportProfileClaude: BackendClaude, SupportProfilePi: BackendPi} {
		rp, err := ResolveProfile(opts, root, name)
		if err != nil {
			t.Fatalf("ResolveProfile(%s): %v", name, err)
		}
		p := rp.Definition
		if rp.Scope != wsconfig.ScopeBuiltin || p.Backend != backend {
			t.Fatalf("%s should resolve from the builtin floor on %s: %+v", name, backend, rp)
		}
		for _, tool := range []string{"tracker.query", "tracker.update", "tracker.show", "tracker.ui", "wiki.query", "wiki.update", "git.search", "playbook.read"} {
			if !slices.Contains(p.WSTools, tool) {
				t.Errorf("%s ws_tools %v lack %s", name, p.WSTools, tool)
			}
		}
		// Claude's support agent may spawn subagents (Agent, nothing else);
		// Pi has no subagent tool, so its profile allows no built-in tool.
		wantTools := []string(nil)
		if backend == BackendClaude {
			wantTools = []string{"Agent"}
		}
		if !slices.Equal(p.Tools, wantTools) {
			t.Errorf("%s built-in tools = %v, want %v", name, p.Tools, wantTools)
		}
		if len(p.MCPServers) != 0 || len(p.Plugins) != 0 || p.Skills || p.ListInIndex {
			t.Errorf("%s should allow no server, plugin, or skill and stay off the index: %+v", name, p)
		}
		if p.InstructionsPath(root) != "" {
			t.Errorf("%s instructions should not resolve to a file: %s", name, p.InstructionsPath(root))
		}
		text, ok := p.BuiltinInstructions()
		if !ok || !strings.Contains(text, "tracker.show") || !strings.Contains(text, "tracker.ui") || !strings.Contains(text, "git.search") || !strings.Contains(text, "wiki.update") {
			t.Errorf("%s should carry the embedded support text, got %v %q", name, ok, text)
		}
	}
	resolver := wsconfig.NewResolver(opts, nil, nil, nil)
	all, err := ResolveProfiles(&resolver)
	if err != nil || len(all) != 2 {
		t.Fatalf("ResolveProfiles with no config should list the two builtin profiles: %+v, %v", all, err)
	}
}

// TestBuiltinSupportProfileOverriddenWhole: a global profile of the same
// name replaces the builtin one with no field merge.
func TestBuiltinSupportProfileOverriddenWhole(t *testing.T) {
	opts := testConfig(t)
	writeStructured(t, filepath.Join(opts.ConfigHome, "config.json"), map[string]string{
		ProfilePrefix + SupportProfileClaude: `{"backend":"claude","model":"sonnet","ws_tools":["tracker.query"],"instructions":"builtin:tracker-support"}`,
	})
	rp, err := ResolveProfile(opts, t.TempDir(), SupportProfileClaude)
	if err != nil {
		t.Fatal(err)
	}
	if rp.Scope != wsconfig.ScopeGlobal || rp.Definition.Model != "sonnet" || !slices.Equal(rp.Definition.WSTools, []string{"tracker.query"}) {
		t.Fatalf("the global definition should replace the builtin one whole: %+v", rp)
	}
	if _, ok := rp.Definition.BuiltinInstructions(); !ok {
		t.Fatalf("a user profile may name the builtin text too")
	}
}

func TestParseProfileRefusesUnknownBuiltinInstructions(t *testing.T) {
	_, err := ParseProfile(json.RawMessage(`{"backend":"claude","instructions":"builtin:nope"}`))
	if err == nil || !strings.Contains(err.Error(), "builtin:nope") || !strings.Contains(err.Error(), "tracker-support") {
		t.Fatalf("an unknown builtin text should be refused naming the known ones, got %v", err)
	}
}

// TestSupportConversationComposesBuiltinText: a conversation of the builtin
// support profile starts with the embedded text and the session key in its
// instruction file, though its root holds no instruction file.
func TestSupportConversationComposesBuiltinText(t *testing.T) {
	h := newHarness(t, nil)
	profile := SupportProfileClaude
	if suiteBackend == BackendPi {
		profile = SupportProfilePi
	}
	st := h.create(profile)
	if _, err := h.runner.Send(st.ID, "hello"); err != nil {
		t.Fatal(err)
	}
	eventually(t, "a process start", func() bool { return h.backend.startCount() == 1 })
	spec := h.backend.start(0)
	raw, err := os.ReadFile(spec.Instructions)
	if err != nil {
		t.Fatalf("instruction file: %v", err)
	}
	text := string(raw)
	if !strings.Contains(text, "# Tracker support agent") || !strings.Contains(text, "Your ws session key is `") {
		t.Fatalf("instruction file should hold the builtin text and the key:\n%s", text)
	}
	if spec.WSMCP == nil || !slices.Contains(spec.Profile.WSTools, "tracker.show") {
		t.Fatalf("the support profile attaches ws-mcp with its ws tools: %+v", spec)
	}
}
