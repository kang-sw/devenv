package mcp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wsinbox"
	"github.com/kang-sw/devenv/internal/wslive"
	"github.com/kang-sw/devenv/internal/wsmailbox"
)

// The ws-mcp side of the dashboard session index: registering the top-level
// lead sessions this process serves in the machine session registry,
// heartbeating them, and nudging the daemon after a store write. Every step
// is best effort: a registry or nudge failure is a debug event, never a tool
// error, and nothing here blocks a tool call on the network.

// Test seams.
var (
	// dashboardHeartbeatInterval is the serve loop's heartbeat tick.
	dashboardHeartbeatInterval = wsdashboard.HeartbeatInterval
	// dashboardNudgeTimeout bounds one nudge round trip.
	dashboardNudgeTimeout = time.Second
)

const (
	// dashboardRecheck is how long a session's resolved dashboard key is
	// trusted before a keyed call resolves it again.
	dashboardRecheck = time.Minute
	// dashboardIdleCutoff stops the heartbeat of a session that made no keyed
	// call for this long, so a session abandoned inside a long-lived host
	// process reads as ended; its next keyed call makes it live again.
	dashboardIdleCutoff = 12 * time.Hour
	// dashboardNudgeQueue bounds pending nudges; a nudge that finds the queue
	// full is dropped and the polling backstop covers it.
	dashboardNudgeQueue = 64
)

// dashboardLink is this process's registry and nudge state.
type dashboardLink struct {
	mu       sync.Mutex
	sessions map[string]*dashboardSession // by session key
	pruned   bool
	// now is the clock; nil uses time.Now.
	now func() time.Time
	// dir resolves the daemon state directory; nil uses wsdashboard.Dir("").
	dir func() (string, error)

	nudgeOnce sync.Once
	nudges    chan wsdashboard.NudgeBody
}

// dashboardSession is one session key this process has seen on a keyed call
// by a top-level lead.
type dashboardSession struct {
	replyID   string
	project   string
	enabled   bool
	checkedAt time.Time
	lastCall  time.Time
	// reg is the registration last written; zero while not registered.
	reg wsdashboard.Registration
	// idle is set once the heartbeat stopped for dashboardIdleCutoff.
	idle bool
	// servedPort is the daemon port this session's most recent nudge round
	// trip was answered on (204); zero after a failed one or before any
	// returned. whiteboard.update's pointer line reads it (dashboardPageURL).
	servedPort int
}

func (d *dashboardLink) clock() time.Time {
	if d.now != nil {
		return d.now()
	}
	return time.Now()
}

func (d *dashboardLink) stateDir() (string, error) {
	if d.dir != nil {
		return d.dir()
	}
	return wsdashboard.Dir("")
}

func dashboardStamp(t time.Time) string { return t.UTC().Format(time.RFC3339) }

// dashboardSessionEnabled resolves dashboard.enabled for one session, with
// its session-scope overlay. An undeclared key (no dashboard manifest
// loaded) or a resolve error is off.
func (s *Server) dashboardSessionEnabled(sessionKey string) bool {
	on, _ := s.dashboardSessionResolve(sessionKey)
	return on
}

// dashboardSessionResolve is dashboardSessionEnabled with the resolve error
// kept apart: an undeclared key is off with no error.
func (s *Server) dashboardSessionResolve(sessionKey string) (bool, error) {
	if _, ok := s.declared.lookup(DashboardEnabledKey); !ok {
		return false, nil
	}
	got, err := s.resolveConfigGet(sessionKey, DashboardEnabledKey)
	if err != nil {
		return false, err
	}
	on, _ := got.Value.(bool)
	return on, nil
}

// dashboardTouch records a keyed call by a top-level lead session rooted at
// root. With the session's dashboard key on, the first call registers the
// session and a call after an idle stretch refreshes its heartbeat; with it
// off, the session's registration is removed. force re-resolves the key at
// once (after a config.tune).
func (s *Server) dashboardTouch(sessionKey, root string, force bool) {
	if _, ok := s.declared.lookup(DashboardEnabledKey); !ok {
		return // no dashboard manifest loaded: the dashboard is off
	}
	d := &s.dashboard
	now := d.clock()
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.sessions == nil {
		d.sessions = map[string]*dashboardSession{}
	}
	sess, known := d.sessions[sessionKey]
	if !known {
		sess = &dashboardSession{replyID: wsmailbox.ReplyID(sessionKey), project: root}
		d.sessions[sessionKey] = sess
	}
	sess.lastCall = now
	if !known || force || now.Sub(sess.checkedAt) >= dashboardRecheck {
		sess.enabled = s.dashboardSessionEnabled(sessionKey)
		sess.checkedAt = now
	}
	dir, err := d.stateDir()
	if err != nil {
		return
	}
	if !sess.enabled {
		if sess.reg.ReplyID != "" || !known {
			// Off from the first call too: a registration a key-on run of
			// this session left behind must not keep it listed.
			if err := wsdashboard.RemoveRegistration(dir, sess.replyID); err != nil {
				appendDebugEvent("dashboard.unregister_failed", map[string]any{"error": err.Error()})
			}
			sess.reg = wsdashboard.Registration{}
			sess.servedPort = 0
		}
		return
	}
	if sess.reg.ReplyID != "" && !sess.idle {
		return
	}
	if !d.pruned {
		d.pruned = true
		wsdashboard.PruneRegistrations(dir, now, wsdashboard.RegistryRetention)
	}
	reg := sess.reg
	if reg.ReplyID == "" {
		reg = wsdashboard.Registration{ReplyID: sess.replyID, Project: sess.project, RegisteredAt: dashboardStamp(now)}
	}
	reg.Harness = s.currentHarness()
	reg.HeartbeatAt = dashboardStamp(now)
	reg.EndedAt = ""
	if err := wsdashboard.WriteRegistration(dir, reg); err != nil {
		appendDebugEvent("dashboard.register_failed", map[string]any{"error": err.Error()})
		return
	}
	sess.reg = reg
	sess.idle = false
	s.queueDashboardNudge(sess)
}

// dashboardHeartbeat refreshes every registered session's heartbeat, stops
// it for a session idle past dashboardIdleCutoff, and removes the
// registration of a session whose key now resolves off.
func (s *Server) dashboardHeartbeat() {
	d := &s.dashboard
	now := d.clock()
	d.mu.Lock()
	defer d.mu.Unlock()
	dir, err := d.stateDir()
	if err != nil {
		return
	}
	for key, sess := range d.sessions {
		if sess.reg.ReplyID == "" || sess.idle {
			continue
		}
		if now.Sub(sess.lastCall) >= dashboardIdleCutoff {
			sess.idle = true
			continue
		}
		sess.enabled = s.dashboardSessionEnabled(key)
		sess.checkedAt = now
		if !sess.enabled {
			if err := wsdashboard.RemoveRegistration(dir, sess.replyID); err != nil {
				appendDebugEvent("dashboard.unregister_failed", map[string]any{"error": err.Error()})
			}
			sess.reg = wsdashboard.Registration{}
			sess.servedPort = 0
			continue
		}
		reg := sess.reg
		reg.HeartbeatAt = dashboardStamp(now)
		if err := wsdashboard.WriteRegistration(dir, reg); err != nil {
			appendDebugEvent("dashboard.heartbeat_failed", map[string]any{"error": err.Error()})
			continue
		}
		sess.reg = reg
	}
}

// dashboardEnd marks every session this process still heartbeats as ended,
// when the serve loop stops.
func (s *Server) dashboardEnd() {
	d := &s.dashboard
	now := d.clock()
	d.mu.Lock()
	defer d.mu.Unlock()
	dir, err := d.stateDir()
	if err != nil {
		return
	}
	for _, sess := range d.sessions {
		if sess.reg.ReplyID == "" || sess.idle {
			continue
		}
		reg := sess.reg
		reg.EndedAt = dashboardStamp(now)
		if err := wsdashboard.WriteRegistration(dir, reg); err == nil {
			sess.reg = reg
			sess.idle = true
		}
	}
}

// DashboardEnd marks the sessions this server serves as ended. ServeStdio
// does it when its loop stops; a process exiting another way calls it first.
func (s *Server) DashboardEnd() { s.dashboardEnd() }

// startDashboardHeartbeat runs dashboardHeartbeat every
// dashboardHeartbeatInterval until ctx ends.
func (s *Server) startDashboardHeartbeat(ctx context.Context) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer func() {
			if r := recover(); r != nil {
				recordPanic("dashboard.heartbeat", "", r, debug.Stack())
			}
		}()
		ticker := time.NewTicker(dashboardHeartbeatInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.dashboardHeartbeat()
			}
		}
	}()
	return done
}

// dashboardNudgingTool reports whether a successful call to name writes a
// store the dashboard renders: the whiteboard and the tools that dirty one
// of its panels.
func dashboardNudgingTool(name string) bool {
	return name == "whiteboard.update" || whiteboardDirtyingTool(name)
}

// dashboardNudge tells the daemon, fire and forget, that a store of the
// session behind sessionKey changed. Nothing is sent for a session that is
// not registered, which includes every session whose dashboard key is off.
func (s *Server) dashboardNudge(sessionKey string) {
	d := &s.dashboard
	d.mu.Lock()
	defer d.mu.Unlock()
	if sess, ok := d.sessions[strings.TrimSpace(sessionKey)]; ok && sess.reg.ReplyID != "" {
		s.queueDashboardNudge(sess)
	}
}

// queueDashboardNudge enqueues a nudge without blocking; the caller holds
// d.mu.
func (s *Server) queueDashboardNudge(sess *dashboardSession) {
	d := &s.dashboard
	d.nudgeOnce.Do(func() {
		d.nudges = make(chan wsdashboard.NudgeBody, dashboardNudgeQueue)
		go s.runDashboardNudges(d.nudges)
	})
	select {
	case d.nudges <- wsdashboard.NudgeBody{ReplyID: sess.replyID, Project: sess.project}:
	default:
	}
}

// runDashboardNudges sends queued nudges one at a time, folding duplicates
// that queued while the previous one was in flight. Each send reads the
// endpoint file afresh, so a replaced daemon is found.
func (s *Server) runDashboardNudges(queue <-chan wsdashboard.NudgeBody) {
	defer func() {
		if r := recover(); r != nil {
			recordPanic("dashboard.nudge", "", r, debug.Stack())
		}
	}()
	client := &http.Client{Timeout: dashboardNudgeTimeout, Transport: &http.Transport{Proxy: nil}}
	for first := range queue {
		batch := []wsdashboard.NudgeBody{first}
		seen := map[wsdashboard.NudgeBody]bool{first: true}
	drain:
		for {
			select {
			case next := <-queue:
				if !seen[next] {
					seen[next] = true
					batch = append(batch, next)
				}
			default:
				break drain
			}
		}
		dir, err := s.dashboard.stateDir()
		if err != nil {
			s.recordDashboardNudges(batch, 0)
			continue
		}
		ep, err := wsdashboard.ReadEndpoint(dir)
		if err != nil {
			s.recordDashboardNudges(batch, 0)
			continue
		}
		for _, body := range batch {
			ctx, cancel := context.WithTimeout(context.Background(), dashboardNudgeTimeout)
			err := wsdashboard.SendNudge(ctx, client, ep, body)
			cancel()
			port := ep.Port
			if err != nil {
				port = 0
				appendDebugEvent("dashboard.nudge_failed", map[string]any{"error": err.Error()})
			}
			s.recordDashboardNudges([]wsdashboard.NudgeBody{body}, port)
		}
	}
}

// recordDashboardNudges records the outcome of the nudges in batch on the
// sessions they name: port is the daemon port that answered 204, or zero
// for a failed round trip.
func (s *Server) recordDashboardNudges(batch []wsdashboard.NudgeBody, port int) {
	d := &s.dashboard
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, body := range batch {
		for _, sess := range d.sessions {
			if sess.replyID == body.ReplyID && sess.project == body.Project {
				sess.servedPort = port
			}
		}
	}
}

// dashboardPageURL is the session's dashboard page URL when the daemon
// serves it: the session is registered (its dashboard key is on) and its
// most recent nudge round trip succeeded. It reads only recorded state, so
// whiteboard.update never waits on, or fails through, the daemon. The
// accepted lag: the answer reflects the previous nudge, so the first update
// after the daemon dies may still name the URL once, and a session's first
// update before any of its nudges returned names the file path.
func (s *Server) dashboardPageURL(sessionKey string) (string, bool) {
	d := &s.dashboard
	d.mu.Lock()
	defer d.mu.Unlock()
	sess, ok := d.sessions[strings.TrimSpace(sessionKey)]
	if !ok || sess.reg.ReplyID == "" || sess.servedPort <= 0 {
		return "", false
	}
	return fmt.Sprintf("http://127.0.0.1:%d/s/%s", sess.servedPort, sess.replyID), true
}

// DashboardSessionSource reads registered sessions for the daemon's session
// index from the session key store and the session's live stores (snapshot
// and conversation ring) in its registry directory, and reads their pages
// (ReadPage, dashboard_page.go). It maps a reply-id to its
// session key by scanning the key file names, so no new file ever records a
// session key; the key stays inside this process and never reaches a page.
type DashboardSessionSource struct {
	store *sessionStore
	// stateDir resolves the daemon state directory; nil uses
	// wsdashboard.Dir("").
	stateDir func() (string, error)
	// now is the clock that judges snapshot staleness; nil uses time.Now.
	now func() time.Time

	mu      sync.Mutex
	byReply map[string][]string
	scanned time.Time
	dirMod  time.Time
}

// NewDashboardSessionSource returns a source over this machine's key store.
func NewDashboardSessionSource() *DashboardSessionSource {
	return &DashboardSessionSource{store: newSessionStore()}
}

// dashboardRescanAfter bounds how stale the reply-id map may be before a
// reply-id it does not know triggers a rescan even when the key directory's
// modification time looks unchanged.
const dashboardRescanAfter = 5 * time.Second

// ReadSession reads the session record behind reg. It reports false when no
// top-level lead record rooted at reg.Project maps to reg.ReplyID.
//
// The fingerprint covers everything the session page shows: the record, the
// live stores (snapshot, ring, and the board-answer inbox's held and drained
// entry names), the board's panels as the served page builds them (ticket
// directories, mailbox queues, and the listening marker live outside the
// record), the mailbox history entry names, and the resolved contacts. A
// contact's liveness is left out: it is derived from the clock, and a
// re-read that differs only by the time is no change.
func (src *DashboardSessionSource) ReadSession(reg wsdashboard.Registration) (wsdashboard.SessionData, bool) {
	key, raw, record, ok := src.lookupRecord(reg)
	if !ok {
		return wsdashboard.SessionData{}, false
	}
	h := sha256.New()
	h.Write(raw)
	updatedAt := src.hashLiveStores(h, reg.ReplyID)
	if record.Whiteboard != nil {
		panels, _ := json.Marshal(servedWhiteboardPanels(record, key, reg.ReplyID))
		fmt.Fprintf(h, "\x00panels\x00%s", panels)
	}
	io.WriteString(h, "\x00history")
	if dir, err := mailboxHistoryDir(key); err == nil {
		names, _ := mailboxHistoryEntryNames(dir)
		for _, name := range names {
			io.WriteString(h, "\x00"+name)
		}
	}
	io.WriteString(h, "\x00contacts")
	for _, c := range src.readContacts(reg) {
		fmt.Fprintf(h, "\x00%s\x01%s\x01%s", c.Address, c.ReplyID, c.LastExchange.UTC().Format(time.RFC3339))
	}
	return wsdashboard.SessionData{
		HasBoard:          record.Whiteboard != nil,
		Fingerprint:       hex.EncodeToString(h.Sum(nil)[:16]),
		SnapshotUpdatedAt: updatedAt,
	}, true
}

// lookupRecord finds the session record behind reg: the top-level lead
// record rooted at reg.Project whose key maps to reg.ReplyID. It returns the
// key (which never leaves this process), the raw record file, and the
// decoded record.
func (src *DashboardSessionSource) lookupRecord(reg wsdashboard.Registration) (key string, raw []byte, record sessionRecord, ok bool) {
	dir, err := src.store.keysDir()
	if err != nil {
		return "", nil, sessionRecord{}, false
	}
	for attempt := 0; attempt < 2; attempt++ {
		for _, key := range src.keysFor(dir, reg.ReplyID, attempt > 0) {
			data, err := os.ReadFile(src.store.keyPath(dir, key))
			if err != nil {
				continue
			}
			var record sessionRecord
			if json.Unmarshal(data, &record) != nil || record.Root != reg.Project || record.Scope != string(roleLead) || record.Parent != "" {
				continue
			}
			return key, data, record, true
		}
	}
	return "", nil, sessionRecord{}, false
}

// clock is the source's clock.
func (src *DashboardSessionSource) clock() time.Time {
	if src.now != nil {
		return src.now()
	}
	return time.Now()
}

// hashLiveStores folds the session's live stores into h and returns the
// snapshot's updatedAt. The snapshot is hashed without updatedAt but with
// its staleness, so a heartbeat rewrite is no change while a writer that
// stopped is one; the ring and the board-answer inbox (held and drained) are
// hashed by their entry names, since an entry file is never rewritten.
func (src *DashboardSessionSource) hashLiveStores(h io.Writer, replyID string) time.Time {
	resolve := src.stateDir
	if resolve == nil {
		resolve = func() (string, error) { return wsdashboard.Dir("") }
	}
	dir, err := resolve()
	if err != nil {
		return time.Time{}
	}
	sessionDir := wsdashboard.SessionDir(dir, replyID)
	var updatedAt time.Time
	if raw, err := os.ReadFile(wslive.SnapshotPath(sessionDir)); err == nil {
		var doc map[string]any
		var snap wslive.Snapshot
		if json.Unmarshal(raw, &doc) == nil && json.Unmarshal(raw, &snap) == nil {
			delete(doc, "updatedAt")
			canonical, _ := json.Marshal(doc)
			fmt.Fprintf(h, "\x00snapshot stale=%t\x00%s", snap.Stale(src.clock()), canonical)
			if snap.UpdatedAt > 0 {
				updatedAt = time.UnixMilli(snap.UpdatedAt)
			}
		}
	}
	if names, err := wslive.RingNames(wslive.RingDir(sessionDir)); err == nil {
		io.WriteString(h, "\x00ring")
		for _, name := range names {
			io.WriteString(h, "\x00"+name)
		}
	}
	// Entry files are never rewritten, so held and drained names tell a send
	// (a new held name, or a merge replacing one) and a drain apart.
	inbox := wsinbox.Dir(sessionDir)
	if names, err := wsinbox.Names(inbox); err == nil {
		io.WriteString(h, "\x00inbox")
		for _, name := range names {
			io.WriteString(h, "\x00"+name)
		}
	}
	if names, err := wsinbox.DrainedNames(inbox); err == nil {
		io.WriteString(h, "\x00inbox-drained")
		for _, name := range names {
			io.WriteString(h, "\x00"+name)
		}
	}
	return updatedAt
}

// keysFor returns the session keys whose reply-id is replyID. A miss, or
// rescan, rebuilds the map when the key directory changed or the map is
// older than dashboardRescanAfter.
func (src *DashboardSessionSource) keysFor(dir, replyID string, rescan bool) []string {
	src.mu.Lock()
	defer src.mu.Unlock()
	if keys, ok := src.byReply[replyID]; ok && !rescan {
		return keys
	}
	info, err := os.Stat(dir)
	if err != nil {
		return nil
	}
	if src.byReply != nil && info.ModTime().Equal(src.dirMod) && time.Since(src.scanned) < dashboardRescanAfter {
		return src.byReply[replyID]
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	byReply := map[string][]string{}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".json") {
			continue
		}
		key := strings.TrimSuffix(name, ".json")
		if sessionKeyFilenamePattern.MatchString(key) {
			id := wsmailbox.ReplyID(key)
			byReply[id] = append(byReply[id], key)
		}
	}
	src.byReply, src.scanned, src.dirMod = byReply, time.Now(), info.ModTime()
	return byReply[replyID]
}
