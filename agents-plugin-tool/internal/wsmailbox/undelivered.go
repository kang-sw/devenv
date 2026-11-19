package wsmailbox

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/kang-sw/devenv/internal/wsstate"
)

var errNoUndelivered = errors.New("no undelivered mail")

// UndeliveredRetryInterval retains the former failed-delivery retry cadence.
const UndeliveredRetryInterval = 30 * time.Second

// QueueStore is one queue store, named by its legacy file path. An empty
// scope names the reply registry; other scopes are the named mailbox tiers.
type QueueStore struct {
	Path  string
	Scope Scope
}

// UndeliveredSweep resolves its authority once and sweeps the memory store.
type UndeliveredSweep struct {
	store                 Store
	reply, machine, cache string
}

func NewUndeliveredSweep(p Paths) (*UndeliveredSweep, error) {
	// The store first: it settles the cache root the paths below resolve
	// under (Paths.Store).
	store := p.Store()
	reply, err := p.ReplyRegistry()
	if err != nil {
		return nil, err
	}
	machine, err := p.Machine()
	if err != nil {
		return nil, err
	}
	cache, err := wsstate.CacheRoot(p.State)
	if err != nil {
		return nil, err
	}
	return &UndeliveredSweep{store: store, reply: filepath.Clean(reply), machine: filepath.Clean(machine), cache: cache}, nil
}

// Stores is startup/safety discovery: the reply registry and every named
// store the memory store holds, including queues whose sessions exited.
func (s *UndeliveredSweep) Stores() ([]QueueStore, error) {
	_, named, err := s.store.QueuePaths()
	if err != nil {
		return nil, err
	}
	stores := []QueueStore{{Path: s.reply}}
	for _, path := range named {
		if target, ok := s.Target(path); ok {
			stores = append(stores, target)
		}
	}
	return stores, nil
}

// Target validates a published path without filesystem probes. A server hosted
// for another cache/config home cannot broaden this daemon's sweep authority.
func (s *UndeliveredSweep) Target(path string) (QueueStore, bool) {
	path = filepath.Clean(path)
	if path == s.reply {
		return QueueStore{Path: path}, true
	}
	if path == s.machine {
		return QueueStore{Path: path, Scope: ScopeMachine}, true
	}
	rel, err := filepath.Rel(s.cache, path)
	if err != nil {
		return QueueStore{}, false
	}
	parts := strings.Split(rel, string(filepath.Separator))
	if len(parts) == 3 && parts[0] == "proj" && parts[2] == storeFileName {
		return QueueStore{Path: path, Scope: ScopeWorktree}, true
	}
	if len(parts) == 4 && parts[0] == "proj" && parts[2] == "shared" && parts[3] == storeFileName {
		return QueueStore{Path: path, Scope: ScopeClone}, true
	}
	return QueueStore{}, false
}

// Sweep handles one changed/due store and returns its earliest remaining
// deadline. Zero means no pending notice. The marks and the notices commit
// in one transaction; a notice that could not be placed leaves its message
// unmarked for the next sweep.
func (s *UndeliveredSweep) Sweep(target QueueStore, now time.Time, threshold time.Duration) (posted int, next time.Time, err error) {
	valid, ok := s.Target(target.Path)
	if !ok || valid != target {
		return 0, time.Time{}, fmt.Errorf("undelivered mail: store is outside this sweep")
	}
	var errs []error
	record := func(e error) {
		if e != nil && !errors.Is(e, errNoUndelivered) {
			errs = append(errs, e)
		}
	}
	record(s.store.Update("sweep", func(tx *Tx) error {
		postNamed := func(path, name, content string) error {
			store, err := tx.Named(path)
			if err != nil {
				return err
			}
			store.Queues = postSystem(store.Queues, name, content, now)
			return nil
		}
		postReply := func(replyID, content string) {
			reply := tx.Reply()
			reply.Queues = postSystem(reply.Queues, replyID, content, now)
		}
		if target.Scope == "" {
			reply := tx.Reply()
			err := sweepQueues(reply.Queues, now, threshold, func(name string) string { return ReplyIDPrefix + name }, func(sender Address, content string) error {
				if sender.Kind == AddressReplyID {
					postReply(sender.ReplyID, content)
				} else if sender.Scope == ScopeMachine {
					if err := postNamed(s.machine, sender.Name, content); err != nil {
						return err
					}
				} else {
					return fmt.Errorf("undelivered mail: reply queue has non-machine sender")
				}
				posted++
				return nil
			}, record)
			next = nextUndelivered(reply.Queues, now, threshold)
			return err
		}
		store, err := tx.Named(target.Path)
		if err != nil {
			return err
		}
		err = sweepQueues(store.Queues, now, threshold, func(name string) string { return name + "@" + string(target.Scope) }, func(sender Address, content string) error {
			if sender.Kind == AddressReplyID {
				postReply(sender.ReplyID, content)
			} else if sender.Scope == target.Scope {
				store.Queues = postSystem(store.Queues, sender.Name, content, now)
			} else if sender.Scope == ScopeMachine {
				if err := postNamed(s.machine, sender.Name, content); err != nil {
					return err
				}
			} else {
				return fmt.Errorf("undelivered mail: named queue has incompatible sender scope")
			}
			posted++
			return nil
		}, record)
		next = nextUndelivered(store.Queues, now, threshold)
		return err
	}))
	return posted, next, errors.Join(errs...)
}

// SweepUndelivered retains the one-shot API for callers needing a full scan.
func (p Paths) SweepUndelivered(now time.Time, threshold time.Duration) (posted int, err error) {
	sweep, err := NewUndeliveredSweep(p)
	if err != nil {
		return 0, err
	}
	stores, err := sweep.Stores()
	if err != nil {
		return 0, err
	}
	var errs []error
	for _, store := range stores {
		n, _, err := sweep.Sweep(store, now, threshold)
		posted += n
		if err != nil {
			errs = append(errs, err)
		}
	}
	return posted, errors.Join(errs...)
}

func nextUndelivered(queues map[string][]Envelope, now time.Time, threshold time.Duration) time.Time {
	var next time.Time
	for _, queue := range queues {
		for _, message := range queue {
			handle := message.From
			if handle == "" {
				handle = message.ReplyTo
			}
			if handle == "" || message.UndeliveredNotified {
				continue
			}
			if _, err := ParseAddress(handle); err != nil {
				continue
			}
			sent, err := time.Parse(time.RFC3339, message.SentAt)
			if err != nil {
				continue
			}
			due := sent.Add(threshold)
			if !due.After(now) {
				due = now.Add(UndeliveredRetryInterval)
			}
			if next.IsZero() || due.Before(next) {
				next = due
			}
		}
	}
	return next
}

// Mark under the source queue lock, then post after scanning: a notice may
// append to the very queue being swept (self-mail), trimming its oldest item.
func sweepQueues(queues map[string][]Envelope, now time.Time, threshold time.Duration, recipient func(string) string, post func(Address, string) error, failed func(error)) error {
	type notice struct {
		sender  Address
		content string
		message *Envelope
	}
	var notices []notice
	for name, queue := range queues {
		for i := range queue {
			m := &queue[i]
			handle := m.From
			if handle == "" {
				handle = m.ReplyTo
			}
			if handle == "" || m.UndeliveredNotified {
				continue
			}
			sent, err := time.Parse(time.RFC3339, m.SentAt)
			if err != nil || now.Sub(sent) < threshold {
				continue
			}
			sender, err := ParseAddress(handle)
			if err != nil {
				continue
			}
			first, _, _ := strings.Cut(m.Content, "\n")
			content := fmt.Sprintf("Message to %s sent %s was not read; the recipient session may have exited. The message remains queued and may still be read.\nFirst line: %s", recipient(name), m.SentAt, first)
			notices = append(notices, notice{sender, content, m})
			m.UndeliveredNotified = true
		}
	}
	if len(notices) == 0 {
		return errNoUndelivered
	}
	for _, n := range notices {
		if err := post(n.sender, n.content); err != nil {
			n.message.UndeliveredNotified = false
			failed(err)
		}
	}
	return nil
}
