package mcp

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsinbox"
	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

func inputEntries(t *testing.T, f *whiteboardFixture) []wswhiteboard.Entry {
	t.Helper()
	rec, err := f.s.sessions.readWhiteboard(f.key)
	if err != nil {
		t.Fatal(err)
	}
	var all []wswhiteboard.Entry
	for _, doc := range rec.Whiteboard.Topics {
		all = append(all, doc.Entries...)
	}
	return all
}
func lastInput(t *testing.T, f *whiteboardFixture) wswhiteboard.Entry {
	all := inputEntries(t, f)
	return all[len(all)-1]
}
func stampList(t *testing.T, e wswhiteboard.Entry) []boardInputStamp {
	t.Helper()
	var inputs []boardInputStamp
	if err := json.Unmarshal(e.Inputs, &inputs); err != nil {
		t.Fatal(err)
	}
	return inputs
}

func TestWhiteboardInputsActivationRestartAndEventless(t *testing.T) {
	f := newWhiteboardFixture(t)
	dir, err := f.s.sessionInboxDir(f.key)
	if err != nil {
		t.Fatal(err)
	}
	send := func(text string) string {
		t.Helper()
		name, _, err := wsinbox.Send(dir, wsinbox.Entry{Parts: []string{text}}, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		return (wsinbox.Record{Name: name}).ID()
	}
	drain := func() {
		t.Helper()
		if _, err := wsinbox.Drain(dir); err != nil {
			t.Fatal(err)
		}
	}
	queued := send("queued before activation")
	old := send("already delivered")
	names, _ := wsinbox.Names(dir)
	if _, err := wsinbox.Steer(dir, names[len(names)-1]); err != nil {
		t.Fatal(err)
	}
	if _, err := wsinbox.DrainSteer(dir); err != nil {
		t.Fatal(err)
	}
	f.update(t, `<wb:topic name="t"><wb:log>activate</wb:log></wb:topic>`)
	all := inputEntries(t, f)
	if string(all[0].Inputs) != "[]" || all[0].InputsFloor <= old || all[0].InputsFloor <= queued {
		t.Fatalf("activation stamp %+v", all[0])
	}
	floor := all[0].InputsFloor
	f.s = NewServer(f.root, "restart")
	drain()
	f.update(t, `<wb:topic name="t"><wb:log>old queued record is below floor</wb:log></wb:topic>`)
	if lastInput(t, f).Inputs != nil {
		t.Fatal("below-floor message credited")
	}
	fresh := send("visible response later")
	drain()
	before := len(inputEntries(t, f))
	f.update(t, "")
	if len(inputEntries(t, f)) != before {
		t.Fatal("eventless batch invented an entry")
	}
	f.update(t, `<wb:topic name="t"><wb:log>response</wb:log></wb:topic>`)
	got := stampList(t, lastInput(t, f))
	if len(got) != 1 || got[0].InboxID != fresh || got[0].Excerpt != "visible response later" {
		t.Fatal("eventless attribution", got)
	}
	all = inputEntries(t, f)
	if all[0].InputsFloor != floor {
		t.Fatal("floor lost on restart")
	}
	f.update(t, `<wb:topic name="t"><wb:log>same iteration</wb:log></wb:topic>`)
	if lastInput(t, f).Inputs != nil {
		t.Fatal("delivery double credited")
	}
}

func TestWhiteboardInputsSteeringSetAndNewestAnswerChip(t *testing.T) {
	f := newWhiteboardFixture(t)
	dir, _ := f.s.sessionInboxDir(f.key)
	f.update(t, `<wb:topic name="t"><wb:add class="policy"><wb:title>pick</wb:title><wb:option>A</wb:option><wb:option>B</wb:option></wb:add></wb:topic>`)
	send := func(text string, at int64, answer string) string {
		t.Helper()
		e := wsinbox.Entry{Parts: []string{text}}
		if answer != "" {
			e.Chips = []wsinbox.Chip{{ID: "1", Answer: answer}}
		}
		name, _, err := wsinbox.Send(dir, e, time.UnixMilli(at))
		if err != nil {
			t.Fatal(err)
		}
		return name
	}
	follow := send("follow up", 1000, "")
	steer := send("steering", 2000, "")
	steer, err := wsinbox.Steer(dir, steer)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := wsinbox.DrainSteer(dir); err != nil {
		t.Fatal(err)
	}
	f.update(t, `<wb:topic name="t"><wb:log>steered</wb:log></wb:topic>`)
	if got := stampList(t, lastInput(t, f)); len(got) != 1 || got[0].InboxID != (wsinbox.Record{Name: steer}).ID() {
		t.Fatal(got)
	}
	if _, err := wsinbox.Drain(dir); err != nil {
		t.Fatal(err)
	}
	f.update(t, `<wb:topic name="t"><wb:log>followed</wb:log></wb:topic>`)
	if got := stampList(t, lastInput(t, f)); len(got) != 1 || got[0].InboxID != (wsinbox.Record{Name: follow}).ID() {
		t.Fatal("high water skipped follow-up", got)
	}
	send("older answer", 3000, "a")
	send("newer answer", 4000, "b")
	if _, err := wsinbox.Drain(dir); err != nil {
		t.Fatal(err)
	}
	f.update(t, `<wb:topic name="t"><wb:log>applied</wb:log></wb:topic><wb:answer id="1" value="b">owner words</wb:answer>`)
	all := inputEntries(t, f)
	last := all[len(all)-1]
	prev := all[len(all)-2]
	if last.AnswerSentAt != 4000 || prev.Seq == 0 || prev.Seq != last.Seq || len(stampList(t, prev)) != 2 {
		t.Fatalf("batch/answer stamps %+v %+v", prev, last)
	}
	f.update(t, `<wb:patch id="1" status="open"/><wb:answer id="1" value="a">manual answer</wb:answer>`)
	if lastInput(t, f).AnswerSentAt != 0 {
		t.Fatal("invented unmatched owner time")
	}
}

func TestWhiteboardInputsRetainedRecordsOnly(t *testing.T) {
	f := newWhiteboardFixture(t)
	dir, _ := f.s.sessionInboxDir(f.key)
	f.update(t, `<wb:topic name="t"><wb:log>activate</wb:log></wb:topic>`)
	for i := 0; i < wsinbox.DrainedCap+1; i++ {
		if _, _, err := wsinbox.Send(dir, wsinbox.Entry{Parts: []string{fmt.Sprint(i)}}, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := wsinbox.Drain(dir); err != nil {
		t.Fatal(err)
	}
	f.update(t, `<wb:topic name="t"><wb:log>retained response</wb:log></wb:topic>`)
	inputs := stampList(t, lastInput(t, f))
	if len(inputs) != wsinbox.DrainedCap || inputs[0].Excerpt != "1" {
		t.Fatal("aged-out record credited", inputs)
	}
	nudge := "Dashboard answer check: the owner's answers to (01) reached you, but those board items are still open. Record each answer you acted on with wb:answer (the owner's words as its body); if an item should stay open, say why. This check runs once per answer batch."
	if _, _, err := wsinbox.Send(dir, wsinbox.Entry{Parts: []string{nudge}}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := wsinbox.Drain(dir); err != nil {
		t.Fatal(err)
	}
	f.update(t, `<wb:topic name="t"><wb:log>machine-only response</wb:log></wb:topic>`)
	if lastInput(t, f).Inputs != nil {
		t.Fatal("answer-check nudge credited as owner input")
	}
}
