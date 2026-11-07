package wsdashboard

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsinbox"
)

// postJSON posts body to url with the given Origin ("" sends none) and
// returns the status, the response, and its body.
func postJSON(t *testing.T, url, origin, body string) (int, http.Header, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	resp, err := testClient().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, resp.Header, string(raw)
}

func (f *pageFixture) inbox(id string) string { return wsinbox.Dir(SessionDir(f.dir, id)) }

func (f *pageFixture) send(t *testing.T, body string) (string, bool) {
	t.Helper()
	code, _, raw := postJSON(t, f.base+"/s/livesession/message", f.base, body)
	if code != http.StatusOK {
		t.Fatalf("send %s = %d: %s", body, code, raw)
	}
	var ans struct {
		ID     string `json:"id"`
		Merged bool   `json:"merged"`
	}
	if err := json.Unmarshal([]byte(raw), &ans); err != nil || ans.ID == "" {
		t.Fatalf("send answer %q: %v", raw, err)
	}
	return ans.ID, ans.Merged
}

func (f *pageFixture) held(t *testing.T) []wsinbox.Record {
	t.Helper()
	held, err := wsinbox.Held(f.inbox("livesession"))
	if err != nil {
		t.Fatal(err)
	}
	return held
}

func TestMessageRouteWritesThenMergesTheHeldEntry(t *testing.T) {
	f := startPageFixture(t)
	first, merged := f.send(t, `{"text":"first send","chips":[{"id":"03","answer":"a"},{"id":"04","answer":"ok"}]}`)
	if merged {
		t.Fatal("first send merged")
	}
	if held := f.held(t); len(held) != 1 || held[0].ID() != first || held[0].Entry.Text() != "first send\n\n(03) a; (04) ok" {
		t.Fatalf("held after one send = %+v", held)
	}
	second, merged := f.send(t, `{"text":"second send","chips":[{"id":"03","answer":"b","note":"changed my mind"}]}`)
	if !merged {
		t.Fatal("second send did not merge into the held message")
	}
	held := f.held(t)
	if len(held) != 1 || held[0].ID() != second {
		t.Fatalf("held after two sends = %+v", held)
	}
	if got, want := held[0].Entry.Text(), "first send\n\nsecond send\n\n(03) b, note: changed my mind; (04) ok"; got != want {
		t.Fatalf("merged message = %q, want %q", got, want)
	}

	// The held bubble shows both sends with a divider, the chips, and a
	// cancel control; the sent data marks the held answers.
	_, page := get(t, f.base+"/s/livesession")
	for _, want := range []string{
		`class="msg user held" data-entry="` + second + `"`,
		`<div class="hpart">first send</div><hr class="pdiv"><div class="hpart">second send</div>`,
		`data-cancel="` + second + `"`,
		`<i data-id="03" data-answer="b" data-note="changed my mind" data-state="held">`,
		`<i data-id="04" data-answer="ok" data-note="" data-state="held">`,
		`id="cchips"`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("session page lacks %q", want)
		}
	}
}

func TestMessageRouteRefusesEmptyAndMalformed(t *testing.T) {
	f := startPageFixture(t)
	for _, body := range []string{`{"text":"  "}`, `{"chips":[{"id":"../x","answer":"a"}]}`, `{"text":`, `{"chips":[{"id":"03","answer":"a b"}]}`} {
		if code, _, raw := postJSON(t, f.base+"/s/livesession/message", f.base, body); code != http.StatusBadRequest {
			t.Errorf("send %s = %d (%s), want 400", body, code, raw)
		}
	}
	if held := f.held(t); len(held) != 0 {
		t.Fatalf("refused sends held %+v", held)
	}
}

func TestCancelRouteReturnsTextAndChips(t *testing.T) {
	f := startPageFixture(t)
	f.send(t, `{"text":"one","chips":[{"id":"03","answer":"a"}]}`)
	id, _ := f.send(t, `{"text":"two","chips":[{"id":"05","note":"why"}]}`)
	code, _, raw := postJSON(t, f.base+"/s/livesession/cancel", f.base, `{"id":"`+id+`"}`)
	if code != http.StatusOK {
		t.Fatalf("cancel = %d: %s", code, raw)
	}
	var ans struct {
		Text  string         `json:"text"`
		Chips []wsinbox.Chip `json:"chips"`
	}
	if err := json.Unmarshal([]byte(raw), &ans); err != nil {
		t.Fatal(err)
	}
	if ans.Text != "one\n\ntwo" || len(ans.Chips) != 2 || ans.Chips[0] != (wsinbox.Chip{ID: "03", Answer: "a"}) || ans.Chips[1] != (wsinbox.Chip{ID: "05", Note: "why"}) {
		t.Fatalf("cancel answer = %+v", ans)
	}
	if held := f.held(t); len(held) != 0 {
		t.Fatalf("cancelled entry still held: %+v", held)
	}
	if code, _, _ := postJSON(t, f.base+"/s/livesession/cancel", f.base, `{"id":"`+id+`"}`); code != http.StatusConflict {
		t.Fatalf("second cancel = %d, want 409", code)
	}
	if code, _, _ := postJSON(t, f.base+"/s/livesession/cancel", f.base, `{"id":"../../registration"}`); code != http.StatusBadRequest {
		t.Fatalf("cancel of a path = %d, want 400", code)
	}
}

func TestCancelRefusedOnAClaimedEntryAndASendAfterADrainStartsANewOne(t *testing.T) {
	f := startPageFixture(t)
	id, _ := f.send(t, `{"text":"one","chips":[{"id":"03","answer":"a"}]}`)
	drained, err := wsinbox.Drain(f.inbox("livesession"))
	if err != nil || len(drained) != 1 {
		t.Fatalf("drain = %+v, %v", drained, err)
	}
	if code, _, raw := postJSON(t, f.base+"/s/livesession/cancel", f.base, `{"id":"`+id+`"}`); code != http.StatusConflict {
		t.Fatalf("cancel of a drained entry = %d (%s), want 409", code, raw)
	}
	// The drain claimed the held entry: the next send is a new entry, not a
	// merge into the delivered one.
	next, merged := f.send(t, `{"text":"two","chips":[{"id":"03","answer":"b"}]}`)
	if merged || next == id {
		t.Fatalf("send after drain merged=%v id=%s", merged, next)
	}
	if held := f.held(t); len(held) != 1 || held[0].Entry.Text() != "two\n\n(03) b" {
		t.Fatalf("held = %+v", held)
	}
	// Sent marks: the held answer shows over the delivered one; once drained
	// the mark reads sent, and survives a reload since it is on disk.
	_, page := get(t, f.base+"/s/livesession")
	if !strings.Contains(page, `<i data-id="03" data-answer="b" data-note="" data-state="held">`) {
		t.Errorf("page lacks the held mark for 03")
	}
	if _, err := wsinbox.Drain(f.inbox("livesession")); err != nil {
		t.Fatal(err)
	}
	_, page = get(t, f.base+"/s/livesession")
	if !strings.Contains(page, `<i data-id="03" data-answer="b" data-note="" data-state="sent">`) || strings.Contains(page, `class="msg user held"`) {
		t.Errorf("after the drain the page lacks the sent mark or still shows a held bubble")
	}
}

func TestSteerRouteRewritesTheHeldEntryMarkedAndAcceptsNoMerge(t *testing.T) {
	f := startPageFixture(t)
	id, _ := f.send(t, `{"text":"promote me","chips":[{"id":"03","answer":"a"}]}`)
	code, _, raw := postJSON(t, f.base+"/s/livesession/steer", f.base, `{"id":"`+id+`"}`)
	if code != http.StatusOK {
		t.Fatalf("steer = %d: %s", code, raw)
	}
	var ans struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(raw), &ans); err != nil || ans.ID == "" || ans.ID == id {
		t.Fatalf("steer answer %q (was %s): %v", raw, id, err)
	}
	// The entry is rewritten under a fresh name with the steer flag.
	held := f.held(t)
	if len(held) != 1 || held[0].ID() != ans.ID || !held[0].Entry.Steer || held[0].Entry.Text() != "promote me\n\n(03) a" {
		t.Fatalf("held after steer = %+v", held)
	}
	// The page shows the steer marker and no steer control for it.
	_, page := get(t, f.base+"/s/livesession")
	if !strings.Contains(page, `class="heldtag steertag"`) || strings.Contains(page, `data-steer="`+ans.ID+`"`) || !strings.Contains(page, `data-cancel="`+ans.ID+`"`) {
		t.Errorf("page lacks the steer marker, still offers steer, or lost cancel for the steered bubble")
	}
	// A later send starts a new held entry, which offers the steer control.
	next, merged := f.send(t, `{"text":"later"}`)
	if merged || next == ans.ID {
		t.Fatalf("send after steer merged=%v id=%s", merged, next)
	}
	if held := f.held(t); len(held) != 2 || held[0].Entry.Text() != "promote me\n\n(03) a" || held[1].Entry.Steer {
		t.Fatalf("held = %+v", held)
	}
	_, page = get(t, f.base+"/s/livesession")
	if !strings.Contains(page, `data-steer="`+next+`"`) {
		t.Errorf("page lacks the steer control for the unsteered bubble")
	}
	// The steered entry is still held: cancel takes it back.
	if code, _, raw := postJSON(t, f.base+"/s/livesession/cancel", f.base, `{"id":"`+ans.ID+`"}`); code != http.StatusOK || !strings.Contains(raw, "promote me") {
		t.Fatalf("cancel of the steered entry = %d: %s", code, raw)
	}
}

func TestSteerRouteRefusesGoneUnknownAndAdapterlessSessions(t *testing.T) {
	f := startPageFixture(t)
	id, _ := f.send(t, `{"text":"one"}`)
	// The original name is gone once the steer rewrote it.
	if code, _, raw := postJSON(t, f.base+"/s/livesession/steer", f.base, `{"id":"`+id+`"}`); code != http.StatusOK {
		t.Fatalf("steer = %d: %s", code, raw)
	}
	if code, _, raw := postJSON(t, f.base+"/s/livesession/steer", f.base, `{"id":"`+id+`"}`); code != http.StatusConflict {
		t.Fatalf("steer of a rewritten entry = %d (%s), want 409", code, raw)
	}
	// A drained entry is refused: the session already received it.
	drainedID, _ := f.send(t, `{"text":"two"}`)
	if _, err := wsinbox.Drain(f.inbox("livesession")); err != nil {
		t.Fatal(err)
	}
	if code, _, raw := postJSON(t, f.base+"/s/livesession/steer", f.base, `{"id":"`+drainedID+`"}`); code != http.StatusConflict {
		t.Fatalf("steer of a drained entry = %d (%s), want 409", code, raw)
	}
	// A well-formed id the inbox never held is gone too; a path is malformed.
	if code, _, _ := postJSON(t, f.base+"/s/livesession/steer", f.base, `{"id":"20260101T000000.000000000Z-0badc0de"}`); code != http.StatusConflict {
		t.Fatalf("steer of an unknown id = %d, want 409", code)
	}
	if code, _, _ := postJSON(t, f.base+"/s/livesession/steer", f.base, `{"id":"../../registration"}`); code != http.StatusBadRequest {
		t.Fatalf("steer of a path = %d, want 400", code)
	}
	if code, _, _ := postJSON(t, f.base+"/s/livesession/steer", f.base, `not json`); code != http.StatusBadRequest {
		t.Fatalf("malformed steer = %d, want 400", code)
	}
	if code, _, _ := postJSON(t, f.base+"/s/boardsession/steer", f.base, `{"id":"`+id+`"}`); code != http.StatusConflict {
		t.Fatalf("steer for a session without an adapter = %d, want 409", code)
	}
	if code, _, _ := postJSON(t, f.base+"/s/nosuchsession/steer", f.base, `{"id":"`+id+`"}`); code != http.StatusNotFound {
		t.Fatalf("steer for an unknown session = %d, want 404", code)
	}
}

// interruptAnswer is the interrupt route's answer.
type interruptAnswer struct {
	Held []struct {
		ID     string         `json:"id"`
		Text   string         `json:"text"`
		Chips  []wsinbox.Chip `json:"chips"`
		Images []string       `json:"images"`
		Nums   []int          `json:"nums"`
	} `json:"held"`
}

func TestInterruptRouteTakesBackEveryHeldMessageAndWritesTheEntry(t *testing.T) {
	f := startPageFixture(t)
	first, _ := f.send(t, `{"text":"promote me","chips":[{"id":"03","answer":"a"}]}`)
	code, _, raw := postJSON(t, f.base+"/s/livesession/steer", f.base, `{"id":"`+first+`"}`)
	if code != http.StatusOK {
		t.Fatalf("steer = %d: %s", code, raw)
	}
	steered := f.held(t)[0].ID()
	second, _ := f.send(t, `{"text":"see [Image #1]","images":`+imagesJSON("a.png", pngBytes)+`}`)
	merged, ok := f.send(t, `{"text":"and more"}`)
	if !ok {
		t.Fatal("follow-up did not merge into the held message")
	}
	paths := f.held(t)[1].Entry.Images
	before := time.Now().UnixMilli()

	code, header, raw := postJSON(t, f.base+"/s/livesession/interrupt", f.base, ``)
	if code != http.StatusOK {
		t.Fatalf("interrupt = %d: %s", code, raw)
	}
	assertNoCORS(t, header)
	var ans interruptAnswer
	if err := json.Unmarshal([]byte(raw), &ans); err != nil {
		t.Fatalf("interrupt answer %q: %v", raw, err)
	}
	if len(ans.Held) != 2 {
		t.Fatalf("interrupt answer = %s, want both held messages", raw)
	}
	a, b := ans.Held[0], ans.Held[1]
	if a.ID != steered || a.Text != "promote me" || len(a.Chips) != 1 || a.Chips[0].ID != "03" || a.Chips[0].Answer != "a" || len(a.Images) != 0 || a.Nums == nil {
		t.Errorf("first held = %+v", a)
	}
	if b.ID != merged || b.ID == second || b.Text != "see [Image #1]\n\nand more" || b.Chips == nil || len(b.Chips) != 0 ||
		len(b.Images) != 1 || b.Images[0] != filepath.Base(paths[0]) || len(b.Nums) != 1 || b.Nums[0] != 1 {
		t.Errorf("second held = %+v", b)
	}
	// Nothing is held any more; the interrupt entry is on disk, and the image
	// stays for a re-send to reference.
	if held := f.held(t); len(held) != 0 {
		t.Fatalf("held after interrupt = %+v", held)
	}
	inbox := f.inbox("livesession")
	names, _ := wsinbox.InterruptNames(inbox)
	if len(names) != 1 {
		t.Fatalf("interrupt entries = %v, want one", names)
	}
	records, err := wsinbox.DrainInterrupt(inbox)
	if err != nil || len(records) != 1 || !records[0].Entry.Interrupt || records[0].Entry.SentAt < before {
		t.Fatalf("interrupt entry = %+v, %v", records, err)
	}
	if got := f.attachments(t); len(got) != 1 {
		t.Fatalf("attachments after interrupt = %v", got)
	}

	// With nothing held the answer is an empty list, and a body is ignored.
	code, _, raw = postJSON(t, f.base+"/s/livesession/interrupt", f.base, `{}`)
	if code != http.StatusOK || strings.TrimSpace(raw) != `{"held":[]}` {
		t.Fatalf("interrupt with nothing held = %d: %s", code, raw)
	}
	// The page renders no bubble for the interrupt entry.
	_, page := get(t, f.base+"/s/livesession")
	if strings.Contains(page, `class="msg user held"`) {
		t.Errorf("page renders a held bubble for an interrupt entry")
	}
}

func TestInterruptEntryIsRefusedByCancelAndSteerAndNeedsAnAdapter(t *testing.T) {
	f := startPageFixture(t)
	if code, _, raw := postJSON(t, f.base+"/s/livesession/interrupt", f.base, ``); code != http.StatusOK {
		t.Fatalf("interrupt = %d: %s", code, raw)
	}
	names, _ := wsinbox.InterruptNames(f.inbox("livesession"))
	if len(names) != 1 {
		t.Fatalf("interrupt entries = %v", names)
	}
	id := wsinbox.Record{Name: names[0]}.ID()
	for _, route := range []string{"cancel", "steer"} {
		if code, _, raw := postJSON(t, f.base+"/s/livesession/"+route, f.base, `{"id":"`+id+`"}`); code != http.StatusConflict {
			t.Errorf("%s of the interrupt entry = %d (%s), want 409", route, code, raw)
		}
	}
	if got, _ := wsinbox.InterruptNames(f.inbox("livesession")); len(got) != 1 || got[0] != names[0] {
		t.Fatalf("interrupt entries after refused cancel and steer = %v", got)
	}
	// A send after it starts a new message rather than merging into it.
	if _, merged := f.send(t, `{"text":"after"}`); merged {
		t.Fatal("a send merged into the interrupt entry")
	}
	if code, _, _ := postJSON(t, f.base+"/s/boardsession/interrupt", f.base, ``); code != http.StatusConflict {
		t.Errorf("interrupt for a session without an adapter = %d, want 409", code)
	}
	if names, _ := wsinbox.Names(f.inbox("boardsession")); len(names) != 0 {
		t.Fatalf("refused interrupt wrote %v", names)
	}
	if code, _, _ := postJSON(t, f.base+"/s/nosuchsession/interrupt", f.base, ``); code != http.StatusNotFound {
		t.Errorf("interrupt for an unknown session = %d, want 404", code)
	}
	if code, _ := get(t, f.base+"/s/livesession/interrupt"); code != http.StatusMethodNotAllowed {
		t.Errorf("GET interrupt = %d, want 405", code)
	}
}

func TestWriteRoutesRequireTheDaemonsOwnOrigin(t *testing.T) {
	f := startPageFixture(t)
	for _, route := range []string{"message", "cancel", "steer", "interrupt"} {
		for _, origin := range []string{"", "null", "http://evil.example", "https://" + strings.TrimPrefix(f.base, "http://"), "http://localhost:1"} {
			code, header, _ := postJSON(t, f.base+"/s/livesession/"+route, origin, `{"text":"x","id":"x"}`)
			if code != http.StatusForbidden {
				t.Errorf("%s with Origin %q = %d, want 403", route, origin, code)
			}
			assertNoCORS(t, header)
		}
	}
	if names, _ := wsinbox.Names(f.inbox("livesession")); len(names) != 0 {
		t.Fatalf("a refused request wrote %v", names)
	}
	// A same-origin send passes and still carries no CORS header.
	code, header, raw := postJSON(t, f.base+"/s/livesession/message", f.base, `{"text":"ok"}`)
	if code != http.StatusOK {
		t.Fatalf("same-origin send = %d: %s", code, raw)
	}
	assertNoCORS(t, header)
	// A preflight is not answered with CORS either.
	req, _ := http.NewRequest(http.MethodOptions, f.base+"/s/livesession/message", nil)
	req.Header.Set("Origin", "http://evil.example")
	req.Header.Set("Access-Control-Request-Method", "POST")
	resp, err := testClient().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("preflight = %d, want 405", resp.StatusCode)
	}
	assertNoCORS(t, resp.Header)
	// GET on a write route is not a page.
	if code, _ := get(t, f.base+"/s/livesession/message"); code != http.StatusMethodNotAllowed {
		t.Errorf("GET message = %d, want 405", code)
	}
}

func assertNoCORS(t *testing.T, h http.Header) {
	t.Helper()
	for name := range h {
		if strings.HasPrefix(strings.ToLower(name), "access-control-") {
			t.Errorf("response carries CORS header %s", name)
		}
	}
}

func TestSessionWithoutAdapterHasNoComposerAndItsMessageRouteRefuses(t *testing.T) {
	f := startPageFixture(t)
	_, page := get(t, f.base+"/s/boardsession")
	for _, absent := range []string{`id="csend"`, `id="cchips"`, `id="ctext"`, `class="sentdata"`} {
		if strings.Contains(page, absent) {
			t.Errorf("board-first page carries %q", absent)
		}
	}
	code, _, _ := postJSON(t, f.base+"/s/boardsession/message", f.base, `{"text":"hello"}`)
	if code != http.StatusConflict {
		t.Fatalf("message to a session without an adapter = %d, want 409", code)
	}
	if names, _ := wsinbox.Names(f.inbox("boardsession")); len(names) != 0 {
		t.Fatalf("refused message was held: %v", names)
	}
	if code, _, _ := postJSON(t, f.base+"/s/nosuchsession/message", f.base, `{"text":"hello"}`); code != http.StatusNotFound {
		t.Fatalf("message to an unknown session = %d, want 404", code)
	}
}

func TestStreamSendsTheConversationAfterASend(t *testing.T) {
	f := startPageFixture(t)
	_, page := get(t, f.base+"/s/livesession")
	client := &http.Client{Transport: &http.Transport{Proxy: nil}}
	resp, err := client.Get(f.base + "/s/livesession/events?h=" + heldOf(page) + ",mode." + newRegion(modeLive).Hash)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	events := make(chan sseEvent, 16)
	go readEvents(t, bufio.NewReader(resp.Body), events)
	// Let the stream settle on the current page first.
	for {
		if _, ok := nextRegion(t, events, 400*time.Millisecond); !ok {
			break
		}
	}
	id, _ := f.send(t, `{"text":"held now"}`)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		ev, ok := nextRegion(t, events, time.Second)
		if ok && ev.Name == regionConvLive && strings.Contains(ev.HTML, `data-entry="`+id+`"`) {
			return
		}
	}
	t.Fatal("no conversation region with the held bubble after a send")
}
