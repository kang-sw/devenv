package wsdashboard

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

// ReadClosedTopics renders the closed topics of the fake's board for the
// session, as the serving ws-mcp renders them.
func (p *pageFake) ReadClosedTopics(reg Registration, lang string, from, n int) (template.HTML, bool) {
	p.mu.Lock()
	st := p.boards[reg.ReplyID]
	p.mu.Unlock()
	if st == nil {
		return "", false
	}
	view := wswhiteboard.BuildView(st, wswhiteboard.ViewInput{Layout: wswhiteboard.LayoutFull, ReplyID: reg.ReplyID, Closed: &wswhiteboard.ClosedWindow{From: from, N: n}})
	view.Lang = lang
	html, err := wswhiteboard.RenderClosedTopics(view)
	return html, err == nil
}

// closedTopicsBoard holds n closed topics c1..cn, closed in that order.
func closedTopicsBoard(n int) *wswhiteboard.State {
	st := &wswhiteboard.State{Seq: 1}
	for i := 1; i <= n; i++ {
		topic := fmt.Sprintf("c%d", i)
		st.Topics = append(st.Topics, wswhiteboard.TopicDoc{Name: topic, Body: "body of " + topic, Changed: 1})
		st.ClosedTopics = append(st.ClosedTopics, topic)
	}
	return st
}

func getClosed(t *testing.T, url string) string {
	t.Helper()
	code, body := get(t, url)
	if code != http.StatusOK {
		t.Fatalf("GET %s = %d: %s", url, code, body)
	}
	var ans struct {
		HTML string `json:"html"`
	}
	if err := json.Unmarshal([]byte(body), &ans); err != nil {
		t.Fatalf("closed answer %q: %v", body, err)
	}
	return ans.HTML
}

// TestClosedRouteServesTheTopicsPastTheCap pins the closed route: from and n
// select closed topics in close order, the most recently closed first, each
// rendered as the board renders it (its body in a sandboxed frame), with the
// next show-more control while more remain.
func TestClosedRouteServesTheTopicsPastTheCap(t *testing.T) {
	f := startPageFixture(t)
	f.src.mu.Lock()
	f.src.boards["boardsession"] = closedTopicsBoard(35)
	f.src.mu.Unlock()

	rest := getClosed(t, f.base+"/s/boardsession/closed?from=30")
	if strings.Count(rest, `<section class="topic`) != 5 || strings.Contains(rest, "data-closed-next") {
		t.Fatalf("the rest of the closed topics:\n%s", rest)
	}
	if first, last := strings.Index(rest, `>c5</span>`), strings.Index(rest, `>c1</span>`); first < 0 || last < first {
		t.Errorf("closed topics out of close order: c5 at %d, c1 at %d", first, last)
	}
	if !strings.Contains(rest, `<div class="doc-frame" title="c5"`) {
		t.Errorf("a closed topic's body is not framed:\n%.600s", rest)
	}
	page := getClosed(t, f.base+"/s/boardsession/closed?from=30&n=2")
	if strings.Count(page, `<section class="topic`) != 2 || !strings.Contains(page, `<button class="btn closed-more" data-closed-next="32" data-closed-more="3">`) {
		t.Errorf("a two-topic page:\n%s", page)
	}
}

// TestClosedRouteRefusesBadRequests pins the route's input rules: from is a
// positive index and n a positive count; a session the daemon does not
// serve, or one without a board, is not found; and it answers reads only.
func TestClosedRouteRefusesBadRequests(t *testing.T) {
	f := startPageFixture(t)
	f.src.mu.Lock()
	f.src.boards["boardsession"] = closedTopicsBoard(35)
	f.src.mu.Unlock()
	for _, q := range []string{"", "from=0", "from=-1", "from=x", "from=30&n=0", "from=30&n=lots"} {
		if code, body := get(t, f.base+"/s/boardsession/closed?"+q); code != http.StatusBadRequest {
			t.Errorf("closed?%s = %d (%s), want 400", q, code, body)
		}
	}
	for _, id := range []string{"nosuchsession", "livesession"} {
		if code, _ := get(t, f.base+"/s/"+id+"/closed?from=30"); code != http.StatusNotFound {
			t.Errorf("closed topics of %s = %d, want 404", id, code)
		}
	}
	resp, err := testClient().Post(f.base+"/s/boardsession/closed?from=30", "text/plain", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("POST closed = %d", resp.StatusCode)
	}
}
