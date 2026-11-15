package wsagent

import (
	"encoding/json"
	"io"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wslive"
)

func observedEvents(t *testing.T, events []Event) []*wslive.ModelObservation {
	t.Helper()
	var out []*wslive.ModelObservation
	for _, ev := range events {
		if ev.Live != nil && ev.Live.Observed != nil {
			out = append(out, ev.Live.Observed)
		}
	}
	return out
}

func observedClaudeLine(id, uuid, model, at, parent string) []byte {
	raw, _ := json.Marshal(map[string]any{
		"type": "assistant", "uuid": uuid, "timestamp": at, "parent_tool_use_id": parent,
		"message": map[string]any{"id": id, "model": model, "content": []any{map[string]string{"type": "text", "text": "answer"}}},
	})
	return raw
}

func observedPiLine(id, model, responseModel, effort string, at int64) []byte {
	raw, _ := json.Marshal(map[string]any{
		"type": "message_end", "message": map[string]any{
			"role": "assistant", "responseId": id, "model": model, "responseModel": responseModel,
			"thinkingLevel": effort, "timestamp": at, "stopReason": "stop",
			"content": []any{map[string]string{"type": "text", "text": "answer"}},
			"usage":   map[string]int{"input": 50, "output": 20},
		},
	})
	return raw
}

func applyObservedEvents(t *testing.T, st *wslive.State, events []Event) {
	t.Helper()
	for _, ev := range events {
		if ev.Live != nil {
			if _, err := st.Apply(*ev.Live, 900000); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestHostedObservedClaudeResponseBlocksAndReplay(t *testing.T) {
	m := newClaudeMapper(StartSpec{Model: "opus", Effort: "max", Lead: &LeadSpec{}})
	m.now = func() time.Time { return time.UnixMilli(900000) }
	st := wslive.NewState("hosted-observed")
	tokens := int64(50)
	m.context = &wslive.UsageContext{Tokens: &tokens}
	m.window = 200000
	cost := 0.25
	m.cost = &wslive.UsageCost{USD: cost}
	first := observedClaudeLine("msg-one", "block-one", "claude-physical-one", "2026-11-15T01:02:03.456Z", "")
	events := m.mapLine(first, nopReplies{})
	observed := observedEvents(t, events)
	at := time.Date(2026, 11, 15, 1, 2, 3, 456000000, time.UTC).UnixMilli()
	want := wslive.ModelObservation{ID: "msg-one", Model: "claude-physical-one", Source: "claude.assistant", At: at}
	if len(observed) != 1 || !reflect.DeepEqual(*observed[0], want) {
		t.Fatalf("response evidence = %+v, want %+v", observed, want)
	}
	applyObservedEvents(t, st, events)
	usage := st.Snapshot(at).Usage
	if usage == nil || usage.Cost == nil || usage.Cost.USD != cost || usage.Context == nil {
		t.Fatalf("observation cleared existing Claude usage: %+v", usage)
	}
	// A later block has its own line identity/time but the same response.
	block := m.mapLine(observedClaudeLine("msg-one", "block-two", "claude-physical-one", "2026-11-15T01:02:04Z", ""), nopReplies{})
	if got := observedEvents(t, block); len(got) != 1 || !reflect.DeepEqual(*got[0], want) {
		t.Fatalf("split block moved original evidence: %+v", got)
	}
	applyObservedEvents(t, st, block)
	applyObservedEvents(t, st, m.mapLine(first, nopReplies{}))
	m.mapLine([]byte(`{"type":"system","subtype":"compact_boundary"}`), nopReplies{})
	if got := m.mapLine(first, nopReplies{}); len(got) != 0 {
		t.Fatalf("compaction replay escaped existing exclusion: %+v", got)
	}
	// A fresh holder mapper replays the original record under a new receipt clock.
	rejoined := newClaudeMapper(StartSpec{Lead: &LeadSpec{}})
	rejoined.now = func() time.Time { return time.UnixMilli(at + 600000) }
	applyObservedEvents(t, st, rejoined.mapLine(first, nopReplies{}))
	snap := st.Snapshot(at + 700000)
	if snap.Lead.Observed == nil || !reflect.DeepEqual(snap.Lead.Observed.Current, want) || len(snap.Lead.Observed.History) != 1 {
		t.Fatalf("repeat/replay changed evidence: %+v", snap.Lead.Observed)
	}
}

func TestHostedObservedClaudeUnknownAndChildExclusion(t *testing.T) {
	m := newClaudeMapper(StartSpec{Model: "opus", Effort: "high", Lead: &LeadSpec{}})
	for _, raw := range [][]byte{
		[]byte(`{"type":"system","subtype":"init","model":"opus"}`),
		observedClaudeLine("", "", "physical", "2026-11-15T01:02:03Z", ""),
		observedClaudeLine("no-time", "uuid", "physical", "", ""),
		observedClaudeLine("bad-time", "uuid", "physical", "not-a-time", ""),
		observedClaudeLine("no-model", "uuid", "", "2026-11-15T01:02:03Z", ""),
		observedClaudeLine("unmatched", "uuid", "other", "2026-11-15T01:02:03Z", "unmatched"),
	} {
		if got := observedEvents(t, m.mapLine(raw, nopReplies{})); len(got) != 0 {
			t.Fatalf("unsupported evidence accepted from %s: %+v", raw, got)
		}
	}
	m.subs["direct-call"] = &claudeSub{task: "direct-task"}
	st := wslive.NewState("independent")
	applyObservedEvents(t, st, []Event{{Live: &wslive.Event{Kind: wslive.KindAgent, AgentID: "direct-task", Status: wslive.StatusRunning, Model: "haiku", Effort: "high"}}})
	applyObservedEvents(t, st, m.mapLine(observedClaudeLine("own", "own-uuid", "lead-output", "2026-11-15T01:02:03Z", ""), nopReplies{}))
	applyObservedEvents(t, st, m.mapLine(observedClaudeLine("child", "child-uuid", "child-output", "2026-11-15T01:02:04Z", "direct-call"), nopReplies{}))
	if got := m.mapLine(observedClaudeLine("deep", "deep-uuid", "deep-output", "2026-11-15T01:02:05Z", "childs-call"), nopReplies{}); len(got) != 0 {
		t.Fatalf("unmatched/deep child was attributed: %+v", got)
	}
	snap := st.Snapshot(900000)
	if snap.Lead.Observed.Current.Model != "lead-output" || len(snap.Agents) != 1 || snap.Agents[0].Observed.Current.Model != "child-output" {
		t.Fatalf("mixed own/child evidence: %+v", snap)
	}
	if child := snap.Agents[0]; child.Effort != "high" || child.Observed.Current.Effort != "" || child.Observed.Current.EffortSource != "" || child.Observed.Current.OutputTps != nil {
		t.Fatalf("spawn effort/rate became response evidence: %+v", child)
	}
	// A worker's parent-tagged lines still cannot become its own evidence.
	worker := newClaudeMapper(StartSpec{})
	worker.subs["direct-call"] = &claudeSub{task: "direct-task"}
	if got := worker.mapLine(observedClaudeLine("child", "child-uuid", "other", "2026-11-15T01:02:03Z", "direct-call"), nopReplies{}); len(got) != 0 {
		t.Fatalf("worker accepted a parent-tagged response: %+v", got)
	}
	fallback := m.mapLine(observedClaudeLine("", "stable-uuid", "fallback-output", "2026-11-15T01:02:03Z", ""), nopReplies{})
	if got := observedEvents(t, fallback); len(got) != 1 || got[0].ID != "stable-uuid" {
		t.Fatalf("original UUID fallback missing: %+v", got)
	}
	if _, err := st.Apply(wslive.Event{Kind: wslive.KindAgent, Observed: &wslive.ModelObservation{ID: "rejected", Model: "bad", Source: "claude.assistant", At: 1}}, 900000); err == nil {
		t.Fatal("existing lead-agent rejection was lost")
	}
}

func TestHostedObservedPiOutputNotSettingsOrLegacyRate(t *testing.T) {
	m := newPiMapper(StartSpec{Model: "virtual-selected", Effort: "max", Lead: &LeadSpec{}})
	m.now = func() time.Time { return time.UnixMilli(900000) }
	if got := observedEvents(t, m.mapLine([]byte(`{"type":"response","command":"get_state","success":true,"data":{"model":{"id":"state-only"},"thinkingLevel":"max"}}`), nopReplies{})); len(got) != 0 {
		t.Fatalf("RPC state became output: %+v", got)
	}
	m.mapLine([]byte(`{"type":"response","command":"get_session_stats","success":true,"data":{"cost":0.75,"contextUsage":{"tokens":50,"contextWindow":200000}}}`), nopReplies{})
	m.outputStartedAt = 899000
	line := observedPiLine("response-one", "physical-output", "provider-concrete", "low", 123456)
	events := m.mapLine(line, nopReplies{})
	got := observedEvents(t, events)
	want := wslive.ModelObservation{ID: "response-one", Model: "provider-concrete", Source: "pi.assistant", At: 123456, Effort: "low", EffortSource: "message"}
	if len(got) != 1 || !reflect.DeepEqual(*got[0], want) {
		t.Fatalf("Pi response evidence = %+v, want %+v", got, want)
	}
	if m.outputTps == nil || *m.outputTps != 20 || got[0].OutputTps != nil {
		t.Fatalf("legacy rate must remain separate: rate=%v observed=%+v", m.outputTps, got[0])
	}
	st := wslive.NewState("pi-observed")
	applyObservedEvents(t, st, events)
	usage := st.Snapshot(900000).Usage
	if usage == nil || usage.Cost == nil || usage.Cost.USD != 0.75 || usage.Context == nil || *usage.Context.Tokens != 50 {
		t.Fatalf("observation cleared Pi statistics: %+v", usage)
	}
	m.now = func() time.Time { return time.UnixMilli(1900000) }
	applyObservedEvents(t, st, m.mapLine(line, nopReplies{}))
	fresh := newPiMapper(StartSpec{})
	applyObservedEvents(t, st, fresh.mapLine(line, nopReplies{}))
	snap := st.Snapshot(2000000)
	if !reflect.DeepEqual(snap.Lead.Observed.Current, want) || len(snap.Lead.Observed.History) != 1 {
		t.Fatalf("Pi replay changed original identity/time: %+v", snap.Lead.Observed)
	}
	// Model alone is physical output per Pi's session-format contract, and
	// absent per-message effort must not inherit the previous response's.
	applyObservedEvents(t, st, m.mapLine(observedPiLine("response-two", "physical-next", "", "", 124456), nopReplies{}))
	snap = st.Snapshot(2000000)
	if current := snap.Lead.Observed.Current; current.Model != "physical-next" || current.Effort != "" || current.EffortSource != "" || current.OutputTps != nil || len(snap.Lead.Observed.History) != 2 {
		t.Fatalf("physical/missing effort mapping: %+v", snap.Lead.Observed)
	}
	for _, raw := range [][]byte{
		observedPiLine("no-time", "physical", "", "high", 0),
		observedPiLine("no-model", "", "", "high", 125456),
		[]byte(`{"type":"message_end","message":{"role":"assistant","responseId":"pending","model":"not-final","stopReason":"pending","timestamp":123,"content":[]}}`),
		[]byte(`{"type":"message_end","message":{"role":"assistant","responseId":"error","model":"request-only","stopReason":"error","timestamp":123,"content":[]}}`),
		[]byte(`{"type":"message_end","message":{"role":"assistant","responseId":"aborted","model":"request-only","stopReason":"aborted","timestamp":123,"content":[]}}`),
	} {
		if got := observedEvents(t, m.mapLine(raw, nopReplies{})); len(got) != 0 {
			t.Fatalf("Pi unsupported evidence accepted from %s: %+v", raw, got)
		}
	}
}

func TestHostedObservedPiFixtureDigestAndReplay(t *testing.T) {
	first, _, firstEvents, _ := replayPi(t, "pi-tool-turn.jsonl", piToolTurnSpec(t), nil)
	_, _, replayEvents, _ := replayPi(t, "pi-tool-turn.jsonl", piToolTurnSpec(t), func(_ string, m *piMapper) {
		m.now = func() time.Time { return time.UnixMilli(9000000) }
	})
	observed, replayed := observedEvents(t, firstEvents), observedEvents(t, replayEvents)
	if len(observed) != 4 || !reflect.DeepEqual(observed, replayed) {
		t.Fatalf("fixture evidence/replay mismatch: %+v / %+v", observed, replayed)
	}
	for _, o := range observed {
		if !strings.HasPrefix(o.ID, "message:") || len(o.ID) < 80 || o.Model != "gpt-6.1-sol" || o.Source != "pi.assistant" || o.Effort != "low" || o.EffortSource != "message" || o.OutputTps != nil {
			t.Fatalf("fixture output evidence is incomplete: %+v", o)
		}
	}
	if current := first.Snapshot(9000000).Lead.Observed.Current; current.At != 1794101912862 {
		t.Fatalf("fixture original timestamp lost: %+v", current)
	}
	applyObservedEvents(t, first, replayEvents)
	if own := first.Snapshot(9000000).Lead.Observed; len(own.History) != 1 || own.Current.At != 1794101912862 {
		t.Fatalf("different receipt clock fabricated a model change: %+v", own)
	}

	m := newPiMapper(StartSpec{Model: "virtual-selection", Effort: "max"})
	one := []byte(`{"type":"message_end","message":{"role":"assistant","model":"physical-one","provider":"p","timestamp":1000,"stopReason":"stop","content":[{"type":"text","text":"private answer"}]}}`)
	// Canonical identity ignores JSON field order and insignificant whitespace,
	// including nested content, without omitting provider or response data.
	reordered := []byte(`{"message": {"content":[{"text":"private answer", "type":"text"}], "stopReason":"stop", "timestamp":1000,"provider":"p","model":"physical-one","role":"assistant"}, "type":"message_end"}`)
	a := observedEvents(t, m.mapLine(one, nopReplies{}))
	b := observedEvents(t, m.mapLine(reordered, nopReplies{}))
	if len(a) != 1 || len(b) != 1 || !reflect.DeepEqual(a, b) {
		t.Fatalf("canonical message digest changed under JSON reordering: %+v / %+v", a, b)
	}
	changed := observedEvents(t, m.mapLine(observedPiLine("", "physical-two", "provider-two", "", 2000), nopReplies{}))
	if len(changed) != 1 || changed[0].ID == a[0].ID || changed[0].Model != "provider-two" || changed[0].Effort != "" {
		t.Fatalf("changed physical response lost independent identity: %+v", changed)
	}
	writer := newLiveWriter(t.TempDir(), "digest-key", "pi", time.Now, io.Discard)
	writer.applyQuiet(wslive.Event{Kind: wslive.KindStatus, Observed: a[0]})
	writer.applyQuiet(wslive.Event{Kind: wslive.KindStatus, Observed: b[0]})
	writer.applyQuiet(wslive.Event{Kind: wslive.KindStatus, Observed: changed[0]})
	records, err := wslive.ReadObserved(writer.dir)
	if err != nil || len(records) != 2 {
		t.Fatalf("digest persistence/replay failed: %+v / %v", records, err)
	}
	raw, err := json.Marshal(records)
	if err != nil || strings.Contains(string(raw), "private answer") || strings.Contains(string(raw), "content") {
		t.Fatalf("raw response content entered metadata persistence: %s / %v", raw, err)
	}
	if own := writer.st.Snapshot(9000000).Lead.Observed; len(own.History) != 2 || own.History[1].Previous != "physical-one" || own.Current.Model != "provider-two" || own.Current.At != 2000 {
		t.Fatalf("digest model change history incorrect: %+v", own)
	}
}

func observedConversation(t *testing.T, r *Runner, id string, parent *ParentRef) *conv {
	t.Helper()
	dir := wsdashboard.SessionDir(r.opts.Dir, id)
	return &conv{r: r, dir: dir, inbox: filepath.Join(dir, "inbox"), rec: Record{ReplyID: id, Model: "requested-" + id, Effort: "max", Parent: parent},
		live: newLiveWriter(dir, "key-"+id, "pi", r.opts.Now, io.Discard)}
}

func TestHostedObservedSpawnOwnDirectAndDeepIndependent(t *testing.T) {
	r := NewRunner(Options{Dir: t.TempDir(), Now: func() time.Time { return time.UnixMilli(900000) }})
	lead := observedConversation(t, r, "ownlead", nil)
	direct := observedConversation(t, r, "directchild", &ParentRef{Kind: ParentLead, ReplyID: "ownlead"})
	deep := observedConversation(t, r, "deepchild", &ParentRef{Kind: ParentConversation, ReplyID: "directchild"})
	unknown := observedConversation(t, r, "unknownchild", &ParentRef{Kind: ParentConversation, ReplyID: "directchild"})
	for i, c := range []*conv{lead, direct, deep} {
		m := newPiMapper(StartSpec{Model: c.rec.Model, Effort: "max"})
		for _, ev := range m.mapLine(observedPiLine("", "output-"+c.rec.ReplyID, "", "", int64(1000+i)), nopReplies{}) {
			if ev.Live != nil {
				c.apply(*ev.Live, true) // holder replay uses the same own-state seam
			}
		}
	}
	for _, c := range []*conv{direct, deep, unknown} {
		lead.apply(SpawnAgentEvent(c.state(), "child-key"), true)
	}
	snap := lead.live.st.Snapshot(900000)
	if lead.state().Observed.Model != "output-ownlead" || snap.Lead.Observed.Current.Model != "output-ownlead" || len(snap.Agents) != 3 {
		t.Fatalf("own evidence changed during child relay: %+v", snap)
	}
	for _, a := range snap.Agents {
		id := a.ID[len("pi:"):]
		if id == "unknownchild" {
			if a.Observed != nil {
				t.Fatalf("unknown child inherited evidence: %+v", a)
			}
			continue
		}
		if a.Observed == nil || a.Observed.Current.Model != "output-"+id || !strings.HasPrefix(a.Observed.Current.ID, "message:") || a.Model != "requested-"+id || a.Observed.Current.Effort != "" {
			t.Fatalf("child evidence misattributed: %+v", a)
		}
		if id == "deepchild" && a.ParentID != "pi:directchild" {
			t.Fatalf("deep parent association lost: %+v", a)
		}
	}
	// Native Pi widget rows carry their own evidence unchanged, and never
	// admit a lead/status event through the existing subagent-only filter.
	own := wslive.ModelObservation{ID: "native-deep", Model: "native-output", Source: "pi.assistant", At: 5000}
	row, _ := json.Marshal(wslive.Event{Kind: wslive.KindAgent, AgentID: "native-deep", ParentID: "native-direct", Observed: &own})
	status, _ := json.Marshal(wslive.Event{Kind: wslive.KindStatus, Observed: &own})
	lines, _ := json.Marshal([]string{string(row), string(status)})
	widget := piAgentEvents(lines)
	if len(widget) != 1 || widget[0].Live.ParentID != "native-direct" || !reflect.DeepEqual(*widget[0].Live.Observed, own) {
		t.Fatalf("native widget evidence/filter changed: %+v", widget)
	}
}
