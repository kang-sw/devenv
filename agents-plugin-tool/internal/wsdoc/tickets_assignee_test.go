package wsdoc

import "testing"

func TestAssigneeGateFor(t *testing.T) {
	cases := []struct {
		name         string
		assignee     []string
		email        string
		wantAny      bool
		wantAssigned bool
		wantWarning  bool
	}{
		{"assign-any empty", nil, "me@example.com", true, true, false},
		{"self exact", []string{"me@example.com"}, "me@example.com", false, true, false},
		{"self case-insensitive", []string{"Me@Example.COM"}, "me@example.com", false, true, false},
		{"other mismatch", []string{"bob@example.com"}, "me@example.com", false, false, true},
		{"multi any-of match", []string{"bob@example.com", "me@example.com"}, "me@example.com", false, true, false},
		{"multi any-of miss", []string{"bob@example.com", "ann@example.com"}, "me@example.com", false, false, true},
		{"unknown identity assigned ticket", []string{"bob@example.com"}, "", false, true, false},
		{"unknown identity assign-any", nil, "", true, true, false},
		{"email whitespace trimmed", []string{" me@example.com "}, "me@example.com", false, true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := AssigneeGateFor(tc.assignee, tc.email)
			if g.AssignAny != tc.wantAny {
				t.Errorf("AssignAny = %v, want %v", g.AssignAny, tc.wantAny)
			}
			if g.AssignedToCurrent != tc.wantAssigned {
				t.Errorf("AssignedToCurrent = %v, want %v", g.AssignedToCurrent, tc.wantAssigned)
			}
			hasWarning := g.Warning != ""
			if hasWarning != tc.wantWarning {
				t.Errorf("Warning present = %v (%q), want %v", hasWarning, g.Warning, tc.wantWarning)
			}
			if hasWarning && g.Warning != AssigneeGateWarning {
				t.Errorf("Warning = %q, want %q", g.Warning, AssigneeGateWarning)
			}
		})
	}
}

func TestReadTicketParsesAssignee(t *testing.T) {
	listForm := "---\ntitle: X\nassignee:\n  - a@example.com\n  - b@example.com\n---\n# X\n"
	info := readTicketFromBytes("ai-docs/tickets/ready/260101-feat-x.md", "ready", listForm)
	if len(info.Assignee) != 2 || info.Assignee[0] != "a@example.com" || info.Assignee[1] != "b@example.com" {
		t.Fatalf("list-form assignee parse = %v, want [a@example.com b@example.com]", info.Assignee)
	}

	scalarForm := "---\ntitle: X\nassignee: solo@example.com\n---\n# X\n"
	info = readTicketFromBytes("ai-docs/tickets/ready/260101-feat-x.md", "ready", scalarForm)
	if len(info.Assignee) != 1 || info.Assignee[0] != "solo@example.com" {
		t.Fatalf("scalar-form assignee parse = %v, want [solo@example.com]", info.Assignee)
	}

	absent := "---\ntitle: X\n---\n# X\n"
	info = readTicketFromBytes("ai-docs/tickets/ready/260101-feat-x.md", "ready", absent)
	if len(info.Assignee) != 0 {
		t.Fatalf("absent assignee parse = %v, want empty (assign-any)", info.Assignee)
	}
}
