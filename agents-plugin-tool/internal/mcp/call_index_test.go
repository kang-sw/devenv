package mcp

import (
	"os"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsagent"
)

func TestCallIndexMatchesConnectionDiscovery(t *testing.T) {
	s, f := hostedBackendConnection(t, wsagent.BackendAgy, descriptorRegistrations())
	defer s.Close()
	reg := descriptorRegistrations()[wsagent.BackendAgy]
	render := HostedRender{Harness: reg.ManualHarness, PlaybookCommands: reg.PlaybookCommands, ToolNameScheme: reg.ToolNameScheme, CallFallback: reg.CallFallback, DirectTools: reg.DirectTools}
	rendered, err := RenderLeadManual(os.Environ(), f.root, render, f.hosted)
	if err != nil {
		t.Fatal(err)
	}
	live := toolText(t, callToolOnce(t, s, 1, "workflow_manual", map[string]any{}))
	names := toolText(t, callToolOnce(t, s, 1, "call", map[string]any{}))
	for source, manual := range map[string]string{"daemon prompt": rendered, "connection manual": live} {
		_, index, ok := strings.Cut(manual, "## Callable ws tools\n\n")
		if !ok {
			t.Fatalf("%s has no fallback index", source)
		}
		lines, guidance, ok := strings.Cut(index, "\n\n")
		if !ok || !strings.Contains(guidance, "[direct] tools are listed directly; all others use `call(tool, arguments)`.") {
			t.Fatalf("%s lacks the fallback guidance: %s", source, index)
		}
		var indexed []string
		for _, line := range strings.Split(lines, "\n") {
			name, description, ok := strings.Cut(line, " - ")
			if !ok || description == "" {
				t.Fatalf("%s malformed index entry: %q", source, line)
			}
			direct := strings.HasSuffix(name, " [direct]")
			name = strings.TrimSuffix(name, " [direct]")
			if direct != (name == "call" || name == "todo_list" || name == "workflow_manual") {
				t.Fatalf("%s: wrong direct marker on %s", source, name)
			}
			indexed = append(indexed, name)
		}
		if got := strings.Join(indexed, "\n"); got != names {
			t.Fatalf("%s index = %s\ncall list = %s", source, got, names)
		}
		if !strings.Contains(lines, "runtime_read - Return ws-mcp runtime metadata for compatibility checks.\n") ||
			!strings.Contains(lines, "call [direct] - Call a ws tool by name when it is missing from the client's tool list.\n") ||
			strings.Contains(lines, "No tool: list names.") {
			t.Fatalf("%s must use advertised names and first sentences: %s", source, lines)
		}
	}
}

func TestCallIndexOnlyWithDescriptorFlag(t *testing.T) {
	setupWorkflowManualNoteEnv(t)
	root := t.TempDir()
	initGit(t, root)
	key, err := NewAgentKeyStore().MintLead(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, harness := range []string{"", "claude", "codex", "pi", "agy"} {
		text, err := RenderLeadManual(os.Environ(), root, HostedRender{Harness: harness}, key)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(text, "## Callable ws tools") || strings.Contains(text, "`call(tool, arguments)`") {
			t.Fatalf("harness %q without opt-in got a fallback index", harness)
		}
	}
	// The flag, not a harness-name branch, controls the index. Its contents
	// also respect the launch's read-only authority and profile allowlist.
	env := append(os.Environ(), envAllowedTools+"=git.commit,runtime.read", wsagent.EnvPermissionLevel+"=read-only")
	text, err := RenderLeadManual(env, root, HostedRender{CallFallback: true}, key)
	if err != nil {
		t.Fatal(err)
	}
	_, index, ok := strings.Cut(text, "## Callable ws tools\n\n")
	if !ok || !strings.HasPrefix(index, "call - ") || !strings.Contains(index, "\nruntime.read - ") || strings.Contains(index, "git.commit - ") {
		t.Fatalf("index must respect descriptor, permission and allowlist: %s", index)
	}
}
