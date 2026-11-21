package wswhiteboard

import "sync"

// Load and dashboard admission share process memory, not presentation caches
// or per-send file probes. The daemon hosts both paths. Ordinary board edits
// retain LoadedAt; only load changes it. An empty token names a never-loaded
// board, distinct from a legacy request that carries no token at all.
var boardIncarnations sync.Map

type boardIncarnation struct {
	mu      sync.Mutex
	loads   sync.Mutex
	token   string
	ready   bool
	loading bool
}

func (f FolderStore) incarnation() *boardIncarnation {
	if v, ok := boardIncarnations.Load(f.Dir); ok {
		return v.(*boardIncarnation)
	}
	v, _ := boardIncarnations.LoadOrStore(f.Dir, new(boardIncarnation))
	return v.(*boardIncarnation)
}

func (f FolderStore) rememberIncarnation(token string) {
	g := f.incarnation()
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.loading {
		g.token, g.ready = token, true
	}
}

// LockIncarnation admits a tagged send until unlock, including its inbox
// enqueue. A replacing or not-yet-opened board fails closed without file I/O.
func (f FolderStore) LockIncarnation(token string) (unlock func(), ok bool) {
	return LockBoardIncarnation(f.Dir, token)
}

// LockBoardIncarnation admits a send for a currently open board incarnation.
func LockBoardIncarnation(dir, token string) (func(), bool) {
	g := (FolderStore{Dir: dir}).incarnation()
	g.mu.Lock()
	if !g.ready || g.loading || token != g.token {
		g.mu.Unlock()
		return nil, false
	}
	return g.mu.Unlock, true
}

func (f FolderStore) beginIncarnationLoad() func(string, error) {
	g := f.incarnation()
	g.loads.Lock()
	g.mu.Lock() // finish already-admitted sends before beginning replacement
	g.loading = true
	g.mu.Unlock()
	return func(token string, err error) {
		g.mu.Lock()
		g.token, g.ready, g.loading = token, err == nil, false
		g.mu.Unlock()
		g.loads.Unlock()
	}
}
