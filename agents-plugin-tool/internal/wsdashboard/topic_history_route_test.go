package wsdashboard

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

// ReadTopicHistory renders the fake board's named topics, or the topic
// holding target, in full, as the serving ws-mcp renders them.
func (p *pageFake) ReadTopicHistory(reg Registration, lang string, names []string, target string) ([]wswhiteboard.TopicFragment, bool) {
	p.mu.Lock()
	st := p.boards[reg.ReplyID]
	p.mu.Unlock()
	if st == nil {
		return nil, false
	}
	if target != "" {
		name, ok := st.TargetTopic(target)
		if !ok {
			return nil, true
		}
		names = []string{name}
	}
	view := wswhiteboard.BuildView(st, wswhiteboard.ViewInput{Layout: wswhiteboard.LayoutFull, ReplyID: reg.ReplyID, LazyHistory: true})
	view.Lang = lang
	frags, err := wswhiteboard.RenderTopicFragments(view, names)
	return frags, err == nil
}

// TestTopicHistoryRoute pins the lazy-history route: name (repeatable) or
// target selects topics, each answered as its complete framed section under
// its name, theme-pinned like the board; bad input is refused, an unknown
// session is not found, and it answers reads only.
func TestTopicHistoryRoute(t *testing.T) {
	f := startPageFixture(t)
	st := closedTopicsBoard(35)
	st.Topics[0].Entries = []wswhiteboard.Entry{{ID: "e1", Kind: wswhiteboard.EntryAgent, Text: "c1 line", At: "2026-11-17T00:00:00Z"}}
	f.src.mu.Lock()
	f.src.boards["boardsession"] = st
	f.src.mu.Unlock()

	read := func(query string) []map[string]string {
		t.Helper()
		code, body := get(t, f.base+"/s/boardsession/topic?"+query)
		if code != http.StatusOK {
			t.Fatalf("topic?%s = %d: %s", query, code, body)
		}
		var ans struct {
			Topics []map[string]string `json:"topics"`
		}
		if err := json.Unmarshal([]byte(body), &ans); err != nil || ans.Topics == nil {
			t.Fatalf("topic answer %q: %v", body, err)
		}
		return ans.Topics
	}
	got := read("name=c1&name=c2&name=nosuch")
	if len(got) != 2 || got[0]["name"] != "c1" || got[1]["name"] != "c2" || !strings.HasPrefix(got[0]["html"], `<section class="topic`) || !strings.Contains(got[0]["html"], "c1 line") {
		t.Fatalf("named topics = %v", got)
	}
	if got := read("target=e1"); len(got) != 1 || got[0]["name"] != "c1" {
		t.Errorf("target=e1 = %v", got)
	}
	if got := read("target=e999"); len(got) != 0 {
		t.Errorf("an unknown target = %v, want none", got)
	}
	for _, q := range []string{"", "target=x1", "target=topic-ab", "name=c1&target=e1"} {
		if code, body := get(t, f.base+"/s/boardsession/topic?"+q); code != http.StatusBadRequest {
			t.Errorf("topic?%s = %d (%s), want 400", q, code, body)
		}
	}
	for _, id := range []string{"nosuchsession", "livesession"} {
		if code, _ := get(t, f.base+"/s/"+id+"/topic?name=c1"); code != http.StatusNotFound {
			t.Errorf("topic history of %s = %d, want 404", id, code)
		}
	}
	resp, err := testClient().Post(f.base+"/s/boardsession/topic?name=c1", "text/plain", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("POST topic = %d", resp.StatusCode)
	}
}
