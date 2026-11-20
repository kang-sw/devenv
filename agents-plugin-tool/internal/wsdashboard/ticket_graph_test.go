package wsdashboard

import (
	"errors"
	"net/http"
	"testing"
)

type ticketGraphFake struct {
	*pageFake
	reads   int
	reg     Registration
	payload []byte
	err     error
	found   bool
}

func (s *ticketGraphFake) ReadTicketGraph(reg Registration) ([]byte, bool, error) {
	s.reads++
	s.reg = reg
	return s.payload, s.found, s.err
}

func TestTicketGraphDemandRoute(t *testing.T) {
	var src *ticketGraphFake
	f := startPageFixture(t, lanMode, func(o *DaemonOptions) {
		src = &ticketGraphFake{pageFake: o.Sessions.(*pageFake), found: true,
			payload: []byte(`{"meta":{"v":1},"nodes":[]} `)}
		o.Sessions = src
	})
	local := viewer{peer: "127.0.0.1:3456", host: "localhost", localCookie: true}
	lan := viewer{peer: "192.168.1.23:3456", host: "192.168.1.2", cookie: readLANToken(t, f.dir)}
	if rec := serve(f.d, local, http.MethodGet, "/s/livesession?frame=1", ""); rec.Code != http.StatusOK {
		t.Fatalf("page: %d %s", rec.Code, rec.Body.String())
	}
	if src.reads != 0 {
		t.Fatal("page open read the ticket graph")
	}
	for _, v := range []viewer{local, lan} {
		rec := serve(f.d, v, http.MethodGet, "/s/livesession/tickets/graph", "")
		if rec.Code != http.StatusOK || rec.Body.String() != string(src.payload) {
			t.Fatalf("demand: %d %s", rec.Code, rec.Body.String())
		}
		for name, want := range map[string]string{
			"Content-Type":           "application/json",
			"Cache-Control":          "no-store",
			"X-Content-Type-Options": "nosniff",
		} {
			if got := rec.Header().Get(name); got != want {
				t.Fatalf("%s = %q, want %q", name, got, want)
			}
		}
		if src.reg.Project != f.project || src.reg.ReplyID != "livesession" {
			t.Fatalf("wrong session: %+v", src.reg)
		}
	}
	before := src.reads
	for _, tc := range []struct {
		method, path string
		v            viewer
		status       int
	}{
		{http.MethodGet, "/s/nosuchsession/tickets/graph", local, http.StatusNotFound},
		{http.MethodPost, "/s/livesession/tickets/graph", local, http.StatusMethodNotAllowed},
		{http.MethodGet, "/s/livesession/tickets/graph", viewer{peer: lan.peer, host: lan.host}, http.StatusUnauthorized},
	} {
		if rec := serve(f.d, tc.v, tc.method, tc.path, ""); rec.Code != tc.status {
			t.Fatalf("%s %s = %d, want %d: %s", tc.method, tc.path, rec.Code, tc.status, rec.Body.String())
		}
	}
	if src.reads != before {
		t.Fatal("refused request reached the ticket source")
	}
	src.err = errors.New("private path")
	rec := serve(f.d, local, http.MethodGet, "/s/livesession/tickets/graph", "")
	if rec.Code != http.StatusInternalServerError || rec.Body.String() != "ticket read failed\n" {
		t.Fatalf("failure = %d %s", rec.Code, rec.Body.String())
	}
	src.err, src.found = nil, false
	if rec = serve(f.d, local, http.MethodGet, "/s/livesession/tickets/graph", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("gone = %d", rec.Code)
	}
	before = src.reads
	if _, ok, err := (agentSessions{inner: src}).ReadTicketGraph(Registration{Agent: true}); ok || err != nil || src.reads != before {
		t.Fatal("agent forwarded a ticket read")
	}
	if _, ok, err := (agentSessions{inner: newPageFake()}).ReadTicketGraph(Registration{}); ok || err != nil {
		t.Fatal("unsupported source accepted")
	}
}
