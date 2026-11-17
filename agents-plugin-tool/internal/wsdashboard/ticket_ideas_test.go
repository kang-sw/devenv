package wsdashboard

import (
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"testing"
)

type ticketIdeasFake struct {
	*pageFake
	reads int
	reg   Registration
	rows  []IdeaTicket
	err   error
	found bool
}

func (s *ticketIdeasFake) ReadTicketIdeas(reg Registration) ([]IdeaTicket, bool, error) {
	s.reads++
	s.reg = reg
	return s.rows, s.found, s.err
}

func TestTicketIdeasDemandRoute(t *testing.T) {
	var src *ticketIdeasFake
	f := startPageFixture(t, lanMode, func(o *DaemonOptions) {
		src = &ticketIdeasFake{pageFake: o.Sessions.(*pageFake), found: true,
			rows: []IdeaTicket{{Category: "feat", Title: "Idea <one>", Stem: "261117-feat-one", Path: "ai-docs/tickets/idea/261117-feat-one.md"}}}
		o.Sessions = src
	})
	local := viewer{peer: "127.0.0.1:3456", host: "localhost", localCookie: true}
	lan := viewer{peer: "192.168.1.23:3456", host: "192.168.1.2", cookie: readLANToken(t, f.dir)}
	if rec := serve(f.d, local, http.MethodGet, "/s/livesession?frame=1", ""); rec.Code != http.StatusOK {
		t.Fatalf("page: %d %s", rec.Code, rec.Body.String())
	}
	if src.reads != 0 {
		t.Fatal("page open read idea tickets")
	}
	for _, v := range []viewer{local, lan} {
		rec := serve(f.d, v, http.MethodGet, "/s/livesession/tickets/idea", "")
		var got struct {
			Tickets []IdeaTicket `json:"tickets"`
		}
		if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &got) != nil || !reflect.DeepEqual(got.Tickets, src.rows) {
			t.Fatalf("demand: %d %s", rec.Code, rec.Body.String())
		}
		if rec.Header().Get("Cache-Control") != "no-store" || src.reg.Project != f.project || src.reg.ReplyID != "livesession" {
			t.Fatalf("wrong cache policy or session: %+v", src.reg)
		}
	}
	before := src.reads
	for _, tc := range []struct {
		method, path string
		v            viewer
		status       int
	}{
		{http.MethodGet, "/s/nosuchsession/tickets/idea", local, http.StatusNotFound},
		{http.MethodPost, "/s/livesession/tickets/idea", local, http.StatusMethodNotAllowed},
		{http.MethodGet, "/s/livesession/tickets/idea", viewer{peer: lan.peer, host: lan.host}, http.StatusUnauthorized},
	} {
		if rec := serve(f.d, tc.v, tc.method, tc.path, ""); rec.Code != tc.status {
			t.Fatalf("%s %s = %d, want %d: %s", tc.method, tc.path, rec.Code, tc.status, rec.Body.String())
		}
	}
	if src.reads != before {
		t.Fatal("refused request reached ticket source")
	}
	src.rows = nil
	rec := serve(f.d, local, http.MethodGet, "/s/livesession/tickets/idea", "")
	if rec.Body.String() != "{\"tickets\":[]}\n" {
		t.Fatalf("empty = %s", rec.Body.String())
	}
	src.err = errors.New("private path")
	rec = serve(f.d, local, http.MethodGet, "/s/livesession/tickets/idea", "")
	if rec.Code != http.StatusInternalServerError || rec.Body.String() != "ticket read failed\n" {
		t.Fatalf("failure = %d %s", rec.Code, rec.Body.String())
	}
	src.err, src.found = nil, false
	if rec = serve(f.d, local, http.MethodGet, "/s/livesession/tickets/idea", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("gone = %d", rec.Code)
	}
	before = src.reads
	if _, ok, err := (agentSessions{inner: src}).ReadTicketIdeas(Registration{Agent: true}); ok || err != nil || src.reads != before {
		t.Fatal("agent forwarded a ticket read")
	}
	if _, ok, err := (agentSessions{inner: newPageFake()}).ReadTicketIdeas(Registration{}); ok || err != nil {
		t.Fatal("unsupported source accepted")
	}
}
