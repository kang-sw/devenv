package mcp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAgentKeyStoreMintAndEnsure(t *testing.T) {
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	root := t.TempDir()
	keys := NewAgentKeyStore()
	key, err := keys.Mint(root)
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}
	store := newSessionStore()
	entry, ok := store.lookup(key)
	if !ok || entry.scope != roleDelegate || entry.parent != "" || entry.root != root {
		t.Fatalf("an agent key is a parentless delegate key at its root: %+v %v", entry, ok)
	}

	dir, err := store.keysDir()
	if err != nil {
		t.Fatal(err)
	}
	path := store.keyPath(dir, key)
	old := time.Now().Add(-20 * 24 * time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	if err := keys.Ensure(root, key); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if info, _ := os.Stat(path); !info.ModTime().After(old.Add(time.Hour)) {
		t.Fatalf("Ensure should refresh the record against age pruning")
	}

	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := keys.Ensure(root, key); err != nil {
		t.Fatalf("Ensure after prune: %v", err)
	}
	if entry, ok := store.lookup(key); !ok || entry.scope != roleDelegate || entry.root != root {
		t.Fatalf("a pruned key is written again with the same scope and root: %+v %v", entry, ok)
	}

	if err := keys.Ensure(t.TempDir(), key); err == nil || !strings.Contains(err.Error(), "not this conversation's") {
		t.Fatalf("a key rooted elsewhere is refused, got %v", err)
	}
	lead, err := store.mint(root, roleLead, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := keys.Ensure(root, lead); err == nil {
		t.Fatalf("Ensure must never adopt a lead key")
	}
	if err := keys.Ensure(root, "../escape"); err == nil {
		t.Fatalf("a malformed key is refused")
	}
}

func TestAgentKeyIsRefusedLeadOnlyAndSessionTools(t *testing.T) {
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	server := NewServer(root, "test")
	key, err := NewAgentKeyStore().Mint(root)
	if err != nil {
		t.Fatal(err)
	}
	for i, tool := range []string{"config.tune", "session.children"} {
		if got := callToolOnce(t, server, 10+i, tool, map[string]any{"session_key": key}); !strings.Contains(got, "tool not available") {
			t.Fatalf("%s should be refused for an agent key: %s", tool, got)
		}
	}
}

func TestNoDashboardModeTurnsTheDashboardOff(t *testing.T) {
	dir := dashboardSessionEnv(t)
	root := dashboardGitRoot(t)
	s, key, _ := dashboardSessionServer(t, root, true)
	if !s.DashboardSettings().Enabled {
		t.Fatal("precondition: the ws dashboard manifest enables the dashboard")
	}
	t.Setenv(envNoDashboard, "1")
	if got := s.DashboardSettings(); got.Enabled {
		t.Fatalf("NoDashboardMode should report the dashboard off (no daemon ensure): %+v", got)
	}
	dashCall(t, s, "todo.list", map[string]any{"session_key": key})
	if _, ok := readRegistration(t, dir, key); ok {
		t.Fatal("NoDashboardMode must not register a session")
	}
}

func TestAllowedToolsEnvLimitsListAndCall(t *testing.T) {
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_MCP_ALLOWED_TOOLS", "runtime.read,tickets.query")
	server := NewServer(root, "test")

	list := callToolsList(t, server)
	for _, want := range []string{`"runtime.read"`, `"tickets.query"`} {
		if !strings.Contains(list, want) {
			t.Fatalf("tools/list should advertise %s: %s", want, list)
		}
	}
	for _, hidden := range []string{`"config.tune"`, `"git.commit"`, `"note.write"`} {
		if strings.Contains(list, hidden) {
			t.Fatalf("tools/list should hide %s outside the allowlist", hidden)
		}
	}
	key, err := NewAgentKeyStore().Mint(root)
	if err != nil {
		t.Fatal(err)
	}
	if got := callToolOnce(t, server, 2, "runtime.read", map[string]any{"session_key": key}); !strings.Contains(got, "version:") {
		t.Fatalf("an allowlisted tool runs: %s", got)
	}
	if got := callToolOnce(t, server, 3, "git.status", map[string]any{"session_key": key}); !strings.Contains(got, "tool not available") {
		t.Fatalf("a tool outside the allowlist is refused at tools/call: %s", got)
	}
}
