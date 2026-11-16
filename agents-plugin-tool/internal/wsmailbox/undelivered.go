package wsmailbox

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kang-sw/devenv/internal/wsstate"
)

var errNoUndelivered = errors.New("no undelivered mail")

// UndeliveredRetryInterval retains the former failed-delivery retry cadence.
const UndeliveredRetryInterval = 30 * time.Second

// QueueStore is one authoritative queue file. An empty scope names the reply
// registry; other scopes are the existing named mailbox tiers.
type QueueStore struct {
	Path  string
	Scope Scope
}

// UndeliveredSweep resolves its authority once. It keeps no queue contents:
// every due/changed store is rechecked under its existing writer lock.
type UndeliveredSweep struct {
	reply, machine, cache string
}

func NewUndeliveredSweep(p Paths) (*UndeliveredSweep, error) {
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
	return &UndeliveredSweep{reply: reply, machine: machine, cache: cache}, nil
}

// Stores is startup/safety discovery, including queues whose sessions exited.
// Fixed absent paths are harmless: Sweep never creates an absent source store.
func (s *UndeliveredSweep) Stores() ([]QueueStore, error) {
	stores := []QueueStore{{Path: s.reply}, {Path: s.machine, Scope: ScopeMachine}}
	for _, tier := range []struct {
		pattern string
		scope   Scope
	}{
		{filepath.Join(s.cache, "proj", "*", storeFileName), ScopeWorktree},
		{filepath.Join(s.cache, "proj", "*", "shared", storeFileName), ScopeClone},
	} {
		paths, err := filepath.Glob(tier.pattern)
		if err != nil {
			return nil, err
		}
		for _, path := range paths {
			stores = append(stores, QueueStore{Path: path, Scope: tier.scope})
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
// deadline. Zero means no pending notice. Receipts and side effects keep the
// same lock ordering as the former whole-cache sweep.
func (s *UndeliveredSweep) Sweep(target QueueStore, now time.Time, threshold time.Duration) (posted int, next time.Time, err error) {
	valid, ok := s.Target(target.Path)
	if !ok || valid != target {
		return 0, time.Time{}, fmt.Errorf("undelivered mail: store is outside this sweep")
	}
	if _, err := os.Stat(target.Path); err != nil {
		if os.IsNotExist(err) {
			return 0, time.Time{}, nil
		}
		return 0, time.Time{}, err
	}
	postNamed := func(path, name, content string) error {
		return WithLock(path, func(store *StoreFile) error {
			store.Queues = postSystem(store.Queues, name, content, now)
			return nil
		})
	}
	var errs []error
	record := func(e error) {
		if e != nil && !errors.Is(e, errNoUndelivered) {
			errs = append(errs, e)
		}
	}
	if target.Scope == "" {
		record(WithReplyLock(target.Path, func(store *ReplyStore) error {
			err := sweepQueues(store.Queues, now, threshold, func(name string) string { return ReplyIDPrefix + name }, func(sender Address, content string) error {
				if sender.Kind == AddressReplyID {
					store.Queues = postSystem(store.Queues, sender.ReplyID, content, now)
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
			next = nextUndelivered(store.Queues, now, threshold)
			return err
		}))
	} else {
		record(WithLock(target.Path, func(store *StoreFile) error {
			err := sweepQueues(store.Queues, now, threshold, func(name string) string { return name + "@" + string(target.Scope) }, func(sender Address, content string) error {
				if sender.Kind == AddressReplyID {
					if err := PostSystem(s.reply, sender.ReplyID, "", content, now); err != nil {
						return err
					}
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
	}
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
