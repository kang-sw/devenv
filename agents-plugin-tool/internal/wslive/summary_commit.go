package wslive

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const summaryPendingCap = 64

// SummaryCommits belongs to the receiver process, not a client connection or
// writer slot. It retains exact accepted natural-turn candidates across writer
// replacement. Only a successful rename retires a candidate and wakes readers.
type SummaryCommits struct {
	mu      sync.Mutex
	pending []*frozenSummaryTurn
	// Only IDs, never successful payloads. These also prevent resurrection
	// after ordinary ring pruning within this receiver's lifetime.
	closed  map[string]bool
	failure error
	write   func(string, string, []byte) error
	dir     string
	timer   *time.Timer
	wake    func()
}

type frozenSummaryTurn struct {
	commit, name string
	raw          []byte
}

var summaryStores sync.Map // canonical ring directory -> *SummaryCommits

func SummaryStore(dir string) *SummaryCommits {
	dir = filepath.Clean(dir)
	if store, ok := summaryStores.Load(dir); ok {
		return store.(*SummaryCommits)
	}
	store := &SummaryCommits{closed: map[string]bool{}, write: writeFrozenRing, dir: dir}
	// Immutable ring records, not snapshots, are retained-summary authority.
	if names, err := RingNames(dir); err == nil {
		for _, name := range names {
			if rec, ok := ReadRingEntry(dir, name); ok && rec.Kind == RingTurn && rec.SummaryHistory != nil {
				store.closed[rec.SummaryHistory.Commit] = true
			}
		}
	}
	actual, _ := summaryStores.LoadOrStore(dir, store)
	return actual.(*SummaryCommits)
}

// Available is checked before accepting any optional mutation. A failed disk
// does not discard an accepted candidate: later admission fails explicitly while
// the existing recovery cadence retries the retained candidate.
func (s *SummaryCommits) Available() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failure != nil {
		return fmt.Errorf("reasoning summary storage unavailable: %w", s.failure)
	}
	if len(s.pending) >= summaryPendingCap {
		return fmt.Errorf("reasoning summary pending capacity exhausted")
	}
	return nil
}

func (s *SummaryCommits) Has(commit string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed[commit] {
		return true
	}
	for _, p := range s.pending {
		if p.commit == commit {
			return true
		}
	}
	return false
}

// Register freezes one real RingTurn before the ordinary intake response. Its
// return value is acceptance, not persistence; Retry reports successful renames.
func (s *SummaryCommits) Register(entry RingEntry) error {
	if entry.Kind != RingTurn || entry.SummaryHistory == nil || entry.SummaryHistory.Commit == "" {
		return fmt.Errorf("summary candidate needs a real natural turn")
	}
	raw, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	commit := entry.SummaryHistory.Commit
	if s.closed[commit] {
		return nil
	}
	for _, p := range s.pending {
		if p.commit == commit {
			if bytes.Equal(p.raw, raw) {
				return nil
			}
			return fmt.Errorf("conflicting frozen summary candidate")
		}
	}
	if len(s.pending) >= summaryPendingCap {
		return fmt.Errorf("reasoning summary pending capacity exhausted")
	}
	// The deterministic commit suffix uses the existing ring-name grammar.
	stamp := entry.ringStamp
	if stamp.IsZero() {
		stamp = nextRingStamp()
	}
	name := stamp.Format(ringStampLayout) + "-" + commit + ".json"
	s.pending = append(s.pending, &frozenSummaryTurn{commit: commit, name: name, raw: append([]byte(nil), raw...)})
	s.scheduleLocked()
	return nil
}

// SetWake follows the receiver's current reader owner, not the source client.
func (s *SummaryCommits) SetWake(wake func()) { s.mu.Lock(); s.wake = wake; s.mu.Unlock() }

func (s *SummaryCommits) scheduleLocked() {
	if s.timer != nil || s.dir == "" || len(s.pending) == 0 {
		return
	}
	s.timer = time.AfterFunc(HeartbeatInterval, func() {
		n, _ := s.Retry(s.dir)
		s.mu.Lock()
		s.timer = nil
		wake := s.wake
		s.scheduleLocked()
		s.mu.Unlock()
		if n > 0 && wake != nil {
			wake()
		}
	})
}

// Retry never replays tools, replies, or reducer applications. Calls serialize
// per session; each candidate is retired immediately after its own rename.
func (s *SummaryCommits) Retry(dir string) (int, error) { return s.retry(dir, true) }

// Try attempts new candidates once; after failure only the heartbeat recovery
// timer retries, so ordinary progress cannot create a disk retry storm.
func (s *SummaryCommits) Try(dir string) (int, error) { return s.retry(dir, false) }

func (s *SummaryCommits) retry(dir string, recovering bool) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !recovering && s.failure != nil {
		return 0, s.failure
	}
	n := 0
	for len(s.pending) > 0 {
		p := s.pending[0]
		if err := s.write(dir, p.name, p.raw); err != nil {
			s.failure = err
			return n, err
		}
		s.closed[p.commit] = true
		s.pending = s.pending[1:]
		n++
	}
	s.failure = nil
	if n > 0 {
		trimRing(dir)
	}
	return n, nil
}

func writeFrozenRing(dir, name string, raw []byte) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	path := filepath.Join(dir, name)
	if prior, err := os.ReadFile(path); err == nil {
		if bytes.Equal(prior, raw) {
			return nil
		}
		return fmt.Errorf("immutable summary turn already exists with different bytes")
	} else if !os.IsNotExist(err) {
		return err
	}
	return writeRingRaw(dir, name, raw)
}
