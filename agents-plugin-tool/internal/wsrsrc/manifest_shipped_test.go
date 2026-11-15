package wsrsrc

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
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
			"Regenerate with: WS_REGEN_MANIFEST=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateShippedManifest\n"+
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

// TestLoadPartReadsAVerifiedPartThatIsNoPlaybook pins the shipped
// command-syntax part: LoadPart reads it hash-verified, and neither Load nor
// a flat fallback resolves it as a playbook of its own.
func TestLoadPartReadsAVerifiedPartThatIsNoPlaybook(t *testing.T) {
	root := shippedRsrcRoot()
	body, err := LoadPart(root, "lead-workflow-manual", "command-syntax")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(body) == "" {
		t.Fatalf("part body = %q", body)
	}
	var notFound ErrPlaybookNotFound
	if _, err := Load(root, "command-syntax", "", nil); !errors.As(err, &notFound) {
		t.Fatalf("Load(command-syntax) = %v, want ErrPlaybookNotFound", err)
	}
	if _, err := LoadPart(root, "lead-workflow-manual", "missing"); err == nil {
		t.Fatal("a part outside the manifest must not load")
	}
	if _, err := LoadPart(root, "lead-workflow-manual", "../whiteboard/whiteboard"); err == nil {
		t.Fatal("a part name must be a bare stem")
	}
}
