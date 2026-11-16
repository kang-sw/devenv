package wsdashboard

import (
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wslive"
)

// chatBatch is one ring user entry holding two enveloped sends, the first
// with an answer block the page draws as capsules.
const chatBatch = `<ws:user at="2026-11-16T18:31+09:00">` + "\n<wb:answers>\n03=a\n</wb:answers>\n\nfirst **ask**\n</ws:user>\n" +
	`<ws:user at="2026-11-16T18:32+09:00">` + "\nsecond ask\n</ws:user>"

// chatFixture is the page fixture's ring plus a reply, an enveloped batch,
// and a truncated reply; it returns the ring's entry names, oldest first.
func chatFixture(t *testing.T) (*pageFixture, []string) {
	t.Helper()
	f := startPageFixture(t)
	dir := wslive.RingDir(SessionDir(f.dir, "livesession"))
	if err := wslive.AppendRing(dir, []wslive.RingEntry{
		{Kind: wslive.RingReply, Text: "**done**: see `x`\n\n- one"},
		{Kind: wslive.RingUser, Origin: wslive.OriginWhiteboard, Text: chatBatch},
		{Kind: wslive.RingReply, Text: "head of a long reply", Truncated: true},
	}); err != nil {
		t.Fatal(err)
	}
	names, err := wslive.RingNames(dir)
	if err != nil || len(names) != 7 {
		t.Fatalf("ring names = %v, %v", names, err)
	}
	return f, names
}

func getEntryText(t *testing.T, url string) entryTextAnswer {
	t.Helper()
	code, body := get(t, url)
	if code != http.StatusOK {
		t.Fatalf("GET %s = %d: %s", url, code, body)
	}
	var ans entryTextAnswer
	if err := json.Unmarshal([]byte(body), &ans); err != nil {
		t.Fatalf("entry text answer %q: %v", body, err)
	}
	return ans
}

// TestEntryTextRouteAnswersStoredText pins what Copy copies: a reply's raw
// markdown, a prompt's text before the display parsers (one text per
// bubble of a batch, the answer block kept), and the truncation flag.
func TestEntryTextRouteAnswersStoredText(t *testing.T) {
	f, names := chatFixture(t)
	url := func(key string) string { return f.base + "/s/livesession/entry?key=" + key }
	for _, c := range []struct {
		key       string
		blocks    []string
		truncated bool
	}{
		{names[1], []string{"from the terminal"}, false},
		{names[4], []string{"**done**: see `x`\n\n- one"}, false},
		{names[5], []string{"<wb:answers>\n03=a\n</wb:answers>\n\nfirst **ask**", "second ask"}, false},
		{names[6], []string{"head of a long reply"}, true},
	} {
		if got := getEntryText(t, url(c.key)); !reflect.DeepEqual(got.Blocks, c.blocks) || got.Truncated != c.truncated {
			t.Errorf("entry %s = %+v, want %q truncated %v", c.key, got, c.blocks, c.truncated)
		}
	}
}

// TestEntryTextRouteRefusesBadRequests pins the route's input rules: key is
// an entry name, never a path; an absent entry, an entry without text (a
// mail line), and an unknown session are not found; it answers reads only.
func TestEntryTextRouteRefusesBadRequests(t *testing.T) {
	f, names := chatFixture(t)
	for _, q := range []string{"", "key=", "key=..%2F..%2Fsnapshot.json", "key=nostamp.json"} {
		if code, body := get(t, f.base+"/s/livesession/entry?"+q); code != http.StatusBadRequest {
			t.Errorf("entry?%s = %d (%s), want 400", q, code, body)
		}
	}
	for _, path := range []string{
		"/s/livesession/entry?key=20261107T090000.000000000Z-ab.json",
		"/s/livesession/entry?key=" + names[2],
		"/s/nosuchsession/entry?key=" + names[4],
	} {
		if code, body := get(t, f.base+path); code != http.StatusNotFound {
			t.Errorf("%s = %d (%s), want 404", path, code, body)
		}
	}
	resp, err := testClient().Post(f.base+"/s/livesession/entry?key="+names[4], "text/plain", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("POST entry = %d", resp.StatusCode)
	}
}

// TestChatBlocksRenderTheActionPill pins where the pill renders: on every
// finished reply (Copy only) and every owner prompt bubble, a batch's
// included (rewind and edit dimmed but focusable, then Copy), never on the
// streaming reply or a held message; a truncated block's Copy says so.
func TestChatBlocksRenderTheActionPill(t *testing.T) {
	f, _ := chatFixture(t)
	f.send(t, `{"text":"held while the lead works"}`)
	_, page := get(t, f.base+"/s/livesession?frame=1")
	for _, want := range []string{`class="msg fromlead streaming"`, `class="msg user held"`} {
		if !strings.Contains(page, want) {
			t.Fatalf("fixture page lacks %q", want)
		}
	}
	// Two plain prompts, two batch bubbles, two finished replies.
	for marker, want := range map[string]int{
		`<div class="d-acts" role="toolbar"`:                                              6,
		`data-act="copy"`:                                                                 6,
		`data-act="rewind" aria-disabled="true"`:                                          4,
		`data-act="edit" aria-disabled="true"`:                                            4,
		`data-tip="Copy (stored text is truncated: only its start)"`:                      1,
		`<span class="d-act-done">Copied (truncated)</span>`:                              1,
		`<span class="d-act-ok"><svg class="i" aria-hidden="true"><use href="#i-check"/>`: 6,
	} {
		if got := strings.Count(page, marker); got != want {
			t.Errorf("%d of %s, want %d", got, marker, want)
		}
	}
	// The pill follows its bubble inside the block, so the block's key and
	// hover cover it.
	if strings.Count(page, ` …</div><div class="d-acts" role="toolbar"`) != 1 {
		t.Error("the reply's pill is not the bubble's next sibling")
	}
}

// TestUserBlockTextsMatchTheBubbles: the route's per-bubble texts line up
// with the bubbles splitUserEntries draws, whatever the envelope shape.
func TestUserBlockTextsMatchTheBubbles(t *testing.T) {
	for _, text := range []string{
		"plain legacy text",
		chatBatch,
		"outside first\n" + `<ws:user at="2026-11-16T18:31+09:00">` + "\ninside\n</ws:user>",
		`<ws:user at="2026-11-16T18:31+09:00">` + "\nunterminated",
		`<ws:user at="2026-11-16T18:31+09:00">` + "\nonly one\n</ws:user>",
	} {
		entries := []ConvEntry{{RingEntry: wslive.RingEntry{Kind: wslive.RingUser, Text: text}, Key: "k"}}
		splitUserEntries(entries, t.TempDir(), "livesession")
		bubbles := max(1, len(entries[0].UserBlocks))
		if got := userBlockTexts(text); len(got) != bubbles {
			t.Errorf("%q: %d texts for %d bubbles", text, len(got), bubbles)
		}
	}
}
