package wsdashboard

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/kang-sw/devenv/internal/wsinbox"
)

// The browser write routes: POST /s/<reply-id>/message holds a message the
// user sent from the composer in the session's board-answer inbox (merged
// into the held one when there is one), and POST /s/<reply-id>/cancel takes a
// held message back. The inbox is the daemon's one write target; the
// session's host adapter drains it at the end of a turn and injects the
// message as the user's own. Both routes are offered only for a session with
// a live host adapter, require an Origin equal to the daemon's own (which
// stops cross-site form posts), and send no CORS headers.

// inboxBodyLimit bounds a write route's request body.
const inboxBodyLimit = 256 << 10

// messageRequest is the message route's body. Unknown fields are ignored.
type messageRequest struct {
	Text  string         `json:"text"`
	Chips []wsinbox.Chip `json:"chips"`
}

// cancelRequest is the cancel route's body.
type cancelRequest struct {
	ID string `json:"id"`
}

// sameOrigin reports whether r carries an Origin equal to the daemon's own
// origin as the request addressed it: http and the request's Host. A missing
// or opaque ("null") Origin is not the daemon's.
func sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" || origin == "null" {
		return false
	}
	u, err := url.Parse(origin)
	if err != nil || u.Scheme != "http" || u.User != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" {
		return false
	}
	return strings.EqualFold(u.Host, r.Host)
}

// handleInboxWrite serves the message and cancel routes of one session.
func (d *Daemon) handleInboxWrite(w http.ResponseWriter, r *http.Request, replyID, route string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !sameOrigin(r) {
		http.Error(w, "cross-origin request refused", http.StatusForbidden)
		return
	}
	if d.index == nil || !ValidReplyID(replyID) {
		http.Error(w, "unknown session", http.StatusNotFound)
		return
	}
	s, ok := d.index.Session(replyID)
	if !ok {
		http.Error(w, "unknown session", http.StatusNotFound)
		return
	}
	now := time.Now()
	if !adapterPresent(d.pages().readSnapshot(replyID), now) {
		http.Error(w, "this session has no live host adapter: copy the answers into its terminal instead", http.StatusConflict)
		return
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, inboxBodyLimit+1))
	if err != nil || len(raw) > inboxBodyLimit {
		http.Error(w, "request body too large or unreadable", http.StatusBadRequest)
		return
	}
	dir := wsinbox.Dir(SessionDir(d.dir, replyID))

	d.inboxMu.Lock()
	status, answer := d.inboxWrite(dir, route, raw, now)
	d.inboxMu.Unlock()

	if status == http.StatusOK {
		d.index.Nudge(replyID, s.Project)
		d.broker.wake(replyID)
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, status, answer)
		return
	}
	http.Error(w, fmt.Sprint(answer), status)
}

// inboxWrite performs one write route against the inbox at dir; d.inboxMu
// is held, so the daemon's own sends and cancels never race each other, and
// the inbox's claims settle a race with the adapter's drain.
func (d *Daemon) inboxWrite(dir, route string, raw []byte, now time.Time) (int, any) {
	switch route {
	case "message":
		var req messageRequest
		if err := json.Unmarshal(raw, &req); err != nil {
			return http.StatusBadRequest, "malformed message"
		}
		name, merged, err := wsinbox.Send(dir, wsinbox.Entry{Parts: []string{req.Text}, Chips: req.Chips}, now)
		if err != nil {
			if errors.Is(err, wsinbox.ErrEmpty) || errors.Is(err, wsinbox.ErrInvalid) {
				return http.StatusBadRequest, err.Error()
			}
			fmt.Fprintf(d.log, "dashboard: inbox send: %v\n", err)
			return http.StatusInternalServerError, "could not hold the message"
		}
		return http.StatusOK, map[string]any{"id": wsinbox.Record{Name: name}.ID(), "merged": merged}
	case "cancel":
		var req cancelRequest
		if err := json.Unmarshal(raw, &req); err != nil {
			return http.StatusBadRequest, "malformed cancel"
		}
		name, ok := wsinbox.NameForID(req.ID)
		if !ok {
			return http.StatusBadRequest, "malformed message id"
		}
		entry, err := wsinbox.Cancel(dir, name)
		if errors.Is(err, wsinbox.ErrGone) {
			return http.StatusConflict, "the session already received this message"
		}
		if err != nil {
			fmt.Fprintf(d.log, "dashboard: inbox cancel: %v\n", err)
			return http.StatusInternalServerError, "could not cancel the message"
		}
		chips := entry.Chips
		if chips == nil {
			chips = []wsinbox.Chip{}
		}
		return http.StatusOK, map[string]any{"text": strings.Join(entry.Parts, "\n\n"), "chips": chips}
	}
	return http.StatusNotFound, "not found"
}
