package wsdashboard

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
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
	if !strings.Contains(page, `<i data-id="03" data-answer="b" data-note="" data-state="sent">`) || strings.Contains(page, `data-entry=`) {
		t.Errorf("after the drain the page lacks the sent mark or still shows a held bubble")
	}
}

func TestWriteRoutesRequireTheDaemonsOwnOrigin(t *testing.T) {
	f := startPageFixture(t)
	for _, route := range []string{"message", "cancel"} {
		for _, origin := range []string{"", "null", "http://evil.example", "https://" + strings.TrimPrefix(f.base, "http://"), "http://localhost:1"} {
			code, header, _ := postJSON(t, f.base+"/s/livesession/"+route, origin, `{"text":"x","id":"x"}`)
			if code != http.StatusForbidden {
				t.Errorf("%s with Origin %q = %d, want 403", route, origin, code)
			}
			assertNoCORS(t, header)
		}
	}
	if held := f.held(t); len(held) != 0 {
		t.Fatalf("a refused request held %+v", held)
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
		if ok && ev.Name == regionConversation && strings.Contains(ev.HTML, `data-entry="`+id+`"`) {
			return
		}
	}
	t.Fatal("no conversation region with the held bubble after a send")
}
