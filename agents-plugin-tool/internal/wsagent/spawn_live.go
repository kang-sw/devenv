package wsagent

import (
	"encoding/json"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
)

// SpawnAgentEvent projects a daemon conversation onto the ordinary subagent
// feed. The prefix separates daemon ids from the harness's own ids.
func SpawnAgentEvent(st State, key string) wslive.Event {
	status := wslive.StatusDone
	switch {
	case st.Status == StatusClosed:
		// Closed records have no live diagnostic; closing settles the row
		// identically before and after its loop leaves the runner.
		status = wslive.StatusDone
	case st.Status == StatusRunning || (st.Status != StatusClosed && st.Held > 0):
		status = wslive.StatusRunning
	case st.Error != "":
		status = wslive.StatusFailed
	}
	label := st.Label
	if label == "" {
		label = "Pi subagent " + st.ID
	}
	ev := wslive.Event{Kind: wslive.KindAgent, AgentID: "pi:" + st.ID,
		AgentKind: wslive.AgentKindPi, Label: label, Status: status,
		Model: st.Model, Effort: st.Effort, Observed: st.Observed, SessionKey: key,
		ContextTokens: st.ContextTokens, OutputTps: st.OutputTps,
		ActiveMs: st.ActiveMs}
	if st.Status == StatusRunning {
		ev.TurnStartedAt = st.TurnStartedAt
	}
	// An explicit empty list clears the previous activity on the row.
	ev.Batches = make([]json.RawMessage, 0, len(st.Batches))
	for _, batch := range st.Batches {
		raw, _ := json.Marshal(batch)
		ev.Batches = append(ev.Batches, raw)
	}
	if st.Parent != nil && st.Parent.Kind == ParentConversation {
		ev.ParentID = "pi:" + st.Parent.ReplyID
	}
	return ev
}

// SpawnAgentEvents reconciles all own response evidence after a coalesced
// parent wake or holder replay, while projecting the lifecycle only once.
func SpawnAgentEvents(st State, key string) []wslive.Event {
	row := SpawnAgentEvent(st, key)
	if len(st.Observations) == 0 {
		return []wslive.Event{row}
	}
	row.Observed = nil
	out := []wslive.Event{row}
	for _, o := range st.Observations {
		ev := row
		ev.Batches, ev.Model, ev.Effort, ev.ContextTokens, ev.OutputTps = nil, "", "", nil, nil
		observed := o
		ev.Observed = &observed
		out = append(out, ev)
	}
	return out
}

// Lineage is SpawnLead's lookup: id's state from its record alone, open or
// closed, so only its identity and spawn edge (ID, Parent) are meaningful.
// Unlike Get it reads no file: Get relists the conversation's inbox for its
// held count, and a watcher walking the chain on every descendant event made
// that the daemon's hottest directory listing.
func (r *Runner) Lineage(id string) (State, error) {
	rec, ok := r.recordOf(id)
	if !ok {
		return State{}, errUnknown
	}
	return closedState(rec), nil
}

// SpawnLead traces a spawned conversation to its owning lead, failing closed
// for an unknown or malformed parent chain. get must be a read-only lookup.
func SpawnLead(st State, get func(string) (State, error)) string {
	seen := map[string]bool{st.ID: true}
	for st.Parent != nil {
		p := *st.Parent
		switch p.Kind {
		case ParentLead:
			return p.ReplyID
		case ParentConversation:
			if seen[p.ReplyID] {
				return ""
			}
			seen[p.ReplyID] = true
			next, err := get(p.ReplyID)
			if err != nil {
				return ""
			}
			st = next
		default:
			return ""
		}
	}
	return ""
}

// SpawnTree lists parent's descendants in parent-before-child order, including
// closed records so an already displayed running row can settle on close.
func SpawnTree(parent ParentRef, children func(ParentRef) []State) []State {
	var out []State
	seen := map[string]bool{parent.ReplyID: true}
	var walk func(ParentRef)
	walk = func(p ParentRef) {
		for _, st := range children(p) {
			if seen[st.ID] {
				continue
			}
			seen[st.ID] = true
			out = append(out, st)
			walk(ParentRef{Kind: ParentConversation, ReplyID: st.ID})
		}
	}
	walk(parent)
	return out
}

// seedSpawnRows runs only in the owning conversation's loop (or before it
// starts). Its writer stays the sole snapshot writer; hosted intake stays off.
func (c *conv) seedSpawnRows() {
	if !c.spawnDirty {
		return
	}
	now := c.r.opts.Now()
	if now.Before(c.spawnNext) {
		// Keep the invalidation until the trailing refresh: no further child
		// event or 30-second heartbeat is required to publish the latest rows.
		if c.spawnTimer == nil {
			c.spawnTimer = time.NewTimer(c.spawnNext.Sub(now))
		}
		return
	}
	if c.spawnTimer != nil {
		c.spawnTimer.Stop()
		c.spawnTimer = nil
	}
	c.spawnNext = now.Add(time.Second)
	c.spawnDirty = false
	kind := ParentConversation
	if c.rec.Lead {
		kind = ParentLead
	}
	known := map[string]bool{}
	for _, a := range c.live.st.Snapshot(c.r.opts.Now().UnixMilli()).Agents {
		known[a.ID] = true
	}
	// Walk only the indexed descendants; child events never list all
	// conversations or read every conversation's inbox.
	if c.r.opts.spawnListed != nil {
		c.r.opts.spawnListed(c.rec.ReplyID)
	}
	for _, st := range SpawnTree(ParentRef{Kind: kind, ReplyID: c.rec.ReplyID}, c.r.Children) {
		if st.Status == StatusClosed && !known["pi:"+st.ID] {
			continue
		}
		rec, ok := c.r.recordOf(st.ID)
		if ok {
			for _, ev := range SpawnAgentEvents(st, rec.WSKey) {
				c.live.applyQuiet(ev)
			}
		}
	}
}

// wakeSpawnParents never waits on another conversation's loop. A parent
// refresh reads current records rather than a queued, potentially stale event.
func (r *Runner) wakeSpawnParents(st State) {
	seen := map[string]bool{st.ID: true}
	for st.Parent != nil {
		p := *st.Parent
		if seen[p.ReplyID] {
			return
		}
		seen[p.ReplyID] = true
		if c := r.conv(p.ReplyID); c != nil {
			select {
			case c.spawnC <- struct{}{}:
			default:
			}
		}
		if p.Kind != ParentConversation {
			return
		}
		next, ok := r.recordOf(p.ReplyID)
		if !ok {
			return
		}
		st = closedState(next) // only the immutable parent edge is needed
	}
}
