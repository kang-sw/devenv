package wsagent

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wslive"
)

func liveWakes(events []Event) []wslive.Event {
	var out []wslive.Event
	for _, ev := range events {
		if ev.Live != nil && ev.Live.Kind == wslive.KindWake {
			out = append(out, *ev.Live)
		}
	}
	return out
}

func TestClaudeRecordedTaskWakes(t *testing.T) {
	for _, tc := range []struct{ fixture, cause, text string }{
		{"claude-lead-background-task.jsonl", "background_exit", `Background command "Sleep 25 seconds then echo bgdone" completed (exit code 0)`},
		{"claude-lead-background-agent-states.jsonl", "subagent_report", "done"},
		{"claude-subagent-turn.jsonl", "subagent_report", "hi"},
		{"claude-lead-steer.jsonl", "", ""}, // a foreground shell is not a wake
	} {
		t.Run(tc.fixture, func(t *testing.T) {
			r := newLeadReplay(t)
			r.all(readLeadFixture(t, tc.fixture))
			wakes := liveWakes(r.events)
			if tc.cause == "" {
				if len(wakes) != 0 {
					t.Fatalf("foreground task wakes = %+v", wakes)
				}
				return
			}
			if len(wakes) != 1 || wakes[0].Cause != tc.cause || wakes[0].Text != tc.text || wakes[0].Source == "" || wakes[0].Caption == nil {
				t.Fatalf("wakes = %+v", wakes)
			}
			if got := ringTexts(r.ring, "wake"); !slices.Equal(got, []string{tc.text}) {
				t.Fatalf("ring wakes = %q", got)
			}
		})
	}
}

func TestClaudeTaskWakeLifecycle(t *testing.T) {
	for _, tc := range []struct{ kind, status, cause string }{
		{"local_agent", "running", "subagent_report"},
		{"local_agent", "completed", "subagent_report"},
		{"local_agent", "failed", "subagent_failed"},
		{"local_agent", "stopped", "subagent_failed"},
		{"local_bash", "failed", "background_exit"},
		{"monitor", "running", "monitor"},
	} {
		t.Run(tc.kind+"/"+tc.status, func(t *testing.T) {
			m := newClaudeMapper(leadSpec())
			start := `{"type":"system","subtype":"task_started","task_id":"t","task_type":"` + tc.kind + `","description":"watch build","is_backgrounded":true}`
			m.mapLine([]byte(start), nopReplies{})
			// A terminal state can precede the notification; its summary is
			// still emitted exactly once, not at the state change.
			updated := `{"type":"system","subtype":"task_updated","task_id":"t","patch":{"status":"` + tc.status + `"}}`
			if got := liveWakes(m.mapLine([]byte(updated), nopReplies{})); len(got) != 0 {
				t.Fatalf("task_updated wakes = %+v", got)
			}
			line := []byte(`{"type":"system","subtype":"task_notification","task_id":"t","status":"` + tc.status + `","summary":"first line\nfull result"}`)
			wakes := liveWakes(m.mapLine(line, nopReplies{}))
			if len(wakes) != 1 || wakes[0].Cause != tc.cause || wakes[0].Source != "watch build" || *wakes[0].Caption != "first line" || wakes[0].Text != "first line\nfull result" {
				t.Fatalf("wakes = %+v", wakes)
			}
			if claudeTaskEnd(tc.status) != "" && len(liveWakes(m.mapLine(line, nopReplies{}))) != 0 {
				t.Fatal("a repeated terminal notification must not wake twice")
			}
		})
	}
	// Completion without a result is still a divider, not an empty report.
	for _, summary := range []string{"", " \n "} {
		m := newClaudeMapper(leadSpec())
		m.rememberWakeTask(claudeLine{TaskID: "t", TaskType: claudeAgentTask}, true)
		wakes := liveWakes(m.taskWake(claudeLine{TaskID: "t", Status: "completed", Summary: summary}))
		if len(wakes) != 1 || wakes[0].Cause != "subagent_done" {
			t.Fatalf("completion-only wakes = %+v", wakes)
		}
	}
	// A notification with no observed task identity cannot be classified.
	m := newClaudeMapper(leadSpec())
	if got := m.mapLine([]byte(`{"type":"system","subtype":"task_notification","task_id":"unknown","status":"completed","summary":"done"}`), nopReplies{}); len(got) != 0 {
		t.Fatalf("unknown task = %+v", got)
	}
}

func TestClaudeBackgroundCountSplits(t *testing.T) {
	m := newClaudeMapper(leadSpec())
	for _, tc := range []struct {
		tasks string
		n, bg, mon int
		known bool
	}{
		{`[{"task_id":"b","task_type":"local_bash"},{"task_id":"a","task_type":"local_agent"},{"task_id":"m","task_type":"monitor"}]`, 3, 1, 1, true},
		{`[{"task_type":"local_agent"}]`, 1, 0, 0, true},
		{`[{"task_type":"local_monitor"}]`, 1, 0, 1, true},
		{`[{"task_type":"future"},{"task_type":"local_bash"}]`, 2, 0, 0, false},
		{`[{}]`, 1, 0, 0, false},
		{`["unreadable"]`, 1, 0, 0, false},
		{`[]`, 0, 0, 0, true}, // finish/abort: zero must be explicit
	} {
		evs := m.mapLine([]byte(`{"type":"system","subtype":"background_tasks_changed","tasks":`+tc.tasks+`}`), nopReplies{})
		if len(evs) != 1 || evs[0].BackgroundTasks == nil || *evs[0].BackgroundTasks != tc.n || m.background != tc.n {
			t.Fatalf("%s: counts = %+v, aggregate %d", tc.tasks, evs, m.background)
		}
		ev := evs[0]
		if tc.known {
			if ev.Background == nil || ev.Monitors == nil || *ev.Background != tc.bg || *ev.Monitors != tc.mon {
				t.Fatalf("%s: split = %+v", tc.tasks, ev)
			}
		} else if ev.Background != nil || ev.Monitors != nil {
			t.Fatalf("%s: unknown split must be nil: %+v", tc.tasks, ev)
		}
		if ev.Live != nil {
			t.Fatal("counts must not replace usage with a count-only status")
		}
	}
	for _, tasks := range []string{"null", "{}"} {
		if evs := m.mapLine([]byte(`{"type":"system","subtype":"background_tasks_changed","tasks":`+tasks+`}`), nopReplies{}); len(evs) != 0 {
			t.Fatalf("invalid list %s should leave counts alone: %+v", tasks, evs)
		}
	}
}

func TestPiHostedEventWidget(t *testing.T) {
	m := newPiMapper(StartSpec{Lead: &LeadSpec{}})
	var lines []string
	causes := []string{"subagent_report", "subagent_done", "subagent_failed", "background_exit", "monitor", "mail", "message", "scheduled"}
	for _, cause := range causes {
		lines = append(lines, `{"kind":"wake","cause":"`+cause+`","source":"worker","caption":"one\nline","text":"full\nbody"}`)
	}
	lines = append(lines,
		`{"kind":"status","background":2,"monitors":1,"cost":{"usd":999}}`,
		`{"kind":"reset"}`, `{"kind":"turn_start"}`, `{"kind":"reply","text":"not a reply"}`,
		`{"kind":"wake","cause":"future"}`, `{"kind":"wake","cause":"message","agentId":"child"}`,
		`{"kind":"status","background":-1,"monitors":0}`, `{"kind":"status","background":1}`,
		`{"kind":"status","background":1.5,"monitors":0}`, `not json`,
	)
	evs := m.mapLine(piWidgetLine(piEventsWidgetKey, lines...), nopReplies{})
	wakes := liveWakes(evs)
	if len(wakes) != len(causes) || len(evs) != len(causes)+1 {
		t.Fatalf("events = %+v", evs)
	}
	for i, wake := range wakes {
		if wake.Cause != causes[i] || wake.Source != "worker" || *wake.Caption != "one line" || wake.Text != "full\nbody" {
			t.Fatalf("wake = %+v", wake)
		}
	}
	counts := evs[len(evs)-1]
	if counts.BackgroundTasks == nil || *counts.BackgroundTasks != 3 || *counts.Background != 2 || *counts.Monitors != 1 || counts.Live != nil {
		t.Fatalf("counts = %+v", counts)
	}
	worker := newPiMapper(StartSpec{})
	if got := worker.mapLine(piWidgetLine(piEventsWidgetKey, lines...), nopReplies{}); len(got) != 0 {
		t.Fatalf("a non-lead must not import hosted wake/count events: %+v", got)
	}
}

func TestHostedCountsKeepAggregateAndUsage(t *testing.T) {
	h := newLeadHarness(t).start()
	id, holder := h.runLead("one")
	c := h.runner.conv(id)
	cost := &wslive.UsageCost{USD: 1.25, Estimate: true}
	c.do(func() {
		c.handle(Event{Live: &wslive.Event{Kind: wslive.KindStatus, Cost: cost}})
	})
	// Only an agent remains: zero shells/monitors must not allow idle stop.
	h.holders.out(holder, `{"type":"system","subtype":"background_tasks_changed","tasks":[{"task_id":"a","task_type":"local_agent"}]}`)
	eventually(t, "known split", func() bool {
		st := h.state(id)
		return st.BackgroundTasks == 1 && st.Background != nil && *st.Background == 0 && st.Monitors != nil && *st.Monitors == 0
	})
	c.do(func() {
		if c.idleNow() {
			t.Error("aggregate background agent must still prevent idle stop")
		}
	})
	snap := h.snapshot(id)
	if snap.Host == nil || snap.Host.BackgroundTasks != 1 || snap.Host.Background == nil || snap.Host.Monitors == nil || snap.Usage == nil || snap.Usage.Cost == nil || snap.Usage.Cost.USD != 1.25 {
		t.Fatalf("snapshot = %+v, host %+v, usage %+v", snap, snap.Host, snap.Usage)
	}
	// The same aggregate with untyped tasks must clear a formerly known split.
	h.holders.out(holder, `{"type":"system","subtype":"background_tasks_changed","tasks":[{}]}`)
	eventually(t, "unknown split", func() bool {
		st := h.state(id)
		return st.BackgroundTasks == 1 && st.Background == nil && st.Monitors == nil
	})
	if host := h.snapshot(id).Host; host.Background != nil || host.Monitors != nil {
		t.Fatalf("unknown host split = %+v", host)
	}
	if _, err := h.runner.Stop(id); err != nil {
		t.Fatal(err)
	}
	if st := h.state(id); st.BackgroundTasks != 0 || st.Background != nil || st.Monitors != nil {
		t.Fatalf("stopped counts = %+v", st)
	}
}

func TestHostedWakeReplayDoesNotRepeatRing(t *testing.T) {
	h := newLeadHarness(t).start()
	id, holder := h.runLead("one")
	start := `{"type":"system","subtype":"task_started","task_id":"b","task_type":"local_bash","description":"build","is_backgrounded":true}`
	note := `{"type":"system","subtype":"task_notification","task_id":"b","status":"completed","summary":"build passed"}`
	h.holders.out(holder, start, note)
	eventually(t, "wake in ring", func() bool { return len(ringTexts(h.ring(id), "wake")) == 1 })
	h.restart()
	h.sync(id, holder)
	if got := ringTexts(h.ring(id), "wake"); !slices.Equal(got, []string{"build passed"}) {
		t.Fatalf("replayed wakes = %q", got)
	}
}

func TestHostedMailWakeAfterSuccessfulSend(t *testing.T) {
	h := newLeadHarness(t).start()
	id := h.lead(BackendClaude, LeadOptions{}).ID
	key := h.record(id).WSKey
	h.holders.failInputs("user", 1)
	h.mail.deliver(key, "From sender: first line\nfull body")
	eventually(t, "successful mail retry", func() bool {
		holder := h.record(id).HolderID
		return holder != "" && len(h.userInputs(holder)) == 1 && len(h.record(id).Mail) == 0
	})
	wakes := ringTexts(h.ring(id), "wake")
	if !slices.Equal(wakes, []string{"From sender: first line\nfull body"}) {
		t.Fatalf("mail wake must show once, only after send: %q", wakes)
	}
	if got := ringTexts(h.ring(id), wslive.RingUser); len(got) != 0 {
		t.Fatalf("mail still creates no user entry: %q", got)
	}
	// Check the wire retains the full text and only a one-line caption.
	raw, _ := json.Marshal(wakeEvent("mail", "mailbox", "", wakes[0]).Live)
	if !strings.Contains(string(raw), `"caption":"From sender: first line"`) {
		t.Fatalf("wire = %s", raw)
	}
}
