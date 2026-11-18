package wsagent

import (
	"fmt"
	"slices"
	"sync"
	"testing"

	"github.com/kang-sw/devenv/internal/wslive"
)

// Synthetic holder loss after an earlier daemon persisted a final reply.
// The holder's output frontier alone no longer contains that reply. A new
// mapper must use the durable native identity, not text equality, to avoid
// repeating it while still recovering a different turn with identical text.
func TestCodexLeadSyntheticGapDoesNotRepeatPersistedReply(t *testing.T) {
	for _, missedTurn := range []bool{false, true} {
		t.Run(fmt.Sprintf("new-turn-same-text-%t", missedTurn), func(t *testing.T) {
			h, fake := newCodexLeadHarness(t)
			h.startAt(codexSyntheticEndpoint)
			id := h.lead(BackendCodex, LeadOptions{}).ID
			fake.replyID = id
			h.send(id, "one")
			holder := h.holder(id)
			h.holders.out(holder, codexSyntheticEnd("t1", "same answer", "completed"))
			h.idle(id)
			codexApplied(t, h, id, holder)
			var persistedSource string
			for _, entry := range h.ring(id) {
				if entry.Kind == wslive.RingReply {
					persistedSource = entry.Source
				}
			}
			if persistedSource != "codex/codex-s1/t1/reply-t1" {
				t.Fatalf("reply and native identity must commit together: %q", persistedSource)
			}
			turn := "t1"
			wantInputs := h.envelopes("one")
			wantReplies := []string{"same answer"}
			if missedTurn {
				h.send(id, "two")
				codexApplied(t, h, id, holder)
				turn = "t2"
				wantInputs = h.envelopes("one", "two")
				wantReplies = append(wantReplies, "same answer")
			}
			h.runner.Close()
			h.holders.restart()
			h.holders.out(holder, `{"method":"synthetic/keepalive","params":{"threadId":"codex-s1"}}`)
			h.holders.trim(holder, h.holders.records(holder)-1)

			gate, started := make(chan struct{}), make(chan struct{})
			var releaseOnce sync.Once
			release := func() { releaseOnce.Do(func() { close(gate) }) }
			defer release()
			fake.mu.Lock()
			fake.readGate, fake.readStarted = gate, started
			fake.recovery = jsonLine(map[string]any{"thread": map[string]any{
				"id": "codex-s1", "status": map[string]any{"type": "idle"},
				"turns": []any{map[string]any{"id": turn, "status": "completed", "items": []any{
					map[string]any{"id": "reply-" + turn, "type": "agentMessage", "text": "same answer"},
				}}},
			}})
			fake.mu.Unlock()
			h.startAt(codexSyntheticEndpoint)
			wait(t, "idle recovery after persisted reply left the holder ring", started)
			h.send(id, "follow-up")
			if got := codexUserInputs(h, holder); !slices.Equal(got, wantInputs) {
				t.Fatalf("input must remain held through recovery: %q", got)
			}
			release()
			wantInputs = append(wantInputs, h.envelope("follow-up"))
			eventually(t, "one follow-up after recovery", func() bool {
				return slices.Equal(codexUserInputs(h, holder), wantInputs)
			})
			codexApplied(t, h, id, holder)
			if got := ringTexts(h.ring(id), wslive.RingReply); !slices.Equal(got, wantReplies) {
				t.Fatalf("native identity, not text, deduplicates recovery: got %q want %q", got, wantReplies)
			}
			if h.holders.opCount("spawn") != 1 {
				t.Fatal("gap recovery must not spawn another process")
			}
			next := "t2"
			if missedTurn {
				next = "t3"
			}
			h.holders.out(holder, codexSyntheticEnd(next, "follow-up done", "completed"))
			h.idle(id)
		})
	}
}
