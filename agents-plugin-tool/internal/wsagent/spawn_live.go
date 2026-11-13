package wsagent

import "github.com/kang-sw/devenv/internal/wslive"

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
		Model: st.Model, Effort: st.Effort, SessionKey: key,
		ContextTokens: st.ContextTokens, OutputTps: st.OutputTps}
	if st.Parent != nil && st.Parent.Kind == ParentConversation {
		ev.ParentID = "pi:" + st.Parent.ReplyID
	}
	return ev
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
	c.spawnDirty = false
	kind := ParentConversation
	if c.rec.Lead {
		kind = ParentLead
	}
	known := map[string]bool{}
	for _, a := range c.live.st.Snapshot(c.r.opts.Now().UnixMilli()).Agents {
		known[a.ID] = true
	}
	// One global listing per invalidation, not one filesystem-backed List
	// per descendant, and never discovery on ordinary streaming snapshots.
	if c.r.opts.spawnListed != nil {
		c.r.opts.spawnListed(c.rec.ReplyID)
	}
	byParent := map[ParentRef][]State{}
	for _, st := range c.r.List() {
		if st.Parent != nil {
			byParent[*st.Parent] = append(byParent[*st.Parent], st)
		}
	}
	for _, st := range SpawnTree(ParentRef{Kind: kind, ReplyID: c.rec.ReplyID}, func(p ParentRef) []State { return byParent[p] }) {
		if st.Status == StatusClosed && !known["pi:"+st.ID] {
			continue
		}
		rec, ok := c.r.recordOf(st.ID)
		if ok {
			c.live.applyQuiet(SpawnAgentEvent(st, rec.WSKey))
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
		next, err := r.Get(p.ReplyID)
		if err != nil {
			return
		}
		st = next
	}
}
