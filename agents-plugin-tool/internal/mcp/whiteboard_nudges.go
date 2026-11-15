package mcp

import (
	"fmt"
	"io"
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
}

func NewWhiteboardNudges(dir string, agents WhiteboardNudgeAgents, log io.Writer) *WhiteboardNudges {
	if log == nil {
		log = io.Discard
	}
	return &WhiteboardNudges{cache: filepath.Dir(dir), dir: dir, agents: agents, log: log, now: time.Now, alive: wsstate.ProcessAlive, tickets: func(root string) wswhiteboard.TicketResolver { return &whiteboardTickets{root: root} }, interval: whiteboardReconcileInterval, stop: make(chan struct{}), exited: make(chan struct{}), changed: make(chan struct{}, 1), pending: map[nudgeTarget]bool{}}
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

// Unlike sessionStore.dirs/listKeys this tolerates either root being absent
// without creating it. A registry remnant or arbitrary board folder is never
// an authority to recover/mint a missing key.
func (n *WhiteboardNudges) discover() map[nudgeTarget]time.Time {
	keys := map[string]bool{}
	for i, dir := range []string{n.dirs().sessions, n.dirs().legacy} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			if !os.IsNotExist(err) {
				fmt.Fprintf(n.log, "whiteboard nudges: discovery: %v\n", err)
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
	out := map[nudgeTarget]time.Time{}
	associations := map[string]int{}
	for key := range keys {
		r, _, err := loadRecordWith(n.dirs(), key, partKey, nil, readRecordFile)
		if err != nil || r.Scope != string(roleLead) || r.Parent != "" || !filepath.IsAbs(r.Root) {
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
	// ReplyID is intentionally readable, not collision-free. Conflicting key
	// associations are unknown process evidence, never a reason to pick a lead.
	for t := range out {
		if associations[wsmailbox.ReplyID(t.key)] != 1 {
			delete(out, t)
		}
	}
	return out
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
