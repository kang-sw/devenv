package mcp

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsagent"
)

// A Claude conversation the daemon launches without the native subagent
// tool (a hosted lead or an agent.spawn child) renders the daemon's agent
// tools as its idioms, as an MCP-only Pi session does; a Claude caller that
// keeps the tool keeps the claude row.

const claudeInitialize = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"clientInfo":{"name":"claude-code","version":"test"}}}`

// TestPlaybookRenderSubagentRemovedClaudeCallerUsesAgentIdioms: the hosted
// Claude lead and an agent.spawn Claude child, both detected as claude,
// render agent.spawn and mailbox.send in place of the Agent tool.
func TestPlaybookRenderSubagentRemovedClaudeCallerUsesAgentIdioms(t *testing.T) {
	f := newPiRecordFixture(t)
	f.record(t, f.leadKey, wsagent.Record{Backend: wsagent.BackendClaude, Lead: true, Profile: wsagent.LeadProfileClaude})
	assertPiMCPOnlyBody(t, renderedBody(t, f.s, f.leadKey, "ticket-worker-elevated", claudeInitialize))

	child, err := NewAgentKeyStore().Mint(f.root)
	if err != nil {
		t.Fatal(err)
	}
	f.record(t, child, wsagent.Record{Backend: wsagent.BackendClaude, Profile: wsagent.AgentWorkerProfile(wsagent.BackendClaude)})
	assertPiMCPOnlyBody(t, renderedBody(t, f.s, child, "ticket-worker-elevated", claudeInitialize))
}

// TestPlaybookRenderClaudeCallerKeepingSubagentsKeepsClaudeRow: a tracker
// support conversation, which keeps its Agent tool, and a terminal Claude
// session with no daemon record render the claude row.
func TestPlaybookRenderClaudeCallerKeepingSubagentsKeepsClaudeRow(t *testing.T) {
	f := newPiRecordFixture(t)
	claude := terminologyForHarness("claude", "ws")["SpawnIdiom"]
	body := renderedBody(t, f.s, f.leadKey, "ticket-worker-elevated", claudeInitialize)
	if !strings.Contains(body, claude) || strings.Contains(body, "agent.spawn") {
		t.Errorf("unrecorded Claude caller body lacks the claude SpawnIdiom or names agent.spawn:\n%s", body)
	}

	support, err := NewAgentKeyStore().Mint(f.root)
	if err != nil {
		t.Fatal(err)
	}
	f.record(t, support, wsagent.Record{Backend: wsagent.BackendClaude, Profile: wsagent.SupportProfileClaude})
	body = renderedBody(t, f.s, support, "ticket-worker-elevated", claudeInitialize)
	if !strings.Contains(body, claude) || strings.Contains(body, "agent.spawn") {
		t.Errorf("tracker support body lacks the claude SpawnIdiom or names agent.spawn:\n%s", body)
	}
}

// TestWorkflowManualSubagentRemovedClaudeLeadDropsClaudeSpawnBinding: the
// hosted Claude lead's workflow manual names agent.spawn for exploration
// and drops the Claude delegate-spawn binding, which a terminal Claude session keeps.
func TestWorkflowManualSubagentRemovedClaudeLeadDropsClaudeSpawnBinding(t *testing.T) {
	manual := func(f *piRecordFixture) string {
		t.Helper()
		var out bytes.Buffer
		if err := f.s.ServeStdio(context.Background(), strings.NewReader(claudeInitialize+"\n"), &out); err != nil {
			t.Fatal(err)
		}
		resp := callToolOnce(t, f.s, 2, "workflow_manual", map[string]any{"session_key": f.leadKey})
		if toolIsError(t, resp) {
			t.Fatalf("workflow_manual returned error: %s", resp)
		}
		return toolText(t, resp)
	}

	terminal := newPiRecordFixture(t)
	body := manual(terminal)
	if !strings.Contains(body, "## Claude delegate spawn") || !strings.Contains(body, "Agent({") {
		t.Fatalf("terminal Claude manual lacks the Claude delegate-spawn binding:\n%s", body)
	}

	hosted := newPiRecordFixture(t)
	hosted.record(t, hosted.leadKey, wsagent.Record{Backend: wsagent.BackendClaude, Lead: true, Profile: wsagent.LeadProfileClaude})
	body = manual(hosted)
	if !strings.Contains(body, piMCPOnlyExplore) {
		t.Errorf("hosted Claude lead manual missing %q:\n%s", piMCPOnlyExplore, body)
	}
	for _, banned := range []string{"## Claude delegate spawn", "Agent({"} {
		if strings.Contains(body, banned) {
			t.Errorf("hosted Claude lead manual names %q:\n%s", banned, body)
		}
	}
}
