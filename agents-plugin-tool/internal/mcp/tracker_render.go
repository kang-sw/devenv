package mcp

import (
	"fmt"
	"sort"
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
			sb.WriteString(strings.Repeat("  ", trackerIndent(q.scope, it)) + "- " + formatTrackerItemRow(v, it) + "\n")
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
		} else {
			sb.WriteString("\nteam: " + trackerTeamLegend + "\n")
		}
		outside := false
		for _, t := range v.My.Team {
			if t.Outside && !outside {
				outside = true
				sb.WriteString("\noutside my subtree (shown to an admin; depth from the org-tree root):\n")
			}
			fmt.Fprintf(&sb, "\n%s\n", formatTrackerTeamHead(v, t))
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
	case "related":
		items("items I am CC'd on, the latest CC first", v.My.Related.CC, false)
		items("items that mention me, the newest mention first", v.My.Related.Mentioned, false)
	case "all":
		items("every item, in tree order", v.ItemOrder, false)
	default:
		assignments("my assignments", v.My.Assignments)
		items("created by me, assigned to nobody", v.My.Created, false)
		requests := func(title string, ids []string) {
			var lines []string
			for _, id := range ids {
				if r, ok := v.Requests[id]; ok && keep(r.Item) {
					lines = append(lines, "- "+formatTrackerRequestRow(v, r)+"\n")
				}
			}
			if len(lines) > 0 {
				section(title)
				sb.WriteString(strings.Join(lines, ""))
				rows += len(lines)
			}
		}
		requests("take requests for me to answer (tk:item.approve or tk:item.deny)", v.My.Requests.Incoming)
		requests("my take requests, waiting for an answer", v.My.Requests.Outgoing)
	}
	if rows == 0 && q.filtered() {
		sb.WriteString("\nno rows pass the filters\n")
	}
	return sb.String()
}

// trackerTeamLegend opens a non-empty team scope: what each count of a
// person's head line counts.
const trackerTeamLegend = "load counts a person's live assignments at todo, doing, or blocked, broken down by status; " +
	"stale and overdue (the due date passed) are part of the load; other statuses (in-review, idea, done, dropped) count on their own; " +
	"fallback-held counts tasks they hold only as the creator, because nobody else holds them, and is not load"

// trackerLoadStatuses are the statuses a team row's load counts, in the
// order of its breakdown (the view's team roll-up counts the same ones).
var trackerLoadStatuses = []string{"todo", "doing", "blocked"}

// formatTrackerTeamHead is one person's head line of the team scope: the
// load with its per-status breakdown, stale and overdue, every other status
// but idea and done (sorted; in-review and dropped among them, each only when
// counted), then idea, fallback-held, done, and the last update.
func formatTrackerTeamHead(v *wstrackerhost.StoreView, t wstracker.Team) string {
	var load, other []string
	seen := map[string]bool{"idea": true, "done": true}
	for _, st := range trackerLoadStatuses {
		if n := t.Counts[st]; n > 0 {
			load = append(load, fmt.Sprintf("%s %d", st, n))
		}
		seen[st] = true
	}
	for st := range t.Counts {
		if !seen[st] {
			other = append(other, st)
		}
	}
	sort.Strings(other)
	head := fmt.Sprintf("%s (depth %d): load %d", trackerPerson(v, t.ID), t.Depth, t.Load)
	if len(load) > 0 {
		head += " (" + strings.Join(load, ", ") + ")"
	}
	head += fmt.Sprintf(", %d stale, %d overdue", t.Stale, t.Overdue)
	for _, st := range other {
		head += fmt.Sprintf(", %d %s", t.Counts[st], st)
	}
	head += fmt.Sprintf(", %d idea, %d fallback-held, %d done", t.Counts["idea"], t.Fallback, t.Counts["done"])
	if t.Last != nil {
		head += ", last update " + *t.Last
	}
	return head
}

// trackerIndent is an item row's tree depth in scope all, else 0.
func trackerIndent(scope string, it wstracker.ItemView) int {
	if scope == "all" {
		return it.Depth
	}
	return 0
}

// trackerPriorityHint ends the priority line: where the levels are changed.
const trackerPriorityHint = "an admin changes the levels in the store settings of the dashboard's Team tab, or with tk:roles.set_priority_levels"

// trackerLabelsHint ends the labels line: how an item takes a label, and
// where the list is changed.
const trackerLabelsHint = "an item's labels attribute takes these IDs; an admin changes the list in the same store settings, or with tk:roles.add_label, tk:roles.edit_label, and tk:roles.remove_label"

// writeTrackerHeader writes the store, viewer, sync, priority, labels (when
// the store has any), and today lines every store view starts with.
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
	// The hint answers "where are the levels changed", which the levels
	// alone left agents unable to tell the user.
	fmt.Fprintf(sb, "priority: %s; %s\n", strings.Join(levels, ", "), trackerPriorityHint)
	if len(v.Labels) > 0 {
		labels := make([]string, len(v.Labels))
		for i, l := range v.Labels {
			labels[i] = fmt.Sprintf("%s %s %s", trackerAlias(l.ID), l.Name, l.Color)
		}
		fmt.Fprintf(sb, "labels: %s; %s\n", strings.Join(labels, ", "), trackerLabelsHint)
	}
	t := v.My.Today
	fmt.Fprintf(sb, "today: my work %d, to answer %d, to confirm %d\n", len(t.Work), len(t.Pending), len(t.Confirms))
	for _, p := range v.RolesProblems {
		fmt.Fprintf(sb, "roles problem: %s\n", p)
	}
}

// trackerItemMatches reports whether an item passes q's filters, the q
// search included. A hidden item passes only status hidden, in the status
// field or as a status:hidden search token.
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
		if it.Hidden && !q.search.WantsHidden() {
			return false
		}
	case "open":
		if it.Hidden || status == "done" || status == "dropped" {
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
	return q.search.Match(v.View, id)
}

// trackerItemStatus is an item's derived status; a topic has none.
func trackerItemStatus(it wstracker.ItemView) string {
	if it.Status == nil {
		return ""
	}
	return *it.Status
}

// trackerLabelNames is the names of an item's labels, in its order.
func trackerLabelNames(v *wstrackerhost.StoreView, it wstracker.ItemView) []string {
	var out []string
	for _, id := range it.Labels {
		for _, l := range v.Labels {
			if l.ID == id {
				out = append(out, l.Name)
			}
		}
	}
	return out
}

// formatTrackerItemRow is one item on one line: alias, kind and status,
// priority, title, then due, project, and labels.
func formatTrackerItemRow(v *wstrackerhost.StoreView, it wstracker.ItemView) string {
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
	if names := trackerLabelNames(v, it); len(names) > 0 {
		extra = append(extra, "labels "+strings.Join(names, ", "))
	}
	if it.Rollup.Total > 0 {
		extra = append(extra, fmt.Sprintf("%d/%d subtasks done", it.Rollup.Counts["done"], it.Rollup.Total))
	}
	if it.Open != nil {
		open := "open to " + it.Open.Scope
		if it.Claimable {
			open += ", claimable by me"
		}
		extra = append(extra, open)
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

// formatTrackerRequest is one take request: its ID (what tk:item.approve,
// tk:item.deny, and tk:item.unrequest take as request), requester, the
// holder asked, state, and notes.
func formatTrackerRequest(r wstracker.RequestView) string {
	s := fmt.Sprintf("%s %s asks for %s's assignment, %s", trackerAlias(r.ID), r.By, r.Assignee, r.State)
	if r.Note != nil && *r.Note != "" {
		s += fmt.Sprintf(", note %q", *r.Note)
	}
	if r.AnswerNote != nil && *r.AnswerNote != "" {
		s += fmt.Sprintf(", answer %q", *r.AnswerNote)
	}
	return s
}

// formatTrackerRequestRow is a take request with its item.
func formatTrackerRequestRow(v *wstrackerhost.StoreView, r wstracker.RequestView) string {
	it := v.Items[r.Item]
	return fmt.Sprintf("%s P%d %s: %s", it.Alias, it.PriorityLevel, it.Title, formatTrackerRequest(r))
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
		return formatTrackerItemRow(v, it)
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
	if names := trackerLabelNames(v, it); len(names) > 0 {
		fields = append(fields, "labels "+strings.Join(names, ", "))
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
	if len(it.CC) > 0 {
		var cc []string
		for _, m := range it.CC {
			cc = append(cc, trackerPerson(v, m))
		}
		fmt.Fprintf(&sb, "cc: %s\n", strings.Join(cc, ", "))
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
	// Printed even for an empty body: an edit passes it back as base.
	if len(it.BodyRev) > 0 {
		rev := make([]string, len(it.BodyRev))
		for i, rid := range it.BodyRev {
			rev[i] = trackerAlias(rid)
		}
		fmt.Fprintf(&sb, "body revision: %s\n", strings.Join(rev, " "))
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
	if o := it.Open; o != nil {
		fmt.Fprintf(&sb, "open for claiming to %s by %s (%s)", o.Scope, o.By, trackerAlias(o.ID))
		if it.Claimable {
			sb.WriteString(", claimable by me")
		}
		sb.WriteString("\n")
	}
	if len(it.Requests) > 0 {
		sb.WriteString("take requests:\n")
		for _, rid := range it.Requests {
			if r, ok := v.Requests[rid]; ok {
				sb.WriteString("- " + formatTrackerRequest(r) + "\n")
			}
		}
	}
	if len(it.Children) > 0 {
		sb.WriteString("children:\n")
		for _, cid := range it.Children {
			if child, ok := v.Items[cid]; ok {
				sb.WriteString("- " + formatTrackerItemRow(v, child) + "\n")
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
