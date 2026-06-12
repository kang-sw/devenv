package wsrsrc

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// shippedRsrcRoot is the real shipped rsrc tree relative to this package dir
// (agents-plugin-tool/internal/wsrsrc → repo root → agents-plugin/rsrc).
func shippedRsrcRoot() string {
	return filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
}

// TestShippedManifestUpToDate guards against the "edited a shipped asset but
// forgot to regenerate manifest.json" failure class (see ticket
// 260611-bug-rsrc-manifest-regen-missed). It regenerates the manifest in-memory
// from the shipped tree and asserts it matches the committed manifest.json.
//
// To update after an intentional shipped-asset change, rerun this package with
// WS_REGEN_MANIFEST=1 (see TestRegenerateShippedManifest).
func TestShippedManifestUpToDate(t *testing.T) {
	root := shippedRsrcRoot()

	want, err := ReadManifest(root)
	if err != nil {
		t.Fatalf("ReadManifest(%s): %v", root, err)
	}
	got, err := GenerateManifest(root)
	if err != nil {
		t.Fatalf("GenerateManifest(%s): %v", root, err)
	}

	if got.SchemaVersion != want.SchemaVersion {
		t.Fatalf("schema version: generated %d, committed %d", got.SchemaVersion, want.SchemaVersion)
	}
	if !reflect.DeepEqual(got.Files, want.Files) {
		t.Fatalf("shipped manifest.json is stale: generated tree hashes differ from committed manifest.\n"+
			"Regenerate with: WS_REGEN_MANIFEST=1 go test ./internal/wsrsrc -run TestRegenerateShippedManifest\n"+
			"generated %d files, committed %d files", len(got.Files), len(want.Files))
	}
}

// TestRegenerateShippedManifest rewrites the shipped manifest.json from the
// current tree. It is a no-op unless WS_REGEN_MANIFEST=1, so an ordinary test
// run never mutates the source tree.
func TestRegenerateShippedManifest(t *testing.T) {
	if os.Getenv("WS_REGEN_MANIFEST") != "1" {
		t.Skip("set WS_REGEN_MANIFEST=1 to regenerate the shipped manifest.json")
	}
	root := shippedRsrcRoot()
	m, err := GenerateManifest(root)
	if err != nil {
		t.Fatalf("GenerateManifest(%s): %v", root, err)
	}
	if err := WriteManifest(root, m); err != nil {
		t.Fatalf("WriteManifest(%s): %v", root, err)
	}
	t.Logf("regenerated %s/manifest.json with %d files", root, len(m.Files))
}
