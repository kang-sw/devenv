package mcp

import (
	"strings"

	"github.com/kang-sw/devenv/internal/wsconfig"
)

// Knob-off text for ticket_review_auto_ack. The shipped rsrc text states the
// default-on rule (ticket-handler confirms review-raised announced and
// technical items with no lead round, and lead-ticket relays them); when the
// knob resolves off, playbook.read and playbook.render substitute the text
// below, so the rsrc mirrors stay byte-identical. Each pair names the shipped
// text it replaces; when a prompt override or a later edit removed that text,
// the knob-off text is appended instead of lost (replaceOrAppend, the
// whiteboard precedent). A test pins every on-text to occur exactly once in
// its shipped playbook.

const ticketReviewAutoAckOnValue = "on"

type textVariant struct {
	on  string
	off string
}

// ticketReviewAutoAckVariants maps each playbook the knob shapes to its
// knob-off substitutions.
var ticketReviewAutoAckVariants = map[string][]textVariant{
	"lead-ticket": {
		{
			on: "- Only user-confirmed decisions, plus items the handler auto-confirms and\n" +
				"  reports.",
			off: "- Only user-confirmed decisions.",
		},
		{
			on: "`(01), (02) confirmed · (04) deferred`, plus each handler auto-confirmed\n" +
				"  item: `(05) auto-confirmed - <one-line decision>`.",
			off: "`(01), (02) confirmed · (04) deferred`.",
		},
		{
			on:  "- An unsettled queue item, apart from handler auto-confirmed ones.",
			off: "- An unsettled queue item.",
		},
	},
	"ticket-handler": {
		{
			on: "Persist only what they\n" +
				"  confirm, or what you auto-confirm below.",
			off: "Persist only what they\n" +
				"  confirm.",
		},
		{
			on: "- Auto-confirm, with no lead round: an announced or technical item from a\n" +
				"  populator `decision_gaps:` item or a reviewer issue marked `autonomous`.\n" +
				"  A reviewer `missing` issue gets a lead round whatever its class: it is what\n" +
				"  blocks the promotion. A knock-on edit of a review-raised item gets a lead\n" +
				"  round unless the reviewer issue described that edit.",
			off: "- No auto-confirm: every announced or technical item,\n" +
				"  including one from a populator `decision_gaps:` item or a reviewer issue,\n" +
				"  gets a lead round.",
		},
	},
}

// applyTicketReviewAutoAckVariant returns body with the knob-off text
// substituted when name is a playbook the knob shapes and enabled is false;
// otherwise body is returned unchanged.
func applyTicketReviewAutoAckVariant(name, body string, enabled bool) string {
	if enabled {
		return body
	}
	for _, v := range ticketReviewAutoAckVariants[name] {
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
