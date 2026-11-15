package wswhiteboard

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
)

func waitingBoard(t *testing.T) *State {
	t.Helper()
	s := &State{}
	stateApply(t, s, `<wb:topic name="blocker" label="Blocker"><wb:log>Work</wb:log></wb:topic><wb:topic name="Work"><wb:log>Wait</wb:log></wb:topic>`, stateNow)
	return s
}

func TestWaitingOnDeclarationsAndAtomicRefusal(t *testing.T) {
	s := waitingBoard(t)
	declare := func(attrs string) { stateApply(t, s, stateCommand("waiting", attrs), stateNow) }
	declare(`on="topic:Blocker ticket:261115-feat-example owner exec:agent_3 topic:blocker"`)
	w := s.WaitingTopics(nil)["Work"]
	if len(w.Targets) != 4 || w.Targets[0].Token != "topic:blocker" || w.Ready {
		t.Fatalf("normalized: %+v", w)
	}
	seq := s.EntrySeq
	declare(`on="topic:blocker ticket:261115-feat-example owner exec:agent_3"`)
	if s.EntrySeq != seq {
		t.Fatal("same normalized declaration appended")
	}
	epoch, _ := s.hourlyEpoch()
	stateEntry := s.TopicStates()["Work"].State.Entry
	declare(`on="owner"`)
	if current, _ := s.hourlyEpoch(); current != epoch || s.TopicStates()["Work"].State.Entry != stateEntry {
		t.Fatal("dependency-only entry changed lifecycle or hourly epoch")
	}
	e := s.topicDoc("Work").Entries[len(s.topicDoc("Work").Entries)-1]
	if len(e.WaitingOn) == 0 || len(e.TopicState) != 0 || !strings.Contains(e.Text, "waiting on changed") {
		t.Fatalf("same-state payload: %+v", e)
	}
	if s.WaitingTopics(nil)["Work"].Declaration == w.Declaration {
		t.Fatal("replacement kept declaration identity")
	}
	declare("")
	if w = s.WaitingTopics(nil)["Work"]; len(w.Targets) != 0 || w.Ready {
		t.Fatal("omitted on did not clear", w)
	}
	seq = s.EntrySeq
	declare(`on=""`)
	if s.EntrySeq != seq {
		t.Fatal("empty repeat appended")
	}
	for _, bad := range []string{`value="discussing" on="owner"`, `value="waiting" on="topic:missing"`, `value="waiting" on="topic:Work"`, `value="waiting" on="ticket:261115-feat"`, `value="waiting" on="ticket:../bad"`, `value="waiting" on="exec:bad/label"`, `value="waiting" on="exec:` + strings.Repeat("a", 33) + `"`, `value="waiting" on="mystery"`, `value="waiting" on="exec:1 exec:2 exec:3 exec:4 exec:5 exec:6 exec:7 exec:8 exec:9"`} {
		before := s.clone()
		_, p := Apply(s, `<wb:topic name="Work"><wb:log>must roll back</wb:log><wb:state `+bad+`/></wb:topic>`, Env{Now: stateNow})
		if len(p) == 0 || !reflect.DeepEqual(before, s) {
			t.Fatalf("not atomically refused %s: %v", bad, p)
		}
	}
	_, badTime := Apply(s, stateCommand("waiting", `on="owner"`), Env{Now: time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)})
	if len(badTime) == 0 {
		t.Fatal("dependency change accepted invalid server timestamp")
	}
	long := strings.Repeat("a", 300)
	stateApply(t, s, `<wb:topic name="`+long+`"><wb:log>long legacy key</wb:log></wb:topic>`, stateNow)
	declare(`on="topic:` + long + `"`)
	declare(`on="topic:blocker"`)
	stateApply(t, s, stateCommand("discussing", ""), stateNow)
	if _, ok := s.WaitingTopics(nil)["Work"]; ok {
		t.Fatal("leaving waiting kept targets")
	}
	declare("")
	if len(s.WaitingTopics(nil)["Work"].Targets) != 0 {
		t.Fatal("re-entry restored stale targets")
	}
	declare(`on="owner"`)
	stateApply(t, s, `<wb:topic name="Work"><wb:close result="done"/><wb:log>reopen</wb:log><wb:state value="waiting"/></wb:topic>`, stateNow)
	if len(s.WaitingTopics(nil)["Work"].Targets) != 0 {
		t.Fatal("close/reopen restored targets")
	}
}

func TestWaitingOnResolutionAndDerivedOwner(t *testing.T) {
	s := waitingBoard(t)
	stateApply(t, s, `<wb:topic name="blocker"><wb:close result="done"/></wb:topic>`, stateNow)
	stateApply(t, s, stateCommand("waiting", `on="topic:blocker ticket:261115-feat-done ticket:261115-bug-dropped owner"`), stateNow)
	tickets := fakeTickets{"261115-feat-done": TicketDone, "261115-bug-dropped": TicketDropped}
	w := s.WaitingTopics(tickets)["Work"]
	if !w.Ready || len(w.Targets) != 4 || w.Targets[2].Status != TicketDropped {
		t.Fatalf("resolved: %+v", w)
	}
	stateApply(t, s, `<wb:topic name="Work"><wb:add class="check"><wb:title>Owner check</wb:title></wb:add><wb:add class="technical"><wb:title>Not an ask</wb:title></wb:add></wb:topic>`, stateNow)
	w = s.WaitingTopics(tickets)["Work"]
	if w.Ready || len(w.Targets) != 4 || w.Targets[3].Resolved || w.Targets[3].Derived {
		t.Fatalf("explicit/derived owner duplicated: %+v", w)
	}
	stateApply(t, s, stateCommand("waiting", `on="topic:blocker"`), stateNow)
	w = s.WaitingTopics(nil)["Work"]
	if w.Ready || len(w.Targets) != 2 || !w.Targets[1].Derived {
		t.Fatal("derived owner absent", w)
	}
	stateApply(t, s, `<wb:answer id="1" value="ok">approved</wb:answer>`, stateNow)
	if !s.WaitingTopics(nil)["Work"].Ready {
		t.Fatal("technical item blocked readiness")
	}
	stateApply(t, s, `<wb:topic name="blocker"><wb:log>reopen</wb:log></wb:topic>`, stateNow)
	if s.WaitingTopics(nil)["Work"].Ready {
		t.Fatal("reopen did not reblock")
	}
	stateApply(t, s, stateCommand("waiting", `on="ticket:261115-feat-missing exec:3"`), stateNow)
	w = s.WaitingTopics(nil)["Work"]
	if w.Ready || w.Targets[0].Status != TicketUnknown || w.Targets[1].Resolved {
		t.Fatal("unknown/exec auto-resolved", w)
	}
	stateApply(t, s, stateCommand("waiting", ""), stateNow)
	if s.WaitingTopics(nil)["Work"].Ready {
		t.Fatal("derived-only ready")
	}
	// Cycles are declarative, not an apply-time graph constraint.
	stateApply(t, s, `<wb:topic name="blocker"><wb:state value="waiting" on="topic:Work"/></wb:topic>`, stateNow)
	stateApply(t, s, stateCommand("waiting", `on="topic:blocker"`), stateNow)
	if s.WaitingTopics(nil)["Work"].Ready {
		t.Fatal("cycle resolved")
	}
}

func TestWaitingOnRawCloneAndProvenance(t *testing.T) {
	s := waitingBoard(t)
	stateApply(t, s, stateCommand("waiting", `on="owner"`), stateNow)
	e := &s.topicDoc("Work").Entries[len(s.topicDoc("Work").Entries)-1]
	e.WaitingOn = bytes.Replace(e.WaitingOn, []byte(`"v":1`), []byte(`"future": { "opaque": [1, 2] }, "v":1`), 1)
	e.UnblockedReceipt = json.RawMessage(`{"v":1,"reply_id":"lead","topic":"t2","declaration":"e3"}`)
	clone := s.clone()
	ce := &clone.topicDoc("Work").Entries[len(clone.topicDoc("Work").Entries)-1]
	if !bytes.Equal(e.WaitingOn, ce.WaitingOn) || !bytes.Equal(e.UnblockedReceipt, ce.UnblockedReceipt) {
		t.Fatal("raw clone changed bytes")
	}
	ce.WaitingOn[0] = '['
	ce.UnblockedReceipt[0] = '['
	if e.WaitingOn[0] != '{' || e.UnblockedReceipt[0] != '{' {
		t.Fatal("raw clone aliases")
	}
	data, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Entry
	var compact bytes.Buffer
	if err = json.Compact(&compact, e.WaitingOn); err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &decoded); err != nil || !bytes.Equal(decoded.WaitingOn, compact.Bytes()) {
		t.Fatal("codec", err)
	}
	for _, kind := range []string{EntryAgent, EntryPanel, EntrySystem} {
		copy := s.clone()
		stateApply(t, copy, stateCommand("waiting", `on="exec:3"`), stateNow)
		copy.appendEntry("Work", Entry{Kind: kind, Imported: kind == EntrySystem, Text: "authored metadata", WaitingOn: waitingPayload([]string{"owner"})}, stateNow)
		if copy.WaitingTopics(nil)["Work"].Ready {
			t.Fatal("authored/imported payload changed dependency authority")
		}
	}
	for _, kind := range []string{EntryAgent, EntryPanel, EntrySystem} {
		copy := s.clone()
		last := &copy.topicDoc("Work").Entries[len(copy.topicDoc("Work").Entries)-1]
		last.Kind = kind
		last.Imported = kind == EntrySystem
		if _, ok := copy.WaitingTopics(nil)["Work"]; ok {
			t.Fatal("untrusted producer selected state/targets")
		}
	}
}

func TestWaitingOnNudgesReceiptAndFaultOrdering(t *testing.T) {
	dir := t.TempDir()
	f := FolderStore{Dir: dir}
	update := func(cmd string) {
		t.Helper()
		if err := f.Update(nil, func(s *State) error { stateApply(t, s, cmd, stateNow); return nil }); err != nil {
			t.Fatal(err)
		}
	}
	update(`<wb:topic name="blocker"><wb:log>work</wb:log></wb:topic><wb:topic name="Work"><wb:log>wait</wb:log><wb:state value="waiting" on="topic:blocker"/></wb:topic>`)
	count := 0
	allow := true
	notify := func(reply string) {
		t.Helper()
		_, err := f.Notify(reply, stateNow, func() bool { return allow }, func(message string) error {
			if !strings.Contains(message, "Work") || !strings.Contains(message, "resolved targets:") {
				t.Fatal(message)
			}
			count++
			return nil
		}, func() {})
		if err != nil {
			t.Fatal(err)
		}
	}
	notify("lead")
	if count != 0 {
		t.Fatal("blocked mail")
	}
	update(`<wb:topic name="blocker"><wb:close result="done"/></wb:topic>`)
	allow = false
	notify("lead")
	if count != 0 {
		t.Fatal("ineligible mail")
	}
	allow = true
	notify("lead")
	notify("lead")
	if count != 1 {
		t.Fatal("duplicate or missing ready mail", count)
	}
	f = FolderStore{Dir: dir}
	notify("lead")
	if count != 1 {
		t.Fatal("restart lost receipt")
	}
	notify("other-lead")
	if count != 2 {
		t.Fatal("reply isolation")
	}
	update(`<wb:topic name="blocker"><wb:log>reopen</wb:log><wb:close result="done again"/></wb:topic>`)
	notify("lead")
	if count != 2 {
		t.Fatal("reblock/resolution renudged same declaration")
	}
	update(stateCommand("waiting", `on="topic:blocker owner"`))
	notify("lead")
	if count != 3 {
		t.Fatal("new declaration did not renudge")
	}
	update(stateCommand("waiting", ""))
	notify("lead")
	if count != 3 {
		t.Fatal("empty declaration nudged")
	}
	// Receipt payloads are independent from the hourly/overdue reason parser.
	for _, raw := range []string{`null`, `{"v":2,"reply_id":"lead","topic":"t2","declaration":"DECL"}`, `{"reply_id":"lead","topic":"t2","declaration":"DECL"}`, `{"v":1,"reply_id":"lead","topic":"bad","declaration":"DECL"}`, `{"v":1,"reply_id":"lead","topic":"t2","declaration":"bad"}`} {
		s := waitingBoard(t)
		stateApply(t, s, stateCommand("waiting", `on="owner"`), stateNow)
		raw = strings.ReplaceAll(raw, "DECL", s.WaitingTopics(nil)["Work"].Declaration)
		s.appendEntry("Work", Entry{Kind: EntrySystem, Text: "receipt", UnblockedReceipt: json.RawMessage(raw)}, stateNow)
		if len(s.planUnblocked("lead", map[string]string{"Work": "t2"}, nil)) != 1 {
			t.Fatal("malformed receipt suppressed mail", raw)
		}
	}
	update(stateCommand("waiting", `on="owner"`))
	f.Fault = func(stage string) error {
		if stage == "notice-enqueued" {
			return folderCrash
		}
		return nil
	}
	_, err := f.Notify("lead", stateNow, func() bool { return true }, func(string) error { count++; return nil }, func() {})
	if err != folderCrash {
		t.Fatal("missing fault", err)
	}
	f.Fault = nil
	notify("lead")
	if count != 5 {
		t.Fatal("enqueue-before-receipt ordering", count)
	}
}

func TestWaitingOnHistoricalWriter(t *testing.T) {
	binary := buildHistoricalWriter(t)
	for _, pending := range []bool{false, true} {
		t.Run(fmt.Sprint("pending=", pending), func(t *testing.T) {
			dir := t.TempDir()
			f := FolderStore{Dir: dir}
			if _, err := f.Open(nil); err != nil {
				t.Fatal(err)
			}
			if pending {
				f.Fault = func(stage string) error {
					if stage == "log-partial" {
						return folderCrash
					}
					return nil
				}
			}
			err := f.Update(nil, func(s *State) error {
				stateApply(t, s, `<wb:topic name="Work"><wb:log>wait</wb:log><wb:state value="waiting" on="owner"/></wb:topic>`, stateNow)
				p := s.planUnblocked("lead", map[string]string{"Work": "t1"}, nil)[0]
				data, _ := json.Marshal(p.receipt)
				s.appendEntry("Work", Entry{Kind: EntrySystem, Text: "Unblocked nudge sent", UnblockedReceipt: data}, stateNow)
				return nil
			})
			if pending && err != folderCrash || !pending && err != nil {
				t.Fatal(err)
			}
			var want []byte
			if pending {
				var intent folderIntent
				if _, err := folderDecode(dir+"/pending.json", &intent); err != nil {
					t.Fatal(err)
				}
				want = intent.Logs[0].Data
				historicalCall(t, binary, dir, "open", "", false)
			} else {
				want = folderRead(t, dir+"/t1/log.jsonl")
			}
			historicalCall(t, binary, dir, "update", `<wb:topic name="Work"><wb:log>old update</wb:log></wb:topic>`, false)
			historicalCall(t, binary, dir, "render", "", false)
			if !bytes.HasPrefix(folderRead(t, dir+"/t1/log.jsonl"), want) {
				t.Fatal("historical writer rewrote payload bytes")
			}
			got := folderOpen(t, dir, nil)
			if !got.WaitingTopics(nil)["Work"].Ready || len(got.planUnblocked("lead", map[string]string{"Work": "t1"}, nil)) != 0 {
				t.Fatal("historical writer lost targets/receipt")
			}
			historicalCall(t, binary, dir, "update", `<wb:topic name="Work"><wb:close result="done"/><wb:log>reopen</wb:log></wb:topic>`, false)
			if _, ok := folderOpen(t, dir, nil).WaitingTopics(nil)["Work"]; ok {
				t.Fatal("old close restored dependencies")
			}
		})
	}
}
