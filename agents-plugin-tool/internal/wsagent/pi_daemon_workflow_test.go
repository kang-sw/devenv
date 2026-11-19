package wsagent

import (
	"errors"
	"strings"
	"testing"
)

// A hosted launch failure must retain dashboard input and claimed mail. Missing
// installed plugins are no longer a failure; inject the actual MCP boundary.
func TestPiDaemonWorkflowBackendFailureRetainsInput(t *testing.T) {
	h := newLeadHarness(t)
	h.opts.WSMCP = func(string) (MCPCommand, error) { return MCPCommand{}, errors.New("staged launcher unavailable") }
	h.start()
	id := h.lead(BackendClaude, LeadOptions{}).ID
	h.send(id, "held dashboard input")
	eventually(t, "the launcher error", func() bool {
		st := h.state(id)
		return !st.Process && st.Held == 1 && strings.Contains(st.Error, "staged launcher unavailable")
	})
	key := h.record(id).WSKey
	eventually(t, "the mail watch", func() bool { return h.mail.watched(key) })
	h.mail.deliver(key, "From fixture: retained mail")
	eventually(t, "claimed mail retained", func() bool {
		mail := h.record(id).Mail
		return len(mail) == 1 && mail[0] == "From fixture: retained mail"
	})
	h.quiesce(id)
	if st := h.state(id); st.Process || st.Held != 2 || !strings.Contains(st.Error, "staged launcher unavailable") {
		t.Fatalf("failure must retain input: %+v", st)
	}
	if h.holders.opCount("spawn") != 0 {
		t.Fatal("failed launcher must not spawn a harness")
	}
}
