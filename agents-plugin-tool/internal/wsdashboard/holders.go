package wsdashboard

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/kang-sw/devenv/internal/wsholder"
)

// The session holders (internal/wsholder): processes that each own one
// child and outlive this daemon. The daemon attaches to every holder it
// finds at start and on each rescan, and never ends one on its own. Two
// routes serve the drawer's holders card:
//
//   - GET /holders answers every known holder (attached or not, with the
//     cause when not, and whether it is restart-safe), the recent holder
//     start failures, and can_stop: whether this viewer may stop one.
//   - POST /holders/<id>/stop ends a holder and its child: through the
//     protocol when attached, otherwise with the kill guarded by the
//     holder's recorded start time. A holder may own a terminal, so the
//     route is for a browser on this machine only (localViewer; a LAN
//     viewer, token or not, gets 404, as if the route did not exist) that
//     sends the daemon's own Origin (sameOrigin, as the write routes do).
//     It answers 204, 404 for an unknown holder, and 409 with the reason
//     when the kill was refused or the holder did not end.

// HolderHost is the session holder manager the daemon runs
// (*wsholder.Manager). Run scans at once and rescans until Close; Close
// detaches from every holder and ends none.
type HolderHost interface {
	Run()
	Close()
	List() wsholder.Listing
	Stop(id string) error
}

// holderList is the holders route's answer.
type holderList struct {
	wsholder.Listing
	// CanStop reports whether this viewer may use the stop route.
	CanStop bool `json:"can_stop"`
}

// handleHolders serves GET /holders and POST /holders/<id>/stop.
func (d *Daemon) handleHolders(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, "/holders"), "/")
	if rest == "" {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(holderList{Listing: d.holders.List(), CanStop: localViewer(r)})
		return
	}
	id, ok := strings.CutSuffix(rest, "/stop")
	if !ok || !wsholder.ValidID(id) || !localViewer(r) {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !sameOrigin(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	switch err := d.holders.Stop(id); {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, wsholder.ErrUnknownHolder):
		http.NotFound(w, r)
	default:
		http.Error(w, err.Error(), http.StatusConflict)
	}
}
