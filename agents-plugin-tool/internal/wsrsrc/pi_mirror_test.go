package wsrsrc

import (
	"bytes"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// piRoot is the agents-plugin-pi package root relative to this package dir
// (agents-plugin-tool/internal/wsrsrc → repo root → agents-plugin-pi).
func piRoot() string {
	return filepath.Join("..", "..", "..", "agents-plugin-pi")
}

// TestPiMirrorUpToDate is the drift guard for the hand-synced agents-plugin-pi/
// mirrors of agents-plugin/'s runtime.json, bin/ws-mcp-launcher.py, and rsrc/.
// These three must be BYTE-IDENTICAL copies: the pi launcher and its startup
// version pin key on runtime.json.plugin_version, and rsrc/ is the same shipped
// playbook tree pi re-registers under sanitized tool names. bump-ws-version.sh
// resyncs all three from agents-plugin/ on every version bump; this guard
// catches drift if that resync is ever skipped or a mirror is hand-edited —
// the exact silent-divergence failure class that already shipped once
// undetected (see the CI "Validate plugin release contract" step and its
// byte-identity checks for these same three paths).
func TestPiMirrorUpToDate(t *testing.T) {
	var diffs []string

	assertFileMatch := func(label, canonicalPath, mirrorPath string) {
		want, err := os.ReadFile(canonicalPath)
		if err != nil {
			t.Fatalf("read canonical %s: %v", canonicalPath, err)
		}
		got, err := os.ReadFile(mirrorPath)
		if err != nil {
			diffs = append(diffs, "missing in agents-plugin-pi: "+label)
			return
		}
		if !bytes.Equal(want, got) {
			diffs = append(diffs, "byte-differs: "+label)
		}
	}

	agentsPluginRoot := filepath.Join("..", "..", "..", "agents-plugin")
	assertFileMatch(
		"runtime.json",
		filepath.Join(agentsPluginRoot, "runtime.json"),
		filepath.Join(piRoot(), "runtime.json"),
	)
	assertFileMatch(
		"bin/ws-mcp-launcher.py",
		filepath.Join(agentsPluginRoot, "bin", "ws-mcp-launcher.py"),
		filepath.Join(piRoot(), "bin", "ws-mcp-launcher.py"),
	)

	canonical := collectTreeBytes(t, shippedRsrcRoot())
	mirror := collectTreeBytes(t, filepath.Join(piRoot(), "rsrc"))
	for rel, want := range canonical {
		got, ok := mirror[rel]
		if !ok {
			diffs = append(diffs, "missing in agents-plugin-pi/rsrc: "+rel)
			continue
		}
		if !bytes.Equal(want, got) {
			diffs = append(diffs, "byte-differs: rsrc/"+rel)
		}
	}
	for rel := range mirror {
		if _, ok := canonical[rel]; !ok {
			diffs = append(diffs, "extra in agents-plugin-pi/rsrc (not in canonical): "+rel)
		}
	}

	if len(diffs) > 0 {
		sort.Strings(diffs)
		t.Fatalf("agents-plugin-pi mirrors have drifted from canonical agents-plugin/:\n%s\n\n"+
			"Resync with: agents-plugin-tool/scripts/bump-ws-version.sh <current-version>",
			strings.Join(diffs, "\n"))
	}
}
