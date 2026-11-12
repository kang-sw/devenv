package mcp

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wsgit"
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
	// dir resolves the daemon state directory; nil uses
	// wsdashboard.Dir(cacheHome).
	dir func() (string, error)
	// cacheHome is the caller process's WS_CACHE_HOME for a hosted server;
	// empty means this process's, read live.
	cacheHome string

	// commonDirs memoizes dashboardCommonDir per session root, read before
	// mu is taken so the registration's git call never holds it.
	commonDirs sync.Map // root -> string

	nudgeOnce sync.Once
	nudges    chan wsdashboard.NudgeBody
	// nudgesDone is closed when the nudge goroutine has exited (after
	// Close closed nudges); nil while the goroutine was never started.
	nudgesDone chan struct{}
	// nudgesClosed is set by Close; a nudge queued after it is dropped.
	nudgesClosed bool
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
	// hosted marks the session the dashboard daemon hosts
	// (hostedSessionKey): the daemon registered it, so reg stays zero and
	// this server never writes its row, yet the daemon serves its page.
	hosted bool
	// daemonRow marks a session whose registration the dashboard daemon
	// wrote (a row with hosted set) while this server is not its hosted
	// harness: a command run from the hosted session's shell with the
	// session's key. The row stays the daemon's; this server writes none.
	daemonRow bool
	// idle is set once the heartbeat stopped for dashboardIdleCutoff.
	idle bool
	// servedPort is the daemon port this session's most recent nudge round
	// trip was answered on (204); zero after a failed one or before any
	// returned. whiteboard.update's pointer line reads it (dashboardPageURL).
	servedPort int
}

// served reports whether the daemon has a registration for the session:
// one this server wrote, or the daemon's own for the session it hosts.
func (sess *dashboardSession) served() bool { return sess.reg.ReplyID != "" || sess.hosted }

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
	return wsdashboard.Dir(d.cacheHome)
}

func dashboardStamp(t time.Time) string { return t.UTC().Format(time.RFC3339) }

// dashboardCommonDirTimeout bounds the registration's one git read.
const dashboardCommonDirTimeout = 2 * time.Second

// dashboardCommonDir is root's git common directory, absolute, which a
// registration carries so the dashboard's session rail groups a linked
// worktree under its repository without running git per session; empty
// when root is not in a repository or git fails. It runs once per
// session root per process (dashboardLink.commonDirs), never under the
// dashboard lock.
func dashboardCommonDir(runner wsgit.Runner, root string) string {
	if root == "" {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), dashboardCommonDirTimeout)
	defer cancel()
	out, err := runner.RunGit(ctx, root, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return ""
	}
	dir := strings.TrimSpace(string(out))
	if !filepath.IsAbs(dir) {
		return ""
	}
	return filepath.Clean(dir)
}

// dashboardSessionEnabled resolves dashboard.enabled for one session, with
// its session-scope overlay. An undeclared key (no dashboard manifest
// loaded) or a resolve error is off.
func (s *Server) dashboardSessionEnabled(sessionKey string) bool {
	on, _ := s.dashboardSessionResolve(sessionKey)
	return on
}

// dashboardDeclared reports whether this ws-mcp has a dashboard at all: a
// loaded dashboard manifest declares its keys, and NoDashboardMode is off.
func (s *Server) dashboardDeclared() bool {
	if s.noDashboardMode() {
		return false
	}
	_, ok := s.declared.lookup(DashboardEnabledKey)
	return ok
}

// dashboardSessionResolve is dashboardSessionEnabled with the resolve error
// kept apart: an undeclared key is off with no error.
func (s *Server) dashboardSessionResolve(sessionKey string) (bool, error) {
	if !s.dashboardDeclared() {
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
	if !s.dashboardDeclared() {
		return // no dashboard manifest loaded: the dashboard is off
	}
	if hosted := s.hostedSessionKey(); hosted != "" {
		// The daemon owns a hosted session's registration: it writes the row
		// at spawn, so this server neither writes, removes, nor prunes. It
		// still tells the daemon of the session's store changes, and learns
		// from the answers that the daemon serves the session's page.
		if sessionKey == hosted {
			s.dashboardTouchHosted(sessionKey, root)
		}
		return
	}
	d := &s.dashboard
	commonDir, cached := d.commonDirs.Load(root)
	if !cached {
		commonDir = dashboardCommonDir(s.gitRunner(), root)
		d.commonDirs.Store(root, commonDir)
	}
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
	if sess.reg.ReplyID == "" && !sess.daemonRow {
		// Before this server's first write or removal for the session.
		if row, err := wsdashboard.ReadRegistration(dir, sess.replyID); err == nil && row.Hosted {
			sess.daemonRow = true
		}
	}
	if sess.daemonRow {
		return
	}
	if !sess.enabled {
		if sess.reg.ReplyID != "" || !known {
			// Off from the first call too: a registration a key-on run of
			// this session left behind must not keep it listed.
			s.dashboardUnregister(dir, sess)
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
		reg.GitCommonDir = commonDir.(string)
	}
	reg.Harness = s.currentHarness()
	// Recorded so the daemon refuses to take the session over while its
	// harness still runs; it never makes the session live.
	reg.HarnessPID = s.harnessPID()
	reg.HeartbeatAt = dashboardStamp(now)
	reg.EndedAt = ""
	if err := wsdashboard.WriteRegistration(dir, reg); err != nil {
		s.debugEvent("dashboard.register_failed", map[string]any{"error": err.Error()})
		return
	}
	sess.reg = reg
	sess.idle = false
	s.queueDashboardNudge(sess)
}

// dashboardUnregister takes a session whose dashboard key resolves off out
// of the registry: its row is removed and this server stops writing it. The
// row of a session the dashboard daemon has hosted (its directory holds the
// runner's record) stays as it is instead: the daemon reads that row to
// learn the session moved to a terminal, and with the row gone it would
// write its own again and resume a conversation this harness holds. The
// caller holds d.mu.
func (s *Server) dashboardUnregister(dir string, sess *dashboardSession) {
	if _, err := os.Stat(filepath.Join(wsdashboard.SessionDir(dir, sess.replyID), wsdashboard.AgentRecordFileName)); err != nil {
		if err := wsdashboard.RemoveRegistration(dir, sess.replyID); err != nil {
			s.debugEvent("dashboard.unregister_failed", map[string]any{"error": err.Error()})
		}
	}
	sess.reg = wsdashboard.Registration{}
	sess.servedPort = 0
}

// dashboardTouchHosted records the hosted session's keyed call: its first
// one sends the nudge a registration would have sent, so the page URL is
// known by the session's first whiteboard.update. Nothing is written.
func (s *Server) dashboardTouchHosted(sessionKey, root string) {
	d := &s.dashboard
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.sessions == nil {
		d.sessions = map[string]*dashboardSession{}
	}
	if _, known := d.sessions[sessionKey]; known {
		return
	}
	sess := &dashboardSession{replyID: wsmailbox.ReplyID(sessionKey), project: root, hosted: true}
	d.sessions[sessionKey] = sess
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
			s.dashboardUnregister(dir, sess)
			continue
		}
		reg := sess.reg
		reg.HeartbeatAt = dashboardStamp(now)
		if err := wsdashboard.WriteRegistration(dir, reg); err != nil {
			s.debugEvent("dashboard.heartbeat_failed", map[string]any{"error": err.Error()})
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
				s.recordPanic("dashboard.heartbeat", "", r, debug.Stack())
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
// not registered, which includes every session whose dashboard key is off;
// the session the daemon hosts counts as registered.
func (s *Server) dashboardNudge(sessionKey string) {
	d := &s.dashboard
	d.mu.Lock()
	defer d.mu.Unlock()
	if sess, ok := d.sessions[strings.TrimSpace(sessionKey)]; ok && sess.served() {
		s.queueDashboardNudge(sess)
	}
}

// dashboardMailNudge is the nudge after a mailbox send on the direct path:
// a mailbox wait the daemon hosts learns of mail written outside the daemon
// only from a nudge, so it is sent whether or not the sender is registered,
// in the same body as a registered session's (its reply-id, its root as
// the project). Without a daemon endpoint it is dropped like any other.
func (s *Server) dashboardMailNudge(sessionKey, root string) {
	d := &s.dashboard
	d.mu.Lock()
	defer d.mu.Unlock()
	sessionKey = strings.TrimSpace(sessionKey)
	if sess, ok := d.sessions[sessionKey]; ok && sess.served() {
		s.queueDashboardNudge(sess)
		return
	}
	s.queueNudgeBody(wsdashboard.NudgeBody{ReplyID: wsmailbox.ReplyID(sessionKey), Project: root})
}

// queueDashboardNudge enqueues a nudge without blocking; the caller holds
// d.mu.
func (s *Server) queueDashboardNudge(sess *dashboardSession) {
	s.queueNudgeBody(wsdashboard.NudgeBody{ReplyID: sess.replyID, Project: sess.project})
}

// queueNudgeBody enqueues body without blocking; the caller holds d.mu.
func (s *Server) queueNudgeBody(body wsdashboard.NudgeBody) {
	d := &s.dashboard
	if d.nudgesClosed {
		return
	}
	d.nudgeOnce.Do(func() {
		d.nudges = make(chan wsdashboard.NudgeBody, dashboardNudgeQueue)
		d.nudgesDone = make(chan struct{})
		go func() {
			defer close(d.nudgesDone)
			s.runDashboardNudges(d.nudges)
		}()
	})
	select {
	case d.nudges <- body:
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
			case next, ok := <-queue:
				if !ok {
					// Close closed the queue while this batch was folding: a
					// closed channel answers at once, forever, so without
					// this the fold never ends.
					break drain
				}
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
				s.debugEvent("dashboard.nudge_failed", map[string]any{"error": err.Error()})
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
// serves it: the session is registered (its dashboard key is on, or the
// daemon hosts it) and its most recent nudge round trip succeeded. It reads only recorded state, so
// whiteboard.update never waits on, or fails through, the daemon. The
// accepted lag: the answer reflects the previous nudge, so the first update
// after the daemon dies may still name the URL once, and a session's first
// update before any of its nudges returned names the file path.
func (s *Server) dashboardPageURL(sessionKey string) (string, bool) {
	d := &s.dashboard
	d.mu.Lock()
	defer d.mu.Unlock()
	sess, ok := d.sessions[strings.TrimSpace(sessionKey)]
	if !ok || !sess.served() || sess.servedPort <= 0 {
		return "", false
	}
	return fmt.Sprintf("http://127.0.0.1:%d/s/%s", sess.servedPort, sess.replyID), true
}

// DashboardSessionSource reads registered sessions for the daemon's session
// index from the session key store and the session's live stores (snapshot
// and conversation ring) in its registry directory, and reads their pages
// (ReadPage, dashboard_page.go). It maps a reply-id to its
// session key by scanning the key store's names (both layouts), so no new
// file ever records a session key; the key stays inside this process and
// never reaches a page. Its reads never migrate a legacy record.
type DashboardSessionSource struct {
	store *sessionStore
	// stateDir resolves the daemon state directory; nil uses
	// wsdashboard.Dir("").
	stateDir func() (string, error)
	// now is the clock that judges snapshot staleness; nil uses time.Now.
	now func() time.Time

	// mu guards byReply, scanned, dirMods, and scan, and is never held
	// across a key directory scan: scan is the rescan in flight, which
	// concurrent misses share (keysFor).
	mu      sync.Mutex
	byReply map[string][]string
	scanned time.Time
	dirMods [2]time.Time
	scan    *keyScan
	// listKeys replaces the key store's listing, so a test can hold a
	// rescan; nil lists the store.
	listKeys func(d sessionDirs) ([]string, error)

	// boards memoizes each session's board render (dashboard_page.go).
	boards boardMemo
	// records, replies, scopes, and snapshots serve the unchanged files a
	// re-read and a page read read again and again, each costing a stat
	// (wsdashboard.FileCache): the session record's part files (with their
	// bytes, which the fingerprint hashes), the machine reply registry, the
	// named scopes' mailbox stores, and the live snapshots HashLiveStores
	// folds. Parsed values are shared: the source only reads them.
	records   wsdashboard.FileCache[recordFile]
	replies   wsdashboard.FileCache[wsmailbox.ReplyStore]
	scopes    wsdashboard.FileCache[wsmailbox.StoreFile]
	snapshots wsdashboard.SnapshotCache
	// history caches the mailbox history entries the pages read.
	history mailboxHistoryCache

	// prints is each session's last ReadSession fingerprint, which tells a
	// changed re-read from an unchanged one for the counters; guarded by mu.
	prints map[string]string
	// readsChanged and readsUnchanged count ReadSession calls whose
	// fingerprint changed (the index then wakes the session's pages, whose
	// re-read renders the board) and ones that found no change.
	readsChanged, readsUnchanged atomic.Int64
}

var (
	_ wsdashboard.CounterSource    = (*DashboardSessionSource)(nil)
	_ wsdashboard.SessionForgetter = (*DashboardSessionSource)(nil)
)

// recordFile is one session record file as the record cache holds it: its
// bytes and its decoded value (recordFileReader).
type recordFile struct {
	data  []byte
	value any
}

// Counters returns the source's counters (wsdashboard.CounterSource): the
// re-reads by outcome, and each file cache's hits and misses.
func (src *DashboardSessionSource) Counters() map[string]int64 {
	out := map[string]int64{
		"session_reads_changed":   src.readsChanged.Load(),
		"session_reads_unchanged": src.readsUnchanged.Load(),
	}
	out["record_cache_hits"], out["record_cache_misses"] = src.records.Counts()
	out["reply_registry_cache_hits"], out["reply_registry_cache_misses"] = src.replies.Counts()
	out["scope_store_cache_hits"], out["scope_store_cache_misses"] = src.scopes.Counts()
	out["source_snapshot_cache_hits"], out["source_snapshot_cache_misses"] = src.snapshots.Counts()
	return out
}

// ForgetSession drops what the source keeps of a session the daemon's index
// dropped: its last fingerprint, its mailbox history entries, its board
// renders, its cached record files, and its cached snapshot
// (wsdashboard.SessionForgetter).
func (src *DashboardSessionSource) ForgetSession(replyID string) {
	src.mu.Lock()
	delete(src.prints, replyID)
	keys := src.byReply[replyID]
	src.mu.Unlock()
	src.history.forget(replyID)
	src.boards.forget(replyID)
	if d, err := src.store.dirs(); err == nil {
		for _, key := range keys {
			src.records.ForgetUnder(d.keyDir(key))
			src.records.ForgetUnder(d.legacyPath(key))
		}
	}
	if dir, err := src.resolveStateDir(); err == nil {
		src.snapshots.Forget(dir, replyID)
	}
}

// SetLiveSnapshot implements wsdashboard.LiveSnapshotHolder: the change
// detector's snapshot reads (HashLiveStores) follow a hosted session's
// in-memory copy.
func (src *DashboardSessionSource) SetLiveSnapshot(stateDir, replyID string, snap wslive.Snapshot) {
	src.snapshots.SetLive(stateDir, replyID, snap)
}

// ClearLiveSnapshot implements wsdashboard.LiveSnapshotHolder.
func (src *DashboardSessionSource) ClearLiveSnapshot(stateDir, replyID string) {
	src.snapshots.ClearLive(stateDir, replyID)
}

// NewDashboardSessionSource returns a source over this machine's key store.
func NewDashboardSessionSource() *DashboardSessionSource {
	return &DashboardSessionSource{store: newSessionStore()}
}

// dashboardRescanAfter bounds how stale the reply-id map may be before a
// reply-id it does not know triggers a rescan even when the key directories'
// modification times look unchanged.
const dashboardRescanAfter = 5 * time.Second

// ReadSession reads the session record behind reg. It reports false when no
// top-level lead record rooted at reg.Project maps to reg.ReplyID.
//
// The fingerprint covers everything the session page shows: the record
// (the board, todos, and agenda), the live stores (snapshot, ring, and the
// board-answer inbox's held and drained entry names), the inputs of the
// board's panels that live outside the record (boardPrint), the mailbox
// history entry names, and the resolved contacts (cwd included: it labels
// the peer row). It builds no panel: a nudge arrives up to several times a
// second, and the panels read every listed ticket in full, so only the page
// read a changed fingerprint wakes renders them. A contact's liveness is
// left out: it is derived from the clock, and a re-read that differs only
// by the time is no change.
func (src *DashboardSessionSource) ReadSession(reg wsdashboard.Registration) (wsdashboard.SessionData, bool) {
	key, raw, record, ok := src.lookupRecord(reg)
	if !ok {
		return wsdashboard.SessionData{}, false
	}
	h := sha256.New()
	h.Write(raw)
	updatedAt := src.hashLiveStores(h, reg.ReplyID)
	replies, repliesOK := src.loadReplies()
	if record.Whiteboard != nil {
		boardPrint(h, key, record, reg.ReplyID, replies, repliesOK)
	}
	io.WriteString(h, "\x00history")
	if dir, err := mailboxHistoryDir("", key); err == nil {
		names, _ := mailboxHistoryEntryNames(dir)
		for _, name := range names {
			io.WriteString(h, "\x00"+name)
		}
	}
	io.WriteString(h, "\x00contacts")
	if repliesOK {
		for _, c := range src.contactsFrom(reg, replies) {
			fmt.Fprintf(h, "\x00%s\x01%s\x01%s\x01%s", c.Address, c.ReplyID, c.LastExchange.UTC().Format(time.RFC3339), c.Cwd)
		}
	}
	fp := hex.EncodeToString(h.Sum(nil)[:16])
	src.mu.Lock()
	prev, had := src.prints[reg.ReplyID]
	if src.prints == nil {
		src.prints = map[string]string{}
	}
	src.prints[reg.ReplyID] = fp
	src.mu.Unlock()
	if had && prev == fp {
		src.readsUnchanged.Add(1)
	} else {
		src.readsChanged.Add(1)
	}
	return wsdashboard.SessionData{
		HasBoard:          record.Whiteboard != nil,
		CountPolicy:       record.Whiteboard.OpenPolicyCount(),
		Fingerprint:       fp,
		SnapshotUpdatedAt: updatedAt,
	}, true
}

// boardPrint folds into h the inputs of the served board's panels that
// live outside the session record, from metadata alone: the board memo's
// non-file inputs (boardMemoBase: the mailbox listening marker's presence,
// process, and liveness, and the session's reply-registry print, its
// contacts and reply-id unread count) and the ticket directories' listing
// (ticketBoardPrint). The mailbox history and the resolved contacts are
// folded by ReadSession itself; the served panels read no named mailbox
// store (servedWhiteboardPanels).
func boardPrint(h io.Writer, key string, record sessionRecord, replyID string, replies wsmailbox.ReplyStore, repliesOK bool) {
	fmt.Fprintf(h, "\x00board\x00%s", boardMemoBase(key, record, "", nil, replyPrint(replies, repliesOK, replyID)))
	ticketBoardPrint(h, record.Root)
}

// ticketBoardStatuses are the ticket directories the ticket panel lists.
var ticketBoardStatuses = []string{"ready", "todo"}

// ticketBoardPrint folds the ticket panel's directories into h: each
// ticket file's name, size, and modification time, read from the directory
// listing alone, so an add, remove, rename, or edit changes it without a
// ticket being read. An edit that keeps both size and modification time
// within the filesystem's timestamp resolution is missed until the next
// change.
func ticketBoardPrint(h io.Writer, root string) {
	for _, status := range ticketBoardStatuses {
		entries, err := os.ReadDir(filepath.Join(root, "ai-docs", "tickets", status))
		if err != nil {
			fmt.Fprintf(h, "\x00tickets\x00%s\x00-", status)
			continue
		}
		fmt.Fprintf(h, "\x00tickets\x00%s", status)
		for _, e := range entries {
			if e.IsDir() || filepath.Ext(e.Name()) != ".md" {
				continue
			}
			if info, err := e.Info(); err == nil {
				fmt.Fprintf(h, "\x00%s\x01%d\x01%d", e.Name(), info.Size(), info.ModTime().UnixNano())
			} else {
				fmt.Fprintf(h, "\x00%s\x01-", e.Name())
			}
		}
	}
}

// lookupRecord finds the session record behind reg: the top-level lead
// record rooted at reg.Project whose key maps to reg.ReplyID. It returns the
// key (which never leaves this process), the raw bytes of the record's files,
// and the decoded record with every part.
func (src *DashboardSessionSource) lookupRecord(reg wsdashboard.Registration) (key string, raw []byte, record sessionRecord, ok bool) {
	d, err := src.store.dirs()
	if err != nil {
		return "", nil, sessionRecord{}, false
	}
	for attempt := 0; attempt < 2; attempt++ {
		for _, key := range src.keysFor(d, reg.ReplyID, attempt > 0) {
			var buf bytes.Buffer
			record, _, err := loadRecordWith(d, key, partAll, &buf, src.readRecordFile)
			if err != nil || record.Root != reg.Project || record.Scope != string(roleLead) || record.Parent != "" {
				continue
			}
			return key, buf.Bytes(), record, true
		}
	}
	return "", nil, sessionRecord{}, false
}

// readRecordFile is the source's recordFileReader: readRecordFile through
// the record cache, so an unchanged part file costs a stat.
func (src *DashboardSessionSource) readRecordFile(path string, newValue func() any) ([]byte, any, error) {
	f, err := src.records.Load(path, func(path string) (recordFile, error) {
		data, value, err := readRecordFile(path, newValue)
		return recordFile{data: data, value: value}, err
	})
	return f.data, f.value, err
}

// clock is the source's clock.
func (src *DashboardSessionSource) clock() time.Time {
	if src.now != nil {
		return src.now()
	}
	return time.Now()
}

// hashLiveStores folds the session's live stores into h and returns the
// snapshot's updatedAt (wsdashboard.SnapshotCache.HashLiveStores, under this
// source's state dir, clock, and snapshot cache).
func (src *DashboardSessionSource) hashLiveStores(h io.Writer, replyID string) time.Time {
	dir, err := src.resolveStateDir()
	if err != nil {
		return time.Time{}
	}
	return src.snapshots.HashLiveStores(h, dir, replyID, src.clock())
}

// resolveStateDir resolves the daemon state directory.
func (src *DashboardSessionSource) resolveStateDir() (string, error) {
	if src.stateDir != nil {
		return src.stateDir()
	}
	return wsdashboard.Dir("")
}

// keyScan is one rescan of the key store; done is closed when it landed.
type keyScan struct {
	done chan struct{}
}

// keysFor returns the session keys whose reply-id is replyID. A miss, or
// rescan, rebuilds the map when a key directory changed or the map is older
// than dashboardRescanAfter. The rebuild lists the key directories without
// holding mu, so a lookup of a known reply-id never waits on it; a miss
// while a rebuild runs waits for that one instead of starting another.
func (src *DashboardSessionSource) keysFor(d sessionDirs, replyID string, rescan bool) []string {
	src.mu.Lock()
	keys, known := src.byReply[replyID]
	src.mu.Unlock()
	if known && !rescan {
		return keys
	}
	mods, ok := keyStoreMods(d)
	if !ok {
		return nil
	}
	src.mu.Lock()
	if src.byReply != nil && mods == src.dirMods && time.Since(src.scanned) < dashboardRescanAfter {
		keys := src.byReply[replyID]
		src.mu.Unlock()
		return keys
	}
	if scan := src.scan; scan != nil {
		src.mu.Unlock()
		<-scan.done
		src.mu.Lock()
		defer src.mu.Unlock()
		return src.byReply[replyID]
	}
	scan := &keyScan{done: make(chan struct{})}
	src.scan = scan
	src.mu.Unlock()
	defer close(scan.done)

	list := src.listKeys
	if list == nil {
		list = src.store.listKeys
	}
	listed, err := list(d)
	src.mu.Lock()
	defer src.mu.Unlock()
	src.scan = nil
	if err != nil {
		return nil
	}
	byReply := map[string][]string{}
	for _, key := range listed {
		id := wsmailbox.ReplyID(key)
		byReply[id] = append(byReply[id], key)
	}
	src.byReply, src.scanned, src.dirMods = byReply, time.Now(), mods
	return byReply[replyID]
}

// keyStoreMods returns the modification times of the key store's two roots,
// zero for a missing legacy root: a key added to or removed from either
// changes one. ok is false when the split root cannot be read.
func keyStoreMods(d sessionDirs) ([2]time.Time, bool) {
	info, err := os.Stat(d.sessions)
	if err != nil {
		return [2]time.Time{}, false
	}
	mods := [2]time.Time{info.ModTime()}
	if info, err := os.Stat(d.legacy); err == nil {
		mods[1] = info.ModTime()
	}
	return mods, true
}
