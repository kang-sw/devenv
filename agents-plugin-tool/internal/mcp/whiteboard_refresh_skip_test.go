package mcp

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// A panel-dirtying write whose page renders the same as the published one
// rewrites neither the store nor any output; a changed page publishes the
// page and its stamp as before.
func TestWhiteboardRefreshSkipsUnchangedPage(t *testing.T) {
	f := newWhiteboardFixture(t)
	f.update(t, ``)
	set := func(step int) {
		t.Helper()
		if resp := f.call(t, "agenda.set", map[string]any{"key": "plan", "value": map[string]any{"step": step}}); isToolError(resp) {
			t.Fatalf("agenda.set: %s", resp)
		}
	}
	set(1)
	stamp := f.stamp(t)
	metaPath := filepath.Join(f.pageDir(), "meta.json")
	meta := readFileForTest(t, metaPath)
	metaInfo, err := os.Stat(metaPath)
	if err != nil {
		t.Fatal(err)
	}

	var renamed []string
	orig := whiteboardRename
	whiteboardRename = func(from, to string) error {
		renamed = append(renamed, filepath.Base(to))
		return orig(from, to)
	}
	defer func() { whiteboardRename = orig }()

	set(1)
	after, err := os.Stat(metaPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(renamed) != 0 || f.stamp(t) != stamp || !bytes.Equal(readFileForTest(t, metaPath), meta) || !after.ModTime().Equal(metaInfo.ModTime()) {
		t.Fatalf("unchanged refresh wrote: outputs=%v stamp %q -> %q, store meta rewritten=%v", renamed, stamp, f.stamp(t), !after.ModTime().Equal(metaInfo.ModTime()))
	}

	set(2)
	if want := []string{"index.html", "index.stamp.js"}; !reflect.DeepEqual(renamed, want) {
		t.Fatalf("changed refresh outputs = %v, want %v", renamed, want)
	}
	if f.stamp(t) == stamp {
		t.Fatal("changed refresh did not bump the stamp")
	}
}
