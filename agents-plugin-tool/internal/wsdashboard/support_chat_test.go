package wsdashboard

import (
	"bufio"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
)

// TestCardEntryRendersTheItemReference pins a card line's markup: the
// store and alias the tracker drawer's chat finds the item by, and the
// title a page without the store's view shows.
func TestCardEntryRendersTheItemReference(t *testing.T) {
	ring := []wslive.RingRecord{
		ringRecord(1, wslive.RingEntry{Kind: wslive.RingReply, Text: "This one first:"}),
		ringRecord(2, wslive.RingEntry{Kind: wslive.RingCard, Card: &wslive.CardRef{Store: "live", Alias: "ada.c89-3", Title: "Fix <the> ribbon"}}),
	}
	set, err := renderSessionRegions(historyView(ring, "", PageContent{}, sessionLookup{}), newEntryCache())
	if err != nil {
		t.Fatal(err)
	}
	html := string(set[regionEntries].HTML)
	for _, want := range []string{
		`<div class="cardref" data-entry="` + ring[1].Name + `" data-tk-store="live" data-tk-alias="ada.c89-3"`,
		`<b class="cra">ada.c89-3</b>`,
		`<span class="crt">Fix &lt;the&gt; ribbon</span>`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("card entry lacks %q:\n%s", want, html)
		}
	}
	if strings.Index(html, ring[0].Name) > strings.Index(html, ring[1].Name) {
		t.Errorf("card entry is not after the reply:\n%s", html)
	}
}

// TestStreamSendsOnlyTheRegionsItNames pins ?regions=: the stream sends the
// named regions and nothing else, not even the mode pseudo-region.
func TestStreamSendsOnlyTheRegionsItNames(t *testing.T) {
	f := startPageFixture(t)
	client := &http.Client{Transport: &http.Transport{Proxy: nil}}
	resp, err := client.Get(f.base + "/s/livesession/events?regions=entries,convlive")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	events := make(chan sseEvent, 16)
	go readEvents(t, bufio.NewReader(resp.Body), events)
	got := map[string]bool{}
	for {
		ev, ok := nextRegion(t, events, 600*time.Millisecond)
		if !ok {
			break
		}
		got[ev.Name] = true
	}
	if len(got) != 2 || !got[regionEntries] || !got[regionConvLive] {
		t.Fatalf("regions sent = %v, want only %s and %s", got, regionEntries, regionConvLive)
	}
}

// TestStoresRouteReportsTheSupportChats pins the stores answer's support
// flag: true exactly when the daemon serves the store chats.
func TestStoresRouteReportsTheSupportChats(t *testing.T) {
	support := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	d := startIndexedDaemon(t, t.TempDir(), newFakeSource(), time.Hour, withTracker(newFakeTracker()), func(o *DaemonOptions) { o.Support = support })
	code, body := call(d, http.MethodGet, "/tracker/stores", "")
	var got struct {
		Support bool `json:"support"`
	}
	if code != http.StatusOK || json.Unmarshal([]byte(body), &got) != nil || !got.Support {
		t.Fatalf("stores with support = %d %s", code, body)
	}
}

// TestSupportChatCarriesTheSubagentBadge pins the chat card's script hooks:
// its composer carries the session composer's subagent badge (without the
// session page's id) and fills it from the convlive region its log streams.
func TestSupportChatCarriesTheSubagentBadge(t *testing.T) {
	f := startPageFixture(t)
	code, index := get(t, f.base+"/?frame=1")
	if code != http.StatusOK {
		t.Fatalf("GET /?frame=1 = %d", code)
	}
	for _, want := range []string{
		`'<div class="composer supcomp"><div class="cbadges"><span class="sbadge" title="' + supEsc(wsT("js.subagents.title")) + '" hidden>'`,
		`'<span class="spin"></span><svg class="i" aria-hidden="true"><use href="#i-agents"/></svg><span class="slabel"></span></span></div>`,
		`subBadge(c.sbadge, c.live.querySelector(".clive"), !!c.conv);`,
	} {
		if !strings.Contains(index, want) {
			t.Errorf("index page script lacks %q", want)
		}
	}
}

// TestSubagentBadgeShowsIconAndCount: the badge reads the count only, like
// the background-task badge beside it, and the word is its title and
// accessible name.
func TestSubagentBadgeShowsIconAndCount(t *testing.T) {
	d, err := dashboardFS.ReadFile("assets/dashboard.js")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`if (el && el.textContent !== count) el.textContent = count;`,
		`badge.setAttribute("aria-label", label);`,
	} {
		if !strings.Contains(string(d), want) {
			t.Errorf("subBadge lacks %q", want)
		}
	}
}
