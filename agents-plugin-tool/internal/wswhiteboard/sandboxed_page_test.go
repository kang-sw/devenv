package wswhiteboard

import (
	"html"
	"regexp"
	"strings"
	"testing"
)

// TestSandboxedPageFrame pins the generic author-page frame: exactly one
// iframe, the whiteboard's sandbox token set (scripts in an opaque origin,
// popups as ordinary pages, nothing else), the title escaped, and the
// document escaped into srcdoc verbatim with nothing injected.
func TestSandboxedPageFrame(t *testing.T) {
	doc := "<!DOCTYPE html>\n<html><head><script src=\"https://cdn.example/lib.js\"></script></head><body onload=\"go()\">\n<p class='x'>a & b \"q\"</p></body></html>\n"
	got := string(SandboxedPage(`notes/"x".html`, doc))
	if strings.Count(got, "<iframe") != 1 || !strings.HasPrefix(got, "<iframe ") || !strings.HasSuffix(got, "></iframe>") {
		t.Fatalf("frame = %s", got)
	}
	m := regexp.MustCompile(`^<iframe title="([^"]*)" sandbox="([^"]*)" srcdoc="([^"]*)"></iframe>$`).FindStringSubmatch(got)
	if m == nil {
		t.Fatalf("frame markup = %s", got)
	}
	if html.UnescapeString(m[1]) != `notes/"x".html` {
		t.Errorf("title = %q", m[1])
	}
	if m[2] != "allow-scripts allow-popups allow-popups-to-escape-sandbox" {
		t.Errorf("sandbox = %q", m[2])
	}
	for _, bad := range []string{"allow-same-origin", "allow-forms", "allow-top-navigation", "allow-modals", "allow-downloads"} {
		if strings.Contains(m[2], bad) {
			t.Errorf("sandbox carries %s", bad)
		}
	}
	if html.UnescapeString(m[3]) != doc {
		t.Errorf("srcdoc = %q, want the document verbatim", html.UnescapeString(m[3]))
	}
}
