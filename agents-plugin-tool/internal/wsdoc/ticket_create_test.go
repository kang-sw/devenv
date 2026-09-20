package wsdoc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readCreatedTicket(t *testing.T, root string, res TicketCreateResult) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(res.Path)))
	if err != nil {
		t.Fatalf("read created ticket: %v", err)
	}
	return string(raw)
}

func TestTicketCreateIdea(t *testing.T) {
	root := t.TempDir()
	res, err := TicketCreate(root, TicketCreateOptions{Stem: "feat-foo", InitialState: "idea", Today: "260101"})
	if err != nil {
		t.Fatalf("TicketCreate idea: %v", err)
	}
	body := readCreatedTicket(t, root, res)
	if !strings.Contains(body, `title: ""`) {
		t.Fatalf("idea stub missing title: %q", body)
	}
	if strings.Contains(body, "sage-review") {
		t.Fatalf("idea stub must not contain sage-review: %q", body)
	}
}

func TestTicketCreateStampsAssignee(t *testing.T) {
	root := t.TempDir()
	res, err := TicketCreate(root, TicketCreateOptions{
		Stem:         "feat-foo",
		InitialState: "idea",
		Today:        "260101",
		Assignee:     []string{"a@example.com", "", "b@example.com"},
	})
	if err != nil {
		t.Fatalf("TicketCreate: %v", err)
	}
	body := readCreatedTicket(t, root, res)
	if !strings.Contains(body, "assignee:\n  - a@example.com\n  - b@example.com\n") {
		t.Fatalf("assignee block not stamped as YAML sequence (blank dropped): %q", body)
	}
	// The stamped block must round-trip back into the parsed []string.
	info := readTicketFromBytes(res.Path, "idea", body)
	if len(info.Assignee) != 2 || info.Assignee[0] != "a@example.com" || info.Assignee[1] != "b@example.com" {
		t.Fatalf("stamped assignee did not round-trip: %v", info.Assignee)
	}
}

func TestTicketCreateNoAssigneeWhenEmpty(t *testing.T) {
	root := t.TempDir()
	res, err := TicketCreate(root, TicketCreateOptions{Stem: "feat-foo", InitialState: "idea", Today: "260101", Assignee: []string{"", "  "}})
	if err != nil {
		t.Fatalf("TicketCreate: %v", err)
	}
	body := readCreatedTicket(t, root, res)
	if strings.Contains(body, "assignee:") {
		t.Fatalf("empty/blank assignee must leave the ticket unassigned (assign-any): %q", body)
	}
}

func TestTicketsFindAssigneeOmitFilter(t *testing.T) {
	root := t.TempDir()
	write := func(stem, assignee string) {
		fm := "---\ntitle: X\n"
		if assignee != "" {
			fm += "assignee:\n  - " + assignee + "\n"
		}
		fm += "---\n# " + stem + "\n"
		p := filepath.Join(root, "ai-docs", "tickets", "ready", stem+".md")
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(fm), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("260101-feat-mine", "me@example.com")
	write("260102-feat-other", "bob@example.com")
	write("260103-feat-anyone", "")

	got, err := TicketsFind(root, TicketFindOptions{Statuses: []string{"ready"}, AssignedToEmail: "ME@example.com"})
	if err != nil {
		t.Fatalf("TicketsFind: %v", err)
	}
	stems := map[string]bool{}
	for _, ti := range got {
		stems[ti.Stem] = true
	}
	if !stems["260101-feat-mine"] || !stems["260103-feat-anyone"] {
		t.Fatalf("filter dropped a self/assign-any ticket: %v", stems)
	}
	if stems["260102-feat-other"] {
		t.Fatalf("filter must omit an others'-assigned ticket: %v", stems)
	}

	// No filter (empty AssignedToEmail) returns the whole board.
	all, err := TicketsFind(root, TicketFindOptions{Statuses: []string{"ready"}})
	if err != nil {
		t.Fatalf("TicketsFind unfiltered: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("unfiltered query = %d tickets, want 3", len(all))
	}
}

func TestTicketCreateEpicTodoStampsResolvedSageReviewDesignPosture(t *testing.T) {
	for _, tc := range []struct {
		name       string
		config     string
		wantReview string
	}{
		{"empty", "", "skipped"},
		{"off", "off", "skipped"},
		{"ask", "ask", "recommended"},
		{"auto", "auto", "required"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			res, err := TicketCreate(root, TicketCreateOptions{Stem: "epic-foo", InitialState: "todo", SageReview: tc.config, Today: "260101"})
			if err != nil {
				t.Fatalf("TicketCreate todo: %v", err)
			}
			body := readCreatedTicket(t, root, res)
			if !strings.Contains(body, `title: ""`) {
				t.Fatalf("todo stub missing title: %q", body)
			}
			wantLine := "sage-review-design: " + tc.wantReview
			if !strings.Contains(body, wantLine) {
				t.Fatalf("todo stub missing %s: %q", wantLine, body)
			}
			if strings.Contains(body, "sage-review-completeness:") {
				t.Fatalf("todo stub must not stamp sage-review-completeness: %q", body)
			}
			if !strings.Contains(res.Tip, tc.wantReview) {
				t.Fatalf("Tip = %q, want resolved posture %q", res.Tip, tc.wantReview)
			}
		})
	}
}

// TestTicketCreateReadyStampsResolvedSageReviewDesignPostureWhenTerminal
// asserts the C4 fix: a ready-landing create stamps *both* required fields
// (design and completeness), mirroring prepareSageReviewForUpwardMove /
// TicketsMove, so create_empty(ready) and move(to: "ready") produce the same
// posture shape.
func TestTicketCreateReadyStampsResolvedSageReviewDesignPostureWhenTerminal(t *testing.T) {
	for _, tc := range []struct {
		name       string
		config     string
		wantReview string
	}{
		{"empty", "", "skipped"},
		{"off", "off", "skipped"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			res, err := TicketCreate(root, TicketCreateOptions{Stem: "feat-foo", InitialState: "ready", SageReview: tc.config, Today: "260101"})
			if err != nil {
				t.Fatalf("TicketCreate ready: %v", err)
			}
			body := readCreatedTicket(t, root, res)
			if !strings.Contains(body, `title: ""`) {
				t.Fatalf("ready stub missing title: %q", body)
			}
			for _, field := range []string{"sage-review-design", "sage-review-completeness"} {
				wantLine := field + ": " + tc.wantReview
				if !strings.Contains(body, wantLine) {
					t.Fatalf("ready stub missing %s: %q", wantLine, body)
				}
			}
			if !strings.Contains(res.Tip, tc.wantReview) {
				t.Fatalf("Tip = %q, want resolved posture %q", res.Tip, tc.wantReview)
			}
		})
	}
}

// TestTicketCreateReadyWarnsOnUnresolvedSageReviewDesignPosture asserts the
// de-blocked never-skippable design-review invariant at direct-to-ready
// creation: a ticket created directly at ready/ with no prior "from" state
// has no opportunity to have already run design review, so a freshly
// resolved non-terminal posture (recommended/required) now succeeds and
// carries the ready-sage-posture warning instead of blocking creation —
// ws/git.commit's ready-sage-posture guardrail is the sole hard gate.
func TestTicketCreateReadyWarnsOnUnresolvedSageReviewDesignPosture(t *testing.T) {
	for _, tc := range []struct {
		name     string
		config   string
		wantWarn string
	}{
		{"ask", "ask", "sage-review-design is unreviewed (posture recommended; review has not run yet)"},
		{"auto", "auto", "sage-review-design is unreviewed (posture required; review has not run yet)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			res, err := TicketCreate(root, TicketCreateOptions{Stem: "feat-foo", InitialState: "ready", SageReview: tc.config, Today: "260101"})
			if err != nil {
				t.Fatalf("TicketCreate ready: unexpected error for unresolved design posture: %v", err)
			}
			if !strings.Contains(res.Tip, tc.wantWarn) {
				t.Fatalf("Tip = %q, want it to contain %q", res.Tip, tc.wantWarn)
			}
			// TicketCreate can only ever produce the unreviewed variant (a
			// brand-new ticket has no prior posture to be blocked from), so it
			// must carry the sage_gate instruction clause, not the sage_stamp
			// one the blocked variant uses.
			if !strings.Contains(res.Tip, "Call ws/tickets.sage_gate(stem, landing: \"ready\") to resolve it") {
				t.Fatalf("Tip = %q, want the unreviewed variant's sage_gate instruction clause", res.Tip)
			}
			body := readCreatedTicket(t, root, res)
			// C4: completeness must be stamped too (same resolved posture as
			// design for a brand-new ticket), so the warning's silence about
			// completeness matches an actually-terminal completeness field
			// rather than an unstamped one that ws/git.commit would also
			// reject.
			if !strings.Contains(body, "sage-review-completeness: ") {
				t.Fatalf("ready stub must stamp sage-review-completeness alongside design: %q", body)
			}
			if _, statErr := os.Stat(filepath.Join(root, "ai-docs", "tickets", "ready", "260101-feat-foo.md")); statErr != nil {
				t.Fatalf("ticket file should have been created, stat err = %v", statErr)
			}
		})
	}
}

func TestTicketCreateExemptCategoryStampsNoSageReviewField(t *testing.T) {
	for _, category := range []string{"research"} {
		t.Run(category, func(t *testing.T) {
			root := t.TempDir()
			res, err := TicketCreate(root, TicketCreateOptions{Stem: category + "-foo", InitialState: "todo", SageReview: "auto", Today: "260101"})
			if err != nil {
				t.Fatalf("TicketCreate todo: %v", err)
			}
			body := readCreatedTicket(t, root, res)
			if strings.Contains(body, "sage-review") {
				t.Fatalf("exempt category stub must not contain sage-review: %q", body)
			}
		})
	}
}

func TestTicketCreateBoardCategoryAtReadyRejectsWithoutWrites(t *testing.T) {
	for _, category := range []string{"epic", "research", "workset"} {
		t.Run(category, func(t *testing.T) {
			root := t.TempDir()
			opts := TicketCreateOptions{Stem: category + "-foo", InitialState: "ready", SageReview: "auto", Today: "260101"}
			res, err := TicketCreate(root, opts)
			if err == nil || res != (TicketCreateResult{}) {
				t.Fatalf("TicketCreate ready = %+v, %v; want rejection", res, err)
			}
			if !strings.Contains(err.Error(), category+" tickets never enter ready/") {
				t.Fatalf("expected ready-category rejection, got %v", err)
			}
			if _, err := os.Stat(filepath.Join(root, "ai-docs")); !os.IsNotExist(err) {
				t.Fatalf("rejected creation changed the filesystem: %v", err)
			}
			path := "ai-docs/tickets/ready/260101-" + category + "-foo.md"
			const original = "existing ticket content\n"
			mustWrite(t, root, path, original)
			if _, err := TicketCreate(root, opts); err == nil {
				t.Fatal("expected rejection with an existing ticket")
			}
			if got := readFileString(t, filepath.Join(root, path)); got != original {
				t.Fatalf("rejected creation changed existing ticket: %q", got)
			}
		})
	}
}

func TestTicketCreateTerminalState(t *testing.T) {
	root := t.TempDir()
	for _, state := range []string{"done", "dropped"} {
		if _, err := TicketCreate(root, TicketCreateOptions{Stem: "feat-foo", InitialState: state, Today: "260101"}); err == nil {
			t.Fatalf("TicketCreate %q: expected error, got nil", state)
		}
	}
}

func TestTicketCreateEmptyStem(t *testing.T) {
	root := t.TempDir()
	if _, err := TicketCreate(root, TicketCreateOptions{Stem: "", InitialState: "idea", Today: "260101"}); err == nil {
		t.Fatalf("TicketCreate empty stem: expected error, got nil")
	}
}

func TestTicketCreateDuplicateFile(t *testing.T) {
	root := t.TempDir()
	opts := TicketCreateOptions{Stem: "feat-foo", InitialState: "idea", Today: "260101"}
	if _, err := TicketCreate(root, opts); err != nil {
		t.Fatalf("first TicketCreate: %v", err)
	}
	if _, err := TicketCreate(root, opts); err == nil {
		t.Fatalf("second TicketCreate: expected error, got nil")
	}
}

func TestTicketCreateDatePrefix(t *testing.T) {
	root := t.TempDir()
	res, err := TicketCreate(root, TicketCreateOptions{Stem: "feat-foo", InitialState: "idea", Today: "260101"})
	if err != nil {
		t.Fatalf("TicketCreate: %v", err)
	}
	if !strings.HasPrefix(res.Path, "ai-docs/tickets/idea/260101-") {
		t.Fatalf("path = %q, want prefix ai-docs/tickets/idea/260101-", res.Path)
	}
}

// TestTicketCreateDatePrefixDedup covers the create_empty coercion trap: stem
// is contractually dateless, but a caller passing the dated form "stem" takes
// everywhere else (filenames, git log --grep, ticket references) must not
// silently double the prefix. A leading YYMMDD- equal to today is a harmless
// duplicate (stripped); one that differs is ambiguous (rejected); no prefix
// passes through unchanged.
func TestTicketCreateDatePrefixDedup(t *testing.T) {
	t.Run("equal-today dedup", func(t *testing.T) {
		root := t.TempDir()
		res, err := TicketCreate(root, TicketCreateOptions{Stem: "260101-feat-foo", InitialState: "idea", Today: "260101"})
		if err != nil {
			t.Fatalf("TicketCreate: %v", err)
		}
		if res.Path != "ai-docs/tickets/idea/260101-feat-foo.md" {
			t.Fatalf("path = %q, want the date prefix deduped, not doubled", res.Path)
		}
	})

	t.Run("differ-reject", func(t *testing.T) {
		root := t.TempDir()
		_, err := TicketCreate(root, TicketCreateOptions{Stem: "260102-feat-foo", InitialState: "idea", Today: "260101"})
		if err == nil {
			t.Fatal("expected rejection for a stem date prefix that differs from today, got nil")
		}
		msg := err.Error()
		for _, want := range []string{"260102", "260101", "dateless semantic stem"} {
			if !strings.Contains(msg, want) {
				t.Fatalf("error %q missing %q", msg, want)
			}
		}
		if _, statErr := os.Stat(filepath.Join(root, "ai-docs")); !os.IsNotExist(statErr) {
			t.Fatalf("rejected creation must not touch the filesystem: %v", statErr)
		}
	})

	t.Run("no-prefix passthrough", func(t *testing.T) {
		root := t.TempDir()
		res, err := TicketCreate(root, TicketCreateOptions{Stem: "feat-foo", InitialState: "idea", Today: "260101"})
		if err != nil {
			t.Fatalf("TicketCreate: %v", err)
		}
		if res.Path != "ai-docs/tickets/idea/260101-feat-foo.md" {
			t.Fatalf("path = %q, want today's date prepended once", res.Path)
		}
	})
}

func TestTicketCreateActionableTodoHasNoReviewPosture(t *testing.T) {
	for _, category := range []string{"feat", "bug", "refactor", "chore"} {
		root := t.TempDir()
		res, err := TicketCreate(root, TicketCreateOptions{Stem: category + "-backlog", InitialState: "todo", SageReview: "auto", Today: "260101"})
		if err != nil {
			t.Fatal(err)
		}
		body := readFileString(t, filepath.Join(root, res.Path))
		if strings.Contains(body, "sage-review") {
			t.Fatalf("ungated backlog acquired posture: %s", body)
		}
		if !strings.Contains(res.Tip, "todo authoring is ungated") {
			t.Fatalf("misleading tip: %s", res.Tip)
		}
	}
}
