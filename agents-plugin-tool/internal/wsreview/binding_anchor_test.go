package wsreview

import (
	"testing"
)

func TestReadAgentsBindingAnchorUndeclaredWhenFileAbsent(t *testing.T) {
	root := t.TempDir()
	got := ReadAgentsBindingAnchor(root)
	if got.Declared || got.Anchor != "" || got.Topics != "" {
		t.Fatalf("ReadAgentsBindingAnchor = %+v, want undeclared zero value", got)
	}
	if got.PrepClause() != "" {
		t.Fatalf("PrepClause = %q, want empty for undeclared", got.PrepClause())
	}
}

func TestReadAgentsBindingAnchorUndeclaredWhenSectionAbsent(t *testing.T) {
	root := t.TempDir()
	mustWriteAgentsMD(t, root, "# AGENTS.md\n\n## Workflow\n\n### Review Policy\nreview-track: develop\n")
	got := ReadAgentsBindingAnchor(root)
	if got.Declared {
		t.Fatalf("ReadAgentsBindingAnchor = %+v, want undeclared when section absent", got)
	}
}

func TestReadAgentsBindingAnchorUndeclaredWhenAnchorKeyMissing(t *testing.T) {
	root := t.TempDir()
	mustWriteAgentsMD(t, root, "## Workflow\n\n### Binding Anchor\ntopics: plugin architecture, adapter boundaries\n")
	got := ReadAgentsBindingAnchor(root)
	if got.Declared {
		t.Fatalf("ReadAgentsBindingAnchor = %+v, want undeclared when anchor missing", got)
	}
	if got.PrepClause() != "" {
		t.Fatalf("PrepClause = %q, want empty when only topics declared", got.PrepClause())
	}
}

func TestReadAgentsBindingAnchorUndeclaredWhenTopicsKeyMissing(t *testing.T) {
	root := t.TempDir()
	mustWriteAgentsMD(t, root, "## Workflow\n\n### Binding Anchor\nanchor: ai-docs/tickets/idea/260605-demo.md\n")
	got := ReadAgentsBindingAnchor(root)
	if got.Declared {
		t.Fatalf("ReadAgentsBindingAnchor = %+v, want undeclared when topics missing", got)
	}
	if got.PrepClause() != "" {
		t.Fatalf("PrepClause = %q, want empty when only anchor declared", got.PrepClause())
	}
}

func TestReadAgentsBindingAnchorDeclaredWhenBothKeysPresent(t *testing.T) {
	root := t.TempDir()
	mustWriteAgentsMD(t, root, "# AGENTS.md\n\n## Workflow\n\n### Binding Anchor\nanchor: ai-docs/tickets/idea/260605-demo.md\ntopics: plugin architecture, host-neutral migration, spawn-removal, adapter boundaries\n\n### Commit Rules\nother content\n")
	got := ReadAgentsBindingAnchor(root)
	if !got.Declared {
		t.Fatalf("ReadAgentsBindingAnchor = %+v, want declared when both keys present", got)
	}
	if got.Anchor != "ai-docs/tickets/idea/260605-demo.md" {
		t.Fatalf("Anchor = %q", got.Anchor)
	}
	if got.Topics != "plugin architecture, host-neutral migration, spawn-removal, adapter boundaries" {
		t.Fatalf("Topics = %q", got.Topics)
	}
	want := "read ai-docs/tickets/idea/260605-demo.md when target touches plugin architecture, host-neutral migration, spawn-removal, adapter boundaries, "
	if got.PrepClause() != want {
		t.Fatalf("PrepClause = %q, want %q", got.PrepClause(), want)
	}
}
