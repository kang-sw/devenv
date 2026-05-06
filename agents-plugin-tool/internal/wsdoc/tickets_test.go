package wsdoc

import "testing"

func TestTicketsListDefaultsToActiveStatuses(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/tickets/idea/260504-idea-demo.md", "---\ntitle: Idea demo\n---\n# Idea\n")
	mustWrite(t, root, "ai-docs/tickets/ready/260504-ready-demo.md", "---\ntitle: Todo demo\nparent: 260504-epic-demo\nrelated:\n  260504-idea-demo: source\nspec:\n  - 260504-spec-demo\nspec-remove: 260504-old-spec\nplans:\n  - ai-docs/.plans/demo.md\nskeletons:\n  - agents-plugin-tool/internal/demo.go\n---\n# Todo\n\n## Phases\n\n### Phase 1: First\n\n### Result (abc123) - 2026-05-04\n\nDone.\n\n### Phase 2: Second\n")
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
	if joined(ready.Specs) != "260504-spec-demo" || joined(ready.SpecRemoves) != "260504-old-spec" {
		t.Fatalf("todo specs = %#v %#v", ready.Specs, ready.SpecRemoves)
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
