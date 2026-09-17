package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

// TestConfigListSurfacesRepoScope verifies config.list resolves and reports a
// key held only in the committed repo file (<root>/.ws-workflow/config.json),
// tagging it with the "repo" scope in the resolved-overrides view. This is the
// end-to-end path the assignee-flag consumer relies on: a session-bound caller
// anchors the repo scope at its worktree root and reads the committed value
// deterministically.
func TestConfigListSurfacesRepoScope(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	// Committed repo-scope config with a project-wide flag.
	mustWrite(t, root, ".ws-workflow/config.json",
		`{"schema_version":1,"overrides":{"ticket-assignee-aware":"on"}}`+"\n")

	input := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"config.list","arguments":{"format":"json"}}}` + "\n"
	var out bytes.Buffer
	if err := serveStdioWithSession(t, NewServer(root, "test"), root, input, &out); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
	byID := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
	showJSON := toolText(t, byID["1"])

	var view struct {
		ResolvedOverrides []struct {
			Key   string `json:"key"`
			Value string `json:"value"`
			Scope string `json:"scope"`
		} `json:"resolved_overrides"`
	}
	if err := json.Unmarshal([]byte(showJSON), &view); err != nil {
		t.Fatalf("config.list json response is not JSON: %v\n%s", err, showJSON)
	}

	var found bool
	for _, item := range view.ResolvedOverrides {
		if item.Key == "ticket-assignee-aware" {
			found = true
			if item.Value != "on" || item.Scope != "repo" {
				t.Fatalf("repo-scope key mismatch: got value=%q scope=%q, want on/repo", item.Value, item.Scope)
			}
		}
	}
	if !found {
		t.Fatalf("config.list did not surface committed repo-scope key: %s", showJSON)
	}
}

// TestConfigListKeylessSkipsRepoScope verifies a keyless config.list caller (the
// former config.show contract) resolves without a repo anchor and does not error
// even though no session root is available to locate a committed file.
func TestConfigListKeylessSkipsRepoScope(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	// A committed file exists, but a keyless caller has no anchor to reach it.
	mustWrite(t, root, ".ws-workflow/config.json",
		`{"schema_version":1,"overrides":{"ticket-assignee-aware":"on"}}`+"\n")

	input := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"config.list","arguments":{"format":"json"}}}` + "\n"
	var out bytes.Buffer
	if err := NewServer(root, "test").ServeStdio(context.Background(), strings.NewReader(input), &out); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
	byID := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
	showJSON := toolText(t, byID["1"])

	var view struct {
		ResolvedOverrides []struct {
			Key   string `json:"key"`
			Scope string `json:"scope"`
		} `json:"resolved_overrides"`
	}
	if err := json.Unmarshal([]byte(showJSON), &view); err != nil {
		t.Fatalf("config.list json response is not JSON: %v\n%s", err, showJSON)
	}
	for _, item := range view.ResolvedOverrides {
		if item.Key == "ticket-assignee-aware" && item.Scope == "repo" {
			t.Fatalf("keyless config.list resolved a repo-scope value with no anchor: %s", showJSON)
		}
	}
}
