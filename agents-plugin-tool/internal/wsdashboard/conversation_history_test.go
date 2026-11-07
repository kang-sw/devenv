package wsdashboard

import (
	"bufio"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
)

// historyView builds a live session view whose ring holds the records and
// whose lead streams reply (empty for none).
func historyView(ring []wslive.RingRecord, reply string, page PageContent, lookup func(string) (Session, *wslive.Snapshot, bool)) SessionView {
	now := testNow
	lead := wslive.Lead{Activity: wslive.Activity{Phase: wslive.PhaseReplying}}
	if reply != "" {
		lead.Reply = &wslive.Reply{Text: reply}
	}
	return buildSessionView(pageInputs{Session: liveSession("self", "/proj", now), Snapshot: liveSnap(now, lead), Ring: ring, Now: now, Page: page, Lookup: lookup})
}

func ringRecord(i int, entry wslive.RingEntry) wslive.RingRecord {
	return wslive.RingRecord{Name: fmt.Sprintf("20261107T0900%02d.000000000Z-ab.json", i), At: testNow.Add(time.Duration(i) * time.Second), RingEntry: entry}
}

// TestStreamingTickChangesOnlyTheLiveRegion pins the region split: a
// streaming reply growing changes the convlive region alone, and a new
// finished message changes the entries region.
func TestStreamingTickChangesOnlyTheLiveRegion(t *testing.T) {
	ring := []wslive.RingRecord{
		ringRecord(1, wslive.RingEntry{Kind: wslive.RingUser, Origin: wslive.OriginTerminal, Text: "question"}),
		ringRecord(2, wslive.RingEntry{Kind: wslive.RingReply, Text: "**answer**"}),
	}
	cache := newEntryCache()
	render := func(ring []wslive.RingRecord, reply string) regionSet {
		t.Helper()
		set, err := renderSessionRegions(historyView(ring, reply, PageContent{}, nil), cache)
		if err != nil {
			t.Fatal(err)
		}
		return set
	}
	a, b := render(ring, "partial"), render(ring, "partial and more")
	if a[regionEntries].Hash != b[regionEntries].Hash {
		t.Error("a streaming tick changed the entries region")
	}
	if a[regionConvLive].Hash == b[regionConvLive].Hash {
		t.Error("a streaming tick left the convlive region unchanged")
	}
	if strings.Contains(string(b[regionEntries].HTML), "partial") || !strings.Contains(string(b[regionConvLive].HTML), "partial and more") {
		t.Errorf("streaming reply is not in convlive alone:\nentries %s\nconvlive %s", b[regionEntries].HTML, b[regionConvLive].HTML)
	}
	c := render(append(ring, ringRecord(3, wslive.RingEntry{Kind: wslive.RingUser, Origin: wslive.OriginTerminal, Text: "next"})), "partial and more")
	if c[regionEntries].Hash == b[regionEntries].Hash {
		t.Error("a new finished message left the entries region unchanged")
	}
	// Each entry is keyed by its ring entry name inside the entries
	// container.
	entries := string(c[regionEntries].HTML)
	for _, r := range ring {
		if !strings.Contains(entries, `data-entry="`+r.Name+`"`) {
			t.Errorf("entries region lacks the key %s: %s", r.Name, entries)
		}
	}
	if !strings.Contains(entries, `<div class="conv centries">`) {
		t.Errorf("entries region lacks its container: %s", entries)
	}
}

// TestEntryRendersOnceThroughTheCache pins the rendered-entry cache: renders
// of unchanged entries run markdown once per entry, and a change to another
// render input (a mail body leaving the history, a peer leaving the index)
// renders that entry anew.
func TestEntryRendersOnceThroughTheCache(t *testing.T) {
	ring := []wslive.RingRecord{
		ringRecord(1, wslive.RingEntry{Kind: wslive.RingReply, Text: "# one"}),
		ringRecord(2, wslive.RingEntry{Kind: wslive.RingReply, Text: "# two"}),
		ringRecord(3, wslive.RingEntry{Kind: wslive.RingMail, Direction: wslive.MailRecv, Peer: "id:peer", Title: "hi", HistoryKey: "h1"}),
	}
	served := map[string]Session{"peer": liveSession("peer", "/q", testNow)}
	page := PageContent{MailBodies: map[string]string{"h1": "hi\nbody"}}
	cache := newEntryCache()
	markdowns := 0
	render := func(page PageContent) regionSet {
		t.Helper()
		v := historyView(ring, "streaming", page, lookupOf(served, nil))
		inner := v.Conversation.markdown
		v.Conversation.markdown = func(s string, finished bool) template.HTML {
			if finished {
				markdowns++
			}
			return inner(s, finished)
		}
		set, err := renderSessionRegions(v, cache)
		if err != nil {
			t.Fatal(err)
		}
		return set
	}
	first := render(page)
	for i := 0; i < 3; i++ {
		if again := render(page); again[regionEntries].Hash != first[regionEntries].Hash {
			t.Fatal("a cached render differs")
		}
	}
	if markdowns != 2 || cache.buildCount() != 3 {
		t.Fatalf("markdown ran %d times and %d entries built, want 2 and 3", markdowns, cache.buildCount())
	}
	if !strings.Contains(string(first[regionEntries].HTML), "<h1>one</h1>") || !strings.Contains(string(first[regionEntries].HTML), `href="/s/peer"`) {
		t.Errorf("cached entries lack their markdown or peer link: %s", first[regionEntries].HTML)
	}

	gone := render(PageContent{})
	if cache.buildCount() != 4 || markdowns != 2 {
		t.Fatalf("after the body left: %d builds, %d markdown runs, want 4 and 2", cache.buildCount(), markdowns)
	}
	if !strings.Contains(string(gone[regionEntries].HTML), "nobody") {
		t.Errorf("the mail line kept its body: %s", gone[regionEntries].HTML)
	}
	delete(served, "peer")
	unlinked := render(PageContent{})
	if cache.buildCount() != 5 || strings.Contains(string(unlinked[regionEntries].HTML), `href="/s/peer"`) {
		t.Fatalf("after the peer left: %d builds, html %s", cache.buildCount(), unlinked[regionEntries].HTML)
	}
}

// TestEntryCacheIsBoundedPerSession pins the cache bound: a session keeps at
// most entryCacheCap entries, evicting the least recently used, and drop
// forgets the session.
func TestEntryCacheIsBoundedPerSession(t *testing.T) {
	cache := newEntryCache()
	build := func(i int) func() (template.HTML, error) {
		return func() (template.HTML, error) { return template.HTML(fmt.Sprint(i)), nil }
	}
	entry := func(i int) ConvEntry { return ConvEntry{Key: fmt.Sprintf("e%04d", i)} }
	for i := 0; i < entryCacheCap; i++ {
		if _, err := cache.render("s", entry(i), build(i)); err != nil {
			t.Fatal(err)
		}
	}
	// Touch the oldest, then overflow by one: the second oldest goes.
	_, _ = cache.render("s", entry(0), build(0))
	_, _ = cache.render("s", entry(entryCacheCap), build(entryCacheCap))
	_, _ = cache.render("other", entry(0), build(0))
	if cache.size("s") != entryCacheCap || cache.size("other") != 1 {
		t.Fatalf("sizes = %d, %d", cache.size("s"), cache.size("other"))
	}
	builds := cache.buildCount()
	_, _ = cache.render("s", entry(0), build(0))
	if cache.buildCount() != builds {
		t.Error("the recently used oldest entry was evicted")
	}
	_, _ = cache.render("s", entry(1), build(1))
	if cache.buildCount() != builds+1 {
		t.Error("the least recently used entry was not evicted")
	}
	cache.drop("s")
	if cache.size("s") != 0 || cache.size("other") != 1 {
		t.Fatalf("after drop: sizes = %d, %d", cache.size("s"), cache.size("other"))
	}
}

// appendUsers appends n terminal user entries to a fixture session's ring.
func appendUsers(t *testing.T, f *pageFixture, id string, from, n int) {
	t.Helper()
	entries := make([]wslive.RingEntry, 0, n)
	for i := from; i < from+n; i++ {
		entries = append(entries, wslive.RingEntry{Kind: wslive.RingUser, Origin: wslive.OriginTerminal, Text: fmt.Sprintf("user message %d", i)})
	}
	if err := wslive.AppendRing(wslive.RingDir(SessionDir(f.dir, id)), entries); err != nil {
		t.Fatal(err)
	}
}

func getHistory(t *testing.T, url string) historyAnswer {
	t.Helper()
	code, body := get(t, url)
	if code != http.StatusOK {
		t.Fatalf("GET %s = %d: %s", url, code, body)
	}
	var ans historyAnswer
	if err := json.Unmarshal([]byte(body), &ans); err != nil {
		t.Fatalf("history answer %q: %v", body, err)
	}
	return ans
}

// TestHistoryRoutePagesByCursor pins the history route: no cursor is the
// newest page, before is exclusive and walks to older pages, the oldest page
// sets end, and entries render as the live view renders them.
func TestHistoryRoutePagesByCursor(t *testing.T) {
	f := startPageFixture(t)
	// The fixture ring's 4 entries, then 57 more: 61 in all.
	appendUsers(t, f, "livesession", 0, wslive.RingCap+7)
	names, err := wslive.RingNames(wslive.RingDir(SessionDir(f.dir, "livesession")))
	if err != nil || len(names) != wslive.RingCap+11 {
		t.Fatalf("ring names = %d, %v", len(names), err)
	}

	newest := getHistory(t, f.base+"/s/livesession/history")
	if newest.End || newest.Before != names[11] || strings.Count(newest.HTML, "data-entry=") != wslive.RingCap {
		t.Fatalf("newest page: end %v before %q, %d entries", newest.End, newest.Before, strings.Count(newest.HTML, "data-entry="))
	}
	if !strings.Contains(newest.HTML, `data-entry="`+names[len(names)-1]+`"`) || strings.Contains(newest.HTML, `data-entry="`+names[10]+`"`) {
		t.Errorf("newest page does not hold the newest %d entries", wslive.RingCap)
	}
	// The live page draws the same newest entries.
	_, page := get(t, f.base+"/s/livesession")
	if !strings.Contains(page, `data-entry="`+names[11]+`"`) || strings.Contains(page, `data-entry="`+names[10]+`"`) {
		t.Errorf("the live page does not draw exactly the newest %d entries", wslive.RingCap)
	}

	older := getHistory(t, f.base+"/s/livesession/history?n=6&before="+newest.Before)
	if older.End || older.Before != names[5] || strings.Count(older.HTML, "data-entry=") != 6 || strings.Contains(older.HTML, `data-entry="`+names[11]+`"`) {
		t.Fatalf("older page: end %v before %q, html %s", older.End, older.Before, older.HTML)
	}
	oldest := getHistory(t, f.base+"/s/livesession/history?n=6&before="+older.Before)
	if !oldest.End || oldest.Before != names[0] || strings.Count(oldest.HTML, "data-entry=") != 5 {
		t.Fatalf("oldest page: end %v before %q, html %s", oldest.End, oldest.Before, oldest.HTML)
	}
	// The fixture's own entries render as the live view renders them: the
	// mail whose body is in the history expands, and its served peer links.
	for _, want := range []string{"from the board", "mail second line", `href="/s/boardsession"`, `<div class="mailline sent nobody"`} {
		if !strings.Contains(oldest.HTML, want) {
			t.Errorf("oldest page lacks %q: %s", want, oldest.HTML)
		}
	}
	if past := getHistory(t, f.base+"/s/livesession/history?before="+oldest.Before); !past.End || past.HTML != "" || past.Before != "" {
		t.Fatalf("page past the oldest = %+v", past)
	}
}

// TestHistoryRouteRefusesBadRequests pins the route's input rules: before is
// an entry name, never a path; n is a positive count; an unknown session is
// not found; and it answers reads only.
func TestHistoryRouteRefusesBadRequests(t *testing.T) {
	f := startPageFixture(t)
	for _, q := range []string{"before=..%2F..%2Fsnapshot.json", "before=nostamp.json", "before=x", "n=0", "n=-3", "n=lots"} {
		if code, body := get(t, f.base+"/s/livesession/history?"+q); code != http.StatusBadRequest {
			t.Errorf("history?%s = %d (%s), want 400", q, code, body)
		}
	}
	if code, _ := get(t, f.base+"/s/nosuchsession/history"); code != http.StatusNotFound {
		t.Errorf("history of an unknown session = %d, want 404", code)
	}
	resp, err := testClient().Post(f.base+"/s/livesession/history", "text/plain", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("POST history = %d", resp.StatusCode)
	}
}

// TestHistoryRouteReadsWithoutAnAdapter: the route reads the ring whenever
// the session page is served, so a board-first session's history loads.
func TestHistoryRouteReadsWithoutAnAdapter(t *testing.T) {
	f := startPageFixture(t)
	appendUsers(t, f, "boardsession", 0, 2)
	ans := getHistory(t, f.base+"/s/boardsession/history")
	if !ans.End || strings.Count(ans.HTML, "data-entry=") != 2 || !strings.Contains(ans.HTML, "user message 1") {
		t.Fatalf("board-first history = %+v", ans)
	}
}

// TestSessionLeavingTheIndexDropsItsCachedEntries pins the cache's lifetime:
// entries cached by a page render are dropped when the session leaves the
// index.
func TestSessionLeavingTheIndexDropsItsCachedEntries(t *testing.T) {
	f := startPageFixture(t)
	if code, _ := get(t, f.base+"/s/livesession"); code != http.StatusOK {
		t.Fatal("page not served")
	}
	if n := f.d.entries.size("livesession"); n != 4 {
		t.Fatalf("cached entries = %d, want the fixture's 4", n)
	}
	if err := os.RemoveAll(SessionDir(f.dir, "livesession")); err != nil {
		t.Fatal(err)
	}
	f.d.index.Refresh()
	if n := f.d.entries.size("livesession"); n != 0 {
		t.Fatalf("cached entries after the session left = %d", n)
	}
}

// TestStreamSendsOnlyTheLiveRegionForAStreamingTick: over SSE, a streamed
// reply growing resends the small convlive region and nothing else.
func TestStreamSendsOnlyTheLiveRegionForAStreamingTick(t *testing.T) {
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
	if ev, ok := nextRegion(t, events, 400*time.Millisecond); ok {
		t.Fatalf("a current page received %+v", ev)
	}

	snap := &wslive.Snapshot{SchemaVersion: 1, Adapter: "pi", UpdatedAt: time.Now().UnixMilli(), HeartbeatMs: 30000,
		Lead: wslive.Lead{Activity: wslive.Activity{Phase: wslive.PhaseReplying}, Reply: &wslive.Reply{Text: "streamed <partial> reply, grown"}}}
	writeSnapshot(t, f.dir, "livesession", snap)
	f.d.broker.wake("livesession")
	var got []string
	for {
		ev, ok := nextRegion(t, events, 800*time.Millisecond)
		if !ok {
			break
		}
		got = append(got, ev.Name)
	}
	if strings.Join(got, ",") != regionConvLive {
		t.Fatalf("regions sent for a streaming tick = %v, want only %s", got, regionConvLive)
	}

	// A finished message changes the entries region.
	appendUsers(t, f, "livesession", 0, 1)
	f.d.broker.wake("livesession")
	ev, ok := nextRegion(t, events, 2*time.Second)
	if !ok || ev.Name != regionEntries || !strings.Contains(ev.HTML, "user message 0") {
		t.Fatalf("after a new message got %+v (%v)", ev, ok)
	}
}

// TestEntryCacheRendersARemovedAttachmentAsText: a user line's thumbnail is
// part of its cache key, so once the attachment file is gone the entry
// renders anew with the path as text, not a cached broken image.
func TestEntryCacheRendersARemovedAttachmentAsText(t *testing.T) {
	attachDir := t.TempDir()
	img := attachDir + "/0123456789abcdef-1.png"
	if err := os.WriteFile(img, []byte("png"), 0o600); err != nil {
		t.Fatal(err)
	}
	ring := []wslive.RingRecord{ringRecord(1, wslive.RingEntry{Kind: wslive.RingUser, Origin: wslive.OriginWhiteboard, Text: "look\n\n[image: " + img + "]"})}
	cache := newEntryCache()
	render := func() string {
		t.Helper()
		v := buildSessionView(pageInputs{Session: liveSession("self", "/proj", testNow), Snapshot: liveSnap(testNow, wslive.Lead{}), Ring: ring, Now: testNow, AttachDir: attachDir})
		set, err := renderSessionRegions(v, cache)
		if err != nil {
			t.Fatal(err)
		}
		return string(set[regionEntries].HTML)
	}
	if html := render(); !strings.Contains(html, `src="/s/self/attachment/0123456789abcdef-1.png"`) {
		t.Fatalf("existing attachment has no thumbnail: %s", html)
	}
	render()
	if cache.buildCount() != 1 {
		t.Fatalf("builds = %d, want 1", cache.buildCount())
	}
	if err := os.Remove(img); err != nil {
		t.Fatal(err)
	}
	html := render()
	if cache.buildCount() != 2 || strings.Contains(html, "/attachment/") || !strings.Contains(html, "[image: "+img+"]") {
		t.Fatalf("after removal: %d builds, html %s", cache.buildCount(), html)
	}
}
