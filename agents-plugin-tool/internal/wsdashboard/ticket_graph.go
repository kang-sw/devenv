package wsdashboard

import "net/http"

// TicketGraphSource builds the ticket map payload on an explicit drawer
// demand, never as part of the live panels region. It returns the encoded
// JSON body, which the route serves verbatim.
type TicketGraphSource interface {
	ReadTicketGraph(reg Registration) (payload []byte, ok bool, err error)
}

func (d *Daemon) handleTicketGraph(w http.ResponseWriter, r *http.Request, replyID string) {
	src, supported := d.source.(TicketGraphSource)
	if !supported || d.index == nil || !ValidReplyID(replyID) {
		http.NotFound(w, r)
		return
	}
	session, ok := d.index.Session(replyID)
	if !ok {
		http.NotFound(w, r)
		return
	}
	payload, ok, err := src.ReadTicketGraph(session.Registration)
	if err != nil {
		http.Error(w, "ticket read failed", http.StatusInternalServerError)
		return
	}
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(payload)
}
