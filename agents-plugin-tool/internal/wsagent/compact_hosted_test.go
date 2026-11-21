package wsagent

import (
	"slices"
	"strings"
	"testing"
)

func TestCompactionNativeHostedRoles(t *testing.T) {
	for _, role := range []struct {
		backend string
		lead    bool
	}{
		{BackendCodex, true},
		{BackendCodex, false},
		{BackendClaude, false},
	} {
		name := role.backend + "/worker"
		if role.lead {
			name = role.backend + "/lead"
		}
		t.Run(name, func(t *testing.T) {
			h, _ := newCodexLeadHarness(t)
			claude := h.opts.Backends[BackendClaude]
			claude.Catalog = ClaudeCatalog()
			h.opts.Backends[BackendClaude] = claude
			codex := h.opts.Backends[BackendCodex]
			codex.WorkerHTTPMCP = true
			h.opts.Backends[BackendCodex] = codex
			probe := &restoreProbe{state: "fresh"}
			h.opts.RestoreBundle = probe.render
			h.startAt(codexSyntheticEndpoint)
			var id string
			if role.lead {
				id = h.lead(role.backend, LeadOptions{}).ID
				h.send(id, "initial")
			} else {
				h.globalProfile("compact-worker", `{"backend":"`+role.backend+`"}`)
				worker, err := h.runner.Spawn(SpawnSpec{Profile: "compact-worker", Root: h.root, Prompt: "initial"})
				if err != nil {
					t.Fatal(err)
				}
				id = worker.ID
			}
			holder := h.holder(id)
			rec := h.record(id)
			if !role.lead && rec.MCPEnv[EnvAllowedTools] != "compact" {
				t.Fatalf("empty worker profile must attach only compact: %q", rec.MCPEnv[EnvAllowedTools])
			}
			key := rec.WSKey
			userInputs := func() []string { return h.userInputs(holder) }
			compactSent := func() bool { return slices.Contains(userInputs(), "/compact") }
			if role.backend == BackendCodex {
				userInputs = func() []string { return codexUserInputs(h, holder) }
				compactSent = func() bool {
					calls := h.holders.acpInputs(holder, "thread/compact/start")
					return len(calls) == 1 && jsonAt(calls[0], "threadId") == "codex-s1"
				}
			} else {
				h.holders.out(holder, `{"type":"system","subtype":"init","session_id":"worker","permissionMode":"dontAsk","claude_code_version":"2.1.326","slash_commands":["compact"]}`)
			}
			eventually(t, "initial turn dispatched", func() bool { return len(userInputs()) == 1 })
			if got := h.runner.Compact(key, "", nil); got.Status != "queued" {
				t.Fatal(got)
			}
			if compactSent() {
				t.Fatal("compaction dispatched during the initial turn")
			}
			if role.backend == BackendCodex {
				h.holders.out(holder, codexSyntheticEnd("t1", "initial complete", "completed"))
			} else {
				h.holders.out(holder, resultLine())
			}
			eventually(t, "native command dispatched at idle", compactSent)
			c := h.runner.conv(id)
			if role.backend == BackendCodex {
				h.holders.out(holder,
					`{"method":"turn/started","params":{"threadId":"codex-s1","turn":{"id":"compact"}}}`,
					`{"method":"item/started","params":{"threadId":"codex-s1","turnId":"compact","item":{"id":"c","type":"contextCompaction"}}}`,
					`{"method":"item/completed","params":{"threadId":"codex-s1","turnId":"compact","item":{"id":"c","type":"contextCompaction"}}}`,
				)
			} else {
				h.holders.out(holder,
					`{"type":"system","subtype":"status","status":"compacting"}`,
					`{"type":"system","subtype":"status","status":null,"compact_result":"success"}`,
					`{"type":"system","subtype":"compact_boundary","compact_metadata":{"trigger":"manual"}}`,
				)
			}
			eventually(t, "native completion arrived while its turn is open", func() bool {
				var waiting bool
				c.do(func() { waiting = c.turn && c.compactCommand && !c.compacting })
				return waiting
			})
			if calls := probe.snapshot(); len(calls) != 0 {
				t.Fatalf("restored before the compaction turn ended: %+v", calls)
			}
			if role.backend == BackendCodex {
				h.holders.out(holder, codexSyntheticEnd("compact", "", "completed"))
			} else {
				h.holders.out(holder, resultLine())
			}
			eventually(t, "restored continuation dispatched", func() bool {
				return slices.ContainsFunc(userInputs(), func(text string) bool {
					return strings.Contains(text, "restore[fresh]") && strings.Contains(text, "Continue the work")
				})
			})
			if calls := probe.snapshot(); len(calls) != 1 || calls[0].key != key || calls[0].cause != RestoreCompaction {
				t.Fatalf("expected exactly one restore for this session: %+v", calls)
			}
		})
	}
}
