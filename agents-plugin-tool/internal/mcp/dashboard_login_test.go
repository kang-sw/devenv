package mcp

import (
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsdashboard"
)

func TestDashboardPageURLPlain(t *testing.T) {
	s := NewServerWithOptions(ServerOptions{Root: t.TempDir(), Env: []string{}})
	t.Cleanup(s.Close)
	s.dashboard.sessions = map[string]*dashboardSession{
		"key":          {replyID: "reply", hosted: true, servedPort: 29417},
		"no nudge":     {hosted: true},
		"unregistered": {servedPort: 29417},
	}
	if got, ok := s.dashboardPageURL(" key "); got != "http://127.0.0.1:29417/s/reply" || !ok {
		t.Fatalf("dashboardPageURL = %q, %v", got, ok)
	}
	for _, key := range []string{"missing", "no nudge", "unregistered"} {
		if got, ok := s.dashboardPageURL(key); got != "" || ok {
			t.Fatalf("%s page = %q, %v", key, got, ok)
		}
	}
}

// Tool results land in transcripts, so the served page URL carries no
// login code; a cookieless browser gets the daemon's login gate instead.
func TestWhiteboardFullPointerHasNoLoginCode(t *testing.T) {
	f := newWhiteboardFixture(t)
	t.Cleanup(f.s.Close)
	f.s.dashboard.sessions = map[string]*dashboardSession{
		f.key: {replyID: f.replyID(), reg: wsdashboard.Registration{ReplyID: f.replyID()}, servedPort: 29417},
	}
	got := f.update(t, "")
	if full, want := resultLine(t, got, "full: "), "http://127.0.0.1:29417/s/"+f.replyID(); full != want {
		t.Fatalf("full = %q, want %q", full, want)
	}
	if strings.Contains(got, "login=") {
		t.Fatalf("whiteboard.update output carries a login code:\n%s", got)
	}
}
