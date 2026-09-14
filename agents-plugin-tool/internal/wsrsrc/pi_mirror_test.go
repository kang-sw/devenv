package wsrsrc

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
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

// TestRootPackageDependenciesMirrored is the drift guard for the repo-root
// package.json `dependencies` mirror of agents-plugin-pi/package.json's
// runtime dependencies. Pi's git-install flow runs `npm install` only against
// the root manifest, so the six runtime deps (yaml, jiti, linkedom, etc.) must
// be declared there too, or the extension fails to load with a
// "Cannot find module" error on a clean machine. Unlike the byte-identical
// runtime.json/bin/rsrc mirrors above, the two package.json files legitimately
// differ elsewhere — the root carries `pi.extensions` and omits the subdir's
// `type`/`files`/`scripts`/`devDependencies` — so this compares only the
// parsed `dependencies` object, not the whole file. devDependencies are
// intentionally excluded: the Pi host provides `@earendil-works/pi-*` at
// runtime.
func TestRootPackageDependenciesMirrored(t *testing.T) {
	repoRoot := filepath.Join("..", "..", "..")

	readDependencies := func(t *testing.T, path string) map[string]string {
		t.Helper()
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		var pkg struct {
			Dependencies map[string]string `json:"dependencies"`
		}
		if err := json.Unmarshal(raw, &pkg); err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		return pkg.Dependencies
	}

	root := readDependencies(t, filepath.Join(repoRoot, "package.json"))
	piBridge := readDependencies(t, filepath.Join(piRoot(), "package.json"))

	if !reflect.DeepEqual(root, piBridge) {
		t.Fatalf("root package.json dependencies have drifted from agents-plugin-pi/package.json:\nroot: %v\nagents-plugin-pi: %v\n\n"+
			"Resync with: agents-plugin-tool/scripts/bump-ws-version.sh <current-version>",
			root, piBridge)
	}
}
