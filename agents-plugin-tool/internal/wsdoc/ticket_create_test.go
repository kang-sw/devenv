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

func TestTicketCreateTodoStampsResolvedSageReviewDesignPosture(t *testing.T) {
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
			res, err := TicketCreate(root, TicketCreateOptions{Stem: "feat-foo", InitialState: "todo", SageReview: tc.config, Today: "260101"})
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
			wantLine := "sage-review-design: " + tc.wantReview
			if !strings.Contains(body, wantLine) {
				t.Fatalf("ready stub missing %s: %q", wantLine, body)
			}
			if strings.Contains(body, "sage-review-completeness:") {
				t.Fatalf("ready stub must not stamp sage-review-completeness: %q", body)
			}
			if !strings.Contains(res.Tip, tc.wantReview) {
				t.Fatalf("Tip = %q, want resolved posture %q", res.Tip, tc.wantReview)
			}
		})
	}
}

// TestTicketCreateReadyBlocksUnresolvedSageReviewDesignPosture asserts the
// never-skippable design-review invariant at direct-to-ready creation: a
// ticket created directly at ready/ with no prior "from" state has no
// opportunity to have already run design review, so a freshly resolved
// non-terminal posture (recommended/required) must block creation rather
// than silently stamping and succeeding.
func TestTicketCreateReadyBlocksUnresolvedSageReviewDesignPosture(t *testing.T) {
	for _, tc := range []struct {
		name    string
		config  string
		wantErr string
	}{
		{"ask", "ask", "run sage review or skip recommended review"},
		{"auto", "auto", "run sage review"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			if _, err := TicketCreate(root, TicketCreateOptions{Stem: "feat-foo", InitialState: "ready", SageReview: tc.config, Today: "260101"}); err == nil {
				t.Fatal("TicketCreate ready: expected error for unresolved design posture, got nil")
			} else if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want %q", err, tc.wantErr)
			}
			if _, statErr := os.Stat(filepath.Join(root, "ai-docs", "tickets", "ready", "260101-feat-foo.md")); !os.IsNotExist(statErr) {
				t.Fatalf("ticket file should not have been created, stat err = %v", statErr)
			}
		})
	}
}

func TestTicketCreateExemptCategoryStampsNoSageReviewField(t *testing.T) {
	for _, category := range []string{"research", "workset"} {
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

// TestTicketCreateExemptCategoryAtReadyStampsNoSageReviewField exercises the
// exempt-category branch at direct-to-ready creation (ticket_create.go:60):
// designRequired must be false for research/workset so the never-skippable
// design-invariant check on that line does not fire even though state ==
// "ready", and creation succeeds with no sage-review-* field stamped despite
// SageReview resolving to a non-terminal posture.
func TestTicketCreateExemptCategoryAtReadyStampsNoSageReviewField(t *testing.T) {
	for _, category := range []string{"research", "workset"} {
		t.Run(category, func(t *testing.T) {
			root := t.TempDir()
			res, err := TicketCreate(root, TicketCreateOptions{Stem: category + "-foo", InitialState: "ready", SageReview: "auto", Today: "260101"})
			if err != nil {
				t.Fatalf("TicketCreate ready: %v", err)
			}
			body := readCreatedTicket(t, root, res)
			if strings.Contains(body, "sage-review") {
				t.Fatalf("exempt category stub must not contain sage-review: %q", body)
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
