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

// SweepUndelivered checks every queue in these cache/config homes, including
// scopes whose sessions have exited. It leaves overdue mail queued and posts
// one system notice to its sender. The caller wakes waits when posted > 0.
func (p Paths) SweepUndelivered(now time.Time, threshold time.Duration) (posted int, err error) {
	replyPath, err := p.ReplyRegistry()
	if err != nil {
		return 0, err
	}
	machinePath, err := p.Machine()
	if err != nil {
		return 0, err
	}
	cache, err := wsstate.CacheRoot(p.State)
	if err != nil {
		return 0, err
	}
	type namedStore struct {
		path  string
		scope Scope
	}
	named := []namedStore{{machinePath, ScopeMachine}}
	for _, tier := range []struct {
		pattern string
		scope   Scope
	}{
		{filepath.Join(cache, "proj", "*", storeFileName), ScopeWorktree},
		{filepath.Join(cache, "proj", "*", "shared", storeFileName), ScopeClone},
	} {
		paths, e := filepath.Glob(tier.pattern)
		if e != nil {
			return 0, e
		}
		for _, path := range paths {
			named = append(named, namedStore{path, tier.scope})
		}
	}

	// Only the sweep holds two store locks at once. A claim/send releases
	// its named lock before taking the reply lock. Same-store notices use
	// postSystem directly, rather than reacquiring their own lock.
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
	if _, e := os.Stat(replyPath); e == nil {
		record(WithReplyLock(replyPath, func(store *ReplyStore) error {
			return sweepQueues(store.Queues, now, threshold, func(name string) string { return ReplyIDPrefix + name }, func(sender Address, content string) error {
				if sender.Kind == AddressReplyID {
					store.Queues = postSystem(store.Queues, sender.ReplyID, content, now)
				} else if sender.Scope == ScopeMachine {
					if e := postNamed(machinePath, sender.Name, content); e != nil {
						return e
					}
				} else {
					return fmt.Errorf("undelivered mail: reply queue has non-machine sender")
				}
				posted++
				return nil
			}, record)
		}))
	} else if !os.IsNotExist(e) {
		record(e)
	}
	for _, target := range named {
		if _, e := os.Stat(target.path); os.IsNotExist(e) {
			continue
		} else if e != nil {
			record(e)
			continue
		}
		record(WithLock(target.path, func(store *StoreFile) error {
			return sweepQueues(store.Queues, now, threshold, func(name string) string { return name + "@" + string(target.scope) }, func(sender Address, content string) error {
				if sender.Kind == AddressReplyID {
					if e := PostSystem(replyPath, sender.ReplyID, "", content, now); e != nil {
						return e
					}
				} else if sender.Scope == target.scope {
					store.Queues = postSystem(store.Queues, sender.Name, content, now)
				} else if sender.Scope == ScopeMachine {
					if e := postNamed(machinePath, sender.Name, content); e != nil {
						return e
					}
				} else {
					return fmt.Errorf("undelivered mail: named queue has incompatible sender scope")
				}
				posted++
				return nil
			}, record)
		}))
	}
	return posted, errors.Join(errs...)
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
			// Persist successful notices even if a different destination is
			// unavailable, or the next sweep would send those again too.
			n.message.UndeliveredNotified = false
			failed(err)
		}
	}
	return nil
}
