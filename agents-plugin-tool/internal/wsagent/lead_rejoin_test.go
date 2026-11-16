package wsagent

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wslive"
)

// TestLeadRejoinsItsProcessAfterADaemonRestart: the daemon's end leaves a
// lead's process running under its holder. The next daemon attaches to it
// and rebuilds the conversation from the holder's replay of its whole
// ring: no second process, no entry written twice, the running turn and
// its waiting prompt back and answerable, the lines the harness wrote
// while no daemon ran applied once, and a wait with no prompt shown again.
func TestLeadRejoinsItsProcessAfterADaemonRestart(t *testing.T) {
	shortActionGrace(t)
	h := newLeadHarness(t).start()
	id, holder := h.runLead("one")
	h.holders.out(holder, turnLines("sess-1", "u1", "first")...)
	h.idle(id)
	h.send(id, "two")
	h.holders.out(holder, stateLine(HarnessRunning), initLine("sess-1", "auto"), toolUseLine("u2", "toolu_1"), stateLine(HarnessWaiting), canUseToolLine("req-1", "toolu_1"))
	eventually(t, "the prompt", func() bool { return len(h.state(id).Approvals) == 1 })
	// Mapper control sends are asynchronous. The fake appends each answer
	// under the input lock, so observe the launch's and both inits' asks
	// before capturing the holder's final sequence, including ws-context-2.
	eventually(t, "the automatic control answers to be queued", func() bool {
		inputs := h.holders.inputs(holder, "control_request")
		for _, request := range []string{"ws-commands", "ws-context-1", "ws-context-2"} {
			if !slices.ContainsFunc(inputs, func(line string) bool {
				return strings.Contains(line, `"request_id":"`+request+`"`)
			}) {
				return false
			}
		}
		return true
	})
	// The answers can follow the prompt; wait for their LineEnds too.
	target := uint64(h.holders.records(holder))
	c := h.runner.conv(id)
	eventually(t, "all holder records to be applied before shutdown", func() bool {
		var applied bool
		c.do(func() { applied = c.seq >= target })
		return applied
	})
	ringBefore := h.ring(id)
	inputsBefore := h.holders.inputs(holder, "")

	h.runner.Close()
	rec := h.record(id)
	if rec.HolderID != holder || rec.HolderSeq != uint64(h.holders.records(holder)) || h.holders.opCount("stop") != 0 {
		t.Fatalf("the daemon's end leaves the process running, with the applied sequence recorded: record %+v, %d holder records, holder calls %v", rec, h.holders.records(holder), h.holders.opList())
	}
	h.holders.restart()
	h.start()
	eventually(t, "the running turn and its prompt to be back", func() bool {
		st := h.state(id)
		return st.Process && st.Status == StatusRunning && len(st.Approvals) == 1 && st.Approvals[0].ID == "req-1"
	})
	eventually(t, "the permission wait in the snapshot", func() bool {
		lead := h.snapshot(id).Lead
		return lead.Permission != nil && lead.Permission.ToolName == "Bash"
	})
	// The prompt can be answered once the rejoin is over.
	eventually(t, "the answer to be taken", func() bool {
		_, err := h.runner.AnswerApproval(id, "req-1", true, "")
		if err != nil && requestStatus(err) != 409 {
			t.Fatalf("AnswerApproval: %v", err)
		}
		return err == nil
	})
	if got := h.holders.inputs(holder, "control_response"); len(got) != 1 || !strings.Contains(got[0], `"req-1"`) {
		t.Fatalf("answers written = %q", got)
	}
	if h.holders.opCount("launch") != 1 || h.holders.opCount("spawn") != 1 || h.holders.opCount("stop") != 0 {
		t.Fatalf("a rejoin starts and ends no process: %v", h.holders.opList())
	}
	if got := h.ring(id); !reflect.DeepEqual(got, ringBefore) {
		t.Fatalf("a rejoin writes no conversation entry:\n%+v\nbefore\n%+v", got, ringBefore)
	}
	if got := h.holders.inputs(holder, ""); !slices.Equal(got[:len(inputsBefore)], inputsBefore) || len(got) != len(inputsBefore)+1 {
		t.Fatalf("a rejoin writes nothing to the process but the answer: %q", got)
	}

	// The harness works on while no daemon runs.
	h.runner.Close()
	h.holders.restart()
	h.holders.out(holder, stateLine(HarnessRunning), toolResultLine("toolu_1"), replyLine("u3", "second"), resultLine(), stateLine(HarnessIdle))
	h.start()
	eventually(t, "the lines written meanwhile", func() bool {
		return slices.Equal(ringTexts(h.ring(id), wslive.RingReply), []string{"first", "second"})
	})
	h.send(id, "three")
	eventually(t, "the next turn after the rejoin", func() bool { return slices.Equal(h.userInputs(holder), h.envelopes("one", "two", "three")) })
	if st := h.state(id); len(st.Approvals) != 0 || st.Status != StatusRunning || st.Error != "" {
		t.Fatalf("after the second rejoin: %+v", st)
	}

	// A wait with no prompt to answer: shown, and shown again by the next
	// daemon, until the harness moves on.
	h.holders.out(holder, stateLine(HarnessRunning), toolUseLine("u4", "toolu_2"), stateLine(HarnessWaiting))
	eventually(t, "the unanswerable wait", func() bool { return h.state(id).ActionNeeded != "" })
	logged := strings.Count(h.log.String(), "requires_action")
	h.restart()
	eventually(t, "the unanswerable wait after the rejoin", func() bool {
		st := h.state(id)
		return st.ActionNeeded != "" && st.Status == StatusRunning && len(st.Approvals) == 0
	})
	eventually(t, "the wait in the snapshot", func() bool {
		lead := h.snapshot(id).Lead
		return lead.Permission != nil && lead.Permission.ToolName == actionNeededTool
	})
	if strings.Count(h.log.String(), "requires_action") != logged+1 {
		t.Fatalf("the rejoined wait should be logged once more:\n%s", h.log.String())
	}
	h.holders.out(holder, stateLine(HarnessRunning), replyLine("u5", "third"), resultLine(), stateLine(HarnessIdle))
	h.idle(id)
	eventually(t, "the wait to end", func() bool { return h.state(id).ActionNeeded == "" && h.snapshot(id).Lead.Permission == nil })

	// Three daemons applied the stream; each entry is there once.
	ring := h.ring(id)
	if !slices.Equal(ringTexts(ring, wslive.RingUser), h.envelopes("one", "two", "three")) || !slices.Equal(ringTexts(ring, wslive.RingReply), []string{"first", "second", "third"}) {
		t.Fatalf("ring = %+v", ring)
	}
	if h.holders.opCount("launch") != 1 || h.holders.opCount("spawn") != 1 || h.holders.opCount("stop") != 0 {
		t.Fatalf("holder calls = %v", h.holders.opList())
	}

	// A holder that ended while no daemon ran: the process is over, and
	// the next message starts a new one on the same backend session.
	h.runner.Close()
	h.holders.restart()
	h.holders.vanish(holder)
	h.start()
	eventually(t, "the vanished process to be noticed", func() bool { return !h.state(id).Process && h.record(id).HolderID == "" })
	// processGone clears the process and record before writing the row.
	// Wait for the whole loop step before checking its persisted results.
	h.quiesce(id)
	if st := h.state(id); st.Status != StatusIdle || len(st.Approvals) != 0 {
		t.Fatalf("after the holder vanished: %+v", st)
	}
	if reg := h.row(id); reg.HostPID != 0 {
		t.Fatalf("the row names no process: %+v", reg)
	}
	h.send(id, "four")
	var second string
	eventually(t, "a new holder", func() bool { second = h.record(id).HolderID; return second != "" && second != holder })
	eventually(t, "the message on the new process", func() bool { return slices.Equal(h.userInputs(second), h.envelopes("four")) })
	if session, _ := flagValue(h.holders.spawned(second).Argv, "--resume"); session != "sess-1" {
		t.Fatalf("the new process resumes the session: %v", h.holders.spawned(second).Argv)
	}
}

// TestLeadRejoinOfAWrappedRingHoldsInput: a lead's holder keeps a ring
// larger than the manager's default, and a rejoin whose ring no longer
// holds the turn's start (its first record is past the one the record
// names) takes the turn as running: a message and mail sent meanwhile wait
// for the harness's next turn end, then go once.
func TestLeadRejoinOfAWrappedRingHoldsInput(t *testing.T) {
	h := newLeadHarness(t).start()
	id, holder := h.runLead("one")
	if got := h.holders.launchRing(holder); got != leadRingBytes {
		t.Fatalf("a lead's holder ring = %d bytes, want %d", got, leadRingBytes)
	}
	key := h.record(id).WSKey
	h.holders.out(holder, turnLines("sess-1", "u1", "first")...)
	h.idle(id)
	h.send(id, "two")
	h.holders.out(holder, stateLine(HarnessRunning), initLine("sess-1", "auto"))
	h.sync(id, holder)
	h.runner.Close()
	h.holders.restart()

	// The turn works on while no daemon runs, and its ring drops all but
	// the last record: a tool result, which tells nothing of the turn.
	h.holders.out(holder, toolResultLine("toolu_1"), toolResultLine("toolu_2"))
	h.holders.trim(holder, h.holders.records(holder)-1)
	h.start()
	eventually(t, "the rejoin to take the turn as running", func() bool {
		st := h.state(id)
		return st.Process && st.Status == StatusRunning
	})
	h.send(id, "three")
	h.mail.deliver(key, "From a@x: ping")
	eventually(t, "the mail to be claimed", func() bool { return slices.Equal(h.record(id).Mail, []string{"From a@x: ping"}) })
	h.sync(id, holder)
	if got := h.userInputs(holder); !slices.Equal(got, h.envelopes("one", "two")) || h.state(id).Held != 2 {
		t.Fatalf("input waits for the turn's end: written %q, state %+v", got, h.state(id))
	}
	if n := strings.Count(h.log.String(), "with no turn boundary after them"); n != 1 {
		t.Fatalf("the loss should be logged once, logged %d times:\n%s", n, h.log.String())
	}

	h.holders.out(holder, replyLine("u2", "second"), resultLine(), stateLine(HarnessIdle))
	eventually(t, "the held input as the next turn", func() bool {
		return slices.Equal(h.userInputs(holder), []string{h.envelope("one"), h.envelope("two"), h.envelope("three") + "\n\nFrom a@x: ping"})
	})
	h.sync(id, holder)
	if got := h.userInputs(holder); len(got) != 3 || h.state(id).Held != 0 {
		t.Fatalf("the held input goes once: written %q, state %+v", got, h.state(id))
	}
	if strings.Count(h.log.String(), "left its ring") != 1 {
		t.Fatalf("the loss should be logged once:\n%s", h.log.String())
	}
	if strings.Contains(h.log.String(), "whether a turn runs") || len(h.holders.inputs(holder, "get_state")) != 0 {
		t.Fatalf("a claude lead is not asked whether a turn runs:\n%s", h.log.String())
	}
}

// TestLeadRejoinOfARingThatWrappedWhileAttachedHoldsInput: a long turn
// pushes its own start out of the ring while the daemon is still attached,
// so the next daemon's rejoin finds the ring beginning no later than the
// record after the one the record names: nothing left it unseen, yet the
// replay shows no turn boundary. The rejoin takes the turn as running: a
// message sent meanwhile waits for the harness's next turn end, then goes
// once.
func TestLeadRejoinOfARingThatWrappedWhileAttachedHoldsInput(t *testing.T) {
	h := newLeadHarness(t).start()
	id, holder := h.runLead("one")
	h.holders.out(holder, turnLines("sess-1", "u1", "first")...)
	h.idle(id)
	h.send(id, "two")
	h.holders.out(holder, stateLine(HarnessRunning), initLine("sess-1", "auto"), toolResultLine("toolu_1"), toolResultLine("toolu_2"))
	h.sync(id, holder)
	h.holders.trim(holder, h.holders.records(holder)-1)
	h.runner.Close()
	first, applied := uint64(h.holders.records(holder)), h.record(id).HolderSeq
	if first <= 1 || first > applied+1 {
		t.Fatalf("the ring should begin past 1 and no later than the record after %d, begins at %d", applied, first)
	}
	h.holders.restart()
	h.start()
	eventually(t, "the rejoin to take the turn as running", func() bool {
		st := h.state(id)
		return st.Process && st.Status == StatusRunning
	})
	h.send(id, "three")
	h.sync(id, holder)
	if got := h.userInputs(holder); !slices.Equal(got, h.envelopes("one", "two")) || h.state(id).Held != 1 {
		t.Fatalf("the message waits for the turn's end: written %q, state %+v", got, h.state(id))
	}
	if n := strings.Count(h.log.String(), "with no turn boundary after them"); n != 1 {
		t.Fatalf("the loss should be logged once, logged %d times:\n%s", n, h.log.String())
	}

	h.holders.out(holder, replyLine("u2", "second"), resultLine(), stateLine(HarnessIdle))
	eventually(t, "the held message as the next turn", func() bool {
		return slices.Equal(h.userInputs(holder), h.envelopes("one", "two", "three"))
	})
	h.sync(id, holder)
	if got := h.userInputs(holder); len(got) != 3 || h.state(id).Held != 0 {
		t.Fatalf("the held message goes once: written %q, state %+v", got, h.state(id))
	}
}

// TestLeadRejoinOfAWrappedRingTrustsATurnBoundary: when the records after
// the loss show the turn's end, the rejoin takes the state they show, and a
// message sent after it goes at once.
func TestLeadRejoinOfAWrappedRingTrustsATurnBoundary(t *testing.T) {
	h := newLeadHarness(t).start()
	id, holder := h.runLead("one")
	h.holders.out(holder, stateLine(HarnessRunning), initLine("sess-1", "auto"))
	h.sync(id, holder)
	h.runner.Close()
	h.holders.restart()
	h.holders.out(holder, toolResultLine("toolu_1"), replyLine("u1", "first"), resultLine(), stateLine(HarnessIdle))
	h.holders.trim(holder, h.holders.records(holder)-2)
	h.start()
	eventually(t, "the rejoined session to be idle", func() bool {
		st := h.state(id)
		return st.Process && st.Status == StatusIdle
	})
	h.send(id, "two")
	eventually(t, "the message to go at once", func() bool { return slices.Equal(h.userInputs(holder), h.envelopes("one", "two")) })
	if log := h.log.String(); !strings.Contains(log, "the turn state is the one the records after them show") || strings.Contains(log, "with no turn boundary after them") {
		t.Fatalf("the loss should be logged as covered by a turn boundary:\n%s", log)
	}
}

// TestLeadRejoinGapHoldsInputAndDropsAPartialLine: records that leave the
// ring while the replay streams arrive as a gap. A line in progress before
// it is dropped, not spliced onto the line after it; and the turn boundary
// before the gap (an idle state) does not count, so the turn is taken as
// running until the harness ends one.
func TestLeadRejoinGapHoldsInputAndDropsAPartialLine(t *testing.T) {
	h := newLeadHarness(t).start()
	id, holder := h.runLead("one")
	h.holders.out(holder, turnLines("sess-1", "u1", "first")...)
	h.idle(id)
	h.sync(id, holder)
	h.runner.Close()
	h.holders.restart()

	// While no daemon runs: a line read in two parts, then a turn starts;
	// all of it but the line's first part leaves the ring mid-replay.
	h.holders.outRaw(holder, `{"type":"assistant","uuid":"u9","mess`, true)
	partial := uint64(h.holders.records(holder))
	h.holders.outRaw(holder, `age":{"id":"msg-u9","content":[]}}`+"\n", false)
	h.holders.out(holder, stateLine(HarnessRunning), initLine("sess-1", "auto"), tasksLine(7))
	h.holders.gapOnReplay(holder, partial+1, partial+4)
	h.start()
	eventually(t, "the line after the gap and the turn taken as running", func() bool {
		st := h.state(id)
		return st.Process && st.BackgroundTasks == 7 && st.Status == StatusRunning
	})
	h.send(id, "two")
	h.sync(id, holder)
	if got := h.userInputs(holder); !slices.Equal(got, h.envelopes("one")) || h.state(id).Held != 1 {
		t.Fatalf("the message waits for the turn's end: written %q, state %+v", got, h.state(id))
	}
	if n := strings.Count(h.log.String(), "with no turn boundary after them"); n != 1 {
		t.Fatalf("the loss should be logged once, logged %d times:\n%s", n, h.log.String())
	}
	h.holders.out(holder, replyLine("u2", "second"), resultLine(), stateLine(HarnessIdle))
	eventually(t, "the held message as the next turn", func() bool { return slices.Equal(h.userInputs(holder), h.envelopes("one", "two")) })
}

// piHeldRejoin runs a pi lead through one turn, restarts the daemon while
// the idle lead's subagent widget resends push every turn boundary out of
// the ring, and rejoins: the rejoin takes the turn as running, a message
// sent then is held, and the rejoin asks pi whether a turn runs. It
// returns the get_state ask's request id.
func piHeldRejoin(t *testing.T) (h *leadHarness, id, holder, ask string) {
	t.Helper()
	h = newLeadHarness(t).start()
	id = h.lead(BackendPi, LeadOptions{}).ID
	h.send(id, "one")
	holder = h.holder(id)
	h.holders.out(holder, `{"type":"agent_start"}`,
		`{"type":"message_end","message":{"role":"assistant","content":[{"type":"text","text":"hi"}],"stopReason":"stop"}}`,
		`{"type":"agent_settled"}`)
	h.idle(id)
	h.runner.Close()
	h.holders.restart()
	resend := string(piWidgetLine(piAgentsWidgetKey, `{"kind":"agent","agentId":"child-1","label":"audit","status":"done","caption":"worker"}`))
	h.holders.out(holder, resend, resend, resend)
	h.holders.trim(holder, h.holders.records(holder)-2)
	h.start()
	eventually(t, "the rejoin to take the turn as running", func() bool {
		st := h.state(id)
		return st.Process && st.Status == StatusRunning
	})
	h.send(id, "two")
	eventually(t, "the get_state ask", func() bool { return len(h.holders.inputs(holder, "get_state")) == 1 })
	var d struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(h.holders.inputs(holder, "get_state")[0]), &d); err != nil || d.ID == "" {
		t.Fatalf("get_state ask = %s", h.holders.inputs(holder, "get_state")[0])
	}
	if got := piPrompts(h, holder); !slices.Equal(got, h.envelopes("one")) || h.state(id).Held != 1 {
		t.Fatalf("the message waits for the answer: prompts %q, state %+v", got, h.state(id))
	}
	return h, id, holder, d.ID
}

// piPrompts are the messages of the prompt lines written to the holder's
// child.
func piPrompts(h *leadHarness, holder string) []string {
	var out []string
	for _, line := range h.holders.inputs(holder, "prompt") {
		var d struct {
			Message string `json:"message"`
		}
		_ = json.Unmarshal([]byte(line), &d)
		out = append(out, d.Message)
	}
	return out
}

func piStateAnswer(ask string, streaming bool) string {
	return jsonLine(map[string]any{"type": "response", "id": ask, "command": "get_state", "success": true, "data": map[string]any{"isStreaming": streaming, "isCompacting": false, "messageCount": 2}})
}

// TestPiLeadRejoinOfAWrappedRingAsksWhetherATurnRuns: a pi lead's rejoin
// that cannot tell from its replay whether a turn runs asks pi, and an idle
// answer ends the held turn: the held message goes once.
func TestPiLeadRejoinOfAWrappedRingAsksWhetherATurnRuns(t *testing.T) {
	h, id, holder, ask := piHeldRejoin(t)
	h.holders.out(holder, piStateAnswer(ask, false))
	eventually(t, "the held message as the next turn", func() bool { return slices.Equal(piPrompts(h, holder), h.envelopes("one", "two")) })
	h.holders.out(holder, `{"type":"agent_start"}`, `{"type":"agent_settled"}`)
	h.idle(id)
	if got := piPrompts(h, holder); len(got) != 2 || h.state(id).Held != 0 || len(h.holders.inputs(holder, "get_state")) != 1 {
		t.Fatalf("the held message goes once, and pi is asked once: prompts %q, state %+v", got, h.state(id))
	}
	if !strings.Contains(h.log.String(), "the harness reports no turn running") {
		t.Fatalf("the answer should be logged:\n%s", h.log.String())
	}
}

func TestPiLeadRejoinProbeKeepsNativeRunAcrossCommandAnswers(t *testing.T) {
	for _, tc := range []struct{ name, response, commandErr string }{
		{"handled prompt", `{"type":"response","command":"prompt","success":true,"data":{"disposition":"handled"}}`, ""},
		{"refused prompt", `{"type":"response","command":"prompt","success":false,"error":"Agent is already processing"}`, "prompt refused: Agent is already processing"},
		{"completed compact", `{"type":"response","command":"compact","success":true}`, ""},
		{"refused compact", `{"type":"response","command":"compact","success":false,"error":"busy"}`, "compact refused: busy"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, id, holder, ask := piHeldRejoin(t)
			before := len(h.ring(id))
			// There is no replayed agent_start. Put the command answer next
			// to the probe response so reconciliation must happen in stream
			// order, not when the waiting probe goroutine gets scheduled.
			h.holders.out(holder, piStateAnswer(ask, true), tc.response)
			// Inspect the refusal before sending later assistant output.
			target := uint64(h.holders.records(holder))
			c := h.runner.conv(id)
			eventually(t, "the probe and command answer to be applied", func() bool {
				var applied bool
				c.do(func() { applied = c.seq >= target })
				return applied
			})
			if st := h.state(id); st.Status != StatusRunning || st.Error != tc.commandErr || st.Failure != nil || st.Held != 1 || !slices.Equal(piPrompts(h, holder), h.envelopes("one")) {
				t.Fatalf("a command answer must preserve the recovered run and held input while exposing only its command error: state %+v, prompts %q", st, piPrompts(h, holder))
			}
			if got := len(h.ring(id)); got != before {
				t.Fatalf("a command answer must add no conversation entry: got %d, want %d", got, before)
			}
			h.holders.out(holder,
				`{"type":"message_end","message":{"role":"assistant","content":[{"type":"text","text":"still running after rejoin"}]}}`)
			eventually(t, "continued output after the probe and command answer", func() bool {
				return slices.Contains(ringTexts(h.ring(id), wslive.RingReply), "still running after rejoin")
			})
			if st := h.state(id); st.Status != StatusRunning || st.Held != 1 || !slices.Equal(piPrompts(h, holder), h.envelopes("one")) {
				t.Fatalf("continued output must not release input into the recovered native run: state %+v, prompts %q", st, piPrompts(h, holder))
			}
			h.holders.out(holder, `{"type":"agent_settled"}`)
			eventually(t, "the held message after native settlement", func() bool {
				return slices.Equal(piPrompts(h, holder), h.envelopes("one", "two"))
			})
			h.holders.out(holder, `{"type":"agent_start"}`, `{"type":"agent_settled"}`)
			h.idle(id)
		})
	}
}

func TestPiLeadWrappedReplayCommandAnswerStillProbesNativeRun(t *testing.T) {
	for _, tc := range []struct{ name, response string }{
		{"handled prompt", `{"type":"response","command":"prompt","success":true,"data":{"disposition":"handled"}}`},
		{"refused prompt", `{"type":"response","command":"prompt","success":false,"error":"Agent is already processing"}`},
		{"completed compact", `{"type":"response","command":"compact","success":true}`},
		{"refused compact", `{"type":"response","command":"compact","success":false,"error":"busy"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newLeadHarness(t).start()
			id := h.lead(BackendPi, LeadOptions{}).ID
			h.send(id, "one")
			holder := h.holder(id)
			h.holders.out(holder, `{"type":"agent_start"}`)
			h.send(id, "two")
			eventually(t, "input held for the native run", func() bool { return h.state(id).Held == 1 })
			dropped := h.holders.records(holder)
			h.holders.out(holder, tc.response)
			answerSeq := uint64(h.holders.records(holder))
			prior := h.runner.conv(id)
			eventually(t, "the command answer before restart", func() bool {
				var applied bool
				prior.do(func() { applied = prior.seq >= answerSeq })
				return applied
			})
			h.runner.Close()
			h.holders.restart()
			// Only the already-applied command answer survives, not the
			// active run's start. It cannot establish native settlement.
			h.holders.trim(holder, dropped)
			before := len(h.ring(id))
			h.start()
			eventually(t, "replay to probe rather than submit held input", func() bool {
				return len(h.holders.inputs(holder, "get_state")) > 0 || len(piPrompts(h, holder)) > 1
			})
			asks := h.holders.inputs(holder, "get_state")
			if st := h.state(id); len(asks) != 1 || st.Status != StatusRunning || st.Held != 1 || !slices.Equal(piPrompts(h, holder), h.envelopes("one")) {
				t.Fatalf("a surviving command answer must not establish idle: state %+v, probes %q, prompts %q", st, asks, piPrompts(h, holder))
			}
			var ask struct {
				ID string `json:"id"`
			}
			if err := json.Unmarshal([]byte(asks[0]), &ask); err != nil || ask.ID == "" {
				t.Fatalf("get_state ask = %q", asks[0])
			}
			h.holders.out(holder, piStateAnswer(ask.ID, true))
			target := uint64(h.holders.records(holder))
			c := h.runner.conv(id)
			eventually(t, "the streaming recovery answer to be applied", func() bool {
				var applied bool
				c.do(func() { applied = c.seq >= target })
				return applied
			})
			if st := h.state(id); st.Status != StatusRunning || st.Held != 1 || !slices.Equal(piPrompts(h, holder), h.envelopes("one")) {
				t.Fatalf("a streaming recovery answer must retain input: state %+v, prompts %q", st, piPrompts(h, holder))
			}
			if got := len(h.ring(id)); got != before {
				t.Fatalf("replay and recovery must add no conversation entry: got %d, want %d", got, before)
			}
			h.holders.out(holder, `{"type":"agent_settled"}`)
			eventually(t, "held input after genuine native settlement", func() bool {
				return slices.Equal(piPrompts(h, holder), h.envelopes("one", "two"))
			})
			h.holders.out(holder, `{"type":"agent_start"}`, `{"type":"agent_settled"}`)
			h.idle(id)
		})
	}
}

func TestPiWorkerCompleteReplaySettlesDetachedCommandAnswer(t *testing.T) {
	for _, tc := range []struct{ name, response, status, commandErr string }{
		{"handled prompt", `{"type":"response","command":"prompt","success":true,"data":{"disposition":"handled"}}`, TurnDone, ""},
		{"refused prompt", `{"type":"response","command":"prompt","success":false,"error":"command refused"}`, TurnFailed, "prompt refused: command refused"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, got := newFakePiRecoveryHarness(t)
			worker, err := h.runner.Spawn(SpawnSpec{Profile: PiWorkerProfile, Root: h.root, Prompt: "/audit", Tier: "medium", Parent: &leadParent})
			if err != nil {
				t.Fatal(err)
			}
			holder := h.holder(worker.ID)
			h.quiesce(worker.ID)
			if rec := h.record(worker.ID); !rec.TurnOpen {
				t.Fatalf("the command's turn must be persisted before shutdown: %+v", rec)
			}
			h.runner.Close()
			h.holders.restart()
			// The complete holder history has no native boundary: the
			// extension command finishes while the daemon is detached.
			h.holders.out(holder, tc.response)
			h.start()
			c := h.runner.conv(worker.ID)
			eventually(t, "the complete worker replay", func() bool {
				var done bool
				c.do(func() { done = !c.replaying })
				return done
			})
			rec := h.record(worker.ID)
			if st := h.state(worker.ID); st.Status != StatusIdle || rec.TurnOpen || rec.LastTurn == nil {
				t.Fatalf("a completed command-only worker must settle after complete replay: state %+v, record %+v", st, rec)
			}
			if res := rec.LastTurn.Result; res.Status != tc.status || res.Error != tc.commandErr {
				t.Fatalf("recovered command result = %+v; want %s, %q", res, tc.status, tc.commandErr)
			}
			eventually(t, "one terminal worker report", func() bool { return len(got.list()) == 1 })
			if res := got.list()[0].Result; res.Status != tc.status || res.Error != tc.commandErr {
				t.Fatalf("recovered command report = %+v; want %s, %q", res, tc.status, tc.commandErr)
			}
			h.restart()
			c = h.runner.conv(worker.ID)
			eventually(t, "the settled worker replay", func() bool {
				var done bool
				c.do(func() { done = !c.replaying })
				return done
			})
			if st := h.state(worker.ID); st.Status != StatusIdle || h.record(worker.ID).TurnOpen || len(got.list()) != 1 || !slices.Equal(piPrompts(h, holder), h.envelopes("/audit")) {
				t.Fatalf("replay must not duplicate the result, report, or prompt: state %+v, reports %+v, prompts %q", st, got.list(), piPrompts(h, holder))
			}
			// That same historical answer must not close a later persisted
			// turn whose native start has not arrived yet.
			h.send(worker.ID, "next task")
			eventually(t, "the later turn's prompt", func() bool {
				return slices.Equal(piPrompts(h, holder), h.envelopes("/audit", "next task"))
			})
			h.restart()
			c = h.runner.conv(worker.ID)
			eventually(t, "replay during the later persisted turn", func() bool {
				var done bool
				c.do(func() { done = !c.replaying })
				return done
			})
			if st := h.state(worker.ID); st.Status != StatusRunning || !h.record(worker.ID).TurnOpen || len(got.list()) != 1 || !slices.Equal(piPrompts(h, holder), h.envelopes("/audit", "next task")) {
				t.Fatalf("an old command answer must not settle or resubmit the later turn: state %+v, reports %+v, prompts %q", st, got.list(), piPrompts(h, holder))
			}
			h.holders.out(holder, `{"type":"agent_start"}`, `{"type":"agent_settled"}`)
			h.idle(worker.ID)
			eventually(t, "the later turn's own terminal report", func() bool { return len(got.list()) == 2 })
		})
	}
}

func TestPiLeadCompleteCommandOnlyReplayKeepsIdleCompletion(t *testing.T) {
	h := newLeadHarness(t).start()
	id := h.lead(BackendPi, LeadOptions{}).ID
	h.send(id, "/audit")
	holder := h.holder(id)
	handled := `{"type":"response","command":"prompt","success":true,"data":{"disposition":"handled"}}`
	h.holders.out(holder, handled)
	h.idle(id)
	h.restart()
	h.send(id, "/audit again")
	eventually(t, "a command after complete replay", func() bool {
		return slices.Equal(piPrompts(h, holder), h.envelopes("/audit", "/audit again"))
	})
	h.holders.out(holder, handled)
	h.idle(id)
}

func TestPiLeadRejoinDoesNotRestorePastCommandError(t *testing.T) {
	for _, tc := range []struct{ name, response, commandErr string }{
		{"prompt", `{"type":"response","command":"prompt","success":false,"error":"busy"}`, "prompt refused: busy"},
		{"compact", `{"type":"response","command":"compact","success":false,"error":"busy"}`, "compact refused: busy"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newLeadHarness(t).start()
			id := h.lead(BackendPi, LeadOptions{}).ID
			h.send(id, "one")
			holder := h.holder(id)
			h.holders.out(holder, `{"type":"agent_start"}`, tc.response)
			eventually(t, "the live refusal", func() bool { return h.state(id).Error == tc.commandErr })
			h.holders.out(holder, `{"type":"agent_settled"}`)
			h.idle(id)
			h.holders.out(holder, `{"type":"agent_start"}`,
				`{"type":"message_end","message":{"role":"assistant","content":[{"type":"text","text":"later run"}]}}`)
			eventually(t, "the later native run", func() bool {
				return h.state(id).Status == StatusRunning && slices.Contains(ringTexts(h.ring(id), wslive.RingReply), "later run")
			})
			target := uint64(h.holders.records(holder))
			before := len(h.ring(id))
			h.restart()
			eventually(t, "the later run to be reconstructed", func() bool {
				if st := h.state(id); !st.Process || st.Status != StatusRunning {
					return false
				}
				var replayed bool
				h.runner.conv(id).do(func() {
					c := h.runner.conv(id)
					replayed = !c.replaying && c.seq >= target
				})
				return replayed
			})
			if st := h.state(id); st.Error != "" || st.Failure != nil {
				t.Fatalf("replay must not restore a prior turn's transient command error: %+v", st)
			}
			if got := len(h.ring(id)); got != before {
				t.Fatalf("replay must add no conversation entry: got %d, want %d", got, before)
			}
			h.send(id, "two")
			eventually(t, "input held for the reconstructed run", func() bool { return h.state(id).Held == 1 })
			if got := piPrompts(h, holder); !slices.Equal(got, h.envelopes("one")) {
				t.Fatalf("replay must not admit input into the running turn: %q", got)
			}
			h.holders.out(holder, `{"type":"agent_settled"}`)
			eventually(t, "held input after native settlement", func() bool {
				return slices.Equal(piPrompts(h, holder), h.envelopes("one", "two"))
			})
			h.holders.out(holder, `{"type":"agent_start"}`, `{"type":"agent_settled"}`)
			h.idle(id)
		})
	}
}

// TestPiLeadRejoinProbeIdleAnswerAfterATurnStartKeepsTheHold: a turn that
// starts after the ask (its start may follow pi's answer on the stream)
// overrides an idle answer; the turn's own end releases the held message,
// once.
func TestPiLeadRejoinProbeIdleAnswerAfterATurnStartKeepsTheHold(t *testing.T) {
	h, id, holder, ask := piHeldRejoin(t)
	h.holders.out(holder, `{"type":"agent_start"}`,
		string(piWidgetLine(piAgentsWidgetKey, `{"kind":"agent","agentId":"sync-1","label":"sync","status":"running","caption":"worker","batches":[]}`)))
	// The widget's row shows once the loop handled the turn start before it.
	eventually(t, "the turn start to be handled", func() bool {
		for _, a := range h.snapshot(id).Agents {
			if a.ID == "sync-1" {
				return true
			}
		}
		return false
	})
	h.holders.out(holder, piStateAnswer(ask, false))
	eventually(t, "the answer to be taken", func() bool { return strings.Contains(h.log.String(), "the harness reports") })
	h.runner.conv(id).do(func() {})
	if got := piPrompts(h, holder); !slices.Equal(got, h.envelopes("one")) || h.state(id).Held != 1 || h.state(id).Status != StatusRunning || !strings.Contains(h.log.String(), "but one started since") {
		t.Fatalf("the message stays held: prompts %q, state %+v, log:\n%s", got, h.state(id), h.log.String())
	}
	h.holders.out(holder, `{"type":"agent_settled"}`)
	eventually(t, "the held message at the turn's end", func() bool { return slices.Equal(piPrompts(h, holder), h.envelopes("one", "two")) })
	h.holders.out(holder, `{"type":"agent_start"}`, `{"type":"agent_settled"}`)
	h.idle(id)
	if got := piPrompts(h, holder); len(got) != 2 || h.state(id).Held != 0 {
		t.Fatalf("the held message goes once: prompts %q, state %+v", got, h.state(id))
	}
}

// TestPiLeadRejoinProbeAnswerForAnotherProcessIsDropped: an answer that
// arrives for a process the conversation no longer runs changes nothing;
// the current process's own answer still releases the hold.
func TestPiLeadRejoinProbeAnswerForAnotherProcessIsDropped(t *testing.T) {
	h, id, holder, ask := piHeldRejoin(t)
	c := h.runner.conv(id)
	c.do(func() { c.wrapProbed(&fakeProc{}, false, nil) })
	if got := piPrompts(h, holder); !slices.Equal(got, h.envelopes("one")) || h.state(id).Held != 1 || h.state(id).Status != StatusRunning {
		t.Fatalf("a stale answer keeps the hold: prompts %q, state %+v", got, h.state(id))
	}
	if strings.Contains(h.log.String(), "the harness reports") {
		t.Fatalf("a stale answer is not taken:\n%s", h.log.String())
	}
	h.holders.out(holder, piStateAnswer(ask, false))
	eventually(t, "the held message as the next turn", func() bool { return slices.Equal(piPrompts(h, holder), h.envelopes("one", "two")) })
}

// TestPiLeadRejoinProbeThatSaysRunningOrFailsKeepsTheHold: a running answer
// or a refused ask keeps the message held until pi ends the turn.
func TestPiLeadRejoinProbeThatSaysRunningOrFailsKeepsTheHold(t *testing.T) {
	for name, answer := range map[string]func(ask string) string{
		"running": func(ask string) string { return piStateAnswer(ask, true) },
		"refused": func(ask string) string {
			return jsonLine(map[string]any{"type": "response", "id": ask, "command": "get_state", "success": false, "error": "boom"})
		},
		"no run state": func(ask string) string {
			return jsonLine(map[string]any{"type": "response", "id": ask, "command": "get_state", "success": true, "data": map[string]any{"messageCount": 2}})
		},
	} {
		t.Run(name, func(t *testing.T) {
			h, id, holder, ask := piHeldRejoin(t)
			h.holders.out(holder, answer(ask))
			eventually(t, "the answer to be taken", func() bool {
				log := h.log.String()
				return strings.Contains(log, "the harness reports a turn running") || strings.Contains(log, "ask the harness whether a turn runs")
			})
			// The log line comes first in the loop step that takes the
			// answer; a round trip lets that step finish.
			h.runner.conv(id).do(func() {})
			if got := piPrompts(h, holder); !slices.Equal(got, h.envelopes("one")) || h.state(id).Held != 1 || h.state(id).Status != StatusRunning {
				t.Fatalf("the message stays held: prompts %q, state %+v", got, h.state(id))
			}
			h.holders.out(holder, `{"type":"agent_settled"}`)
			eventually(t, "the held message at the turn's end", func() bool { return slices.Equal(piPrompts(h, holder), h.envelopes("one", "two")) })
		})
	}
}
