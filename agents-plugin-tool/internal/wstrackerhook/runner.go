package wstrackerhook

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/kang-sw/devenv/internal/wsledger"
	"github.com/kang-sw/devenv/internal/wstracker"
	"github.com/kang-sw/devenv/internal/wstrackerhost"
)

const (
	// RunTimeout bounds one run of a hook command; the run's process tree
	// is killed when it passes.
	RunTimeout = 30 * time.Second
	// fileCheckInterval is how often the hooks file is checked for a change.
	fileCheckInterval = 10 * time.Second
	// scanInterval is how often every store is read with no change signal:
	// the due_soon clock, and a backstop for a missed signal.
	scanInterval = time.Minute
	// readTimeout bounds one store read, which waits for the store's lock.
	readTimeout = 2 * time.Minute
)

// Source is the tracker the runner reads: *wstrackerhost.Host.
type Source interface {
	Stores() ([]wstrackerhost.StoreEntry, error)
	ReadActivity(ctx context.Context, id string, after wsledger.Cursor, want func(id wsledger.EventID, ts time.Time, catchUp bool) bool) (wstrackerhost.ActivityRead, error)
}

// Exec runs one hook command: argv without a shell, in dir, with stdin.
type Exec func(ctx context.Context, argv []string, dir string, stdin []byte) error

// Options configures a Runner.
type Options struct {
	// Dir is the dashboard daemon's state directory: it holds the hooks
	// file and the hook state, and is each command's working directory.
	Dir    string
	Source Source
	// BaseURL returns the dashboard's origin ("http://127.0.0.1:<port>")
	// an event's url opens the item under; nil or "" leaves url empty.
	BaseURL func() string
	// Log receives the runner's diagnostics; nil discards them.
	Log io.Writer
	// Now is the clock; nil is time.Now.
	Now func() time.Time
	// Location is where a due date's day starts; nil is time.Local.
	Location *time.Location
	// Exec runs a command; nil runs it as RunCommand does, bounded by
	// RunTimeout.
	Exec Exec
}

// Runner delivers tracker events to the hooks the hooks file names. Each
// hook has its own serial queue; a failed run is logged and not retried.
type Runner struct {
	dir  string
	src  Source
	base func() string
	log  io.Writer
	now  func() time.Time
	loc  *time.Location
	exec Exec

	// mu guards the hook set and the settings the file gives.
	mu      sync.Mutex
	workers map[string]*worker
	order   []string // hook keys in file order
	dueSoon time.Duration
	// active: the hooks file exists (valid or not). Without it nothing is
	// read or written.
	active bool
	// fileStamp is the hooks file's size and modification time at the
	// last load; loaded reports that a load ran.
	fileStamp string
	loaded    bool
	// fresh: the hooks file appeared while the daemon ran, so the next
	// scan starts every store at its current head.
	fresh bool

	// closed: Close ran; no hook is added after it.
	closed bool

	// pending counts the queued and running deliveries (a test seam).
	pending sync.WaitGroup

	// started: Start runs loop, which closes done when it returns.
	started bool
	nudge   chan struct{}
	stop    chan struct{}
	done    chan struct{}
	ctx     context.Context
	halt    context.CancelFunc
}

// worker is one hook's serial queue.
type worker struct {
	hook  Hook
	mu    sync.Mutex
	queue [][]byte
	wake  chan struct{}
	quit  chan struct{}
}

func newRunner(opts Options) *Runner {
	r := &Runner{dir: opts.Dir, src: opts.Source, base: opts.BaseURL, log: opts.Log, now: opts.Now, loc: opts.Location,
		exec: opts.Exec, workers: map[string]*worker{}, dueSoon: DefaultDueSoon,
		nudge: make(chan struct{}, 1), stop: make(chan struct{}), done: make(chan struct{})}
	r.ctx, r.halt = context.WithCancel(context.Background())
	if r.log == nil {
		r.log = io.Discard
	}
	if r.now == nil {
		r.now = time.Now
	}
	if r.loc == nil {
		r.loc = time.Local
	}
	if r.exec == nil {
		r.exec = func(ctx context.Context, argv []string, dir string, stdin []byte) error {
			return RunCommand(ctx, argv, dir, stdin, RunTimeout)
		}
	}
	return r
}

// Start loads the hooks file and runs the runner until Close: it reads
// every store at once, at each Nudge, and every scanInterval, and checks
// the hooks file every fileCheckInterval.
func Start(opts Options) *Runner {
	r := newRunner(opts)
	r.started = true
	go r.loop()
	return r
}

// Nudge asks for a read of every store soon: a store may have changed.
// Safe from any goroutine; calls while a read is due coalesce.
func (r *Runner) Nudge() {
	select {
	case r.nudge <- struct{}{}:
	default:
	}
}

// Close stops the runner: a running command is killed, and the queued
// events not yet run are dropped. The stores' cursors already count them
// as delivered (at-most-once beyond the queue).
func (r *Runner) Close() {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return
	}
	r.closed = true
	for key, w := range r.workers {
		close(w.quit)
		delete(r.workers, key)
	}
	r.order = nil
	r.mu.Unlock()
	close(r.stop)
	r.halt()
	if r.started {
		<-r.done
	}
}

func (r *Runner) loop() {
	defer close(r.done)
	r.reload()
	r.scanAll()
	files := time.NewTicker(fileCheckInterval)
	defer files.Stop()
	last := r.now()
	for {
		select {
		case <-r.stop:
			return
		case <-r.nudge:
			r.scanAll()
			last = r.now()
		case <-files.C:
			if r.reload() || r.now().Sub(last) >= scanInterval {
				r.scanAll()
				last = r.now()
			}
		}
	}
}

func (r *Runner) logf(format string, args ...any) {
	fmt.Fprintf(r.log, "tracker hooks: "+format+"\n", args...)
}

// reload loads the hooks file when it changed since the last load, and
// reports whether it did. An absent file turns the runner idle; an invalid
// one is logged and runs no hook, while the stores' cursors still advance,
// so fixing it starts no backfill. Hooks whose command and scope stay keep
// their queues; a removed hook's queue is dropped once its running command
// finishes, and an added hook starts empty.
func (r *Runner) reload() bool {
	path := filepath.Join(r.dir, FileName)
	info, statErr := os.Stat(path)
	stamp := ""
	if statErr == nil {
		stamp = fmt.Sprintf("%d %d", info.Size(), info.ModTime().UnixNano())
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed || (r.loaded && stamp == r.fileStamp) {
		return false
	}
	first := !r.loaded
	r.loaded, r.fileStamp = true, stamp
	var cfg Config
	switch {
	case errors.Is(statErr, fs.ErrNotExist):
		if r.active {
			r.logf("%s removed; no hook runs", path)
		}
		r.active = false
	case statErr != nil:
		r.logf("%s: %v; no hook runs", path, statErr)
		r.activate(first)
	default:
		data, err := os.ReadFile(path)
		if err == nil {
			cfg, err = parseConfig(data)
		}
		if err != nil {
			r.logf("%s is invalid, so no hook runs until it is fixed: %v", path, err)
			cfg = Config{}
		} else {
			r.logf("%s loaded: %d hook(s)", path, len(cfg.Hooks))
		}
		r.activate(first)
	}
	r.dueSoon = cfg.dueSoon()
	keep := map[string]bool{}
	r.order = r.order[:0]
	for _, h := range cfg.Hooks {
		key := h.key()
		keep[key] = true
		r.order = append(r.order, key)
		if r.workers[key] == nil {
			w := &worker{hook: h, wake: make(chan struct{}, 1), quit: make(chan struct{})}
			r.workers[key] = w
			go r.work(w)
		}
	}
	for key, w := range r.workers {
		if !keep[key] {
			close(w.quit)
			delete(r.workers, key)
		}
	}
	return true
}

// activate turns the runner active; a file that appears while the daemon
// runs (not at its start) starts every store at its current head.
// Callers hold r.mu.
func (r *Runner) activate(first bool) {
	if !r.active && !first {
		r.fresh = true
	}
	r.active = true
}

// scanAll reads every registered store and queues its events.
func (r *Runner) scanAll() {
	r.mu.Lock()
	active, fresh := r.active, r.fresh
	r.fresh = false
	r.mu.Unlock()
	if !active {
		return
	}
	stores, err := r.src.Stores()
	if err != nil {
		r.logf("list stores: %v", err)
		return
	}
	for _, e := range stores {
		select {
		case <-r.stop:
			return
		default:
		}
		r.scan(e, fresh)
	}
}

// scan reads store e since its cursor, queues to each hook the events its
// scope accepts, and saves the advanced cursor and processed ids. fresh
// starts the store at its current head: nothing it already holds is
// delivered.
//
// A catch-up read (the cursor is of another Ledger instance, as after
// every daemon start) returns every event; those older than the window are
// dropped, and the processed ids keep the rest from being delivered twice.
// Events seen by a read that is not a catch-up are delivered whatever their
// ts, so a teammate's event that syncs late is not lost. An event still
// waiting for its verdict is left for a later read.
func (r *Runner) scan(e wstrackerhost.StoreEntry, fresh bool) {
	st, ok := loadState(r.dir, e.ID, e.Nonce)
	fresh = fresh || !ok
	if fresh {
		st.Cursor, st.Processed = wsledger.Cursor{}, map[string]mark{}
	}
	now := r.now()
	ctx, cancel := context.WithTimeout(r.ctx, readTimeout)
	defer cancel()
	rd, err := r.src.ReadActivity(ctx, e.ID, st.Cursor, func(id wsledger.EventID, ts time.Time, catchUp bool) bool {
		if fresh {
			return false
		}
		if _, done := st.Processed[id.String()]; done {
			return false
		}
		return !catchUp || now.Sub(ts) <= window
	})
	if err != nil {
		r.logf("read store %q: %v", e.ID, err)
		return
	}
	e = rd.Entry
	base := ""
	if r.base != nil {
		base = r.base()
	}
	var cands []candidate
	for _, ev := range rd.Events {
		key := ev.ID.String()
		if _, done := st.Processed[key]; done {
			continue
		}
		if fresh {
			if now.Sub(ev.TS) <= window {
				st.Processed[key] = mark{At: now, TS: ev.TS}
			}
			continue
		}
		if !ev.Chosen || !ev.Settled {
			continue
		}
		for _, a := range ev.Activity {
			cands = append(cands, activityCandidate(e, rd.View, a, base, r.loc))
		}
		st.Processed[key] = mark{At: now, TS: ev.TS}
	}
	r.mu.Lock()
	dueSoon := r.dueSoon
	r.mu.Unlock()
	cands = append(cands, dueCandidates(e, rd.View, &st, now, dueSoon, base, r.loc)...)
	r.queue(e.Member, cands)
	st.Cursor = rd.Cursor
	st.prune(now, r.loc)
	if err := saveState(r.dir, e.ID, st); err != nil {
		r.logf("save the state of store %q: %v", e.ID, err)
	}
}

// dueCandidates are the due_soon events of store e's items now: an item
// with a due date and a live assignment not done whose due date starts
// within dueSoon, and has not ended, fires once per (item, due date),
// recorded in st.
func dueCandidates(e wstrackerhost.StoreEntry, v *wstracker.View, st *storeState, now time.Time, dueSoon time.Duration, base string, loc *time.Location) []candidate {
	var out []candidate
	for _, id := range v.ItemOrder {
		it, ok := v.Items[id]
		if !ok || it.Hidden || it.Due == nil {
			continue
		}
		at, ok := dueTime(*it.Due, loc)
		if !ok || now.Before(at.Add(-dueSoon)) || !now.Before(at.Add(window)) {
			continue
		}
		key := id + ":" + *it.Due
		if _, fired := st.Due[key]; fired {
			continue
		}
		var holders []string
		for _, aid := range it.Assignments {
			if a, ok := v.Assignments[aid]; ok && a.State == "live" && a.Status != "done" && a.Status != "dropped" {
				holders = append(holders, a.To)
			}
		}
		if len(holders) == 0 {
			continue
		}
		st.Due[key] = now
		ev := eventBase(e, v, id, base, loc)
		ev.ID, ev.Kind, ev.TS = "due:"+key, KindDueSoon, now.UTC().Format(time.RFC3339)
		out = append(out, candidate{ev: ev, concerns: holders})
	}
	return out
}

// queue hands each hook, in order, the candidates its scope accepts.
func (r *Runner) queue(member string, cands []candidate) {
	if len(cands) == 0 {
		return
	}
	payloads := make([][]byte, len(cands))
	for i, c := range cands {
		data, err := json.Marshal(c.ev)
		if err != nil {
			r.logf("encode event %s: %v", c.ev.ID, err)
			continue
		}
		payloads[i] = append(data, '\n')
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, key := range r.order {
		w := r.workers[key]
		var add [][]byte
		for i, c := range cands {
			if payloads[i] != nil && w.hook.accepts(c, member) {
				add = append(add, payloads[i])
			}
		}
		if len(add) == 0 {
			continue
		}
		r.pending.Add(len(add))
		w.mu.Lock()
		w.queue = append(w.queue, add...)
		w.mu.Unlock()
		select {
		case w.wake <- struct{}{}:
		default:
		}
	}
}

// work runs w's queue in order until w quits; what is left queued then is
// dropped.
func (r *Runner) work(w *worker) {
	defer func() {
		w.mu.Lock()
		r.pending.Add(-len(w.queue))
		w.queue = nil
		w.mu.Unlock()
	}()
	for {
		w.mu.Lock()
		var next []byte
		if len(w.queue) > 0 {
			next, w.queue = w.queue[0], w.queue[1:]
		}
		w.mu.Unlock()
		if next == nil {
			select {
			case <-w.quit:
				return
			case <-w.wake:
				continue
			}
		}
		select {
		case <-w.quit:
			r.pending.Done()
			return
		default:
		}
		if err := r.exec(r.ctx, w.hook.Command, r.dir, next); err != nil {
			r.logf("hook %q failed: %v", w.hook.Command[0], err)
		}
		r.pending.Done()
	}
}
