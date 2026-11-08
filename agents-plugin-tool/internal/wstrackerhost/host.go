package wstrackerhost

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsgit"
	"github.com/kang-sw/devenv/internal/wsledger"
	"github.com/kang-sw/devenv/internal/wstracker"
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
}

// Host is one process's handle on the machine's tracker stores.
type Host struct {
	root          string
	runner        wsgit.Runner
	clock         wsledger.Clock
	pushDelay     time.Duration
	checkInterval time.Duration

	mu     sync.Mutex
	stores map[string]*store
	closed bool
}

// store is one registered store as this process holds it. mu serializes
// this process's operations on it; the ledger serializes across processes.
type store struct {
	mu      sync.Mutex
	entry   StoreEntry
	ledger  *wsledger.Ledger
	tracker *wstracker.Tracker
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }
func (systemClock) AfterFunc(d time.Duration, f func()) wsledger.Timer {
	return time.AfterFunc(d, f)
}

// New returns a Host over opts.Root. It touches nothing on disk until a
// store is registered or opened.
func New(opts Options) (*Host, error) {
	root := opts.Root
	if root == "" {
		var err error
		if root, err = Root(wsconfig.Options{}); err != nil {
			return nil, err
		}
	}
	h := &Host{root: root, runner: opts.Runner, clock: opts.Clock, pushDelay: opts.PushDelay,
		checkInterval: opts.CheckInterval, stores: map[string]*store{}}
	if h.runner == nil {
		h.runner = batchRunner{}
	}
	if h.clock == nil {
		h.clock = systemClock{}
	}
	if h.checkInterval <= 0 {
		h.checkInterval = DefaultCheckInterval
	}
	return h, nil
}

// Root returns the tracker root.
func (h *Host) Root() string { return h.root }

func (h *Host) now() time.Time { return h.clock.Now().UTC() }

func (h *Host) ledgerOptions(e StoreEntry, name string) wsledger.Options {
	return wsledger.Options{Dir: e.Dir, Member: e.Member, Name: name, KeyDir: filepath.Join(h.root, keysDir),
		Clock: h.clock, PushDelay: h.pushDelay, Runner: h.runner}
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
	st := &store{entry: e}
	h.stores[id] = st
	return st, nil
}

// open opens st's ledger once; it stays open until Close. Callers hold
// st.mu.
func (h *Host) open(ctx context.Context, st *store) error {
	if st.ledger != nil {
		return nil
	}
	l, err := wstracker.Open(ctx, h.ledgerOptions(st.entry, ""))
	if err != nil {
		return fmt.Errorf("tracker: open store %q: %w", st.entry.ID, err)
	}
	st.ledger = l
	st.tracker = wstracker.New(l, l.MemberID()+"."+l.CloneID())
	return nil
}

// with runs fn on the opened store id under its lock.
func (h *Host) with(ctx context.Context, id string, fn func(*store) error) error {
	st, err := h.store(id)
	if err != nil {
		return err
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	if err := h.open(ctx, st); err != nil {
		return err
	}
	return fn(st)
}

// Close closes every open store. Unpushed writes stay in their journals and
// restart the push delay when a store is next opened.
func (h *Host) Close() error {
	h.mu.Lock()
	stores := h.stores
	h.stores, h.closed = map[string]*store{}, true
	h.mu.Unlock()
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

// RegisterRequest registers a store on this machine: creating it (an empty
// repository, or no remote at all) or joining it (a repository holding a
// store).
type RegisterRequest struct {
	// ID names the store on this machine; a valid wsledger id other than
	// "keys".
	ID string
	// Label is the optional display label.
	Label string
	// Remote is the repository URL; empty creates a store with no remote.
	Remote string
	// Member is this machine's member id in the store.
	Member string
	// Profile is the member's profile; nil uses the saved profile used
	// last.
	Profile *wstracker.Profile
	// Origin is the origin of the profile write ("human", "agent"); empty
	// is human.
	Origin string
}

// RegisterResult reports a registration.
type RegisterResult struct {
	Entry StoreEntry
	// Creator: the store's genesis lists this member, so this registration
	// (or an earlier one with the same key) created it.
	Creator bool
	// Profile is the profile the member now has in the store.
	Profile wstracker.Profile
}

// Register creates or joins a store, writes the member's profile, records
// the store in the registry, and saves the profile as the one used last.
// It keeps the store open. A store that cannot be registered leaves no
// directory or registry entry behind; wsledger.ErrIDTaken means another
// join holds the member id.
func (h *Host) Register(ctx context.Context, req RegisterRequest) (RegisterResult, error) {
	var res RegisterResult
	if !wsledger.ValidID(req.ID) || req.ID == keysDir {
		return res, fmt.Errorf("tracker: store id %q is not valid (lowercase letters, digits, '_' and '-'; not %q)", req.ID, keysDir)
	}
	if !wsledger.ValidID(req.Member) {
		return res, fmt.Errorf("tracker: member id %q is not valid (lowercase letters, digits, '_' and '-')", req.Member)
	}
	var profile wstracker.Profile
	if req.Profile != nil {
		profile = *req.Profile
	} else {
		saved, err := h.Profiles()
		if err != nil {
			return res, err
		}
		last, ok := saved.LastProfile()
		if !ok {
			return res, errors.New("tracker: no profile given and none saved")
		}
		profile = last
	}
	profile, err := cleanProfile(profile)
	if err != nil {
		return res, err
	}
	if _, err := h.Entry(req.ID); err == nil {
		return res, fmt.Errorf("tracker: store %q is already registered", req.ID)
	}
	entry := StoreEntry{ID: req.ID, Label: req.Label, Remote: req.Remote, Dir: filepath.Join(h.root, req.ID), Member: req.Member}
	if err := os.MkdirAll(h.root, 0o700); err != nil {
		return res, err
	}
	// Creating the directory claims the id against a concurrent registration.
	if err := os.Mkdir(entry.Dir, 0o700); err != nil {
		if errors.Is(err, os.ErrExist) {
			return res, fmt.Errorf("tracker: %s already exists; remove it or register under another id", entry.Dir)
		}
		return res, err
	}
	st := &store{entry: entry}
	err = func() error {
		opts := h.ledgerOptions(entry, profile.Given)
		var l *wsledger.Ledger
		var err error
		if entry.Remote == "" {
			l, err = wstracker.Create(ctx, opts)
		} else {
			l, err = wstracker.Register(ctx, opts, entry.Remote)
		}
		if err != nil {
			return err
		}
		st.ledger, st.tracker = l, wstracker.New(l, l.MemberID()+"."+l.CloneID())
		if res.Profile, err = writeProfile(ctx, st, profile, req.Origin); err != nil {
			return err
		}
		res.Creator = genesisLists(l.Identity().Genesis, req.Member)
		return h.addEntry(entry)
	}()
	if err != nil {
		if st.ledger != nil {
			st.ledger.Close()
		}
		os.RemoveAll(entry.Dir)
		return RegisterResult{}, err
	}
	if entry.Remote != "" {
		// Publish the profile now; a failure leaves it to the idle push.
		_ = h.attempt(ctx, st, networkTimeout, func(ctx context.Context) error { _, err := st.ledger.Sync(ctx); return err })
	}
	res.Entry = entry
	_ = h.SaveProfile(profile) // the store is registered; a lost profile save costs only the default
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		st.ledger.Close()
		return res, nil
	}
	h.stores[entry.ID] = st
	h.mu.Unlock()
	return res, nil
}

// writeProfile writes a member.profile carrying the fields of want that
// differ from what the store holds. wsledger writes only a name (read as
// the given name) into a genesis member or a join, so org and family always
// arrive this way.
func writeProfile(ctx context.Context, st *store, want wstracker.Profile, origin string) (wstracker.Profile, error) {
	member := st.entry.Member
	have := st.tracker.View(time.Time{}, 0).Members[member].Profile
	changed := map[string]string{}
	if want.Org != have.Org {
		changed["org"] = want.Org
	}
	if want.Family != have.Family {
		changed["family"] = want.Family
	}
	if want.Given != have.Given {
		changed["given"] = want.Given
	}
	if len(changed) == 0 {
		return have, nil
	}
	payload, err := json.Marshal(wstracker.MemberProfile{Profile: changed})
	if err != nil {
		return have, err
	}
	if _, err := st.tracker.Dispatch(ctx, []wsledger.NewEvent{{Type: wstracker.TypeMemberProfile, Origin: origin, Payload: payload}}); err != nil {
		return have, fmt.Errorf("tracker: write the profile: %w", err)
	}
	return want, nil
}

// genesisLists reports whether the genesis roles.json lists member.
func genesisLists(genesis json.RawMessage, member string) bool {
	var roles wsledger.Roles
	if json.Unmarshal(genesis, &roles) != nil {
		return false
	}
	for _, m := range roles.Members {
		if m.ID == member {
			return true
		}
	}
	return false
}

// Dispatch writes events to store id through wstracker's Dispatch, against
// the store directory as every process left it. A *wstracker.Refusal means
// nothing was written. The store stays open, so the idle push fires.
func (h *Host) Dispatch(ctx context.Context, id string, events []wsledger.NewEvent) ([]wsledger.EventID, error) {
	var ids []wsledger.EventID
	err := h.with(ctx, id, func(st *store) error {
		if _, err := st.ledger.Reload(ctx); err != nil {
			return err
		}
		var err error
		ids, err = st.tracker.Dispatch(ctx, events)
		return err
	})
	return ids, err
}

// View returns store id's view state at the host's now, with the
// caller-filled store, sync, clones, and stats.commits. It reads the store
// directory as every process left it and never calls the remote.
// staleDays <= 0 means wstracker's default.
func (h *Host) View(ctx context.Context, id string, staleDays int) (*StoreView, error) {
	var out *StoreView
	err := h.with(ctx, id, func(st *store) error {
		if _, err := st.ledger.Reload(ctx); err != nil {
			return err
		}
		v := st.tracker.View(h.now(), staleDays)
		commits, err := countCommits(ctx, h.runner, st.entry.Dir)
		if err != nil {
			return err
		}
		evs, _ := st.ledger.Events(wsledger.Cursor{})
		out = fill(v, st.entry, st.ledger.MemberID()+"."+st.ledger.CloneID(), evs, st.ledger.Pending(), readSyncState(st.entry.Dir), commits)
		return nil
	})
	return out, err
}

// Check is the immediate check (a receive-only fetch) the drawer runs when
// a store's drawer opens. It is never throttled; its outcome is recorded
// for every process. A store with no remote only reloads.
func (h *Host) Check(ctx context.Context, id string) (wsledger.SyncReport, error) {
	var rep wsledger.SyncReport
	err := h.with(ctx, id, func(st *store) error {
		if st.entry.Remote == "" {
			var err error
			rep, err = st.ledger.Reload(ctx)
			return err
		}
		return h.attempt(ctx, st, networkTimeout, func(ctx context.Context) error {
			var err error
			rep, err = st.ledger.Fetch(ctx)
			return err
		})
	})
	return rep, err
}

// Sync is the apply path: close the open batch, push this clone's writes,
// and fetch. It runs at an explicit apply and when the drawer panel
// collapses; its outcome is recorded for every process. With no remote it
// closes the batch.
func (h *Host) Sync(ctx context.Context, id string) (wsledger.SyncReport, error) {
	var rep wsledger.SyncReport
	err := h.with(ctx, id, func(st *store) error {
		if st.entry.Remote == "" {
			var err error
			rep, err = st.ledger.Sync(ctx)
			return err
		}
		return h.attempt(ctx, st, networkTimeout, func(ctx context.Context) error {
			var err error
			rep, err = st.ledger.Sync(ctx)
			return err
		})
	})
	return rep, err
}

// syncState is a store's last check or sync outcome, persisted in its
// directory so every process sharing the store agrees on it.
type syncState struct {
	// AttemptedAt is the last check or sync that ran, success or not; the
	// throttle counts from it.
	AttemptedAt time.Time `json:"attemptedAt"`
	// SyncedAt is the last one that reached the remote.
	SyncedAt time.Time `json:"syncedAt,omitempty"`
	// Error is the last attempt's error; empty when it reached the remote.
	Error string `json:"error,omitempty"`
}

const syncStateFile = "host-sync.json"

func readSyncState(dir string) syncState {
	var s syncState
	_ = readJSON(filepath.Join(dir, syncStateFile), &s)
	return s
}

// attempt runs one network call on st within timeout and records its
// outcome for every process. Callers hold st.mu.
func (h *Host) attempt(ctx context.Context, st *store, timeout time.Duration, call func(context.Context) error) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	err := call(ctx)
	s := readSyncState(st.entry.Dir)
	s.AttemptedAt = h.now()
	if err != nil {
		s.Error = err.Error()
	} else {
		s.SyncedAt, s.Error = s.AttemptedAt, ""
	}
	_ = writeJSON(filepath.Join(st.entry.Dir, syncStateFile), s)
	return err
}
