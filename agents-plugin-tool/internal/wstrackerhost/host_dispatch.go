// Event dispatch and in-batch reference resolution.
package wstrackerhost

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/gofrs/flock"
	"github.com/kang-sw/devenv/internal/wsledger"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Dispatch writes events to store id through wstracker's Dispatch, against
// the store directory as every process left it. A *wstracker.Refusal means
// nothing was written. The store stays open, so the idle push fires.
func (h *Host) Dispatch(ctx context.Context, id string, events []wsledger.NewEvent) ([]wsledger.EventID, error) {
	return h.DispatchWith(ctx, id, func(wsledger.EventID) ([]wsledger.NewEvent, error) { return events, nil })
}

// DispatchWith is Dispatch with the events built under the store's lock
// from next, the id the first of them gets (the i-th gets next.Seq+i), so a
// batch can name an item or comment an earlier event of it creates. An
// error from build writes nothing and is returned as is.
//
// Every process on this machine shares the store's clone, and wsledger
// assigns seqs only inside Append, so the reload, the build, and the append
// run under the store's write lock (writeLockFile): without it another
// process's append in between would shift the batch's ids under its $N
// references and leave the validation a step stale.
func (h *Host) DispatchWith(ctx context.Context, id string, build func(next wsledger.EventID) ([]wsledger.NewEvent, error)) ([]wsledger.EventID, error) {
	var ids []wsledger.EventID
	err := h.with(ctx, id, func(st *store) error {
		fl := flock.New(filepath.Join(st.entry.Dir, writeLockFile))
		lockCtx, cancel := context.WithTimeout(ctx, writeLockWait)
		defer cancel()
		if locked, err := fl.TryLockContext(lockCtx, 20*time.Millisecond); err != nil || !locked {
			return fmt.Errorf("tracker: store write lock %s not acquired: %v", fl.Path(), err)
		}
		defer fl.Unlock() //nolint:errcheck
		if err := h.checkRegistered(st); err != nil {
			return err
		}
		if _, err := st.ledger.Reload(ctx); err != nil {
			return err
		}
		st.tracker.Refresh()
		events, err := build(st.tracker.NextID())
		if err != nil {
			return err
		}
		blobs, staged, err := collectBlobs(st, events)
		if err != nil {
			return err
		}
		if ids, err = st.tracker.DispatchWithBlobs(ctx, events, blobs); err != nil {
			return err
		}
		// The append holds the blobs (as git objects named by the journal),
		// so the staged copies have served.
		for _, path := range staged {
			os.Remove(path)
		}
		h.sweepStagedIfDue(st)
		return nil
	})
	return ids, err
}

// DispatchBatch is DispatchWith over events whose payloads may name an
// earlier event of the same batch: in an event's item, parent, comment, or
// reply_to field, the string "$N" (N >= 1, the 1-based position of an
// earlier event) is replaced by the id that event gets. The ids come from
// the store's next id under the write lock, so a browser never computes
// one. A "$..." string that names no earlier event is an error naming the
// event and field, and nothing is written. Every other field, and a
// payload that is not a JSON object, passes through untouched.
func (h *Host) DispatchBatch(ctx context.Context, id string, events []wsledger.NewEvent) ([]wsledger.EventID, error) {
	return h.DispatchWith(ctx, id, func(next wsledger.EventID) ([]wsledger.NewEvent, error) {
		out := make([]wsledger.NewEvent, len(events))
		for i, ev := range events {
			payload, err := resolveBatchRefs(ev.Payload, i, next)
			if err != nil {
				return nil, err
			}
			ev.Payload = payload
			out[i] = ev
		}
		return out, nil
	})
}

// batchRefFields are the payload fields that may hold a "$N" reference.
var batchRefFields = []string{"item", "parent", "comment", "reply_to"}

// resolveBatchRefs resolves the "$N" references of the payload of the
// event at 0-based index i; next is the id the batch's first event gets.
// It returns payload itself when nothing needed resolving.
func resolveBatchRefs(payload json.RawMessage, i int, next wsledger.EventID) (json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(payload, &fields) != nil || fields == nil {
		return payload, nil // not an object: it has no reference field
	}
	changed := false
	for _, field := range batchRefFields {
		raw, ok := fields[field]
		if !ok {
			continue
		}
		var ref string
		if json.Unmarshal(raw, &ref) != nil || !strings.HasPrefix(ref, "$") {
			continue
		}
		n, err := strconv.Atoi(ref[1:])
		if err != nil || ref[1:] != strconv.Itoa(n) || n < 1 || n >= i+1 {
			return nil, fmt.Errorf("tracker: event %d field %q: %q is not a reference to an earlier event of the batch", i+1, field, ref)
		}
		target := next
		target.Seq += n - 1
		resolved, err := json.Marshal(target.String())
		if err != nil {
			return nil, err
		}
		fields[field] = resolved
		changed = true
	}
	if !changed {
		return payload, nil
	}
	return json.Marshal(fields)
}

// writeLockFile, in a store directory, serializes this machine's writes to
// the store across processes (DispatchWith). Its holder can wait inside
// Append for wsledger's store lock while another process holds it across
// the network (a wiki save's fetch and push, each up to networkTimeout; a
// fetch or sync holds it only for its local steps), so a waiter waits that
// long.
const (
	writeLockFile = "write.lock"
	writeLockWait = 2*networkTimeout + lockWait
)
