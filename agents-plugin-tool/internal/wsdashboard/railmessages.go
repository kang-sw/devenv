package wsdashboard

import (
	"strings"
	"sync"

	"github.com/kang-sw/devenv/internal/wsinbox"
	"github.com/kang-sw/devenv/internal/wslive"
)

// railMessageScan bounds how far back a ring read walks for a user message
// or a lead reply: past it the row keeps the message it last showed (none
// on a first read) rather than reading a whole ring of tool and mail
// entries.
const railMessageScan = 64

// railMessages holds each session's last conversation message for the
// rail, so an unchanged ring is not reread: a session whose index revision
// is the one last read is served from memory, and otherwise its ring's
// names are listed and, while its newest name is the one last read, still
// no entry is read (an entry file is never rewritten, so the newest name
// alone tells whether the ring grew). The daemon drops a session's entry
// when it leaves the index (Daemon.sessionChanged).
type railMessages struct {
	mu       sync.Mutex
	sessions map[string]railMessageEntry
	// reads counts the ring entry files read (a test seam).
	reads int
}

type railMessageEntry struct {
	rev    uint64
	newest string
	msg    railMessage
}

func newRailMessages() *railMessages {
	return &railMessages{sessions: map[string]railMessageEntry{}}
}

// last returns the newest user message or lead reply in the ring at dir,
// session replyID's at index revision rev. Mail, tool, turn, compaction,
// and card entries are skipped; a reply still streaming is not in the ring
// yet, so it shows once finished. A nil cache reads each time.
func (c *railMessages) last(replyID string, rev uint64, dir string) railMessage {
	var known railMessageEntry
	var have bool
	if c != nil {
		c.mu.Lock()
		known, have = c.sessions[replyID]
		c.mu.Unlock()
		if have && rev != 0 && known.rev == rev {
			return known.msg
		}
	}
	names, err := wslive.RingNames(dir)
	if err != nil {
		if have {
			return known.msg
		}
		return railMessage{}
	}
	newest := ""
	if len(names) > 0 {
		newest = names[len(names)-1]
	}
	msg := known.msg
	read := 0
	if !have || newest != known.newest {
		for i := len(names) - 1; i >= 0 && len(names)-i <= railMessageScan; i-- {
			read++
			rec, ok := wslive.ReadRingEntry(dir, names[i])
			if !ok || (rec.Kind != wslive.RingUser && rec.Kind != wslive.RingReply) {
				continue
			}
			text := rec.Text
			if rec.Kind == wslive.RingUser {
				text = withoutImageLines(text)
			}
			msg = railMessage{Text: oneLine(text, railMessageRunes), User: rec.Kind == wslive.RingUser}
			break
		}
	}
	if c != nil {
		c.mu.Lock()
		c.reads += read
		c.sessions[replyID] = railMessageEntry{rev: rev, newest: newest, msg: msg}
		c.mu.Unlock()
	}
	return msg
}

// drop forgets a session.
func (c *railMessages) drop(replyID string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	delete(c.sessions, replyID)
	c.mu.Unlock()
}

// withoutImageLines drops a user message's image lines (the attached
// images' file paths, wsinbox.ParseImageLine): the session page shows them
// as thumbnails, and a one-line row has no room for either.
func withoutImageLines(text string) string {
	if !strings.Contains(text, "[image: ") && !strings.Contains(text, "Image #") {
		return text
	}
	var kept []string
	for _, line := range strings.Split(text, "\n") {
		if _, _, ok := wsinbox.ParseImageLine(strings.TrimSpace(line)); !ok {
			kept = append(kept, line)
		}
	}
	return strings.Join(kept, "\n")
}

// oneLine collapses s's whitespace runs, line breaks included, to single
// spaces and cuts it to at most n runes, marking a cut with an ellipsis.
func oneLine(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}
