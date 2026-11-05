package mcp

import (
	"strings"

	"github.com/kang-sw/devenv/internal/wsconfig"
)

// Knob-off lead-ticket text for ticket_review_auto_ack. The shipped rsrc text
// states the default-on rule (the lead confirms review-raised announced and
// technical items on its own judgment); when the knob resolves off, the
// playbook.read render substitutes the acknowledgement rule below, so the rsrc
// mirrors stay byte-identical. Each pair names the shipped text it replaces;
// when a prompt override or a later edit removed that text, the knob-off text
// is appended instead of lost (replaceOrAppend, the whiteboard precedent).
// A test pins every on-text to occur exactly once in the rendered playbook.

const ticketReviewAutoAckOnValue = "on"

type textVariant struct {
	on  string
	off string
}

var ticketReviewAutoAckVariants = []textVariant{
	{
		on: "The settle point is reached when the announced defaults and\n" +
			"technical decisions are acknowledged or auto-confirmed (see\n" +
			"`### Item classes`) and every policy question is confirmed, rejected, or\n" +
			"explicitly deferred. At the settle point, persist without showing the\n" +
			"confirmed set again for approval: the acknowledgement already showed every\n" +
			"item it confirmed, the settlement report lists every auto-confirmed item, and\n" +
			"the trace below and the Sage reviewers guard against interacting decisions.",
		off: "The settle point is reached when the announced defaults and\n" +
			"technical decisions are acknowledged and every policy question is confirmed,\n" +
			"rejected, or explicitly deferred. At the settle point, persist\n" +
			"without showing the confirmed set again for approval: the acknowledgement\n" +
			"already showed every item, and the trace below and the Sage reviewers guard\n" +
			"against interacting decisions.",
	},
	{
		on: "- Announced or technical items raised later in the same settlement need\n" +
			"  their own acknowledgement, except review-raised ones: an announced or\n" +
			"  technical item that comes from a fact-populator `decision_gaps:` item or\n" +
			"  from a reviewer issue marked `autonomous` is confirmed on your own\n" +
			"  judgment, with no acknowledgement round, and reported as auto-confirmed.\n" +
			"  A reviewer issue marked `missing` gets a user round whatever class you\n" +
			"  give it, because it is what blocks the promotion and the user sees what\n" +
			"  cleared the block. A trace knock-on of a review-raised item needs its own\n" +
			"  acknowledgement unless the reviewer issue itself named it.",
		off: "- Announced or technical items raised later in the same settlement (a trace\n" +
			"  knock-on, a new fact-populator gap) need their own acknowledgement.",
	},
	{
		on: "   Send each `decision_gaps:` item to the Open Decision Queue rather than\n" +
			"   resolving it from the populator's evidence alone; `### Item classes` says\n" +
			"   which of them you confirm without asking.",
		off: "   Send each `decision_gaps:` item to the Open Decision Queue; never resolve\n" +
			"   one from the populator's evidence alone.",
	},
	{
		on: "- Any Open Decision Queue item the user has not settled, including an\n" +
			"  announced default or technical decision not yet acknowledged, apart from\n" +
			"  one auto-confirmed under `### Item classes`.",
		off: "- Any Open Decision Queue item the user has not settled, including an\n" +
			"  announced default or technical decision not yet acknowledged.",
	},
	{
		on: "- Report the items settled this round in one line, such as\n" +
			"  `(01), (02) confirmed · (04) deferred`; items settled in earlier rounds are not\n" +
			"  repeated. This per-round settlement report is not the announced or\n" +
			"  technical group. It also lists each item you auto-confirmed this round\n" +
			"  with its one-line decision, since the user has not seen it:\n" +
			"  `(05) auto-confirmed - <one-line decision>`. The report may first reach the\n" +
			"  user in the turn that moves the ticket to `ready/`.",
		off: "- Report the items settled this round in one line, such as\n" +
			"  `(01), (02) confirmed · (04) deferred`; items settled in earlier rounds are not\n" +
			"  repeated. This per-round settlement report is not the announced or\n" +
			"  technical group.",
	},
}

// applyTicketReviewAutoAckVariant returns body with the knob-off text
// substituted when name is lead-ticket and enabled is false; otherwise body
// is returned unchanged.
func applyTicketReviewAutoAckVariant(name, body string, enabled bool) string {
	if name != "lead-ticket" || enabled {
		return body
	}
	for _, v := range ticketReviewAutoAckVariants {
		body = replaceOrAppend(body, v.on, v.off)
	}
	return body
}

// ticketReviewAutoAckEnabled resolves ticket_review_auto_ack for sessionKey
// through the session resolver, so a session, project, or repo-scope value
// reaches the render; a keyless call drops the session and repo layers.
func (s *Server) ticketReviewAutoAckEnabled(sessionKey string) (bool, error) {
	resolver := s.sessionResolver(sessionKey, builtinConfigDefaults())
	rv, err := resolver.Get(sessionKey, wsconfig.ItemTicketReviewAutoAck)
	if err != nil {
		return false, err
	}
	return strings.ToLower(strings.TrimSpace(rv.Value)) == ticketReviewAutoAckOnValue, nil
}
