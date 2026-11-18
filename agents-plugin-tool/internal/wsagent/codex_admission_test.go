package wsagent

import (
	"os"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsdashboard"
)

func TestCodexLeadDoesNotAdmitTerminalTransfers(t *testing.T) {
	h, fake := newCodexLeadHarness(t)
	h.startAt(codexSyntheticEndpoint)
	before := h.terminalSession("codexterminal", LeadSession{
		Key: "terminal-key", Harness: BackendCodex, Session: "native-thread", Cwd: h.root,
	}, nil)
	if _, err := h.runner.TakeOver(before.ReplyID); err == nil || !strings.Contains(err.Error(), "codex terminal takeover is not supported") {
		t.Fatalf("registered lead profile must not grant terminal adoption: %v", err)
	}
	if h.row(before.ReplyID) != before {
		t.Fatal("refused terminal takeover changed the registration")
	}
	if _, err := os.Stat(recordPath(wsdashboard.SessionDir(h.dir, before.ReplyID))); !os.IsNotExist(err) {
		t.Fatalf("refused terminal takeover wrote a record: %v", err)
	}
	id := h.lead(BackendCodex, LeadOptions{}).ID
	fake.replyID = id
	h.send(id, "one")
	holder := h.holder(id)
	h.holders.out(holder, codexSyntheticEnd("t1", "done", "completed"))
	h.idle(id)
	if _, err := h.runner.HandOff(id); err == nil || !strings.Contains(err.Error(), "codex terminal handoff is not supported") {
		t.Fatalf("hosted Codex handoff must be explicitly unsupported: %v", err)
	}
	if state := h.state(id); !state.Process || !state.Lead || state.ResumeCommand != "" || h.holder(id) != holder || h.holders.opCount("stop") != 0 {
		t.Fatalf("refused handoff must not end or transfer the process: %+v", state)
	}
}
