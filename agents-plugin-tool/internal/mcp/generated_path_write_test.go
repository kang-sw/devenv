package mcp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPathWriteRequiresSameSessionGeneratedCachePath(t *testing.T) {
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
	root := t.TempDir()
	initGit(t, root)
	s := NewServer(root, "test")
	parent, err := s.sessions.mint(root, roleLead, "")
	if err != nil {
		t.Fatal(err)
	}
	child, err := s.sessions.mint(root, roleDelegate, parent)
	if err != nil {
		t.Fatal(err)
	}
	other, err := s.sessions.mint(root, roleDelegate, parent)
	if err != nil {
		t.Fatal(err)
	}

	generate := func(id int, key, kind string) string {
		t.Helper()
		resp := callToolOnce(t, s, id, "path.generate", map[string]any{
			"session_key": key, "kind": kind, "stems": []any{"findings"},
		})
		if toolIsError(t, resp) {
			t.Fatalf("path.generate(%s): %s", kind, resp)
		}
		return strings.TrimSpace(toolText(t, resp))
	}
	write := func(id int, key, path, content string) string {
		t.Helper()
		return callToolOnce(t, s, id, "path.write", map[string]any{
			"session_key": key, "path": path, "content": content,
		})
	}

	childPath := generate(1, child, "review")
	if resp := write(2, child, childPath, "complete findings\n"); toolIsError(t, resp) {
		t.Fatalf("same-session path.write: %s", resp)
	}
	if got, err := os.ReadFile(childPath); err != nil || string(got) != "complete findings\n" {
		t.Fatalf("written findings = %q, %v", got, err)
	}

	// Proxy reconnects construct a fresh Server in the same daemon process.
	// The same session key must retain its minted cache-path capability.
	reconnected := NewServer(root, "test")
	if resp := callToolOnce(t, reconnected, 22, "path.write", map[string]any{
		"session_key": child, "path": childPath, "content": "after reconnect\n",
	}); toolIsError(t, resp) {
		t.Fatalf("same-session path.write after reconnect: %s", resp)
	}
	if got, err := os.ReadFile(childPath); err != nil || string(got) != "after reconnect\n" {
		t.Fatalf("reconnected findings = %q, %v", got, err)
	}

	if resp := callToolOnce(t, s, 23, "path.write", map[string]any{
		"session_key": child, "path": childPath, "content": 7,
	}); !toolIsError(t, resp) || !strings.Contains(resp, "path and content must be strings") {
		t.Fatalf("non-string content refusal = %s", resp)
	}
	if got, err := os.ReadFile(childPath); err != nil || string(got) != "after reconnect\n" {
		t.Fatalf("non-string content changed findings = %q, %v", got, err)
	}

	for name, tc := range map[string]struct {
		key  string
		path string
	}{
		"unminted":        {child, filepath.Join(filepath.Dir(childPath), "unminted.md")},
		"cross-session":   {other, childPath},
		"parent-minted":   {child, generate(3, parent, "review")},
		"repository-path": {child, filepath.Join(root, "README.md")},
		"traversal-alias": {child, filepath.Dir(childPath) + string(os.PathSeparator) + ".." + string(os.PathSeparator) + filepath.Base(filepath.Dir(childPath)) + string(os.PathSeparator) + filepath.Base(childPath)},
	} {
		t.Run(name, func(t *testing.T) {
			before, _ := os.ReadFile(tc.path)
			resp := write(10, tc.key, tc.path, "must not land")
			if !toolIsError(t, resp) || !strings.Contains(resp, "not generated for this session key") {
				t.Fatalf("refusal = %s", resp)
			}
			after, _ := os.ReadFile(tc.path)
			if string(after) != string(before) {
				t.Fatalf("refused write changed %s", tc.path)
			}
		})
	}

	planPath := generate(20, child, "plan")
	if resp := write(21, child, planPath, "source tree write"); !toolIsError(t, resp) {
		t.Fatalf("repo-local generated plan was writable: %s", resp)
	}
}

func TestPathWriteRefusesSymlinkReplacementAndAcceptsOversizedContent(t *testing.T) {
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
	root := t.TempDir()
	initGit(t, root)
	s := NewServer(root, "test")
	key, err := s.sessions.mint(root, roleDelegate, "")
	if err != nil {
		t.Fatal(err)
	}
	generate := func(id int) string {
		resp := callToolOnce(t, s, id, "path.generate", map[string]any{
			"session_key": key, "kind": "review", "stems": []any{"oversized"},
		})
		if toolIsError(t, resp) {
			t.Fatal(resp)
		}
		return strings.TrimSpace(toolText(t, resp))
	}

	path := generate(1)
	large := strings.Repeat("finding\n", (17<<20)/len("finding\n"))
	if resp := callToolOnce(t, s, 2, "path.write", map[string]any{"session_key": key, "path": path, "content": large}); toolIsError(t, resp) {
		t.Fatalf("oversized path.write: %s", resp)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != large {
		t.Fatalf("oversized artifact bytes = %d, err = %v", len(got), err)
	}

	symlinkPath := generate(3)
	victim := filepath.Join(t.TempDir(), "victim.md")
	if err := os.WriteFile(victim, []byte("safe"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(symlinkPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(victim, symlinkPath); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	resp := callToolOnce(t, s, 4, "path.write", map[string]any{"session_key": key, "path": symlinkPath, "content": "escaped"})
	if !toolIsError(t, resp) || !strings.Contains(resp, "not a regular file") {
		t.Fatalf("symlink refusal = %s", resp)
	}
	if got, err := os.ReadFile(victim); err != nil || string(got) != "safe" {
		t.Fatalf("symlink victim = %q, %v", got, err)
	}

	parentSwapPath := generate(5)
	originalParent := filepath.Dir(parentSwapPath)
	movedParent := originalParent + "-moved"
	victimDir := t.TempDir()
	victimPath := filepath.Join(victimDir, filepath.Base(parentSwapPath))
	if err := os.WriteFile(victimPath, []byte("victim-safe"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(originalParent, movedParent); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(victimDir, originalParent); err != nil {
		t.Skipf("directory symlink unavailable: %v", err)
	}
	resp = callToolOnce(t, s, 6, "path.write", map[string]any{"session_key": key, "path": parentSwapPath, "content": "escaped"})
	if !toolIsError(t, resp) || !strings.Contains(resp, "not a regular file") {
		t.Fatalf("parent symlink refusal = %s", resp)
	}
	if got, err := os.ReadFile(victimPath); err != nil || string(got) != "victim-safe" {
		t.Fatalf("parent symlink victim = %q, %v", got, err)
	}
}
