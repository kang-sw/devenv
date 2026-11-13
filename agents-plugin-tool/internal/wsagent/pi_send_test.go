package wsagent

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsinbox"
)

func waitSteer(t *testing.T, p *fakeProc, n int) string {
	t.Helper()
	var texts []string
	eventually(t, "the native steer", func() bool {
		texts, _ = p.steered()
		return len(texts) == n
	})
	return texts[n-1]
}

func TestPiSendSteersTheRunningTurnAndReportsItOnce(t *testing.T) {
	h, got := childHarness(t)
	w := h.spawnWorker("medium")
	p := h.backend.waitProc(t, 0)
	p.nextSent(t)
	for i, text := range []string{"change direction", "check this too"} {
		if _, err := h.runner.Send(w.ID, text); err != nil {
			t.Fatal(err)
		}
		if steer := waitSteer(t, p, i+1); steer != text {
			t.Fatalf("steer = %q, want %q", steer, text)
		}
		noSend(t, p, "a running-turn send must not prompt")
		p.events <- Event{SteerAnswer: &SteerAnswer{}}
		p.events <- Event{UserTaken: text}
	}
	if res, err := h.runner.Wait(context.Background(), w.ID, 20*time.Millisecond); err != nil || res.Status != TurnRunning {
		t.Fatalf("a wait before settlement = %+v, %v", res, err)
	}
	p.reply("redirected result")
	res, err := h.runner.Wait(context.Background(), w.ID, 5*time.Second)
	if err != nil || res.Status != TurnDone || res.Message != "redirected result" || res.mark != 3 {
		t.Fatalf("the same turn covers both sends = %+v, %v", res, err)
	}
	eventually(t, "one completion report", func() bool { return len(got.lead()) == 1 })
	noSend(t, p, "consumed steers must not leave an empty extra turn")
	if rec := h.record(w.ID); len(rec.Steered) != 0 || len(rec.Reports) != 0 || rec.TurnMark != 3 || rec.TurnOpen {
		t.Fatalf("settled record = %+v", rec)
	}
	if _, clears := p.steered(); clears != 0 {
		t.Fatalf("consumed steers must not be cleared: %d", clears)
	}
	// Idle delivery remains an immediate new prompt on the same process.
	if _, err := h.runner.Send(w.ID, "next task"); err != nil {
		t.Fatal(err)
	}
	if text := p.nextSent(t); text != "next task" {
		t.Fatalf("idle send = %q", text)
	}
	p.reply("next result")
	if res, _ := h.runner.Wait(context.Background(), w.ID, 5*time.Second); res.Message != "next result" || res.mark != 4 {
		t.Fatalf("idle turn = %+v", res)
	}
	eventually(t, "the second completion report", func() bool { return len(got.lead()) == 2 })
}

func TestPiSendSteerTheTurnMissesIsDeliveredOnceAsANewTurn(t *testing.T) {
	for _, mode := range []string{"not taken", "late acknowledgement", "refused"} {
		t.Run(mode, func(t *testing.T) {
			h, got := childHarness(t)
			w := h.spawnWorker("medium")
			p := h.backend.waitProc(t, 0)
			p.nextSent(t)
			if _, err := h.runner.Send(w.ID, "redirect"); err != nil {
				t.Fatal(err)
			}
			waitSteer(t, p, 1)
			answer := &SteerAnswer{}
			if mode == "refused" {
				answer.Refusal = "no running agent"
			}
			if mode != "late acknowledgement" {
				p.events <- Event{SteerAnswer: answer}
			}
			p.reply("before redirect")
			if mode == "late acknowledgement" {
				noSend(t, p, "wait for the steer acknowledgement before a new prompt")
				p.events <- Event{SteerAnswer: answer}
			}
			if text := p.nextSent(t); text != "redirect" {
				t.Fatalf("fallback prompt = %q", text)
			}
			if res, _ := h.runner.Wait(context.Background(), w.ID, 20*time.Millisecond); res.Status != TurnRunning {
				t.Fatalf("a wait must not return the turn that missed the steer: %+v", res)
			}
			noLeadReport(t, got, "a missed steer still needs its own turn")
			p.reply("after redirect")
			if res, _ := h.runner.Wait(context.Background(), w.ID, 5*time.Second); res.Message != "after redirect" || res.mark != 2 {
				t.Fatalf("fallback completion = %+v", res)
			}
			eventually(t, "one final completion report", func() bool { return len(got.lead()) == 1 })
			noSend(t, p, "a fallback must not duplicate the message")
			_, clears := p.steered()
			want := 1
			if mode == "refused" {
				want = 0
			}
			if clears != want {
				t.Fatalf("clear_queue calls = %d, want %d", clears, want)
			}
		})
	}
}

func TestPiSendSteerCoverageSurvivesDaemonRestart(t *testing.T) {
	for _, takenBeforeRestart := range []bool{false, true} {
		name := "pending steer"
		if takenBeforeRestart {
			name = "consumed steer"
		}
		t.Run(name, func(t *testing.T) {
			h, got := newFakePiRecoveryHarness(t)
			worker, holder := spawnRecoveryWorker(t, h)
			if _, err := h.runner.Send(worker.ID, "redirect across restart"); err != nil {
				t.Fatal(err)
			}
			eventually(t, "persisted steer coverage", func() bool {
				rec := h.record(worker.ID)
				return len(rec.Steered) == 1 && rec.SteerMarks[rec.Steered[0].Seq] == 2
			})
			take := func() {
				h.holders.out(holder,
					`{"type":"response","command":"steer","success":true}`,
					`{"type":"message_end","message":{"role":"user","content":"redirect across restart"}}`)
				eventually(t, "the turn covers the steer", func() bool { return h.record(worker.ID).TurnMark == 2 })
			}
			if takenBeforeRestart {
				take()
			}
			h.runner.conv(worker.ID).do(h.runner.conv(worker.ID).persistSeq)
			h.restart()
			c := h.runner.conv(worker.ID)
			eventually(t, "worker replay done", func() bool {
				done := false
				c.do(func() { done = !c.replaying })
				return done
			})
			if !takenBeforeRestart {
				take()
			}
			h.holders.out(holder, recoveryPiReply("redirected after restart")...)
			res, err := h.runner.Wait(context.Background(), worker.ID, 5*time.Second)
			if err != nil || res.Status != TurnDone || res.Message != "redirected after restart" || res.mark != 2 {
				t.Fatalf("recovered completion = %+v, %v", res, err)
			}
			eventually(t, "one completion report after restart", func() bool { return len(got.lead()) == 1 })
			if h.holders.opCount("launch") != 1 || len(h.holders.inputs(holder, "steer")) != 1 || len(h.holders.inputs(holder, "prompt")) != 1 {
				t.Fatal("restart repeated the prompt or steer, or launched a second process")
			}
			if rec := h.record(worker.ID); len(rec.SteerMarks) != 0 || len(rec.Steered) != 0 {
				t.Fatalf("consumed steer metadata remains: %+v", rec)
			}
		})
	}
}

func TestPiTokenSendSteersAChildWithoutPromotingHeldDashboardText(t *testing.T) {
	h, _ := childHarness(t)
	_, _, token, gc, p := workerWithChild(t, h)
	c := h.runner.conv(gc.ID)
	if _, _, err := wsinbox.Send(c.inbox, wsinbox.Entry{Parts: []string{"hold until done"}}, time.Now()); err != nil {
		t.Fatal(err)
	}
	code, body := h.tokenCall(token, "send", map[string]any{"id": gc.ID, "message": "steer now"})
	if code != http.StatusOK {
		t.Fatalf("token send = %d %s", code, body)
	}
	if text := waitSteer(t, p, 1); text != "steer now" {
		t.Fatalf("token steer = %q", text)
	}
	p.events <- Event{SteerAnswer: &SteerAnswer{}}
	p.events <- Event{UserTaken: "steer now"}
	p.reply("steered result")
	if text := p.nextSent(t); text != "hold until done" {
		t.Fatalf("only the dashboard text remains for the next turn: %q", text)
	}
	if texts, _ := p.steered(); len(texts) != 1 || strings.Contains(texts[0], "hold until done") {
		t.Fatalf("unrelated held text was promoted: %q", texts)
	}
}
