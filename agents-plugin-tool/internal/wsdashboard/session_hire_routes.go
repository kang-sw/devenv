package wsdashboard

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const hireRoute = "/session-hire"

func trustedHireWrite(w http.ResponseWriter, r *http.Request) bool {
	local, own := TrustedWrite(r)
	if !local {
		http.Error(w, "unknown route", http.StatusNotFound)
		return false
	}
	if !own {
		http.Error(w, "cross-origin request refused", http.StatusForbidden)
		return false
	}
	return true
}

func readHireBody(w http.ResponseWriter, r *http.Request, limit int64, into any) bool {
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, limit))
	if err != nil {
		http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
		return false
	}
	if err = json.Unmarshal(raw, into); err != nil {
		http.Error(w, "malformed request body", http.StatusBadRequest)
		return false
	}
	return true
}

func (d *Daemon) handleSessionHire(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	rest := strings.TrimPrefix(r.URL.Path, hireRoute)
	if rest == "/roster" {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", 405)
			return
		}
		rows, err := d.sessionRoster(r.URL.Query().Get("requester"))
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		writeJSON(w, 200, rows)
		return
	}
	// Even the queue exposes private recovery context: trusted local viewers only.
	local, _ := TrustedWrite(r)
	if !local {
		http.Error(w, "unknown route", 404)
		return
	}
	if rest == "" && r.Method == http.MethodGet {
		queue, err := d.pendingHires()
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		writeJSON(w, 200, queue)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	if !trustedHireWrite(w, r) {
		return
	}
	id, action, ok := strings.Cut(strings.TrimPrefix(rest, "/"), "/")
	if !ok || id == "" || (action != "approve" && action != "reject") {
		http.NotFound(w, r)
		return
	}
	var body struct {
		HireLaunch
		Reason string `json:"reason"`
	}
	if !readHireBody(w, r, 64<<10, &body) {
		return
	}
	if action == "approve" {
		label, err := cleanLabel(body.Label)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		body.Label = label
	}
	d.hireMu.Lock()
	defer d.hireMu.Unlock()
	proposals, err := d.readHires()
	if err == nil {
		err = d.settleHiresLocked(proposals)
	}
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	for i := range proposals {
		p := &proposals[i]
		if p.ID != id {
			continue
		}
		if p.Status != "pending" {
			writeJSON(w, http.StatusConflict, p)
			return
		}
		if action == "reject" {
			p.Status, p.ReasonClosed = "rejected", body.Reason
		} else {
			p.Launch, p.CreationError = &body.HireLaunch, ""
			// Intent precedes creation. A disconnected browser must not replay an
			// unrelated create; repeated approvals find this same proposal/session.
			if err = d.saveHires(proposals); err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
			if err = d.approveHireLocked(proposals, i); err != nil {
				p.Launch, p.CreationError = nil, err.Error()
				if saveErr := d.saveHires(proposals); saveErr != nil {
					err = fmt.Errorf("%v; save: %w", err, saveErr)
				}
				http.Error(w, err.Error(), 409)
				return
			}
		}
		if err = d.saveHires(proposals); err == nil {
			err = d.settleHiresLocked(proposals)
		}
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		writeJSON(w, 200, p)
		return
	}
	http.NotFound(w, r)
}

// /s/<rid>/hiring uses the same trusted control gate for hosted and CLI leads.
func (d *Daemon) handleHiringFlag(w http.ResponseWriter, r *http.Request, replyID string) {
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		http.Error(w, "method not allowed", 405)
		return
	}
	local, _ := TrustedWrite(r)
	if !local {
		http.NotFound(w, r)
		return
	}
	if r.Method == http.MethodPost && !trustedHireWrite(w, r) {
		return
	}
	d.hireMu.Lock()
	defer d.hireMu.Unlock()
	reg, err := ReadRegistration(d.dir, replyID)
	if err != nil || reg.Agent || reg.Unlisted {
		http.NotFound(w, r)
		return
	}
	meta, err := ReadHiringMetadata(d.dir, replyID)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if r.Method == http.MethodGet {
		writeJSON(w, 200, meta)
		return
	}
	var req struct {
		Enabled *bool `json:"enabled"`
	}
	if !readHireBody(w, r, 4096, &req) {
		return
	}
	if req.Enabled == nil {
		http.Error(w, "enabled must be a boolean", http.StatusBadRequest)
		return
	}
	if *req.Enabled && (d.index == nil || !d.index.Loaded()) {
		http.Error(w, "session index is not ready", http.StatusServiceUnavailable)
		return
	}
	if *req.Enabled && (!reg.Live(d.hireClock()) || meta.Hired || d.hireParent(replyID) != "") {
		http.Error(w, "hiring cannot be enabled for an ended, nested, or hired session", 409)
		return
	}
	meta.Enabled = *req.Enabled
	if err = writeHiringMetadata(d.dir, replyID, meta); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	d.broker.wake(railListRepoKey)
	writeJSON(w, 200, meta)
}

// SessionRosterRow has exactly the seven agreed read-only fields. Unknown
// model and activity are null, not requested settings or heartbeat times.
type SessionRosterRow struct {
	Mailbox          string  `json:"mailbox"`
	Harness          string  `json:"harness"`
	Model            *string `json:"model"`
	Label            string  `json:"label"`
	State            string  `json:"state"`
	RunningSubagents int     `json:"running_subagents"`
	LastActivity     *int64  `json:"last_activity"`
}

func (d *Daemon) sessionRoster(replyID string) ([]SessionRosterRow, error) {
	if !ValidReplyID(replyID) {
		return nil, fmt.Errorf("unknown requester")
	}
	view := d.railBaseView(d.pages())
	facts := railFactsOf(view)
	parent, ok := facts.rows[replyID]
	if !ok {
		return []SessionRosterRow{}, nil
	}
	doc, _ := d.layout.current()
	scope := doc.scope(parent.WS, facts, false)
	rows := []SessionRosterRow{}
	add := func(list []RailRow) {
		for _, row := range list {
			if !scope.child(row.ReplyID) || scope.parent(row.ReplyID) != replyID {
				continue
			}
			item := SessionRosterRow{Mailbox: "id:" + row.ReplyID, Harness: row.Harness, Label: row.Label, State: "idle"}
			if row.Live && row.Exit == nil && !row.Hover.Stale {
				if row.Working {
					item.State = "busy"
				}
				item.RunningSubagents = row.Subagents
			}
			if !row.Hover.Stale && row.Hover.Observed != nil && row.Hover.Observed.Current.Model != "" {
				model := row.Hover.Observed.Current.Model
				item.Model = &model
			}
			if row.LastTool != 0 {
				last := row.LastTool
				item.LastActivity = &last
			}
			rows = append(rows, item)
		}
	}
	for _, ws := range view.Workspaces {
		add(ws.Sessions)
		add(ws.Ended)
		for _, wt := range ws.Worktrees {
			add(wt.Sessions)
			add(wt.Ended)
		}
	}
	return rows, nil
}
