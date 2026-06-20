package wsrsrc

import (
	"bytes"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// wsflowRsrcRoot is the generated wsflow rsrc copy relative to this package dir
// (agents-plugin-tool/internal/wsrsrc → repo root → agents-plugin-wsflow/rsrc).
func wsflowRsrcRoot() string {
	return filepath.Join("..", "..", "..", "agents-plugin-wsflow", "rsrc")
}

// collectTreeBytes walks root and returns a map of slash-relative path → file
// bytes for every regular file.
func collectTreeBytes(t *testing.T, root string) map[string][]byte {
	t.Helper()
	files := map[string][]byte{}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(rel)] = data
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	return files
}

// TestWsflowRsrcMirrorUpToDate is the drift guard for the generated wsflow rsrc
// copy. The wsflow package ships an rsrc tree that must be a BYTE-IDENTICAL copy
// of canonical agents-plugin/rsrc/ (the ws/ to wsflow/ namespace difference is
// applied at render time in the playbook rendering layer, not in stored files,
// so the stored copies are identical). A drift here is exactly the failure class
// — silent divergence — that let the old embedded prompt copies rot; this guard
// makes it loud.
func TestWsflowRsrcMirrorUpToDate(t *testing.T) {
	canonical := collectTreeBytes(t, shippedRsrcRoot())
	mirror := collectTreeBytes(t, wsflowRsrcRoot())

	var diffs []string
	for rel, want := range canonical {
		got, ok := mirror[rel]
		if !ok {
			diffs = append(diffs, "missing in wsflow: "+rel)
			continue
		}
		if !bytes.Equal(want, got) {
			diffs = append(diffs, "byte-differs: "+rel)
		}
	}
	for rel := range mirror {
		if _, ok := canonical[rel]; !ok {
			diffs = append(diffs, "extra in wsflow (not in canonical): "+rel)
		}
	}

	if len(diffs) > 0 {
		sort.Strings(diffs)
		t.Fatalf("wsflow rsrc mirror has drifted from canonical agents-plugin/rsrc:\n%s\n\n"+
			"Regenerate with: WS_REGEN_WSFLOW_RSRC=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateWsflowRsrcMirror",
			strings.Join(diffs, "\n"))
	}
}

// TestRegenerateWsflowRsrcMirror rewrites the wsflow rsrc copy from canonical.
// It is a no-op unless WS_REGEN_WSFLOW_RSRC=1, so an ordinary test run never
// mutates the source tree. The wsflow copy is committed (per the provisioning
// decision: on-disk real copy, not a symlink or release-only artifact), so this
// regen is the maintenance entry point after any canonical rsrc change.
func TestRegenerateWsflowRsrcMirror(t *testing.T) {
	if os.Getenv("WS_REGEN_WSFLOW_RSRC") != "1" {
		t.Skip("set WS_REGEN_WSFLOW_RSRC=1 to regenerate the wsflow rsrc mirror")
	}
	canonical := shippedRsrcRoot()
	mirror := wsflowRsrcRoot()

	if err := os.RemoveAll(mirror); err != nil {
		t.Fatalf("clear wsflow rsrc mirror: %v", err)
	}
	src := collectTreeBytes(t, canonical)
	rels := make([]string, 0, len(src))
	for rel := range src {
		rels = append(rels, rel)
	}
	sort.Strings(rels)
	for _, rel := range rels {
		dst := filepath.Join(mirror, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			t.Fatalf("mkdir for %s: %v", rel, err)
		}
		if err := os.WriteFile(dst, src[rel], 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	t.Logf("regenerated %s from canonical with %d files", mirror, len(rels))
}
