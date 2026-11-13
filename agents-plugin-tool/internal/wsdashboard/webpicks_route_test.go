package wsdashboard

import (
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsinbox"
	"github.com/kang-sw/devenv/internal/wslive"
)

// TestMessageCarriesPicksAndConsoles: a send's picks and console blocks are
// held in order, merge with a later send's, are delivered as wb:pick and
// wb:console elements after the mentions and before the text, and a cancel,
// a steer, and an interrupt hand them back with the same field names.
func TestMessageCarriesPicksAndConsoles(t *testing.T) {
	f := startPageFixture(t)
	f.send(t, `{"text":"see","picks":[{"url":"http://localhost:5173/settings","selector":"#save","text":"Save"}],"consoles":[{"url":"http://localhost:5173/settings","lines":["first error","second error"]}]}`)
	id, merged := f.send(t, `{"picks":[{"url":"http://localhost:5173/settings","selector":"#empty","text":""}]}`)
	if !merged {
		t.Fatal("a pick-only send did not merge")
	}
	wantPicks := []wsinbox.Pick{{URL: "http://localhost:5173/settings", Selector: "#save", Text: "Save"}, {URL: "http://localhost:5173/settings", Selector: "#empty"}}
	wantConsoles := []wsinbox.Console{{URL: "http://localhost:5173/settings", Lines: []string{"first error", "second error"}}}
	held := f.held(t)
	if len(held) != 1 || !reflect.DeepEqual(held[0].Entry.Picks, wantPicks) || !reflect.DeepEqual(held[0].Entry.Consoles, wantConsoles) {
		t.Fatalf("held = %+v", held)
	}
	wantText := `<wb:pick url="http://localhost:5173/settings" selector="#save">Save</wb:pick>` + "\n" +
		`<wb:pick url="http://localhost:5173/settings" selector="#empty"></wb:pick>` + "\n\n" +
		`<wb:console url="http://localhost:5173/settings">first error` + "\nsecond error</wb:console>\n\nsee"
	if held[0].Entry.Text() != wantText {
		t.Fatalf("held text = %q", held[0].Entry.Text())
	}

	// The held bubble shows capsules, not element text.
	_, page := get(t, f.base+"/s/livesession")
	for _, want := range []string{
		`<span class="achip mchip pchip" title="http://localhost:5173/settings&#10;#save&#10;Save"><b>#save</b><span class="anote">Save</span></span>`,
		`<b>#empty</b></span>`,
		`<span class="achip mchip cchip" title="http://localhost:5173/settings&#10;first error&#10;second error"><b>console &times;2</b><span class="anote">http://localhost:5173/settings</span></span>`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("held bubble lacks %q", want)
		}
	}
	if strings.Contains(page, "wb:pick") || strings.Contains(page, "wb:console") {
		t.Error("the page shows raw element text")
	}

	code, _, raw := postJSON(t, f.base+"/s/livesession/cancel", f.base, `{"id":"`+id+`"}`)
	var cancelled struct {
		Text     string            `json:"text"`
		Picks    []wsinbox.Pick    `json:"picks"`
		Consoles []wsinbox.Console `json:"consoles"`
	}
	if code != http.StatusOK || json.Unmarshal([]byte(raw), &cancelled) != nil || cancelled.Text != "see" ||
		!reflect.DeepEqual(cancelled.Picks, wantPicks) || !reflect.DeepEqual(cancelled.Consoles, wantConsoles) {
		t.Fatalf("cancel = %d %s", code, raw)
	}
	if !strings.Contains(raw, `"picks":[{"url":`) || !strings.Contains(raw, `"consoles":[{"url":`) {
		t.Fatalf("cancel answer lacks the fields: %s", raw)
	}
	// A message without them hands back empty lists, never null.
	id, _ = f.send(t, `{"text":"plain"}`)
	if _, _, raw := postJSON(t, f.base+"/s/livesession/cancel", f.base, `{"id":"`+id+`"}`); !strings.Contains(raw, `"picks":[]`) || !strings.Contains(raw, `"consoles":[]`) {
		t.Fatalf("plain cancel answer = %s", raw)
	}

	// The returned body re-sends as it is; steer and interrupt hand it back.
	body, err := json.Marshal(cancelled)
	if err != nil {
		t.Fatal(err)
	}
	id, _ = f.send(t, string(body))
	if code, _, raw := postJSON(t, f.base+"/s/livesession/steer", f.base, `{"id":"`+id+`"}`); code != http.StatusOK {
		t.Fatalf("steer = %d %s", code, raw)
	}
	if held := f.held(t); len(held) != 1 || !held[0].Entry.Steer || !reflect.DeepEqual(held[0].Entry.Picks, wantPicks) || !reflect.DeepEqual(held[0].Entry.Consoles, wantConsoles) {
		t.Fatalf("steered = %+v", held)
	}
	code, _, raw = postJSON(t, f.base+"/s/livesession/interrupt", f.base, ``)
	var interrupted struct {
		Held []struct {
			Picks    []wsinbox.Pick    `json:"picks"`
			Consoles []wsinbox.Console `json:"consoles"`
		} `json:"held"`
	}
	if code != http.StatusOK || json.Unmarshal([]byte(raw), &interrupted) != nil || len(interrupted.Held) != 1 ||
		!reflect.DeepEqual(interrupted.Held[0].Picks, wantPicks) || !reflect.DeepEqual(interrupted.Held[0].Consoles, wantConsoles) {
		t.Fatalf("interrupt = %d %s", code, raw)
	}
}

// TestMessageRouteRefusesMalformedPicksAndConsoles: a pick without a selector
// or url, a console line with a line break, and a count over a cap answer 400
// and hold nothing; a command carrying either is refused too.
func TestMessageRouteRefusesMalformedPicksAndConsoles(t *testing.T) {
	f := startPageFixture(t)
	tooMany := `{"picks":[` + strings.Repeat(`{"url":"u","selector":"#a"},`, 50) + `{"url":"u","selector":"#a"}]}`
	for _, body := range []string{
		`{"picks":[{"url":"u","selector":""}]}`,
		`{"picks":[{"url":"","selector":"#a"}]}`,
		`{"consoles":[{"url":"u","lines":["a\nb"]}]}`,
		`{"consoles":[{"lines":["a"]}]}`,
		tooMany,
	} {
		if code, _, raw := postJSON(t, f.base+"/s/livesession/message", f.base, body); code != http.StatusBadRequest {
			t.Errorf("send %.80s = %d (%s), want 400", body, code, raw)
		}
	}
	if held := f.held(t); len(held) != 0 {
		t.Fatalf("refused sends held %+v", held)
	}
}

// TestMessageBodyLimitAdmitsEveryCap: a message with picks and console
// blocks at every cap, quote-heavy so JSON escaping doubles it, and text at
// its limit is accepted; the same message without picks or console blocks
// keeps the text-only body limit.
func TestMessageBodyLimitAdmitsEveryCap(t *testing.T) {
	f := startPageFixture(t)
	quotes := func(n int) string { return strings.Repeat(`"`, n) }
	type pick struct {
		URL      string `json:"url"`
		Selector string `json:"selector"`
		Text     string `json:"text"`
	}
	type console struct {
		URL   string   `json:"url"`
		Lines []string `json:"lines"`
	}
	req := struct {
		Text     string    `json:"text"`
		Picks    []pick    `json:"picks"`
		Consoles []console `json:"consoles"`
	}{Text: strings.Repeat("x", 64<<10)}
	for i := 0; i < 50; i++ {
		req.Picks = append(req.Picks, pick{URL: "http://localhost:1/" + quotes(2048-19), Selector: quotes(512), Text: quotes(1024)})
	}
	for i := 0; i < 10; i++ {
		c := console{URL: "http://localhost:1/" + quotes(2048-19)}
		for j := 0; j < 50; j++ {
			c.Lines = append(c.Lines, quotes(1024))
		}
		req.Consoles = append(req.Consoles, c)
	}
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	if len(body) <= inboxBodyLimit {
		t.Fatalf("test body is %d bytes, not over the text-only limit", len(body))
	}
	if code, raw := f.postMessage(t, string(body)); code != http.StatusOK {
		t.Fatalf("message at every cap = %d (%.200s)", code, raw)
	}
	held := f.held(t)
	if len(held) != 1 || len(held[0].Entry.Picks) != 50 || len(held[0].Entry.Consoles) != 10 || len(held[0].Entry.Consoles[9].Lines) != 50 {
		t.Fatalf("held = %d entries", len(held))
	}
	// The text-only limit still holds without picks or console blocks.
	long := strings.Repeat("x", inboxBodyLimit+10)
	if code, _ := f.postMessage(t, `{"text":"`+long+`"}`); code != http.StatusBadRequest {
		t.Errorf("oversized text-only body = %d, want 400", code)
	}
}

// TestDeliveredPicksAndConsolesRenderAsCapsules: a delivered message's pick
// and console elements parse out of its text after the mentions and render
// as capsules, the typed text stays, and a malformed element stays text.
func TestDeliveredPicksAndConsolesRenderAsCapsules(t *testing.T) {
	f := startPageFixture(t)
	delivered := wsinbox.Entry{
		Parts:    []string{"user words"},
		Mentions: []wsinbox.Mention{{Topic: "plan", Text: "m"}},
		Picks:    []wsinbox.Pick{{URL: "http://localhost:5173/", Selector: `[data-testid="a&b"]`, Text: "x < y"}},
		Consoles: []wsinbox.Console{{URL: "http://localhost:5173/", Lines: []string{"E1 <boom>", "E2", "E3"}}},
	}.Text()
	malformed := `<wb:pick url="u" selector="#a">raw <b>tag</b></wb:pick>` + "\nafter"
	if err := wslive.AppendRing(wslive.RingDir(SessionDir(f.dir, "livesession")), []wslive.RingEntry{
		{Kind: wslive.RingUser, Origin: wslive.OriginWhiteboard, Text: delivered},
		{Kind: wslive.RingUser, Origin: wslive.OriginWhiteboard, Text: malformed},
	}); err != nil {
		t.Fatal(err)
	}
	_, page := get(t, f.base+"/s/livesession")
	for _, want := range []string{
		`<span class="achip mchip pchip" title="http://localhost:5173/&#10;[data-testid=&#34;a&amp;b&#34;]&#10;x &lt; y"><b>[data-testid=&#34;a&amp;b&#34;]</b><span class="anote">x &lt; y</span></span>`,
		`<span class="achip mchip cchip" title="http://localhost:5173/&#10;E1 &lt;boom&gt;&#10;E2&#10;E3"><b>console &times;3</b><span class="anote">http://localhost:5173/</span></span></div>user words</div>`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("session page lacks %q", want)
		}
	}
	if strings.Contains(page, "&lt;wb:console") || strings.Contains(page, "&lt;wb:pick url=&#34;http://localhost:5173/") {
		t.Error("a well-formed element shows as raw text")
	}
	if !strings.Contains(page, "&lt;wb:pick url=&#34;u&#34; selector=&#34;#a&#34;&gt;raw") {
		t.Error("a malformed element did not stay in the text")
	}
}
