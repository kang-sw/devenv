package wsdashboard

import (
	"crypto/sha256"
	"fmt"
	"regexp"
	"slices"
	"strconv"
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

// Peer pairing deliberately retains mailBatchHead's single-message contract.
var nudgeBatchHead = regexp.MustCompile(`^\[(?:ws|wsflow) mailbox\] ([1-9][0-9]*) message\(s\) arrived for session_key ([^\s.]+)\. They were already received through mailbox\.recv; do not call it again for these\.$`)

// formatMailBatch puts the header on only the first mail; runner emits one
// wake per mail. The count bounds membership, including peer/malformed mails.
// Without that header (for example at a page boundary), do not infer a batch.
func collapseBoardNudges(entries []ConvEntry) {
	remaining, run, source := 0, -1, ""
	for i := range entries {
		e := &entries[i]
		if !e.MailWake() {
			remaining, run = 0, -1
			continue
		}
		head, _, _ := strings.Cut(e.Text, "\n\n")
		if count := nudgeBatchSize(head); count > 0 {
			remaining = count
			run, source = -1, e.Source
		} else if e.Source != source || strings.HasPrefix(e.Text, "[") {
			remaining = 0
		}
		if remaining <= 0 {
			run = -1
			continue
		}
		remaining--
		if _, topics := e.boardNudge(); len(topics) == 0 {
			run = -1
			continue
		}
		if run < 0 {
			run = i
			continue
		}
		first := &entries[run]
		if len(first.BoardNudgeMails) == 0 {
			first.BoardNudgeMails = []string{first.Text}
		}
		first.BoardNudgeMails = append(first.BoardNudgeMails, e.Text)
		e.MailWakeSuppressed = true
	}
}

// Board nudges have no peer or retained ring mail. Each text is one envelope,
// optionally preceded by the batch header; unfamiliar diagnostics stay raw.
func (e ConvEntry) boardNudge() (reasons, topics []string) {
	if !e.MailWake() || e.Truncated || e.Peer != "" {
		return nil, nil
	}
	if len(e.BoardNudgeMails) == 0 {
		return boardNudgeMail(e.Text)
	}
	for _, text := range e.BoardNudgeMails {
		r, t := boardNudgeMail(text)
		for _, reason := range r {
			if !slices.Contains(reasons, reason) {
				reasons = append(reasons, reason)
			}
		}
		for _, topic := range t {
			if !slices.Contains(topics, topic) {
				topics = append(topics, topic)
			}
		}
	}
	return reasons, topics
}

// The state token uses the existing mail-wake replacement path when a window
// grows/shrinks a collapsed run. The same token invalidates its cached HTML.
func (e ConvEntry) BoardNudgeState() string {
	if len(e.BoardNudgeMails) < 2 {
		return ""
	}
	return fmt.Sprintf("nudge-%x", sha256.Sum256([]byte(strings.Join(e.BoardNudgeMails, "\x00"))))
}

func (e ConvEntry) WakeText() string {
	if len(e.BoardNudgeMails) > 1 {
		return strings.Join(e.BoardNudgeMails, "\n\n")
	}
	return e.Text
}

func nudgeBatchSize(head string) int {
	batch := nudgeBatchHead.FindStringSubmatch(head)
	if batch == nil {
		return 0
	}
	count, err := strconv.Atoi(batch[1])
	if err != nil {
		return 0
	}
	return count
}

func boardNudgeMail(text string) (reasons, topics []string) {
	envelope := text
	if head, rest, ok := strings.Cut(text, "\n\n"); ok && nudgeBatchSize(head) > 0 {
		envelope = rest
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
