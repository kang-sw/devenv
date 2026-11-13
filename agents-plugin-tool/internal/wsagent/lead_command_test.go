package wsagent

import (
	"encoding/json"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wsinbox"
	"github.com/kang-sw/devenv/internal/wslive"
	"github.com/kang-sw/devenv/internal/wsmailbox"
)

// sendCommand holds a slash command the page sent for the conversation, as
// the dashboard's message route writes it, and tells the runner.
func (h *leadHarness) sendCommand(id, name, args string) {
	h.t.Helper()
	inbox := wsinbox.Dir(wsdashboard.SessionDir(h.dir, id))
	if _, _, err := wsinbox.Send(inbox, wsinbox.Entry{Command: &wsinbox.Command{Name: name, Args: args}}, time.Now()); err != nil {
		h.t.Fatalf("send the command: %v", err)
	}
	h.runner.InboxChanged(id)
}

func commandNames(list []wslive.SlashCommand) []string {
	var out []string
	for _, c := range list {
		out = append(out, c.Name)
	}
	return out
}

// TestClaudeLeadInitReportsSlashCommands: a lead's init carries the
// commands a message may run, which become the session's command list;
// the terminal-only ones stay out, a later init repeating the list changes
// nothing, and a profile conversation reports none. The fixture is the
// local-commands recording's /context turn twice, with the init lists
// filled as a session with plugins reports them.
func TestClaudeLeadInitReportsSlashCommands(t *testing.T) {
	lines := readLeadFixture(t, "claude-lead-slash-commands.jsonl")
	r := newLeadReplay(t)
	first := findLine(t, lines, 0, "init", func(l leadLine) bool { return l.field("subtype") == "init" })
	r.all(lines[:first+1])
	want := []string{"compact", "context", "cost", "review", "ws:lead-run", "mcp__ws__workflow"}
	if got := commandNames(r.st.Snapshot(r.clock).Commands); !slices.Equal(got, want) {
		t.Fatalf("command list = %q, want %q", got, want)
	}
	for _, c := range r.st.Snapshot(r.clock).Commands {
		if c.Layer != "" || c.Source != "" || c.Description != "" {
			t.Fatalf("an init names its commands only: %+v", c)
		}
	}
	second := findLine(t, lines, first+1, "second init", func(l leadLine) bool { return l.field("subtype") == "init" })
	r.all(lines[first+1 : second])
	evs := r.m.mapLine(lines[second].raw, nopReplies{})
	var repeated *wslive.Event
	for _, ev := range evs {
		if ev.Live != nil && ev.Live.Kind == wslive.KindCommands {
			repeated = ev.Live
		}
	}
	if repeated == nil {
		t.Fatalf("each init reports the list: %+v", evs)
	}
	if res, err := r.st.Apply(*repeated, r.clock); err != nil || res.Changed {
		t.Fatalf("a repeated list changes nothing: %+v %v", res, err)
	}

	m := newClaudeMapper(claudeSpec(t, `{"backend":"claude"}`))
	init := []byte(`{"type":"system","subtype":"init","session_id":"s","tools":[],"permissionMode":"dontAsk","slash_commands":["compact"]}`)
	for _, ev := range m.mapLine(init, nopReplies{}) {
		if ev.Live != nil && ev.Live.Kind == wslive.KindCommands {
			t.Fatalf("a profile conversation reports no command list: %+v", ev)
		}
	}
}

// TestClaudeLeadInitializeOffersCommandsBeforeAnyTurn: a lead's launch asks
// the harness to initialize, and its answer is the command list the page
// offers before the first turn (whose init brings none); a hidden "__" name
// stays out, a later init keeps the answer's descriptions, and a profile
// conversation takes no such answer.
func TestClaudeLeadInitializeOffersCommandsBeforeAnyTurn(t *testing.T) {
	answer := []byte(`{"type":"control_response","response":{"subtype":"success","request_id":"` + claudeCommandsAsk + `","response":{"commands":[` +
		`{"name":"compact","description":"Free up context","builtin":true},{"name":"review","description":"Review a branch"},{"name":"__remote-workflow","builtin":true}]}}}`)
	m := newClaudeMapper(leadSpec())
	var list *wslive.Event
	for _, ev := range m.mapLine(answer, nopReplies{}) {
		if ev.Live != nil && ev.Live.Kind == wslive.KindCommands {
			list = ev.Live
		}
	}
	if list == nil {
		t.Fatal("the initialize answer reports no command list")
	}
	want := []wslive.SlashCommand{{Name: "compact", Description: "Free up context"}, {Name: "review", Description: "Review a branch"}}
	if !slices.Equal(list.Commands, want) {
		t.Fatalf("commands = %+v, want %+v", list.Commands, want)
	}
	init := []byte(`{"type":"system","subtype":"init","session_id":"s","tools":[],"slash_commands":["compact","review","ws:lead-run"]}`)
	var after *wslive.Event
	for _, ev := range m.mapLine(init, nopReplies{}) {
		if ev.Live != nil && ev.Live.Kind == wslive.KindCommands {
			after = ev.Live
		}
	}
	if after == nil || len(after.Commands) != 3 || after.Commands[0].Description != "Free up context" || after.Commands[2].Name != "ws:lead-run" {
		t.Fatalf("a turn's init keeps the described names: %+v", after)
	}

	p := newClaudeMapper(claudeSpec(t, `{"backend":"claude"}`))
	for _, ev := range p.mapLine(answer, nopReplies{}) {
		if ev.Live != nil {
			t.Fatalf("a profile conversation takes no command list: %+v", ev)
		}
	}
}

// TestClaudeLeadLaunchAsksForCommands: a hosted claude lead's process,
// spawned under a new holder, asks the harness to initialize, and the
// answer is the session's command list before any init; a daemon restart's
// rejoin asks nothing. The holder id a spawn fills must not read as a
// rejoin's.
func TestClaudeLeadLaunchAsksForCommands(t *testing.T) {
	h := newLeadHarness(t).start()
	h.holders.setControl("initialize", "", `{"commands":[{"name":"compact","description":"Free up context"},{"name":"review"}]}`)
	id, holder := h.runLead("hello")
	initialize := func() int {
		n := 0
		for _, line := range h.holders.inputs(holder, "control_request") {
			var d struct {
				Request struct {
					Subtype string `json:"subtype"`
				} `json:"request"`
			}
			if json.Unmarshal([]byte(line), &d) == nil && d.Request.Subtype == "initialize" {
				n++
			}
		}
		return n
	}
	eventually(t, "the initialize ask", func() bool { return initialize() == 1 })
	want := []wslive.SlashCommand{{Name: "compact", Description: "Free up context"}, {Name: "review"}}
	eventually(t, "the command list", func() bool { return slices.Equal(h.snapshot(id).Commands, want) })

	h.sync(id, holder)
	h.restart()
	eventually(t, "the rejoined process", func() bool { return h.state(id).Process })
	h.sync(id, holder)
	if n := initialize(); n != 1 {
		t.Fatalf("a rejoin asks no initialize: %d asks", n)
	}
}

// TestLeadSeedsCommandsFromLastSession: a new hosted lead, whose process
// starts only at its first send, shows the command list of the newest
// other lead of its root and profile that has one, a closed session an
// earlier daemon left included; its launch's own list replaces it. No such
// session, another root, or another harness seeds nothing.
func TestLeadSeedsCommandsFromLastSession(t *testing.T) {
	h := newLeadHarness(t).start()
	if got := h.snapshot(h.lead(BackendClaude, LeadOptions{}).ID).Commands; len(got) != 0 {
		t.Fatalf("no earlier session, no commands: %+v", got)
	}

	oldList := []wslive.SlashCommand{{Name: "old", Description: "From the last session"}}
	oldKey := "lead-key-old"
	old := Record{
		ReplyID: wsmailbox.ReplyID(oldKey), Profile: LeadProfileFor(BackendClaude), Backend: BackendClaude, Root: h.root,
		WSKey: oldKey, Lead: true, CreatedAt: "2026-01-01T00:00:00Z", ClosedAt: "2026-01-02T00:00:00Z",
	}
	if err := WriteRecordForTest(h.dir, old); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(wslive.Snapshot{Commands: oldList})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(wslive.SnapshotPath(wsdashboard.SessionDir(h.dir, old.ReplyID)), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	h.restart()

	id := h.lead(BackendClaude, LeadOptions{}).ID
	if got := h.snapshot(id).Commands; !slices.Equal(got, oldList) {
		t.Fatalf("seeded from the closed session before any send: %+v", got)
	}
	h.holders.setControl("initialize", "", `{"commands":[{"name":"compact","description":"Free up context"},{"name":"review"}]}`)
	launched := []wslive.SlashCommand{{Name: "compact", Description: "Free up context"}, {Name: "review"}}
	h.send(id, "hello")
	h.holder(id)
	eventually(t, "the launch's list to replace the seed", func() bool { return slices.Equal(h.snapshot(id).Commands, launched) })

	if got := h.snapshot(h.lead(BackendClaude, LeadOptions{}).ID).Commands; !slices.Equal(got, launched) {
		t.Fatalf("seeded from the newest session with a list: %+v", got)
	}
	if got := h.snapshot(h.lead(BackendPi, LeadOptions{}).ID).Commands; len(got) != 0 {
		t.Fatalf("another harness seeds nothing: %+v", got)
	}
	other, err := h.runner.CreateLead(BackendClaude, t.TempDir(), LeadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got := h.snapshot(other.ID).Commands; len(got) != 0 {
		t.Fatalf("another root seeds nothing: %+v", got)
	}
}

// TestClaudeLeadCommandSplitsTheDrain: messages and commands held during a
// turn reach the harness in order: the text before a command joined with
// the claimed mail as one turn, the command alone as its bare typed line
// at that turn's end, the text after it at the command's end. What waits
// is in the record and survives a daemon restart; a batch that starts with
// a command runs it first and mail waits for the next message turn.
func TestClaudeLeadCommandSplitsTheDrain(t *testing.T) {
	h := newLeadHarness(t).start()
	id, holder := h.runLead("one")
	key := h.record(id).WSKey
	eventually(t, "the first turn", func() bool { return slices.Equal(h.userInputs(holder), []string{"one"}) })
	h.holders.out(holder, stateLine(HarnessRunning), initLine("sess-1", "auto"))
	h.sync(id, holder)

	h.send(id, "before")
	h.sendCommand(id, "compact", "keep the names")
	h.send(id, "after")
	h.mail.deliver(key, "From a@x: ping")
	eventually(t, "the mail to be claimed", func() bool { return len(h.record(id).Mail) == 1 })
	if st := h.state(id); st.Held != 4 {
		t.Fatalf("three messages and the mail are held: %+v", st)
	}
	h.holders.out(holder, replyLine("u1", "first"), resultLine(), stateLine(HarnessIdle))
	eventually(t, "the text before the command", func() bool {
		return slices.Equal(h.userInputs(holder), []string{"one", "before\n\nFrom a@x: ping"})
	})
	wantQueue := []QueuedInput{{Command: &wsinbox.Command{Name: "compact", Args: "keep the names"}}, {Text: "after"}}
	eventually(t, "the rest to be queued", func() bool {
		rec := h.record(id)
		return len(rec.Mail) == 0 && sameJSON(t, rec.Queued, mustJSON(t, wantQueue))
	})
	if st := h.state(id); st.Held != 2 || st.Status != StatusRunning {
		t.Fatalf("the queued command and message count as held: %+v", st)
	}

	// A daemon restart keeps the queue: the record holds it, and the
	// rejoined turn's end sends the command.
	h.holders.out(holder, stateLine(HarnessRunning), initLine("sess-1", "auto"))
	h.sync(id, holder)
	h.restart()
	if rec := h.record(id); !sameJSON(t, rec.Queued, mustJSON(t, wantQueue)) {
		t.Fatalf("the queue reloads from the record: %+v", rec.Queued)
	}
	eventually(t, "the running turn to be back", func() bool { st := h.state(id); return st.Process && st.Status == StatusRunning })
	h.holders.out(holder, replyLine("u2", "second"), resultLine(), stateLine(HarnessIdle))
	eventually(t, "the command alone", func() bool {
		return slices.Equal(h.userInputs(holder), []string{"one", "before\n\nFrom a@x: ping", "/compact keep the names"})
	})
	h.sync(id, holder)
	if got := h.userInputs(holder); len(got) != 3 {
		t.Fatalf("the text after the command waits for the command's end: %q", got)
	}
	h.holders.out(holder, turnLines("sess-1", "u3", "compacted")...)
	eventually(t, "the text after the command", func() bool {
		return slices.Equal(h.userInputs(holder)[3:], []string{"after"})
	})
	eventually(t, "the queue to empty", func() bool { return len(h.record(id).Queued) == 0 })
	h.holders.out(holder, turnLines("sess-1", "u4", "done")...)
	h.idle(id)

	// A batch that starts with a command: the command runs alone first,
	// and the mail goes with the message after it.
	h.send(id, "three")
	eventually(t, "the third turn", func() bool { return len(h.userInputs(holder)) == 5 })
	h.holders.out(holder, stateLine(HarnessRunning), initLine("sess-1", "auto"))
	h.sync(id, holder)
	h.sendCommand(id, "cost", "")
	h.send(id, "four")
	h.mail.deliver(key, "From b@x: more")
	eventually(t, "the mail to be claimed", func() bool { return len(h.record(id).Mail) == 1 })
	h.holders.out(holder, replyLine("u5", "third"), resultLine(), stateLine(HarnessIdle))
	eventually(t, "the leading command", func() bool { return slices.Equal(h.userInputs(holder)[5:], []string{"/cost"}) })
	if rec := h.record(id); len(rec.Mail) != 1 {
		t.Fatalf("mail never rides with a command: %+v", rec)
	}
	h.holders.out(holder, turnLines("sess-1", "u6", "cost")...)
	eventually(t, "the message with the mail", func() bool {
		return slices.Equal(h.userInputs(holder)[5:], []string{"/cost", "four\n\nFrom b@x: more"})
	})
	h.holders.out(holder, turnLines("sess-1", "u7", "four")...)
	h.idle(id)

	want := []string{"one", "before", "/compact keep the names", "after", "three", "/cost", "four"}
	if got := ringTexts(h.ring(id), wslive.RingUser); !slices.Equal(got, want) {
		t.Fatalf("ring user entries = %q, want %q", got, want)
	}
}

// TestLeadQueuedInputDropsWithTheConversation: a closed conversation drops
// what it queued, as it drops its held messages, and an interrupt keeps
// it: the inbox shows it received, so it is sent after the interrupted
// turn.
func TestLeadQueuedInputDropsWithTheConversation(t *testing.T) {
	h := newLeadHarness(t).start()
	id, holder := h.runLead("one")
	eventually(t, "the first turn", func() bool { return slices.Equal(h.userInputs(holder), []string{"one"}) })
	h.holders.out(holder, stateLine(HarnessRunning), initLine("sess-1", "auto"))
	h.sync(id, holder)
	h.send(id, "two")
	h.sendCommand(id, "compact", "")
	h.send(id, "three")
	h.holders.out(holder, replyLine("u1", "first"), resultLine(), stateLine(HarnessIdle))
	eventually(t, "the text before the command", func() bool { return len(h.userInputs(holder)) == 2 })
	h.holders.out(holder, stateLine(HarnessRunning), initLine("sess-1", "auto"))
	h.sync(id, holder)

	_, returned, err := h.runner.Interrupt(id)
	if err != nil || len(returned) != 0 {
		t.Fatalf("an interrupt returns held messages only: %q %v", returned, err)
	}
	if rec := h.record(id); len(rec.Queued) != 2 {
		t.Fatalf("an interrupt keeps the queue: %+v", rec.Queued)
	}
	h.holders.out(holder, resultLine(), stateLine(HarnessIdle))
	eventually(t, "the queued command after the interrupted turn", func() bool {
		return slices.Equal(h.userInputs(holder)[2:], []string{"/compact"})
	})
	if _, err := h.runner.CloseConversation(id); err != nil {
		t.Fatal(err)
	}
	if rec := h.record(id); len(rec.Queued) != 0 {
		t.Fatalf("a closed conversation drops its queue: %+v", rec.Queued)
	}
}

// TestLeadStopKeepsTheQueueWithoutARestart: a stop by the user ends the
// process with input still queued, and no process starts until the next
// message.
func TestLeadStopKeepsTheQueueWithoutARestart(t *testing.T) {
	h := newLeadHarness(t).start()
	id, holder := h.runLead("one")
	eventually(t, "the first turn", func() bool { return slices.Equal(h.userInputs(holder), []string{"one"}) })
	h.holders.out(holder, stateLine(HarnessRunning), initLine("sess-1", "auto"))
	h.sync(id, holder)
	h.send(id, "two")
	h.sendCommand(id, "compact", "")
	h.send(id, "three")
	h.holders.out(holder, replyLine("u1", "first"), resultLine(), stateLine(HarnessIdle))
	eventually(t, "the text before the command", func() bool { return len(h.userInputs(holder)) == 2 })
	eventually(t, "the rest to be queued", func() bool { return len(h.record(id).Queued) == 2 })

	if _, err := h.runner.Stop(id); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	st, rec := h.state(id), h.record(id)
	if st.Process || len(rec.Queued) != 2 || h.holders.opCount("launch") != 1 {
		t.Fatalf("a stop starts no process and keeps the queue: state %+v, queue %+v, holder calls %v", st, rec.Queued, h.holders.opList())
	}
}

// TestPiLeadCommands: a pi lead asks for its commands at start and after
// each turn, and lists them with each one's source and the compact command
// pi leaves out; a held command goes as a prompt of its typed line at the
// turn's end (never a steer or follow-up), and a held /compact as the
// compact command.
func TestPiLeadCommands(t *testing.T) {
	h := newLeadHarness(t).start()
	id := h.lead(BackendPi, LeadOptions{}).ID
	h.send(id, "hello")
	holder := h.holder(id)
	eventually(t, "the command list ask at start", func() bool { return len(h.holders.inputs(holder, "get_commands")) == 1 })
	h.holders.out(holder, `{"type":"response","command":"get_commands","success":true,"data":{"commands":[`+
		`{"name":"review","description":"Review the diff","source":"extension","sourceInfo":{"path":"/x"}},`+
		`{"name":"fix","source":"prompt"},`+
		`{"name":"skill:audit","description":"Audit","source":"skill"}]}}`)
	want := []wslive.SlashCommand{
		{Name: "compact", Description: "Manually compact the session context", Source: "builtin"},
		{Name: "review", Description: "Review the diff", Source: "extension"},
		{Name: "fix", Source: "prompt"},
		{Name: "skill:audit", Description: "Audit", Source: "skill"},
	}
	eventually(t, "the command list", func() bool { return sameJSON(t, h.snapshot(id).Commands, mustJSON(t, want)) })

	h.holders.out(holder, `{"type":"agent_start"}`)
	eventually(t, "the turn", func() bool { return h.state(id).Status == StatusRunning })
	h.sendCommand(id, "review", "the tests")
	h.sendCommand(id, "compact", "keep the names")
	h.runner.conv(id).do(func() {})
	prompts := func() []string { return h.holders.inputs(holder, "prompt") }
	if got := prompts(); len(got) != 1 || h.state(id).Held != 2 {
		t.Fatalf("held commands wait for the turn's end: prompts %q, state %+v", got, h.state(id))
	}
	h.holders.out(holder, `{"type":"agent_settled"}`)
	eventually(t, "the command's prompt", func() bool { return len(prompts()) == 2 })
	if got := prompts()[1]; !sameJSON(t, json.RawMessage(got), []byte(`{"type":"prompt","message":"/review the tests"}`)) {
		t.Fatalf("command prompt = %s", got)
	}
	eventually(t, "the command list ask after the turn", func() bool { return len(h.holders.inputs(holder, "get_commands")) == 2 })
	h.holders.out(holder, `{"type":"agent_start"}`, `{"type":"agent_settled"}`)
	eventually(t, "the compact command", func() bool { return len(h.holders.inputs(holder, "compact")) == 1 })
	if got := h.holders.inputs(holder, "compact")[0]; !sameJSON(t, json.RawMessage(got), []byte(`{"type":"compact","customInstructions":"keep the names"}`)) {
		t.Fatalf("compact line = %s", got)
	}
	h.holders.out(holder, `{"type":"response","command":"compact","success":true}`)
	h.idle(id)
	if n := len(h.holders.inputs(holder, "steer")) + len(h.holders.inputs(holder, "follow_up")); n != 0 {
		t.Fatalf("a command is never steered: %d lines", n)
	}
	if got := ringTexts(h.ring(id), wslive.RingUser); !slices.Equal(got, []string{"hello", "/review the tests", "/compact keep the names"}) {
		t.Fatalf("ring user entries = %q", got)
	}
}

// TestPiCommandsMapping pins the get_commands answer's mapping: pi's own
// compact entry is kept rather than added twice, and a profile
// conversation's settled turn asks for no command list.
func TestPiCommandsMapping(t *testing.T) {
	evs := piCommands(json.RawMessage(`{"commands":[{"name":"compact","description":"mine","source":"extension"}]}`))
	if len(evs) != 1 || !sameJSON(t, evs[0].Live.Commands, []byte(`[{"name":"compact","description":"mine","source":"extension"}]`)) {
		t.Fatalf("events = %+v", evs)
	}
	if evs := piCommands(json.RawMessage(`{"commands":[]}`)); len(evs) != 1 || len(evs[0].Live.Commands) != 1 || evs[0].Live.Commands[0] != piCompact {
		t.Fatalf("an empty list still offers compact: %+v", evs)
	}
	replies := &recordedReplies{}
	newPiMapper(piSpec(t, `{"backend":"pi"}`)).mapLine([]byte(`{"type":"agent_settled"}`), replies)
	if len(replies.sent) != 1 || replies.sent[0]["type"] != "get_session_stats" {
		t.Fatalf("a profile conversation asks for statistics only: %v", replies.sent)
	}
	replies = &recordedReplies{}
	lead := newPiMapper(StartSpec{Root: "/work", Dir: "/sess", Profile: Profile{Backend: BackendPi, Lead: true}, Lead: &LeadSpec{}})
	lead.mapLine([]byte(`{"type":"agent_settled"}`), replies)
	if len(replies.sent) != 2 || replies.sent[1]["type"] != "get_commands" {
		t.Fatalf("a lead's settled turn asks for its commands: %v", replies.sent)
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
