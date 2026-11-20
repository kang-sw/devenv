// tracker_wiki_routes_test.go tests tracker wiki routes.
package wsdashboard

import (
	"errors"
	"net/http"
	"strings"
	"testing"
)

// TestTrackerWikiRoutes: the wiki list, page list, page read, and save
// reach the host with the wiki name and answer its results; a save bumps
// the revision; an unknown wiki or page is 404, a refused save 422, a
// conflict 409 with both texts, a host failure 500; the render request's
// wiki mode reaches the host; a malformed wiki name, a bare wiki path, a
// wrong method, a cross-origin save, and an unknown store are refused
// before the host.
func TestTrackerWikiRoutes(t *testing.T) {
	fake := newFakeTracker()
	d := startTrackerDaemon(t, fake)

	code, body := call(d, http.MethodGet, "/tracker/stores/team/wiki", "")
	if code != http.StatusOK || body != `{"wikis":[]}`+"\n" {
		t.Fatalf("wiki list = %d %s", code, body)
	}
	for name, path := range map[string]string{
		"pages of an unknown wiki": "/tracker/stores/team/wiki/notes/pages",
		"page of an unknown wiki":  "/tracker/stores/team/wiki/notes/page?path=home",
	} {
		if code, body := call(d, http.MethodGet, path, ""); code != http.StatusNotFound || !strings.Contains(body, "not found") {
			t.Errorf("%s = %d %s, want 404", name, code, body)
		}
	}
	if code, body := call(d, http.MethodGet, "/tracker/stores/team/wiki/notes/page", ""); code != http.StatusBadRequest || !strings.Contains(body, "path required") {
		t.Errorf("page without a path = %d %s, want 400", code, body)
	}

	rev := trackerRev(t, d)
	save := `{"base":"","newWiki":true,"op":"write","path":"guide/home","content":"# Home\n","attachments":[{"oid":"abc","name":"a.png","size":3,"mime":"image/png"}]}`
	code, body = call(d, http.MethodPost, "/tracker/stores/team/wiki/notes/save", save)
	if code != http.StatusOK || body != `{"commit":"c1","merged":false,"created":true,"warnings":[]}`+"\n" {
		t.Fatalf("save = %d %s", code, body)
	}
	if got := trackerRev(t, d); got != rev+1 {
		t.Errorf("revision after a save = %d, want %d", got, rev+1)
	}
	if n := len(fake.wikiSaves); n != 1 || fake.wikiSaves[0].Path != "guide/home" || len(fake.wikiSaves[0].Attachments) != 1 || fake.wikiSaves[0].Attachments[0].OID != "abc" {
		t.Fatalf("the save reached the host as %+v", fake.wikiSaves)
	}
	code, body = call(d, http.MethodGet, "/tracker/stores/team/wiki", "")
	if code != http.StatusOK || body != `{"wikis":[{"name":"notes","tip":"c1","unpushed":false}]}`+"\n" {
		t.Fatalf("wiki list after a save = %d %s", code, body)
	}
	code, body = call(d, http.MethodGet, "/tracker/stores/team/wiki/notes/pages", "")
	if code != http.StatusOK || body != `{"pages":["guide/home"],"commit":"c1"}`+"\n" {
		t.Fatalf("pages = %d %s", code, body)
	}
	code, body = call(d, http.MethodGet, "/tracker/stores/team/wiki/notes/page?path=guide%2Fhome", "")
	if code != http.StatusOK || body != `{"path":"guide/home","kind":"markdown","content":"# Home\n","commit":"c1","html":"\u003cp\u003e{wiki notes} # Home\n\u003c/p\u003e"}`+"\n" {
		t.Fatalf("page = %d %s", code, body)
	}
	if code, body := call(d, http.MethodGet, "/tracker/stores/team/wiki/notes/page?path=nope", ""); code != http.StatusNotFound {
		t.Errorf("unknown page = %d %s, want 404", code, body)
	}

	// A refusal, a conflict, and a failure, none bumping the revision.
	rev = trackerRev(t, d)
	code, body = call(d, http.MethodPost, "/tracker/stores/team/wiki/notes/save", `{"base":"stale","op":"write","path":"guide/home","content":"x"}`)
	if code != http.StatusUnprocessableEntity || !strings.Contains(body, "wiki save refused") {
		t.Errorf("refused save = %d %s, want 422", code, body)
	}
	fake.wikiConflict = &TrackerWikiConflict{Path: "guide/home", Kind: "text", Reason: "lines overlap", Base: "c1", Theirs: "c2", OursText: "mine", TheirsText: "theirs"}
	code, body = call(d, http.MethodPost, "/tracker/stores/team/wiki/notes/save", `{"base":"c1","op":"write","path":"guide/home","content":"mine"}`)
	want := `{"conflict":{"path":"guide/home","kind":"text","reason":"lines overlap","base":"c1","theirs":"c2","oursText":"mine","theirsText":"theirs"},"error":"wiki text refused on guide/home: lines overlap"}` + "\n"
	if code != http.StatusConflict || body != want {
		t.Errorf("conflicting save = %d %s, want 409 %s", code, body, want)
	}
	fake.wikiConflict = nil
	fake.wikiErr = errors.New("wiki broke")
	for _, r := range []struct{ method, path string }{
		{http.MethodGet, "/tracker/stores/team/wiki"},
		{http.MethodGet, "/tracker/stores/team/wiki/notes/pages"},
		{http.MethodGet, "/tracker/stores/team/wiki/notes/page?path=guide/home"},
		{http.MethodPost, "/tracker/stores/team/wiki/notes/save"},
	} {
		if code, body := call(d, r.method, r.path, `{"base":"c1","op":"write","path":"p","content":"x"}`); code != http.StatusInternalServerError || !strings.Contains(body, "wiki broke") {
			t.Errorf("%s %s with a failing host = %d %s, want 500", r.method, r.path, code, body)
		}
	}
	fake.wikiErr = nil
	if got := trackerRev(t, d); got != rev {
		t.Errorf("revision after refused saves = %d, want %d", got, rev)
	}
	if code, body := call(d, http.MethodPost, "/tracker/stores/team/wiki/notes/save", `{"base":`); code != http.StatusBadRequest || !strings.Contains(body, "malformed") {
		t.Errorf("malformed save = %d %s, want 400", code, body)
	}

	// The render request's wiki mode reaches the host.
	code, body = call(d, http.MethodPost, "/tracker/stores/team/render", `{"text":"x","wiki":"notes"}`)
	if code != http.StatusOK || body != `{"html":"\u003cp\u003ex\u003c/p\u003e{wiki notes}"}`+"\n" {
		t.Fatalf("render in wiki mode = %d %q", code, body)
	}

	before := len(fake.recorded())
	for name, r := range map[string]struct {
		method, path string
		want         int
	}{
		"uppercase wiki":   {http.MethodGet, "/tracker/stores/team/wiki/Notes/pages", http.StatusNotFound},
		"leading hyphen":   {http.MethodGet, "/tracker/stores/team/wiki/-notes/pages", http.StatusNotFound},
		"bare wiki":        {http.MethodGet, "/tracker/stores/team/wiki/notes", http.StatusNotFound},
		"op as the wiki":   {http.MethodGet, "/tracker/stores/team/wiki/pages", http.StatusNotFound},
		"unknown wiki op":  {http.MethodGet, "/tracker/stores/team/wiki/notes/history", http.StatusNotFound},
		"POST list":        {http.MethodPost, "/tracker/stores/team/wiki", http.StatusMethodNotAllowed},
		"POST pages":       {http.MethodPost, "/tracker/stores/team/wiki/notes/pages", http.StatusMethodNotAllowed},
		"GET save":         {http.MethodGet, "/tracker/stores/team/wiki/notes/save", http.StatusMethodNotAllowed},
		"unknown store":    {http.MethodGet, "/tracker/stores/other/wiki", http.StatusNotFound},
		"unknown store op": {http.MethodPost, "/tracker/stores/other/wiki/notes/save", http.StatusNotFound},
	} {
		if code, body := call(d, r.method, r.path, "{}"); code != r.want {
			t.Errorf("%s: %s %s = %d %s, want %d", name, r.method, r.path, code, body, r.want)
		}
	}
	if rec := callRaw(d, http.MethodPost, "/tracker/stores/team/wiki/notes/save", []byte(save), map[string]string{"Origin": "http://evil.example"}); rec.Code != http.StatusForbidden {
		t.Errorf("cross-origin save = %d, want 403", rec.Code)
	}
	if got := fake.recorded(); len(got) != before {
		t.Errorf("refused requests reached the host: %v", got[before:])
	}
}
