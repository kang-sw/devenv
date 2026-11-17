package mcp

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsgit"
	"github.com/kang-sw/devenv/internal/wsstate"
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

// whiteboardToolReads counts read attempts made from whiteboard_tools.go.
func whiteboardToolReads() int64 {
	var n int64
	for site, count := range wsstate.FileIOCounters() {
		if strings.HasPrefix(site, "file_io_site:mcp/whiteboard_tools.go:") && strings.HasSuffix(site, ":read_file") {
			n += count
		}
	}
	return n
}

// An unchanged refresh compares the multi-megabyte page by its remembered
// hash while the page's stat identity holds; a page another writer replaced
// is read back once and republished.
func TestWhiteboardUnchangedPublishSkipsPageRead(t *testing.T) {
	f := newWhiteboardFixture(t)
	f.update(t, ``)
	set := func(step int) {
		t.Helper()
		if resp := f.call(t, "agenda.set", map[string]any{"key": "plan", "value": map[string]any{"step": step}}); isToolError(resp) {
			t.Fatalf("agenda.set: %s", resp)
		}
	}
	set(1)
	page := readFileForTest(t, f.pagePath())
	before := whiteboardToolReads()
	set(1)
	if got := whiteboardToolReads() - before; got != 0 {
		t.Fatalf("unchanged refresh read the published page %d times", got)
	}
	if !bytes.Equal(readFileForTest(t, f.pagePath()), page) {
		t.Fatal("unchanged refresh rewrote the page")
	}
	if err := os.WriteFile(f.pagePath(), []byte("<html>another writer</html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	before = whiteboardToolReads()
	set(1)
	if got := whiteboardToolReads() - before; got != 1 {
		t.Fatalf("replaced page read %d times, want 1", got)
	}
	if bytes.Contains(readFileForTest(t, f.pagePath()), []byte("another writer")) {
		t.Fatal("replaced page was not republished")
	}
	before = whiteboardToolReads()
	set(1)
	if got := whiteboardToolReads() - before; got != 0 {
		t.Fatalf("republished page read back %d times", got)
	}
}

type countingGitRunner struct {
	calls int
	inner wsgit.Runner
}

func (r *countingGitRunner) RunGit(ctx context.Context, root string, args ...string) ([]byte, error) {
	r.calls++
	return r.inner.RunGit(ctx, root, args...)
}

func TestWhiteboardExcludeResolvesCommonDirOnce(t *testing.T) {
	root := t.TempDir()
	initGit(t, root)
	runner := &countingGitRunner{inner: wsgit.ExecRunner{}}
	dir := filepath.Join(root, "ai-docs", "whiteboards")
	for i := 0; i < 3; i++ {
		if err := registerWhiteboardExclude(runner, root, dir); err != nil {
			t.Fatal(err)
		}
	}
	if runner.calls != 1 || excludeEntries(t, root) != 1 {
		t.Fatalf("git runs=%d exclude entries=%d, want 1 and 1", runner.calls, excludeEntries(t, root))
	}
}
