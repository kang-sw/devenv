package wsdashboard

import (
	"encoding/json"
	"net/http"
)

// TicketIdeasSource reads idea/ only on an explicit drawer demand, never as
// part of the live panels region. Paths are relative to the session root.
type TicketIdeasSource interface {
	ReadTicketIdeas(reg Registration) (tickets []IdeaTicket, ok bool, err error)
}

type IdeaTicket struct {
	Category string `json:"category"`
	Title    string `json:"title"`
	Stem     string `json:"stem"`
	Path     string `json:"path"`
	Abs      string `json:"abs"`
}

func (d *Daemon) handleTicketIdeas(w http.ResponseWriter, r *http.Request, replyID string) {
	src, supported := d.source.(TicketIdeasSource)
	if !supported || d.index == nil || !ValidReplyID(replyID) {
		http.NotFound(w, r)
		return
	}
	session, ok := d.index.Session(replyID)
	if !ok {
		http.NotFound(w, r)
		return
	}
	tickets, ok, err := src.ReadTicketIdeas(session.Registration)
	if err != nil {
		http.Error(w, "ticket read failed", http.StatusInternalServerError)
		return
	}
	if !ok {
		http.NotFound(w, r)
		return
	}
	if tickets == nil {
		tickets = []IdeaTicket{}
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_ = json.NewEncoder(w).Encode(struct {
		Tickets []IdeaTicket `json:"tickets"`
	}{tickets})
}
