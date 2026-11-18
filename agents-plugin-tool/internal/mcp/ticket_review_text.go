package mcp

import (
	"strings"

	"github.com/kang-sw/devenv/internal/wsconfig"
)

const ticketReviewAutoAckOnValue = "on"

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
