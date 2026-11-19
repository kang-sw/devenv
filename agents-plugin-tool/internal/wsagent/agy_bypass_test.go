package wsagent

import (
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsdashboard"
)

func agyBypassAsk(tool, request string) string {
	return `{"jsonrpc":"2.0","method":"session/request_permission","params":{"sessionId":"agy-s1","toolCall":{"toolCallId":"` + tool + `","name":"run_command","title":"fixture command","args":{"CommandLine":"echo fixture"}},"options":[{"optionId":"yes","kind":"allow_once"},{"optionId":"no","kind":"reject_once"}]},"id":"` + request + `"}`
}

func agyBypassAnswers(h *leadHarness, holder, request string) int {
	n := 0
	for _, line := range h.holders.inputs(holder, "") {
		var reply struct {
			ID     string `json:"id"`
			Result struct {
				Outcome struct {
					Option string `json:"optionId"`
				} `json:"outcome"`
			} `json:"result"`
		}
		if json.Unmarshal([]byte(line), &reply) == nil && reply.ID == request && reply.Result.Outcome.Option == "yes" {
			n++
		}
	}
	return n
}

func TestAgyLeadBypassApprovalResumeAndReattach(t *testing.T) {
	h, _ := newAgyLeadHarness(t)
	h.startAt("http://127.0.0.1:1")
	id := h.lead(BackendAgy, LeadOptions{PermissionMode: "bypass"}).ID
	if rec := h.record(id); !rec.AgyBypass || rec.PermissionMode != "default" || h.state(id).PermissionMode != "bypass" {
		t.Fatalf("bypass record/state: %+v", rec)
	}
	h.send(id, "work")
	holder := h.holder(id)
	eventually(t, "first prompt", func() bool { return len(agyPromptIDs(h, holder)) == 1 })
	for _, p := range h.holders.acpInputs(holder, "session/set_mode") {
		if acpField(t, p, "modeId") != "default" {
			t.Fatalf("non-native bypass sent: %s", p)
		}
	}
	h.holders.out(holder, agyBypassAsk("call-1", "request-1"))
	eventually(t, "auto-allow", func() bool { return agyBypassAnswers(h, holder, "request-1") == 1 })
	h.quiesce(id)
	if st := h.state(id); len(st.Approvals) != 0 || st.PermissionMode != "bypass" {
		t.Fatalf("auto-allow left a prompt: %+v", st)
	}
	if !slices.Contains(h.record(id).Answered, "call-1") {
		t.Fatal("auto-answer not durably tracked")
	}
	if _, err := h.runner.AnswerApproval(id, "call-1", true, ""); requestStatus(err) != 409 {
		t.Fatalf("second answer: %v", err)
	}

	// Native reports still update the native record, never ws's intent.
	h.holders.out(holder, `{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"agy-s1","update":{"sessionUpdate":"current_mode_update","currentModeId":"accept-edits"}}}`)
	eventually(t, "native report", func() bool {
		return h.state(id).ActiveMode == "accept-edits" && h.record(id).PermissionMode == "accept-edits"
	})
	if !h.record(id).AgyBypass || h.state(id).PermissionMode != "bypass" {
		t.Fatal("native mode report cleared bypass")
	}

	// A replacement daemon rejoins without a handshake or a duplicate answer.
	h.runner.Close()
	h.holders.restart()
	h.startAt("http://127.0.0.1:1")
	eventually(t, "replay completed", func() bool {
		c := h.runner.conv(id)
		done := false
		if c != nil {
			c.do(func() { done = c.proc != nil && !c.replaying })
		}
		return done
	})
	if !h.record(id).AgyBypass || agyBypassAnswers(h, holder, "request-1") != 1 || len(h.holders.acpInputs(holder, "initialize")) != 1 {
		t.Fatal("reattach lost bypass or re-answered")
	}
	h.holders.out(holder, agyBypassAsk("call-2", "request-2"))
	eventually(t, "auto-allow after reattach", func() bool { return agyBypassAnswers(h, holder, "request-2") == 1 })

	// A fresh process resumes in native default even after a native report
	// changed PermissionMode; the separate intent survives stopping it.
	if _, err := h.runner.Stop(id); err != nil {
		t.Fatal(err)
	}
	h.send(id, "resume")
	next := h.holder(id)
	eventually(t, "resumed session", func() bool { return len(agyPromptIDs(h, next)) == 1 })
	eventually(t, "resume reports native default with bypass retained", func() bool {
		rec := h.record(id)
		return len(h.holders.acpInputs(next, "session/resume")) == 1 && rec.AgyBypass && rec.PermissionMode == "default"
	})
	h.holders.out(next, agyBypassAsk("call-3", "request-3"))
	eventually(t, "auto-allow after resume", func() bool { return agyBypassAnswers(h, next, "request-3") == 1 })
}

func TestAgyDefaultLeadPromptsAndBypassToggle(t *testing.T) {
	h, _ := newAgyLeadHarness(t)
	h.startAt("http://127.0.0.1:1")
	id := h.lead(BackendAgy, LeadOptions{PermissionMode: "default"}).ID
	h.send(id, "work")
	holder := h.holder(id)
	eventually(t, "prompt", func() bool { return len(agyPromptIDs(h, holder)) == 1 })
	h.holders.out(holder, agyBypassAsk("waiting", "req-waiting"))
	eventually(t, "manual approval", func() bool { return len(h.state(id).Approvals) == 1 })
	h.quiesce(id)
	if agyBypassAnswers(h, holder, "req-waiting") != 0 || h.record(id).AgyBypass {
		t.Fatal("default lead auto-allowed")
	}
	// Non-user entry must fail before it changes intent or answers the
	// waiting request. The trusted user seam is the settings route's same
	// local && own decision, not a caller-supplied permission field.
	_, err := h.runner.SetPermissionMode(id, "bypass")
	var denied *requestError
	if !errors.As(err, &denied) || denied.status != http.StatusForbidden || denied.msg != "only the user may enable agy bypass through permission_mode" {
		t.Fatalf("non-user bypass entry: %v; want user-only 403", err)
	}
	h.quiesce(id)
	if rec := h.record(id); rec.AgyBypass || rec.PermissionMode != "default" || len(rec.Answered) != 0 {
		t.Fatalf("refused bypass changed the record: %+v", rec)
	}
	if st := h.state(id); st.PermissionMode != "default" || len(st.Approvals) != 1 || agyBypassAnswers(h, holder, "req-waiting") != 0 {
		t.Fatalf("refused bypass auto-allowed a waiting request: %+v", st)
	}
	if st, err := h.runner.setPermissionMode(id, "bypass", true); err != nil || st.PermissionMode != "bypass" {
		t.Fatalf("user enable bypass: %+v %v", st, err)
	}
	eventually(t, "release existing approval", func() bool { return agyBypassAnswers(h, holder, "req-waiting") == 1 })
	if rec := h.record(id); !rec.AgyBypass || rec.PermissionMode != "default" {
		t.Fatal("toggle persisted ws mode as native")
	}
	if st, err := h.runner.SetPermissionMode(id, "bypass"); err != nil || st.PermissionMode != "bypass" {
		t.Fatalf("non-user reselect existing bypass: %+v %v", st, err)
	}
	if st, err := h.runner.SetPermissionMode(id, "default"); err != nil || st.PermissionMode != "default" {
		t.Fatalf("non-user leave bypass: %+v %v", st, err)
	}
	h.holders.out(holder, agyBypassAsk("manual-again", "req-manual"))
	eventually(t, "manual again", func() bool { return len(h.state(id).Approvals) == 1 })
	if h.record(id).AgyBypass || agyBypassAnswers(h, holder, "req-manual") != 0 {
		t.Fatal("disabling bypass retained auto-allow")
	}

	// Simulate an old daemon, which preserves the unknown bypass flag but
	// prompts in native default. The new daemon must answer its pending
	// replayed prompt once, not leave it stuck in the UI.
	h.runner.Close()
	rec := h.record(id)
	rec.AgyBypass = true
	if err := writeRecord(wsdashboard.SessionDir(h.dir, id), rec); err != nil {
		t.Fatal(err)
	}
	h.holders.restart()
	h.startAt("http://127.0.0.1:1")
	eventually(t, "auto-allow replayed pending request", func() bool { return agyBypassAnswers(h, holder, "req-manual") == 1 })
	if len(h.state(id).Approvals) != 0 {
		t.Fatal("replayed request still pending")
	}
}

func TestAgyBypassScopeAndLegacyRecordRewrite(t *testing.T) {
	for _, rec := range []Record{
		{Lead: false, Backend: BackendAgy, AgyBypass: true},
		{Lead: true, Backend: BackendClaude, AgyBypass: true},
		{Lead: true, Backend: BackendCodex, AgyBypass: true},
	} {
		if rec.agyBypass() || rec.selectedPermissionMode() == "bypass" {
			t.Fatalf("bypass escaped agy lead scope: %+v", rec)
		}
	}
	original := Record{Lead: true, Backend: BackendAgy, PermissionMode: "default", AgyBypass: true}
	raw, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	// This typed decoder intentionally does not know agy_bypass. As the
	// pre-change Record decoder does, retain unknown top-level raw members,
	// mutate known state, and overlay known fields when writing.
	var old struct {
		Lead           bool   `json:"lead,omitempty"`
		Backend        string `json:"backend"`
		PermissionMode string `json:"permission_mode,omitempty"`
	}
	if err := json.Unmarshal(raw, &old); err != nil {
		t.Fatal(err)
	}
	var unknown map[string]json.RawMessage
	if err := json.Unmarshal(raw, &unknown); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"lead", "backend", "permission_mode"} {
		delete(unknown, key)
	}
	if string(unknown["agy_bypass"]) != "true" || old.PermissionMode != "default" {
		t.Fatal("legacy decoder cannot retain bypass as unknown")
	}
	old.PermissionMode = "accept-edits"
	knownRaw, err := json.Marshal(old)
	if err != nil {
		t.Fatal(err)
	}
	var rewritten map[string]json.RawMessage
	if err := json.Unmarshal(knownRaw, &rewritten); err != nil {
		t.Fatal(err)
	}
	for key, value := range unknown {
		if _, known := rewritten[key]; !known {
			rewritten[key] = value
		}
	}
	raw, err = json.Marshal(rewritten)
	if err != nil {
		t.Fatal(err)
	}
	var restored Record
	if err := json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	if !restored.AgyBypass || restored.PermissionMode != "accept-edits" || restored.launchPermissionMode() != "default" || restored.selectedPermissionMode() != "bypass" {
		t.Fatalf("downgrade lost intent: %+v", restored)
	}
	// A modern writer can explicitly clear the now-known optional member;
	// unknown preservation must not resurrect it.
	restored.selectPermissionMode("default")
	raw, err = json.Marshal(restored)
	if err != nil || strings.Contains(string(raw), "agy_bypass") {
		t.Fatalf("cleared bypass resurrected: %s %v", raw, err)
	}
}

// approvalSpy replaces a live process's answerer to see what the runner
// answered; the rest of the process is the real one.
type approvalSpy struct {
	Process
	answered []ApprovalRequest
	allowed  []bool
}

func (s *approvalSpy) AnswerApproval(req ApprovalRequest, allow bool, _ string) error {
	s.answered = append(s.answered, req)
	s.allowed = append(s.allowed, allow)
	return nil
}

// TestAgyBypassAnswersBeforePublishing: under bypass a permission request is
// allowed without ever being published (no pending approval, no page wake),
// so the dashboard modal never flashes. Other modes still publish it.
func TestAgyBypassAnswersBeforePublishing(t *testing.T) {
	for _, tc := range []struct {
		mode    string
		bypass  bool
		publish bool
	}{
		{mode: "bypass", bypass: true},
		{mode: "default", publish: true},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			h, _ := newAgyLeadHarness(t)
			h.startAt("http://127.0.0.1:1")
			id := h.lead(BackendAgy, LeadOptions{PermissionMode: tc.mode}).ID
			h.send(id, "work")
			holder := h.holder(id)
			eventually(t, "prompt", func() bool { return len(agyPromptIDs(h, holder)) == 1 })
			h.quiesce(id)

			c := h.runner.conv(id)
			var (
				spy       = &approvalSpy{}
				published int
				notes     int64
				answered  []string
			)
			c.do(func() {
				spy.Process, c.proc = c.proc, spy
				defer func() { c.proc = spy.Process }()
				before := h.notes.Load()
				c.addApproval(ApprovalRequest{ID: "spy-call", Tool: "run_command"}, false)
				notes, published, answered = h.notes.Load()-before, len(c.approvals), c.record().Answered
			})
			if tc.bypass {
				if published != 0 || notes != 0 {
					t.Fatalf("bypass published the request: %d pending, %d page notifications", published, notes)
				}
				if len(spy.answered) != 1 || spy.answered[0].ID != "spy-call" || !spy.allowed[0] || !slices.Contains(answered, "spy-call") {
					t.Fatalf("bypass answer: %+v %v answered=%v", spy.answered, spy.allowed, answered)
				}
				return
			}
			if published != 1 || notes == 0 || len(spy.answered) != 0 {
				t.Fatalf("%s did not publish the request: %d pending, %d notifications, %d answers", tc.mode, published, notes, len(spy.answered))
			}
		})
	}
}
