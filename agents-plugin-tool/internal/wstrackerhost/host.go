// Host lifecycle, store handles, and local views.
package wstrackerhost

import (
	"context"
	"errors"
	"fmt"
	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsgit"
	"github.com/kang-sw/devenv/internal/wsledger"
	"github.com/kang-sw/devenv/internal/wstracker"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

const (
	// DefaultPollInterval is the background poll period ((396)).
	DefaultPollInterval = 3 * time.Minute
	// DefaultCheckInterval is the least time between two throttled checks of
	// one store (the session-start and pre-read checks), counted from the
	// last attempt by any process, so an offline remote costs one timeout
	// per interval rather than one per read.
	DefaultCheckInterval = 30 * time.Second
	// networkTimeout bounds one check or sync.
	networkTimeout = 60 * time.Second
	// preReadTimeout bounds the check a read waits for.
	preReadTimeout = 15 * time.Second
)

// Options configures a Host. The zero value is the real machine.
type Options struct {
	// Root is the tracker root; empty means Root(wsconfig.Options{}).
	Root string
	// Runner runs git for every store; nil means the non-interactive
	// runner (batchRunner).
	Runner wsgit.Runner
	// Clock is the time source of the stores and of the check throttle;
	// nil means the system clock.
	Clock wsledger.Clock
	// PushDelay is wsledger's idle push delay; zero means its default.
	PushDelay time.Duration
	// CheckInterval throttles the session-start and pre-read checks; zero
	// means DefaultCheckInterval.
	CheckInterval time.Duration
	// Environ is the environment the default runner's git runs under (and
	// where it is looked up); nil means this process's. A daemon-hosted
	// server passes its connection's environment.
	Environ func() []string
}

// Host is one process's handle on the machine's tracker stores.
type Host struct {
	root   string
	runner wsgit.Runner
	// environ is Options.Environ: the environment the stores' ssh-keygen
	// runs under too.
	environ       func() []string
	clock         wsledger.Clock
	pushDelay     time.Duration
	checkInterval time.Duration

	// ctx is canceled by Close; the background reloads of ViewWithin run on
	// it, and wg counts them.
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	mu     sync.Mutex
	stores map[string]*store
	closed bool
	// onChange is the change callback StartWatch sets; a late ViewWithin
	// reload calls it too. nil when no watch runs.
	onChange func()

	// registryLoadMu orders disk refreshes and successful local publications.
	// Registry readers in Seen take only registryMu, never the I/O mutex.
	registryLoadMu  sync.Mutex
	registryMu      sync.RWMutex
	registryEntries map[string]StoreEntry
	registryStores  []StoreEntry
	registryErr     error

	// seenMu guards the clone-local marks, loaded once per registration.
	// Only this daemon writes ui-seen.json; writes update this memory copy.
	seenMu sync.Mutex
	seen   map[string]seenCache

	// reloads, lateReloads, and lateChanges count ViewWithin's background
	// reloads (ReloadStats).
	reloads, lateReloads, lateChanges atomic.Int64
}

// store is one registered store as this process holds it. mu serializes
// this process's local operations on it, and net its calls that talk to
// the store's remote; the ledger serializes both across processes.
type store struct {
	// mu guards the fields below and the tracker, and is held across every
	// local read and write of the store, never across a fetch or a push
	// (a wiki save's and a registration's own excepted).
	mu storeLock
	// net is the network mutex (withNet): held by the one call of this
	// process that is talking to the store's remote. It is taken before mu
	// and never while holding it, because the ledger runs the local steps
	// of a fetch or sync through applyGate, which takes mu.
	net storeLock
	// id and nonce name the registration the handle was made for
	// (StoreEntry.ID and Nonce). They never change, so they are read
	// without mu.
	id, nonce string
	// entry is the registry entry as last read under mu (lockStore).
	entry   StoreEntry
	ledger  *wsledger.Ledger
	tracker *wstracker.Tracker
	// retired: the registration this handle was made for is gone (the
	// store was unregistered, here or by another process), so the handle
	// left the host's map with its ledger closed; whoever still holds it
	// asks the host again. Guarded by mu.
	retired bool
	// lastSweep is when the staged-attachment sweep last ran
	// (attachments.go).
	lastSweep time.Time
	// commits is the store's commit count as last counted, at the ledger
	// refs version commitsAt (commitCount).
	commits   int
	commitsAt string

	// viewMu guards ViewWithin's state, apart from mu so a waiting read
	// never holds mu: snap is the view the last reload folded, cur the
	// reload in flight, and next the one queued behind it.
	viewMu    sync.Mutex
	snap      *StoreView
	cur, next *reload
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }
func (systemClock) AfterFunc(d time.Duration, f func()) wsledger.Timer {
	return time.AfterFunc(d, f)
}

// New returns a Host over opts.Root. It writes nothing on disk until a
// store is registered or opened, apart from deleting what an interrupted
// Unregister left behind (sweepRemoved).
func New(opts Options) (*Host, error) {
	root := opts.Root
	if root == "" {
		var err error
		if root, err = Root(wsconfig.Options{}); err != nil {
			return nil, err
		}
	}
	h := &Host{root: root, runner: opts.Runner, environ: opts.Environ, clock: opts.Clock, pushDelay: opts.PushDelay,
		checkInterval: opts.CheckInterval, stores: map[string]*store{}}
	h.ctx, h.cancel = context.WithCancel(context.Background())
	if h.runner == nil {
		h.runner = batchRunner{environ: opts.Environ}
	}
	if h.clock == nil {
		h.clock = systemClock{}
	}
	if h.checkInterval <= 0 {
		h.checkInterval = DefaultCheckInterval
	}
	h.sweepRemoved()
	// Recover the registration snapshot before serving interactive reads. Keep
	// a load error in the snapshot (as Stores/Entry historically report it),
	// rather than turning a malformed registry into a constructor failure.
	_, _ = h.Stores()
	return h, nil
}

// Root returns the tracker root.
func (h *Host) Root() string { return h.root }

func (h *Host) now() time.Time { return h.clock.Now().UTC() }

func (h *Host) ledgerOptions(st *store, name string) wsledger.Options {
	var env []string
	if h.environ != nil {
		env = h.environ()
	}
	e := st.entry
	return wsledger.Options{Dir: e.Dir, Member: e.Member, Name: name, KeyDir: filepath.Join(h.root, keysDir),
		Clock: h.clock, PushDelay: h.pushDelay, Runner: h.runner, Env: env, ApplyGate: h.applyGate(st)}
}

// applyGate is the wsledger.Options.ApplyGate of st's ledger. The ledger
// talks to the remote holding neither st.mu nor its own store lock, so the
// store can be unregistered meanwhile (and its id registered again); every
// local step of a fetch or sync, the idle push's included, therefore runs
// under st.mu after the registry entry is read again. A step whose
// registration is gone does not run, which discards what was fetched, and
// the handle is retired, which stops its ledger's idle push.
func (h *Host) applyGate(st *store) func(step func() error) error {
	return func(step func() error) error {
		st.mu.Lock()
		defer st.mu.Unlock()
		cur, err := h.Entry(st.id)
		switch {
		case err == nil && cur.Nonce == st.nonce && !st.retired:
			return step()
		case err != nil && !errors.Is(err, ErrNotRegistered):
			return err
		}
		h.retire(st)
		if err != nil {
			return err
		}
		return fmt.Errorf("tracker: store %q was unregistered and registered again during the call; retry", st.id)
	}
}

// store returns this process's handle on the registered store id, creating
// it unopened.
func (h *Host) store(id string) (*store, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return nil, errors.New("tracker: host closed")
	}
	if st := h.stores[id]; st != nil {
		return st, nil
	}
	e, err := h.Entry(id)
	if err != nil {
		return nil, err
	}
	st := &store{id: e.ID, nonce: e.Nonce, entry: e}
	h.stores[id] = st
	return st, nil
}

// open opens st's ledger once; it stays open until Close. Callers hold
// st.mu.
func (h *Host) open(ctx context.Context, st *store) error {
	if st.ledger != nil {
		return nil
	}
	l, err := wstracker.Open(ctx, h.ledgerOptions(st, ""))
	if err != nil {
		return fmt.Errorf("tracker: open store %q: %w", st.entry.ID, err)
	}
	st.ledger = l
	st.tracker = wstracker.New(l, l.MemberID()+"."+l.CloneID())
	h.trySweep(st)
	return nil
}

// View returns store id's view state at the host's now, with the
// caller-filled store, sync, clones, and stats.commits. It reads the store
// directory as every process left it and never calls the remote.
// staleDays <= 0 means wstracker's default.
func (h *Host) View(ctx context.Context, id string, staleDays int) (*StoreView, error) {
	return h.view(ctx, id, staleDays, nil)
}

// view is View; reading, when not nil, runs just before the store directory
// is re-read.
func (h *Host) view(ctx context.Context, id string, staleDays int, reading func()) (*StoreView, error) {
	var out *StoreView
	err := h.with(ctx, id, func(st *store) error {
		if reading != nil {
			reading()
		}
		rep, err := st.ledger.Reload(ctx)
		if err != nil {
			return err
		}
		v := st.tracker.View(h.now(), staleDays)
		commits, err := h.commitCount(ctx, st)
		if err != nil {
			return err
		}
		evs, _ := st.ledger.Events(wsledger.Cursor{})
		// rep.Pending is the ledger's Pending as the reload left it, which
		// nothing since changed; asking again would read the push state
		// file again.
		out = fill(v, st.entry, st.ledger.MemberID()+"."+st.ledger.CloneID(), evs, rep.Pending, readSyncState(st.entry.Dir), commits)
		return nil
	})
	return out, err
}

// commitCount is countCommits for st, counted again only when the refs
// its ledger last loaded moved since the last count (wsledger
// RefsVersion), so a view of an unchanged store runs no git. Callers hold
// st.mu.
func (h *Host) commitCount(ctx context.Context, st *store) (int, error) {
	at := st.ledger.RefsVersion()
	if at != "" && at == st.commitsAt {
		return st.commits, nil
	}
	n, err := countCommits(ctx, h.runner, st.entry.Dir)
	if err != nil {
		return 0, err
	}
	// A ref another process moved since the reload is counted under the
	// reload's version; the next reload reads a new version and counts
	// again.
	st.commits, st.commitsAt = n, at
	return n, nil
}
