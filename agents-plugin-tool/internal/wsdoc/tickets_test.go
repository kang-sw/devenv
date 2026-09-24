package wsdoc

import "testing"

func TestTicketsListDefaultsToActiveStatuses(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/tickets/idea/260504-idea-demo.md", "---\ntitle: Idea demo\n---\n# Idea\n")
	mustWrite(t, root, "ai-docs/tickets/ready/260504-ready-demo.md", "---\ntitle: Todo demo\nparent: 260504-epic-demo\nrelated:\n  260504-idea-demo: source\nplans:\n  - ai-docs/.plans/demo.md\nskeletons:\n  - agents-plugin-tool/internal/demo.go\n---\n# Todo\n\n## Phases\n\n### Phase 1: First\n\n### Result (abc123) - 2026-05-04\n\nDone.\n\n### Phase 2: Second\n")
	mustWrite(t, root, "ai-docs/tickets/todo/260504-todo-demo.md", "---\ntitle: Todo backlog\n---\n# Todo backlog\n")
	mustWrite(t, root, "ai-docs/tickets/.done/260504-done-demo.md", "---\ntitle: Done demo\ncompleted: 2026-05-04\n---\n# Done\n")
	mustWrite(t, root, "ai-docs/tickets/.dropped/260504-dropped-demo.md", "---\ntitle: Dropped demo\n---\n# Dropped\n")

	got, err := TicketsList(root, TicketListOptions{})
	if err != nil {
		t.Fatalf("TicketsList returned error: %v", err)
	}
	if stems(got) != "260504-ready-demo,260504-todo-demo,260504-idea-demo" {
		t.Fatalf("default stems = %s", stems(got))
	}

	ready := findTicket(t, got, "260504-ready-demo")
	if ready.Path != "ai-docs/tickets/ready/260504-ready-demo.md" || ready.Status != "ready" || ready.Title != "Todo demo" {
		t.Fatalf("ready metadata = %#v", ready)
	}
	if ready.Parent != "260504-epic-demo" || ready.Related["260504-idea-demo"] != "source" {
		t.Fatalf("ready relationships = %#v", ready)
	}
	if joined(ready.Plans) != "ai-docs/.plans/demo.md" || joined(ready.Skeletons) != "agents-plugin-tool/internal/demo.go" {
		t.Fatalf("todo artifacts = %#v %#v", ready.Plans, ready.Skeletons)
	}
	if !ready.ResultPresent || joined(ready.UnresolvedPhases) != "Phase 2: Second" {
		t.Fatalf("ready phases = %#v", ready)
	}
}

func TestTicketsListIncludeDoneAndDroppedAreSeparate(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/tickets/todo/260504-active-demo.md", "---\ntitle: Active\n---\n# Active\n")
	mustWrite(t, root, "ai-docs/tickets/.done/260504-done-demo.md", "---\ntitle: Done\n---\n# Done\n")
	mustWrite(t, root, "ai-docs/tickets/.dropped/260504-dropped-demo.md", "---\ntitle: Dropped\n---\n# Dropped\n")

	doneOnly, err := TicketsList(root, TicketListOptions{Statuses: []string{".done", ".dropped"}, IncludeDone: true})
	if err != nil {
		t.Fatal(err)
	}
	if stems(doneOnly) != "260504-done-demo" {
		t.Fatalf("doneOnly stems = %s", stems(doneOnly))
	}

	droppedOnly, err := TicketsList(root, TicketListOptions{Statuses: []string{"done", "dropped"}, IncludeDropped: true})
	if err != nil {
		t.Fatal(err)
	}
	if stems(droppedOnly) != "260504-dropped-demo" {
		t.Fatalf("droppedOnly stems = %s", stems(droppedOnly))
	}
}

func TestTicketsListAndFindAcceptReadyStatusFilter(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/tickets/ready/260504-ready-demo.md", "---\ntitle: Ready\n---\n# Shared query\n")
	mustWrite(t, root, "ai-docs/tickets/todo/260504-todo-demo.md", "---\ntitle: Todo\n---\n# Shared query\n")
	mustWrite(t, root, "ai-docs/tickets/idea/260504-idea-demo.md", "---\ntitle: Idea\n---\n# Shared query\n")

	listed, err := TicketsList(root, TicketListOptions{Statuses: []string{"ready"}})
	if err != nil {
		t.Fatal(err)
	}
	if stems(listed) != "260504-ready-demo" || listed[0].Status != "ready" {
		t.Fatalf("ready list filter = %#v", listed)
	}

	found, err := TicketsFind(root, TicketFindOptions{Statuses: []string{"ready"}, Query: "Shared query"})
	if err != nil {
		t.Fatal(err)
	}
	if stems(found) != "260504-ready-demo" || found[0].Status != "ready" {
		t.Fatalf("ready find filter = %#v", found)
	}
}

func TestTicketsFindPaginationFollowsFilteredStatusAndStemOrder(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/tickets/ready/260100-ready-nope.md", "---\ntitle: No match\n---\n# Other\n")
	mustWrite(t, root, "ai-docs/tickets/ready/260101-ready-a.md", "---\ntitle: A\n---\n# Shared\n")
	mustWrite(t, root, "ai-docs/tickets/ready/260103-ready-c.md", "---\ntitle: C\n---\n# Shared\n")
	mustWrite(t, root, "ai-docs/tickets/todo/260102-todo-b.md", "---\ntitle: B\n---\n# Shared\n")
	mustWrite(t, root, "ai-docs/tickets/todo/260104-todo-d.md", "---\ntitle: D\n---\n# Shared\n")
	mustWrite(t, root, "ai-docs/tickets/idea/260105-idea-e.md", "---\ntitle: E\n---\n# Shared\n")

	page, err := TicketsFind(root, TicketFindOptions{Query: "Shared", Offset: 1, Limit: 3})
	if err != nil {
		t.Fatalf("TicketsFind returned error: %v", err)
	}
	if got, want := stems(page), "260103-ready-c,260102-todo-b,260104-todo-d"; got != want {
		t.Fatalf("paginated stems = %s, want %s", got, want)
	}

	terminal, err := TicketsFind(root, TicketFindOptions{Query: "Shared", Offset: 4, Limit: 3})
	if err != nil {
		t.Fatalf("TicketsFind terminal page returned error: %v", err)
	}
	if got, want := stems(terminal), "260105-idea-e"; got != want {
		t.Fatalf("terminal stems = %s, want %s", got, want)
	}
}

func TestTicketsFindExcludeAppliesBeforePagination(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/tickets/ready/260101-ready-a.md", "---\ntitle: A\n---\n# Shared\n")
	mustWrite(t, root, "ai-docs/tickets/ready/260103-ready-c.md", "---\ntitle: C\n---\n# Shared\n")
	mustWrite(t, root, "ai-docs/tickets/todo/260102-todo-b.md", "---\ntitle: B\n---\n# Shared\n")
	mustWrite(t, root, "ai-docs/tickets/todo/260104-todo-d.md", "---\ntitle: D\n---\n# Shared\n")
	mustWrite(t, root, "ai-docs/tickets/idea/260105-idea-e.md", "---\ntitle: E\n---\n# Shared\n")

	excluded := map[string]bool{"260101-ready-a": true, "260102-todo-b": true}
	exclude := func(ticket TicketInfo) bool { return excluded[ticket.Stem] }

	// Unfiltered order is a,c,b,d,e; after the exclusion it is c,d,e. A page
	// sliced before the exclusion would be c,b thinned to c.
	page, err := TicketsFind(root, TicketFindOptions{Query: "Shared", Offset: 1, Limit: 2, Exclude: exclude})
	if err != nil {
		t.Fatalf("TicketsFind returned error: %v", err)
	}
	if got, want := stems(page), "260104-todo-d,260105-idea-e"; got != want {
		t.Fatalf("excluded page stems = %s, want %s", got, want)
	}

	all, err := TicketsFind(root, TicketFindOptions{Query: "Shared", Exclude: exclude})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := stems(all), "260103-ready-c,260104-todo-d,260105-idea-e"; got != want {
		t.Fatalf("excluded stems = %s, want %s", got, want)
	}
}

func TestTicketsFindByMentionAndQuery(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/tickets/todo/260504-parent-demo.md", "---\ntitle: Parent\n---\n# Parent\n")
	mustWrite(t, root, "ai-docs/tickets/todo/260504-child-demo.md", "---\ntitle: Child\nparent: 260504-parent-demo\n---\n# Child\n\nMentions 260504-parent-demo and deterministic discovery.\n")

	got, err := TicketsFind(root, TicketFindOptions{MentionsTicketStem: "260504-parent-demo", Query: "deterministic"})
	if err != nil {
		t.Fatalf("TicketsFind returned error: %v", err)
	}
	if len(got) != 1 || got[0].Stem != "260504-child-demo" || !got[0].MentionsTicketStem {
		t.Fatalf("find result = %#v", got)
	}
	if joined(got[0].MatchingSnippets) != "Mentions 260504-parent-demo and deterministic discovery." {
		t.Fatalf("snippets = %#v", got[0].MatchingSnippets)
	}
}

func TestTicketsStatusRequiresTicketStem(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/tickets/todo/260504-demo.md", "---\ntitle: Demo\n---\n# Demo\n")

	if _, err := TicketsStatus(root, TicketStatusOptions{TicketStem: "260504-demo"}); err != nil {
		t.Fatalf("TicketsStatus returned error: %v", err)
	}
	if _, err := TicketsStatus(root, TicketStatusOptions{TicketStem: "not-a-spec-stem"}); err == nil {
		t.Fatal("TicketsStatus accepted a non-ticket stem")
	}
}

// TestTicketBlockedHeadingsAreVerbatimAndUninterpreted pins the advisory
// body-marker projection: every heading line beginning with `## Blocked` is
// collected verbatim and in document order, a nonstandard suffix such as
// `— RESOLVED ...` is returned whole rather than classified, a ticket with no
// such heading yields no field, and the marker never promotes to the typed
// dispatch gate.
func TestTicketBlockedHeadingsAreVerbatimAndUninterpreted(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/tickets/ready/260101-feat-current.md",
		"---\ntitle: Current blocker\n---\n# Current\n\n## Blocked (2026-09-16)\n\nOwner-only smoke remains.\n")
	mustWrite(t, root, "ai-docs/tickets/ready/260102-feat-resolved.md",
		"---\ntitle: Resolved blocker\n---\n# Resolved\n\n## Blocked (2026-07-27) — RESOLVED 2026-08-11\n\nCleared.\n")
	mustWrite(t, root, "ai-docs/tickets/ready/260103-feat-multi.md",
		"---\ntitle: Two markers\n---\n# Multi\n\n## Blocked (2026-01-01)\n\nfirst\n\n## Blocked (2026-02-02)\n\nsecond\n")
	mustWrite(t, root, "ai-docs/tickets/ready/260104-feat-clean.md",
		"---\ntitle: No marker\n---\n# Clean\n\n## Background\n\nNothing blocked here.\n")

	got, err := TicketsList(root, TicketListOptions{Statuses: []string{"ready"}})
	if err != nil {
		t.Fatal(err)
	}

	current := findTicket(t, got, "260101-feat-current")
	if joined(current.BlockedHeadings) != "## Blocked (2026-09-16)" {
		t.Fatalf("current blocked headings = %#v", current.BlockedHeadings)
	}
	// Advisory only: the marker never becomes the typed dispatch gate.
	if current.DispatchBlocked != nil {
		t.Fatalf("body marker must not populate DispatchBlocked: %#v", current.DispatchBlocked)
	}

	resolved := findTicket(t, got, "260102-feat-resolved")
	if joined(resolved.BlockedHeadings) != "## Blocked (2026-07-27) — RESOLVED 2026-08-11" {
		t.Fatalf("resolved heading not returned verbatim: %#v", resolved.BlockedHeadings)
	}

	multi := findTicket(t, got, "260103-feat-multi")
	if joined(multi.BlockedHeadings) != "## Blocked (2026-01-01),## Blocked (2026-02-02)" {
		t.Fatalf("multi headings/order wrong: %#v", multi.BlockedHeadings)
	}

	clean := findTicket(t, got, "260104-feat-clean")
	if len(clean.BlockedHeadings) != 0 {
		t.Fatalf("ticket without marker must carry no headings: %#v", clean.BlockedHeadings)
	}
}

// TestTicketBlockedHeadingsSurfaceInsideFence pins a design-accepted limitation
// of blockedHeadings: it matches by trimmed line prefix alone, so a
// `## Blocked (...)` line inside a fenced code block (or quoted example prose)
// is surfaced as an advisory marker too, exactly like a live one. Both the
// correctness and test reviewers judged this acceptable — the marker is
// advisory-only and currency is the caller's judgment — so this test locks the
// current behavior in place rather than asserting the fence should suppress it.
// If that verdict changes, it takes a ticket, not a silent test edit.
func TestTicketBlockedHeadingsSurfaceInsideFence(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/tickets/ready/260105-feat-fenced.md",
		"---\ntitle: Fenced example\n---\n# Fenced\n\nExample template:\n\n```\n## Blocked (2026-03-03)\n```\n\nNot actually blocked.\n")

	got, err := TicketsList(root, TicketListOptions{Statuses: []string{"ready"}})
	if err != nil {
		t.Fatal(err)
	}

	fenced := findTicket(t, got, "260105-feat-fenced")
	if joined(fenced.BlockedHeadings) != "## Blocked (2026-03-03)" {
		t.Fatalf("fenced heading must still surface (accepted limitation): %#v", fenced.BlockedHeadings)
	}
}

func stems(tickets []TicketInfo) string {
	values := make([]string, 0, len(tickets))
	for _, ticket := range tickets {
		values = append(values, ticket.Stem)
	}
	return joined(values)
}

func findTicket(t *testing.T, tickets []TicketInfo, stem string) TicketInfo {
	t.Helper()
	for _, ticket := range tickets {
		if ticket.Stem == stem {
			return ticket
		}
	}
	t.Fatalf("ticket not found: %s", stem)
	return TicketInfo{}
}

func joined(values []string) string {
	out := ""
	for i, value := range values {
		if i > 0 {
			out += ","
		}
		out += value
	}
	return out
}
