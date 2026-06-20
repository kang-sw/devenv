package wsrsrc

import (
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

func TestRetiredAPIGuidanceNotShipped(t *testing.T) {
	repoRoot := filepath.Join("..", "..", "..")
	roots := []string{
		filepath.Join(repoRoot, "agents-plugin", "rsrc"),
		filepath.Join(repoRoot, "agents-plugin-wsflow", "rsrc"),
		filepath.Join(repoRoot, "agents-plugin", "skills"),
		filepath.Join(repoRoot, "ai-docs", "spec"),
		filepath.Join(repoRoot, "ai-docs", "mental-model"),
	}
	forbidden := []string{
		"ws/api.ask(prompt",
		"ws/api.ask_async(prompt",
		"api.ask_async(prompt",
		"api_job_key",
		"Use `ws/api.ask",
		"call `ws/api.ask",
		"route external dependency/API documentation questions through `ws/api.ask`",
		"ask through `ws/api.ask`",
	}
	for _, root := range roots {
		if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			text := string(data)
			for _, needle := range forbidden {
				if strings.Contains(text, needle) {
					t.Fatalf("retired API ask guidance %q found in %s", needle, path)
				}
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
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
