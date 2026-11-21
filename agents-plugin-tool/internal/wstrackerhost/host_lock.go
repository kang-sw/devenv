// Store lock waits, network ordering, and handle retirement.
package wstrackerhost

import (
	"context"
	"errors"
	"fmt"
	"github.com/kang-sw/devenv/internal/wsledger"
	"sync"
	"time"
)

// ErrStoreBusy is the answer of a call whose context carries a lock wait
// (WithLockWait) when a lock it needs stayed held that long: its store's
// lock, which a fetch or push no longer holds but a wiki save does across
// its own, or, for a call that has to talk to the remote itself (a read of
// an attachment this clone lacks), the store's network lock, which a fetch
// or push in flight holds. Nothing was read or written; the call may be
// retried.
var ErrStoreBusy = errors.New("tracker: store busy")

type lockWaitKey struct{}

// WithLockWait returns ctx bounding how long a Host call run with it waits
// for each lock of its store it needs (the store's lock; for a call that
// talks to the remote, the network lock in this process and across
// processes too): a call that waited d for one answers ErrStoreBusy. A call
// whose context carries no bound waits until the lock is free, which for
// the network lock is up to a network timeout.
func WithLockWait(ctx context.Context, d time.Duration) context.Context {
	return context.WithValue(ctx, lockWaitKey{}, d)
}

// storeLock is a mutex whose acquisition can give up after a bound
// (lockWithin), which a sync.Mutex cannot do without polling. The zero
// value is unlocked.
type storeLock struct {
	once sync.Once
	ch   chan struct{}
}

func (l *storeLock) c() chan struct{} {
	l.once.Do(func() { l.ch = make(chan struct{}, 1) })
	return l.ch
}

func (l *storeLock) Lock() { l.c() <- struct{}{} }

// tryLock takes the lock when it is free right now.
func (l *storeLock) tryLock() bool {
	select {
	case l.c() <- struct{}{}:
		return true
	default:
		return false
	}
}

func (l *storeLock) Unlock() {
	select {
	case <-l.c():
	default:
		panic("wstrackerhost: unlock of unlocked store lock")
	}
}

// lockWithin takes the lock unless it stays held for d (ErrStoreBusy) or
// ctx ends first (ctx's error).
func (l *storeLock) lockWithin(ctx context.Context, d time.Duration) error {
	select {
	case l.c() <- struct{}{}:
		return nil
	default:
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case l.c() <- struct{}{}:
		return nil
	case <-t.C:
		return ErrStoreBusy
	case <-ctx.Done():
		return ctx.Err()
	}
}

// lockCtx takes the lock unless ctx ends first (ctx's error).
func (l *storeLock) lockCtx(ctx context.Context) error {
	select {
	case l.c() <- struct{}{}:
		return nil
	default:
	}
	select {
	case l.c() <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// withNet runs fn for a call that talks to store id's remote: holding the
// store's network mutex, with the store opened, and without its lock
// (st.mu), so local reads and writes go on while fn waits on the network.
// fn gets the registry entry and the ledger as read under the lock, and a
// ctx that carries the caller's lock wait on to the ledger's network lock
// (WithLockWait); it takes st.mu itself for a local step (locked), and the
// ledger's Fetch and Sync take it through applyGate. An ErrStoreBusy is
// either network lock held past the caller's bound.
func (h *Host) withNet(ctx context.Context, id string, fn func(ctx context.Context, st *store, e StoreEntry, l *wsledger.Ledger) error) error {
	wait, bounded := ctx.Value(lockWaitKey{}).(time.Duration)
	busy := func() error { return fmt.Errorf("%w: %q", ErrStoreBusy, id) }
	var st *store
	var e StoreEntry
	var l *wsledger.Ledger
	for st == nil {
		held, err := h.store(id)
		if err != nil {
			return err
		}
		if bounded {
			err = held.net.lockWithin(ctx, wait)
		} else {
			err = held.net.lockCtx(ctx)
		}
		if errors.Is(err, ErrStoreBusy) {
			return busy()
		}
		if err != nil {
			return err
		}
		cur, err := h.lockStore(ctx, id, wait, bounded)
		if err != nil {
			held.net.Unlock()
			return err
		}
		if cur != held {
			// The id was registered again while this call waited: the
			// network mutex to hold is the new handle's.
			cur.mu.Unlock()
			held.net.Unlock()
			continue
		}
		if err := h.open(ctx, cur); err != nil {
			if _, gone := h.Entry(id); errors.Is(gone, ErrNotRegistered) {
				h.retire(cur)
				err = gone
			}
			cur.mu.Unlock()
			held.net.Unlock()
			return err
		}
		st, e, l = cur, cur.entry, cur.ledger
		cur.mu.Unlock()
	}
	defer st.net.Unlock()
	if bounded {
		ctx = wsledger.WithNetWait(ctx, wait)
	}
	err := fn(ctx, st, e, l)
	if errors.Is(err, wsledger.ErrBusy) {
		return busy()
	}
	if err != nil {
		// A store unregistered while the call ran fails on its missing
		// directory or at applyGate; the caller is told what happened.
		if _, gone := h.Entry(id); errors.Is(gone, ErrNotRegistered) {
			st.mu.Lock()
			h.retire(st)
			st.mu.Unlock()
			return gone
		}
	}
	return err
}

// locked runs fn holding st.mu, for a local step of a call inside withNet.
// A handle retired meanwhile answers ErrNotRegistered.
func (h *Host) locked(st *store, fn func() error) error {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.retired {
		return fmt.Errorf("%w %q", ErrNotRegistered, st.id)
	}
	return fn()
}

// with runs fn on the opened store id under its lock, waiting for the lock
// at most the bound ctx carries (WithLockWait).
func (h *Host) with(ctx context.Context, id string, fn func(*store) error) error {
	wait, bounded := ctx.Value(lockWaitKey{}).(time.Duration)
	st, err := h.lockStore(ctx, id, wait, bounded)
	if err != nil {
		return err
	}
	defer st.mu.Unlock()
	if err = h.open(ctx, st); err == nil {
		err = fn(st)
	}
	if err != nil {
		// A store another process unregistered while the call ran fails on
		// its missing directory; the caller is told what happened instead.
		if _, gone := h.Entry(id); errors.Is(gone, ErrNotRegistered) {
			h.retire(st)
			return gone
		}
	}
	return err
}

// lockStore returns this process's handle on the registered store id with
// its mu held; bounded gives up after wait (ErrStoreBusy).
//
// Another process may have unregistered the store, and registered the id
// again, since this one cached the handle, so the registry entry is read
// again under the lock and its registration nonce compared. A handle whose
// registration is gone is retired (ledger closed, idle push timer stopped):
// the call then answers ErrNotRegistered, or goes on with a fresh handle on
// the new registration. An entry that is the same registration replaces the
// cached one, which is how a relabel reaches an open store.
func (h *Host) lockStore(ctx context.Context, id string, wait time.Duration, bounded bool) (*store, error) {
	for {
		st, err := h.store(id)
		if err != nil {
			return nil, err
		}
		if bounded {
			if err := st.mu.lockWithin(ctx, wait); err != nil {
				if errors.Is(err, ErrStoreBusy) {
					return nil, fmt.Errorf("%w: %q", ErrStoreBusy, id)
				}
				return nil, err
			}
		} else {
			st.mu.Lock()
		}
		cur, err := h.Entry(id)
		if err == nil && !st.retired && cur.Nonce == st.nonce {
			st.entry = cur
			return st, nil
		}
		if err != nil && !errors.Is(err, ErrNotRegistered) {
			st.mu.Unlock()
			return nil, err
		}
		h.retire(st)
		st.mu.Unlock()
		if err != nil {
			return nil, err
		}
	}
}

// retire closes st's ledger, which stops its idle push timer, and takes st
// out of the host's map. Callers hold st.mu.
func (h *Host) retire(st *store) {
	if st.ledger != nil {
		st.ledger.Close()
		st.ledger, st.tracker = nil, nil
	}
	st.retired = true
	h.mu.Lock()
	if h.stores[st.id] == st {
		delete(h.stores, st.id)
	}
	h.mu.Unlock()
}

// retireStale retires every cached handle whose registration is not among
// entries, skipping one that is busy (its next call retires it).
func (h *Host) retireStale(entries []StoreEntry) {
	nonces := make(map[string]string, len(entries))
	for _, e := range entries {
		nonces[e.ID] = e.Nonce
	}
	h.mu.Lock()
	var stale []*store
	for id, st := range h.stores {
		if nonce, ok := nonces[id]; !ok || nonce != st.nonce {
			stale = append(stale, st)
		}
	}
	h.mu.Unlock()
	for _, st := range stale {
		if !st.mu.tryLock() {
			continue
		}
		// The registry may have moved since entries was read.
		if cur, err := h.Entry(st.id); errors.Is(err, ErrNotRegistered) || (err == nil && cur.Nonce != st.nonce) {
			h.retire(st)
		}
		st.mu.Unlock()
	}
}

// checkRegistered fails unless st is still the registration the registry
// holds under its id. A write runs it once it holds the store's write lock:
// Unregister holds that lock from before its unpushed check until the entry
// is gone, so a write that passes cannot land in a store being removed.
func (h *Host) checkRegistered(st *store) error {
	cur, err := h.Entry(st.id)
	if err != nil {
		return err
	}
	if cur.Nonce != st.nonce {
		return fmt.Errorf("tracker: store %q was unregistered and registered again during the call; retry", st.id)
	}
	return nil
}

// Close stops the background reloads and closes every open store. Unpushed
// writes stay in their journals and restart the push delay when a store is
// next opened.
func (h *Host) Close() error {
	h.mu.Lock()
	stores := h.stores
	h.stores, h.closed = map[string]*store{}, true
	h.mu.Unlock()
	h.cancel()
	h.wg.Wait()
	var first error
	for _, st := range stores {
		st.mu.Lock()
		if st.ledger != nil {
			if err := st.ledger.Close(); err != nil && first == nil {
				first = err
			}
		}
		st.mu.Unlock()
	}
	return first
}
