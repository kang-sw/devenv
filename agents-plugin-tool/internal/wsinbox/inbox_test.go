package wsinbox

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func send(t *testing.T, dir, text string, chips ...Chip) (string, bool) {
	t.Helper()
	var parts []string
	if text != "" {
		parts = []string{text}
	}
	name, merged, err := Send(dir, Entry{Parts: parts, Chips: chips}, time.Now())
	if err != nil {
		t.Fatalf("Send(%q): %v", text, err)
	}
	return name, merged
}

func TestAnswerBlockMatchesTheBoardReplyGrammar(t *testing.T) {
	cases := []struct {
		chips []Chip
		want  string
	}{
		{[]Chip{{ID: "03", Answer: "a"}, {ID: "04", Answer: "ok"}}, "(03) a; (04) ok"},
		{[]Chip{{ID: "01", Answer: "ok"}, {ID: "02", Answer: "b"}, {ID: "05", Answer: "ok"}}, "(02) b; (01), (05) ok"},
		{[]Chip{{ID: "03", Answer: "a", Note: "but later"}}, "(03) a, note: but later"},
		{[]Chip{{ID: "07", Answer: "ok", Note: "seen"}}, "(07) ok, note: seen"},
		{[]Chip{{ID: "08", Note: "why?"}}, "(08) note: why?"},
		{[]Chip{{ID: "09", Answer: "no", Note: "use b"}}, "(09) no, note: use b"},
		{nil, ""},
	}
	for _, c := range cases {
		if got := AnswerBlock(c.chips); got != c.want {
			t.Errorf("AnswerBlock(%+v) = %q, want %q", c.chips, got, c.want)
		}
	}
}

func TestEntryTextKeepsEachSendAsAParagraph(t *testing.T) {
	e := Entry{Parts: []string{"first", "second\nline"}, Chips: []Chip{{ID: "03", Answer: "a"}}}
	if got, want := e.Text(), "first\n\nsecond\nline\n\n(03) a"; got != want {
		t.Fatalf("Text = %q, want %q", got, want)
	}
	if got := (Entry{Chips: []Chip{{ID: "01", Answer: "ok"}}}).Text(); got != "(01) ok" {
		t.Fatalf("chips-only Text = %q", got)
	}
}

func TestNormalizeRejectsEmptyAndMalformed(t *testing.T) {
	if _, err := Normalize(Entry{Parts: []string{"  "}}); !errors.Is(err, ErrEmpty) {
		t.Fatalf("blank send: %v, want ErrEmpty", err)
	}
	for _, c := range []Chip{{ID: "../x", Answer: "a"}, {ID: "03", Answer: "a b"}, {ID: "", Answer: "a"}} {
		if _, err := Normalize(Entry{Chips: []Chip{c}}); !errors.Is(err, ErrInvalid) {
			t.Errorf("chip %+v accepted", c)
		}
	}
	e, err := Normalize(Entry{Chips: []Chip{{ID: "03", Answer: "a"}, {ID: "03", Answer: "b"}, {ID: "04"}}, Parts: []string{" hi "}})
	if err != nil {
		t.Fatal(err)
	}
	if len(e.Chips) != 1 || e.Chips[0].Answer != "b" || e.Parts[0] != "hi" {
		t.Fatalf("normalized = %+v", e)
	}
}

func TestSendWritesThenMergesIntoTheHeldEntry(t *testing.T) {
	dir := Dir(t.TempDir())
	first, merged := send(t, dir, "hello", Chip{ID: "03", Answer: "a"}, Chip{ID: "04", Answer: "ok"})
	if merged {
		t.Fatal("first send merged")
	}
	second, merged := send(t, dir, "and more", Chip{ID: "03", Answer: "b", Note: "changed"})
	if !merged || second == first {
		t.Fatalf("second send: merged=%v name=%s (first %s)", merged, second, first)
	}
	held, err := Held(dir)
	if err != nil || len(held) != 1 || held[0].Name != second {
		t.Fatalf("held = %+v, %v", held, err)
	}
	e := held[0].Entry
	if got, want := e.Text(), "hello\n\nand more\n\n(03) b, note: changed; (04) ok"; got != want {
		t.Fatalf("merged text = %q, want %q", got, want)
	}
	if len(e.Parts) != 2 {
		t.Fatalf("parts = %q", e.Parts)
	}
}

func TestCancelReturnsTheHeldEntry(t *testing.T) {
	dir := Dir(t.TempDir())
	name, _ := send(t, dir, "hello", Chip{ID: "03", Answer: "a"})
	e, err := Cancel(dir, name)
	if err != nil || e.Text() != "hello\n\n(03) a" {
		t.Fatalf("Cancel = %+v, %v", e, err)
	}
	if names, _ := Names(dir); len(names) != 0 {
		t.Fatalf("inbox after cancel = %v", names)
	}
	if _, err := Cancel(dir, name); !errors.Is(err, ErrGone) {
		t.Fatalf("second cancel: %v, want ErrGone", err)
	}
}

func TestCancelRefusesAClaimedEntryAndForeignNames(t *testing.T) {
	dir := Dir(t.TempDir())
	name, _ := send(t, dir, "hello")
	if got, err := Drain(dir); err != nil || len(got) != 1 {
		t.Fatalf("Drain = %+v, %v", got, err)
	}
	if _, err := Cancel(dir, name); !errors.Is(err, ErrGone) {
		t.Fatalf("cancel after drain: %v, want ErrGone", err)
	}
	if _, err := Cancel(dir, "../../registration.json"); !errors.Is(err, ErrGone) {
		t.Fatalf("foreign name: %v", err)
	}
	if _, ok := NameForID("../x"); ok {
		t.Fatal("NameForID accepted a path")
	}
}

func TestDrainClaimsOnceAndKeepsRecords(t *testing.T) {
	dir := Dir(t.TempDir())
	if got, err := Drain(dir); err != nil || got != nil {
		t.Fatalf("drain of a missing inbox = %+v, %v", got, err)
	}
	name, _ := send(t, dir, "hello")
	got, err := Drain(dir)
	if err != nil || len(got) != 1 || got[0].Name != name || got[0].Entry.Text() != "hello" {
		t.Fatalf("Drain = %+v, %v", got, err)
	}
	if again, _ := Drain(dir); len(again) != 0 {
		t.Fatalf("second drain = %+v", again)
	}
	if rec, _ := Drained(dir); len(rec) != 1 || rec[0].Name != name {
		t.Fatalf("drained records = %+v", rec)
	}
	// A send after the drain starts a new entry instead of merging.
	if _, merged := send(t, dir, "next"); merged {
		t.Fatal("send after drain merged into a drained entry")
	}
}

func TestDrainedRecordsAreCapped(t *testing.T) {
	dir := Dir(t.TempDir())
	for i := 0; i < DrainedCap+3; i++ {
		send(t, dir, "m")
		if _, err := Drain(dir); err != nil {
			t.Fatal(err)
		}
	}
	if names, _ := DrainedNames(dir); len(names) != DrainedCap {
		t.Fatalf("drained records = %d, want %d", len(names), DrainedCap)
	}
}

func TestReadersIgnoreUnknownFieldsAndStrayFiles(t *testing.T) {
	dir := Dir(t.TempDir())
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	name := "20261106T120000.000000000Z-0123abcd.json"
	if err := os.WriteFile(filepath.Join(dir, name), []byte(`{"schemaVersion":9,"parts":["x"],"future":{"a":1}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, stray := range []string{".tmp-1", "notes.txt", ".x.json.merge-1"} {
		_ = os.WriteFile(filepath.Join(dir, stray), []byte("{}"), 0o644)
	}
	held, err := Held(dir)
	if err != nil || len(held) != 1 || held[0].Entry.Text() != "x" || held[0].ID() != strings.TrimSuffix(name, ".json") {
		t.Fatalf("Held = %+v, %v", held, err)
	}
}

// TestDrainAndCancelRaceHasOneWinner runs a drain and a cancel against one
// held entry many times: exactly one of them gets it, never both.
func TestDrainAndCancelRaceHasOneWinner(t *testing.T) {
	for i := 0; i < 300; i++ {
		dir := Dir(t.TempDir())
		name, _ := send(t, dir, "hello")
		var (
			wg        sync.WaitGroup
			drained   []Record
			cancelled bool
			cancelErr error
		)
		wg.Add(2)
		go func() { defer wg.Done(); drained, _ = Drain(dir) }()
		go func() {
			defer wg.Done()
			_, cancelErr = Cancel(dir, name)
			cancelled = cancelErr == nil
		}()
		wg.Wait()
		if cancelErr != nil && !errors.Is(cancelErr, ErrGone) {
			t.Fatalf("cancel error: %v", cancelErr)
		}
		if cancelled == (len(drained) == 1) {
			t.Fatalf("run %d: cancelled=%v drained=%d; want exactly one winner", i, cancelled, len(drained))
		}
		if names, _ := Names(dir); len(names) != 0 {
			t.Fatalf("run %d: entry left held: %v", i, names)
		}
	}
}

// TestDrainAndMergeRaceLosesNothing runs a drain against a merging send many
// times: the first send's text is delivered or held exactly once, and so is
// the second's; a drained entry never comes back.
func TestDrainAndMergeRaceLosesNothing(t *testing.T) {
	for i := 0; i < 300; i++ {
		dir := Dir(t.TempDir())
		send(t, dir, "one", Chip{ID: "03", Answer: "a"})
		var (
			wg      sync.WaitGroup
			drained []Record
			merged  bool
			sendErr error
		)
		wg.Add(2)
		go func() { defer wg.Done(); drained, _ = Drain(dir) }()
		go func() {
			defer wg.Done()
			_, merged, sendErr = Send(dir, Entry{Parts: []string{"two"}, Chips: []Chip{{ID: "03", Answer: "b"}}}, time.Now())
		}()
		wg.Wait()
		if sendErr != nil {
			t.Fatalf("send: %v", sendErr)
		}
		held, _ := Held(dir)
		var texts []string
		for _, r := range append(drained, held...) {
			texts = append(texts, r.Entry.Text())
		}
		all := strings.Join(texts, "|")
		if strings.Count(all, "one") != 1 || strings.Count(all, "two") != 1 {
			t.Fatalf("run %d: delivered+held = %q", i, texts)
		}
		if merged {
			// The merge won: nothing of the first send was drained on its own.
			for _, r := range drained {
				if r.Entry.Text() == "one\n\n(03) a" {
					t.Fatalf("run %d: merged entry also drained alone", i)
				}
			}
		} else if len(drained) != 1 || drained[0].Entry.Text() != "one\n\n(03) a" {
			t.Fatalf("run %d: send did not merge, but the drain got %+v", i, drained)
		}
	}
}

func TestUnreadableEntryStaysHeld(t *testing.T) {
	dir := Dir(t.TempDir())
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	name := "20261106T120000.000000000Z-0123abcd.json"
	if err := os.WriteFile(filepath.Join(dir, name), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Cancel(dir, name); err == nil || errors.Is(err, ErrGone) {
		t.Fatalf("cancel of an unreadable entry: %v", err)
	}
	if got, err := Drain(dir); err == nil || len(got) != 0 {
		t.Fatalf("drain of an unreadable entry = %+v, %v", got, err)
	}
	if names, _ := Names(dir); len(names) != 1 || names[0] != name {
		t.Fatalf("unreadable entry not left held: %v", names)
	}
}

func TestSteerRewritesTheEntryMarkedUnderAFreshName(t *testing.T) {
	dir := Dir(t.TempDir())
	name, _ := send(t, dir, "hello", Chip{ID: "03", Answer: "a"})
	steered, err := Steer(dir, name)
	if err != nil || steered == name {
		t.Fatalf("Steer = %q, %v (was %q)", steered, err, name)
	}
	held, err := Held(dir)
	if err != nil || len(held) != 1 || held[0].Name != steered {
		t.Fatalf("held after steer = %+v, %v", held, err)
	}
	if e := held[0].Entry; !e.Steer || e.Text() != "hello\n\n(03) a" || e.SentAt == 0 {
		t.Fatalf("steered entry = %+v", e)
	}
	// Steering it again leaves it as it is.
	if again, err := Steer(dir, steered); err != nil || again != steered {
		t.Fatalf("second Steer = %q, %v", again, err)
	}
	// The original name is gone, so a stale steer or cancel of it is refused.
	if _, err := Steer(dir, name); !errors.Is(err, ErrGone) {
		t.Fatalf("steer of the old name: %v, want ErrGone", err)
	}
	if _, err := Steer(dir, "../../registration"); !errors.Is(err, ErrGone) {
		t.Fatalf("steer of a foreign name: %v, want ErrGone", err)
	}
	// A steered entry is still held: cancel takes it.
	if e, err := Cancel(dir, steered); err != nil || !e.Steer {
		t.Fatalf("cancel of a steered entry = %+v, %v", e, err)
	}
	if names, _ := Names(dir); len(names) != 0 {
		t.Fatalf("inbox after cancel = %v", names)
	}
}

func TestSendNeverMergesIntoASteeredEntry(t *testing.T) {
	dir := Dir(t.TempDir())
	name, _ := send(t, dir, "promoted")
	steered, err := Steer(dir, name)
	if err != nil {
		t.Fatal(err)
	}
	if target, ok, err := MergeTarget(dir); err != nil || ok {
		t.Fatalf("merge target with only a steered entry = %+v, %v", target, err)
	}
	next, merged := send(t, dir, "later")
	if merged {
		t.Fatal("send merged into a steered entry")
	}
	// The next send merges into the unsteered entry, not the steered one.
	third, merged := send(t, dir, "and more")
	if !merged {
		t.Fatal("send did not merge into the unsteered held entry")
	}
	held, _ := Held(dir)
	if len(held) != 2 || held[0].Name != steered || held[1].Name != third || held[1].Name == next {
		t.Fatalf("held = %+v", held)
	}
	if held[0].Entry.Text() != "promoted" || held[1].Entry.Text() != "later\n\nand more" || held[1].Entry.Steer {
		t.Fatalf("held texts = %q / %q", held[0].Entry.Text(), held[1].Entry.Text())
	}
	// A steered entry newer than the unsteered one is skipped as merge target.
	if _, err := Steer(dir, third); err != nil {
		t.Fatal(err)
	}
	fourth, _ := send(t, dir, "fourth")
	if target, ok, _ := MergeTarget(dir); !ok || target.Name != fourth || target.Entry.Text() != "fourth" {
		t.Fatalf("merge target = %+v, want the newest unsteered %q", target, fourth)
	}
}

func TestSteerOnlyDrainTakesOnlySteeredEntries(t *testing.T) {
	dir := Dir(t.TempDir())
	if got, err := DrainSteer(dir); err != nil || got != nil {
		t.Fatalf("steer drain of a missing inbox = %+v, %v", got, err)
	}
	first, _ := send(t, dir, "promoted")
	steered, err := Steer(dir, first)
	if err != nil {
		t.Fatal(err)
	}
	followUp, _ := send(t, dir, "follow-up")
	if names, err := SteerNames(dir); err != nil || len(names) != 1 || names[0] != steered {
		t.Fatalf("SteerNames = %v, %v", names, err)
	}
	got, err := DrainSteer(dir)
	if err != nil || len(got) != 1 || got[0].Name != steered || got[0].Entry.Text() != "promoted" {
		t.Fatalf("DrainSteer = %+v, %v", got, err)
	}
	if again, _ := DrainSteer(dir); len(again) != 0 {
		t.Fatalf("second steer drain = %+v", again)
	}
	if names, _ := SteerNames(dir); len(names) != 0 {
		t.Fatalf("SteerNames after the steer drain = %v", names)
	}
	// The follow-up stays held for the full drain.
	if held, _ := Held(dir); len(held) != 1 || held[0].Name != followUp {
		t.Fatalf("held after the steer drain = %+v", held)
	}
	rest, err := Drain(dir)
	if err != nil || len(rest) != 1 || rest[0].Name != followUp {
		t.Fatalf("full drain = %+v, %v", rest, err)
	}
	if rec, _ := Drained(dir); len(rec) != 2 || !rec[0].Entry.Steer || rec[1].Entry.Steer {
		t.Fatalf("drained records = %+v", rec)
	}
}

func TestFullDrainAlsoTakesSteeredEntries(t *testing.T) {
	dir := Dir(t.TempDir())
	first, _ := send(t, dir, "promoted")
	if _, err := Steer(dir, first); err != nil {
		t.Fatal(err)
	}
	send(t, dir, "follow-up")
	got, err := Drain(dir)
	if err != nil || len(got) != 2 || !got[0].Entry.Steer || got[1].Entry.Text() != "follow-up" {
		t.Fatalf("Drain = %+v, %v", got, err)
	}
}

func TestSteerAndDrainRaceHasOneWinner(t *testing.T) {
	for i := 0; i < 50; i++ {
		dir := Dir(t.TempDir())
		name, _ := send(t, dir, "hello")
		var wg sync.WaitGroup
		var steered string
		var steerErr error
		var drained []Record
		wg.Add(2)
		go func() { defer wg.Done(); steered, steerErr = Steer(dir, name) }()
		go func() { defer wg.Done(); drained, _ = Drain(dir) }()
		wg.Wait()
		held, _ := Held(dir)
		total := len(drained) + len(held)
		if total > 1 {
			t.Fatalf("message duplicated: drained %+v held %+v", drained, held)
		}
		if steerErr == nil && total == 0 {
			t.Fatalf("steer won (%s) but the message is gone", steered)
		}
		if steerErr != nil && !errors.Is(steerErr, ErrGone) {
			t.Fatalf("steer error: %v", steerErr)
		}
		// Whatever is left is drained once.
		rest, _ := Drain(dir)
		if len(drained)+len(rest) != 1 {
			t.Fatalf("message lost or duplicated: drained %d, rest %d", len(drained), len(rest))
		}
	}
}
