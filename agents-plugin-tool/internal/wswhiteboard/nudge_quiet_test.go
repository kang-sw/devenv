package wswhiteboard

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestQuietHourlyNoticeTracksBoardChangesNotSystemEntries(t *testing.T) {
	s := &State{}
	noticeTopic(t, s, "Wait", StateWaiting, 0, stateNow)
	dirs := map[string]string{"Wait": "t1"}
	p := s.planNotice("lead", dirs, stateNow.Add(time.Hour))
	noticeReasons(t, p, "hourly")
	noticeStamp(t, s, p, stateNow.Add(time.Hour))

	// A receipt and unrelated daemon-generated entries must not unmute it.
	s.appendEntry("Wait", Entry{Kind: EntrySystem, Text: "reconciliation"}, stateNow.Add(2*time.Hour))
	noticeReasons(t, s.planNotice("lead", dirs, stateNow.Add(4*time.Hour)))
	stateApply(t, s, `<wb:topic name="Wait"><wb:log>Lead progress</wb:log></wb:topic>`, stateNow.Add(4*time.Hour))
	p = s.planNotice("lead", dirs, stateNow.Add(5*time.Hour))
	noticeReasons(t, p, "hourly")
	noticeStamp(t, s, p, stateNow.Add(5*time.Hour))
	noticeReasons(t, s.planNotice("lead", dirs, stateNow.Add(9*time.Hour)))
}

func TestQuietOverdueNoticeNewDeadlineAndBoardChange(t *testing.T) {
	s := &State{}
	noticeTopic(t, s, "A", StateImplementing, 10, stateNow)
	noticeTopic(t, s, "B", StateImplementing, 50, stateNow)
	dirs := map[string]string{"A": "t1", "B": "t2"}
	p := s.planNotice("lead", dirs, stateNow.Add(10*time.Minute))
	noticeReasons(t, p, "overdue")
	noticeStamp(t, s, p, stateNow.Add(10*time.Minute))
	noticeReasons(t, s.planNotice("lead", dirs, stateNow.Add(45*time.Minute)))

	// B becomes overdue without an intervening board edit and must still send.
	p = s.planNotice("lead", dirs, stateNow.Add(50*time.Minute))
	noticeReasons(t, p, "overdue")
	if !strings.Contains(p.message, "- B;") {
		t.Fatalf("newly overdue topic missing: %s", p.message)
	}
	noticeStamp(t, s, p, stateNow.Add(50*time.Minute))
	// A lead log changes the board while both deadlines remain overdue.
	stateApply(t, s, `<wb:topic name="A"><wb:log>Still working</wb:log></wb:topic>`, stateNow.Add(51*time.Minute))
	p = s.planNotice("lead", dirs, stateNow.Add(52*time.Minute))
	noticeReasons(t, p, "overdue")
	if !strings.Contains(p.message, "- A;") || !strings.Contains(p.message, "- B;") {
		t.Fatalf("updated board omitted overdue topics: %s", p.message)
	}
}

func TestQuietNoticeReceiptSurvivesFolderReopen(t *testing.T) {
	f := noticeFolder(t)
	var messages []string
	notify := func(at time.Time) {
		t.Helper()
		_, err := f.Notify("lead", at, func() bool { return true }, func(message string) error {
			messages = append(messages, message)
			return nil
		}, func() {})
		if err != nil {
			t.Fatal(err)
		}
	}
	notify(stateNow.Add(time.Hour))
	if len(messages) != 1 {
		t.Fatalf("initial messages: %v", messages)
	}
	// A fresh FolderStore forces the receipt to be loaded from the store.
	f = FolderStore{Dir: f.Dir}
	notify(stateNow.Add(8 * time.Hour))
	if len(messages) != 1 {
		t.Fatalf("receipt did not survive reopen: %v", messages)
	}
	if err := f.Update(nil, func(s *State) error {
		stateApply(t, s, `<wb:topic name="Work"><wb:log>Progress after restart</wb:log></wb:topic>`, stateNow.Add(8*time.Hour))
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	notify(stateNow.Add(9 * time.Hour))
	if len(messages) != 2 {
		t.Fatalf("board update did not re-enable notice: %v", messages)
	}
}

func TestQuietStaleNudgeSurvivesReloadAndIgnoresSystemEvents(t *testing.T) {
	s := &State{}
	applyAt(t, s, bodies("A", "B", "C", "D", "E"), testNow)
	if out := applyAt(t, s, "", testNow.Add(time.Hour)); nudgeOf(out) == "" {
		t.Fatal("first stale nudge missing")
	}
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var reopened State
	if err := json.Unmarshal(data, &reopened); err != nil {
		t.Fatal(err)
	}
	reopened.appendEntry("A", Entry{Kind: EntrySystem, Text: "reconciliation"}, testNow.Add(2*time.Hour))
	if out := applyAt(t, &reopened, "", testNow.Add(3*time.Hour)); nudgeOf(out) != "" {
		t.Fatalf("system event or reload unmuted stale notice: %s", out.Info)
	}
	out := applyAt(t, &reopened, `<wb:topic name="A"><wb:log>Lead progress</wb:log></wb:topic>`, testNow.Add(4*time.Hour))
	if nudgeOf(out) == "" {
		t.Fatalf("board change did not re-enable stale notice: %s", out.Info)
	}
	if out := applyAt(t, &reopened, "", testNow.Add(6*time.Hour)); nudgeOf(out) != "" {
		t.Fatalf("stale notice repeated without another change: %s", out.Info)
	}
}
