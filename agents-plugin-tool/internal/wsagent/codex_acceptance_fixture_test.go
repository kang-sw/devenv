package wsagent

import (
	"slices"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wslive"
)

// Recorded by TestCodexInstalledAcceptance on macOS arm64, Codex 0.160.0,
// 2026-11-18. This is the real adapter/Runner/Python-holder wire with a
// read-only loopback MCP stand-in, not a synthetic app-server transcript.
func TestCodexMapperRecordedHostedAcceptance(t *testing.T) {
	r := feedCodexFixture(t, "codex-0.160.0-hosted-lead.jsonl")
	starts, ends, compactStarts, compactEnds := 0, 0, 0, 0
	for _, event := range r.events {
		if event.TurnStart {
			starts++
		}
		if event.TurnEnd {
			ends++
			if event.TurnError != "" || event.TurnReason != "" {
				t.Fatalf("recorded acceptance turn: %+v", event)
			}
		}
		if event.Live != nil && event.Live.Kind == wslive.KindCompact {
			switch event.Live.Phase {
			case wslive.CompactStart:
				compactStarts++
			case wslive.CompactEnd:
				compactEnds++
			}
		}
		if event.Harness != nil && (event.Harness.Version != "0.160.0" || len(event.Harness.Demoted) != 0) {
			t.Fatalf("installed profile report: %+v", event.Harness)
		}
	}
	// The idle recovery response ends the recovery hold in reader order;
	// its idempotent summary must not repeat the final answer or compaction.
	if starts != 3 || ends != 4 || compactStarts != 1 || compactEnds != 1 {
		t.Fatalf("starts=%d ends=%d compact starts=%d ends=%d", starts, ends, compactStarts, compactEnds)
	}
	replies := finalReplies(r.events)
	for _, marker := range []string{"ECHO:CODEX-ADAPTER-OK", "ECHO:CODEX-RESUME-OK"} {
		count := 0
		for _, reply := range replies {
			if strings.TrimSpace(reply) == marker {
				count++
			}
		}
		if count != 1 {
			t.Fatalf("%s appears %d times in final replies: %q", marker, count, replies)
		}
	}
	if !slices.ContainsFunc(r.answersValues(), func(answer controlAnswer) bool { return answer.ok }) {
		t.Fatal("no successful recorded control response")
	}
}

func (r *codexRun) answersValues() []controlAnswer {
	values := make([]controlAnswer, 0, len(r.answers))
	for _, answer := range r.answers {
		values = append(values, answer)
	}
	return values
}
