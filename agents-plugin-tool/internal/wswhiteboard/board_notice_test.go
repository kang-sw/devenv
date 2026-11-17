package wswhiteboard

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gofrs/flock"
	"github.com/kang-sw/devenv/internal/wsmailbox"
)

func noticeTopic(t *testing.T, s *State, topic, value string, eta int, now time.Time) {
	t.Helper()
	attrs := ""
	if value == StateImplementing {
		attrs = fmt.Sprintf(` owner="%s executor" eta_minutes="%d"`, topic, eta)
	}
	command := fmt.Sprintf(`<wb:topic name="%s"><wb:log>%s summary</wb:log>`, topic, topic)
	if value != "" {
		command += `<wb:state value="` + value + `"` + attrs + `/>`
	}
	stateApply(t, s, command+`</wb:topic>`, now)
}

func noticeStamp(t *testing.T, s *State, p noticePlan, now time.Time) {
	t.Helper()
	if p.message == "" {
		t.Fatal("cannot stamp an absent notification")
	}
	data, err := json.Marshal(p.receipt)
	if err != nil {
		t.Fatal(err)
	}
	s.appendEntry(p.topic, Entry{Kind: EntrySystem, Text: "Lead nudge sent", BoardNotice: data}, now)
}

func noticeReasons(t *testing.T, p noticePlan, reasons ...string) {
	t.Helper()
	if !reflect.DeepEqual(p.receipt.Reasons, reasons) {
		t.Fatalf("reasons = %v, want %v; %s", p.receipt.Reasons, reasons, p.message)
	}
	if len(reasons) > 0 && p.message == "" {
		t.Fatal("due receipt without message")
	}
	if len(reasons) == 0 && p.message != "" {
		t.Fatal("message without a due reason")
	}
}

func TestBoardNoticeResearchingBodyWithoutTriggerOrOverdue(t *testing.T) {
	s := stateBoard(t)
	stateApply(t, s, stateCommand(StateResearching, `owner="investigator" eta_minutes="1"`), stateNow)
	dirs := map[string]string{"Work": "t1"}
	p := s.planNotice("lead", dirs, stateNow.Add(2*time.Hour))
	noticeReasons(t, p)
	if !p.next.IsZero() || len(p.receipt.Covered) != 0 || eligibleState(s.TopicStates()["Work"]) {
		t.Fatal("informational research activated a clock")
	}
	if epoch, _ := s.hourlyEpoch(); epoch != "" {
		t.Fatal("research activated hourly epoch")
	}
	noticeTopic(t, s, "Wait", StateWaiting, 0, stateNow.Add(2*time.Hour))
	dirs["Wait"] = "t2"
	noticeReasons(t, s.planNotice("lead", dirs, stateNow.Add(179*time.Minute)))
	p = s.planNotice("lead", dirs, stateNow.Add(3*time.Hour))
	noticeReasons(t, p, "hourly")
	if !strings.Contains(p.message, "- Work: researching\n") || !strings.Contains(p.message, "- Wait: waiting\n") || !strings.Contains(p.message, "Overdue topics:\n- none\n") || len(p.receipt.Covered) != 0 {
		t.Fatalf("hourly research body/coverage: %+v", p)
	}
	stateApply(t, s, `<wb:topic name="Wait"><wb:state value="researching" eta_minutes="1"/></wb:topic>`, stateNow.Add(3*time.Hour))
	p = s.planNotice("lead", dirs, stateNow.Add(4*time.Hour))
	noticeReasons(t, p)
	if epoch, _ := s.hourlyEpoch(); epoch != "" {
		t.Fatal("research retained waiting hourly epoch")
	}
	stateApply(t, s, stateCommand(StateImplementing, `owner="executor" eta_minutes="2"`), stateNow.Add(4*time.Hour))
	noticeReasons(t, s.planNotice("lead", dirs, stateNow.Add(4*time.Hour+time.Minute)))
	p = s.planNotice("lead", dirs, stateNow.Add(4*time.Hour+2*time.Minute))
	noticeReasons(t, p, "overdue")
	if len(p.receipt.Covered) != 1 || p.receipt.Covered[0].TopicDir != "t1" || strings.Contains(p.message, "investigator") {
		t.Fatal("research metadata leaked into implementation overdue notice")
	}
}

func TestBoardNoticePerTopicFullListAndRevision(t *testing.T) {
	s := &State{}
	noticeTopic(t, s, "A", StateImplementing, 10, stateNow)
	noticeTopic(t, s, "B", StateImplementing, 20, stateNow)
	dirs := map[string]string{"A": "t2", "B": "t1"}
	p := s.planNotice("lead", dirs, stateNow.Add(10*time.Minute))
	noticeReasons(t, p, "overdue")
	if len(p.receipt.Covered) != 1 || p.receipt.Covered[0].TopicDir != "t2" || strings.Contains(p.message, "- B;") {
		t.Fatalf("non-overdue topic stamped: %+v", p)
	}
	noticeStamp(t, s, p, stateNow.Add(10*time.Minute))
	p = s.planNotice("lead", dirs, stateNow.Add(20*time.Minute))
	noticeReasons(t, p, "overdue")
	if len(p.receipt.Covered) != 2 || p.receipt.Covered[0].TopicDir != "t1" || p.topic != "B" || !strings.Contains(p.message, "- A; owner A executor; initial/current ETA 10/10 minutes; elapsed 1200000 ms; extensions 0") || !strings.Contains(p.message, "- B;") {
		t.Fatalf("new overdue did not send/stamp stable full list: %+v", p)
	}
	noticeStamp(t, s, p, stateNow.Add(20*time.Minute))
	p = s.planNotice("lead", dirs, stateNow.Add(40*time.Minute))
	noticeReasons(t, p)
	if !p.next.Equal(stateNow.Add(time.Hour)) {
		t.Fatalf("covered overdue topics must not schedule repeats: %s", p.next)
	}
	p = s.planNotice("lead", dirs, stateNow.Add(50*time.Minute))
	noticeReasons(t, p)
	// A future deadline waits; the board change also unmutes still-overdue B.
	stateApply(t, s, `<wb:topic name="A"><wb:eta minutes="60" reason="scope grew"/></wb:topic>`, stateNow.Add(51*time.Minute))
	noticeReasons(t, s.planNotice("lead", dirs, stateNow.Add(51*time.Minute)), "overdue")
	stateApply(t, s, `<wb:topic name="B"><wb:eta minutes="30" reason="extra work"/></wb:topic>`, stateNow.Add(52*time.Minute))
	p = s.planNotice("lead", dirs, stateNow.Add(52*time.Minute))
	noticeReasons(t, p, "overdue")
	if len(p.receipt.Covered) != 1 || p.receipt.Covered[0].ETAEntry != s.TopicStates()["B"].State.Entry || !strings.Contains(p.message, "extension to 30 minutes: extra work") {
		t.Fatalf("revision not covered: %+v", p)
	}
	noticeStamp(t, s, p, stateNow.Add(52*time.Minute))
	p = s.planNotice("lead", dirs, stateNow.Add(60*time.Minute))
	noticeReasons(t, p, "overdue", "hourly")
	if len(p.receipt.Covered) != 2 || !strings.Contains(p.message, "extension to 60 minutes: scope grew") {
		t.Fatal("new revision omitted full list/reason")
	}
	// A new run is a fresh debounce identity, even with the same topic folder.
	stateApply(t, s, `<wb:topic name="A"><wb:state value="waiting"/><wb:state value="implementing" owner="replacement" eta_minutes="1"/></wb:topic>`, stateNow.Add(60*time.Minute))
	p = s.planNotice("lead", dirs, stateNow.Add(61*time.Minute))
	noticeReasons(t, p, "overdue", "hourly")
	if p.receipt.Covered[1].RunID != s.TopicStates()["A"].State.Run.ID {
		t.Fatal("new run inherited old coverage")
	}
}

func TestBoardNoticeAllReasonsWithoutTruncation(t *testing.T) {
	s := etaBoard(t)
	stateApply(t, s, etaCommand("60", "dependency expanded"), stateNow.Add(20*time.Minute))
	stateApply(t, s, etaCommand("90", "integration validation"), stateNow.Add(40*time.Minute))
	dirs := map[string]string{"Work": "t1"}
	for i := 2; i <= 12; i++ {
		name := fmt.Sprintf("topic-%02d", i)
		noticeTopic(t, s, name, StateImplementing, 1, stateNow)
		dirs[name] = fmt.Sprintf("t%d", i)
	}
	p := s.planNotice("lead", dirs, stateNow.Add(2*time.Hour))
	noticeReasons(t, p, "overdue", "hourly")
	if len(p.receipt.Covered) != 12 || strings.Count(p.message, "; owner ") != 12 {
		t.Fatalf("truncated full list: %s", p.message)
	}
	for _, text := range []string{"initial/current ETA 40/90 minutes; elapsed 7200000 ms; extensions 2", "extension to 60 minutes: dependency expanded", "extension to 90 minutes: integration validation"} {
		if !strings.Contains(p.message, text) {
			t.Fatalf("missing %q: %s", text, p.message)
		}
	}
}

func TestBoardNoticeHourlyIndependentAndCombinedCatchup(t *testing.T) {
	// New topics default to discussing, so an absent state exists only on a
	// topic written before that default: seed one as a literal legacy topic.
	s := &State{EntrySeq: 1, Topics: []TopicDoc{{Name: "absent", Entries: []Entry{{ID: "e1", Kind: EntryAgent, Text: "absent summary", At: stateNow.Format(time.RFC3339Nano)}}}}}
	noticeTopic(t, s, "wait", StateWaiting, 0, stateNow)
	noticeTopic(t, s, "discussion", StateDiscussing, 0, stateNow)
	dirs := map[string]string{"wait": "t1", "absent": "t2", "discussion": "t3", "run": "t4"}
	noticeReasons(t, s.planNotice("lead", dirs, stateNow.Add(time.Hour-time.Nanosecond)))
	p := s.planNotice("lead", dirs, stateNow.Add(time.Hour))
	noticeReasons(t, p, "hourly")
	for _, text := range []string{"wait: waiting", "absent: absent", "discussion: discussing", "Overdue topics:\n- none"} {
		if !strings.Contains(p.message, text) {
			t.Fatalf("hourly omitted %q", text)
		}
	}
	noticeStamp(t, s, p, stateNow.Add(time.Hour))
	noticeTopic(t, s, "run", StateImplementing, 1, stateNow.Add(65*time.Minute))
	p = s.planNotice("lead", dirs, stateNow.Add(66*time.Minute))
	noticeReasons(t, p, "overdue")
	noticeStamp(t, s, p, stateNow.Add(66*time.Minute))
	p = s.planNotice("lead", dirs, stateNow.Add(96*time.Minute))
	noticeReasons(t, p)
	p = s.planNotice("lead", dirs, stateNow.Add(2*time.Hour))
	noticeReasons(t, p, "hourly") // overdue-only sends did not move the hourly anchor
	if !strings.Contains(p.message, "- run;") {
		t.Fatal("hourly lacks full overdue section")
	}
	noticeStamp(t, s, p, stateNow.Add(2*time.Hour))
	// Hourly-only includes and covers overdue topics for this board revision.
	p = s.planNotice("lead", dirs, stateNow.Add(126*time.Minute))
	noticeReasons(t, p)
	if !p.next.IsZero() {
		t.Fatalf("covered board retained a repeat timer: %s", p.next)
	}
	// Restart/resume folds committed receipts without replaying missed intervals.
	restarted := s.clone()
	p = restarted.planNotice("lead", dirs, stateNow.Add(8*time.Hour))
	noticeReasons(t, p)
	if !p.next.IsZero() {
		t.Fatalf("restart scheduled a repeat on an unchanged board: %s", p.next)
	}
	stateApply(t, restarted, `<wb:topic name="wait"><wb:state value="discussing"/></wb:topic><wb:topic name="run"><wb:close result="done"/></wb:topic>`, stateNow.Add(9*time.Hour))
	p = restarted.planNotice("lead", dirs, stateNow.Add(24*time.Hour))
	noticeReasons(t, p)
	if !p.next.IsZero() {
		t.Fatal("periodic timer retained without waiting/implementing")
	}
}

func TestBoardNoticeHourlyNumericGlobalClosedHistoryEpoch(t *testing.T) {
	s := &State{}
	stateRaw(s, "Z-closed", "e2", `{"v":1,"op":"set","value":"waiting"}`)
	stateRaw(s, "Z-closed", "e8", `{"v":1,"op":"reset","from":"waiting"}`)
	stateRaw(s, "B-closed", "e9", `{"v":1,"op":"set","value":"waiting"}`)
	stateRaw(s, "A-open", "e10", `{"v":1,"op":"set","value":"waiting"}`)
	s.topicDoc("B-closed").Entries = append(s.topicDoc("B-closed").Entries, Entry{ID: "e11", Kind: EntrySystem, At: stateNow.Format(time.RFC3339Nano), Text: "Topic closed: historical writer"})
	s.ClosedTopics = []string{"Z-closed", "B-closed"}
	stateRaw(s, "recap", "e99", `{"v":1,"op":"set","value":"waiting"}`)
	s.topicDoc("recap").Entries[0].Imported = true
	// Topic slice order and every event's timestamp deliberately disagree with
	// neither authority nor one another: only numeric global IDs distinguish it.
	s.Topics[0], s.Topics[2] = s.Topics[2], s.Topics[0]
	id, at := s.hourlyEpoch()
	if id != "e9" || !at.Equal(stateNow) {
		t.Fatalf("closed-history/equal-time epoch: %s %s", id, at)
	}
	dirs := map[string]string{"Z-closed": "t1", "B-closed": "t2", "A-open": "t3", "recap": "t4"}
	p := s.planNotice("lead", dirs, stateNow.Add(time.Hour))
	noticeReasons(t, p, "hourly")
	if p.receipt.HourlyEpoch != "e9" {
		t.Fatalf("epoch: %+v", p.receipt)
	}
	noticeStamp(t, s, p, stateNow.Add(time.Hour))
	stateApply(t, s, `<wb:topic name="A-open"><wb:close result="done"/><wb:log>reopen</wb:log><wb:state value="waiting"/></wb:topic>`, stateNow.Add(2*time.Hour))
	id, at = s.hourlyEpoch()
	if id == "e9" || !at.Equal(stateNow.Add(2*time.Hour)) {
		t.Fatal("new eligibility epoch inherited old activation")
	}
	p = s.planNotice("lead", dirs, stateNow.Add(2*time.Hour))
	noticeReasons(t, p)
	if !p.next.Equal(stateNow.Add(3 * time.Hour)) {
		t.Fatalf("new epoch due: %s", p.next)
	}
}

func TestBoardNoticeReplyIsolationDiagnosticsAndFutureClocks(t *testing.T) {
	s := etaBoard(t)
	dirs := map[string]string{"Work": "t1"}
	p := s.planNotice("source", dirs, stateNow.Add(2*time.Hour))
	noticeStamp(t, s, p, stateNow.Add(2*time.Hour))
	noticeReasons(t, s.planNotice("source", dirs, stateNow.Add(2*time.Hour)))
	copy := s.clone() // quiescent source-copy seam: no source writes afterwards
	p = copy.planNotice("destination", dirs, stateNow.Add(2*time.Hour))
	noticeReasons(t, p, "overdue", "hourly")
	if copy.TopicStates()["Work"].State.Run.StartedAt != stateNow.Format(time.RFC3339Nano) {
		t.Fatal("copy lost deadline")
	}
	noticeStamp(t, copy, p, stateNow.Add(2*time.Hour))
	noticeReasons(t, copy.planNotice("destination", dirs, stateNow.Add(2*time.Hour)))
	for _, raw := range []string{`{"v":1,"op":"extend"}`, `{"v":2,"op":"set","value":"waiting"}`} {
		bad := etaBoard(t)
		stateRaw(bad, "Work", "e3", raw)
		p = bad.planNotice("lead", dirs, stateNow.Add(2*time.Hour))
		noticeReasons(t, p)
		if !p.next.IsZero() {
			t.Fatal("unknown state retained stale deadline/epoch")
		}
		noticeTopic(t, bad, "Wait", StateWaiting, 0, stateNow)
		p = bad.planNotice("lead", map[string]string{"Work": "t1", "Wait": "t2"}, stateNow.Add(2*time.Hour))
		noticeReasons(t, p, "hourly")
		if len(p.receipt.Covered) != 0 || !strings.Contains(p.message, "Work: unknown (") {
			t.Fatal("unknown state triggered timer or vanished from hourly list")
		}
	}
	// A future receipt still suppresses the unchanged board revision.
	p = s.planNotice("source", dirs, stateNow.Add(90*time.Minute))
	noticeReasons(t, p)
	if !p.next.IsZero() {
		t.Fatalf("future receipt retained a repeat timer: %s", p.next)
	}
	noticeTopic(t, s, "new", StateImplementing, 1, stateNow.Add(89*time.Minute))
	p = s.planNotice("source", map[string]string{"Work": "t1", "new": "t2"}, stateNow.Add(90*time.Minute))
	noticeReasons(t, p, "overdue")
	if !strings.Contains(p.message, "Clock diagnostic: future stored timestamp") {
		t.Fatal("future receipt not diagnosed on new trigger")
	}
	future := &State{}
	noticeTopic(t, future, "wait", StateWaiting, 0, stateNow.Add(-2*time.Hour))
	noticeTopic(t, future, "run", StateImplementing, 1, stateNow.Add(time.Hour))
	p = future.planNotice("lead", map[string]string{"wait": "t1", "run": "t2"}, stateNow)
	noticeReasons(t, p, "hourly")
	if len(p.receipt.Covered) != 0 || !strings.Contains(p.message, "future stored timestamp") || runElapsed(future.TopicStates()["run"].State.Run, stateNow) != 0 {
		t.Fatal("future start was overdue or negative elapsed/undiagnosed")
	}
}

func TestBoardNoticeRawCloneAndReceiptValidation(t *testing.T) {
	s := etaBoard(t)
	p := s.planNotice("lead", map[string]string{"Work": "t1"}, stateNow.Add(2*time.Hour))
	noticeStamp(t, s, p, stateNow.Add(2*time.Hour))
	e := &s.Topics[0].Entries[len(s.Topics[0].Entries)-1]
	e.BoardNotice = bytes.Replace(e.BoardNotice, []byte(`"v":1`), []byte(`"future": { "opaque": [ 1, 2 ] }, "v":1`), 1)
	copy := s.clone()
	if !bytes.Equal(e.BoardNotice, copy.Topics[0].Entries[len(copy.Topics[0].Entries)-1].BoardNotice) {
		t.Fatal("clone changed raw receipt bytes")
	}
	copy.Topics[0].Entries[len(copy.Topics[0].Entries)-1].BoardNotice[0] = '['
	if e.BoardNotice[0] != '{' {
		t.Fatal("receipt clone aliases authority")
	}
	encoded, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Entry
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if n, ok := parseBoardNotice(decoded, "lead"); !ok || n.HourlyEpoch != p.receipt.HourlyEpoch || !bytes.Contains(decoded.BoardNotice, []byte(`"opaque"`)) {
		t.Fatalf("receipt JSON round trip: %+v %v", n, ok)
	}
	for name, change := range map[string]func(*boardNotice){
		"version": func(n *boardNotice) { n.V = 2 }, "reply": func(n *boardNotice) { n.ReplyID = "other" },
		"empty-reasons": func(n *boardNotice) { n.Reasons = nil }, "unknown-reason": func(n *boardNotice) { n.Reasons = []string{"future"} },
		"duplicate-reason": func(n *boardNotice) { n.Reasons = []string{"overdue", "overdue"} },
		"epoch":            func(n *boardNotice) { n.HourlyEpoch = "e0" }, "timestamp": func(n *boardNotice) { n.SentAt = stateNow.Format(time.RFC3339Nano) },
		"folder": func(n *boardNotice) { n.Covered[0].TopicDir = "../t1" }, "run": func(n *boardNotice) { n.Covered[0].RunID = "e0" }, "revision": func(n *boardNotice) { n.Covered[0].ETAEntry = "e01" },
	} {
		t.Run(name, func(t *testing.T) {
			bad := s.clone()
			entry := &bad.Topics[0].Entries[len(bad.Topics[0].Entries)-1]
			n, ok := parseBoardNotice(*entry, "lead")
			if !ok {
				t.Fatal("fixture receipt invalid")
			}
			change(&n)
			entry.BoardNotice, err = json.Marshal(n)
			if err != nil {
				t.Fatal(err)
			}
			if _, ok := parseBoardNotice(*entry, "lead"); ok {
				t.Fatal("accepted invalid receipt")
			}
			noticeReasons(t, bad.planNotice("lead", map[string]string{"Work": "t1"}, stateNow.Add(2*time.Hour)), "overdue", "hourly")
		})
	}
	for _, raw := range []string{"null", "[]", "{}"} {
		bad := s.clone()
		bad.Topics[0].Entries[len(bad.Topics[0].Entries)-1].BoardNotice = json.RawMessage(raw)
		noticeReasons(t, bad.planNotice("lead", map[string]string{"Work": "t1"}, stateNow.Add(2*time.Hour)), "overdue", "hourly")
	}
	for _, provenance := range []string{"imported", "item", "agent"} {
		bad := s.clone()
		entry := &bad.Topics[0].Entries[len(bad.Topics[0].Entries)-1]
		switch provenance {
		case "imported":
			entry.Imported = true
		case "item":
			entry.ItemID = 1
		case "agent":
			entry.Kind = EntryAgent
		}
		noticeReasons(t, bad.planNotice("lead", map[string]string{"Work": "t1"}, stateNow.Add(2*time.Hour)), "overdue", "hourly")
	}
}

func noticeFolder(t *testing.T) FolderStore {
	t.Helper()
	f := FolderStore{Dir: filepath.Join(t.TempDir(), "board")}
	if err := f.Update(nil, func(s *State) error { noticeTopic(t, s, "Work", StateImplementing, 1, stateNow); return nil }); err != nil {
		t.Fatal(err)
	}
	return f
}

func noticeReceiptCount(s *State) int {
	n := 0
	for _, doc := range s.Topics {
		for _, e := range doc.Entries {
			if len(e.BoardNotice) > 0 {
				n++
			}
		}
	}
	return n
}

func TestBoardNotifyAbsentFailureEligibilityAndWakeOrdering(t *testing.T) {
	absent := FolderStore{Dir: filepath.Join(t.TempDir(), "absent")}
	unexpected := func() { t.Error("absent/ineligible board invoked delivery") }
	if next, err := absent.Notify("lead", stateNow, func() bool { unexpected(); return true }, func(string) error { unexpected(); return nil }, unexpected); err != nil || !next.IsZero() {
		t.Fatal(next, err)
	}
	if _, err := os.Stat(absent.Dir); !os.IsNotExist(err) {
		t.Fatal("Notify created absent folder", err)
	}
	f := noticeFolder(t)
	before := folderTree(t, f.Dir)
	if _, err := f.Notify("lead", stateNow.Add(time.Hour), func() bool { return false }, func(string) error { unexpected(); return nil }, unexpected); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, folderTree(t, f.Dir)) {
		t.Fatal("ineligible notify advanced receipt")
	}
	failed := errors.New("enqueue failed")
	if _, err := f.Notify("lead", stateNow.Add(time.Hour), func() bool { return true }, func(string) error { return failed }, unexpected); !errors.Is(err, failed) {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, folderTree(t, f.Dir)) {
		t.Fatal("enqueue failure wrote receipt")
	}
	var order []string
	var next time.Time
	var err error
	next, err = f.Notify("lead", stateNow.Add(time.Hour), func() bool { order = append(order, "eligible"); return true }, func(string) error {
		order = append(order, "enqueue")
		lock := flock.New(filepath.Join(f.Dir, ".lock"))
		acquired, e := lock.TryLock()
		if e != nil {
			t.Error(e)
		}
		if acquired {
			_ = lock.Unlock()
			t.Error("enqueue outside folder transaction")
		}
		return nil
	}, func() {
		order = append(order, "wake")
		if bytes.Contains(folderRead(t, filepath.Join(f.Dir, "t1", "log.jsonl")), []byte(`"board_notice"`)) {
			t.Error("receipt published before wake")
		}
	})
	if err != nil || !reflect.DeepEqual(order, []string{"eligible", "enqueue", "wake"}) || !next.IsZero() {
		t.Fatal(order, next, err)
	}
	s := folderOpen(t, f.Dir, nil)
	if noticeReceiptCount(s) != 1 {
		t.Fatal("missing receipt")
	}
	if latest, _ := s.LatestAgent("Work"); latest.Text != "Work summary" {
		t.Fatal("receipt replaced agent summary")
	}
}

func TestBoardNotifyCrashRetryAndIntentRecovery(t *testing.T) {
	for _, stage := range []string{"notice-enqueued", "intent-staged", "intent", "log-partial", "session-meta", "cleanup"} {
		t.Run(stage, func(t *testing.T) {
			f := noticeFolder(t)
			sends, wakes := 0, 0
			enqueue := func(string) error { sends++; return nil }
			wake := func() { wakes++ }
			f.Fault = func(at string) error {
				if at == stage {
					return folderCrash
				}
				return nil
			}
			if _, err := f.Notify("lead", stateNow.Add(2*time.Hour), func() bool { return true }, enqueue, wake); !errors.Is(err, folderCrash) {
				t.Fatalf("fault not reached: %v", err)
			}
			f.Fault = nil
			// Notify itself must recover uncertain intent before checking eligibility.
			if _, err := f.Notify("lead", stateNow.Add(2*time.Hour), func() bool { return true }, enqueue, wake); err != nil {
				t.Fatal(err)
			}
			want := 1
			if stage == "notice-enqueued" || stage == "intent-staged" {
				want = 2
			}
			if sends != want || wakes != want || noticeReceiptCount(folderOpen(t, f.Dir, nil)) != 1 {
				t.Fatalf("retry accounting: sends=%d wakes=%d want=%d", sends, wakes, want)
			}
			before := folderTree(t, f.Dir)
			if _, err := f.Notify("lead", stateNow.Add(2*time.Hour), func() bool { return true }, enqueue, wake); err != nil {
				t.Fatal(err)
			}
			if sends != want || !reflect.DeepEqual(before, folderTree(t, f.Dir)) {
				t.Fatal("recovery replayed receipt/delivery")
			}
		})
	}
}

func TestBoardNotifyNormalUpdateAndMailboxLockDirection(t *testing.T) {
	f := noticeFolder(t)
	mail := filepath.Join(t.TempDir(), "reply.json")
	enqueued := make(chan struct{})
	release := make(chan struct{})
	notified := make(chan error, 1)
	go func() {
		_, err := f.Notify("lead", stateNow.Add(time.Hour), func() bool { return true }, func(message string) error {
			if err := wsmailbox.PostSystem(mail, "lead", "", message, stateNow.Add(time.Hour)); err != nil {
				return err
			}
			close(enqueued)
			<-release
			return nil
		}, func() {
			// Drain/sweep's low-level reply-lock seam is available before receipt
			// commit, even though Notify still holds the board transaction lock.
			if err := wsmailbox.WithReplyLock(mail, func(store *wsmailbox.ReplyStore) error {
				if len(store.Queues["lead"]) != 1 {
					return fmt.Errorf("wake preceded enqueue")
				}
				delete(store.Queues, "lead")
				return nil
			}); err != nil {
				t.Error(err)
			}
		})
		notified <- err
	}()
	<-enqueued
	updated := make(chan error, 1)
	started := make(chan struct{})
	go func() {
		close(started)
		updated <- f.Update(nil, func(s *State) error {
			_, p := Apply(s, `<wb:topic name="Work"><wb:log>concurrent normal update</wb:log></wb:topic>`, Env{Now: stateNow.Add(time.Hour)})
			if len(p) > 0 {
				return fmt.Errorf("Apply: %v", p)
			}
			return nil
		})
	}()
	<-started
	close(release)
	if err := <-notified; err != nil {
		t.Fatal(err)
	}
	if err := <-updated; err != nil {
		t.Fatal(err)
	}
	s := folderOpen(t, f.Dir, nil)
	if noticeReceiptCount(s) != 1 {
		t.Fatal("normal update lost receipt")
	}
	if latest, _ := s.LatestAgent("Work"); latest.Text != "concurrent normal update" {
		t.Fatal("notification lost concurrent update")
	}
	store, err := wsmailbox.LoadReplyStore(mail)
	if err != nil || len(store.Queues["lead"]) != 0 {
		t.Fatal("wake drain failed", err)
	}
	// A normal update that wins first must invalidate stale timer authority.
	if err := f.Update(nil, func(s *State) error {
		_, p := Apply(s, `<wb:topic name="Work"><wb:close result="done"/></wb:topic>`, Env{Now: stateNow.Add(2 * time.Hour)})
		if len(p) > 0 {
			return fmt.Errorf("Apply: %v", p)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Notify("lead", stateNow.Add(3*time.Hour), func() bool { return true }, func(string) error { t.Error("closed state enqueued"); return nil }, func() { t.Error("closed state woke") }); err != nil {
		t.Fatal(err)
	}
}

func TestBoardNotifyMailboxTimeoutDoesNotCommitReceipt(t *testing.T) {
	f := noticeFolder(t)
	mail := filepath.Join(t.TempDir(), "reply.json")
	lock := flock.New(mail + ".lock")
	if err := lock.Lock(); err != nil {
		t.Fatal(err)
	}
	defer lock.Unlock()
	before := folderTree(t, f.Dir)
	_, err := f.Notify("lead", stateNow.Add(time.Hour), func() bool { return true }, func(message string) error {
		return wsmailbox.PostSystem(mail, "lead", "", message, stateNow.Add(time.Hour))
	}, func() { t.Error("failed enqueue woke") })
	if err == nil || !strings.Contains(err.Error(), "lock") {
		t.Fatal("mailbox contention did not fail", err)
	}
	if !reflect.DeepEqual(before, folderTree(t, f.Dir)) {
		t.Fatal("mailbox timeout committed watermark")
	}
}
