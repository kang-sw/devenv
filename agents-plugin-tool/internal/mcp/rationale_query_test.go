package mcp

import (
	"path/filepath"
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
