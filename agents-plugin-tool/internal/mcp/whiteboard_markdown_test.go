package mcp

import (
	"encoding/json"
	"net/url"
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
// paths reach the pages: a relative path resolves against the session root
// and a bare absolute path is a file path; the full page, inside the
// repository, links both relative to itself, and an embed written to an
// embed_dir outside the repository links them by absolute file: URLs.
func TestWhiteboardMarkdownFileLinks(t *testing.T) {
	f := newWhiteboardFixture(t)
	abs := filepath.Join(f.root, "docs", "notes.md")
	raw, _ := json.Marshal(map[string]any{"add": []any{map[string]any{
		"class": "policy", "topic": "t", "title": "see [notes](" + filepath.ToSlash(abs) + ")",
		"options":   []string{"x", "y"},
		"rationale": "![shot](img/shot%20one.png)\n[web](https://example.com/)",
	}}})
	f.update(t, string(raw))
	full := readWhiteboardPage(t, filepath.Join(f.dir(), f.replyID()+".html"))
	for _, want := range []string{
		`<img src="../../img/shot%20one.png" alt="shot">`,
		`<a href="../../docs/notes.md">notes</a>`,
		`<a href="https://example.com/" target="_blank" rel="noopener noreferrer">web</a>`,
	} {
		if !strings.Contains(full, want) {
			t.Errorf("full page lacks %q", want)
		}
	}

	host := t.TempDir()
	raw, _ = json.Marshal(map[string]any{"embed_dir": host})
	embed := resultLine(t, f.update(t, string(raw)), "embed: ")
	page := readWhiteboardPage(t, filepath.Join(host, embed))
	fileURL := func(p string) string {
		return (&url.URL{Scheme: "file", Path: filepath.ToSlash(p)}).String()
	}
	for _, want := range []string{
		`<img src="` + fileURL(filepath.Join(f.root, "img", "shot one.png")) + `" alt="shot">`,
		`<a href="` + fileURL(abs) + `">notes</a>`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("embed in an outside embed_dir lacks %q", want)
		}
	}
	if full := readWhiteboardPage(t, filepath.Join(f.dir(), f.replyID()+".html")); !strings.Contains(full, `<img src="../../img/shot%20one.png"`) {
		t.Errorf("the full page must keep its repository-relative link while embed_dir is set")
	}
}

// TestWhiteboardMermaidAssetWrittenOnce pins the shared mermaid script: a
// page with a mermaid block loads it by its bare hashed name, and the page
// writer stores it beside that page, in the board folder and in embed_dir,
// only when it is missing; a board without a mermaid block writes none.
func TestWhiteboardMermaidAssetWrittenOnce(t *testing.T) {
	name, data := wswhiteboard.MermaidAsset()

	plain := newWhiteboardFixture(t)
	plain.update(t, `{"add":[{"class":"technical","topic":"t","title":"no diagram"}]}`)
	if _, err := os.Stat(filepath.Join(plain.dir(), name)); !os.IsNotExist(err) {
		t.Fatalf("a board without a mermaid block wrote the mermaid script: %v", err)
	}

	f := newWhiteboardFixture(t)
	raw, _ := json.Marshal(map[string]any{"add": []any{map[string]any{
		"class": "policy", "topic": "t", "title": "diagram", "options": []string{"x", "y"},
		"rationale": "```mermaid\ngraph LR\nA-->B\n```",
	}}})
	embed := resultLine(t, f.update(t, string(raw)), "embed: ")
	asset := filepath.Join(f.dir(), name)
	got, err := os.ReadFile(asset)
	if err != nil || len(got) != len(data) {
		t.Fatalf("mermaid script not written beside the pages: %v (%d of %d bytes)", err, len(got), len(data))
	}
	tag := `<script src="` + name + `"></script>`
	for _, page := range []string{filepath.Join(f.dir(), f.replyID()+".html"), filepath.Join(f.root, embed)} {
		if !strings.Contains(readWhiteboardPage(t, page), tag) {
			t.Errorf("%s does not load the shared script by its relative name", page)
		}
	}

	// Written only when missing: a later update leaves the file alone.
	if err := os.WriteFile(asset, []byte("sentinel"), 0o644); err != nil {
		t.Fatal(err)
	}
	f.update(t, `{}`)
	if got, _ := os.ReadFile(asset); string(got) != "sentinel" {
		t.Fatalf("an update rewrote the existing mermaid script")
	}

	host := t.TempDir()
	raw, _ = json.Marshal(map[string]any{"embed_dir": host})
	embed = resultLine(t, f.update(t, string(raw)), "embed: ")
	if got, err := os.ReadFile(filepath.Join(host, name)); err != nil || len(got) != len(data) {
		t.Fatalf("mermaid script not written into embed_dir: %v", err)
	}
	if !strings.Contains(readWhiteboardPage(t, filepath.Join(host, embed)), tag) {
		t.Errorf("the embed in embed_dir does not load the shared script by its relative name")
	}

	// The shared script is not a session file, so whiteboard GC keeps it.
	if whiteboardFilePattern.MatchString(name) {
		t.Errorf("GC pattern matches the shared mermaid script %q", name)
	}
}
