package wsdashboard

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/kang-sw/devenv/internal/wsi18n"
)

// The pooled-worktree routes. GET /s/<reply-id>/worktrees answers the
// worktree pool of the session's project as JSON (WorktreePool), with each
// worktree's disk size when one is known; ?wait=1 holds the answer until the
// sizes the read started are measured (worktreeSizeWait at most), so a page
// draws the list at once and asks again for the sizes. POST
// /s/<reply-id>/worktrees/cleanup holds one message in the session's inbox,
// through the composer's own path: a line in the page's language saying the
// user asks for unused worktrees to be cleaned up, then the pool as plain
// text. The dashboard only reads the pool: removing or releasing a worktree
// is left to the session the message asks.

// WorktreeSource reads the worktree pool of a registered session's project.
// What it returns reaches a browser, so it never carries a session key.
type WorktreeSource interface {
	ReadWorktrees(reg Registration) (WorktreePool, error)
}

// WorktreePool is a project's worktree pool.
type WorktreePool struct {
	// Pool is the resolved worktree_pool location.
	Pool string `json:"pool"`
	// Worktrees are the pooled worktrees, ordered by path.
	Worktrees []PooledWorktree `json:"worktrees"`
	// Sizing reports, in a route's answer, that a size is still being
	// measured.
	Sizing bool `json:"sizing,omitempty"`
}

// PooledWorktree is one worktree of a pool.
type PooledWorktree struct {
	// Name is the worktree directory's name.
	Name string `json:"name"`
	Path string `json:"path"`
	// Profile is the worktree profile it belongs to ("legacy" for one from
	// before profiles), empty when unreadable.
	Profile string `json:"profile"`
	// Branch is the checked-out branch's short name, empty when Detached.
	Branch   string `json:"branch,omitempty"`
	Detached bool   `json:"detached"`
	// Leased reports a worktree lease: a session holds the worktree. Holder
	// and Parent are the reply-ids of the lease's worker and parent keys.
	Leased bool   `json:"leased"`
	Holder string `json:"holder,omitempty"`
	Parent string `json:"parent,omitempty"`
	// AcquiredAt is the lease's acquire time, ActiveAt the last activity
	// (the later of that and the HEAD commit time), both epoch ms, 0 when
	// unknown.
	AcquiredAt int64 `json:"acquired_at,omitempty"`
	ActiveAt   int64 `json:"active_at,omitempty"`
	// Quarantined reports a failed profile hook: the pool never reuses it.
	Quarantined bool `json:"quarantined,omitempty"`
	// Size is the worktree directory's disk use in bytes, nil until
	// measured.
	Size *int64 `json:"size"`
}

const (
	// worktreeSizeTTL is how long a measured size answers before a read
	// measures it again.
	worktreeSizeTTL = 2 * time.Minute
	// worktreeSizeWait bounds a ?wait=1 read's wait for the sizes.
	worktreeSizeWait = 20 * time.Second
	// worktreeSizeWalkers bounds the walks that run at once.
	worktreeSizeWalkers = 2
	// worktreeTextMax bounds the pool's text in the cleanup message, well
	// below the inbox's 64 KiB cap on one send's text, so a large pool
	// still sends.
	worktreeTextMax = 48 << 10
)

// sizeCache measures worktree sizes off the request path: a read takes the
// fresh sizes it has and starts a walk for each path it lacks, one walk per
// path at a time.
type sizeCache struct {
	mu      sync.Mutex
	entries map[string]*sizeEntry
	sem     chan struct{}
	// walk measures one path; dirSize unless a test replaces it.
	walk func(path string) int64
}

type sizeEntry struct {
	bytes int64
	at    time.Time
	// measured reports that bytes and at hold a finished walk's figure;
	// running, that a walk is on, which closes done when it ends.
	measured, running bool
	done              chan struct{}
}

func newSizeCache() *sizeCache {
	return &sizeCache{entries: map[string]*sizeEntry{}, sem: make(chan struct{}, worktreeSizeWalkers), walk: dirSize}
}

// fill sets each worktree's Size from the last measurement, starting a walk
// for every one whose measurement is missing or stale (a stale size still
// answers meanwhile), and returns the walks running for those with none.
func (c *sizeCache) fill(wts []PooledWorktree, now time.Time) (pending []chan struct{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for path, e := range c.entries {
		if !e.running && now.Sub(e.at) > 4*worktreeSizeTTL {
			delete(c.entries, path)
		}
	}
	for i := range wts {
		path := wts[i].Path
		e := c.entries[path]
		if e == nil {
			e = &sizeEntry{}
			c.entries[path] = e
		}
		if e.measured {
			n := e.bytes
			wts[i].Size = &n
		}
		if !e.running && (!e.measured || now.Sub(e.at) > worktreeSizeTTL) {
			e.running, e.done = true, make(chan struct{})
			go c.measure(path, e, e.done)
		}
		if e.running && !e.measured {
			pending = append(pending, e.done)
		}
	}
	return pending
}

func (c *sizeCache) measure(path string, e *sizeEntry, done chan struct{}) {
	c.sem <- struct{}{}
	n := c.walk(path)
	<-c.sem
	c.mu.Lock()
	e.bytes, e.at, e.measured, e.running = n, time.Now(), true, false
	c.mu.Unlock()
	close(done)
}

// dirSize sums the sizes of the regular files under root, without following
// symlinks. An unreadable entry only shrinks the figure.
func dirSize(root string) int64 {
	var total int64
	_ = filepath.WalkDir(root, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.Type().IsRegular() {
			if info, err := d.Info(); err == nil {
				total += info.Size()
			}
		}
		return nil
	})
	return total
}

// readPool reads the pool of session s with the sizes known now; with wait
// it waits for the sizes it started, up to worktreeSizeWait.
func (d *Daemon) readPool(s Session, wait bool) (WorktreePool, error) {
	pool, err := d.worktrees.ReadWorktrees(s.Registration)
	if err != nil {
		return pool, err
	}
	if pool.Worktrees == nil {
		pool.Worktrees = []PooledWorktree{}
	}
	pending := d.sizes.fill(pool.Worktrees, time.Now())
	if wait && len(pending) > 0 {
		deadline := time.NewTimer(worktreeSizeWait)
		defer deadline.Stop()
	waiting:
		for _, ch := range pending {
			select {
			case <-ch:
			case <-deadline.C:
				break waiting
			}
		}
		pending = d.sizes.fill(pool.Worktrees, time.Now())
	}
	pool.Sizing = len(pending) > 0
	return pool, nil
}

// worktreeSession resolves the session a pooled-worktree route names, or
// answers not found.
func (d *Daemon) worktreeSession(w http.ResponseWriter, replyID string) (Session, bool) {
	if d.worktrees == nil || d.index == nil || !ValidReplyID(replyID) {
		http.Error(w, "unknown session", http.StatusNotFound)
		return Session{}, false
	}
	s, ok := d.index.Session(replyID)
	if !ok {
		http.Error(w, "unknown session", http.StatusNotFound)
	}
	return s, ok
}

// handleWorktrees serves GET /s/<reply-id>/worktrees.
func (d *Daemon) handleWorktrees(w http.ResponseWriter, r *http.Request, replyID string) {
	s, ok := d.worktreeSession(w, replyID)
	if !ok {
		return
	}
	pool, err := d.readPool(s, r.URL.Query().Get("wait") == "1")
	if err != nil {
		http.Error(w, "the worktree pool could not be read: "+err.Error(), http.StatusInternalServerError)
		return
	}
	raw, _ := json.Marshal(pool)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(raw)
}

// handleWorktreeCleanup serves POST /s/<reply-id>/worktrees/cleanup, under
// the inbox write routes' rules. Its body is ignored.
func (d *Daemon) handleWorktreeCleanup(w http.ResponseWriter, r *http.Request, replyID string) {
	now := time.Now()
	s, ok := d.inboxSession(w, r, replyID, now)
	if !ok {
		return
	}
	if d.worktrees == nil {
		http.Error(w, "unknown session", http.StatusNotFound)
		return
	}
	pool, err := d.readPool(s, false)
	if err != nil {
		http.Error(w, "the worktree pool could not be read: "+err.Error(), http.StatusInternalServerError)
		return
	}
	lang, _ := requestLang(r)
	raw, _ := json.Marshal(messageRequest{Text: wsi18n.T(lang, "worktree.cleanup.ask") + "\n\n" + pool.text(worktreeTextMax)})
	status, answer := d.messageWrite(sessionInbox(d.dir, replyID), replyID, raw, localViewer(r), now)
	d.inboxAnswered(w, s, status, answer)
}

// text is the pool as the cleanup message carries it: plain text for the
// session, in the shape of worktree.list's text, at most limit bytes plus a
// last "... N more worktrees" line (singular for one) for the worktrees it
// leaves out.
func (p WorktreePool) text(limit int) string {
	var out strings.Builder
	fmt.Fprintf(&out, "pool: %s\n", p.Pool)
	if len(p.Worktrees) == 0 {
		out.WriteString("worktrees: none\n")
	}
	stamp := func(ms int64) string { return time.UnixMilli(ms).UTC().Format(time.RFC3339) }
	for i, wt := range p.Worktrees {
		var b strings.Builder
		fmt.Fprintf(&b, "\nworktree: %s\n", wt.Path)
		if wt.Profile != "" {
			fmt.Fprintf(&b, "  profile: %s\n", wt.Profile)
		}
		if wt.Detached {
			b.WriteString("  branch: (detached)\n")
		} else {
			fmt.Fprintf(&b, "  branch: %s\n", wt.Branch)
		}
		if wt.Leased {
			b.WriteString("  state: leased")
			if wt.Holder != "" {
				fmt.Fprintf(&b, "; holder id:%s", wt.Holder)
			}
			if wt.Parent != "" {
				fmt.Fprintf(&b, "; parent id:%s", wt.Parent)
			}
			if wt.AcquiredAt != 0 {
				fmt.Fprintf(&b, "; acquired_at %s", stamp(wt.AcquiredAt))
			}
			b.WriteString("\n")
		} else {
			b.WriteString("  state: idle\n")
		}
		if wt.Quarantined {
			b.WriteString("  quarantined: a profile hook failed; never reused\n")
		}
		if wt.ActiveAt != 0 {
			fmt.Fprintf(&b, "  last_activity: %s\n", stamp(wt.ActiveAt))
		}
		if wt.Size != nil {
			fmt.Fprintf(&b, "  size: %s\n", byteSize(*wt.Size))
		} else {
			b.WriteString("  size: unknown\n")
		}
		if out.Len()+b.Len() > limit {
			if n := len(p.Worktrees) - i; n == 1 {
				out.WriteString("\n... 1 more worktree\n")
			} else {
				fmt.Fprintf(&out, "\n... %d more worktrees\n", n)
			}
			break
		}
		out.WriteString(b.String())
	}
	return out.String()
}

// byteSize is n bytes in decimal units, one decimal past the first unit.
func byteSize(n int64) string {
	const unit = 1000
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	v, i := float64(n)/unit, 0
	for v >= unit && i < 3 {
		v /= unit
		i++
	}
	return fmt.Sprintf("%.1f %cB", v, "kMGT"[i])
}
