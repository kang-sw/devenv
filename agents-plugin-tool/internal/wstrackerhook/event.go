package wstrackerhook

import (
	"net/url"
	"time"

	"github.com/kang-sw/devenv/internal/wstracker"
	"github.com/kang-sw/devenv/internal/wstrackerhost"
)

// EventVersion is the event JSON's v. Adding a field keeps it; changing a
// field's meaning bumps it. A new kind keeps it too: a script ignores a
// kind it does not know.
const EventVersion = 1

// KindDueSoon is the kind of the clock's event: an item's due date is
// within the threshold. The other kinds are wstracker's Act* kinds.
const KindDueSoon = "due_soon"

// commentLimit is how many characters of a comment detail.comment keeps.
const commentLimit = 500

// Event is the JSON a hook command reads on its stdin, one per run.
type Event struct {
	V int `json:"v"`
	// ID is the ledger event's id; a due_soon event's is
	// "due:<item id>:<due date>". One ledger event that assigns several
	// members gives one event per member, each with this id.
	ID    string   `json:"id"`
	Kind  string   `json:"kind"`
	TS    string   `json:"ts"`
	Store StoreRef `json:"store"`
	Item  ItemRef  `json:"item"`
	// Actor is the member whose event it is; null for due_soon.
	Actor  *Member `json:"actor"`
	Target *Member `json:"target,omitempty"`
	Detail Detail  `json:"detail"`
	// URL opens the item in this machine's dashboard.
	URL string `json:"url"`
}

// StoreRef names the event's store.
type StoreRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ItemRef names the event's item.
type ItemRef struct {
	ID    string `json:"id"`
	Alias string `json:"alias"`
	Title string `json:"title"`
	Kind  string `json:"kind"`
}

// Member is a member of the store.
type Member struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Detail carries what the kind says: status for status and closed, the
// comment's first characters for comment, and the item's due date whenever
// it has one.
type Detail struct {
	Status  string `json:"status,omitempty"`
	Comment string `json:"comment,omitempty"`
	Due     string `json:"due,omitempty"`
}

// candidate is one event the hooks may receive, before the scope filter.
type candidate struct {
	ev    Event
	actor string
	// concerns are the members the event concerns (the mine filter).
	concerns []string
}

// accepts reports whether hook h receives c in a store whose local member
// is member.
func (h Hook) accepts(c candidate, member string) bool {
	if h.Scope == ScopeAll {
		return true
	}
	if c.actor == member {
		return false
	}
	for _, m := range c.concerns {
		if m == member {
			return true
		}
	}
	return false
}

// eventBase fills what every event of item id in the store carries.
func eventBase(e wstrackerhost.StoreEntry, v *wstracker.View, item, base string, loc *time.Location) Event {
	name := e.Label
	if name == "" {
		name = e.ID
	}
	ev := Event{V: EventVersion, Store: StoreRef{ID: e.ID, Name: name}, Item: ItemRef{ID: item}}
	if it, ok := v.Items[item]; ok {
		ev.Item.Alias, ev.Item.Title, ev.Item.Kind = it.Alias, it.Title, it.Kind
		if it.Due != nil {
			if at, ok := dueTime(*it.Due, loc); ok {
				ev.Detail.Due = at.Format(time.RFC3339)
			}
		}
	}
	if base != "" {
		ev.URL = base + "/#/" + url.PathEscape(e.ID) + "/" + url.PathEscape(item)
	}
	return ev
}

// memberRef is member id as an event names it.
func memberRef(v *wstracker.View, id string) *Member {
	if id == "" {
		return nil
	}
	m := &Member{ID: id, Name: id}
	if mv, ok := v.Members[id]; ok && mv.Name != "" {
		m.Name = mv.Name
	}
	return m
}

// activityCandidate is the event of one wstracker.Activity.
func activityCandidate(e wstrackerhost.StoreEntry, v *wstracker.View, a wstracker.Activity, base string, loc *time.Location) candidate {
	ev := eventBase(e, v, a.Item, base, loc)
	ev.ID, ev.Kind, ev.TS = a.Event.String(), a.Kind, a.TS.UTC().Format(time.RFC3339)
	ev.Actor, ev.Target = memberRef(v, a.Actor), memberRef(v, a.Target)
	ev.Detail.Status = a.Status
	if r := []rune(a.Comment); len(r) > commentLimit {
		ev.Detail.Comment = string(r[:commentLimit])
	} else {
		ev.Detail.Comment = a.Comment
	}
	return candidate{ev: ev, actor: a.Actor, concerns: a.Concerns}
}

// dueTime is the instant a due date (YYYY-MM-DD) names: its start in loc.
func dueTime(due string, loc *time.Location) (time.Time, bool) {
	t, err := time.ParseInLocation("2006-01-02", due, loc)
	return t, err == nil
}
