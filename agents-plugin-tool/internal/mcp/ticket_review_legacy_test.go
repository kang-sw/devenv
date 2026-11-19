package mcp

import (
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsconfig"
)

// legacyLeadTicketFixture carries two auto-confirm passages verbatim from the
// 0.50.7 lead-ticket resource, which predates the TicketHandler* markers.
const legacyLeadTicketFixture = `---
kind: print
---

# Ticket

The settle point is reached when the announced defaults and
technical decisions are acknowledged or auto-confirmed (see
` + "`### Item classes`" + `) and every policy question is confirmed, rejected, or
explicitly deferred. At the settle point, persist without showing the
confirmed set again for approval: the acknowledgement already showed every
item it confirmed, the settlement report lists every auto-confirmed item, and
the trace below and the Sage reviewers guard against interacting decisions.

- Report the items settled this round in one line, such as
  ` + "`(01), (02) confirmed · (04) deferred`" + `; items settled in earlier rounds are not
  repeated. This per-round settlement report is not the announced or
  technical group. It also lists each item you auto-confirmed this round
  with its one-line decision, since the user has not seen it:
  ` + "`(05) auto-confirmed - <one-line decision>`" + `. The report may first reach the
  user in the turn that moves the ticket to ` + "`ready/`" + `.
`

func TestLegacyLeadTicketHonorsReviewAutoAckKnob(t *testing.T) {
	f := newWhiteboardFixture(t)
	t.Setenv("WS_RSRC_ROOT", buildTestRsrcTree(t, map[string]string{
		"lead-ticket/lead-ticket.md": legacyLeadTicketFixture,
	}))
	read := func() string {
		t.Helper()
		resp := f.call(t, "playbook.read", map[string]any{"name": "lead-ticket"})
		if isToolError(resp) {
			t.Fatalf("playbook.read: %s", resp)
		}
		return toolText(t, resp)
	}
	tune := func(value string) {
		t.Helper()
		resp := f.call(t, "config.tune", map[string]any{"key": wsconfig.ItemTicketReviewAutoAck, "value": value, "scope": "session"})
		if isToolError(resp) {
			t.Fatalf("config.tune %s: %s", value, resp)
		}
	}

	tune("on")
	on := read()
	if !strings.Contains(on, "acknowledged or auto-confirmed") || !strings.Contains(on, "(05) auto-confirmed") {
		t.Fatalf("knob on must keep the legacy auto-confirm text:\n%s", on)
	}

	tune("off")
	off := read()
	if strings.Contains(off, "auto-confirm") {
		t.Fatalf("knob off must strip auto-confirm from a marker-less lead-ticket:\n%s", off)
	}
	if !strings.Contains(off, "technical decisions are acknowledged and every policy question") ||
		!strings.Contains(off, "technical group.") {
		t.Fatalf("knob off must substitute the acknowledgement text:\n%s", off)
	}
}
