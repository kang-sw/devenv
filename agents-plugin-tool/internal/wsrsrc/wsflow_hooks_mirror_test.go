package wsrsrc

import (
	"bytes"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// TestWsflowHooksMirrorUpToDate is the drift guard for the Claude mod's
// wsflow copy. agents-plugin/hooks/claude-mod/ is the one source (the mailbox
// push and lead compaction timing features under one entry);
// agents-plugin-wsflow/hooks/claude-mod/ must be a BYTE-IDENTICAL copy,
// because the mod reads its package identity (plugin name, root, MCP server)
// from the engine at runtime and so has nothing to vary per package: even the
// ws-only compaction feature is gated on that name inside the shared source.
// A per-package edit would silently fork the mod between the two packages.
// Each package's hooks.json differs on purpose and is not compared.
func TestWsflowHooksMirrorUpToDate(t *testing.T) {
	repoRoot := filepath.Join("..", "..", "..")
	canonical := collectTreeBytes(t, filepath.Join(repoRoot, "agents-plugin", "hooks", "claude-mod"))
	mirror := collectTreeBytes(t, filepath.Join(repoRoot, "agents-plugin-wsflow", "hooks", "claude-mod"))
	if len(canonical) == 0 {
		t.Fatal("canonical agents-plugin/hooks/claude-mod/ holds no files")
	}

	var diffs []string
	for rel, want := range canonical {
		got, ok := mirror[rel]
		if !ok {
			diffs = append(diffs, "missing in agents-plugin-wsflow/hooks/claude-mod: "+rel)
			continue
		}
		if !bytes.Equal(want, got) {
			diffs = append(diffs, "byte-differs: claude-mod/"+rel)
		}
	}
	for rel := range mirror {
		if _, ok := canonical[rel]; !ok {
			diffs = append(diffs, "extra in agents-plugin-wsflow/hooks/claude-mod (not in canonical): "+rel)
		}
	}
	if len(diffs) > 0 {
		sort.Strings(diffs)
		t.Fatalf("the wsflow Claude mod copy has drifted from agents-plugin/hooks/claude-mod/:\n%s\n\n"+
			"Resync with: rsync -a --delete agents-plugin/hooks/claude-mod/ agents-plugin-wsflow/hooks/claude-mod/",
			strings.Join(diffs, "\n"))
	}
}
