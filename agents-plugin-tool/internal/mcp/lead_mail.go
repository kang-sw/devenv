package mcp

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wslive"
	"github.com/kang-sw/devenv/internal/wsmailbox"
	"github.com/kang-sw/devenv/internal/wswake"
)

// LeadMailbox is the ws mailbox of the lead sessions the dashboard daemon
// hosts (internal/wsagent's LeadMail). Such a session runs no mailbox wait
// of its own: the daemon watches the session key's queues, claims what
// arrives through the store operations mailbox.recv uses, and hands each
// message to the session as the text of a user turn. It reads the stores of
// this process's environment, where the daemon keeps the keys it mints and
// the sessions it spawns keep theirs.
type LeadMailbox struct {
	dir    string
	server *Server
	wake   *wswake.Hub

	mu      sync.Mutex
	watches map[*leadMailWatch]struct{}
}

// leadMailWatch is one Watch call. claimed receives after a Drain took its
// key's mail, so the watch looks again at once.
type leadMailWatch struct {
	key     string
	claimed chan struct{}
	done    chan struct{}
	exited  chan struct{}
}

const (
	// leadMailLabel and leadMailRecvTool are the names the head of a
	// delivered batch carries (formatMailBatch). A session's own mailbox
	// wait knows its package's name and its harness's spelling of the recv
	// tool; the daemon serves every package and harness, so it uses the
	// canonical ones.
	leadMailLabel    = "ws mailbox"
	leadMailRecvTool = "mailbox.recv"
)

// NewLeadMailbox returns the mailbox of the hosted lead sessions whose
// registry lives in the daemon state directory dir.
func NewLeadMailbox(dir string) *LeadMailbox {
	// The daemon's environment is that of whichever session's launcher
	// started it. Its mailbox identity and hosted-session variables are
	// that session's, never a hosted lead's, so the claiming server starts
	// without them: a hosted lead is reached through its reply-id queue.
	var env []string
	for _, kv := range os.Environ() {
		if name, _, _ := strings.Cut(kv, "="); strings.HasPrefix(name, "WS_MAILBOX") || strings.HasPrefix(name, "WS_HOSTED_") {
			continue
		}
		env = append(env, kv)
	}
	if env == nil {
		env = []string{}
	}
	return &LeadMailbox{
		dir:     dir,
		server:  NewServerWithOptions(ServerOptions{Env: env}),
		wake:    wswake.Mailbox,
		watches: map[*leadMailWatch]struct{}{},
	}
}

// Watch calls changed whenever the mailbox of key holds mail, until the
// returned stop is called; stop returns once no further call will come.
// While it runs the session has a listening marker owned by this process,
// so the mailbox panel reads it as waiting for mail.
func (m *LeadMailbox) Watch(root, key string, changed func()) (stop func()) {
	w := &leadMailWatch{key: key, claimed: make(chan struct{}, 1), done: make(chan struct{}), exited: make(chan struct{})}
	m.mu.Lock()
	m.watches[w] = struct{}{}
	m.mu.Unlock()
	go m.watch(w, root, changed)
	var once sync.Once
	return func() {
		once.Do(func() {
			close(w.done)
			<-w.exited
			m.mu.Lock()
			delete(m.watches, w)
			m.mu.Unlock()
		})
	}
}

func (m *LeadMailbox) watch(w *leadMailWatch, root string, changed func()) {
	defer close(w.exited)
	paths := m.server.mailboxPaths()
	pid := os.Getpid()
	_ = paths.WriteListeningMarker(wsmailbox.ListeningMarker{SessionKey: w.key, PID: pid, StartedAt: mailboxNowString()})
	// Owner-checked: a wait the session armed itself may have replaced it.
	defer func() { _ = paths.ClearListeningMarker(w.key, pid) }()

	target := wsmailbox.WaitTarget{SessionKey: w.key, Root: root, Paths: paths}
	// pause waits out d; it reports false when the watch stopped.
	pause := func(d time.Duration, early <-chan struct{}) bool {
		t := time.NewTimer(d)
		defer t.Stop()
		select {
		case <-w.done:
			return false
		case <-early:
		case <-t.C:
		}
		return true
	}
	for {
		// The wait only peeks, and returns as soon as the queues hold mail:
		// this process's own mailbox writes and the daemon's nudges wake it,
		// and its poll covers a lost one.
		_, err := wsmailbox.Wait(target, wsmailbox.WaitOptions{Wake: m.wake, Done: w.done})
		if errors.Is(err, wsmailbox.ErrWaitCancelled) {
			return
		}
		if err != nil {
			// An unreadable store: look again later rather than end the
			// session's only mail delivery.
			if !pause(wsmailbox.WakeBackstopPoll, nil) {
				return
			}
			continue
		}
		select {
		case <-w.claimed:
		default:
		}
		changed()
		// The mail stays queued until Drain claims it, and the wait would
		// return over it at once: wait for the claim, and say so again
		// when none came (a failed drain, a signal that folded away).
		if !pause(wsmailbox.WakeBackstopPoll, w.claimed) {
			return
		}
	}
}

// Drain claims every message waiting for key, as mailbox.recv does: the
// messages leave their queues, their senders become the session's
// correspondents, and each is a received entry of its mail history and a
// mail line of its conversation ring. It returns one text per message,
// oldest first, which joined by blank lines read as the one prompt a
// session's own mailbox wait submits for the batch (formatMailBatch).
func (m *LeadMailbox) Drain(root, key string) ([]string, error) {
	entry, found := m.server.sessions.lookup(key)
	if !found {
		return nil, fmt.Errorf("mailbox: unknown session key")
	}
	drained, history, keys, err := m.server.claimMail(key, entry)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	for w := range m.watches {
		if w.key == key {
			select {
			case w.claimed <- struct{}{}:
			default:
			}
		}
	}
	m.mu.Unlock()
	if len(drained) == 0 {
		return nil, nil
	}
	// The daemon serves this session's page whatever its dashboard key
	// reads, so the lines go to its ring directly; the page reads the ring
	// by listing it.
	ringDir := wslive.RingDir(wsdashboard.SessionDir(m.dir, wsmailbox.ReplyID(key)))
	if err := wslive.AppendRing(ringDir, mailRingLines(key, history, keys)); err != nil {
		m.server.debugEvent("dashboard.ring_write_error", map[string]any{"error": err.Error()})
	}
	return formatMailBatch(leadMailLabel, leadMailRecvTool, key, drained), nil
}

// Return puts mail a Drain claimed for key, and the session never read,
// back at the head of the key's own reply-id queue, so the next claim (a
// terminal session's mailbox wait or mailbox.recv, once the daemon let the
// session go) delivers it. texts are Drain's texts, in its order. Each
// goes back as the envelope it was claimed from, as far as its text tells
// (parseMailText); a text that does not read as one goes back whole, from
// no sender. The first claim already recorded the mail's history entry and
// correspondent, so the second claim records each once more.
func (m *LeadMailbox) Return(root, key string, texts []string) error {
	if len(texts) == 0 {
		return nil
	}
	if _, found := m.server.sessions.lookup(key); !found {
		return fmt.Errorf("mailbox: unknown session key")
	}
	path, err := m.server.mailboxPaths().ReplyRegistry()
	if err != nil {
		return err
	}
	returned := make([]wsmailbox.Envelope, 0, len(texts))
	for _, text := range texts {
		returned = append(returned, parseMailText(text))
	}
	replyID := m.server.mailboxReplyID(key)
	if err := wsmailbox.WithReplyLock(path, func(store *wsmailbox.ReplyStore) error {
		if store.Queues == nil {
			store.Queues = map[string][]wsmailbox.Envelope{}
		}
		store.Queues[replyID] = append(returned, store.Queues[replyID]...)
		return nil
	}); err != nil {
		return err
	}
	m.wake.Wake()
	return nil
}

// parseMailText reads one of formatMailBatch's texts back into the envelope
// it was made from: the batch head, when the text opens with it, is
// dropped, and the "mail from <handle> (<sent_at>):" line gives the sender
// and the time. An address goes back as From; a display-only handle goes
// back as Label so it never becomes a correspondent or reply target.
// A text of another shape (a record written by other code) goes back
// whole with no sender and the current time.
func parseMailText(text string) wsmailbox.Envelope {
	body := text
	if strings.HasPrefix(body, "["+leadMailLabel+"] ") {
		if _, rest, ok := strings.Cut(body, "\n\n"); ok {
			body = rest
		}
	}
	whole := wsmailbox.Envelope{Content: text, SentAt: mailboxNowString()}
	head, content, ok := strings.Cut(body, ":\n")
	if !ok || !strings.HasPrefix(head, "mail from ") {
		return whole
	}
	handle := strings.TrimPrefix(head, "mail from ")
	stamp := ""
	if open := strings.LastIndex(handle, " ("); open >= 0 && strings.HasSuffix(handle, ")") {
		handle, stamp = handle[:open], handle[open+2:len(handle)-1]
	}
	if handle == "" || strings.ContainsAny(handle, "\n") {
		return whole
	}
	if stamp == "" {
		stamp = whole.SentAt
	}
	m := wsmailbox.Envelope{Content: content, SentAt: stamp}
	if handle != "unknown" {
		if _, err := wsmailbox.ParseAddress(handle); err == nil {
			m.From = handle
		} else {
			m.Label = handle
		}
	}
	return m
}

// formatMailEnvelope is one message as a session reads it: a "mail from
// <handle> (<sent_at>):" line, the handle being the sender's address or
// else its reply handle or display label, then the body.
func formatMailEnvelope(m wsmailbox.Envelope) string {
	handle := strings.TrimSpace(m.From)
	if handle == "" {
		handle = strings.TrimSpace(m.ReplyTo)
	}
	if handle == "" {
		handle = strings.TrimSpace(m.Label)
	}
	if handle == "" {
		handle = "unknown"
	}
	head := "mail from " + handle
	if stamp := strings.TrimSpace(m.SentAt); stamp != "" {
		head += " (" + stamp + ")"
	}
	return head + ":\n" + m.Content
}

// formatMailBatch is a claimed batch as the texts of its messages, the
// first opening with the batch's head: joined by blank lines they are, byte
// for byte, the prompt the Claude adapter's mailbox wait submits for the
// same batch under the same label and recv tool name (its formatBatch).
// Go, Claude, and Pi envelope heads are pinned to matching literals in
// their test suites; change them together.
func formatMailBatch(label, recvTool, key string, mail []wsmailbox.Envelope) []string {
	texts := make([]string, 0, len(mail))
	for _, m := range mail {
		texts = append(texts, formatMailEnvelope(m))
	}
	if len(texts) > 0 {
		head := fmt.Sprintf("[%s] %d message(s) arrived for session_key %s. They were already received through %s; do not call it again for these.", label, len(mail), key, recvTool)
		texts[0] = head + "\n\n" + texts[0]
	}
	return texts
}
