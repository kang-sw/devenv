package mcp

import (
	"errors"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsdashboard"
)

func TestDashboardPageURLLoginLink(t *testing.T) {
	for _, test := range []struct {
		name string
		hook bool
		fail bool
		want string
		ok   bool
	}{
		{name: "standalone", want: "http://127.0.0.1:29417/s/reply", ok: true},
		{name: "hosted", hook: true, want: "http://127.0.0.1:29417/s/reply?login=once", ok: true},
		{name: "mint failed", hook: true, fail: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			opts := ServerOptions{Root: t.TempDir(), Env: []string{}}
			if test.hook {
				opts.LoginLink = func(path string) (string, error) {
					calls++
					if path != "/s/reply" {
						t.Fatalf("login path = %q", path)
					}
					if test.fail {
						return "", errors.New("mint failed")
					}
					return "http://127.0.0.1:29417" + path + "?login=once", nil
				}
			}
			s := NewServerWithOptions(opts)
			t.Cleanup(s.Close)
			s.dashboard.sessions = map[string]*dashboardSession{
				"key": {replyID: "reply", hosted: true, servedPort: 29417},
			}
			got, ok := s.dashboardPageURL(" key ")
			if got != test.want || ok != test.ok {
				t.Fatalf("dashboardPageURL = %q, %v; want %q, %v", got, ok, test.want, test.ok)
			}
			if test.hook && calls != 1 {
				t.Fatalf("hook calls = %d", calls)
			}
			// Missing, failed-nudge and unregistered sessions never mint codes.
			s.dashboard.sessions["no nudge"] = &dashboardSession{hosted: true}
			s.dashboard.sessions["unregistered"] = &dashboardSession{servedPort: 29417}
			before := calls
			for _, key := range []string{"missing", "no nudge", "unregistered"} {
				if got, ok := s.dashboardPageURL(key); got != "" || ok {
					t.Fatalf("%s page = %q, %v", key, got, ok)
				}
			}
			if calls != before {
				t.Fatal("unserved session minted a login code")
			}
		})
	}
}

func TestWhiteboardFullPointerLoginLink(t *testing.T) {
	f := newWhiteboardFixture(t)
	t.Cleanup(f.s.Close)
	f.s.dashboard.sessions = map[string]*dashboardSession{
		f.key: {replyID: f.replyID(), reg: wsdashboard.Registration{ReplyID: f.replyID()}, servedPort: 29417},
	}
	calls := 0
	f.s.loginLink = func(path string) (string, error) {
		// Hooks run outside the registry lock.
		f.s.dashboard.mu.Lock()
		f.s.dashboard.mu.Unlock()
		calls++
		if path != "/s/"+f.replyID() {
			t.Fatalf("login path = %q", path)
		}
		return "http://127.0.0.1:29417" + path + "?login=once", nil
	}
	got := f.update(t, "")
	want := "http://127.0.0.1:29417/s/" + f.replyID() + "?login=once"
	if full := resultLine(t, got, "full: "); full != want || calls != 1 {
		t.Fatalf("full = %q, calls = %d; want %q, 1", full, calls, want)
	}
	// Mint failure does not fail the committed update or expose a plain URL.
	f.s.loginLink = func(string) (string, error) { return "", errors.New("mint failed") }
	if got := resultLine(t, f.update(t, ""), "full: "); !strings.HasSuffix(got, "/index.html") || strings.HasPrefix(got, "http") {
		t.Fatalf("mint failure full = %q", got)
	}
}
