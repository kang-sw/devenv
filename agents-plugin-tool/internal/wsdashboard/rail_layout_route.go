package wsdashboard

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// railLayoutRoute is the rail layout's write route (POST): one op on the
// stored layout (rail_layout.go). Like the rail's other writes it needs the
// daemon's own Origin and nothing more, so a LAN viewer holding the token
// can reorder and star too: the layout is a view preference every viewer
// shares, with no execution authority.
//
// The body is {op, rid?, parent?, before?, after?, path?, workspace?, order?, stars?,
// wsorder?}:
//
//   - move {rid, before|after}: reorder a live session among its group's
//     top-level rows on its side of the stars, or among its parent's
//     children.
//   - attach {rid, parent, before?|after?}: make rid a child of the live,
//     top-level parent (before or after one of its children, else last).
//   - detach {rid, before?|after?}: make a child top-level (before or after
//     a top-level row of its own group, else next to its old parent).
//   - star, unstar {rid}: add or remove rid in its workspace's stars.
//   - wsmove {path, before|after}: reorder a workspace.
//   - wtmove {workspace, path, before|after}: reorder a displayed worktree
//     group within its workspace, including root and empty groups.
//   - import {order, stars, wsorder}: a browser's retired local layout,
//     taken only into an empty layout.
//
// The answer is JSON {applied, reason?, gen}: 200 applied:true when the op
// applied (whether or not it changed anything); 200 applied:false with
// reason "not_applied" for an import over a non-empty layout; 409 with the
// refusal's reason (one of the layout* codes in rail_layout.go: unknown,
// ended, self, depth, workspace, group, level, stars, child, toplevel); 400
// "invalid" for a malformed body; 503 "retry" for an import before the
// index has read the registry; 500 "store" when the layout file could not
// be read or written. gen is the layout's generation after the op. A
// change wakes the rail's streams.
const railLayoutRoute = "/rail/layout"

// railLayoutBodyLimit bounds the route's body; an import carries a
// browser's whole local order.
const railLayoutBodyLimit = 128 << 10

// railLayoutReply is the layout route's answer.
type railLayoutReply struct {
	Applied bool   `json:"applied"`
	Reason  string `json:"reason,omitempty"`
	Gen     uint64 `json:"gen"`
}

// railLayoutStatus is the HTTP status of an answer with reason.
func railLayoutStatus(reason string) int {
	switch reason {
	case "", layoutNotApplied:
		return http.StatusOK
	case layoutInvalid:
		return http.StatusBadRequest
	case layoutRetry:
		return http.StatusServiceUnavailable
	case layoutStore:
		return http.StatusInternalServerError
	}
	return http.StatusConflict
}

// handleRailLayout serves POST /rail/layout (railLayoutRoute).
func (d *Daemon) handleRailLayout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !sameOrigin(r) {
		http.Error(w, "cross-origin request refused", http.StatusForbidden)
		return
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, railLayoutBodyLimit))
	if err != nil {
		http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	var req railLayoutRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, railLayoutReply{Reason: layoutInvalid})
		return
	}
	loaded := d.index != nil && d.index.Loaded()
	answer, err := d.layout.apply(req, railFactsOf(d.railBaseView(d.pages())), loaded)
	if err != nil {
		fmt.Fprintf(d.log, "dashboard: rail layout %s: %v\n", req.Op, err)
	}
	if answer.Changed {
		d.broker.wake(railListRepoKey)
	}
	reply := railLayoutReply{Applied: answer.Reason == "", Reason: answer.Reason, Gen: answer.Gen}
	writeJSON(w, railLayoutStatus(answer.Reason), reply)
}
