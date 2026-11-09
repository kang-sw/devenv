package wstrackerhost

import (
	"context"
	"encoding/binary"
	"hash/fnv"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"time"
)

// DefaultWatchInterval is the store watch period: how soon a write by
// another process (a lead session's or an agent's ws-mcp) reaches a page.
const DefaultWatchInterval = 2 * time.Second

// StartWatch runs the store watch in this process: every interval it stamps
// each registered store's ledger state (the journal files' sizes and times
// and the repository's branch and push-known refs; no fold, no network) and calls
// onChange when a stamp, or the set of stores, changed since the last tick.
// It re-reads the registry each tick, so a store registered later is
// watched. Only what a write moves is stamped, never a file a read writes
// (host-sync.json, ui-seen.json, push state), so an idle store with an open
// page causes no repeated calls; this process's own writes are seen too,
// and a duplicate call is the caller's to absorb.
//
// onChange is also the callback a ViewWithin reload that outlived its
// reader's wait runs. stop ends the watch and clears the callback.
func (h *Host) StartWatch(interval time.Duration, onChange func()) (stop func()) {
	if interval <= 0 {
		interval = DefaultWatchInterval
	}
	h.mu.Lock()
	h.onChange = onChange
	h.mu.Unlock()
	last := h.watchStamps()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				now := h.watchStamps()
				if !maps.Equal(now, last) {
					last = now
					if onChange != nil {
						onChange()
					}
				}
			}
		}
	}()
	return func() {
		cancel()
		<-done
		h.mu.Lock()
		h.onChange = nil
		h.mu.Unlock()
	}
}

// watchStamps stamps every registered store, by id; an unreadable registry
// stamps none.
func (h *Host) watchStamps() map[string]uint64 {
	stores, err := h.Stores()
	if err != nil {
		return nil
	}
	out := make(map[string]uint64, len(stores))
	for _, e := range stores {
		out[e.ID] = storeStamp(e.Dir)
	}
	return out
}

// storeStamp hashes what a write to the store directory moves: each journal
// (journal.<stream>.jsonl, appended by Append) by size and modification
// time, the repository's branch refs (wsledger's tracker, attachment, and
// wiki refs, under refs/heads/), and the refs/ledger/known/ refs a push
// moves (the view's unpushed list reads them), by content, loose or packed,
// in either ref backend. A missing file stamps as absent.
func storeStamp(dir string) uint64 {
	sum := fnv.New64a()
	field := func(parts ...[]byte) {
		for _, p := range parts {
			var n [8]byte
			binary.LittleEndian.PutUint64(n[:], uint64(len(p)))
			sum.Write(n[:])
			sum.Write(p)
		}
	}
	num := func(v int64) []byte {
		var b [8]byte
		binary.LittleEndian.PutUint64(b[:], uint64(v))
		return b[:]
	}
	journals, _ := filepath.Glob(filepath.Join(dir, "journal.*.jsonl"))
	for _, path := range journals {
		if info, err := os.Stat(path); err == nil {
			field([]byte(filepath.Base(path)), num(info.Size()), num(info.ModTime().UnixNano()))
		}
	}
	repo := filepath.Join(dir, "repo.git")
	for _, name := range []string{"packed-refs", filepath.Join("reftable", "tables.list")} {
		if data, err := os.ReadFile(filepath.Join(repo, name)); err == nil {
			field([]byte(name), data)
		}
	}
	refs := filepath.Join(repo, "refs")
	for _, ns := range []string{"heads", filepath.Join("ledger", "known")} {
		_ = filepath.WalkDir(filepath.Join(refs, ns), func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			if data, err := os.ReadFile(path); err == nil {
				rel, _ := filepath.Rel(refs, path)
				field([]byte(filepath.ToSlash(rel)), data)
			}
			return nil
		})
	}
	return sum.Sum64()
}
