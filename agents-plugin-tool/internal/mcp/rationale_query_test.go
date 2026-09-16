package mcp

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestRationaleQueryListedAndSessionKeyed is Phase 1 acceptance test 12: the
// tool is advertised in tools/list and requires a session_key like git.log.
func TestRationaleQueryListedAndSessionKeyed(t *testing.T) {
	if !toolSchemaRequiresSessionKey("rationale.query") {
		t.Fatal("toolSchemaRequiresSessionKey(\"rationale.query\") = false, want true")
	}

	root := t.TempDir()
	mustWrite(t, root, "README.md", "# Test\n")
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	server := NewServer(root, "test")
	listResp := callToolsList(t, server)
	if !toolNameListed(t, listResp, "rationale.query") {
		t.Fatalf("tools/list missing rationale.query: %s", listResp)
	}
	props := toolPropertiesByName(t, listResp, "rationale.query")
	if _, ok := props["session_key"]; !ok {
		t.Fatalf("rationale.query schema missing session_key property: %v", props)
	}
	if _, ok := props["site"]; !ok {
		t.Fatalf("rationale.query schema missing site property: %v", props)
	}
}

// TestRationaleQueryDispatchMapsArguments exercises the tools/call dispatch and
// the JSON-argument-to-Options mapping in server.go (which wsrationale's own
// tests bypass), so a typo'd or dropped argument key would be caught.
func TestRationaleQueryDispatchMapsArguments(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	mustWrite(t, root, "svc/handler.go", "package svc\n")
	runGit(t, root, "add", "-A")
	runGit(t, root, "commit", "--cleanup=verbatim", "-m",
		"seed\n\n## AI Context\n- wired the widget for 260701-feat-widget\n")

	server := NewServer(root, "test")
	key, err := server.sessions.mint(root, roleLead, "")
	if err != nil {
		t.Fatalf("mint key: %v", err)
	}

	// query + format=json map through the dispatch.
	jsonResp := toolText(t, callToolOnce(t, server, 2, "rationale.query", map[string]any{
		"session_key": key, "query": "widget", "format": "json",
	}))
	if !strings.Contains(jsonResp, "wired the widget") || !strings.Contains(jsonResp, "scanned_commits") {
		t.Fatalf("query/format args did not map through dispatch: %s", jsonResp)
	}

	// paths (array) maps: a non-matching glob excludes the record, a matching one keeps it.
	miss := toolText(t, callToolOnce(t, server, 3, "rationale.query", map[string]any{
		"session_key": key, "paths": []any{"no/such/dir"},
	}))
	if strings.Contains(miss, "wired the widget") {
		t.Fatalf("paths arg did not map (non-matching glob still returned the record): %s", miss)
	}
	hit := toolText(t, callToolOnce(t, server, 4, "rationale.query", map[string]any{
		"session_key": key, "paths": []any{"svc"},
	}))
	if !strings.Contains(hit, "wired the widget") {
		t.Fatalf("paths arg did not map (matching glob missed the record): %s", hit)
	}

	// exclude_stem (string) maps through.
	excl := toolText(t, callToolOnce(t, server, 5, "rationale.query", map[string]any{
		"session_key": key, "exclude_stem": "260701-feat-widget",
	}))
	if strings.Contains(excl, "wired the widget") {
		t.Fatalf("exclude_stem arg did not map (record not dropped): %s", excl)
	}
}
