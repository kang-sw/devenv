package wsdashboard

import (
	"path/filepath"
	"reflect"
	"sync"
	"time"
)

// The session rail's worktree lists: for each repository the rail shows,
// the worktrees git lists (git worktree list --porcelain, which reads the
// repository's worktree records only, never a working tree, and never
// fetches) and the repository's worktree pool roots. Like the git badge
// (gitbadge.go) the lists are read only while some page showing the rail
// holds its stream open: when the first such stream opens (unless the last
// full read is under gitReadInterval old, as it is when a page moves to
// another session through the rail: its stream closes, the next one opens),
// then checks Git metadata each gitReadInterval until the last one closes,
// forcing complete reads within five minutes even when metadata stays unchanged.
// Explicit refresh, mutation rereads and opt-in visible resumption bypass the
// hint. It also reads at once when a page asks
// (POST /rail/refresh, the rail header's refresh button). A repository the
// rail starts to show between those reads (a session registered in it) is
// read on its own at once; the others keep their lists. One goroutine runs
// every read, so a burst of refresh requests never runs git concurrently.
// With no viewer a render shows the lists last read, and reads nothing.

// railListRepoKey is the broker token the worktree lister wakes after a
// read that changed a list; the rail streams depend on every token, so the
// name only has to be one no session or git root takes.
const railListRepoKey = "rail:worktrees"

// RepoWorktreeSource is an optional interface of a WorktreeSource: the
// worktrees of one repository and its worktree pool roots, which the
// session rail lists. regs are the registrations of the sessions the rail
// shows in the repository (none for a stored workspace without a session),
// so the pool roots can resolve as one of their sessions resolves its
// worktree_pool (a source falls back to the default value when none
// resolves). What it returns reaches a browser, so it never
// carries a session key.
type RepoWorktreeSource interface {
	ReadRepoWorktrees(repo string, regs []Registration) (RepoWorktrees, error)
}

// RepoWorktrees is a repository's worktree list.
type RepoWorktrees struct {
	// Worktrees are what git lists, the root worktree first.
	Worktrees []RepoWorktree
	// Pools are the repository's worktree pool roots.
	Pools []string
}

// RepoWorktree is one listed worktree.
type RepoWorktree struct {
	Path string
	// Branch is the checked-out branch's short name; empty when Detached.
	Branch   string
	Detached bool
	// Prunable reports a worktree git marks prunable: its folder is gone.
	Prunable bool
}

// railLister reads the rail's worktree lists (see the file comment).
type railLister struct {
	admission *railAdmissionTracker
	// read reads one repository; it runs on the lister's goroutine, never
	// under mu. Nil lists nothing.
	read func(repo string, regs []Registration) (RepoWorktrees, error)
	// interval is the period between full reads.
	interval time.Duration
	// after is time.After and now time.Now, injected so a test drives the
	// clock.
	after func(time.Duration) <-chan time.Time
	now   func() time.Time
	probe func(string) (string, error)
	// changed is called after a read that changed a list.
	changed func()
	// sweep runs before each full read, on the lister's goroutine and never
	// under mu: the stored workspaces' own re-read (workspaceStore.sweep).
	// It reports a change the rail shows. Nil sweeps nothing.
	sweep func() bool
	stop  <-chan struct{}

	mu      sync.Mutex
	viewers int
	running bool
	wake    chan struct{}
	// full asks the next read for every wanted repository, not only the
	// pending ones; lastFull is when the last full read began (zero before
	// one).
	full     bool
	force    bool // explicit refresh/resumption; timer checks may use metadata
	lastFull time.Time
	gates    map[string]gitReadGate
	// wanted holds the repositories the last rail render showed, with the
	// registrations of their sessions; pending the wanted ones not read yet.
	wanted  map[string][]Registration
	pending map[string]bool
	repos   map[string]railRepo
	// reading holds the repositories the goroutine is reading now: a render
	// that lands meanwhile (a second stream opening) waits for that read
	// rather than queuing the repository again.
	reading map[string]bool
	// reads counts the repository reads run (a test seam).
	reads int
}

func newRailLister(read func(string, []Registration) (RepoWorktrees, error), changed func(), stop <-chan struct{}) *railLister {
	return &railLister{
		read:     read,
		interval: gitReadInterval,
		after:    time.After,
		now:      time.Now,
		probe:    gitMetadataStamp,
		gates:    map[string]gitReadGate{},
		changed:  changed,
		stop:     stop,
		wake:     make(chan struct{}, 1),
		wanted:   map[string][]Registration{},
		pending:  map[string]bool{},
		repos:    map[string]railRepo{},
		reading:  map[string]bool{},
	}
}

// watch adds a viewer and returns its release. The first viewer starts the
// lister's goroutine, which reads every wanted repository at once unless
// the last full read is under the interval old; the next then comes when
// the interval since it ends.
func (l *railLister) watch() (release func()) { return l.watchWithRefresh(false) }

// watchFresh is opt-in visible resumption; legacy stream lifetimes retain
// their recent-list reuse across navigation.
func (l *railLister) watchFresh() (release func()) { return l.watchWithRefresh(true) }

func (l *railLister) watchWithRefresh(fresh bool) (release func()) {
	if l == nil {
		return func() {}
	}
	l.mu.Lock()
	l.viewers++
	if fresh {
		l.full, l.force = true, true
		signal(l.wake)
	}
	if l.viewers == 1 {
		if l.lastFull.IsZero() || l.now().Sub(l.lastFull) >= l.interval {
			l.full, l.force = true, true
		}
		signal(l.wake)
		if !l.running {
			l.running = true
			go l.run()
		}
	}
	l.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			l.mu.Lock()
			l.viewers--
			if l.viewers == 0 {
				signal(l.wake)
			}
			l.mu.Unlock()
		})
	}
}

// refresh asks for an immediate read of every wanted repository; false when
// no page watches, which reads nothing.
func (l *railLister) refresh() bool {
	if l == nil {
		return false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.viewers == 0 {
		return false
	}
	l.full, l.force = true, true
	signal(l.wake)
	return true
}

// reread asks for an immediate read of one repository, the others keeping
// their lists: after the daemon added a worktree to it. With no page
// watching, the list it holds is dropped instead, so the next render that
// wants it reads it.
func (l *railLister) reread(repo string) {
	if l == nil {
		return
	}
	key := filepath.Clean(repo)
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.viewers == 0 {
		delete(l.repos, key)
		delete(l.gates, key)
		return
	}
	if _, ok := l.wanted[key]; ok {
		l.pending[key] = true
		signal(l.wake)
	}
}

// lists records the repositories a render shows (with their sessions'
// registrations) and returns the lists read so far. A wanted repository not
// read yet is read at once while a page watches; one no render wants any
// more is forgotten.
func (l *railLister) lists(wanted map[string][]Registration) map[string]railRepo {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make(map[string]railRepo, len(wanted))
	next := make(map[string][]Registration, len(wanted))
	missing := false
	for repo, regs := range wanted {
		key := filepath.Clean(repo)
		next[key] = regs
		if r, ok := l.repos[key]; ok {
			out[key] = r
		} else if !l.pending[key] && !l.reading[key] {
			l.pending[key] = true
			missing = true
		}
	}
	for key := range l.repos {
		if _, ok := next[key]; !ok {
			delete(l.repos, key)
			delete(l.gates, key)
		}
	}
	for key := range l.pending {
		if _, ok := next[key]; !ok {
			delete(l.pending, key)
		}
	}
	l.wanted = next
	if missing && l.viewers > 0 {
		signal(l.wake)
	}
	return out
}

// run is the lister's goroutine: it reads on each wake or interval while a
// page watches, and ends once none does.
func (l *railLister) run() {
	// The first wake of a run arms the interval from the last full read
	// when it reads less than everything (a recent full read was kept).
	var tick <-chan time.Time
	armed := false
	for {
		select {
		case <-l.stop:
			l.mu.Lock()
			l.running = false
			l.mu.Unlock()
			return
		case <-l.wake:
		case <-tick:
			l.mu.Lock()
			l.full = true
			l.mu.Unlock()
		}
		l.mu.Lock()
		if l.viewers == 0 {
			l.running = false
			l.mu.Unlock()
			return
		}
		full, force := l.full, l.force
		l.full, l.force = false, false
		if full {
			l.lastFull = l.now()
		}
		rearm := time.Duration(-1)
		if !full && !armed {
			rearm = max(0, l.interval-l.now().Sub(l.lastFull))
		}
		todo := map[string][]Registration{}
		forced := map[string]bool{}
		for repo, regs := range l.wanted {
			if full || l.pending[repo] {
				todo[repo] = regs
				forced[repo] = force || l.pending[repo]
				l.reading[repo] = true
			}
		}
		l.pending = map[string]bool{}
		l.mu.Unlock()
		changed := false
		if full && l.sweep != nil && l.sweep() {
			changed = true
		}
		for repo, regs := range todo {
			l.mu.Lock()
			gate := l.gates[repo]
			l.mu.Unlock()
			stamp, probeErr := l.probe(repo)
			if probeErr != nil {
				stamp = ""
			}
			if !gate.needed(stamp, l.now(), forced[repo]) {
				l.mu.Lock()
				delete(l.reading, repo)
				l.mu.Unlock()
				continue
			}
			r := l.readRepo(repo, regs)
			gate.complete(stamp, l.now(), r.Read)
			l.mu.Lock()
			l.reads++
			delete(l.reading, repo)
			if _, ok := l.wanted[repo]; ok {
				if old, had := l.repos[repo]; !had || !reflect.DeepEqual(old, r) {
					changed = true
				}
				l.repos[repo] = r
				l.gates[repo] = gate
			}
			l.mu.Unlock()
		}
		if full || rearm >= 0 {
			delay := l.interval
			if !full {
				delay = rearm
			}
			l.mu.Lock()
			for repo := range l.wanted {
				delay = min(delay, l.gates[repo].delay(l.now(), l.interval))
			}
			l.mu.Unlock()
			tick, armed = l.after(delay), true
		}
		if changed && l.changed != nil {
			l.changed()
		}
	}
}

// readRepo reads one repository. A failed read keeps no list: the rail then
// shows the worktrees the repository's sessions are rooted in.
func (l *railLister) readRepo(repo string, regs []Registration) railRepo {
	if l.read == nil {
		return railRepo{}
	}
	got, err := l.read(repo, regs)
	if err != nil {
		return railRepo{}
	}
	r := railRepo{Read: true, Pools: got.Pools}
	var observations map[string]admissionObservation
	if l.admission != nil {
		observations, err = l.admission.observe(repo, got)
		if err != nil {
			return railRepo{}
		}
	}
	for _, w := range got.Worktrees {
		o := observations[filepath.Clean(w.Path)]
		r.Entries = append(r.Entries, railWorktreeEntry{Path: filepath.Clean(w.Path), Branch: w.Branch, Detached: w.Detached, Prunable: w.Prunable, AdmissionRepository: o.identity.root, AdmissionPath: o.identity.path, Revision: o.revision})
	}
	return r
}
