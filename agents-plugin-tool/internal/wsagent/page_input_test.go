package wsagent

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wsinbox"
)

func TestHostedPageInputDoesNotWaitForLoop(t *testing.T) {
	for _, hidden := range []bool{false, true} {
		name := "visible"
		if hidden {
			name = "hidden"
		}
		t.Run(name, func(t *testing.T) {
			h, daemon, post := hostedPageInputHarness(t)
			id := h.lead(BackendClaude, LeadOptions{}).ID
			if hidden {
				if _, err := h.runner.SetHidden(id, true); err != nil {
					t.Fatal(err)
				}
			}
			eventually(t, "hosted index row", func() bool {
				s, ok := daemon.Index().Session(id)
				return ok && s.Hosted
			})

			c := h.runner.conv(id)
			entered, gate := make(chan struct{}), make(chan struct{})
			var once sync.Once
			release := func() { once.Do(func() { close(gate) }) }
			defer release()
			go c.do(func() { close(entered); <-gate })
			wait(t, "blocked conversation loop", entered)
			response := make(chan *httptest.ResponseRecorder, 1)
			go func() { response <- post(id) }()
			select {
			case out := <-response:
				if out.Code != http.StatusOK {
					t.Fatalf("page send = %d %s", out.Code, out.Body)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("page send waited for the blocked conversation loop")
			}
			held, err := wsinbox.Held(c.inbox)
			if err != nil || len(held) != 1 {
				t.Fatalf("held while loop blocked = %v, %v", held, err)
			}
			release()
			eventually(t, "page input delivered", func() bool { return h.state(id).Process })
			h.quiesce(id)
			if st := h.state(id); st.Hidden {
				t.Fatal("page send left the conversation hidden")
			}
			holder := h.holder(id)
			if got := h.userInputs(holder); len(got) != 1 || !strings.Contains(got[0], "page message") {
				t.Fatalf("page input = %q", got)
			}
			// A redundant post-write wake must not submit the message twice.
			h.holders.out(holder, turnLines("sess-1", "u1", "done")...)
			h.idle(id)
			h.runner.InboxChanged(id)
			h.quiesce(id)
			if got := h.userInputs(holder); len(got) != 1 {
				t.Fatalf("duplicate delivery = %q", got)
			}
		})
	}
}

func TestHostedPageInputEndedRefusesBeforeWrite(t *testing.T) {
	for _, closed := range []bool{false, true} {
		name := "loop ended"
		if closed {
			name = "closed"
		}
		t.Run(name, func(t *testing.T) {
			h, daemon, post := hostedPageInputHarness(t)
			id := h.lead(BackendClaude, LeadOptions{}).ID
			eventually(t, "hosted index row", func() bool {
				s, ok := daemon.Index().Session(id)
				return ok && s.Hosted
			})
			c := h.runner.conv(id)
			// Leave the adapter evidence intact, exercising admission even
			// before the dashboard has learned that the conversation ended.
			if closed {
				c.do(func() { c.setStatus(func() { c.closed = true }) })
				wait(t, "closed loop", c.done)
			} else {
				h.runner.Close()
			}
			out := post(id)
			if out.Code != http.StatusInternalServerError || strings.TrimSpace(out.Body.String()) != errClosed.Error() {
				t.Fatalf("ended page send = %d %s", out.Code, out.Body)
			}
			if held, err := wsinbox.Held(c.inbox); err != nil || len(held) != 0 {
				t.Fatalf("ended send left inbox input = %v, %v", held, err)
			}
		})
	}
}

type hostedPageSessions struct{}

func (hostedPageSessions) ReadSession(wsdashboard.Registration) (wsdashboard.SessionData, bool, error) {
	return wsdashboard.SessionData{}, true, nil
}

func hostedPageInputHarness(t *testing.T) (*leadHarness, *wsdashboard.Daemon, func(string) *httptest.ResponseRecorder) {
	t.Helper()
	h := newLeadHarness(t)
	h.runner = NewRunner(h.opts)
	daemon, err := wsdashboard.StartDaemon(wsdashboard.DaemonOptions{
		Dir: h.dir, Port: 0, Sessions: hostedPageSessions{}, Agents: h.runner, PollInterval: time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(daemon.Close)
	cookie := daemonCookieForTest(t, daemon)
	return h, daemon, func(id string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/s/"+id+"/message", strings.NewReader(`{"text":"page message"}`))
		req.RemoteAddr = "127.0.0.1:12345"
		req.Header.Set("Origin", "http://127.0.0.1")
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(cookie)
		out := httptest.NewRecorder()
		daemon.Handler().ServeHTTP(out, req)
		return out
	}
}
