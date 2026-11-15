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
// display association, not delivery identity: keep both keys and raw wake
// diagnostics, but suppress the mail row at its original position. History
// lacks the envelope's sent time, so never use claim time or proximity to
// select between repeated/reclaimed records.
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
		entries[j].MailWakeSuppressed = true
	}
}

// Board nudges have no peer or retained ring mail. Read only their known
// single-message envelope; unfamiliar or incomplete diagnostics stay unmatched.
func (e ConvEntry) boardNudge() (reasons, topics []string) {
	if !e.MailWake() || e.Truncated || e.Peer != "" {
		return nil, nil
	}
	head, envelope, ok := strings.Cut(e.Text, "\n\n")
	if !ok || !mailBatchHead.MatchString(head) {
		return nil, nil
	}
	from, body, ok := strings.Cut(envelope, ":\n")
	if !ok {
		return nil, nil
	}
	if from != "mail from unknown" {
		stamp, ok := strings.CutPrefix(from, "mail from unknown (")
		if !ok || !strings.HasSuffix(stamp, ")") {
			return nil, nil
		}
		if _, err := time.Parse(time.RFC3339, strings.TrimSuffix(stamp, ")")); err != nil {
			return nil, nil
		}
	}
	line, rest, ok := strings.Cut(body, "\n")
	reasonText, prefix := strings.CutPrefix(line, "Whiteboard lead nudge (")
	reasonText, suffix := strings.CutSuffix(reasonText, ").")
	if !ok || !prefix || !suffix {
		return nil, nil
	}
	seenReasons := map[string]bool{}
	for _, reason := range strings.Split(reasonText, ", ") {
		switch reason {
		case "overdue", "unblocked", "hourly":
			if !seenReasons[reason] {
				reasons = append(reasons, reason)
				seenReasons[reason] = true
			}
		default:
			return nil, nil
		}
	}
	seenTopics := map[string]bool{}
	for _, line := range strings.Split(rest, "\n") {
		bullet, ok := strings.CutPrefix(line, "- ")
		if !ok || bullet == "none" {
			continue
		}
		label := ""
		if i := strings.Index(bullet, "; owner "); i >= 0 && seenReasons["overdue"] {
			label = bullet[:i]
		} else if i := strings.Index(bullet, ": ready to proceed; "); i >= 0 && seenReasons["unblocked"] {
			label = bullet[:i]
		} else if i := strings.LastIndex(bullet, ": "); i >= 0 && seenReasons["hourly"] {
			label = bullet[:i]
		}
		label = strings.TrimSpace(label)
		if label == "" {
			return nil, nil
		}
		if !seenTopics[label] {
			topics = append(topics, label)
			seenTopics[label] = true
		}
	}
	if len(topics) == 0 {
		return nil, nil
	}
	return reasons, topics
}

func (e ConvEntry) MatchedMailWake() bool { return e.Kind == wslive.RingMail && e.Cause == "mail" }

func (e ConvEntry) MailDeliveryText() string {
	if e.MatchedMailWake() {
		return e.Text
	}
	return e.Body
}
