package mcp

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/kang-sw/devenv/internal/wsagent"
	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wsmailbox"
	"github.com/kang-sw/devenv/internal/wsstate"
	"github.com/kang-sw/devenv/internal/wswake"
	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

const whiteboardReconcileInterval = 30 * time.Second

type WhiteboardNudgeAgents interface {
	Get(string) (wsagent.State, error)
}
type nudgeTarget struct{ key, root string }

// WhiteboardNudges belongs to the daemon's single cache-root lock, not to its
// pages or MCP connections. It never ensures/touches keys or converts absent
// boards. Direct key/row reads are also the bounded under-folder-lock gate.
type WhiteboardNudges struct {
	cache, dir   string
	agents       WhiteboardNudgeAgents
	log          io.Writer
	now          func() time.Time
	alive        func(int) bool
	tickets      func(root string) wswhiteboard.TicketResolver
	interval     time.Duration
	start, close sync.Once
	stop, exited chan struct{}
	changed      chan struct{}
	mu           sync.Mutex
	pending      map[nudgeTarget]bool
	// Discovery is serialized by run; deliveryGate deliberately never uses it.
	identities map[string]nudgeIdentity
	stat       func(string) (os.FileInfo, error)
	read       recordFileReader
}

type nudgeIdentity struct {
	path string
	info os.FileInfo
	key  nudgeKeyIdentity // no legacy board, overrides, or mutable session fields
}

type nudgeKeyIdentity struct{ Root, Scope, Parent string }

// An actively replaced file can have equal size and a coarse timestamp. Do
// not promote it to cached evidence until its metadata is older than this gap.
const nudgeIdentityQuietPeriod = 2 * time.Second

func NewWhiteboardNudges(dir string, agents WhiteboardNudgeAgents, log io.Writer) *WhiteboardNudges {
	if log == nil {
		log = io.Discard
	}
	return &WhiteboardNudges{cache: filepath.Dir(dir), dir: dir, agents: agents, log: log, now: time.Now, alive: wsstate.ProcessAlive, tickets: func(root string) wswhiteboard.TicketResolver { return &whiteboardTickets{root: root} }, interval: whiteboardReconcileInterval, stop: make(chan struct{}), exited: make(chan struct{}), changed: make(chan struct{}, 1), pending: map[nudgeTarget]bool{}, identities: map[string]nudgeIdentity{}}
}

// Poke is called only after a successful board transaction. A caller attached
// to another cache root cannot cause discovery or delivery in this daemon.
func (n *WhiteboardNudges) Poke(cacheHome, root, key string) {
	cache, err := wsstate.CacheRoot(wsstate.Options{CacheHome: cacheHome})
	// The cache may not have existed at construction; canonicalization then
	// could not resolve a symlinked parent (notably macOS /var). Resolve both
	// sides now, without changing the scheduler's stored paths or authority.
	ownCache, ownErr := wsstate.CacheRoot(wsstate.Options{CacheHome: n.cache})
	if err != nil || ownErr != nil || cache != ownCache || !sessionKeyFilenamePattern.MatchString(key) || !filepath.IsAbs(root) {
		return
	}
	n.mu.Lock()
	n.pending[nudgeTarget{key, root}] = true
	n.mu.Unlock()
	select {
	case n.changed <- struct{}{}:
	default:
	}
}

func (n *WhiteboardNudges) Start() { n.start.Do(func() { go n.run() }) }
func (n *WhiteboardNudges) Close() {
	n.close.Do(func() { close(n.stop) })
	n.start.Do(func() { close(n.exited) })
	<-n.exited
}

func (n *WhiteboardNudges) dirs() sessionDirs {
	return sessionDirs{sessions: filepath.Join(n.cache, sessionsDirName), legacy: filepath.Join(n.cache, legacyKeysDirName)}
}

func (n *WhiteboardNudges) identity(t nudgeTarget) (sessionRecord, bool) {
	r, _, err := loadRecordWith(n.dirs(), t.key, partKey, nil, readRecordFile)
	return r, err == nil && r.Scope == string(roleLead) && r.Parent == "" && r.Root == t.root && filepath.IsAbs(r.Root)
}

// discoveryIdentity selects the effective file with split-over-legacy
// precedence, including when the split file is unreadable. A failed stat is
// never permission to reuse cached authority or fall back to legacy.
func (n *WhiteboardNudges) discoveryIdentity(d sessionDirs, key string) (string, os.FileInfo, error) {
	stat := n.stat
	if stat == nil {
		stat = os.Stat
	}
	path := d.partPath(key, partKey)
	info, err := stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		path = d.legacyPath(key)
		info, err = stat(path)
	}
	return path, info, err
}

func nudgeSameIdentity(a, b os.FileInfo) bool {
	return a.Mode().IsRegular() && b.Mode().IsRegular() && a.Size() == b.Size() && a.ModTime().Equal(b.ModTime()) && os.SameFile(a, b)
}

func (n *WhiteboardNudges) discoverRecord(d sessionDirs, key string, prior nudgeIdentity) (nudgeKeyIdentity, *nudgeIdentity, bool) {
	path, info, err := n.discoveryIdentity(d, key)
	if err != nil || !info.Mode().IsRegular() {
		return nudgeKeyIdentity{}, nil, false
	}
	// Wall-clock recent/future metadata is uncertain; always reread it.
	stable := time.Since(info.ModTime()) > nudgeIdentityQuietPeriod
	if stable && prior.info != nil && path == prior.path && nudgeSameIdentity(info, prior.info) {
		return prior.key, &prior, true
	}
	read := n.read
	if read == nil {
		read = readRecordFile
	}
	r, _, err := loadRecordWith(d, key, partKey, nil, read)
	if err != nil {
		return nudgeKeyIdentity{}, nil, false
	}
	// A replace/migration between stat and read cannot publish a stale stamp.
	afterPath, after, err := n.discoveryIdentity(d, key)
	if err != nil || path != afterPath || !nudgeSameIdentity(info, after) {
		return nudgeKeyIdentity{}, nil, false
	}
	identity := nudgeKeyIdentity{r.Root, r.Scope, r.Parent}
	if stable {
		return identity, &nudgeIdentity{path, after, identity}, true
	}
	return identity, nil, true
}

// Unlike sessionStore.dirs/listKeys this tolerates either root being absent
// without creating it. A registry remnant or arbitrary board folder is never
// an authority to recover/mint a missing key.
func (n *WhiteboardNudges) discover() map[nudgeTarget]time.Time {
	d := n.dirs()
	keys := map[string]bool{}
	for i, dir := range []string{d.sessions, d.legacy} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			if !os.IsNotExist(err) {
				fmt.Fprintf(n.log, "whiteboard nudges: discovery: %v\n", err)
				// Without both listings, a new split file could shadow a
				// legacy key. Fail closed for this pass, including cache state.
				n.identities = map[string]nudgeIdentity{}
				return map[nudgeTarget]time.Time{}
			}
			continue
		}
		for _, e := range entries {
			key := e.Name()
			if i == 0 && !e.IsDir() || i == 1 && e.IsDir() {
				continue
			}
			if i == 1 {
				var ok bool
				key, ok = strings.CutSuffix(key, ".json")
				if !ok {
					continue
				}
			}
			if sessionKeyFilenamePattern.MatchString(key) {
				keys[key] = true
			}
		}
	}
	// Keep only currently discoverable, validated identities. Corrupt records
	// and files whose metadata/read races never gain cached authority.
	prior := n.identities
	n.identities = map[string]nudgeIdentity{}
	out := map[nudgeTarget]time.Time{}
	associations := map[string]int{}
	for key := range keys {
		r, stamp, ok := n.discoverRecord(d, key, prior[key])
		if !ok {
			continue
		}
		// Cache valid nonlead/delegate identities too: they dominate a
		// representative key store, and may become leads on a later write.
		if stamp != nil {
			n.identities[key] = *stamp
		}
		if r.Scope != string(roleLead) || r.Parent != "" || !filepath.IsAbs(r.Root) {
			continue
		}
		associations[wsmailbox.ReplyID(key)]++
		t := nudgeTarget{key, r.Root}
		dir := whiteboardFolder(t.root, t.key).Dir
		for _, name := range []string{"meta.json", "pending.json", "committed.json"} {
			if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
				out[t] = time.Time{}
				break
			}
		}
	}
	nudgeSuppressCollisions(out, associations)
	return out
}

// ReplyID is intentionally readable, not collision-free. Conflicting key
// associations are unknown process evidence, never a reason to pick a lead.
func nudgeSuppressCollisions(out map[nudgeTarget]time.Time, associations map[string]int) {
	for t := range out {
		if associations[wsmailbox.ReplyID(t.key)] != 1 {
			delete(out, t)
		}
	}
}

func (n *WhiteboardNudges) deliveryGate(t nudgeTarget) func() bool {
	initial, ok := n.identity(t)
	if !ok {
		return func() bool { return false }
	}
	id := wsmailbox.ReplyID(t.key)
	reg, err := wsdashboard.ReadRegistration(n.dir, id)
	if err != nil {
		return func() bool { return false }
	}
	var st wsagent.State
	if reg.Hosted {
		if n.agents == nil {
			return func() bool { return false }
		}
		st, err = n.agents.Get(id) // runner/inbox access happens BEFORE the board lock
		if err != nil {
			return func() bool { return false }
		}
	}
	return func() bool {
		r, ok := n.identity(t)
		if !ok || r.Harness != initial.Harness || r.HarnessSession != initial.HarnessSession || r.HarnessCwd != initial.HarnessCwd {
			return false
		}
		current, err := wsdashboard.ReadRegistration(n.dir, id)
		if err != nil || current.SchemaVersion != 1 || current.Project != t.root || current.Agent || current.Hosted != reg.Hosted || current.Harness != reg.Harness {
			return false
		}
		if current.Hosted {
			if !st.Lead || !st.Process || st.ID != id || st.Root != t.root || st.Parent != nil || st.ClosedAt != "" || st.HandedOffAt != "" || st.Status != wsagent.StatusIdle && st.Status != wsagent.StatusRunning || current.EndedAt != "" || current.HostStopped || current.HostPID <= 0 || current.HarnessPID != 0 {
				return false
			}
			rec, err := wsagent.ReadRecord(n.dir, id)
			return err == nil && rec.WSKey == t.key && rec.Root == t.root && rec.Lead && rec.Parent == nil && rec.ClosedAt == "" && rec.HandedOffAt == "" && rec.HostPID == current.HostPID && rec.HolderID != "" && rec.Backend == current.Harness && st.Backend == rec.Backend && n.alive(current.HostPID)
		}
		// EndedAt/heartbeat describe the MCP connection, not harness death. An
		// idle live terminal remains eligible, but a missing registration (e.g.
		// dashboard-disabled login) supplies no process proof and is suppressed.
		return r.Harness != "" && r.HarnessSession != "" && r.Harness == current.Harness && current.HostPID == 0 && !current.HostStopped && current.HarnessPID > 0 && current.HarnessPID == reg.HarnessPID && n.alive(current.HarnessPID)
	}
}

func (n *WhiteboardNudges) process(t nudgeTarget, now time.Time) time.Time {
	gate := n.deliveryGate(t)
	path, err := (wsmailbox.Paths{State: wsstate.Options{CacheHome: n.cache}}).ReplyRegistry()
	if err != nil {
		fmt.Fprintf(n.log, "whiteboard nudges: mailbox path: %v\n", err)
		return now.Add(n.interval)
	}
	var tickets wswhiteboard.TicketResolver
	if n.tickets != nil {
		tickets = n.tickets(t.root)
	}
	next, err := whiteboardFolder(t.root, t.key).Notify(wsmailbox.ReplyID(t.key), now, gate, func(message string) error {
		return wsmailbox.PostSystem(path, wsmailbox.ReplyID(t.key), "", message, now)
	}, wswake.Mailbox.Wake, tickets)
	if err != nil {
		fmt.Fprintf(n.log, "whiteboard nudges: %s: %v\n", wsmailbox.ReplyID(t.key), err)
	}
	// Due-but-ineligible and failed delivery retry at recovery cadence, never
	// spin a zero timer or advance a receipt. Precise future deadlines stay exact.
	if err != nil || !next.IsZero() && !next.After(now) {
		return now.Add(n.interval)
	}
	return next
}

func (n *WhiteboardNudges) run() {
	defer close(n.exited)
	scan := n.now()
	boards := map[nudgeTarget]time.Time{}
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-n.stop:
			return
		case <-n.changed:
		case <-timer.C:
		}
		select {
		case <-n.stop:
			return
		default:
		}
		now := n.now()
		if !now.Before(scan) {
			boards = n.discover()
			scan = now.Add(n.interval)
		}
		n.mu.Lock()
		pending := n.pending
		n.pending = map[nudgeTarget]bool{}
		n.mu.Unlock()
		if len(pending) > 0 {
			valid := n.discover()
			for t := range boards {
				if _, ok := valid[t]; !ok {
					delete(boards, t)
				}
			}
			for t := range pending {
				if _, ok := valid[t]; ok {
					boards[t] = time.Time{}
				}
			}
		}
		targets := make([]nudgeTarget, 0, len(boards))
		for t, due := range boards {
			if due.IsZero() || !now.Before(due) {
				targets = append(targets, t)
			}
		}
		sort.Slice(targets, func(i, j int) bool {
			if targets[i].root == targets[j].root {
				return targets[i].key < targets[j].key
			}
			return targets[i].root < targets[j].root
		})
		for _, t := range targets {
			select {
			case <-n.stop:
				return
			default:
			}
			boards[t] = n.process(t, now)
		}
		next := scan
		for _, due := range boards {
			if !due.IsZero() && due.Before(next) {
				next = due
			}
		}
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer.Reset(max(time.Millisecond, next.Sub(n.now())))
	}
}
