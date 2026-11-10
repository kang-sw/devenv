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
// holds its stream open: once when the first such stream opens, then every
// gitReadInterval until the last one closes, and at once when a page asks
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
// shows in the repository, so the pool roots can resolve as one of their
// sessions resolves its worktree_pool (a source falls back to the default
// value when none resolves). What it returns reaches a browser, so it never
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
	// read reads one repository; it runs on the lister's goroutine, never
	// under mu. Nil lists nothing.
	read func(repo string, regs []Registration) (RepoWorktrees, error)
	// interval is the period between full reads.
	interval time.Duration
	// after is time.After, injected so a test drives the clock.
	after func(time.Duration) <-chan time.Time
	// changed is called after a read that changed a list.
	changed func()
	stop    <-chan struct{}

	mu      sync.Mutex
	viewers int
	running bool
	wake    chan struct{}
	// full asks the next read for every wanted repository, not only the
	// pending ones.
	full bool
	// wanted holds the repositories the last rail render showed, with the
	// registrations of their sessions; pending the wanted ones not read yet.
	wanted  map[string][]Registration
	pending map[string]bool
	repos   map[string]railRepo
	// reads counts the repository reads run (a test seam).
	reads int
}

func newRailLister(read func(string, []Registration) (RepoWorktrees, error), changed func(), stop <-chan struct{}) *railLister {
	return &railLister{
		read:     read,
		interval: gitReadInterval,
		after:    time.After,
		changed:  changed,
		stop:     stop,
		wake:     make(chan struct{}, 1),
		wanted:   map[string][]Registration{},
		pending:  map[string]bool{},
		repos:    map[string]railRepo{},
	}
}

// watch adds a viewer and returns its release. The first viewer starts the
// lister's goroutine, which reads every wanted repository at once.
func (l *railLister) watch() (release func()) {
	if l == nil {
		return func() {}
	}
	l.mu.Lock()
	l.viewers++
	if l.viewers == 1 {
		l.full = true
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
	l.full = true
	signal(l.wake)
	return true
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
		} else if !l.pending[key] {
			l.pending[key] = true
			missing = true
		}
	}
	for key := range l.repos {
		if _, ok := next[key]; !ok {
			delete(l.repos, key)
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
	var tick <-chan time.Time
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
		full := l.full
		l.full = false
		todo := map[string][]Registration{}
		for repo, regs := range l.wanted {
			if full || l.pending[repo] {
				todo[repo] = regs
			}
		}
		l.pending = map[string]bool{}
		l.mu.Unlock()
		changed := false
		for repo, regs := range todo {
			r := l.readRepo(repo, regs)
			l.mu.Lock()
			l.reads++
			if _, ok := l.wanted[repo]; ok {
				if old, had := l.repos[repo]; !had || !reflect.DeepEqual(old, r) {
					changed = true
				}
				l.repos[repo] = r
			}
			l.mu.Unlock()
		}
		if full {
			tick = l.after(l.interval)
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
	for _, w := range got.Worktrees {
		r.Entries = append(r.Entries, railWorktreeEntry{Path: filepath.Clean(w.Path), Branch: w.Branch, Detached: w.Detached, Prunable: w.Prunable})
	}
	return r
}
