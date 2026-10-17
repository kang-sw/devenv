package wsrsrc

import (
	"bytes"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// TestWsflowHooksMirrorUpToDate is the drift guard for the Claude mailbox
// push mod's wsflow copy. agents-plugin/hooks/mailbox-push/ is the one source;
// agents-plugin-wsflow/hooks/mailbox-push/ must be a BYTE-IDENTICAL copy,
// because the mod reads its package identity (plugin name, root, MCP server)
// from the engine at runtime and so has nothing to vary per package. A
// per-package edit would silently fork the push loop between the two
// packages. Each package's hooks.json differs on purpose and is not compared.
func TestWsflowHooksMirrorUpToDate(t *testing.T) {
	repoRoot := filepath.Join("..", "..", "..")
	canonical := collectTreeBytes(t, filepath.Join(repoRoot, "agents-plugin", "hooks", "mailbox-push"))
	mirror := collectTreeBytes(t, filepath.Join(repoRoot, "agents-plugin-wsflow", "hooks", "mailbox-push"))
	if len(canonical) == 0 {
		t.Fatal("canonical agents-plugin/hooks/mailbox-push/ holds no files")
	}

	var diffs []string
	for rel, want := range canonical {
		got, ok := mirror[rel]
		if !ok {
			diffs = append(diffs, "missing in agents-plugin-wsflow/hooks/mailbox-push: "+rel)
			continue
		}
		if !bytes.Equal(want, got) {
			diffs = append(diffs, "byte-differs: mailbox-push/"+rel)
		}
	}
	for rel := range mirror {
		if _, ok := canonical[rel]; !ok {
			diffs = append(diffs, "extra in agents-plugin-wsflow/hooks/mailbox-push (not in canonical): "+rel)
		}
	}
	if len(diffs) > 0 {
		sort.Strings(diffs)
		t.Fatalf("the wsflow mailbox push mod copy has drifted from agents-plugin/hooks/mailbox-push/:\n%s\n\n"+
			"Resync with: rsync -a --delete agents-plugin/hooks/mailbox-push/ agents-plugin-wsflow/hooks/mailbox-push/",
			strings.Join(diffs, "\n"))
	}
}
