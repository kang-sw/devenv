package mcp

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/kang-sw/devenv/internal/wstracker"
	"github.com/kang-sw/devenv/internal/wstrackerhost"
)

// The tracker.query text renderers. Every ID is printed in the form
// tracker.update takes back (maya.pc-5); people are member ids, with the
// display name where the row introduces a person.

// formatTrackerScope is a store's header, then the rows of q.scope that
// pass q's filters.
func formatTrackerScope(v *wstrackerhost.StoreView, q trackerQuery) string {
	var sb strings.Builder
	writeTrackerHeader(&sb, v)
	keep := func(itemID string) bool { return trackerItemMatches(v, itemID, q) }
	rows := 0
	section := func(title string) { fmt.Fprintf(&sb, "\n%s:\n", title) }
	assignments := func(title string, ids []string) {
		section(title)
		n := 0
		for _, id := range ids {
			if a, ok := v.Assignments[id]; ok && keep(a.Item) {
				sb.WriteString("- " + formatTrackerAssignmentRow(v, a) + "\n")
				n++
			}
		}
		if n == 0 {
			sb.WriteString("(none)\n")
		}
		rows += n
	}
	items := func(title string, ids []string, withAssignments bool) {
		section(title)
		n := 0
		for _, id := range ids {
			if !keep(id) {
				continue
			}
			it := v.Items[id]
			sb.WriteString(strings.Repeat("  ", trackerIndent(q.scope, it)) + "- " + formatTrackerItemRow(it) + "\n")
			if withAssignments {
				for _, aid := range it.Assignments {
					if a, ok := v.Assignments[aid]; ok && a.Ended == nil {
						sb.WriteString("  - " + formatTrackerAssignment(v, a) + "\n")
					}
				}
			}
			n++
		}
		if n == 0 {
			sb.WriteString("(none)\n")
		}
		rows += n
	}
	switch q.scope {
	case "received":
		assignments("sent to me, waiting for my answer", v.My.Received.Pending)
		assignments("answered by me", v.My.Received.Answered)
		assignments("sent by me, not forced", v.My.Received.Sent)
	case "assigned":
		items("items I assigned or handed to others, with open assignments", v.My.Given, true)
	case "team":
		if len(v.My.Team) == 0 {
			sb.WriteString("\nteam: no followers\n")
		}
		for _, t := range v.My.Team {
			head := fmt.Sprintf("%s (depth %d): %d done, %d blocked, %d stale", trackerPerson(v, t.ID), t.Depth, t.Done, t.Blocked, t.Stale)
			if t.Last != nil {
				head += ", last update " + *t.Last
			}
			fmt.Fprintf(&sb, "\n%s\n", head)
			for _, group := range []struct {
				label string
				ids   []string
			}{{"open", t.Open}, {"pending", t.Pending}, {"waiting on others", t.Waiting}} {
				for _, id := range group.ids {
					if a, ok := v.Assignments[id]; ok && keep(a.Item) {
						fmt.Fprintf(&sb, "- %s: %s\n", group.label, formatTrackerAssignmentRow(v, a))
						rows++
					}
				}
			}
		}
	case "all":
		items("every item, in tree order", v.ItemOrder, false)
	default:
		assignments("my assignments", v.My.Assignments)
		items("created by me, assigned to nobody", v.My.Created, false)
	}
	if rows == 0 && (q.kind != "" || q.status != "" || q.project != "" || q.text != "") {
		sb.WriteString("\nno rows pass the filters\n")
	}
	return sb.String()
}

// trackerIndent is an item row's tree depth in scope all, else 0.
func trackerIndent(scope string, it wstracker.ItemView) int {
	if scope == "all" {
		return it.Depth
	}
	return 0
}

// writeTrackerHeader writes the store, viewer, sync, priority, and today
// lines every store view starts with.
func writeTrackerHeader(sb *strings.Builder, v *wstrackerhost.StoreView) {
	fmt.Fprintf(sb, "store %s", v.Store.Name)
	if v.Store.Label != "" && v.Store.Label != v.Store.Name {
		fmt.Fprintf(sb, " (%s)", v.Store.Label)
	}
	viewer := v.Viewer.Member
	if m, ok := v.Members[viewer]; ok && m.Name != "" && m.Name != viewer {
		viewer += " (" + m.Name + ")"
	}
	fmt.Fprintf(sb, ", viewer %s", viewer)
	if v.Viewer.Admin {
		sb.WriteString(", admin")
	}
	if !v.Viewer.IsMember {
		sb.WriteString(", not a member yet")
	}
	sb.WriteString("\n")
	fmt.Fprintf(sb, "sync: %s\n", formatTrackerSyncLine(v))
	levels := make([]string, len(v.Priority.Names))
	for i, name := range v.Priority.Names {
		levels[i] = fmt.Sprintf("%d %s", i, name)
		if i == v.Priority.Default {
			levels[i] += " (default)"
		}
	}
	fmt.Fprintf(sb, "priority: %s\n", strings.Join(levels, ", "))
	t := v.My.Today
	fmt.Fprintf(sb, "today: my work %d, to answer %d, to confirm %d\n", len(t.Work), len(t.Pending), len(t.Confirms))
	for _, p := range v.RolesProblems {
		fmt.Fprintf(sb, "roles problem: %s\n", p)
	}
}

// trackerItemMatches reports whether an item passes q's filters. A hidden
// item passes only status hidden.
func trackerItemMatches(v *wstrackerhost.StoreView, id string, q trackerQuery) bool {
	it, ok := v.Items[id]
	if !ok {
		return false
	}
	status := trackerItemStatus(it)
	switch q.status {
	case "hidden":
		if !it.Hidden {
			return false
		}
	case "":
		if it.Hidden {
			return false
		}
	case "open":
		if it.Hidden || status == "done" {
			return false
		}
	default:
		if it.Hidden || status != q.status {
			return false
		}
	}
	if q.kind != "" && it.Kind != q.kind {
		return false
	}
	if q.project != "" && it.Project != q.project {
		return false
	}
	if q.text != "" {
		needle := strings.ToLower(q.text)
		hay := strings.ToLower(strings.Join([]string{it.Title, it.Body, it.Project, it.Alias}, "\n"))
		if !strings.Contains(hay, needle) {
			return false
		}
	}
	return true
}

// trackerItemStatus is an item's derived status; a topic has none.
func trackerItemStatus(it wstracker.ItemView) string {
	if it.Status == nil {
		return ""
	}
	return *it.Status
}

// formatTrackerItemRow is one item on one line: alias, kind and status,
// priority, title, then due and project.
func formatTrackerItemRow(it wstracker.ItemView) string {
	tag := it.Kind
	if s := trackerItemStatus(it); s != "" {
		tag += " " + s
	}
	if it.Hidden {
		tag += " hidden"
	}
	s := fmt.Sprintf("%s [%s] P%d %s", it.Alias, tag, it.PriorityLevel, it.Title)
	var extra []string
	if it.Due != nil {
		extra = append(extra, "due "+*it.Due)
	}
	if it.Project != "" {
		extra = append(extra, "project "+it.Project)
	}
	if it.Rollup.Total > 0 {
		extra = append(extra, fmt.Sprintf("%d/%d subtasks done", it.Rollup.Counts["done"], it.Rollup.Total))
	}
	if len(extra) > 0 {
		s += " (" + strings.Join(extra, ", ") + ")"
	}
	return s
}

// formatTrackerAssignmentRow is an assignment with its item: the item's
// alias and title, then the assignment.
func formatTrackerAssignmentRow(v *wstrackerhost.StoreView, a wstracker.Assignment) string {
	it := v.Items[a.Item]
	s := fmt.Sprintf("%s P%d %s: %s", it.Alias, it.PriorityLevel, it.Title, formatTrackerAssignment(v, a))
	if it.Due != nil {
		s += ", due " + *it.Due
	}
	return s
}

// formatTrackerAssignment is one assignment: holder, state and status, who
// assigned it, the rule it comes from or asks for, and its notes.
func formatTrackerAssignment(v *wstrackerhost.StoreView, a wstracker.Assignment) string {
	state := a.State
	if a.State == "live" {
		state = a.Status
	}
	s := fmt.Sprintf("%s %s (by %s", a.To, state, a.By)
	if a.Rule != nil {
		if r, ok := v.Rules[*a.Rule]; ok {
			if a.RuleRequest {
				s += ", rule request " + trackerRuleLabel(r)
			} else {
				s += ", rule " + trackerRuleLabel(r)
			}
			s += " " + trackerAlias(r.ID)
		}
	}
	if a.Completion == "assigner" {
		s += ", assigner closes"
	}
	if a.Stale {
		s += ", stale"
	}
	s += ")"
	if a.HandoffFrom != nil {
		s += ", handed from " + *a.HandoffFrom
	}
	if a.Note != nil && *a.Note != "" {
		s += fmt.Sprintf(", note %q", *a.Note)
	}
	if a.HandoffNote != nil && *a.HandoffNote != "" {
		s += fmt.Sprintf(", handoff note %q", *a.HandoffNote)
	}
	if a.AnswerNote != nil && *a.AnswerNote != "" {
		s += fmt.Sprintf(", answer %q", *a.AnswerNote)
	}
	return s
}

// trackerRuleLabel is a rule as tk:item.assign's rules attribute writes
// it: root, + when it includes the root, then :N or :* (whole subtree).
func trackerRuleLabel(r wstracker.RuleView) string {
	s := r.Root
	if r.IncludeRoot {
		s += "+"
	}
	if r.Levels == nil {
		return s + ":*"
	}
	return s + ":" + strconv.FormatFloat(*r.Levels, 'f', -1, 64)
}

// trackerRefRow is a referenced or referencing item or comment on one
// line: an item's row, or a comment's alias and author with its item.
func trackerRefRow(v *wstrackerhost.StoreView, id string) string {
	if it, ok := v.Items[id]; ok {
		return formatTrackerItemRow(it)
	}
	if c, ok := v.Comments[id]; ok {
		on := trackerAlias(c.Item)
		if it, ok := v.Items[c.Item]; ok {
			on += " " + it.Title
		}
		return fmt.Sprintf("comment %s by %s on %s", trackerAlias(id), c.Author, on)
	}
	return trackerAlias(id)
}

// trackerPerson is a member id with its display name.
func trackerPerson(v *wstrackerhost.StoreView, id string) string {
	if m, ok := v.Members[id]; ok && m.Name != "" && m.Name != id {
		return id + " (" + m.Name + ")"
	}
	return id
}

// formatTrackerItem is one item in full: its fields, body, assignments,
// children, and comment thread.
func formatTrackerItem(v *wstrackerhost.StoreView, id string) string {
	it := v.Items[id]
	var sb strings.Builder
	writeTrackerHeader(&sb, v)
	fmt.Fprintf(&sb, "\n%s %s %q\n", it.Alias, it.Kind, it.Title)
	var fields []string
	if s := trackerItemStatus(it); s != "" {
		fields = append(fields, "status "+s)
	}
	if it.Hidden {
		fields = append(fields, "hidden")
	}
	fields = append(fields, fmt.Sprintf("priority %d (%s)", it.PriorityLevel, it.PriorityName))
	if it.Due != nil {
		fields = append(fields, "due "+*it.Due)
	}
	if it.Project != "" {
		fields = append(fields, "project "+it.Project)
	}
	sb.WriteString(strings.Join(fields, ", ") + "\n")
	fmt.Fprintf(&sb, "created by %s at %s", trackerPerson(v, it.Author), it.TS)
	if it.Edited != nil {
		fmt.Fprintf(&sb, ", edited at %s", it.Edited.TS)
	}
	sb.WriteString("\n")
	if it.Parent != nil {
		parent := v.Items[*it.Parent]
		fmt.Fprintf(&sb, "parent: %s %s\n", trackerAlias(*it.Parent), parent.Title)
	}
	if len(it.Mentions) > 0 {
		fmt.Fprintf(&sb, "mentions: %s\n", strings.Join(it.Mentions, " "))
	}
	for _, refs := range []struct {
		label string
		ids   []string
	}{{"refs", it.Refs}, {"referenced by", it.Backrefs}} {
		if len(refs.ids) > 0 {
			sb.WriteString(refs.label + ":\n")
			for _, rid := range refs.ids {
				sb.WriteString("- " + trackerRefRow(v, rid) + "\n")
			}
		}
	}
	if strings.TrimSpace(it.Body) != "" {
		sb.WriteString("body:\n" + strings.TrimRight(it.Body, "\n") + "\n")
	}
	if len(it.Assignments) > 0 {
		sb.WriteString("assignments:\n")
		for _, aid := range it.Assignments {
			if a, ok := v.Assignments[aid]; ok {
				sb.WriteString("- " + formatTrackerAssignment(v, a) + "\n")
			}
		}
	}
	if len(it.Children) > 0 {
		sb.WriteString("children:\n")
		for _, cid := range it.Children {
			if child, ok := v.Items[cid]; ok {
				sb.WriteString("- " + formatTrackerItemRow(child) + "\n")
			}
		}
	}
	// The thread holds every event on the item; the comments are its text.
	wroteComments := false
	for _, cid := range it.Thread {
		c, ok := v.Comments[cid]
		if !ok {
			continue
		}
		if !wroteComments {
			sb.WriteString("comments:\n")
			wroteComments = true
		}
		head := fmt.Sprintf("- %s %s at %s", trackerAlias(c.ID), c.Author, c.TS)
		if c.ReplyTo != nil {
			head += ", reply to " + trackerAlias(*c.ReplyTo)
		}
		if c.Hidden != nil {
			sb.WriteString(head + ": (hidden)\n")
			continue
		}
		if c.Edited != nil {
			head += ", edited"
		}
		if len(c.Refs) > 0 {
			aliases := make([]string, len(c.Refs))
			for i, r := range c.Refs {
				aliases[i] = trackerAlias(r)
			}
			head += ", refs " + strings.Join(aliases, " ")
		}
		body := strings.ReplaceAll(strings.TrimRight(c.Body, "\n"), "\n", "\n  ")
		sb.WriteString(head + ":\n  " + body + "\n")
	}
	return sb.String()
}
