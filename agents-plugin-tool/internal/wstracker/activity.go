package wstracker

import (
	"sort"
	"strings"
	"time"

	"github.com/kang-sw/devenv/internal/wsledger"
)

// Activity kinds: what an applied event did that concerns members.
const (
	// ActAssigned: an assignment became live for its target at once (a
	// forced assignment, or a rule's record).
	ActAssigned = "assigned"
	// ActAssignPending: a pending assignment to its target (a request, a
	// handoff, or a rule request to the rule's root).
	ActAssignPending = "assign_pending"
	// ActComment: a comment was added to the item.
	ActComment = "comment"
	// ActStatus: an assignment's status was set (item.status, item.reject,
	// item.reopen).
	ActStatus = "status"
	// ActClosed: an assignment was closed or confirmed (item.close,
	// item.confirm); Status says done or awaiting.
	ActClosed = "closed"
)

// Activity is one thing an applied event did that concerns members, judged
// against the state folded from the event's causal past, so the answer for
// an event never changes as later events arrive.
type Activity struct {
	Event wsledger.EventID
	TS    time.Time
	Kind  string
	Item  string
	// Actor is the event's author.
	Actor string
	// Target is the member the activity is aimed at: an assignment's
	// assignee, or the assignee a confirm, reject, or reopen names; empty
	// when there is none.
	Target string
	// Status is the status the event wrote (ActStatus, ActClosed).
	Status string
	// Comment is the added comment's body (ActComment).
	Comment string
	// Concerns are the members the activity concerns, sorted: an
	// assignment's target; otherwise the item's creator, the members
	// holding a live assignment on it just before the event, and Target.
	Concerns []string
}

// Activity returns what the applied event id did that concerns members.
// settled is false while the event's verdict may still change to applied
// (it is waiting, or this clone does not hold it): it then has no activity
// yet. A flagged event is settled with none. Only item.* events other than
// item.create, and comment.add, have activity.
func (t *Tracker) Activity(id wsledger.EventID) (acts []Activity, settled bool) {
	t.Refresh()
	r := t.r
	n := r.byID[id.String()]
	if n == nil {
		return nil, false
	}
	switch n.status {
	case "applied":
	case "waiting":
		return nil, false
	default:
		return nil, true
	}
	itemID := n.p.str("item")
	if itemID == "" || n.typ == TypeItemCreate || (n.typ != TypeCommentAdd && !isItemEvent(n.typ)) {
		return nil, true
	}
	S := r.stateBefore(n)
	creator := ""
	if it := S.items[itemID]; it != nil {
		creator = it.author
	}
	known := map[string]bool{}
	var holders []string
	for _, x := range S.recs {
		known[x.id] = true
		if x.item == itemID && x.state == "live" && !x.ruleRequest {
			holders = append(holders, x.to)
		}
	}
	base := Activity{Event: n.ev.ID, TS: n.ts, Item: itemID, Actor: n.member}
	about := func(a Activity) Activity {
		a.Concerns = sortedSet(append([]string{creator, a.Target}, holders...))
		return a
	}

	switch n.typ {
	case TypeCommentAdd:
		a := base
		a.Kind, a.Comment = ActComment, jsString(n.p["body"])
		acts = append(acts, about(a))
	case TypeItemStatus, TypeItemClose, TypeItemConfirm, TypeItemReject, TypeItemReopen:
		// The status the event writes, as apply computes it.
		who := n.member
		if n.typ != TypeItemStatus && n.typ != TypeItemClose && n.p.truthy("assignee") {
			who = n.p.str("assignee")
		}
		a := base
		if who != n.member {
			a.Target = who
		}
		switch n.typ {
		case TypeItemStatus:
			a.Kind, a.Status = ActStatus, n.p.str("status")
		case TypeItemClose:
			a.Kind, a.Status = ActClosed, "done"
			if x := S.liveRec(itemID, who); x != nil && x.completion == "assigner" {
				a.Status = "awaiting"
			}
		case TypeItemConfirm:
			a.Kind, a.Status = ActClosed, "done"
		default:
			a.Kind, a.Status = ActStatus, "doing"
		}
		acts = append(acts, about(a))
	}

	// The assignments the event made: records new after it, the fallback
	// (the creator's own default holding) left out.
	S.apply(n)
	for _, x := range S.recs {
		if known[x.id] || x.fallback {
			continue
		}
		a := base
		a.Item, a.Target = x.item, x.to
		a.Kind = ActAssigned
		if x.state == "pending" || x.ruleRequest {
			a.Kind = ActAssignPending
		}
		a.Concerns = []string{x.to}
		acts = append(acts, a)
	}
	return acts, true
}

// isItemEvent reports an item.* event type.
func isItemEvent(t string) bool { return strings.HasPrefix(t, "item.") }

// sortedSet returns the sorted, duplicate-free members of l without "".
func sortedSet(l []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range l {
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}
