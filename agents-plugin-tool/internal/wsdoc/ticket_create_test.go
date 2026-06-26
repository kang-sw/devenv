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

func TestTicketCreateTodo(t *testing.T) {
	root := t.TempDir()
	res, err := TicketCreate(root, TicketCreateOptions{Stem: "feat-foo", InitialState: "todo", Today: "260101"})
	if err != nil {
		t.Fatalf("TicketCreate todo: %v", err)
	}
	body := readCreatedTicket(t, root, res)
	if !strings.Contains(body, `title: ""`) {
		t.Fatalf("todo stub missing title: %q", body)
	}
	if !strings.Contains(body, "sage-review: pending") {
		t.Fatalf("todo stub missing sage-review: pending: %q", body)
	}
}

func TestTicketCreateReady(t *testing.T) {
	root := t.TempDir()
	res, err := TicketCreate(root, TicketCreateOptions{Stem: "feat-foo", InitialState: "ready", Today: "260101"})
	if err != nil {
		t.Fatalf("TicketCreate ready: %v", err)
	}
	body := readCreatedTicket(t, root, res)
	if !strings.Contains(body, `title: ""`) {
		t.Fatalf("ready stub missing title: %q", body)
	}
	if !strings.Contains(body, "sage-review: pending") {
		t.Fatalf("ready stub missing sage-review: pending: %q", body)
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
