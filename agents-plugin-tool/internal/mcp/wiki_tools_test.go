package mcp

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
)

// wikiCall calls a wiki tool as the fixture's lead and returns its text,
// failing the test when the call is refused.
func (f *whiteboardFixture) wikiCall(t *testing.T, tool string, args map[string]any) string {
	t.Helper()
	resp := f.call(t, tool, args)
	if isToolError(resp) {
		t.Fatalf("%s(%v) failed: %s", tool, args, resp)
	}
	return toolText(t, resp)
}

// wikiSaved returns the commit a successful wiki.update reports.
func wikiSaved(t *testing.T, out string) string {
	t.Helper()
	commit, _, _ := strings.Cut(resultLine(t, out, "commit: "), " ")
	if len(commit) < 40 {
		t.Fatalf("no commit in the save result:\n%s", out)
	}
	return commit
}

// wikiRead reads page path of wiki docs in store me and returns its text
// and the base the read names.
func (f *whiteboardFixture) wikiRead(t *testing.T, path string) (string, string) {
	t.Helper()
	out := f.wikiCall(t, wikiQueryTool, map[string]any{"store": "me", "op": "read", "wiki": "docs", "path": path, "format": "json"})
	var got struct{ Wiki, Path, Commit, Content string }
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("read json: %v\n%s", err, out)
	}
	if got.Wiki != "docs" || got.Path != path || len(got.Commit) < 40 {
		t.Fatalf("read json lacks its fields: %s", out)
	}
	return got.Content, got.Commit
}

// TestWikiToolsSaveReadHistoryAndConflicts drives a wiki through the
// tools: create, read with its base, save on it, merge a non-overlapping
// edit, refuse an overlapping one and a stale base with both texts, and
// read history, search, and diff.
func TestWikiToolsSaveReadHistoryAndConflicts(t *testing.T) {
	f := newTrackerLeadFixture(t)
	f.trackerUpdate(t, trackerPersonal)

	out := f.wikiCall(t, wikiUpdateTool, map[string]any{"store": "me", "wiki": "docs", "op": "write", "new_wiki": true, "path": "guides/setup", "content": "one\ntwo\nthree\n"})
	c1 := wikiSaved(t, out)
	if !strings.Contains(out, "created the wiki") {
		t.Fatalf("a new_wiki save should say it created the wiki:\n%s", out)
	}

	text := f.wikiCall(t, wikiQueryTool, map[string]any{"store": "me", "op": "read", "wiki": "docs", "path": "guides/setup"})
	if !strings.HasPrefix(text, "wiki docs page guides/setup @ "+c1+"\n") || !strings.Contains(text, "base for wiki.update: "+c1) || !strings.HasSuffix(text, "one\ntwo\nthree\n") {
		t.Fatalf("read should name its commit as the base, then the content:\n%s", text)
	}
	content, base := f.wikiRead(t, "guides/setup")
	if content != "one\ntwo\nthree\n" || base != c1 {
		t.Fatalf("json read = %q @ %s, want the page @ %s", content, base, c1)
	}

	c2 := wikiSaved(t, f.wikiCall(t, wikiUpdateTool, map[string]any{"store": "me", "wiki": "docs", "op": "write", "base": c1, "path": "guides/setup", "content": "one\ntwo\nthree, revised\n", "message": "why it changed"}))

	// A save on the older base that touches other lines merges.
	merged := f.wikiCall(t, wikiUpdateTool, map[string]any{"store": "me", "wiki": "docs", "op": "write", "base": c1, "path": "guides/setup", "content": "one, revised\ntwo\nthree\n"})
	c3 := wikiSaved(t, merged)
	if !strings.Contains(merged, "merged:") {
		t.Fatalf("a non-overlapping save on an older base should merge:\n%s", merged)
	}
	if content, _ := f.wikiRead(t, "guides/setup"); content != "one, revised\ntwo\nthree, revised\n" {
		t.Fatalf("merged page = %q", content)
	}

	// An overlapping save on the older base is refused with both texts.
	refused := f.trackerRefused(t, wikiUpdateTool, map[string]any{"store": "me", "wiki": "docs", "op": "write", "base": c1, "path": "guides/setup", "content": "one\ntwo\nthree, mine\n"})
	for _, want := range []string{"kind: text", "theirs: " + c3, "base: " + c1, "three, mine", "three, revised", "ours_text", "theirs_text"} {
		if !strings.Contains(refused, want) {
			t.Errorf("an overlapping save's conflict lacks %q:\n%s", want, refused)
		}
	}
	// A base that is not an ancestor of the tip (another wiki's commit) is
	// stale; the json conflict carries the same fields.
	other := wikiSaved(t, f.wikiCall(t, wikiUpdateTool, map[string]any{"store": "me", "wiki": "other", "op": "write", "new_wiki": true, "path": "home", "content": "x\n"}))
	raw := f.trackerRefused(t, wikiUpdateTool, map[string]any{"store": "me", "wiki": "docs", "op": "write", "base": other, "path": "guides/setup", "content": "stale\n", "format": "json"})
	var conflict struct {
		Conflict map[string]string `json:"conflict"`
	}
	if err := json.Unmarshal([]byte(raw), &conflict); err != nil {
		t.Fatalf("json conflict: %v\n%s", err, raw)
	}
	if c := conflict.Conflict; c["kind"] != "stale-base" || c["theirs"] != c3 || c["base"] != other || c["ours_text"] != "stale\n" || c["theirs_text"] != "one, revised\ntwo\nthree, revised\n" {
		t.Fatalf("stale conflict = %+v", conflict.Conflict)
	}

	// Refusals: no base, new_wiki on an existing wiki, an unchanged page.
	if text := f.trackerRefused(t, wikiUpdateTool, map[string]any{"store": "me", "wiki": "docs", "op": "write", "path": "guides/setup", "content": "x\n"}); !strings.Contains(text, "base") {
		t.Errorf("a save without base must name it: %s", text)
	}
	if text := f.trackerRefused(t, wikiUpdateTool, map[string]any{"store": "me", "wiki": "docs", "op": "write", "new_wiki": true, "path": "x", "content": "x\n"}); !strings.Contains(text, "already exists") {
		t.Errorf("new_wiki on an existing wiki must be refused: %s", text)
	}

	hist := f.wikiCall(t, wikiQueryTool, map[string]any{"store": "me", "op": "history", "wiki": "docs", "format": "json"})
	var h struct {
		Commits []struct {
			OID, Short, Subject, Message string
			Verified                     bool
			Signer                       string
			Parents                      []string
		}
	}
	if err := json.Unmarshal([]byte(hist), &h); err != nil {
		t.Fatalf("history json: %v\n%s", err, hist)
	}
	// c3 is the merge of c2 and the older-base edit.
	if len(h.Commits) != 4 || h.Commits[0].OID != c3 || len(h.Commits[0].Parents) != 2 {
		t.Fatalf("history = %+v", h.Commits)
	}
	for _, c := range h.Commits {
		if !c.Verified || c.Signer != "ada" || !strings.HasPrefix(c.OID, c.Short) || c.Subject == "" {
			t.Errorf("commit should be verified as ada with a subject: %+v", c)
		}
	}
	textHist := f.wikiCall(t, wikiQueryTool, map[string]any{"store": "me", "op": "history", "wiki": "docs", "path": "guides/setup"})
	if !strings.Contains(textHist, c2) || !strings.Contains(textHist, "verified, signed by ada") || !strings.Contains(textHist, "2 parents") {
		t.Fatalf("text history:\n%s", textHist)
	}

	if hits := f.wikiCall(t, wikiQueryTool, map[string]any{"store": "me", "op": "search", "wiki": "docs", "query": "REVISED"}); !strings.Contains(hits, "2 hits") || !strings.Contains(hits, "guides/setup:3: three, revised") {
		t.Fatalf("search:\n%s", hits)
	}
	if diff := f.wikiCall(t, wikiQueryTool, map[string]any{"store": "me", "op": "diff", "wiki": "docs", "path": "guides/setup", "to": c2}); !strings.Contains(diff, "-three\n+three, revised") {
		t.Fatalf("diff:\n%s", diff)
	}
	if pages := f.wikiCall(t, wikiQueryTool, map[string]any{"store": "me", "op": "pages", "wiki": "docs"}); !strings.Contains(pages, "1 page\nguides/setup\n") {
		t.Fatalf("pages:\n%s", pages)
	}
	if list := f.wikiCall(t, wikiQueryTool, map[string]any{"store": "me", "op": "list"}); !strings.Contains(list, "docs @ "+c3) || !strings.Contains(list, "other @ "+other) {
		t.Fatalf("list:\n%s", list)
	}
	if rec := f.wikiCall(t, wikiQueryTool, map[string]any{"store": "me", "op": "recovery"}); !strings.Contains(rec, "no recovery points") {
		t.Fatalf("recovery:\n%s", rec)
	}
	// The central precheck holds the new schemas: required and declared names.
	if text := f.trackerRefused(t, wikiQueryTool, map[string]any{"store": "me", "op": "list", "bogus": 1}); !strings.Contains(text, "bogus") {
		t.Errorf("an undeclared argument must be refused: %s", text)
	}
	if text := f.trackerRefused(t, wikiUpdateTool, map[string]any{"store": "me", "wiki": "docs", "op": "delete"}); !strings.Contains(text, "path") {
		t.Errorf("a missing path must be refused: %s", text)
	}
}

// TestWikiUpdateImages: a write pastes a base64 image and reports its
// sha256 to reference; bytes that are no image, or no base64, are refused.
func TestWikiUpdateImages(t *testing.T) {
	f := newTrackerLeadFixture(t)
	f.trackerUpdate(t, trackerPersonal)
	png := []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR")
	sum := sha256.Sum256(png)
	oid := hex.EncodeToString(sum[:])

	out := f.wikiCall(t, wikiUpdateTool, map[string]any{"store": "me", "wiki": "docs", "op": "write", "new_wiki": true, "path": "home",
		"content": "![shot](attachment:" + oid + ")\n", "images": []any{map[string]any{"data": base64.StdEncoding.EncodeToString(png), "name": "shot.png"}}})
	if !strings.Contains(out, "image shot.png: attachment:"+oid) {
		t.Fatalf("the save should report the image's sha256:\n%s", out)
	}
	_, base := f.wikiRead(t, "home")
	text := f.trackerRefused(t, wikiUpdateTool, map[string]any{"store": "me", "wiki": "docs", "op": "write", "base": base, "path": "home",
		"content": "x\n", "images": []any{map[string]any{"data": base64.StdEncoding.EncodeToString([]byte("plain text, not an image"))}}})
	if !strings.Contains(text, "not a supported image") {
		t.Errorf("a non-image must be refused: %s", text)
	}
	text = f.trackerRefused(t, wikiUpdateTool, map[string]any{"store": "me", "wiki": "docs", "op": "write", "base": base, "path": "home",
		"content": "x\n", "images": []any{map[string]any{"data": "!!not base64!!"}}})
	if !strings.Contains(text, "not base64") {
		t.Errorf("garbage base64 must be refused: %s", text)
	}
}

// TestWikiToolsSupportConversationOwnStoreOnly: a store's support
// conversation lists, reads, and saves the wikis of its own store and is
// refused another store's; its tool list carries both tools.
func TestWikiToolsSupportConversationOwnStoreOnly(t *testing.T) {
	f := newTrackerSupportFixture(t)
	t.Setenv("WS_MCP_ALLOWED_TOOLS", supportAllowlist)
	list := callToolsList(t, f.s)
	for _, name := range []string{wikiQueryTool, wikiUpdateTool} {
		if !strings.Contains(list, `"`+name+`"`) {
			t.Errorf("the support tool list lacks %s", name)
		}
	}

	resp := f.call(t, wikiUpdateTool, f.as(f.support, map[string]any{"store": "me", "wiki": "notes", "op": "write", "new_wiki": true, "path": "home", "content": "hello\n"}))
	if isToolError(resp) {
		t.Fatalf("the support key should save to its own store's wiki: %s", resp)
	}
	if out := toolText(t, f.call(t, wikiQueryTool, f.as(f.support, map[string]any{"store": "me", "op": "read", "wiki": "notes", "path": "home"}))); !strings.HasSuffix(out, "hello\n") {
		t.Fatalf("the support key should read its own store's wiki: %s", out)
	}
	for tool, args := range map[string]map[string]any{
		wikiQueryTool:  {"store": "other", "op": "list"},
		wikiUpdateTool: {"store": "other", "wiki": "notes", "op": "write", "new_wiki": true, "path": "home", "content": "smuggled\n"},
	} {
		text := f.trackerRefused(t, tool, f.as(f.support, args))
		if !strings.Contains(text, `its own store "me" only`) {
			t.Errorf("%s on another store must be refused naming the pinned store: %s", tool, text)
		}
	}
	if out := f.wikiCall(t, wikiQueryTool, map[string]any{"store": "other", "op": "list"}); !strings.Contains(out, "has no wikis") {
		t.Fatalf("a refused save must write nothing: %s", out)
	}
}
