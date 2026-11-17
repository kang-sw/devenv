package wsdashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsnote"
)

type notesFake struct {
	*pageFake
	calls chan bool
	gate  chan struct{}
	rows  []NotesLayer
}

func (s *notesFake) ReadNotes(reg Registration, refresh bool) ([]NotesLayer, error) {
	s.calls <- refresh
	<-s.gate
	return s.rows, nil
}

func notesDone(t *testing.T, d *Daemon, project string) {
	t.Helper()
	loaded, ok := d.notes.Load(project)
	if !ok {
		t.Fatal("no notes worker")
	}
	v := loaded.(*notesView)
	v.mu.Lock()
	done := v.done
	v.mu.Unlock()
	if done != nil {
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("notes worker did not finish")
		}
	}
}

func TestNotesRouteMemoryResponseLANFilterAndRefresh(t *testing.T) {
	var src *notesFake
	f := startPageFixture(t, lanMode, func(o *DaemonOptions) {
		src = &notesFake{pageFake: o.Sessions.(*pageFake), calls: make(chan bool, 10), gate: make(chan struct{}), rows: []NotesLayer{
			{Layer: wsnote.LayerMachine, Notes: []wsnote.Record{{Key: "private-machine", Value: "secret local path"}}},
			{Layer: wsnote.LayerClone, Notes: []wsnote.Record{{Key: "private-clone"}}},
			{Layer: wsnote.LayerRepo, Notes: []wsnote.Record{{Key: "handoff", Value: "public repo handoff"}}},
			{Layer: wsnote.LayerWorktree, Notes: []wsnote.Record{{Key: "private-worktree"}}},
		}}
		o.Sessions = src
	})
	local := viewer{peer: "127.0.0.1:3456", host: "localhost", localCookie: true}
	lan := viewer{peer: "192.168.1.23:3456", host: "192.168.1.2", cookie: readLANToken(t, f.dir)}
	if rec := serve(f.d, local, http.MethodGet, "/s/livesession?frame=1", ""); rec.Code != 200 {
		t.Fatalf("page: %d", rec.Code)
	}
	if len(src.calls) != 0 {
		t.Fatal("page render read notes")
	}
	rec := serve(f.d, local, http.MethodGet, "/s/livesession/notes", "")
	var got notesResponse
	if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &got) != nil || !got.Loading || len(got.Layers) != 0 {
		t.Fatalf("initial = %d %s", rec.Code, rec.Body.String())
	}
	select {
	case refresh := <-src.calls:
		if refresh {
			t.Fatal("open forced all layers")
		}
	case <-time.After(time.Second):
		t.Fatal("worker did not start")
	}
	// Refresh arriving during the first open must not be lost/coalesced away.
	if rec := serve(f.d, local, http.MethodPost, "/s/livesession/notes", ""); rec.Code != 200 {
		t.Fatalf("refresh = %d", rec.Code)
	}
	close(src.gate)
	notesDone(t, f.d, f.project)
	select {
	case refresh := <-src.calls:
		if !refresh {
			t.Fatal("queued refresh was not forced")
		}
	default:
		t.Fatal("refresh did not reach source")
	}
	for _, v := range []viewer{local, lan} {
		rec := serve(f.d, v, http.MethodGet, "/s/livesession/notes?wait=1", "")
		if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &got) != nil || got.Loading || got.Error {
			t.Fatalf("complete = %d %s", rec.Code, rec.Body.String())
		}
		if v.cookie != "" {
			if !got.LocalOnly || len(got.Layers) != 1 || got.Layers[0].Layer != wsnote.LayerRepo || strings.Contains(rec.Body.String(), "private-") || strings.Contains(rec.Body.String(), "secret") {
				t.Fatalf("LAN leaked local layer: %s", rec.Body.String())
			}
		} else if got.LocalOnly || len(got.Layers) != 4 {
			t.Fatalf("local lost layers: %s", rec.Body.String())
		}
		if rec.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("notes cacheable by browser")
		}
	}
	if len(src.calls) != 0 {
		t.Fatal("wait requests scheduled extra reads")
	}
	for _, tc := range []struct {
		method, path string
		v            viewer
		code         int
	}{
		{http.MethodGet, "/s/unknown/notes", local, 404},
		{http.MethodDelete, "/s/livesession/notes", local, 405},
		{http.MethodGet, "/s/livesession/notes", viewer{peer: lan.peer, host: lan.host}, 401},
	} {
		if rec := serve(f.d, tc.v, tc.method, tc.path, ""); rec.Code != tc.code {
			t.Fatalf("refusal = %d want %d", rec.Code, tc.code)
		}
	}
	req := httptest.NewRequest(http.MethodPost, "http://localhost/s/livesession/notes", nil)
	req.RemoteAddr = local.peer
	req.Header.Set("Origin", "http://evil.invalid")
	cookie, _ := f.d.localhostCookieValue()
	req.AddCookie(&http.Cookie{Name: localhostCookieName, Value: cookie})
	bad := httptest.NewRecorder()
	f.d.Handler().ServeHTTP(bad, req)
	if bad.Code != 403 || len(src.calls) != 0 {
		t.Fatalf("foreign-origin refresh = %d", bad.Code)
	}
}
