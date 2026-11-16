package wsdashboard

import (
	"encoding/json"
	"net/http"

	"github.com/kang-sw/devenv/internal/wslive"
)

// The entry text route: GET /s/<reply-id>/entry?key=<entry name> answers the
// stored text of one conversation entry, for the chat blocks' Copy action.
// Only a lead reply and an owner prompt have one: a reply's raw markdown, and
// a prompt's text before the display parsers take its capsules, images, and
// selections out (one text per bubble splitUserEntries draws, in order). The
// page renders neither raw form, so Copy reads it here, on demand, instead of
// carrying every block's text twice. key is only ever an entry name, never a
// path. It is read-only and serves what the session page shows, so it takes
// the page's own access rule, as the history route does.

// entryTextAnswer is the route's answer. Truncated reports that the ring
// stored only the text's head (wslive.TextBytes).
type entryTextAnswer struct {
	Blocks    []string `json:"blocks"`
	Truncated bool     `json:"truncated"`
}

// handleEntryText serves the entry text route of one session.
func (d *Daemon) handleEntryText(w http.ResponseWriter, r *http.Request, replyID string) {
	key := r.URL.Query().Get("key")
	if !wslive.ValidRingName(key) {
		http.Error(w, "key is not a conversation entry name", http.StatusBadRequest)
		return
	}
	if _, _, ok := d.pages().served(replyID); !ok {
		http.Error(w, "unknown session", http.StatusNotFound)
		return
	}
	rec, ok := wslive.ReadRingEntry(wslive.RingDir(SessionDir(d.dir, replyID)), key)
	if !ok {
		http.Error(w, "unknown entry", http.StatusNotFound)
		return
	}
	var ans entryTextAnswer
	switch rec.Kind {
	case wslive.RingReply:
		ans.Blocks = []string{rec.Text}
	case wslive.RingUser:
		ans.Blocks = userBlockTexts(rec.Text)
	default:
		http.Error(w, "entry has no text", http.StatusNotFound)
		return
	}
	ans.Truncated = rec.Truncated
	raw, _ := json.Marshal(ans)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(raw)
}
