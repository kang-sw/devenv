package wsinbox

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func sendCommand(t *testing.T, dir, name, args string) string {
	t.Helper()
	got, merged, err := Send(dir, Entry{Command: &Command{Name: name, Args: args}}, time.Now())
	if err != nil {
		t.Fatalf("Send(/%s): %v", name, err)
	}
	if merged {
		t.Fatalf("command /%s merged", name)
	}
	return got
}

func TestCommandEntryTextIsItsTypedLine(t *testing.T) {
	if got := (Entry{Command: &Command{Name: "compact"}}).Text(); got != "/compact" {
		t.Fatalf("bare Text = %q", got)
	}
	if got := (Entry{Command: &Command{Name: "compact", Args: "keep the plan"}}).Text(); got != "/compact keep the plan" {
		t.Fatalf("args Text = %q", got)
	}
}

func TestNormalizeChecksACommand(t *testing.T) {
	e, err := Normalize(Entry{Command: &Command{Name: " /ws:lead-run ", Args: "  ticket  "}, SentAt: 5})
	if err != nil {
		t.Fatal(err)
	}
	if e.Command.Name != "ws:lead-run" || e.Command.Args != "ticket" || e.SentAt != 5 {
		t.Fatalf("normalized = %+v %+v", e, *e.Command)
	}
	bad := []Entry{
		{Command: &Command{Name: ""}},
		{Command: &Command{Name: "two words"}},
		{Command: &Command{Name: "../x"}},
		{Command: &Command{Name: "compact"}, Parts: []string{"text"}},
		{Command: &Command{Name: "compact"}, Chips: []Chip{{ID: "01", Answer: "ok"}}},
		{Command: &Command{Name: "compact"}, Mentions: []Mention{{Topic: "t", Text: "x"}}},
		{Command: &Command{Name: "compact"}, Images: []string{"/tmp/a.png"}},
		{Command: &Command{Name: "compact"}, Steer: true},
	}
	for _, b := range bad {
		if _, err := Normalize(b); !errors.Is(err, ErrInvalid) {
			t.Errorf("Normalize(%+v) = %v, want ErrInvalid", b, err)
		}
	}
	// A blank part is no text: the composer may send an empty text field.
	if _, err := Normalize(Entry{Command: &Command{Name: "compact"}, Parts: []string{"  "}}); err != nil {
		t.Fatalf("blank part refused: %v", err)
	}
}

func TestCommandIsNeverMergedOrMergedInto(t *testing.T) {
	dir := t.TempDir()
	text, _ := send(t, dir, "before")
	cmd := sendCommand(t, dir, "compact", "")
	if cmd == text {
		t.Fatal("command reused the text entry")
	}
	// A send after a command never joins the text before it, which would
	// deliver that text ahead of the command.
	if rec, ok, _ := MergeTarget(dir); ok {
		t.Fatalf("MergeTarget = %s past a command entry", rec.Name)
	}
	if _, merged := send(t, dir, "after"); merged {
		t.Fatal("text after a command merged into the text before it")
	}
	if held, _ := Held(dir); len(held) != 3 || held[0].Name != text || held[1].Name != cmd {
		t.Fatalf("held = %+v", held)
	}
	// A command alone is no merge target.
	only := t.TempDir()
	sendCommand(t, only, "compact", "")
	if _, ok, _ := MergeTarget(only); ok {
		t.Fatal("a command entry is a merge target")
	}
	after, merged := send(t, only, "after")
	if merged {
		t.Fatal("text merged into a command entry")
	}
	held, err := Held(only)
	if err != nil || len(held) != 2 || held[0].Entry.Command == nil || held[1].Name != after || held[1].Entry.Command != nil {
		t.Fatalf("held = %+v %v", held, err)
	}
	// Two commands stay two entries.
	sendCommand(t, only, "context", "")
	if held, _ := Held(only); len(held) != 3 {
		t.Fatalf("held after a second command = %d entries", len(held))
	}
}

func TestSteerRefusesACommand(t *testing.T) {
	dir := t.TempDir()
	cmd := sendCommand(t, dir, "compact", "")
	if _, err := Steer(dir, cmd); !errors.Is(err, ErrCommand) {
		t.Fatalf("Steer(command) = %v, want ErrCommand", err)
	}
	held, _ := Held(dir)
	if len(held) != 1 || held[0].Name != cmd || held[0].Entry.Steer {
		t.Fatalf("held after the refused steer = %+v", held)
	}
	if recs, _ := DrainSteer(dir); len(recs) != 0 {
		t.Fatalf("steer drain took %d entries", len(recs))
	}
}

func TestHeldCommandCancelsAndInterruptReturnsIt(t *testing.T) {
	dir := t.TempDir()
	cmd := sendCommand(t, dir, "compact", "focus")
	e, err := Cancel(dir, cmd)
	if err != nil || e.Command == nil || e.Command.Name != "compact" || e.Command.Args != "focus" {
		t.Fatalf("Cancel = %+v %v", e, err)
	}
	send(t, dir, "text")
	sendCommand(t, dir, "context", "")
	held, _, err := Interrupt(dir, time.Now())
	if err != nil || len(held) != 2 || held[0].Entry.Command != nil || held[1].Entry.Command == nil || held[1].Entry.Command.Name != "context" {
		t.Fatalf("Interrupt = %+v %v", held, err)
	}
}

func TestDrainKeepsACommandInOrder(t *testing.T) {
	dir := t.TempDir()
	send(t, dir, "A")
	sendCommand(t, dir, "compact", "")
	send(t, dir, "B")
	recs, err := Drain(dir)
	if err != nil || len(recs) != 3 {
		t.Fatalf("Drain = %d records, %v", len(recs), err)
	}
	if recs[0].Entry.Text() != "A" || recs[1].Entry.Command == nil || recs[1].Entry.Text() != "/compact" || recs[2].Entry.Text() != "B" {
		t.Fatalf("drained order = %q %q %q", recs[0].Entry.Text(), recs[1].Entry.Text(), recs[2].Entry.Text())
	}
}

// TestSteeredTextKeepsItsPlaceBeforeACommand pins that a steer keeps the
// entry's stamp: text sent before a held command, steered but not taken
// mid-turn, still drains ahead of the command.
func TestSteeredTextKeepsItsPlaceBeforeACommand(t *testing.T) {
	dir := t.TempDir()
	a, _ := send(t, dir, "A")
	sendCommand(t, dir, "compact", "")
	steered, err := Steer(dir, a)
	if err != nil || steered == a {
		t.Fatalf("Steer = %s, %v", steered, err)
	}
	if steered[:strings.LastIndexByte(steered, '-')] != a[:strings.LastIndexByte(a, '-')] {
		t.Fatalf("steered name %s lost the stamp of %s", steered, a)
	}
	recs, err := Drain(dir)
	if err != nil || len(recs) != 2 || recs[0].Name != steered || recs[1].Entry.Command == nil {
		t.Fatalf("Drain = %+v, %v; want the steered text first", recs, err)
	}
}
