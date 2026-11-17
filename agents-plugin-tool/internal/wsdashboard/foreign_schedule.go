package wsdashboard

import (
	"sync"
	"time"

	"github.com/kang-sw/devenv/internal/wswake"
)

// Foreign-input reconciliation for refresh stamps (stream_cache.go). A
// refresh tick folds a page's in-memory, clock-driven state every tick: the
// index entries, their liveness, the adapter evidence already read, the
// watched processes, and the lapse of each contact last read
// (Contact.liveAt). Inputs a writer may change without telling the daemon
// are looked at once per foreignSafetyInterval instead, and that look is
// shared by every page: the files a page names (PageContent.Watch), the rail
// layout file, and the mailbox stores behind a session's mail print and
// contacts. An in-daemon mailbox queue write (wswake.Hub.StoreChanged) or a
// nudge renews the mailbox look at the next tick; every other in-daemon
// writer wakes the broker, which re-renders without waiting for a tick.
// Nothing here is persisted: a replacement daemon starts with a fresh look.

// foreignSafetyInterval is the longest a refresh stamp reuses its look at a
// foreign input, so a change no writer reports reaches open pages within it
// plus at most one refresh tick (the look renews on the first tick past it).
const foreignSafetyInterval = 5 * time.Minute

// storeHinter is a mailbox wake hub that publishes changed queue stores
// (*wswake.Hub).
type storeHinter interface {
	SubscribeStores() *wswake.Sub
}

// foreignSchedule is one daemon's shared foreign-input look. A nil schedule
// keeps no look: watched paths revalidate on their own one-second window and
// every mailbox stamp is read again.
type foreignSchedule struct {
	interval time.Duration
	now      func() time.Time
	// hints carries in-daemon mailbox queue changes; nil without a hub.
	hints *wswake.Sub

	mu sync.Mutex
	// filesAt starts the current file look; mailAt the current mailbox look,
	// whose generation mailGen also rises on every hint or nudge.
	filesAt, mailAt time.Time
	mailGen         uint64
	mail            map[string]mailLook
}

type mailLook struct {
	gen      uint64
	stamp    string
	contacts []Contact
}

// mailLookMax bounds the kept mailbox stamps; past it the map starts over.
const mailLookMax = 4096

func newForeignSchedule(interval time.Duration, wake MailWaker) *foreignSchedule {
	if interval <= 0 {
		interval = foreignSafetyInterval
	}
	s := &foreignSchedule{interval: interval, now: time.Now}
	if hub, ok := wake.(storeHinter); ok {
		s.hints = hub.SubscribeStores()
	}
	return s
}

func (s *foreignSchedule) close() {
	if s != nil && s.hints != nil {
		s.hints.Close()
	}
}

func (s *foreignSchedule) due(at, now time.Time) bool {
	return at.IsZero() || now.Before(at) || now.Sub(at) >= s.interval
}

// filesSince is the start of the current file look: a watched path's stamp
// taken at or after it is current, whichever page or render took it. Zero
// (a nil schedule) asks for the one-second revalidation window.
func (s *foreignSchedule) filesSince() time.Time {
	if s == nil {
		return time.Time{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if now := s.now(); s.due(s.filesAt, now) {
		s.filesAt = now
	}
	return s.filesAt
}

// mailChanged renews the mailbox look: a nudge's sender may have written mail.
func (s *foreignSchedule) mailChanged() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.mailGen++
	s.mu.Unlock()
}

// mailStamp returns key's mailbox stamp and the contacts it read, calling
// read only when the mailbox look was renewed (a hint, a nudge, or the
// safety interval) since key's last stamp. A failed read (ok false) is never
// kept, so it retries. The contacts are shared: callers only read them.
func (s *foreignSchedule) mailStamp(key string, read func() (string, []Contact, bool)) (string, []Contact, bool) {
	if s == nil {
		return read()
	}
	s.mu.Lock()
	if now := s.now(); s.due(s.mailAt, now) {
		s.mailAt = now
		s.mailGen++
	}
	if s.hints != nil {
		select {
		case <-s.hints.Stores():
			s.hints.TakeStores()
			s.mailGen++
		default:
		}
	}
	gen := s.mailGen
	if look, ok := s.mail[key]; ok && look.gen == gen {
		s.mu.Unlock()
		return look.stamp, look.contacts, true
	}
	s.mu.Unlock()
	// A hint landing during read raises mailGen past gen, so the next call
	// reads again.
	stamp, contacts, ok := read()
	if !ok {
		return "", nil, false
	}
	s.mu.Lock()
	if _, replaces := s.mail[key]; s.mail == nil || !replaces && len(s.mail) >= mailLookMax {
		s.mail = map[string]mailLook{}
	}
	s.mail[key] = mailLook{gen: gen, stamp: stamp, contacts: contacts}
	s.mu.Unlock()
	return stamp, contacts, true
}
