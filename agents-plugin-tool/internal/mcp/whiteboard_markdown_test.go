package mcp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

func readWhiteboardPage(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

// TestWhiteboardMarkdownFileLinks pins how an item's markdown image and link
// paths reach the page: a relative path resolves against the session root and
// a bare absolute path is a file path; the full page links both relative to
// itself.
func TestWhiteboardMarkdownFileLinks(t *testing.T) {
	f := newWhiteboardFixture(t)
	abs := filepath.Join(f.root, "docs", "notes.md")
	f.update(t, `<wb:topic name="t"><wb:add class="policy">
<wb:title>see [notes](`+filepath.ToSlash(abs)+`)</wb:title>
<wb:option>x</wb:option>
<wb:option>y</wb:option>
<wb:why>![shot](img/shot%20one.png)
[web](https://example.com/)</wb:why>
</wb:add></wb:topic>`)
	full := readWhiteboardPage(t, f.pagePath())
	for _, want := range []string{
		`<img src="../../../img/shot%20one.png" alt="shot"/>`,
		`<a href="../../../docs/notes.md">notes</a>`,
		`<a href="https://example.com/" target="_blank" rel="noopener noreferrer">web</a>`,
	} {
		if !strings.Contains(full, want) {
			t.Errorf("full page lacks %q", want)
		}
	}
}

// TestWhiteboardMarkdownFileLinkNeverYieldsAScheme pins the real link rule
// against a file named like a URL beside the page: relative to the page it
// is a bare "javascript:..." name, which must not reach an href as a scheme.
func TestWhiteboardMarkdownFileLinkNeverYieldsAScheme(t *testing.T) {
	f := newWhiteboardFixture(t)
	f.update(t, `
<wb:topic name="t"><wb:add class="policy">
<wb:title>[a](./ai-docs/whiteboards/javascript:alert(1)) [b](ai-docs/whiteboards/javascript%3Aalert%282%29)</wb:title>
<wb:option>x</wb:option>
<wb:option>y</wb:option>
<wb:why>[c](./javascript:alert(3))
![d](javascript%3Ax.png)</wb:why>
</wb:add></wb:topic>`)
	full := readWhiteboardPage(t, f.pagePath())
	for _, bad := range []string{`href="javascript`, `src="javascript`} {
		if strings.Contains(full, bad) {
			t.Errorf("full page holds %q", bad)
		}
	}
	for _, want := range []string{`href="../javascript:alert(1)">a</a>`, `href="../javascript:alert(2)">b</a>`} {
		if !strings.Contains(full, want) {
			t.Errorf("full page lacks %q", want)
		}
	}
}

// TestWhiteboardMermaidAssetWrittenOnce pins the shared mermaid script: a
// page with a mermaid block loads it by its bare hashed name, and the page
// writer stores it beside that page only when it is missing; a board without
// a mermaid block writes none.
func TestWhiteboardMermaidAssetWrittenOnce(t *testing.T) {
	name, data := wswhiteboard.MermaidAsset()

	plain := newWhiteboardFixture(t)
	plain.update(t, `<wb:topic name="t"><wb:add class="technical"><wb:title>no diagram</wb:title></wb:add></wb:topic>`)
	if _, err := os.Stat(filepath.Join(plain.pageDir(), name)); !os.IsNotExist(err) {
		t.Fatalf("a board without a mermaid block wrote the mermaid script: %v", err)
	}

	f := newWhiteboardFixture(t)
	f.update(t, "<wb:topic name=\"t\"><wb:add class=\"policy\">\n<wb:title>diagram</wb:title>\n<wb:option>x</wb:option>\n<wb:option>y</wb:option>\n<wb:why>\n```mermaid\ngraph LR\nA-->B\n```\n</wb:why>\n</wb:add></wb:topic>")
	asset := filepath.Join(f.pageDir(), name)
	got, err := os.ReadFile(asset)
	if err != nil || len(got) != len(data) {
		t.Fatalf("mermaid script not written beside the pages: %v (%d of %d bytes)", err, len(got), len(data))
	}
	tag := `<script src="` + name + `"></script>`
	for _, page := range []string{f.pagePath()} {
		if got, err := os.ReadFile(filepath.Join(filepath.Dir(page), name)); err != nil || len(got) != len(data) {
			t.Errorf("%s is missing its adjacent mermaid asset: %v", page, err)
		}
		if !strings.Contains(readWhiteboardPage(t, page), tag) {
			t.Errorf("%s does not load the shared script by its relative name", page)
		}
	}

	// Written only when missing: a later update leaves the file alone.
	if err := os.WriteFile(asset, []byte("sentinel"), 0o644); err != nil {
		t.Fatal(err)
	}
	f.update(t, ``)
	if got, _ := os.ReadFile(asset); string(got) != "sentinel" {
		t.Fatalf("an update rewrote the existing mermaid script")
	}
	// The shared script is not a session file, so whiteboard GC keeps it.
	if whiteboardFilePattern.MatchString(name) {
		t.Errorf("GC pattern matches the shared mermaid script %q", name)
	}
}
