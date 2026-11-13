package wsagent

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsdashboard"
)

func TestPiSendRevivesOnTheSameSessionFile(t *testing.T) {
	for _, mode := range []string{"closed", "closed after restart", "process died after restart"} {
		t.Run(mode, func(t *testing.T) {
			h, got := newFakePiRecoveryHarness(t)
			cwd := filepath.Join(h.root, "work")
			if err := os.MkdirAll(cwd, 0o700); err != nil {
				t.Fatal(err)
			}
			worker, err := h.runner.Spawn(SpawnSpec{Profile: PiWorkerProfile, Root: h.root, Cwd: cwd, Model: "prov/medium", Effort: "high", Label: "resume target", Prompt: "work", Tier: "medium", Parent: &leadParent})
			if err != nil {
				t.Fatal(err)
			}
			holder := h.holder(worker.ID)
			eventually(t, "the initial prompt", func() bool { return len(h.holders.inputs(holder, "prompt")) == 1 })
			h.holders.out(holder, `{"type":"agent_start"}`)
			eventually(t, "persisted worker stream", func() bool { return h.record(worker.ID).HolderSeq > 0 })
			before := h.record(worker.ID)
			first := h.holders.spawned(holder)
			file := piSessionFile(wsdashboard.SessionDir(h.opts.Dir, worker.ID))
			// The fake holder does not execute Pi, so stand in for its file.
			if err := os.WriteFile(file, []byte("persisted conversation\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if strings.HasPrefix(mode, "closed") {
				if _, err := h.runner.CloseConversation(worker.ID); err != nil {
					t.Fatal(err)
				}
				if st := h.state(worker.ID); st.Status != StatusClosed || st.Process {
					t.Fatalf("stopped child = %+v", st)
				}
			} else {
				h.runner.Close()
				h.holders.restart()
				h.holders.vanish(holder)
				h.start()
			}
			if mode == "closed after restart" {
				h.restart()
			}
			children := h.runner.Children(leadParent)
			if len(children) != 1 || children[0].ID != worker.ID {
				t.Fatalf("the same parent still addresses its child: %+v", children)
			}
			if _, err := h.runner.Send(worker.ID, "continue"); err != nil {
				t.Fatal(err)
			}
			var newHolder string
			eventually(t, "a new process on the same record", func() bool {
				newHolder = h.record(worker.ID).HolderID
				return newHolder != "" && newHolder != holder
			})
			eventually(t, "the resumed prompt", func() bool { return len(h.holders.inputs(newHolder, "prompt")) == 1 })
			second := h.holders.spawned(newHolder)
			for _, args := range [][]string{first.Argv, second.Argv} {
				i := slices.Index(args, "--session")
				if i < 0 || i+1 == len(args) || args[i+1] != file {
					t.Fatalf("Pi must reuse --session %s: %v", file, args)
				}
			}
			after := h.record(worker.ID)
			if after.ReplyID != before.ReplyID || after.WSKey != before.WSKey || after.Cwd != before.Cwd || after.Model != before.Model || after.Effort != before.Effort || after.Label != before.Label || after.Tier != before.Tier || *after.Parent != *before.Parent || after.ClosedAt != "" || first.Cwd != second.Cwd {
				t.Fatalf("revival changed identity/settings: before=%+v after=%+v", before, after)
			}
			h.holders.out(newHolder, recoveryPiReply("resumed result")...)
			res, err := h.runner.Wait(context.Background(), worker.ID, 5*time.Second)
			if err != nil || res.Status != TurnDone || res.Message != "resumed result" {
				t.Fatalf("resumed turn = %+v, %v", res, err)
			}
			eventually(t, "the resumed completion report", func() bool {
				for _, rep := range got.lead() {
					if rep.Result.ID == worker.ID && rep.Result.Message == "resumed result" {
						return true
					}
				}
				return false
			})
			if len(h.runner.Children(leadParent)) != 1 {
				t.Fatal("revival duplicated the child record")
			}
		})
	}
}

func TestPiSendCannotReviveDuringParentClose(t *testing.T) {
	h, _ := childHarness(t)
	worker, parentProc, _, child, _ := workerWithChild(t, h)
	file := piSessionFile(wsdashboard.SessionDir(h.dir, child.ID))
	if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("persisted conversation\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// The cascade has closed the child, but is still ending the parent.
	g := parentProc.gateClose(t)
	closed := make(chan error, 1)
	go func() {
		_, err := h.runner.CloseConversation(worker.ID)
		closed <- err
	}()
	g.awaitEntered(t)
	if h.state(child.ID).Status != StatusClosed {
		t.Fatal("the cascade has not closed the descendant")
	}
	// Stand in for a token send authorized just before the close began.
	sent := make(chan error, 1)
	go func() {
		_, err := h.runner.Send(child.ID, "too late")
		sent <- err
	}()
	g.release()
	if err := <-closed; err != nil {
		t.Fatal(err)
	}
	if err := <-sent; err == nil || !strings.Contains(err.Error(), "parent conversation is closed") {
		t.Fatalf("a racing send must not outlive its closed parent: %v", err)
	}
	if h.state(child.ID).Status != StatusClosed || h.backend.startCount() != 2 {
		t.Fatal("the racing send revived a descendant")
	}
}

func TestPiSendRevivalDiscardsAnAbandonedHeldTurn(t *testing.T) {
	h, got := childHarness(t)
	worker, parentProc, _, child, _ := workerWithChild(t, h)
	parentProc.reply("interim held for child")
	eventually(t, "a held done turn", func() bool { return h.record(worker.ID).HeldTurn != nil })
	file := piSessionFile(wsdashboard.SessionDir(h.dir, worker.ID))
	if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("persisted conversation\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := h.runner.CloseConversation(worker.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := h.runner.Send(worker.ID, "new task"); err != nil {
		t.Fatal(err)
	}
	p := h.backend.waitProc(t, 2)
	if text := p.nextSent(t); text != "new task" {
		t.Fatalf("revived turn = %q", text)
	}
	if h.record(worker.ID).HeldTurn != nil || h.state(child.ID).Status != StatusClosed {
		t.Fatal("revival restored an abandoned turn or a closed descendant")
	}
	noLeadReport(t, got, "the abandoned held turn must not report")
	p.reply("new result")
	eventually(t, "one new completion report", func() bool { return len(got.lead()) == 1 })
	if rep := got.lead()[0]; rep.Result.Message != "new result" {
		t.Fatalf("revival reported the abandoned turn: %+v", rep)
	}
}

func TestPiTokenSendRevivesClosedChildAndRefusesMissingFile(t *testing.T) {
	h, _ := childHarness(t)
	_, _, token, child, oldProc := workerWithChild(t, h)
	if code, body := h.tokenCall(token, "stop", map[string]any{"id": child.ID}); code != http.StatusOK {
		t.Fatalf("stop = %d %s", code, body)
	}
	if !oldProc.isClosed() {
		t.Fatal("pi.stop left the process alive")
	}
	code, body := h.tokenCall(token, "send", map[string]any{"id": child.ID, "message": "continue"})
	if code != http.StatusConflict || !strings.Contains(body, "session file is missing") {
		t.Fatalf("missing session = %d %s", code, body)
	}
	if h.state(child.ID).Status != StatusClosed || h.backend.startCount() != 2 {
		t.Fatal("a missing file must leave the record closed and start nothing")
	}
	file := piSessionFile(wsdashboard.SessionDir(h.dir, child.ID))
	if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("persisted conversation\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, body = h.tokenCall(token, "send", map[string]any{"id": child.ID, "message": "continue"})
	var st State
	if code != http.StatusOK || json.Unmarshal([]byte(body), &st) != nil || st.ID != child.ID || st.Status == StatusClosed {
		t.Fatalf("revived child = %d %s", code, body)
	}
	if text := h.backend.waitProc(t, 2).nextSent(t); text != "continue" {
		t.Fatalf("resumed prompt = %q", text)
	}
}
