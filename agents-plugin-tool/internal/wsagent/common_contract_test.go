package wsagent

import (
	"encoding/json"
	"reflect"
	"slices"
	"testing"

	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wslive"
)

// These cases share lifecycle expectations, not a promise that the harnesses
// accept the same control arguments. The holder never executes a command.
type commonContractCase struct {
	backend string
	inputs  func(*leadHarness, string) []string
	start   []string
	finish  func(string) []string
}

func commonContractCases() []commonContractCase {
	return []commonContractCase{
		{BackendClaude, (*leadHarness).userInputs,
			[]string{stateLine(HarnessRunning), initLine("common-session", "auto")},
			func(text string) []string {
				return []string{replyLine("common-reply", text), resultLine(), stateLine(HarnessIdle)}
			}},
		{BackendPi, piPrompts, []string{`{"type":"agent_start"}`},
			func(text string) []string {
				return []string{jsonLine(map[string]any{"type": "message_end", "message": map[string]any{"role": "assistant", "content": []any{map[string]any{"type": "text", "text": text}}, "stopReason": "stop"}}), `{"type":"agent_settled"}`}
			}},
	}
}

func (tc commonContractCase) end(h *leadHarness, id, holder, text string) {
	h.t.Helper()
	h.holders.out(holder, tc.finish(text)...)
	h.idle(id)
}

func (tc commonContractCase) assertResume(h *leadHarness, id, holder string) {
	h.t.Helper()
	argv := h.holders.spawned(holder).Argv
	flag, want := "--resume", "common-session"
	if tc.backend == BackendPi {
		flag, want = "--session", piSessionFile(wsdashboard.SessionDir(h.dir, id))
		if slices.Contains(argv, "--resume") {
			h.t.Fatalf("Pi resumes its file, not a Claude session: %v", argv)
		}
	}
	if got, ok := flagValue(argv, flag); !ok || got != want {
		h.t.Fatalf("%s = %q (present %v), want %q: %v", flag, got, ok, want, argv)
	}
}

func TestCommonContractLifecycle(t *testing.T) {
	for _, tc := range commonContractCases() {
		t.Run(tc.backend, func(t *testing.T) {
			h := newLeadHarness(t).start()
			id := h.lead(tc.backend, LeadOptions{}).ID
			key := h.record(id).WSKey
			if key == "" || len(h.holders.opList()) != 0 {
				t.Fatal("creation must mint an identity without starting a process")
			}
			h.send(id, "one")
			holder := h.holder(id)
			if got := tc.inputs(h, holder); !slices.Equal(got, []string{"one"}) {
				t.Fatalf("fresh input = %q", got)
			}
			h.holders.out(holder, tc.start...)
			// A completed first turn establishes the backend's resume inputs.
			tc.end(h, id, holder, "first")
			h.send(id, "two")
			h.holders.out(holder, tc.start...)
			h.send(id, "held-a")
			h.send(id, "held-b")
			h.mail.deliver(key, "From a@x: mail-a")
			eventually(t, "held messages and mail", func() bool { return h.state(id).Held == 2 })
			h.mailSettled(id)
			h.send(id, "held-c")
			if got := tc.inputs(h, holder); !slices.Equal(got, []string{"one", "two"}) {
				t.Fatalf("held input reached a running turn: %q", got)
			}

			h.restart()
			eventually(t, "rejoin", func() bool {
				st := h.state(id)
				return st.Process && st.Status == StatusRunning && st.Held == 2
			})
			if rec := h.record(id); rec.WSKey != key || rec.HolderID != holder {
				t.Fatalf("rejoin changed identity: %+v", rec)
			}
			if h.holders.opCount("launch") != 1 || h.holders.opCount("spawn") != 1 || h.holders.opCount("stop") != 0 {
				t.Fatalf("rejoin changed process: %v", h.holders.opList())
			}
			h.holders.out(holder, tc.finish("second")...)
			wantInputs := []string{"one", "two", "held-a\n\nheld-b\n\nheld-c\n\nFrom a@x: mail-a"}
			eventually(t, "one ordered merged delivery", func() bool { return slices.Equal(tc.inputs(h, holder), wantInputs) })
			h.holders.out(holder, tc.start...)
			tc.end(h, id, holder, "third")
			if got := tc.inputs(h, holder); !slices.Equal(got, wantInputs) || h.state(id).Held != 0 || len(h.record(id).Mail) != 0 {
				t.Fatalf("delivery repeated or left held input: %q", got)
			}
			var conversation []string
			for _, entry := range h.ring(id) {
				switch entry.Kind {
				case wslive.RingUser, wslive.RingReply, wslive.RingTurn:
					conversation = append(conversation, entry.Kind+":"+entry.Text)
				}
			}
			want := []string{"user:one", "reply:first", "turn:", "user:two", "reply:second", "turn:", "user:held-a\n\nheld-b\n\nheld-c", "reply:third", "turn:"}
			if !slices.Equal(conversation, want) {
				t.Fatalf("ordered user/reply/end meanings = %q, want %q", conversation, want)
			}
			if _, err := h.runner.Stop(id); err != nil {
				t.Fatal(err)
			}
			h.send(id, "resume")
			second := h.holder(id)
			if second == holder || h.record(id).WSKey != key || h.holders.opCount("stop "+holder) != 1 {
				t.Fatalf("stop/resume identity or lifecycle changed: %v", h.holders.opList())
			}
			tc.assertResume(h, id, second)
			if got := tc.inputs(h, second); !slices.Equal(got, []string{"resume"}) {
				t.Fatalf("resumed input = %q", got)
			}
		})
	}
}

func TestCommonContractNextBoundarySettings(t *testing.T) {
	for _, tc := range commonContractCases() {
		t.Run(tc.backend, func(t *testing.T) {
			h := newLeadHarness(t).start()
			id := h.lead(tc.backend, LeadOptions{Effort: "medium"}).ID
			h.send(id, "one")
			holder := h.holder(id)
			h.holders.out(holder, tc.start...)
			model := "opus"
			if tc.backend == BackendPi {
				model = "anthropic/claude-opus"
			}
			if _, err := h.runner.SetSettings(id, &model, ptr("high")); err != nil {
				t.Fatal(err)
			}
			if got := commonSettingWires(h, holder, tc.backend); len(got) != 0 {
				t.Fatalf("settings reached a running turn: %v", got)
			}
			h.holders.out(holder, tc.finish("first")...)
			if tc.backend == BackendPi {
				// Unlike Claude controls, fakeHolders does not auto-answer Pi
				// RPCs. Supply the real protocol's correlated success replies.
				for _, command := range []struct {
					kind  string
					count int
				}{{"set_model", 1}, {"set_thinking_level", 1}, {"set_thinking_level", 2}} {
					eventually(t, command.kind, func() bool { return len(h.holders.inputs(holder, command.kind)) == command.count })
					var request map[string]any
					if err := json.Unmarshal([]byte(h.holders.inputs(holder, command.kind)[command.count-1]), &request); err != nil {
						t.Fatal(err)
					}
					h.holders.out(holder, jsonLine(map[string]any{"type": "response", "id": request["id"], "command": command.kind, "success": true}))
				}
			}
			h.idle(id)
			want := []any{
				map[string]any{"subtype": "set_model", "model": "opus"},
				map[string]any{"subtype": "apply_flag_settings", "settings": map[string]any{"effortLevel": "high"}},
			}
			if tc.backend == BackendPi {
				want = []any{
					map[string]any{"type": "set_model", "provider": "anthropic", "modelId": "claude-opus"},
					map[string]any{"type": "set_thinking_level", "level": "medium"},
					map[string]any{"type": "set_thinking_level", "level": "high"},
				}
			}
			if got := commonSettingWires(h, holder, tc.backend); !reflect.DeepEqual(got, want) {
				t.Fatalf("setting wire bodies = %v, want %v", got, want)
			}
			if st := h.state(id); !st.Process || st.Model != model || st.Effort != "high" || h.holders.opCount("spawn") != 1 || h.holders.opCount("stop") != 0 {
				t.Fatalf("supported update restarted or lost settings: %+v, %v", st, h.holders.opList())
			}
		})
	}
}

func commonSettingWires(h *leadHarness, holder, backend string) []any {
	if backend == BackendClaude {
		return h.controls(holder)
	}
	var out []any
	for _, line := range h.holders.inputs(holder, "") {
		var body map[string]any
		if err := json.Unmarshal([]byte(line), &body); err != nil {
			h.t.Fatal(err)
		}
		if body["type"] == "set_model" || body["type"] == "set_thinking_level" {
			delete(body, "id") // RPC correlation ids are intentionally not shared.
			out = append(out, body)
		}
	}
	return out
}

func TestCommonContractPiUnsupportedSettingsRestart(t *testing.T) {
	for _, change := range []struct {
		name          string
		model, effort *string
		flag, want    string
	}{
		{"fuzzy-model", ptr("opus"), nil, "--model", "opus"},
		{"default-model", ptr(""), nil, "--model", ""},
		{"default-effort", nil, ptr(""), "--thinking", ""},
	} {
		t.Run(change.name, func(t *testing.T) {
			tc := commonContractCases()[1]
			h := newLeadHarness(t).start()
			id := h.lead(BackendPi, LeadOptions{Model: "anthropic/claude-opus", Effort: "high"}).ID
			h.send(id, "one")
			holder := h.holder(id)
			h.holders.out(holder, tc.start...)
			tc.end(h, id, holder, "first")
			if _, err := h.runner.SetSettings(id, change.model, change.effort); err != nil {
				t.Fatal(err)
			}
			if h.state(id).Process || h.holders.opCount("stop "+holder) != 1 || len(commonSettingWires(h, holder, BackendPi)) != 0 {
				t.Fatalf("unsupported arguments must stop without an RPC: %v", h.holders.opList())
			}
			h.send(id, "two")
			second := h.holder(id)
			tc.assertResume(h, id, second)
			got, present := flagValue(h.holders.spawned(second).Argv, change.flag)
			if got != change.want || present != (change.want != "") {
				t.Fatalf("restart %s = %q (present %v), want %q", change.flag, got, present, change.want)
			}
		})
	}
}

func TestCommonContractJSONCompatibility(t *testing.T) {
	// Exact representative encodings also reject newly serialized internal
	// control/capability metadata. Existing worker capability secrets are not
	// new control descriptions and are deliberately left outside these leads.
	for _, backend := range []string{BackendPi, BackendClaude} {
		t.Run(backend, func(t *testing.T) {
			if backend != "pi" && backend != "claude" {
				t.Fatalf("backend wire name changed: %q", backend)
			}
			commonAssertJSON(t, Profile{Backend: backend}, `{"backend":"`+backend+`"}`)
			commonAssertJSON(t, Record{SchemaVersion: 1, ReplyID: "row", Profile: "lead", Backend: backend, Root: "/repo", SessionID: "native", WSKey: "stable", CreatedAt: "then", Lead: true, HolderID: "holder", HostPID: 42, HolderSeq: 7, Mail: []string{"waiting"}},
				`{"schema_version":1,"reply_id":"row","profile":"lead","backend":"`+backend+`","root":"/repo","backend_session_id":"native","ws_session_key":"stable","created_at":"then","lead":true,"holder_id":"holder","host_pid":42,"holder_seq":7,"mail":["waiting"]}`)
		})
	}
	commonAssertJSON(t, holderSession{Kind: "lead", ReplyID: "row", Cwd: "/repo"}, `{"kind":"lead","reply_id":"row","cwd":"/repo"}`)
	for _, event := range []struct {
		value wslive.Event
		want  string
	}{
		{wslive.Event{Kind: wslive.KindReset}, `{"kind":"reset"}`},
		{wslive.Event{Kind: wslive.KindTurnStart}, `{"kind":"turn_start"}`},
		{wslive.Event{Kind: wslive.KindUser, Text: "question"}, `{"kind":"user","text":"question"}`},
		{wslive.Event{Kind: wslive.KindReply, Text: "answer", Final: true}, `{"kind":"reply","text":"answer","final":true}`},
		{wslive.Event{Kind: wslive.KindTurnEnd}, `{"kind":"turn_end"}`},
	} {
		commonAssertJSON(t, event.value, event.want)
	}
}

func commonAssertJSON(t *testing.T, value any, want string) {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if !sameJSON(t, json.RawMessage(raw), []byte(want)) {
		t.Fatalf("%T JSON = %s, want %s", value, raw, want)
	}
}
