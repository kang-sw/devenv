package wsdashboard

import (
	"encoding/json"
	"net/http"
	"sync"

	"github.com/kang-sw/devenv/internal/wsnote"
)

// NotesSource runs only in a drawer-open/explicit-refresh worker. It returns
// no store paths or session credentials to the browser.
type NotesSource interface {
	ReadNotes(reg Registration, refresh bool) ([]NotesLayer, error)
}

type NotesLayer struct {
	Layer wsnote.Layer    `json:"layer"`
	Notes []wsnote.Record `json:"notes"`
	Error bool            `json:"error,omitempty"`
}

type notesView struct {
	mu      sync.Mutex
	layers  []NotesLayer
	err     bool
	done    chan struct{}
	refresh bool
}

type notesResponse struct {
	Layers    []NotesLayer `json:"layers"`
	Loading   bool         `json:"loading"`
	LocalOnly bool         `json:"local_only"`
	Error     bool         `json:"error,omitempty"`
}

func (v *notesView) start(src NotesSource, reg Registration, refresh bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.done != nil {
		v.refresh = v.refresh || refresh
		return
	}
	v.done = make(chan struct{})
	go func() {
		for {
			layers, err := src.ReadNotes(reg, refresh)
			v.mu.Lock()
			if err == nil {
				v.layers = layers
			}
			v.err = err != nil
			if v.refresh {
				v.refresh = false
				refresh = true
				v.mu.Unlock()
				continue
			}
			close(v.done)
			v.done = nil
			v.mu.Unlock()
			return
		}
	}()
}

// GET opens from memory immediately and schedules only the repo fingerprint
// check (first use also loads the layers). ?wait=1 joins that worker without
// scheduling any new I/O. POST is the explicit all-layer refresh. There are
// no live-event hooks or periodic reads/writes here.
func (d *Daemon) handleNotes(w http.ResponseWriter, r *http.Request, replyID string) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		w.Header().Set("Allow", "GET, POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if r.Method == http.MethodPost && !sameOrigin(r) {
		http.Error(w, "own origin required", http.StatusForbidden)
		return
	}
	src, supported := d.source.(NotesSource)
	if !supported || d.index == nil || !ValidReplyID(replyID) {
		http.NotFound(w, r)
		return
	}
	session, ok := d.index.Session(replyID)
	if !ok {
		http.NotFound(w, r)
		return
	}
	loaded, _ := d.notes.LoadOrStore(replyID, &notesView{})
	view := loaded.(*notesView)
	if r.Method == http.MethodPost || r.URL.Query().Get("wait") != "1" {
		view.start(src, session.Registration, r.Method == http.MethodPost)
	} else {
		view.mu.Lock()
		done := view.done
		view.mu.Unlock()
		if done != nil {
			select {
			case <-done:
			case <-r.Context().Done():
				return
			}
		}
	}
	view.mu.Lock()
	out := notesResponse{Layers: []NotesLayer{}, Loading: view.done != nil, LocalOnly: !LocalViewer(r), Error: view.err}
	for _, layer := range view.layers {
		if !out.LocalOnly || layer.Layer == wsnote.LayerRepo {
			out.Layers = append(out.Layers, layer)
		}
	}
	view.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_ = json.NewEncoder(w).Encode(out)
}
