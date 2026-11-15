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

func TestEntryTextLeadsWithTheAnswerBlock(t *testing.T) {
	e := Entry{Parts: []string{"first", "second\nline"}, Chips: []Chip{{ID: "03", Answer: "a"}}, Topics: []TopicNote{{Topic: "ship", Note: "why?"}}}
	if got, want := e.Text(), "<wb:answers>\n03=a\n[ship]^why?\n</wb:answers>\n\nfirst\n\nsecond\nline"; got != want {
		t.Fatalf("Text = %q, want %q", got, want)
	}
	if got := (Entry{Chips: []Chip{{ID: "01", Answer: "ok"}}}).Text(); got != "<wb:answers>\n01=ok\n</wb:answers>" {
		t.Fatalf("chips-only Text = %q", got)
	}
	if got := (Entry{Parts: []string{"just text"}}).Text(); got != "just text" {
		t.Fatalf("text-only Text = %q", got)
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
	for _, tn := range []TopicNote{{Topic: "", Note: "x"}, {Topic: "a\nb", Note: "x"}, {Topic: "a]^b", Note: "x"}, {Topic: strings.Repeat("t", maxTopicLen+1), Note: "x"}, {Topic: "t", Note: strings.Repeat("n", maxNoteBytes+1)}} {
		if _, err := Normalize(Entry{Topics: []TopicNote{tn}}); !errors.Is(err, ErrInvalid) {
			t.Errorf("topic note %q accepted", tn.Topic)
		}
	}
	e, err = Normalize(Entry{Topics: []TopicNote{{Topic: " ship ", Note: "a"}, {Topic: "ship", Note: " b "}, {Topic: "empty", Note: "  "}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(e.Topics) != 1 || e.Topics[0] != (TopicNote{Topic: "ship", Note: "b"}) {
		t.Fatalf("normalized topics = %+v", e.Topics)
	}
	if _, err := Normalize(Entry{Topics: []TopicNote{{Topic: "empty", Note: " "}}}); !errors.Is(err, ErrEmpty) {
		t.Fatalf("blank topic note only: %v, want ErrEmpty", err)
	}
}

// TestEachSendIsItsOwnHeldEntry: a send never joins a held entry; two
// follow-ups are two entries, held in send order, each with its own text and
// answers.
func TestEachSendIsItsOwnHeldEntry(t *testing.T) {
	dir := Dir(t.TempDir())
	first, merged := send(t, dir, "hello", Chip{ID: "03", Answer: "a"}, Chip{ID: "04", Answer: "ok"})
	if merged {
		t.Fatal("first send merged")
	}
	second, merged := send(t, dir, "and more", Chip{ID: "03", Answer: "b", Note: "changed"})
	if merged || second == first {
		t.Fatalf("second send: merged=%v name=%s (first %s)", merged, second, first)
	}
	held, err := Held(dir)
	if err != nil || len(held) != 2 || held[0].Name != first || held[1].Name != second {
		t.Fatalf("held = %+v, %v", held, err)
	}
	if got, want := held[0].Entry.Text(), "<wb:answers>\n03=a\n04=ok\n</wb:answers>\n\nhello"; got != want {
		t.Fatalf("first text = %q, want %q", got, want)
	}
	if got, want := held[1].Entry.Text(), "<wb:answers>\n03=b^changed\n</wb:answers>\n\nand more"; got != want {
		t.Fatalf("second text = %q, want %q", got, want)
	}
	recs, err := Drain(dir)
	if err != nil || len(recs) != 2 || recs[0].Name != first || recs[1].Name != second {
		t.Fatalf("Drain = %+v, %v; want both in send order", recs, err)
	}
}

func TestSteeredSendDoesNotMergeOrPromoteHeldText(t *testing.T) {
	dir := Dir(t.TempDir())
	first, _ := send(t, dir, "held")
	name, merged, err := Send(dir, Entry{Parts: []string{"steer"}, Steer: true}, time.Now())
	if err != nil || merged || name == first {
		t.Fatalf("steered Send = %s, %v, %v", name, merged, err)
	}
	recs, err := DrainSteer(dir)
	if err != nil || len(recs) != 1 || recs[0].Entry.Text() != "steer" {
		t.Fatalf("steered drain = %+v, %v", recs, err)
	}
	held, err := Held(dir)
	if err != nil || len(held) != 1 || held[0].Name != first || held[0].Entry.Text() != "held" {
		t.Fatalf("unrelated text stays held = %+v, %v", held, err)
	}
}

func TestCancelReturnsTheHeldEntry(t *testing.T) {
	dir := Dir(t.TempDir())
	name, _ := send(t, dir, "hello", Chip{ID: "03", Answer: "a"})
	e, err := Cancel(dir, name)
	if err != nil || e.Text() != "<wb:answers>\n03=a\n</wb:answers>\n\nhello" {
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
	// A send after the drain starts a new entry.
	if next, _ := send(t, dir, "next"); next == name {
		t.Fatal("send after drain reused the drained entry's name")
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

// TestConcurrentClaimHasOneWinner races claimers on one entry directly, the
// exclusive rename every state change starts with: exactly one holds it, its
// claim file keeps the entry whole, and every other claimer gets ErrGone.
func TestConcurrentClaimHasOneWinner(t *testing.T) {
	const claimers = 4
	for i := 0; i < 200; i++ {
		dir := Dir(t.TempDir())
		name, _ := send(t, dir, "hello")
		want, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		var (
			wg      sync.WaitGroup
			start   = make(chan struct{})
			claimed [claimers]string
			errs    [claimers]error
		)
		for c := range claimers {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				claimed[c], errs[c] = claim(dir, name, "test")
			}()
		}
		close(start)
		wg.Wait()
		winners := 0
		for c := range claimers {
			switch {
			case errs[c] == nil:
				winners++
				got, err := os.ReadFile(claimed[c])
				if err != nil || string(got) != string(want) {
					t.Fatalf("run %d: the winner's claim file = %q, %v; want the entry", i, got, err)
				}
			case !errors.Is(errs[c], ErrGone):
				t.Fatalf("run %d: a losing claim = %v, want ErrGone", i, errs[c])
			}
		}
		if winners != 1 {
			t.Fatalf("run %d: %d claimers won, want exactly one", i, winners)
		}
		if _, err := os.Stat(filepath.Join(dir, name)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("run %d: the entry is still held after its claim: %v", i, err)
		}
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

// TestDrainAndSendRaceLosesNothing runs a drain against a send many times:
// the first send's text is delivered exactly once, the second's is delivered
// or held exactly once, and a drained entry never comes back.
func TestDrainAndSendRaceLosesNothing(t *testing.T) {
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
			t.Fatalf("run %d: send merged", i)
		}
		if len(drained) == 0 || drained[0].Entry.Text() != "<wb:answers>\n03=a\n</wb:answers>\n\none" {
			t.Fatalf("run %d: the drain got %+v, want the first send first", i, drained)
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
	if e := held[0].Entry; !e.Steer || e.Text() != "<wb:answers>\n03=a\n</wb:answers>\n\nhello" || e.SentAt == 0 {
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

// texts is each record's text, in order.
func texts(recs []Record) []string {
	out := make([]string, len(recs))
	for i, r := range recs {
		out[i] = r.Entry.Text()
	}
	return out
}

// TestSteerMovesTheEntryToTheEndOfTheSteerQueue: the held messages are two
// queues, steered then follow-ups, each in order; promoting a follow-up
// moves it to the end of the steer queue, behind messages steered before
// it even when it was sent first. Held and a full drain agree on that order.
func TestSteerMovesTheEntryToTheEndOfTheSteerQueue(t *testing.T) {
	dir := Dir(t.TempDir())
	f1, _ := send(t, dir, "f1")
	f2, _ := send(t, dir, "f2")
	s1, _, err := Send(dir, Entry{Parts: []string{"s1"}, Steer: true}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	f3, _ := send(t, dir, "f3")
	held, _ := Held(dir)
	if got, want := strings.Join(texts(held), " "), "s1 f1 f2 f3"; got != want {
		t.Fatalf("held order = %q, want %q", got, want)
	}
	// Promote the oldest follow-up: it goes behind s1, not ahead of it.
	p1, err := Steer(dir, f1)
	if err != nil {
		t.Fatal(err)
	}
	if p1 <= s1 {
		t.Fatalf("promoted name %s does not sort after the earlier steer %s", p1, s1)
	}
	held, _ = Held(dir)
	if got, want := strings.Join(texts(held), " "), "s1 f1 f2 f3"; got != want {
		t.Fatalf("held order after promoting f1 = %q, want %q", got, want)
	}
	// Promote the newest follow-up, then an older one: steer order is the
	// order they were promoted in.
	if _, err := Steer(dir, f3); err != nil {
		t.Fatal(err)
	}
	if _, err := Steer(dir, f2); err != nil {
		t.Fatal(err)
	}
	f4, _ := send(t, dir, "f4")
	held, _ = Held(dir)
	if got, want := strings.Join(texts(held), " "), "s1 f1 f3 f2 f4"; got != want {
		t.Fatalf("held order = %q, want %q", got, want)
	}
	for i, r := range held {
		if r.Entry.Steer != (i < 4) {
			t.Fatalf("held[%d] %s steer = %v", i, r.Entry.Text(), r.Entry.Steer)
		}
	}
	if held[len(held)-1].Name != f4 {
		t.Fatalf("follow-up %s not last: %+v", f4, held)
	}
	recs, err := Drain(dir)
	if err != nil || strings.Join(texts(recs), " ") != "s1 f1 f3 f2 f4" {
		t.Fatalf("Drain = %q, %v; want the held order", texts(recs), err)
	}
	if rec, _ := Drained(dir); len(rec) != 5 {
		t.Fatalf("drained records = %d, want 5", len(rec))
	}
}

// TestDrainTakesTheSteerQueueBeforeTheFollowUps: a full drain returns every
// steered entry, in steer order, ahead of every follow-up, in send order.
func TestDrainTakesTheSteerQueueBeforeTheFollowUps(t *testing.T) {
	dir := Dir(t.TempDir())
	send(t, dir, "f1")
	if _, _, err := Send(dir, Entry{Parts: []string{"s1"}, Steer: true}, time.Now()); err != nil {
		t.Fatal(err)
	}
	send(t, dir, "f2")
	if _, _, err := Send(dir, Entry{Parts: []string{"s2"}, Steer: true}, time.Now()); err != nil {
		t.Fatal(err)
	}
	recs, err := Drain(dir)
	if err != nil || strings.Join(texts(recs), " ") != "s1 s2 f1 f2" {
		t.Fatalf("Drain = %q, %v", texts(recs), err)
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

func TestInterruptClaimsEveryHeldMessageAndWritesTheEntry(t *testing.T) {
	dir := Dir(t.TempDir())
	first, _ := send(t, dir, "promoted")
	steered, err := Steer(dir, first)
	if err != nil {
		t.Fatal(err)
	}
	second, _ := send(t, dir, "follow-up", Chip{ID: "03", Answer: "a"})
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	held, name, err := Interrupt(dir, now)
	if err != nil {
		t.Fatalf("Interrupt: %v", err)
	}
	if len(held) != 2 || held[0].Name != steered || held[1].Name != second || held[0].Entry.Text() != "promoted" || held[1].Entry.Text() != "<wb:answers>\n03=a\n</wb:answers>\n\nfollow-up" {
		t.Fatalf("Interrupt held = %+v", held)
	}
	names, _ := Names(dir)
	if len(names) != 1 || names[0] != name {
		t.Fatalf("inbox after Interrupt = %v, want only the interrupt entry %q", names, name)
	}
	e, err := readEntry(filepath.Join(dir, name))
	if err != nil || !e.Interrupt || e.SentAt != now.UnixMilli() || e.SchemaVersion != SchemaVersion || len(e.Parts) != 0 || e.Steer {
		t.Fatalf("interrupt entry = %+v, %v", e, err)
	}
	// Nothing held: the claim is empty and a second entry is written.
	again, name2, err := Interrupt(dir, now.Add(time.Second))
	if err != nil || len(again) != 0 || name2 == name {
		t.Fatalf("second Interrupt = %+v, %q, %v", again, name2, err)
	}
	if names, _ := InterruptNames(dir); len(names) != 2 {
		t.Fatalf("InterruptNames = %v, want both entries", names)
	}
}

func TestInterruptPutsTheClaimsBackWhenTheEntryWriteFails(t *testing.T) {
	dir := Dir(t.TempDir())
	first, _ := send(t, dir, "one")
	steered, err := Steer(dir, first)
	if err != nil {
		t.Fatal(err)
	}
	second, _ := send(t, dir, "two")
	orig := writeInterruptEntry
	writeInterruptEntry = func(string, Entry) (string, error) { return "", errors.New("disk full") }
	t.Cleanup(func() { writeInterruptEntry = orig })
	if held, name, err := Interrupt(dir, time.Now()); err == nil || held != nil || name != "" {
		t.Fatalf("Interrupt with a failing write = %+v, %q, %v", held, name, err)
	}
	names, _ := Names(dir)
	if len(names) != 2 || names[0] != steered || names[1] != second {
		t.Fatalf("inbox after a failed Interrupt = %v, want both messages back", names)
	}
	if des, _ := os.ReadDir(dir); len(des) != 2 {
		t.Fatalf("inbox dir holds %d files, want no claim file left", len(des))
	}
}

func TestInterruptEntryIsNeverAMessage(t *testing.T) {
	dir := Dir(t.TempDir())
	_, name, err := Interrupt(dir, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	msg, merged := send(t, dir, "after")
	if merged || msg == name {
		t.Fatalf("send after an interrupt merged=%v name=%s", merged, msg)
	}
	// Held and MessageNames leave it out; Names and InterruptNames list it.
	if held, _ := Held(dir); len(held) != 1 || held[0].Name != msg {
		t.Fatalf("Held = %+v, want only the message", held)
	}
	if names, _ := MessageNames(dir); len(names) != 1 || names[0] != msg {
		t.Fatalf("MessageNames = %v", names)
	}
	if names, _ := InterruptNames(dir); len(names) != 1 || names[0] != name {
		t.Fatalf("InterruptNames = %v", names)
	}
	if names, _ := Names(dir); len(names) != 2 {
		t.Fatalf("Names = %v, want both entries", names)
	}
	// Cancel and Steer refuse it and leave it in place.
	if _, err := Cancel(dir, name); !errors.Is(err, ErrGone) {
		t.Fatalf("Cancel of an interrupt entry: %v, want ErrGone", err)
	}
	if _, err := Steer(dir, name); !errors.Is(err, ErrGone) {
		t.Fatalf("Steer of an interrupt entry: %v, want ErrGone", err)
	}
	if names, _ := InterruptNames(dir); len(names) != 1 || names[0] != name {
		t.Fatalf("InterruptNames after refused cancel and steer = %v", names)
	}
	if names, _ := MessageNames(Dir(t.TempDir())); names != nil {
		t.Fatalf("MessageNames of a missing inbox = %v", names)
	}
}

func TestDrainsAndTheInterruptEntry(t *testing.T) {
	dir := Dir(t.TempDir())
	first, _ := send(t, dir, "promoted")
	if _, err := Steer(dir, first); err != nil {
		t.Fatal(err)
	}
	_, interrupt, err := Interrupt(dir, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	steered, _ := send(t, dir, "steer me")
	steered, err = Steer(dir, steered)
	if err != nil {
		t.Fatal(err)
	}
	followUp, _ := send(t, dir, "follow-up")

	// The steer-only drain leaves the interrupt entry in place.
	got, err := DrainSteer(dir)
	if err != nil || len(got) != 1 || got[0].Name != steered {
		t.Fatalf("DrainSteer = %+v, %v", got, err)
	}
	if names, _ := InterruptNames(dir); len(names) != 1 || names[0] != interrupt {
		t.Fatalf("InterruptNames after the steer drain = %v", names)
	}
	// The interrupt drain takes only it, keeping no drained record.
	got, err = DrainInterrupt(dir)
	if err != nil || len(got) != 1 || got[0].Name != interrupt || !got[0].Entry.Interrupt || got[0].Entry.SentAt == 0 {
		t.Fatalf("DrainInterrupt = %+v, %v", got, err)
	}
	if again, err := DrainInterrupt(dir); err != nil || len(again) != 0 {
		t.Fatalf("second DrainInterrupt = %+v, %v", again, err)
	}
	if held, _ := Held(dir); len(held) != 1 || held[0].Name != followUp {
		t.Fatalf("held after the interrupt drain = %+v", held)
	}
	if rec, _ := DrainedNames(dir); len(rec) != 1 || rec[0] != steered {
		t.Fatalf("drained records = %v, want only the steered message", rec)
	}

	// The full drain drops an interrupt entry: not returned, not recorded.
	_, interrupt, err = Interrupt(dir, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	later, _ := send(t, dir, "later")
	got, err = Drain(dir)
	if err != nil || len(got) != 1 || got[0].Name != later {
		t.Fatalf("Drain = %+v, %v", got, err)
	}
	if names, _ := Names(dir); len(names) != 0 {
		t.Fatalf("inbox after the full drain = %v", names)
	}
	rec, _ := DrainedNames(dir)
	for _, n := range rec {
		if n == interrupt {
			t.Fatalf("the full drain kept the interrupt entry as a drained record: %v", rec)
		}
	}
	if len(rec) != 2 {
		t.Fatalf("drained records = %v", rec)
	}
	// A full drain over an interrupt entry alone returns nothing.
	if _, _, err := Interrupt(dir, time.Now()); err != nil {
		t.Fatal(err)
	}
	if got, err := Drain(dir); err != nil || len(got) != 0 {
		t.Fatalf("Drain of an interrupt entry alone = %+v, %v", got, err)
	}
	if names, _ := Names(dir); len(names) != 0 {
		t.Fatalf("inbox after the full drain = %v", names)
	}
	if got, err := DrainInterrupt(Dir(t.TempDir())); err != nil || got != nil {
		t.Fatalf("DrainInterrupt of a missing inbox = %+v, %v", got, err)
	}
}
