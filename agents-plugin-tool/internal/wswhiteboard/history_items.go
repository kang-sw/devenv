package wswhiteboard

import (
	"fmt"
	"strings"
)

// historyTransition recognizes only attributed, server-written settlement
// events. A later content-only patch is not a new answer/settlement, and a
// mutable item clock is never a substitute for a missing historical event.
func historyTransition(e EntryView, item ItemView) bool {
	if e.Kind != EntrySystem || e.ItemID != item.ID {
		return false
	}
	prefix := fmt.Sprintf("Item {{%d}} ", item.ID)
	switch item.Status {
	case StatusAnswered:
		if strings.HasPrefix(e.Text, prefix+"answered [") {
			return true
		}
	case StatusAssumed:
		if strings.HasPrefix(e.Text, prefix+"assumed [") {
			return true
		}
	case StatusWithdrawn:
		if strings.HasPrefix(e.Text, prefix+"withdrawn [") {
			return true
		}
	}
	return strings.HasPrefix(e.Text, prefix+"patched [") &&
		strings.Contains(e.Text, " -> "+item.Status+"):") &&
		!strings.Contains(e.Text, "("+item.Status+" -> "+item.Status+"):")
}

// projectHistoryItems decorates a recorded transition in place; it does not
// timestamp-sort entries or pretend their current item is a historical
// snapshot. Unmatched legacy items form an explicitly undated prefix in the
// store's existing item order. Their existing item-N DOM anchors, not made-up
// event IDs, let paging/reload/navigation preserve that order too.
func projectHistoryItems(all []EntryView, items []ItemView) []EntryView {
	var undated []EntryView
	for i := range items {
		item := &items[i]
		at := -1
		for j := range all {
			if historyTransition(all[j], *item) {
				at = j
			}
		}
		if at >= 0 {
			all[at].Item = item
		} else {
			undated = append(undated, EntryView{Entry: Entry{ID: fmt.Sprintf("item-%d", item.ID), Kind: "item"}, Item: item})
		}
	}
	return append(undated, all...)
}

// coalesceCreated is called only by the accepting Apply that knows exactly
// which creation events it just produced. It never examines old timestamps or
// rewrites an existing log. The existing system-entry shape carries one new
// batch line; gaps in the monotonic entry counter are not batch identifiers.
func coalesceCreated(work *State, created map[string]Item) {
	for i := range work.Topics {
		doc := &work.Topics[i]
		var out []Entry
		for at := 0; at < len(doc.Entries); {
			end := at
			var names []string
			for end < len(doc.Entries) {
				item, ok := created[doc.Entries[end].ID]
				if !ok {
					break
				}
				names = append(names, fmt.Sprintf("%s {{%d}}", item.Class, item.ID))
				end++
			}
			if len(names) > 1 {
				e := doc.Entries[at]
				e.ItemID = 0 // a batch has no single trusted item attribution
				e.Text = "Items added: " + strings.Join(names, ", ") + "."
				out = append(out, e)
				at = end
			} else {
				out = append(out, doc.Entries[at])
				at++
			}
		}
		doc.Entries = out
	}
}
