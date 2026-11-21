package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/mcpframe"
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
	reconnectPath := generate(21, child, "review")
	reconnected := NewServer(root, "test")
	if resp := callToolOnce(t, reconnected, 22, "path.write", map[string]any{
		"session_key": child, "path": reconnectPath, "content": "after reconnect\n",
	}); toolIsError(t, resp) {
		t.Fatalf("same-session path.write after reconnect: %s", resp)
	}
	if got, err := os.ReadFile(reconnectPath); err != nil || string(got) != "after reconnect\n" {
		t.Fatalf("reconnected findings = %q, %v", got, err)
	}

	if resp := callToolOnce(t, s, 23, "path.write", map[string]any{
		"session_key": child, "path": reconnectPath, "content": 7,
	}); !toolIsError(t, resp) || !strings.Contains(resp, "path and content must be strings") {
		t.Fatalf("non-string content refusal = %s", resp)
	}
	if got, err := os.ReadFile(reconnectPath); err != nil || string(got) != "after reconnect\n" {
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

func TestPathWriteGrantsAreBoundedAndConsumed(t *testing.T) {
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
	root := t.TempDir()
	initGit(t, root)
	s := NewServer(root, "test")
	key, err := s.sessions.mint(root, roleDelegate, "")
	if err != nil {
		t.Fatal(err)
	}

	paths := make([]string, 0, maxGeneratedPathGrants+1)
	for i := 0; i <= maxGeneratedPathGrants; i++ {
		resp := callToolOnce(t, s, i+1, "path.generate", map[string]any{
			"session_key": key, "kind": "review", "stems": []any{"bounded"},
		})
		if toolIsError(t, resp) {
			t.Fatalf("path.generate(%d): %s", i, resp)
		}
		paths = append(paths, strings.TrimSpace(toolText(t, resp)))
	}
	processGeneratedWrites.mu.RLock()
	count := processGeneratedWrites.grantCountLocked()
	processGeneratedWrites.mu.RUnlock()
	if count > maxGeneratedPathGrants {
		t.Fatalf("open generated-path grants = %d, want <= %d", count, maxGeneratedPathGrants)
	}
	if resp := callToolOnce(t, s, 100, "path.write", map[string]any{
		"session_key": key, "path": paths[0], "content": "evicted",
	}); !toolIsError(t, resp) {
		t.Fatalf("oldest grant remained writable after capacity eviction: %s", resp)
	}
	last := paths[len(paths)-1]
	if resp := callToolOnce(t, s, 101, "path.write", map[string]any{
		"session_key": key, "path": last, "content": "complete",
	}); toolIsError(t, resp) {
		t.Fatalf("newest grant write: %s", resp)
	}
	if resp := callToolOnce(t, s, 102, "path.write", map[string]any{
		"session_key": key, "path": last, "content": "second",
	}); !toolIsError(t, resp) {
		t.Fatalf("completed grant was not consumed: %s", resp)
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

func TestServeStdioRejectsPathWriteBeyondRequestLimit(t *testing.T) {
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
	root := t.TempDir()
	initGit(t, root)
	s := NewServer(root, "test")
	key, err := s.sessions.mint(root, roleDelegate, "")
	if err != nil {
		t.Fatal(err)
	}
	generated := callToolOnce(t, s, 1, "path.generate", map[string]any{
		"session_key": key, "kind": "review", "stems": []any{"limit"},
	})
	path := strings.TrimSpace(toolText(t, generated))
	oversize := `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"path.write","arguments":{"session_key":` +
		fmt.Sprintf("%q", key) + `,"path":` + fmt.Sprintf("%q", path) + `,"content":"` + strings.Repeat("x", mcpframe.MaxRequestBytes) + `"}}}`
	followUp := `{"jsonrpc":"2.0","id":3,"method":"ping"}`
	var out bytes.Buffer
	err = s.ServeStdio(context.Background(), strings.NewReader(oversize+"\n"+followUp+"\n"), &out)
	if err != nil {
		t.Fatalf("ServeStdio oversized request error = %v, output = %.200q", err, out.String())
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("ServeStdio produced %d responses, want 2: %q", len(lines), out.String())
	}
	var errResp struct {
		ID    json.RawMessage `json:"id"`
		Error *rpcError       `json:"error"`
	}
	if err := json.Unmarshal([]byte(lines[0]), &errResp); err != nil {
		t.Fatalf("decode oversize response: %v (%q)", err, lines[0])
	}
	if string(errResp.ID) != "2" || errResp.Error == nil || errResp.Error.Code != -32600 || errResp.Error.Message != mcpframe.TooLargeMessage(mcpframe.MaxRequestBytes) {
		t.Fatalf("oversize response = %s, want id=2 -32600 %q", lines[0], mcpframe.TooLargeMessage(mcpframe.MaxRequestBytes))
	}
	var pingResp struct {
		ID     json.RawMessage `json:"id"`
		Result map[string]any  `json:"result"`
		Error  *rpcError       `json:"error"`
	}
	if err := json.Unmarshal([]byte(lines[1]), &pingResp); err != nil {
		t.Fatalf("decode follow-up ping response: %v (%q)", err, lines[1])
	}
	if string(pingResp.ID) != "3" || pingResp.Error != nil || pingResp.Result == nil {
		t.Fatalf("follow-up response = %s, want id=3 ok result", lines[1])
	}
	if got, readErr := os.ReadFile(path); readErr != nil || len(got) != 0 {
		t.Fatalf("rejected oversized request changed artifact: bytes=%d err=%v", len(got), readErr)
	}
}
