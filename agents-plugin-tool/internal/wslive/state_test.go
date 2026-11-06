package wslive

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsmailbox"
)

const ownerKey = "alpha-bravo-charlie"

func batchesJSON(t *testing.T, batches ...map[string]any) []json.RawMessage {
	t.Helper()
	out := make([]json.RawMessage, len(batches))
	for i, b := range batches {
		raw, err := json.Marshal(b)
		if err != nil {
			t.Fatal(err)
		}
		out[i] = raw
	}
	return out
}

func finished(seq int64, calls, failed int, tools ...string) map[string]any {
	return map[string]any{"epoch": "ep1", "seq": seq, "toolNames": tools, "callCount": calls, "failed": failed, "startedAt": 1000 + seq, "endedAt": 2000 + seq}
}

func running(seq int64, calls, done int, tool, preview string) map[string]any {
	return map[string]any{"epoch": "ep1", "seq": seq, "toolNames": []string{tool}, "callCount": calls, "failed": 0, "startedAt": 3000,
		"running": map[string]any{"toolName": tool, "argsPreview": preview, "startedAt": 3000, "done": done, "othersRunning": 0}}
}

func mustApply(t *testing.T, st *State, ev Event, now int64) Result {
	t.Helper()
	res, err := st.Apply(ev, now)
	if err != nil {
		t.Fatalf("Apply(%+v): %v", ev, err)
	}
	return res
}

func strp(s string) *string { return &s }

// TestLeadRootCarriesTurnFieldsAndTotals pins the lead root: the current
// turn's start, its done and failed totals (finished batches plus the
// running batch's finished calls, never recounted), the previous turn's
// duration, and the turn summary ring entry turn_end writes.
func TestLeadRootCarriesTurnFieldsAndTotals(t *testing.T) {
	st := NewState(ownerKey)
	mustApply(t, st, Event{Kind: KindTurnStart, At: 10_000, Origin: OriginWhiteboard}, 10_000)
	mustApply(t, st, Event{Kind: KindAgent, Batches: batchesJSON(t, finished(1, 3, 1, "Read", "Bash"), running(2, 2, 1, "Bash", "ls"))}, 11_000)
	// The same finished batch again must not count twice.
	mustApply(t, st, Event{Kind: KindAgent, Batches: batchesJSON(t, finished(1, 3, 1, "Read", "Bash"), running(2, 2, 1, "Bash", "ls"))}, 11_500)

	lead := st.Snapshot(12_000).Lead
	if lead.TurnStartedAt != 10_000 || lead.TurnOrigin != OriginWhiteboard {
		t.Fatalf("turn start = %d origin %q", lead.TurnStartedAt, lead.TurnOrigin)
	}
	if lead.Done != 3 || lead.Failed != 1 {
		t.Fatalf("totals = %d done %d failed, want 3 and 1", lead.Done, lead.Failed)
	}
	if lead.Phase != PhaseTool {
		t.Fatalf("phase = %q, want tool while a batch runs", lead.Phase)
	}

	res := mustApply(t, st, Event{Kind: KindTurnEnd, At: 15_000}, 15_000)
	lead = st.Snapshot(15_000).Lead
	if lead.TurnStartedAt != 0 || lead.PreviousTurnMs != 5_000 || lead.Phase != PhaseIdle {
		t.Fatalf("after turn_end: start %d previous %d phase %q", lead.TurnStartedAt, lead.PreviousTurnMs, lead.Phase)
	}
	// The running batch closes with its unfinished call failed, as the host
	// batch model closes it.
	if last := lead.Batches[len(lead.Batches)-1]; last.Running != nil || last.EndedAt == 0 || last.Failed != 1 {
		t.Fatalf("running batch not closed at turn end: %+v", last)
	}
	if len(res.Ring) != 1 || res.Ring[0] != (RingEntry{Kind: RingTurn, Origin: OriginWhiteboard, Done: 3, Failed: 2, DurationMs: 5_000}) {
		t.Fatalf("turn summary = %+v", res.Ring)
	}

	// A new turn restarts the totals; earlier batches stay counted.
	mustApply(t, st, Event{Kind: KindTurnStart, At: 20_000}, 20_000)
	if lead := st.Snapshot(20_000).Lead; lead.Done != 0 || lead.Failed != 0 || lead.PreviousTurnMs != 5_000 {
		t.Fatalf("new turn: %+v", lead)
	}
}

// TestBatchCapsBoundTheSnapshot pins the batch, tool-name, and preview caps
// and drops malformed batches.
func TestBatchCapsBoundTheSnapshot(t *testing.T) {
	st := NewState(ownerKey)
	var batches []map[string]any
	for i := int64(1); i <= 7; i++ {
		batches = append(batches, finished(i, 1, 0, "Read"))
	}
	tools := []string{"a", "b", "c", "d", "e", "f", "g", strings.Repeat("x", 100)}
	batches = append(batches,
		map[string]any{"seq": 0, "toolNames": []string{}, "callCount": 0, "failed": 0, "startedAt": 1, "endedAt": 2},                             // seq 0
		map[string]any{"seq": 9, "toolNames": []string{}, "callCount": 1, "failed": 2, "startedAt": 1, "endedAt": 2},                             // failed > calls
		map[string]any{"seq": 10, "toolNames": []string{}, "callCount": 1, "failed": 0, "startedAt": 1},                                          // no state
		map[string]any{"seq": 11, "toolNames": tools, "callCount": 8, "failed": 0, "startedAt": 1, "endedAt": 5, "generating": map[string]any{}}, // two states
		running(12, 8, 0, strings.Repeat("T", 100), strings.Repeat("p", 500)),
	)
	batches[len(batches)-1]["toolNames"] = tools
	mustApply(t, st, Event{Kind: KindAgent, Batches: batchesJSON(t, batches...)}, 5000)

	got := st.Snapshot(5000).Lead.Batches
	if len(got) != BatchCap {
		t.Fatalf("%d batches kept, want %d", len(got), BatchCap)
	}
	if got[0].Seq != 4 || got[BatchCap-1].Seq != 12 {
		t.Fatalf("kept seqs %d..%d, want the newest valid five (4..7, 12)", got[0].Seq, got[BatchCap-1].Seq)
	}
	last := got[BatchCap-1]
	if len(last.ToolNames) != ToolNameCap {
		t.Fatalf("tool names = %v, want %d", last.ToolNames, ToolNameCap)
	}
	if n := len([]rune(last.Running.ArgsPreview)); n != ArgsPreviewCap {
		t.Fatalf("args preview is %d runes, want %d", n, ArgsPreviewCap)
	}
	if n := len([]rune(last.Running.ToolName)); n != ToolNameRunes {
		t.Fatalf("tool name is %d runes, want %d", n, ToolNameRunes)
	}
	for _, b := range got[:BatchCap-1] {
		if b.Running != nil || b.Generating != nil || b.EndedAt == 0 {
			t.Fatalf("finished batch carries running state: %+v", b)
		}
	}
}

// TestStreamingToolCallShowsNameAndChars pins a call whose arguments are
// still streaming: its tool name and character count, and the tool phase.
func TestStreamingToolCallShowsNameAndChars(t *testing.T) {
	st := NewState(ownerKey)
	gen := map[string]any{"epoch": "ep1", "seq": 1, "toolNames": []string{}, "callCount": 0, "failed": 0, "startedAt": 100,
		"generating": map[string]any{"kind": "tool", "phaseStartedAt": 150, "toolName": "Write", "chars": 42}}
	res := mustApply(t, st, Event{Kind: KindAgent, Batches: batchesJSON(t, gen)}, 200)
	lead := st.Snapshot(200).Lead
	g := lead.Batches[0].Generating
	if g == nil || g.Kind != PhaseTool || g.ToolName != "Write" || g.Chars != 42 {
		t.Fatalf("generating = %+v", g)
	}
	if lead.Phase != PhaseTool || lead.PhaseStartedAt != 150 || !res.Urgent {
		t.Fatalf("phase %q since %d urgent %v", lead.Phase, lead.PhaseStartedAt, res.Urgent)
	}
	raw, _ := json.Marshal(lead.Batches[0])
	if strings.Contains(string(raw), "argsPreview") {
		t.Fatalf("a streaming call carries an args preview: %s", raw)
	}
}

// TestTreeRetainsRunningAndLastFiveFinished pins tree retention: every
// running agent stays, finished agents beyond the newest five drop, and a
// failed agent keeps its failed marker.
func TestTreeRetainsRunningAndLastFiveFinished(t *testing.T) {
	st := NewState(ownerKey)
	for i := 0; i < 3; i++ {
		mustApply(t, st, Event{Kind: KindAgent, AgentID: fmt.Sprintf("run%d", i), Label: "worker"}, int64(100+i))
	}
	for i := 0; i < 8; i++ {
		id := fmt.Sprintf("fin%d", i)
		mustApply(t, st, Event{Kind: KindAgent, AgentID: id}, int64(200+i))
		status := StatusDone
		if i == 6 {
			status = StatusFailed
		}
		mustApply(t, st, Event{Kind: KindAgent, AgentID: id, Status: status}, int64(1000+i))
	}
	snap := st.Snapshot(2000)
	byID := map[string]Agent{}
	for _, a := range snap.Agents {
		byID[a.ID] = a
	}
	if len(snap.Agents) != 3+FinishedAgentCap {
		t.Fatalf("%d agents kept, want 3 running + %d finished", len(snap.Agents), FinishedAgentCap)
	}
	for i := 0; i < 3; i++ {
		if a := byID[fmt.Sprintf("run%d", i)]; a.Status != StatusRunning || a.Phase != PhaseThinking {
			t.Fatalf("running agent run%d: %+v", i, a)
		}
	}
	for i := 0; i < 3; i++ {
		if _, ok := byID[fmt.Sprintf("fin%d", i)]; ok {
			t.Fatalf("old finished agent fin%d kept", i)
		}
	}
	if a := byID["fin6"]; a.Status != StatusFailed || a.EndedAt != 1006 {
		t.Fatalf("failed agent lost its marker: %+v", a)
	}
}

// TestPermissionWaitShowsToolAndPreview pins the permission-wait state on
// the lead and on a subagent.
func TestPermissionWaitShowsToolAndPreview(t *testing.T) {
	st := NewState(ownerKey)
	mustApply(t, st, Event{Kind: KindTurnStart, At: 100}, 100)
	res := mustApply(t, st, Event{Kind: KindPermission, Tool: "Bash", Args: map[string]any{"command": "touch x", "timeout": 5}, At: 150}, 150)
	lead := st.Snapshot(160).Lead
	if lead.Phase != PhasePermission || lead.PhaseStartedAt != 150 || !res.Urgent {
		t.Fatalf("phase %q since %d urgent %v", lead.Phase, lead.PhaseStartedAt, res.Urgent)
	}
	if lead.Permission == nil || lead.Permission.ToolName != "Bash" || lead.Permission.ArgsPreview != "command: touch x, timeout: 5" {
		t.Fatalf("permission = %+v", lead.Permission)
	}
	mustApply(t, st, Event{Kind: KindPermission, Clear: true}, 170)
	if lead := st.Snapshot(170).Lead; lead.Permission != nil || lead.Phase != PhaseThinking {
		t.Fatalf("after clear: %+v", lead)
	}

	mustApply(t, st, Event{Kind: KindPermission, AgentID: "sub1", Tool: "Edit", Args: "file.go"}, 200)
	if a := st.Snapshot(200).Agents[0]; a.Phase != PhasePermission || a.Permission.ArgsPreview != "file.go" {
		t.Fatalf("subagent permission: %+v", a)
	}
	if _, err := st.Apply(Event{Kind: KindPermission}, 210); err == nil {
		t.Fatal("permission without tool or clear accepted")
	}
}

// TestSessionKeysNeverReachTheSnapshot pins the relabelling: a delegate's
// session key becomes its reply-id, and the session's own key and every
// reported delegate key are scrubbed from previews, labels, captions, and
// reply text.
func TestSessionKeysNeverReachTheSnapshot(t *testing.T) {
	const delegateKey = "delta-echo-foxtrot"
	st := NewState(ownerKey)
	mustApply(t, st, Event{Kind: KindAgent, AgentID: "a1", SessionKey: delegateKey, Label: "reviewer " + delegateKey, Caption: strp("holding " + ownerKey)}, 100)
	mustApply(t, st, Event{Kind: KindAgent, AgentID: "a1", Batches: batchesJSON(t, running(1, 1, 0, "playbook.render", "name: reviewer, session_key: "+ownerKey))}, 110)
	mustApply(t, st, Event{Kind: KindPermission, Tool: "mailbox.send", Args: map[string]any{"session_key": "golf-hotel-india", "to": "x"}}, 120)
	mustApply(t, st, Event{Kind: KindReply, Text: "my key is " + ownerKey}, 130)
	mustApply(t, st, Event{Kind: KindAgent, Caption: strp(`{"sessionKey":"` + delegateKey + `"}`)}, 140)

	raw, err := json.Marshal(st.Snapshot(200))
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{ownerKey, delegateKey, "golf-hotel-india"} {
		if strings.Contains(string(raw), key) {
			t.Fatalf("snapshot carries session key %q: %s", key, raw)
		}
	}
	agent := st.Snapshot(200).Agents[0]
	if agent.ReplyID != wsmailbox.ReplyID(delegateKey) {
		t.Fatalf("delegate replyId = %q, want %q", agent.ReplyID, wsmailbox.ReplyID(delegateKey))
	}
	if !strings.Contains(agent.Label, "id:"+wsmailbox.ReplyID(delegateKey)) {
		t.Fatalf("label not relabelled: %q", agent.Label)
	}
	if st.Snapshot(200).ReplyID != wsmailbox.ReplyID(ownerKey) {
		t.Fatal("snapshot replyId is not the session's")
	}
}

// TestReplyStreamsThenMovesToRing pins streamed reply text: replaced while
// streaming (urgent), moved to the ring on final, and flushed by turn_end
// when no final came.
func TestReplyStreamsThenMovesToRing(t *testing.T) {
	st := NewState(ownerKey)
	mustApply(t, st, Event{Kind: KindTurnStart}, 100)
	if res := mustApply(t, st, Event{Kind: KindReply, Text: "Hel"}, 110); !res.Urgent {
		t.Fatal("streamed reply not urgent")
	}
	mustApply(t, st, Event{Kind: KindReply, Text: "Hello wor"}, 120)
	lead := st.Snapshot(120).Lead
	if lead.Reply == nil || lead.Reply.Text != "Hello wor" || lead.Reply.StartedAt != 110 || lead.Phase != PhaseReplying {
		t.Fatalf("reply = %+v phase %q", lead.Reply, lead.Phase)
	}
	res := mustApply(t, st, Event{Kind: KindReply, Text: "Hello world", Final: true}, 130)
	if len(res.Ring) != 1 || res.Ring[0].Kind != RingReply || res.Ring[0].Text != "Hello world" || st.Snapshot(130).Lead.Reply != nil {
		t.Fatalf("final reply: %+v", res.Ring)
	}
	mustApply(t, st, Event{Kind: KindReply, Text: "unfinished"}, 140)
	res = mustApply(t, st, Event{Kind: KindTurnEnd}, 150)
	if len(res.Ring) != 2 || res.Ring[0].Text != "unfinished" || res.Ring[1].Kind != RingTurn {
		t.Fatalf("turn_end ring: %+v", res.Ring)
	}

	long := strings.Repeat("é", TextBytes)
	mustApply(t, st, Event{Kind: KindReply, Text: long}, 160)
	if r := st.Snapshot(160).Lead.Reply; !r.Truncated || len(r.Text) > TextBytes {
		t.Fatalf("long reply: %d bytes truncated %v", len(r.Text), r.Truncated)
	}
}

// TestOriginTags pins the origin tags: user messages are terminal or
// whiteboard, and a turn may carry mail.
func TestOriginTags(t *testing.T) {
	st := NewState(ownerKey)
	for origin, want := range map[string]string{"": OriginTerminal, OriginTerminal: OriginTerminal, OriginWhiteboard: OriginWhiteboard} {
		res := mustApply(t, st, Event{Kind: KindUser, Text: "hi", Origin: origin}, 100)
		if len(res.Ring) != 1 || res.Ring[0] != (RingEntry{Kind: RingUser, Origin: want, Text: "hi"}) || res.Changed {
			t.Fatalf("user origin %q: %+v", origin, res)
		}
	}
	if _, err := st.Apply(Event{Kind: KindUser, Text: "hi", Origin: OriginMail}, 100); err == nil {
		t.Fatal("a user message tagged mail was accepted")
	}
	mustApply(t, st, Event{Kind: KindTurnStart, Origin: OriginMail}, 200)
	res := mustApply(t, st, Event{Kind: KindTurnEnd}, 300)
	if res.Ring[0].Origin != OriginMail {
		t.Fatalf("mail-started turn summary: %+v", res.Ring)
	}
	if _, err := st.Apply(Event{Kind: KindTurnStart, Origin: "browser"}, 400); err == nil {
		t.Fatal("unknown turn origin accepted")
	}
}

func TestResetClearsTreeAndUnknownKindsFail(t *testing.T) {
	st := NewState(ownerKey)
	st.SetAdapter("pi")
	mustApply(t, st, Event{Kind: KindTurnStart}, 100)
	mustApply(t, st, Event{Kind: KindAgent, AgentID: "a1"}, 110)
	mustApply(t, st, Event{Kind: KindReset}, 120)
	snap := st.Snapshot(130)
	if len(snap.Agents) != 0 || snap.Lead.TurnStartedAt != 0 || snap.Lead.Phase != PhaseIdle || snap.Adapter != "pi" {
		t.Fatalf("after reset: %+v", snap)
	}
	for _, ev := range []Event{{}, {Kind: "bogus"}, {Kind: KindAgent, AgentID: "bad id!"}, {Kind: KindAgent, Status: StatusDone}, {Kind: KindAgent, AgentID: "a", Status: "paused"}} {
		if _, err := st.Apply(ev, 140); err == nil {
			t.Fatalf("event %+v accepted", ev)
		}
	}
}

// TestReaderIgnoresUnknownFields pins version tolerance: unknown fields
// are ignored and absent ones read as empty.
func TestReaderIgnoresUnknownFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), SnapshotName)
	doc := `{"schemaVersion": 99, "replyId": "r1", "adapter": "pi", "updatedAt": 5, "futureField": {"x": 1},
		"lead": {"phase": "thinking", "newLeadField": true, "batches": [{"seq": 1, "toolNames": ["Read"], "callCount": 1, "failed": 0, "startedAt": 1, "endedAt": 2, "extra": 3}]},
		"agents": [{"id": "a1", "status": "running", "unknown": [1, 2]}]}`
	if err := os.WriteFile(path, []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	snap, err := ReadSnapshot(path)
	if err != nil {
		t.Fatalf("ReadSnapshot: %v", err)
	}
	if snap.ReplyID != "r1" || snap.Lead.Phase != PhaseThinking || len(snap.Lead.Batches) != 1 || snap.Agents[0].ID != "a1" || snap.HeartbeatMs != 0 || snap.Lead.Permission != nil {
		t.Fatalf("snapshot = %+v", snap)
	}
}

// TestStaleSnapshotReading pins the stale rule the daemon ages snapshots
// by: updatedAt older than StaleAfterBeats heartbeats is stale, and a stale
// or adapter-less snapshot does not count as an adapter.
func TestStaleSnapshotReading(t *testing.T) {
	now := time.UnixMilli(1_000_000_000)
	beat := HeartbeatInterval.Milliseconds()
	fresh := Snapshot{UpdatedAt: now.UnixMilli() - StaleAfterBeats*beat, HeartbeatMs: beat}
	if fresh.Stale(now) {
		t.Fatal("a snapshot at the stale boundary reads stale")
	}
	old := Snapshot{UpdatedAt: now.UnixMilli() - StaleAfterBeats*beat - 1}
	if !old.Stale(now) {
		t.Fatal("an old snapshot with no heartbeat field reads live")
	}
	if (Snapshot{UpdatedAt: now.UnixMilli() - 3_000, HeartbeatMs: 500}).Stale(now) != true {
		t.Fatal("the writer's own heartbeat interval is not honoured")
	}

	dir := t.TempDir()
	write := func(name string, snap Snapshot) string {
		raw, _ := json.Marshal(snap)
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, raw, 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	live := NewState(ownerKey)
	live.SetAdapter("pi")
	if !AdapterPresent(write("live.json", live.Snapshot(now.UnixMilli())), now) {
		t.Fatal("a fresh adapter snapshot reads as no adapter")
	}
	if AdapterPresent(write("stale.json", live.Snapshot(now.UnixMilli()-StaleAfterBeats*beat-1)), now) {
		t.Fatal("a stale snapshot reads as an adapter")
	}
	if AdapterPresent(write("none.json", NewState(ownerKey).Snapshot(now.UnixMilli())), now) {
		t.Fatal("a snapshot with no adapter reads as an adapter")
	}
	if AdapterPresent(filepath.Join(dir, "missing.json"), now) {
		t.Fatal("a missing snapshot reads as an adapter")
	}
}

func TestRejectedPermissionCreatesNoAgent(t *testing.T) {
	st := NewState(ownerKey)
	if _, err := st.Apply(Event{Kind: KindPermission, AgentID: "ghost"}, 100); err == nil {
		t.Fatal("permission without tool accepted")
	}
	if agents := st.Snapshot(100).Agents; len(agents) != 0 {
		t.Fatalf("a rejected event created %+v", agents)
	}
}
