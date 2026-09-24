package wsindex

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// Pending entry operations. Every one is idempotent on replay, so a crash
// between the remote push and the local clear re-flushes cleanly.
const (
	OpRegister = "register"
	OpAcquire  = "acquire"
	OpRelease  = "release"
	OpClose    = "close"
)

// PendingEntry is one index write recorded while the remote was unreachable.
// The pending log is local (never pushed), so Worktree may name a path; it
// is used only for local reports and never reaches the remote index.
type PendingEntry struct {
	ID         string    `json:"id"`
	Op         string    `json:"op"`
	Stem       string    `json:"stem"`
	Owner      Owner     `json:"owner"`
	RecordedAt time.Time `json:"recorded_at"`
	Worktree   string    `json:"worktree,omitempty"`
	// ImplBranch marks an acquire that records only the worker impl branch.
	ImplBranch string `json:"impl_branch,omitempty"`
	// Override carries the holder the entry overrode in the cached view, with
	// the reason; replay applies it only while that holder still holds.
	Override *Override `json:"override,omitempty"`
}

// Override is the audit payload of a dangerously overridden operation.
type Override struct {
	Holder Owner  `json:"holder"`
	Reason string `json:"reason"`
}

type pendingLog struct {
	Entries []PendingEntry `json:"entries"`
}

// maxLocalCASAttempts bounds every local compare-and-swap loop.
const maxLocalCASAttempts = 64

// readPending returns the pending log and the commit it was read from ("" when
// the log is empty and the ref absent).
func (c *Client) readPending(ctx context.Context) ([]PendingEntry, string, error) {
	oid, ok, err := c.refOID(ctx, PendingRef)
	if err != nil || !ok {
		return nil, "", err
	}
	data, err := c.readBlobAt(ctx, oid, pendingFile)
	if err != nil {
		return nil, "", fmt.Errorf("read pending log: %w", err)
	}
	var log pendingLog
	if err := json.Unmarshal(data, &log); err != nil {
		return nil, "", fmt.Errorf("decode pending log: %w", err)
	}
	return log.Entries, oid, nil
}

func (c *Client) writePendingCommit(ctx context.Context, entries []PendingEntry) (string, error) {
	data, err := json.MarshalIndent(pendingLog{Entries: entries}, "", "  ")
	if err != nil {
		return "", err
	}
	return c.writeSingleFileCommit(ctx, pendingFile, append(data, '\n'), "", "ticket-index pending log\n")
}

// Pending returns this clone's pending entries in recorded order.
func (c *Client) Pending(ctx context.Context) ([]PendingEntry, error) {
	entries, _, err := c.readPending(ctx)
	return entries, err
}

// appendPending appends entry under local CAS, so concurrent worktrees of
// the clone never lose an entry.
func (c *Client) appendPending(ctx context.Context, entry PendingEntry) (PendingEntry, error) {
	if entry.ID == "" {
		entry.ID = randomHex(8)
	}
	if entry.RecordedAt.IsZero() {
		entry.RecordedAt = c.now().UTC()
	}
	for attempt := 0; attempt < maxLocalCASAttempts; attempt++ {
		entries, oid, err := c.readPending(ctx)
		if err != nil {
			return entry, err
		}
		next, err := c.writePendingCommit(ctx, append(append([]PendingEntry{}, entries...), entry))
		if err != nil {
			return entry, err
		}
		ok, err := c.casRef(ctx, PendingRef, next, oid)
		if err != nil {
			return entry, err
		}
		if ok {
			return entry, nil
		}
	}
	return entry, fmt.Errorf("append pending ticket-index entry: local compare-and-swap kept losing")
}

// removePending removes exactly the entries whose ids are in ids, keeping any
// entry appended after the caller read the log. It returns how many it
// removed.
func (c *Client) removePending(ctx context.Context, ids map[string]bool) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	for attempt := 0; attempt < maxLocalCASAttempts; attempt++ {
		entries, oid, err := c.readPending(ctx)
		if err != nil {
			return 0, err
		}
		if oid == "" {
			return 0, nil
		}
		kept := make([]PendingEntry, 0, len(entries))
		for _, e := range entries {
			if !ids[e.ID] {
				kept = append(kept, e)
			}
		}
		removed := len(entries) - len(kept)
		if removed == 0 {
			return 0, nil
		}
		next := ""
		if len(kept) > 0 {
			if next, err = c.writePendingCommit(ctx, kept); err != nil {
				return 0, err
			}
		}
		ok, err := c.casRef(ctx, PendingRef, next, oid)
		if err != nil {
			return 0, err
		}
		if ok {
			return removed, nil
		}
	}
	return 0, fmt.Errorf("clear pending ticket-index entries: local compare-and-swap kept losing")
}

// discardPending drops the whole pending log, keyed on the log value it read;
// a CAS miss re-reads and discards the current log. The count is exactly the
// entries the successful delete removed.
func (c *Client) discardPending(ctx context.Context) (int, error) {
	for attempt := 0; attempt < maxLocalCASAttempts; attempt++ {
		entries, oid, err := c.readPending(ctx)
		if err != nil {
			return 0, err
		}
		if oid == "" {
			return 0, nil
		}
		ok, err := c.casRef(ctx, PendingRef, "", oid)
		if err != nil {
			return 0, err
		}
		if ok {
			return len(entries), nil
		}
	}
	return 0, fmt.Errorf("discard pending ticket-index entries: local compare-and-swap kept losing")
}

// DiscardReport is the one-line report of an index-discontinuity discard.
func DiscardReport(n int) string {
	return fmt.Sprintf("%d offline entries were discarded because the remote index disappeared or was recreated", n)
}
