// Tracker batch planning and writer defaults.
package mcp

import (
	"encoding/json"
	"strings"

	"github.com/kang-sw/devenv/internal/wsledger"
	"github.com/kang-sw/devenv/internal/wstracker"
	"github.com/kang-sw/devenv/internal/wstrackerhost"
	"github.com/kang-sw/devenv/internal/wsxmlcmd"
)

func planTrackerBatch(p *wsxmlcmd.Problems, commands string, registered []wstrackerhost.StoreEntry, reach func(store string) (map[string]int, error)) *trackerBatch {
	b := &trackerBatch{p: p, byPos: map[int]wsxmlcmd.Command{}, eventOf: map[int]int{}}
	b.cmds = wsxmlcmd.Parse(p, trackerGrammar, commands)
	if len(b.cmds) == 0 && len(p.List) == 0 {
		p.Add("commands: no %s command given", trackerPrefix)
	}
	known := map[string]bool{}
	ids := make([]string, 0, len(registered))
	for _, e := range registered {
		known[e.ID] = true
		ids = append(ids, e.ID)
	}
	unknownStore := func(at, store string) {
		if len(ids) == 0 {
			p.Add("%s: no store is registered; register one with <%s%s>", at, trackerPrefix, trackerRegister)
			return
		}
		p.Add("%s: %q is not a registered store (registered: %s)", at, store, strings.Join(ids, ", "))
	}
	// Number the event commands first, so a $N can name a later-parsed
	// command's event index while it is planned.
	for _, cmd := range b.cmds {
		b.byPos[cmd.Pos] = cmd
		name := strings.TrimPrefix(cmd.Tag, trackerPrefix)
		if trackerCommands[name].build != nil {
			b.eventOf[cmd.Pos] = len(b.eventOf)
		}
	}
	groups := map[*wsxmlcmd.Scope]bool{}
	for i := range b.cmds {
		cmd := b.cmds[i]
		name := strings.TrimPrefix(cmd.Tag, trackerPrefix)
		def := trackerCommands[name]
		if def.build == nil {
			if len(b.cmds) > 1 {
				if name == trackerRegister || name == trackerSync {
					p.Add("%s: reaches the network and cannot be rolled back, so it stands alone in its batch", cmd.Label())
				} else {
					p.Add("%s: changes this machine's store registry, not the store, and cannot be rolled back with a batch, so it stands alone in its batch", cmd.Label())
				}
			}
			b.storeCmd = &b.cmds[i]
			switch name {
			case trackerRegister:
				if store, ok := wsxmlcmd.RequiredAttr(p, cmd, "store"); ok && known[store] {
					p.Add("%s: %q is already registered on this machine", cmd.At("store"), store)
				}
				wsxmlcmd.RequiredAttr(p, cmd, "member")
			case trackerSync:
				if store, ok := cmd.Attr("store"); ok {
					if !known[store] {
						unknownStore(cmd.At("store"), store)
					}
				} else if len(ids) == 0 {
					p.Add("%s: no store is registered", cmd.Label())
				}
			case trackerUnregister, trackerRelabel, trackerHide, trackerUnhide:
				if store, ok := wsxmlcmd.RequiredAttr(p, cmd, "store"); ok && !known[store] {
					unknownStore(cmd.At("store"), store)
				}
				// label="" clears the label, so only its absence is refused.
				if _, ok := cmd.Attr("label"); name == trackerRelabel && !ok {
					p.Add("%s: required (label=\"\" clears the label back to the store id)", cmd.At("label"))
				}
			}
			continue
		}
		ev := &trackerEvent{cmd: cmd, typ: name, origin: wstracker.OriginAgent, payload: map[string]any{}, refs: map[string]int{}, refLists: map[string][]trackerListRef{}}
		// The parser has refused a command outside a group and a group
		// with no name; each group is checked once, at its first command.
		if scope := cmd.Scope; scope != nil && !groups[scope] {
			switch {
			case len(groups) > 0:
				p.Add("%s: a second <%sstore> group; one batch writes to one store, so it holds one group", scope.Label(), trackerPrefix)
			case scope.Value == "":
			case !known[scope.Value]:
				unknownStore(scope.At(), scope.Value)
			default:
				b.store = scope.Value
			}
			groups[scope] = true
		}
		if _, ok := cmd.Attr("origin"); ok {
			if v, ok := wsxmlcmd.EnumAttr(p, cmd, "origin", trackerOrigins); ok {
				ev.origin = v
			}
		}
		def.build(b, cmd, ev)
		b.events = append(b.events, ev)
	}
	for _, e := range registered {
		if e.ID == b.store && e.Member != "" && len(p.List) == 0 {
			b.writerDefaults(e.Member, func() (map[string]int, error) { return reach(e.ID) })
		}
	}
	return b
}

// writerDefaults fills what the writer, member writer of the batch's
// store, decides for an agent as the dashboard does for a person. An
// item.assign without completion gets it stored explicitly, by the
// writer's relationship to each target (completionClasses); targets of both
// classes are written as one item.assign per class, the second right after
// the first. An explicit completion is kept as written: the store refuses
// assignee for a target below the writer (wstracker checkWrite), and that
// refusal reaches the caller instead of a silent rewrite. The fold's
// absent-means-assignee rule stays for old events. A
// topic created that no item.assign of the batch names by $N is
// self-assigned (people = the writer) by an item.assign inserted right after
// its create, so the writer holds it, as on the dashboard. Every event index
// is renumbered to match an inserted event. A task needs no self-assignment:
// the store holds it for its creator until someone else holds it.
//
// reach returns the members below the writer in the store's org tree (any
// depth); it is read once, and only for an assign that names someone other
// than the writer.
func (b *trackerBatch) writerDefaults(writer string, reach func() (map[string]int, error)) {
	var below map[string]int
	readErr, read := error(nil), false
	subordinate := func(id string) bool {
		if id == writer {
			return false
		}
		if !read {
			read = true
			below, readErr = reach()
		}
		_, ok := below[id]
		return ok
	}
	assigned := map[int]bool{}
	split := map[int]*trackerEvent{} // the event split off right after event i
	for i, ev := range b.events {
		if ev.typ != wstracker.TypeItemAssign {
			continue
		}
		if idx, ok := ev.refs["item"]; ok {
			assigned[idx] = true
		}
		if _, ok := ev.payload["completion"]; ok {
			continue
		}
		if x := completionClasses(ev, writer, subordinate); x != nil {
			split[i] = x
		}
		if readErr != nil {
			b.p.Add("%s: the org tree, which picks the default completion, could not be read (%s); set completion explicitly", ev.cmd.Label(), wstrackerhost.RedactCredentials(readErr.Error()))
			return
		}
	}
	events := make([]*trackerEvent, 0, len(b.events))
	moved := make([]int, len(b.events))
	for i, ev := range b.events {
		moved[i] = len(events)
		events = append(events, ev)
		if x := split[i]; x != nil {
			events = append(events, x)
		}
		if ev.typ != wstracker.TypeItemCreate || ev.payload["kind"] != "topic" || assigned[i] {
			continue
		}
		events = append(events, &trackerEvent{
			cmd: ev.cmd, typ: wstracker.TypeItemAssign, origin: ev.origin, selfAssign: true,
			payload:  map[string]any{"mode": "chosen", "people": []string{writer}, "completion": "assignee"},
			refs:     map[string]int{"item": i},
			refLists: map[string][]trackerListRef{},
		})
	}
	if len(events) == len(b.events) {
		return
	}
	for _, ev := range events {
		for field, idx := range ev.refs {
			ev.refs[field] = moved[idx]
		}
		for _, list := range ev.refLists {
			for j := range list {
				if list[j].id == "" {
					list[j].event = moved[list[j].event]
				}
			}
		}
	}
	for pos, idx := range b.eventOf {
		b.eventOf[pos] = moved[idx]
	}
	b.events = events
}

// completionClasses stores the default completion of an item.assign that
// sets none, which follows the writer's relationship to each target: a
// member below the writer in the org tree, or a rule rooted at the writer
// or below them (levels is one rooted at the writer), takes assigner, so a
// close goes through review; the writer themself, a peer, or a rule rooted
// elsewhere takes assignee, closing directly. When the targets fall in both
// classes, ev keeps the class of its first target and the rest are moved to
// the event it returns, which carries the same command, origin, and item;
// otherwise it returns nil.
func completionClasses(ev *trackerEvent, writer string, subordinate func(string) bool) *trackerEvent {
	class := func(target any) string {
		id, _ := target.(string)
		if rule, ok := target.(map[string]any); ok {
			if id, _ = rule["root"].(string); id == writer {
				return "assigner"
			}
		}
		if subordinate(id) {
			return "assigner"
		}
		return "assignee"
	}
	field := "targets"
	var targets []any
	switch {
	case ev.payload["mode"] == "all":
		ev.payload["completion"] = "assigner"
		return nil
	case ev.payload["mode"] == "chosen":
		field = "people"
		people, _ := ev.payload["people"].([]string)
		for _, p := range people {
			targets = append(targets, p)
		}
	default:
		targets, _ = ev.payload["targets"].([]any)
	}
	if len(targets) == 0 {
		return nil
	}
	first := class(targets[0])
	var keep, rest []any
	for _, t := range targets {
		if class(t) == first {
			keep = append(keep, t)
		} else {
			rest = append(rest, t)
		}
	}
	ev.payload["completion"] = first
	if len(rest) == 0 {
		return nil
	}
	list := func(l []any) any {
		if field != "people" {
			return l
		}
		out := make([]string, len(l))
		for i, x := range l {
			out[i] = x.(string)
		}
		return out
	}
	ev.payload[field] = list(keep)
	x := &trackerEvent{cmd: ev.cmd, typ: ev.typ, origin: ev.origin, splitOff: true,
		payload: map[string]any{}, refs: map[string]int{}, refLists: map[string][]trackerListRef{}}
	for k, v := range ev.payload {
		x.payload[k] = v
	}
	for k, v := range ev.refs {
		x.refs[k] = v
	}
	x.payload[field] = list(rest)
	x.payload["completion"] = "assignee"
	if first == "assignee" {
		x.payload["completion"] = "assigner"
	}
	return x
}

// newEvents builds the batch's events for a store whose next event gets
// next, resolving each $N to the id its event gets.
func (b *trackerBatch) newEvents(next wsledger.EventID) ([]wsledger.NewEvent, error) {
	out := make([]wsledger.NewEvent, len(b.events))
	for i, ev := range b.events {
		payload := make(map[string]any, len(ev.payload)+len(ev.refs)+len(ev.refLists))
		for k, v := range ev.payload {
			payload[k] = v
		}
		at := func(idx int) string {
			id := next
			id.Seq += idx
			return id.String()
		}
		for field, idx := range ev.refs {
			payload[field] = at(idx)
		}
		for field, list := range ev.refLists {
			ids := make([]string, len(list))
			for i, r := range list {
				ids[i] = r.id
				if r.id == "" {
					ids[i] = at(r.event)
				}
			}
			payload[field] = ids
		}
		raw, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		out[i] = wsledger.NewEvent{Type: ev.typ, Origin: ev.origin, Payload: raw}
	}
	return out, nil
}
