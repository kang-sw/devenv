package wswhiteboard

import (
	"errors"
	"io/fs"
	"strings"
	"testing"
)

func whiteboardTestAsset(name string) ([]byte, error) {
	return readPageSource(embeddedAsset, strings.TrimPrefix(name, "assets/"))
}

// Every embedded stylesheet fragment must participate in the served assembly.
// Otherwise adding a page-*.css file silently leaves its styles out of pages.
func TestPageSourceIncludesAllEmbeddedStyles(t *testing.T) {
	files, err := fs.Glob(assetFS, "assets/page-*.css")
	if err != nil {
		t.Fatal(err)
	}
	included := make(map[string]bool, len(pageStyleFiles))
	for _, name := range pageStyleFiles {
		if included[name] {
			t.Errorf("stylesheet %s is included more than once", name)
		}
		included[name] = true
	}
	for _, name := range files {
		if !included[strings.TrimPrefix(name, "assets/")] {
			t.Errorf("embedded stylesheet %s is missing from pageStyleFiles", name)
		}
	}
}

func TestPageSourceAssembly(t *testing.T) {
	files := []string{"page.css", "page-items.css", "page-stream.css", "page-panel-diff.css", "page-panels.css", "page-sticky.css"}
	var calls []string
	raw, err := readPageSource(func(name string) ([]byte, error) {
		calls = append(calls, name)
		return []byte("<" + name + ">"), nil
	}, "page.css")
	want := "<" + strings.Join(files, "><") + ">"
	if err != nil || string(raw) != want || strings.Join(calls, ",") != strings.Join(files, ",") {
		t.Fatalf("assembly = %q, %v, reads %v; want %q, reads %v", raw, err, calls, want, files)
	}
	for _, missing := range files {
		failure := errors.New("missing " + missing)
		_, err := readPageSource(func(name string) ([]byte, error) {
			if name == missing {
				return nil, failure
			}
			return []byte(name), nil
		}, "page.css")
		if !errors.Is(err, failure) {
			t.Errorf("missing %s: got %v", missing, err)
		}
	}
	raw, err = readPageSource(func(name string) ([]byte, error) { return []byte(name), nil }, "stream.js")
	if err != nil || string(raw) != "stream.js" {
		t.Fatalf("unsplit source = %q, %v", raw, err)
	}
}
