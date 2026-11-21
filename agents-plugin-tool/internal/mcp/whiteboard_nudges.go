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

// Ticket status and external or interrupted writes are not fully represented by
// the session metadata stamp. Periodically revisit even unchanged boards.
const whiteboardNudgeRecoveryInterval = 5 * time.Minute

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
	known       map[string]nudgeKeyIdentity // collision evidence retained between recovery scans
	identities  map[string]nudgeIdentity
	stat        func(string) (os.FileInfo, error)
	read        recordFileReader
	stores      *WhiteboardStores
	ownStores   bool
	storesClose sync.Once
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

func NewWhiteboardNudges(dir string, agents WhiteboardNudgeAgents, log io.Writer, stores ...*WhiteboardStores) *WhiteboardNudges {
	if log == nil {
		log = io.Discard
	}
	n := &WhiteboardNudges{cache: filepath.Dir(dir), dir: dir, agents: agents, log: log, now: time.Now, alive: wsstate.ProcessAlive, tickets: func(root string) wswhiteboard.TicketResolver { return &whiteboardTickets{root: root} }, interval: whiteboardReconcileInterval, stop: make(chan struct{}), exited: make(chan struct{}), changed: make(chan struct{}, 1), pending: map[nudgeTarget]bool{}, identities: map[string]nudgeIdentity{}}
	if len(stores) > 0 && stores[0] != nil {
		n.stores = stores[0]
	} else {
		n.stores, n.ownStores = NewWhiteboardStores(log), true
	}
	return n
}

// Poke is called only after a successful board transaction. A caller attached
// to another cache root cannot cause discovery or delivery in this daemon.
func (n *WhiteboardNudges) Poke(cacheHome, root, key string) {
	if !sessionKeyFilenamePattern.MatchString(key) || !filepath.IsAbs(root) {
		return
	}
	// The daemon already has the scheduler's cache path. Other callers may
	// spell it with a symlinked parent, which can only resolve once it exists.
	if cacheHome != n.cache {
		cache, err := wsstate.CacheRoot(wsstate.Options{CacheHome: cacheHome})
		ownCache, ownErr := wsstate.CacheRoot(wsstate.Options{CacheHome: n.cache})
		if err != nil || ownErr != nil || cache != ownCache {
			return
		}
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
	if n.ownStores {
		n.storesClose.Do(func() { _ = n.stores.Close() })
	}
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
		stat = wsstate.Stat
	}
	path := d.partPath(key, partKey)
	info, err := stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		path = d.legacyPath(key)
		info, err = stat(path)
	}
	return path, info, err
}

// nudgeListing is what discovery's two listings already say about a key.
type nudgeListing struct {
	split  bool        // sessions/ listed an entry named key, of any type
	legacy os.DirEntry // keys/ entry of key's legacy file; nil when unlisted
}

// listedIdentity is discoveryIdentity from the listings when they settle it:
// with no split entry listed, a regular legacy entry is the effective file and
// its listed metadata is its stamp (free on Windows, one uncounted lstat
// elsewhere). A recovery pass over a large key store otherwise made two path
// stats per legacy-only key, a failed split stat and the legacy stat: under
// on-access antivirus most of the pass's cost. Anything else, including a
// split entry of any type and a non-regular legacy entry, takes the stat path.
// A split file created after the listing is caught by the after-read check
// when the key is reread; a key whose listed stamp still matches keeps its
// legacy identity until the next poke (discoverTarget always stats) or pass.
// That window is the pass's length, not the stat-to-use gap it was, and stays
// within the recovery bound.
func (n *WhiteboardNudges) listedIdentity(d sessionDirs, key string, l *nudgeListing) (string, os.FileInfo, error) {
	if l != nil && !l.split && l.legacy != nil && l.legacy.Type().IsRegular() {
		info, err := l.legacy.Info()
		return d.legacyPath(key), info, err
	}
	return n.discoveryIdentity(d, key)
}

func nudgeSameIdentity(a, b os.FileInfo) bool {
	return wsstate.SameFileMetadata(a, b)
}

func (n *WhiteboardNudges) discoverRecord(d sessionDirs, key string, prior nudgeIdentity, listed *nudgeListing) (nudgeKeyIdentity, *nudgeIdentity, bool) {
	path, info, err := n.listedIdentity(d, key, listed)
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
	if split := d.partPath(key, partKey); path != split {
		// The identity already found no split file: read the legacy file
		// directly rather than first failing a split read. A split file that
		// appears meanwhile fails the after-read path check below.
		base := read
		read = func(p string, newValue func() any) ([]byte, any, error) {
			if p == split {
				return nil, nil, fs.ErrNotExist
			}
			return base(p, newValue)
		}
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
// without creating it. Discovery schedules identities, not evidence that a
// board exists; process resolves that through the registry without creation.
func (n *WhiteboardNudges) discover() map[nudgeTarget]time.Time {
	d := n.dirs()
	keys := map[string]*nudgeListing{}
	splitOther := map[string]bool{}
	listing := func(key string) *nudgeListing {
		if keys[key] == nil {
			keys[key] = &nudgeListing{}
		}
		return keys[key]
	}
	for i, dir := range []string{d.sessions, d.legacy} {
		entries, err := wsstate.ReadDir(dir)
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
			if i == 0 && !e.IsDir() {
				// Not a key source, but its stat (it may be a symlink to a
				// key directory) still decides the identity.
				splitOther[key] = true
			}
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
				if i == 0 {
					listing(key).split = true
				} else {
					listing(key).legacy = e
				}
			}
		}
	}
	for key := range splitOther {
		if l := keys[key]; l != nil {
			l.split = true
		}
	}
	// Keep only currently discoverable, validated identities. Corrupt records
	// and files whose metadata/read races never gain cached authority.
	prior := n.identities
	n.identities = map[string]nudgeIdentity{}
	n.known = map[string]nudgeKeyIdentity{}
	out := map[nudgeTarget]time.Time{}
	associations := map[string]int{}
	for key, listed := range keys {
		r, stamp, ok := n.discoverRecord(d, key, prior[key], listed)
		if !ok {
			continue
		}
		n.known[key] = r
		// Cache valid nonlead/delegate identities too: they dominate a
		// representative key store, and may become leads on a later write.
		if stamp != nil {
			n.identities[key] = *stamp
		}
		if r.Scope != string(roleLead) || r.Parent != "" || !filepath.IsAbs(r.Root) {
			continue
		}
		associations[wsmailbox.ReplyID(key)]++
		out[nudgeTarget{key, r.Root}] = time.Time{}
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

// discoverTarget checks only a poked identity. Collision checks use
// the in-memory discovery evidence, never an enumeration of unrelated keys.
func (n *WhiteboardNudges) discoverTarget(t nudgeTarget) bool {
	r, stamp, ok := n.discoverRecord(n.dirs(), t.key, n.identities[t.key], nil)
	if !ok {
		delete(n.identities, t.key)
		delete(n.known, t.key)
		return false
	}
	if n.known == nil {
		n.known = map[string]nudgeKeyIdentity{}
	}
	n.known[t.key] = r
	if stamp != nil {
		n.identities[t.key] = *stamp
	} else {
		delete(n.identities, t.key)
	}
	if r.Scope != string(roleLead) || r.Parent != "" || r.Root != t.root {
		return false
	}
	if !n.uniqueTarget(t) {
		return false
	}
	return true
}

func (n *WhiteboardNudges) uniqueTarget(t nudgeTarget) bool {
	return nudgeIdentityUnique(t.key, n.known, wsmailbox.ReplyID)
}

func nudgeAssociationCounts(known map[string]nudgeKeyIdentity, replyID func(string) string) map[string]int {
	counts := map[string]int{}
	for key, identity := range known {
		if identity.Scope == string(roleLead) && identity.Parent == "" {
			counts[replyID(key)]++
		}
	}
	return counts
}

func nudgeIdentityUnique(target string, known map[string]nudgeKeyIdentity, replyID func(string) string) bool {
	targetID := replyID(target)
	for key, other := range known {
		if key != target && other.Scope == string(roleLead) && other.Parent == "" && replyID(key) == targetID {
			return false
		}
	}
	return true
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
	next, _ := n.processBoard(t, now)
	return next
}

type nudgePresence uint8

const (
	nudgeUnknown nudgePresence = iota
	nudgeAbsent
	nudgePresent
)

func (n *WhiteboardNudges) processBoard(t nudgeTarget, now time.Time) (time.Time, nudgePresence) {
	gate := n.deliveryGate(t)
	store := (wsmailbox.Paths{State: wsstate.Options{CacheHome: n.cache}}).Store()
	var tickets wswhiteboard.TicketResolver
	if n.tickets != nil {
		tickets = n.tickets(t.root)
	}
	var next time.Time
	presence := nudgeAbsent
	err := n.stores.use(t.root, t.key, nil, false, func(slot *whiteboardSlot) error {
		if slot.board == nil {
			return nil
		}
		presence = nudgePresent
		var err error
		next, err = slot.board.Notify(wsmailbox.ReplyID(t.key), now, gate, func(message string) error {
			return store.PostSystem(wsmailbox.ReplyID(t.key), wsmailbox.SystemLabel, message, now)
		}, wswake.Mailbox.Wake, tickets)
		return err
	})
	if err != nil {
		fmt.Fprintf(n.log, "whiteboard nudges: %s: %v\n", wsmailbox.ReplyID(t.key), err)
	}
	// Due-but-ineligible and failed delivery retry at recovery cadence, never
	// spin a zero timer or advance a receipt. Precise future deadlines stay exact.
	if err != nil || !next.IsZero() && !next.After(now) {
		if err != nil {
			presence = nudgeUnknown
		}
		return now.Add(n.interval), presence
	}
	return next, presence
}

func (n *WhiteboardNudges) run() {
	defer close(n.exited)
	scan := n.now()
	boards := map[nudgeTarget]time.Time{}
	presence := map[nudgeTarget]nudgePresence{}
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
		// Take only work that predates this discovery. A poke arriving while
		// keys are being enumerated remains queued for a later, fresh scan.
		n.mu.Lock()
		pending := n.pending
		n.pending = map[nudgeTarget]bool{}
		n.mu.Unlock()
		recovery := !now.Before(scan)
		if recovery {
			found := n.discover()
			for t := range presence {
				if _, ok := found[t]; !ok {
					delete(presence, t)
				}
			}
			boards = found
			for t := range boards {
				if presence[t] == nudgeAbsent && !pending[t] {
					delete(boards, t)
				}
			}
			scan = now.Add(whiteboardNudgeRecoveryInterval)
		}
		for t := range pending {
			if recovery {
				if _, ok := boards[t]; ok {
					boards[t] = time.Time{}
				}
				continue
			}
			if n.discoverTarget(t) {
				boards[t] = time.Time{}
			} else {
				delete(boards, t)
				delete(presence, t)
			}
		}
		// Hash each known identity once, not once for every scheduled board.
		associations := nudgeAssociationCounts(n.known, wsmailbox.ReplyID)
		targets := make([]nudgeTarget, 0, len(boards))
		for t, due := range boards {
			// Discovering a conflicting key suppresses the existing target
			// too, not just the newly poked one. Uniqueness is RAM-only.
			if associations[wsmailbox.ReplyID(t.key)] > 1 {
				delete(boards, t)
				delete(presence, t)
				continue
			}
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
			due, state := n.processBoard(t, now)
			presence[t] = state
			if state == nudgeAbsent {
				delete(boards, t)
				continue
			}
			if due.IsZero() {
				// Zero means no deadline, not "process again on every wake".
				due = time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC)
			}
			boards[t] = due
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
