package wsagent

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsdashboard"
)

func agyBypassAsk(tool, request string) string {
	return `{"jsonrpc":"2.0","method":"session/request_permission","params":{"sessionId":"agy-s1","toolCall":{"toolCallId":"` + tool + `","name":"run_command","title":"fixture command","args":{"CommandLine":"echo fixture"}},"options":[{"optionId":"yes","kind":"allow_once"},{"optionId":"no","kind":"reject_once"}]},"id":"` + request + `"}`
}

func agyBypassAnswers(h *leadHarness, holder, request string) int {
	return agyOptionAnswers(h, holder, request, "yes")
}

// agyOptionAnswers counts the answers to request that picked option.
func agyOptionAnswers(h *leadHarness, holder, request, option string) int {
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
		if json.Unmarshal([]byte(line), &reply) == nil && reply.ID == request && reply.Result.Outcome.Option == option {
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
	// The bypass answer is tracked in memory and rides the next record
	// write (the holder sequence's): no write of its own per tool call.
	eventually(t, "auto-answer tracked by the next record write", func() bool { return slices.Contains(h.record(id).Answered, "call-1") })
	if _, err := h.runner.AnswerApproval(id, "call-1", true, ""); requestStatus(err) != 409 {
		t.Fatalf("second answer: %v", err)
	}

	// A native report shows as the active mode but leaves the record:
	// bypass holds only with native default on record (agyBypass).
	h.holders.out(holder, `{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"agy-s1","update":{"sessionUpdate":"current_mode_update","currentModeId":"accept-edits"}}}`)
	eventually(t, "native report", func() bool { return h.state(id).ActiveMode == "accept-edits" })
	h.quiesce(id)
	if rec := h.record(id); !rec.AgyBypass || rec.PermissionMode != "default" || h.state(id).PermissionMode != "bypass" {
		t.Fatalf("native mode report moved the bypass record: %+v", rec)
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
	// of another mode; the separate intent survives stopping it.
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
	// The old writer changed the mode without knowing the flag: the stale
	// flag keeps the bytes but no longer widens approval.
	if !restored.AgyBypass || restored.agyBypass() || restored.launchPermissionMode() != "accept-edits" || restored.selectedPermissionMode() != "accept-edits" {
		t.Fatalf("stale bypass flag still allows after an old writer changed the mode: %+v", restored)
	}
	if kept := (Record{Lead: true, Backend: BackendAgy, PermissionMode: "default", AgyBypass: true}); !kept.agyBypass() {
		t.Fatal("bypass stored with native default does not hold")
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
				settled   []string
				disk      [3][]byte
				stamps    [3]time.Time
			)
			c.do(func() {
				spy.Process, c.proc = c.proc, spy
				defer func() { c.proc = spy.Process }()
				// The loop is held here, so only these steps could write
				// the record.
				snap := func(i int) {
					disk[i], _ = os.ReadFile(recordPath(c.dir))
					if info, err := os.Stat(recordPath(c.dir)); err == nil {
						stamps[i] = info.ModTime()
					}
				}
				snap(0)
				before := h.notes.Load()
				c.addApproval(ApprovalRequest{ID: "spy-call", Tool: "run_command"}, false)
				notes, published, answered = h.notes.Load()-before, len(c.approvals), c.record().Answered
				snap(1)
				c.handle(Event{ApprovalDone: "spy-call"})
				settled = c.record().Answered
				snap(2)
			})
			if tc.bypass {
				if published != 0 || notes != 0 {
					t.Fatalf("bypass published the request: %d pending, %d page notifications", published, notes)
				}
				if len(spy.answered) != 1 || spy.answered[0].ID != "spy-call" || !spy.allowed[0] || !slices.Contains(answered, "spy-call") || slices.Contains(settled, "spy-call") {
					t.Fatalf("bypass answer: %+v %v answered=%v settled=%v", spy.answered, spy.allowed, answered, settled)
				}
				// Neither the answer nor its settling rewrote the record.
				for i := 1; i < 3; i++ {
					if len(disk[0]) == 0 || !bytes.Equal(disk[i], disk[0]) || !stamps[i].Equal(stamps[0]) {
						t.Fatalf("bypass answer step %d wrote the record:\n%s", i, disk[i])
					}
				}
				return
			}
			if published != 1 || notes == 0 || len(spy.answered) != 0 {
				t.Fatalf("%s did not publish the request: %d pending, %d notifications, %d answers", tc.mode, published, notes, len(spy.answered))
			}
		})
	}
}

// TestAgyBypassUnanswerableRequestGoesToTheUser: a request agy offers no
// allow option for cannot be auto-allowed. Nothing was written, so bypass
// publishes it as default mode does instead of stopping the lead, and the
// user can still deny it.
func TestAgyBypassUnanswerableRequestGoesToTheUser(t *testing.T) {
	h, _ := newAgyLeadHarness(t)
	h.startAt("http://127.0.0.1:1")
	id := h.lead(BackendAgy, LeadOptions{PermissionMode: "bypass"}).ID
	h.send(id, "work")
	holder := h.holder(id)
	eventually(t, "prompt", func() bool { return len(agyPromptIDs(h, holder)) == 1 })
	h.holders.out(holder, `{"jsonrpc":"2.0","method":"session/request_permission","params":{"sessionId":"agy-s1","toolCall":{"toolCallId":"deny-only","name":"run_command","title":"fixture command"},"options":[{"optionId":"no","kind":"reject_once"}]},"id":"req-deny-only"}`)
	eventually(t, "published for the user", func() bool { return len(h.state(id).Approvals) == 1 })
	h.quiesce(id)
	if st := h.state(id); !st.Process {
		t.Fatalf("an unanswerable bypass request stopped the lead: %+v", st)
	}
	c := h.runner.conv(id)
	var answered []string
	c.do(func() { answered = c.record().Answered })
	if slices.Contains(answered, "deny-only") || agyOptionAnswers(h, holder, "req-deny-only", "no") != 0 {
		t.Fatalf("unwritten answer tracked or written: %v", answered)
	}
	if _, err := h.runner.AnswerApproval(id, "deny-only", false, ""); err != nil {
		t.Fatalf("user denial: %v", err)
	}
	eventually(t, "denial written", func() bool { return agyOptionAnswers(h, holder, "req-deny-only", "no") == 1 })
	if st := h.state(id); !st.Process || len(st.Approvals) != 0 {
		t.Fatalf("after denial: %+v", st)
	}
}

// TestAgyBypassStaleFlagPrompts: a bypass flag on record with a mode other
// than the native default it is stored with (an older writer changed the
// mode) prompts instead of auto-allowing.
func TestAgyBypassStaleFlagPrompts(t *testing.T) {
	h, _ := newAgyLeadHarness(t)
	h.startAt("http://127.0.0.1:1")
	id := h.lead(BackendAgy, LeadOptions{PermissionMode: "default"}).ID
	h.send(id, "work")
	holder := h.holder(id)
	eventually(t, "prompt", func() bool { return len(agyPromptIDs(h, holder)) == 1 })
	h.holders.out(holder, agyBypassAsk("waiting", "req-waiting"))
	eventually(t, "manual approval", func() bool { return len(h.state(id).Approvals) == 1 })
	h.runner.Close()
	rec := h.record(id)
	rec.AgyBypass, rec.PermissionMode = true, "plan"
	if err := writeRecord(wsdashboard.SessionDir(h.dir, id), rec); err != nil {
		t.Fatal(err)
	}
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
	h.holders.out(holder, agyBypassAsk("next", "req-next"))
	eventually(t, "both prompts wait", func() bool { return len(h.state(id).Approvals) == 2 })
	if st := h.state(id); st.PermissionMode != "plan" || agyBypassAnswers(h, holder, "req-waiting") != 0 || agyBypassAnswers(h, holder, "req-next") != 0 {
		t.Fatalf("stale bypass flag auto-allowed: %+v", st)
	}
}

// TestAgyBypassEnableDuringReplay: enabling bypass while the session
// rejoins its process succeeds; the waiting prompt is answered when the
// rejoin ends, not refused mid-replay as a failed settings call.
func TestAgyBypassEnableDuringReplay(t *testing.T) {
	h, _ := newAgyLeadHarness(t)
	h.startAt("http://127.0.0.1:1")
	id := h.lead(BackendAgy, LeadOptions{PermissionMode: "default"}).ID
	h.send(id, "work")
	holder := h.holder(id)
	eventually(t, "prompt", func() bool { return len(agyPromptIDs(h, holder)) == 1 })
	h.holders.out(holder, agyBypassAsk("waiting", "req-waiting"))
	eventually(t, "manual approval", func() bool { return len(h.state(id).Approvals) == 1 })
	h.quiesce(id)
	c := h.runner.conv(id)
	var (
		err     error
		pending int
	)
	c.do(func() {
		c.replaying = true
		err = c.setNativePermissionMode("bypass", true)
		pending = len(c.approvals)
		// The rejoin's end (replayDone) answers what waits.
		c.replaying = false
		c.allowAgyBypassPending()
	})
	if err != nil || pending != 1 {
		t.Fatalf("enable during replay: err=%v pending=%d", err, pending)
	}
	eventually(t, "answered after the rejoin", func() bool { return agyBypassAnswers(h, holder, "req-waiting") == 1 })
	if st := h.state(id); st.PermissionMode != "bypass" || len(st.Approvals) != 0 {
		t.Fatalf("after the rejoin: %+v", st)
	}
}
