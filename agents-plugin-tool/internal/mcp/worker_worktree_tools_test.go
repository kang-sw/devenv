package mcp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsagent"
)

// Exercise registration, the profile allowlist and actual dispatch together.
// A server launched in the original checkout must use the caller key's linked
// worktree, not its own root. A caller cannot override that binding.
func TestPiWorkerToolsUseBoundWorktree(t *testing.T) {
	for _, profile := range []struct {
		name  string
		tools []string
	}{
		{wsagent.PiWorkerProfile, wsagent.PiWorkerWSTools},
		{wsagent.PiMiniLeadProfile, wsagent.PiMiniLeadWSTools},
	} {
		for _, scope := range []toolRole{roleLead, roleDelegate} {
			t.Run(profile.name+"/"+string(scope), func(t *testing.T) {
				t.Setenv(envAllowedTools, strings.Join(profile.tools, ","))
				t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
				t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
				original := t.TempDir()
				initGit(t, original)
				mustWrite(t, original, "README.md", "fixture\n")
				runGit(t, original, "add", ".")
				runGit(t, original, "commit", "-m", "fixture")
				bound := filepath.Join(t.TempDir(), "worker")
				runGit(t, original, "worktree", "add", "-b", "impl/develop/worker-fixture", bound, "HEAD")
				original = canonicalRootForTest(t, original)
				bound = canonicalRootForTest(t, bound)

				path := "ai-docs/tickets/todo/261116-bug-bound-worker.md"
				mustWrite(t, bound, path, "---\ntitle: Bound worker fixture\n---\n\nBody.\n")
				// Same path, deliberately invalid in the original checkout.
				mustWrite(t, original, path, "---\ntitle: Wrong checkout\n")
				server := NewServer(original, "test")
				parent, err := server.sessions.mint(original, roleLead, "")
				if err != nil {
					t.Fatal(err)
				}
				key, err := server.sessions.mint(bound, scope, parent)
				if err != nil {
					t.Fatal(err)
				}

				list := callToolsList(t, server)
				for _, tool := range []string{"git.resolve_impl_branch", "tickets.create_empty", "tickets.verify"} {
					if entry := toolEntryTextByName(t, list, tool); entry == "" {
						t.Fatalf("%s not registered in %s", tool, profile.name)
					}
				}
				verify := func(key string) string {
					return callToolWithKey(t, server, 1, key, "tickets.verify", map[string]any{
						"paths": []any{path},
					})
				}
				if got := verify(key); !strings.Contains(got, "verify: PASS") {
					t.Fatalf("bound-key verification: %s", got)
				}
				if got := verify(parent); !strings.Contains(got, "verify: FAIL") {
					t.Fatalf("original key must still inspect the original checkout: %s", got)
				}
				got := callToolWithKey(t, server, 7, key, "tickets.verify", map[string]any{
					"paths": []any{path}, "root": original,
				})
				if !strings.Contains(got, "unknown arguments: root") {
					t.Fatalf("explicit root override was not rejected: %s", got)
				}
				if got := verify("unknown-worker-key"); strings.Contains(got, "verify: PASS") || !strings.Contains(got, "unknown_session") {
					t.Fatalf("unknown key must not fall back to any checkout: %s", got)
				}

				got = callToolWithKey(t, server, 2, key, "git.resolve_impl_branch", map[string]any{
					"ticket_stem": "261116-bug-bound-worker", "base": "develop",
				})
				if !strings.Contains(got, "branch: impl/develop/") || !strings.Contains(got, "merge_root: develop") {
					t.Fatalf("worker branch resolution: %s", got)
				}
				got = callToolWithKey(t, server, 3, key, "tickets.create_empty", map[string]any{
					"stem": "bug-worker-native-create", "initial_state": "todo",
				})
				if !strings.Contains(got, "Created ai-docs/tickets/todo/") {
					t.Fatalf("worker native creation: %s", got)
				}
				created, err := filepath.Glob(filepath.Join(bound, "ai-docs/tickets/todo/*-bug-worker-native-create.md"))
				if err != nil || len(created) != 1 {
					t.Fatalf("creation in bound worktree: %v, %v", created, err)
				}
				if matches, err := filepath.Glob(filepath.Join(original, "ai-docs/tickets/todo/*-bug-worker-native-create.md")); err != nil || len(matches) != 0 {
					t.Fatalf("creation escaped to original checkout: %v, %v", matches, err)
				}
				if profile.name == wsagent.PiMiniLeadProfile {
					got = callToolWithKey(t, server, 4, key, "tickets.query", map[string]any{
						"ticket_stem": "261116-bug-bound-worker",
					})
					if !strings.Contains(got, "Bound worker fixture") || strings.Contains(got, "Wrong checkout") {
						t.Fatalf("ticket query used wrong root: %s", got)
					}
				}
				// No routine profile addition should expose unrelated lead tools.
				got = callToolOnce(t, server, 5, "git.merge", map[string]any{"session_key": key})
				if !strings.Contains(got, "tool not available") {
					t.Fatalf("worker gained git.merge: %s", got)
				}
				if scope == roleDelegate {
					// Even an allowlist cannot confer lead-scoped lease authority.
					t.Setenv(envAllowedTools, strings.Join(append(append([]string{}, profile.tools...), "worktree.acquire"), ","))
					got = callToolOnce(t, server, 6, "worktree.acquire", map[string]any{"session_key": key})
					if !strings.Contains(got, "tool not available") {
						t.Fatalf("allowlist bypassed delegate scope: %s", got)
					}
				}
				if body, err := os.ReadFile(filepath.Join(original, path)); err != nil || string(body) != "---\ntitle: Wrong checkout\n" {
					t.Fatalf("original fixture changed: %q, %v", body, err)
				}
			})
		}
	}
}
