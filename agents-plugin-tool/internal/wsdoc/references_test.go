package wsdoc

import "testing"

func TestReferencesTraceRejectsMissingOrAmbiguousInput(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/tickets/todo/260504-demo.md", "---\ntitle: Demo\n---\n# Demo\n")
	mustWrite(t, root, "ai-docs/spec/demo.md", "# Demo\n\n## Feature {#260504-spec-demo}\n")
	mustWrite(t, root, "ai-docs/mental-model/demo.md", "---\ndomain: demo\n---\n# Demo\n")

	if _, err := ReferencesTrace(root, ReferenceTraceOptions{}); err == nil {
		t.Fatal("ReferencesTrace accepted missing selectors")
	}
	if _, err := ReferencesTrace(root, ReferenceTraceOptions{TicketStem: "260504-demo", SpecStem: "260504-spec-demo"}); err == nil {
		t.Fatal("ReferencesTrace accepted both selectors")
	}
}

func TestReferencesTraceFromTicketToSpecAndMentalModel(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/tickets/todo/260504-ticket-demo.md", "---\ntitle: Demo\nspec:\n  - 260504-spec-demo\n---\n# Demo\n")
	mustWrite(t, root, "ai-docs/spec/demo.md", "---\ntitle: Demo Spec\nfeatures:\n  - planned [260504-ticket-demo/p1]\n---\n# Demo\n\n## Feature {#260504-spec-demo}\n")
	mustWrite(t, root, "ai-docs/mental-model/demo.md", "---\ndomain: demo\nsources:\n  - ai-docs/spec/demo.md#260504-spec-demo\n---\n# Demo\n")

	got, err := ReferencesTrace(root, ReferenceTraceOptions{TicketStem: "260504-ticket-demo"})
	if err != nil {
		t.Fatalf("ReferencesTrace returned error: %v", err)
	}
	if got.InputType != "ticket" || got.Input != "260504-ticket-demo" {
		t.Fatalf("trace input = %#v", got)
	}
	if len(got.Tickets) != 1 || got.Tickets[0].Stem != "260504-ticket-demo" {
		t.Fatalf("trace tickets = %#v", got.Tickets)
	}
	if len(got.Specs) != 1 || got.Specs[0].Path != "ai-docs/spec/demo.md" {
		t.Fatalf("trace specs = %#v", got.Specs)
	}
	if len(got.MentalModels) != 1 || got.MentalModels[0].Path != "ai-docs/mental-model/demo.md" {
		t.Fatalf("trace mental models = %#v", got.MentalModels)
	}
}

func TestReferencesTraceFromSpecToTicketAndMentalModel(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/tickets/todo/260504-ticket-demo.md", "---\ntitle: Demo\n---\n# Demo\n\nMentions 260504-spec-demo.\n")
	mustWrite(t, root, "ai-docs/spec/demo.md", "---\ntitle: Demo Spec\n---\n# Demo\n\n## Feature {#260504-spec-demo}\n")
	mustWrite(t, root, "ai-docs/mental-model/demo.md", "---\ndomain: demo\nsources:\n  - ai-docs/spec/demo.md#260504-spec-demo\n---\n# Demo\n")

	got, err := ReferencesTrace(root, ReferenceTraceOptions{SpecStem: "260504-spec-demo"})
	if err != nil {
		t.Fatalf("ReferencesTrace returned error: %v", err)
	}
	if got.InputType != "spec" || got.Input != "260504-spec-demo" {
		t.Fatalf("trace input = %#v", got)
	}
	if len(got.Specs) != 1 || got.Specs[0].Path != "ai-docs/spec/demo.md" {
		t.Fatalf("trace specs = %#v", got.Specs)
	}
	if len(got.Tickets) != 1 || got.Tickets[0].Stem != "260504-ticket-demo" {
		t.Fatalf("trace tickets = %#v", got.Tickets)
	}
	if len(got.MentalModels) != 1 || got.MentalModels[0].Path != "ai-docs/mental-model/demo.md" {
		t.Fatalf("trace mental models = %#v", got.MentalModels)
	}
}
