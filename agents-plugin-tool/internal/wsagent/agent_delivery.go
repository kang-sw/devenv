// agent_delivery.go holds child delivery, expiry, retention, and token routing.
package wsagent

import (
	"errors"
	"fmt"
	"maps"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wslive"
)

func reportSummary(res TurnResult) string {
	line, _, _ := strings.Cut(strings.TrimSpace(res.Message), "\n")
	if runes := []rune(line); len(runes) > labelRunes {
		line = string(runes[:labelRunes-1]) + "…"
	}
	if line == "" && res.Error != "" {
		line = strings.Join(strings.Fields(res.Error), " ")
	}
	if line == "" {
		return res.Status
	}
	return res.Status + ": " + line
}

// Delivery is one message for the delivery seam: its text and the sender's
// reply-id, told when the wake it asks is refused ("" tells no one).
type Delivery struct {
	From string
	Text string
}

// Deliver hands d to conversation id as its next input, waking it when it
// sleeps; the width cap does not apply. When it sleeps and its admission
// is refused now, the message still queues, it stays asleep, and the
// sender gets one notice (Options.WakeRefused).
func (r *Runner) Deliver(id string, d Delivery) (State, error) {
	var refusal error
	if c := r.conv(id); c != nil {
		c.mu.Lock()
		asleep := c.proc == nil && !c.turn
		rec := c.rec
		c.mu.Unlock()
		if asleep {
			rp, resolveErr := r.resolve(rec.Profile, rec.Root)
			if resolveErr != nil {
				refusal = resolveErr
			} else {
				_, _, refusal = r.childAdmission(rec, rp.Definition)
			}
		}
	}
	st, err := r.Send(id, d.Text)
	if err != nil {
		return st, err
	}
	if refusal != nil && d.From != "" && r.opts.WakeRefused != nil {
		r.opts.WakeRefused(d.From, id, refusal.Error())
	}
	return st, nil
}

// mailRelation is how sender, a reply-id, relates to rec through the
// Runner spawn lineage (Record.Parent): "parent" for rec's parent, "child"
// for a conversation rec spawned, else "peer"; "" for no sender (system
// mail). It is computed at each delivery, never stored.
func (r *Runner) mailRelation(rec Record, sender string) string {
	switch {
	case sender == "":
		return ""
	case rec.Parent != nil && rec.Parent.ReplyID == sender:
		return "parent"
	}
	if s, ok := r.recordOf(sender); ok && s.Parent != nil && s.Parent.ReplyID == rec.ReplyID {
		return "child"
	}
	return "peer"
}

// childWaitingExpiry is how long a child may wait before it is deleted:
// agent.child_waiting_days, read once per runner.
func (r *Runner) childWaitingExpiry() time.Duration {
	r.expiryOnce.Do(func() {
		days := DefaultChildWaitingDays
		if r.opts.ChildWaitingDays > 0 {
			days = r.opts.ChildWaitingDays
		} else {
			resolver := wsconfig.NewResolver(r.opts.Config, map[string]string{wsconfig.ItemAgentChildWaitingDays: strconv.Itoa(DefaultChildWaitingDays)}, nil, nil)
			if rv, err := resolver.Get("", wsconfig.ItemAgentChildWaitingDays); err == nil {
				if n, err := strconv.Atoi(strings.TrimSpace(rv.Value)); err == nil && n > 0 {
					days = n
				}
			}
		}
		r.expiry = time.Duration(days) * 24 * time.Hour
	})
	return r.expiry
}

// childWaitingLocked reports a spawned child asleep with no turn and no
// input or claimed mail about to start one (a refused wake's does not
// count). Under mu.
func (c *conv) childWaitingLocked() bool {
	return !c.closed && !c.rec.Lead && c.rec.Parent != nil && c.proc == nil && !c.turn && !c.spawnPending &&
		(c.wakeRefused || c.sends <= max(c.settled.mark, c.voided) && len(c.rec.Mail) == 0)
}

// lastActive is a spawned record's expiry clock start.
func lastActive(rec Record) time.Time {
	for _, v := range []string{rec.LastActiveAt, rec.CreatedAt} {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			return t
		}
	}
	return time.Time{}
}

// expireIfDue has the runner delete a child waiting past its expiry
// (expireLater). It runs in the loop, on the heartbeat: no file is read.
func (c *conv) expireIfDue() {
	c.mu.Lock()
	due := !c.expiring && c.childWaitingLocked()
	rec := c.rec
	c.mu.Unlock()
	if !due {
		return
	}
	at := lastActive(rec)
	if at.IsZero() || c.r.opts.Now().Sub(at) < c.r.childWaitingExpiry() {
		return
	}
	c.expiring = true
	c.r.expireLater(rec.ReplyID)
}

// expireLater deletes waiting child id and its descendants, deepest first,
// off the caller's goroutine, unless a descendant is awake or id woke
// meanwhile.
func (r *Runner) expireLater(id string) {
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		r.expire(id)
	}()
}

// expire is expireLater's body; it reports whether id was deleted.
func (r *Runner) expire(id string) bool {
	c := r.conv(id)
	if c == nil {
		return false
	}
	c.mu.Lock()
	waiting := c.childWaitingLocked()
	c.mu.Unlock()
	desc := r.descendants(id)
	busy := !waiting
	for _, d := range desc {
		if dc := r.conv(d); dc != nil {
			dc.mu.Lock()
			busy = busy || dc.awakeLocked()
			dc.mu.Unlock()
		}
	}
	if busy {
		c.do(func() { c.expiring = false })
		return false
	}
	if _, err := r.CloseConversation(id); err != nil {
		if errors.Is(err, errClosed) {
			// Someone closed it meanwhile (a dashboard close): what they
			// closed is theirs to delete, not the expiry's.
			return false
		}
		fmt.Fprintf(r.opts.Log, "agent %s: expire: %v\n", id, err)
		// A later heartbeat tries again.
		c.do(func() { c.expiring = false })
		return false
	}
	for _, d := range append(desc, id) {
		r.forget(d)
	}
	return true
}

var _ wsdashboard.AgentForgetter = (*Runner)(nil)

// ForgetClosed drops closed or handed-off conversation id from the runner's
// in-memory state without touching disk or notifying the daemon, for a caller
// that is purging the session directory itself.
func (r *Runner) ForgetClosed(id string) {
	r.forgetClosed(id)
}

func (r *Runner) forgetClosed(id string) (Record, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rec, ok := r.closed[id]
	delete(r.closed, id)
	delete(r.closedRows, id)
	if ok && rec.Parent != nil && r.childrenIndex != nil {
		delete(r.childrenIndex[*rec.Parent], id)
		if len(r.childrenIndex[*rec.Parent]) == 0 {
			delete(r.childrenIndex, *rec.Parent)
		}
	}
	return rec, ok
}

// forget drops closed conversation id from the runner and deletes its
// session directory.
func (r *Runner) forget(id string) {
	rec, ok := r.forgetClosed(id)
	if !ok {
		return
	}
	sessionDir := wsdashboard.SessionDir(r.opts.Dir, id)
	wslive.DropRing(wslive.RingDir(sessionDir))
	if err := os.RemoveAll(sessionDir); err != nil {
		fmt.Fprintf(r.opts.Log, "agent %s: delete: %v\n", id, err)
	}
	r.notify(id, rec.Root)
}

// expiredAtStart picks the open spawned children a daemon start deletes
// instead of serving, deepest first: each quiet (no turn open or held, no
// input) past its expiry whose descendants are all quiet, with those
// descendants. A child whose input waits on a refused wake is not quiet
// here; its loop's heartbeat deletes it once its start is refused again.
func (r *Runner) expiredAtStart(open []Record) []string {
	now := r.opts.Now()
	expiry := r.childWaitingExpiry()
	byID := map[string]Record{}
	kids := map[string][]string{}
	for _, rec := range open {
		byID[rec.ReplyID] = rec
		if rec.Parent != nil {
			kids[rec.Parent.ReplyID] = append(kids[rec.Parent.ReplyID], rec.ReplyID)
		}
	}
	quiet := func(rec Record) bool {
		mark := rec.Voided
		if rec.LastTurn != nil {
			mark = max(mark, rec.LastTurn.Mark)
		}
		return !rec.Lead && rec.Parent != nil && rec.HolderID == "" && !rec.TurnOpen && rec.HeldTurn == nil &&
			rec.Sends <= mark && len(rec.Queued)+len(rec.Mail)+len(rec.Reports)+len(rec.Steered)+len(rec.PendingReports) == 0
	}
	seen := map[string]bool{}
	var subtree func(id string, out *[]string) bool
	subtree = func(id string, out *[]string) bool {
		if seen[id] {
			return false
		}
		seen[id] = true
		ok := true
		for _, k := range kids[id] {
			ok = subtree(k, out) && quiet(byID[k]) && ok
		}
		*out = append(*out, id)
		return ok
	}
	var out []string
	var visit func(id string)
	visit = func(id string) {
		rec := byID[id]
		if at := lastActive(rec); quiet(rec) && !at.IsZero() && now.Sub(at) >= expiry {
			var tree []string
			saved := maps.Clone(seen)
			if subtree(id, &tree) {
				out = append(out, tree...)
				return
			}
			seen = saved
		}
		seen[id] = true
		for _, k := range kids[id] {
			if !seen[k] {
				visit(k)
			}
		}
	}
	for _, rec := range open {
		if rec.Parent == nil || byID[rec.Parent.ReplyID].ReplyID == "" {
			visit(rec.ReplyID)
		}
	}
	return out
}

// agentTokenRequest is the token route's body for the agent.* ops.
type agentTokenRequest struct {
	AgentSpawnRequest
	Address string `json:"address"`
}

// serveAgentToken serves a token holder's agent.* op (serveToken).
func (r *Runner) serveAgentToken(w http.ResponseWriter, req *http.Request, caller *conv, op string) {
	var body agentTokenRequest
	if !decode(w, req, &body) {
		return
	}
	ac := r.holderCaller(caller.record())
	switch op {
	case "agent.spawn":
		res, err := r.AgentSpawn(ac, body.AgentSpawnRequest)
		writeAnswer(w, http.StatusCreated, res, err)
	case "agent.interrupt":
		res, err := r.AgentInterrupt(ac, body.Address)
		writeAnswer(w, http.StatusOK, res, err)
	case "agent.list":
		writeAnswer(w, http.StatusOK, map[string]any{"agents": r.AgentList(ac)}, nil)
	case "agent.tiers":
		res, err := r.AgentTiers(ac)
		writeAnswer(w, http.StatusOK, res, err)
	default:
		http.Error(w, "unknown agent route", http.StatusNotFound)
	}
}
