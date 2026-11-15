package wsdashboard

import (
	"regexp"
	"strings"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
	"github.com/kang-sw/devenv/internal/wsmailbox"
)

var mailBatchHead = regexp.MustCompile(`^\[(?:ws|wsflow) mailbox\] 1 message\(s\) arrived for session_key ([^\s.]+)\. They were already received through mailbox\.recv; do not call it again for these\.$`)

// Present an unambiguous single-message wake as its stored mail. This is a
// display association, not delivery identity: keep both records and raw wake
// diagnostics. History lacks the envelope's sent time, so never use claim time
// or proximity to select between repeated/reclaimed records.
func linkMailWakes(entries []ConvEntry, self string) {
	matches := make(map[int]int)
	uses := make(map[int]int)
	for i, wake := range entries {
		if !wake.MailWake() || wake.Truncated {
			continue
		}
		head, envelope, ok := strings.Cut(wake.Text, "\n\n")
		if !ok {
			continue
		}
		batch := mailBatchHead.FindStringSubmatch(head)
		if len(batch) != 2 || self == "" {
			continue
		}
		// Live-state scrubbing replaces the real session key in the header
		// with id:<reply-id> before the dashboard ever sees the wake.
		recipient := strings.TrimPrefix(batch[1], wsmailbox.ReplyIDPrefix)
		if !strings.HasPrefix(batch[1], wsmailbox.ReplyIDPrefix) {
			recipient = wsmailbox.ReplyID(batch[1])
		}
		if recipient != self {
			continue
		}
		from, body, ok := strings.Cut(envelope, ":\n")
		if !ok || !strings.HasPrefix(from, "mail from ") {
			continue
		}
		peer := strings.TrimPrefix(from, "mail from ")
		if open := strings.LastIndex(peer, " ("); open >= 0 && strings.HasSuffix(peer, ")") {
			if _, err := time.Parse(time.RFC3339, peer[open+2:len(peer)-1]); err != nil {
				continue
			}
			peer = peer[:open]
		}
		match, count := -1, 0
		for j, mail := range entries {
			if mail.Kind == wslive.RingMail && mail.Direction == wslive.MailRecv && mail.HasBody && mail.Peer == peer && mail.Body == body {
				match, count = j, count+1
			}
		}
		if count == 1 {
			matches[i] = match
			uses[match]++
		}
	}
	for i, j := range matches {
		if uses[j] != 1 {
			continue
		}
		wake, mail := entries[i], entries[j]
		mail.Key, mail.At = wake.Key, wake.At
		mail.Cause, mail.Source, mail.Caption, mail.Text = wake.Cause, wake.Source, wake.Caption, wake.Text
		entries[i] = mail
	}
}

func (e ConvEntry) MatchedMailWake() bool { return e.Kind == wslive.RingMail && e.Cause == "mail" }

func (e ConvEntry) MailDeliveryText() string {
	if e.MatchedMailWake() {
		return e.Text
	}
	return e.Body
}
