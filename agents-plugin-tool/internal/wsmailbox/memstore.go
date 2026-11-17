package wsmailbox

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kang-sw/devenv/internal/wsstate"
	"github.com/kang-sw/devenv/internal/wswake"
)

// memstore.go is the memory-first mailbox store
// (261117-feat-ws-mailbox-memory-first-store). Every scope tier's presence
// records and queues, the machine reply-id registry, each session's history
// window, and the armed-wait listening markers live in one in-memory state
// per cache root. The only steady-state file write is one JSONL record
// appended to the restore ring (ring.go) per committed change: a send, a
// recv, a presence heartbeat. The legacy per-scope mailbox.json files, the
// reply registry file, and the history entry files are read once at start
// and written once at a graceful daemon shutdown, for a daemon downgrade
// (legacy.go).
//
// The scope files' resolved paths stay the keys of the named stores, so the
// callers' path resolution is unchanged, and the path is what the export
// writes back to.
//
// Two lifetimes share one implementation. The dashboard daemon holds a
// persistent instance per cache root: the ring lock and the newest
// segment's descriptor stay open for its life, and the state is served from
// memory. Any other process (a direct binary run, a test without
// OpenPersistent) gets a transient instance per call: it takes the ring
// lock, loads the ring, runs the call, and releases everything, so the
// process holds no descriptor between calls. A transient call made while
// the daemon holds the ring lock fails after LockTimeout: direct-binary
// mailbox access while the daemon runs is unsupported.

// Store is the mailbox store of one cache root. The zero value is not
// usable; obtain one with Paths.Store.
type Store struct {
	paths   Paths
	ringDir string
	err     error
}

// settledRoots holds the cache roots this process has seen exist.
var settledRoots sync.Map

// Store resolves p's mailbox store. Resolution errors surface from every
// call.
func (p Paths) Store() Store {
	root, err := wsstate.CacheRoot(p.State)
	if err != nil {
		return Store{paths: p, err: err}
	}
	if _, ok := settledRoots.Load(root); !ok {
		// wsstate resolves a cache root that does not exist yet without its
		// symlinks, and the canonical form once it does: create it first,
		// so every call of this process names one ring and one registry
		// entry.
		if err := os.MkdirAll(root, 0o755); err != nil {
			return Store{paths: p, err: fmt.Errorf("create cache root: %w", err)}
		}
		if root, err = wsstate.CacheRoot(p.State); err != nil {
			return Store{paths: p, err: err}
		}
		settledRoots.Store(root, true)
	}
	return Store{paths: p, ringDir: filepath.Join(root, RingDirName)}
}

// rawCacheRoot is p's cache root as configured, before symlink resolution:
// a scope path resolved while the root did not exist yet starts with it.
func rawCacheRoot(p Paths) string {
	raw := p.State.CacheHome
	if raw == "" {
		raw = os.Getenv(wsstate.EnvCacheHome)
	}
	if raw == "" {
		return ""
	}
	abs, err := filepath.Abs(raw)
	if err != nil {
		return ""
	}
	return abs
}

var (
	registryMu sync.Mutex
	registry   = map[string]*instance{}
	// transientMu serializes this process's transient calls per ring before
	// the cross-process ring lock, so they queue instead of polling it.
	transientMu = map[string]*sync.Mutex{}
	// openings holds each cache root's persistent open in flight, or its
	// failure for persistentRetryAfter, so the ring lock wait runs outside
	// registryMu and a held ring costs each later call no wait of its own.
	openings = map[string]*opening{}
	// persistentMode makes Paths.Store open persistent instances for any
	// cache root (EnablePersistent).
	persistentMode atomic.Bool
)

// persistentRetryAfter is how long a failed persistent open answers later
// calls with its error before one of them tries again.
const persistentRetryAfter = 5 * time.Second

type opening struct {
	done     chan struct{}
	in       *instance
	err      error
	failedAt time.Time
}

// EnablePersistent makes every later store call in this process hold a
// persistent instance for its cache root, opened on first use. The
// dashboard daemon enables it at start and closes the instances with
// ClosePersistent at shutdown.
func EnablePersistent() { persistentMode.Store(true) }

// OpenPersistent opens and registers p's persistent instance (or returns
// the registered one): the ring lock and descriptor stay held until Close.
func OpenPersistent(p Paths) (*Persistent, error) {
	s := p.Store()
	if s.err != nil {
		return nil, s.err
	}
	in, err := registeredInstance(s, true)
	if err != nil {
		return nil, err
	}
	return &Persistent{in: in}, nil
}

// ClosePersistent ends persistent mode and closes every registered
// persistent instance (Close), and returns their errors joined. A later
// store call in this process runs transient: a call that outlives a daemon
// shutdown takes the ring lock for itself, and fails once a successor
// holds it.
func ClosePersistent() error {
	registryMu.Lock()
	// Under registryMu: an open in flight sees the mode off when it
	// finishes and closes its instance instead of registering it.
	persistentMode.Store(false)
	list := make([]*instance, 0, len(registry))
	for _, in := range registry {
		list = append(list, in)
	}
	registryMu.Unlock()
	var errs []error
	for _, in := range list {
		errs = append(errs, (&Persistent{in: in}).Close())
	}
	return errors.Join(errs...)
}

// Persistent is a registered persistent instance.
type Persistent struct{ in *instance }

// Close exports the live state to the legacy files a previous version reads
// (legacy.go), appends the export record, and releases the ring lock and
// descriptor. The next call for this cache root opens the ring again.
func (p *Persistent) Close() error {
	in := p.in
	registryMu.Lock()
	if registry[in.ringDir] == in {
		delete(registry, in.ringDir)
	}
	registryMu.Unlock()
	in.mu.Lock()
	defer in.mu.Unlock()
	if in.closed {
		return nil
	}
	err := in.export()
	in.closed = true
	return errors.Join(err, in.release())
}

// registeredInstance returns s's registered instance, opening a persistent
// one when none is registered. One open per cache root runs at a time, its
// ring lock wait outside registryMu; concurrent callers wait for its
// outcome, and a failure answers later callers until persistentRetryAfter
// (explicit, an OpenPersistent, always tries). An open that finishes after
// ClosePersistent ended persistent mode is closed again, not registered:
// the returned instance is then closed, and the caller goes transient.
func registeredInstance(s Store, explicit bool) (*instance, error) {
	registryMu.Lock()
	if in := registry[s.ringDir]; in != nil {
		registryMu.Unlock()
		return in, nil
	}
	if op := openings[s.ringDir]; op != nil {
		select {
		case <-op.done:
			if !explicit && storeNow().Sub(op.failedAt) < persistentRetryAfter {
				registryMu.Unlock()
				return nil, op.err
			}
		default:
			registryMu.Unlock()
			<-op.done
			if op.err != nil {
				return nil, op.err
			}
			return op.in, nil
		}
	}
	op := &opening{done: make(chan struct{})}
	openings[s.ringDir] = op
	registryMu.Unlock()

	in, err := openInstance(s.paths, s.ringDir, true, false)

	registryMu.Lock()
	op.in, op.err = in, err
	discard := false
	if err != nil {
		op.failedAt = storeNow()
	} else {
		delete(openings, s.ringDir)
		if explicit || persistentMode.Load() {
			registry[s.ringDir] = in
		} else {
			discard = true
		}
	}
	close(op.done)
	registryMu.Unlock()
	if discard {
		_ = (&Persistent{in: in}).Close()
	}
	return in, err
}

// with runs fn against s's instance under its lock: the registered
// persistent one, a new persistent one under EnablePersistent, or a
// transient one opened for this call alone.
func (s Store) with(fn func(*instance) error) error { return s.run(false, fn) }

// read is with for a call that changes nothing. A transient read that finds
// the ring lock held by another process (the daemon) reads the ring without
// it (openView) instead of failing.
func (s Store) read(fn func(*instance) error) error { return s.run(true, fn) }

func (s Store) run(readOnly bool, fn func(*instance) error) error {
	if s.err != nil {
		return s.err
	}
	for {
		registryMu.Lock()
		in := registry[s.ringDir]
		registryMu.Unlock()
		if in == nil && persistentMode.Load() {
			var err error
			if in, err = registeredInstance(s, false); err != nil {
				return err
			}
		}
		if in == nil {
			break
		}
		in.mu.Lock()
		if in.closed {
			in.mu.Unlock()
			continue
		}
		err := fn(in)
		in.mu.Unlock()
		return err
	}
	registryMu.Lock()
	mu := transientMu[s.ringDir]
	if mu == nil {
		mu = &sync.Mutex{}
		transientMu[s.ringDir] = mu
	}
	registryMu.Unlock()
	mu.Lock()
	defer mu.Unlock()
	in, err := openInstance(s.paths, s.ringDir, false, readOnly)
	if errors.Is(err, errRingHeld) && readOnly {
		in, err = openView(s.paths, s.ringDir)
	}
	if err != nil {
		return err
	}
	in.mu.Lock()
	defer in.mu.Unlock()
	return errors.Join(fn(in), in.release())
}

// Update runs fn as one transaction and, when it changed anything, appends
// one record to the ring before committing it to memory. fn's error
// discards every change. op labels the record. A queue change wakes the
// mailbox waits' store subscribers with the changed store's path.
func (s Store) Update(op string, fn func(*Tx) error) error {
	var changed []string
	err := s.with(func(in *instance) error {
		var err error
		changed, err = in.update(op, fn)
		return err
	})
	for _, path := range changed {
		wswake.Mailbox.StoreChanged(path)
	}
	return err
}

// WithLock is Update over the one named store at path.
func (s Store) WithLock(path string, fn func(*StoreFile) error) error {
	return s.Update("named", func(tx *Tx) error {
		store, err := tx.Named(path)
		if err != nil {
			return err
		}
		return fn(store)
	})
}

// WithReplyLock is Update over the reply-id registry.
func (s Store) WithReplyLock(fn func(*ReplyStore) error) error {
	return s.Update("reply", func(tx *Tx) error { return fn(tx.Reply()) })
}

// PostSystem appends a system message to reply-id replyID's queue: one the
// ws runtime itself sends (a spawned agent's turn report), from no session,
// so it carries neither From nor ReplyTo and has no address to answer.
// label is display-only (empty is valid). The caller wakes the waits.
func (s Store) PostSystem(replyID, label, content string, sentAt time.Time) error {
	if replyID == "" {
		return fmt.Errorf("post system message: empty reply-id")
	}
	return s.Update("system", func(tx *Tx) error {
		reply := tx.Reply()
		reply.Queues = AppendQueue(reply.Queues, replyID, Envelope{Label: label, Content: content, SentAt: sentAt.UTC().Format(time.RFC3339)})
		return nil
	})
}

// Load returns a copy of the named store at path (empty when it holds
// nothing).
func (s Store) Load(path string) (StoreFile, error) {
	var out StoreFile
	err := s.read(func(in *instance) error {
		store, err := in.named(path)
		if err != nil {
			return err
		}
		out = cloneStore(store, true)
		return nil
	})
	return out, err
}

// LoadReply returns a copy of the reply-id registry.
func (s Store) LoadReply() (ReplyStore, error) {
	var out ReplyStore
	err := s.read(func(in *instance) error {
		out = cloneReply(in.st.Reply)
		return nil
	})
	return out, err
}

// History returns sessionKey's history window, newest first.
func (s Store) History(sessionKey string) ([]HistoryEntry, error) {
	var out []HistoryEntry
	err := s.read(func(in *instance) error {
		window := in.st.History[sessionKey]
		out = make([]HistoryEntry, 0, len(window))
		for i := len(window) - 1; i >= 0; i-- {
			out = append(out, window[i])
		}
		return nil
	})
	return out, err
}

// Listening returns sessionKey's armed-wait marker; false when none.
func (s Store) Listening(sessionKey string) (ListeningMarker, bool, error) {
	var (
		out ListeningMarker
		ok  bool
	)
	err := s.read(func(in *instance) error {
		out, ok = in.st.Listening[sessionKey]
		return nil
	})
	return out, ok, err
}

// QueuePaths lists the store paths the state holds: every named store and
// the reply registry's path, for the undelivered-mail sweep's discovery.
func (s Store) QueuePaths() (reply string, named []string, err error) {
	err = s.read(func(in *instance) error {
		reply = in.replyPath
		named = slices.Sorted(maps.Keys(in.st.Named))
		return nil
	})
	return reply, named, err
}

// Tx is one Update's working view. The stores it hands out are private
// copies until the transaction commits.
type Tx struct {
	in        *instance
	named     map[string]*StoreFile
	reply     *ReplyStore
	history   map[string][]HistoryEntry
	listening map[string]*ListeningMarker
}

// Named returns the working copy of the named store at path.
func (tx *Tx) Named(path string) (*StoreFile, error) {
	path = tx.in.key(path)
	if store, ok := tx.named[path]; ok {
		return store, nil
	}
	current, err := tx.in.named(path)
	if err != nil {
		return nil, err
	}
	working := cloneStore(current, false)
	tx.named[path] = &working
	return &working, nil
}

// Reply returns the working copy of the reply-id registry.
func (tx *Tx) Reply() *ReplyStore {
	if tx.reply == nil {
		working := cloneReply(tx.in.st.Reply)
		tx.reply = &working
	}
	return tx.reply
}

// AppendHistory appends entries to sessionKey's window, stamped at (each
// later than the last stamp this store issued), and returns their names.
func (tx *Tx) AppendHistory(sessionKey string, at time.Time, entries ...HistoryEntry) ([]string, error) {
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		at = tx.in.nextStamp(at)
		name, err := newHistoryName(at)
		if err != nil {
			return names, err
		}
		e.Name = name
		tx.history[sessionKey] = append(tx.history[sessionKey], e)
		names = append(names, name)
	}
	return names, nil
}

// SetListening records m as its session's armed-wait marker.
func (tx *Tx) SetListening(m ListeningMarker) {
	tx.listening[m.SessionKey] = &m
}

// ClearListening removes sessionKey's marker when it belongs to ownerPID
// (see Paths.ClearListeningMarker).
func (tx *Tx) ClearListening(sessionKey string, ownerPID int) {
	current, ok := tx.in.st.Listening[sessionKey]
	if pending, set := tx.listening[sessionKey]; set {
		ok = pending != nil
		if ok {
			current = *pending
		}
	}
	if ok && current.PID == ownerPID {
		tx.listening[sessionKey] = nil
	}
}

// --- instance ---------------------------------------------------------

// instance is one opened ring and its state.
type instance struct {
	mu         sync.Mutex
	paths      Paths
	cacheRoot  string
	rawRoot    string // set when it differs from cacheRoot
	ringDir    string
	replyPath  string
	persistent bool
	view       bool // openView: no lock, read only
	closed     bool

	ring ring
	st   state
	// loaded holds the named store paths this instance reconciled with
	// their legacy files (legacy.go): each once per open.
	loaded    map[string]bool
	lastStamp time.Time
}

// state is the whole mailbox state of one cache root, and the body of a
// ring snapshot record.
type state struct {
	Named     map[string]StoreFile       `json:"named,omitempty"`
	Reply     ReplyStore                 `json:"reply"`
	History   map[string][]HistoryEntry  `json:"history,omitempty"`
	Listening map[string]ListeningMarker `json:"listening,omitempty"`
	// Baseline is what the legacy files held when this state last read or
	// wrote them (legacy.go).
	Baseline baseline `json:"baseline"`
}

func emptyState() state {
	return state{
		Named:     map[string]StoreFile{},
		Reply:     emptyReplyStore(),
		History:   map[string][]HistoryEntry{},
		Listening: map[string]ListeningMarker{},
		Baseline:  emptyBaseline(),
	}
}

// storeNow is the store's clock (a test seam).
var storeNow = func() time.Time { return time.Now().UTC() }

// newInstance is an instance of p's cache root with nothing loaded.
func newInstance(p Paths, ringDir string, persistent bool) (*instance, error) {
	cacheRoot, err := wsstate.CacheRoot(p.State)
	if err != nil {
		return nil, err
	}
	replyPath, err := p.ReplyRegistry()
	if err != nil {
		return nil, err
	}
	in := &instance{paths: p, cacheRoot: cacheRoot, ringDir: ringDir, replyPath: filepath.Clean(replyPath),
		persistent: persistent, loaded: map[string]bool{}, st: emptyState()}
	if raw := rawCacheRoot(p); raw != cacheRoot {
		in.rawRoot = raw
	}
	return in, nil
}

// openInstance locks and loads the ring and reconciles the legacy files.
// noWait tries the ring lock once (a read that can fall back to openView).
func openInstance(p Paths, ringDir string, persistent, noWait bool) (*instance, error) {
	in, err := newInstance(p, ringDir, persistent)
	if err != nil {
		return nil, err
	}
	wait := LockTimeout
	switch {
	case persistent:
		wait = persistentLockWait
	case noWait:
		wait = 0
	}
	if err := in.ring.open(ringDir, persistent, wait, &in.st); err != nil {
		return nil, err
	}
	// A legacy file that does not reconcile (unreadable, or a failed clear)
	// is left as is: it must not take the store down with it.
	if err := in.reconcileAll(); err != nil {
		ringWarn("reconcile legacy mailbox files: %v", err)
	}
	// History stamps keep rising across opens (and across the transient
	// instances of one process), so one session's entries keep their order
	// even when the clock does not advance between them.
	for _, window := range in.st.History {
		if len(window) == 0 {
			continue
		}
		if at, ok := HistoryTime(window[len(window)-1].Name); ok && at.After(in.lastStamp) {
			in.lastStamp = at
		}
	}
	return in, nil
}

func (in *instance) release() error { return in.ring.close() }

// openView is a read-only instance loaded from the ring without its lock:
// every committed call appended its record before it returned, so the
// newest segment holds the committed state. It reconciles no legacy file
// (the lock holder did at its open) and writes nothing.
func openView(p Paths, ringDir string) (*instance, error) {
	in, err := newInstance(p, ringDir, false)
	if err != nil {
		return nil, err
	}
	in.view = true
	if err := loadView(ringDir, &in.st); err != nil {
		return nil, err
	}
	return in, nil
}

// key is the named store key of path: cleaned, and with the cache root's
// unresolved form (rawCacheRoot) replaced by the canonical one.
func (in *instance) key(path string) string {
	path = filepath.Clean(path)
	if in.rawRoot != "" && strings.HasPrefix(path, in.rawRoot+string(filepath.Separator)) {
		return in.cacheRoot + path[len(in.rawRoot):]
	}
	return path
}

// named returns the committed named store at path, reconciling it with its
// legacy file on first use (once per open, failed or not).
func (in *instance) named(path string) (StoreFile, error) {
	path = in.key(path)
	if path == in.replyPath {
		return StoreFile{}, fmt.Errorf("wsmailbox: %s is the reply registry, not a named store", path)
	}
	if !in.loaded[path] && !in.view {
		if err := in.reconcileNamedPaths([]string{path}); err != nil {
			ringWarn("reconcile legacy mailbox file: %v", err)
		}
	}
	store, ok := in.st.Named[path]
	if !ok {
		return emptyStore(), nil
	}
	return store, nil
}

func (in *instance) nextStamp(at time.Time) time.Time {
	at = at.UTC()
	if !at.After(in.lastStamp) {
		at = in.lastStamp.Add(time.Nanosecond)
	}
	in.lastStamp = at
	return at
}

func (in *instance) update(op string, fn func(*Tx) error) ([]string, error) {
	tx := &Tx{in: in, named: map[string]*StoreFile{}, history: map[string][]HistoryEntry{}, listening: map[string]*ListeningMarker{}}
	if err := fn(tx); err != nil {
		return nil, err
	}
	rec, changed := tx.record(op)
	if rec.empty() {
		return nil, nil
	}
	if err := in.ring.append(rec); err != nil {
		return nil, err
	}
	in.st.apply(rec)
	in.maybeRotate()
	return changed, nil
}

// record builds tx's delta record and the store paths whose queues changed.
func (tx *Tx) record(op string) (*record, []string) {
	rec := &record{Kind: recordTx, Op: op, At: storeNow().Format(time.RFC3339Nano)}
	var changed []string
	for path, working := range tx.named {
		normalizeQueues(working.Queues)
		d := diffNamed(tx.in.st.Named[path], *working)
		if d == nil {
			continue
		}
		if rec.Named == nil {
			rec.Named = map[string]*namedDelta{}
		}
		rec.Named[path] = d
		if len(d.Queues) > 0 {
			changed = append(changed, path)
		}
	}
	if tx.reply != nil {
		normalizeQueues(tx.reply.Queues)
		if d := diffReply(tx.in.st.Reply, *tx.reply); d != nil {
			rec.Reply = d
			if len(d.Queues) > 0 {
				changed = append(changed, tx.in.replyPath)
			}
		}
	}
	if len(tx.history) > 0 {
		rec.History = tx.history
	}
	if len(tx.listening) > 0 {
		rec.Listening = tx.listening
	}
	slices.Sort(changed)
	return rec, changed
}

// --- copies, diffs, and replay -----------------------------------------

// cloneStore copies s: a fresh presence map and queue slices. deep is
// unused for named stores (presence values hold no references); it keeps
// the signature parallel to cloneReply.
func cloneStore(s StoreFile, _ bool) StoreFile {
	out := StoreFile{SchemaVersion: s.SchemaVersion, Presence: maps.Clone(s.Presence), Queues: copyQueues(s.Queues)}
	if out.SchemaVersion == 0 {
		out.SchemaVersion = schemaVersion
	}
	if out.Presence == nil {
		out.Presence = map[string]Presence{}
	}
	return out
}

// cloneReply copies r, each entry's Contacts map included: a transaction's
// fn may write a contact in place, and a shared map would change the
// committed state behind the diff, so the change would never reach the
// ring.
func cloneReply(r ReplyStore) ReplyStore {
	out := ReplyStore{SchemaVersion: r.SchemaVersion, Entries: maps.Clone(r.Entries), Queues: copyQueues(r.Queues)}
	if out.SchemaVersion == 0 {
		out.SchemaVersion = schemaVersion
	}
	if out.Entries == nil {
		out.Entries = map[string]ReplyEntry{}
	}
	for id, e := range out.Entries {
		e.Contacts = maps.Clone(e.Contacts)
		out.Entries[id] = e
	}
	return out
}

// normalizeQueues drops empty queues: an empty queue and an absent one are
// the same state.
func normalizeQueues(queues map[string][]Envelope) {
	for name, q := range queues {
		if len(q) == 0 {
			delete(queues, name)
		}
	}
}

// queueDelta turns a queue's previous value into its next: drop Drop
// envelopes from the front, then append Add.
type queueDelta struct {
	Drop int        `json:"drop,omitempty"`
	Add  []Envelope `json:"add,omitempty"`
}

// diffQueue finds the smallest front drop after which before's remainder
// opens after, so an append, a trim, and a drain each record only what
// changed; a change in place (a flag) falls back to a longer drop.
func diffQueue(before, after []Envelope) (queueDelta, bool) {
	for k := 0; k <= len(before); k++ {
		rest := before[k:]
		if len(rest) <= len(after) && slices.Equal(rest, after[:len(rest)]) {
			add := after[len(rest):]
			if k == 0 && len(add) == 0 {
				return queueDelta{}, false
			}
			return queueDelta{Drop: k, Add: slices.Clone(add)}, true
		}
	}
	return queueDelta{}, false // unreachable: k == len(before) always matches
}

func diffQueues(before, after map[string][]Envelope) map[string]queueDelta {
	var out map[string]queueDelta
	for name, a := range after {
		if d, ok := diffQueue(before[name], a); ok {
			if out == nil {
				out = map[string]queueDelta{}
			}
			out[name] = d
		}
	}
	for name, b := range before {
		if _, ok := after[name]; !ok && len(b) > 0 {
			if out == nil {
				out = map[string]queueDelta{}
			}
			out[name] = queueDelta{Drop: len(b)}
		}
	}
	return out
}

func applyQueues(queues map[string][]Envelope, deltas map[string]queueDelta) map[string][]Envelope {
	if len(deltas) == 0 {
		return queues
	}
	out := copyQueues(queues)
	for name, d := range deltas {
		q := out[name]
		q = append(q[min(d.Drop, len(q)):], d.Add...)
		if len(q) == 0 {
			delete(out, name)
		} else {
			out[name] = q
		}
	}
	return out
}

// namedDelta is one named store's change; a nil presence deletes it.
type namedDelta struct {
	Presence map[string]*Presence  `json:"presence,omitempty"`
	Queues   map[string]queueDelta `json:"queues,omitempty"`
}

func diffNamed(before, after StoreFile) *namedDelta {
	d := &namedDelta{Queues: diffQueues(before.Queues, after.Queues)}
	for name, p := range after.Presence {
		if b, ok := before.Presence[name]; !ok || b != p {
			if d.Presence == nil {
				d.Presence = map[string]*Presence{}
			}
			d.Presence[name] = &p
		}
	}
	for name := range before.Presence {
		if _, ok := after.Presence[name]; !ok {
			if d.Presence == nil {
				d.Presence = map[string]*Presence{}
			}
			d.Presence[name] = nil
		}
	}
	if d.Presence == nil && d.Queues == nil {
		return nil
	}
	return d
}

func applyNamed(s StoreFile, d *namedDelta) StoreFile {
	out := StoreFile{SchemaVersion: s.SchemaVersion, Presence: maps.Clone(s.Presence), Queues: applyQueues(s.Queues, d.Queues)}
	if out.SchemaVersion == 0 {
		out.SchemaVersion = schemaVersion
	}
	if out.Presence == nil {
		out.Presence = map[string]Presence{}
	}
	if out.Queues == nil {
		out.Queues = map[string][]Envelope{}
	}
	for name, p := range d.Presence {
		if p == nil {
			delete(out.Presence, name)
		} else {
			out.Presence[name] = *p
		}
	}
	return out
}

// replyDelta is the reply registry's change; a nil entry deletes it.
type replyDelta struct {
	Entries map[string]*ReplyEntry `json:"entries,omitempty"`
	Queues  map[string]queueDelta  `json:"queues,omitempty"`
}

func entryEqual(a, b ReplyEntry) bool {
	return a.LastSeen == b.LastSeen && a.Harness == b.Harness && a.Cwd == b.Cwd && maps.Equal(a.Contacts, b.Contacts)
}

func diffReply(before, after ReplyStore) *replyDelta {
	d := &replyDelta{Queues: diffQueues(before.Queues, after.Queues)}
	for id, e := range after.Entries {
		if b, ok := before.Entries[id]; !ok || !entryEqual(b, e) {
			if d.Entries == nil {
				d.Entries = map[string]*ReplyEntry{}
			}
			d.Entries[id] = &e
		}
	}
	for id := range before.Entries {
		if _, ok := after.Entries[id]; !ok {
			if d.Entries == nil {
				d.Entries = map[string]*ReplyEntry{}
			}
			d.Entries[id] = nil
		}
	}
	if d.Entries == nil && d.Queues == nil {
		return nil
	}
	return d
}

func applyReply(r ReplyStore, d *replyDelta) ReplyStore {
	out := ReplyStore{SchemaVersion: r.SchemaVersion, Entries: maps.Clone(r.Entries), Queues: applyQueues(r.Queues, d.Queues)}
	if out.SchemaVersion == 0 {
		out.SchemaVersion = schemaVersion
	}
	if out.Entries == nil {
		out.Entries = map[string]ReplyEntry{}
	}
	if out.Queues == nil {
		out.Queues = map[string][]Envelope{}
	}
	for id, e := range d.Entries {
		if e == nil {
			delete(out.Entries, id)
		} else {
			out.Entries[id] = *e
		}
	}
	return out
}

// apply replays a tx record onto st, replacing (never writing into) every
// value it changes, so copies already handed out stay unchanged.
func (st *state) apply(rec *record) {
	if len(rec.Named) > 0 {
		named := maps.Clone(st.Named)
		for path, d := range rec.Named {
			named[path] = applyNamed(named[path], d)
		}
		st.Named = named
	}
	if rec.Reply != nil {
		st.Reply = applyReply(st.Reply, rec.Reply)
	}
	if len(rec.History) > 0 {
		history := maps.Clone(st.History)
		for key, entries := range rec.History {
			window := append(slices.Clone(history[key]), entries...)
			// Names lead with their stamp: an imported older entry sorts
			// into its place.
			sort.SliceStable(window, func(i, j int) bool { return window[i].Name < window[j].Name })
			history[key] = trimHistory(window)
		}
		st.History = history
	}
	if len(rec.Listening) > 0 {
		listening := maps.Clone(st.Listening)
		for key, m := range rec.Listening {
			if m == nil {
				delete(listening, key)
			} else {
				listening[key] = *m
			}
		}
		st.Listening = listening
	}
	if rec.Baseline != nil {
		st.Baseline = st.Baseline.merge(*rec.Baseline)
	}
}

// reap applies the lifetimes a rotation carries forward: a session's
// history window idle past HistoryRetention goes, and a reply entry expires
// as ReapReplyEntries rules. Queues, presence, and markers keep theirs.
func (st *state) reap(now time.Time) {
	history := maps.Clone(st.History)
	for key, window := range history {
		if historyIdle(window, now) {
			delete(history, key)
		}
	}
	st.History = history
	reply := cloneReply(st.Reply)
	ReapReplyEntries(&reply, now)
	st.Reply = reply
}

// NamedQueue returns name's presence (ok false when none) and a copy of its
// queue in the named store at path.
func (s Store) NamedQueue(path, name string) (presence Presence, ok bool, queue []Envelope, err error) {
	err = s.read(func(in *instance) error {
		store, err := in.named(path)
		if err != nil {
			return err
		}
		presence, ok = store.Presence[name]
		queue = slices.Clone(store.Queues[name])
		return nil
	})
	return presence, ok, queue, err
}

// ReplyQueue returns a copy of reply-id id's queue.
func (s Store) ReplyQueue(id string) ([]Envelope, error) {
	var queue []Envelope
	err := s.read(func(in *instance) error {
		queue = slices.Clone(in.st.Reply.Queues[id])
		return nil
	})
	return queue, err
}
