package mcp

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsagent"
	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wsmailbox"
	"github.com/kang-sw/devenv/internal/wswake"
	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

var nudgeNow = time.Date(2026, 11, 15, 1, 0, 0, 0, time.UTC)

type nudgeFixture struct {
	n      *WhiteboardNudges
	target nudgeTarget
	row    wsdashboard.Registration
	record sessionKeyRecord
}

func nudgeWriteJSON(t *testing.T, path string, v any) {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}

func newNudgeFixture(t *testing.T) *nudgeFixture {
	t.Helper()
	setupMailboxTestEnv(t)
	dir, err := wsdashboard.Dir("")
	if err != nil {
		t.Fatal(err)
	}
	f := &nudgeFixture{n: NewWhiteboardNudges(dir, nil, io.Discard), target: nudgeTarget{"nudge-terminal-lead", t.TempDir()}}
	f.record = sessionKeyRecord{SchemaVersion: 2, Scope: string(roleLead), Root: f.target.root, Harness: "pi", HarnessSession: "native-session", HarnessCwd: f.target.root}
	f.row = wsdashboard.Registration{SchemaVersion: 1, ReplyID: wsmailbox.ReplyID(f.target.key), Project: f.target.root, Harness: "pi", HarnessPID: os.Getpid(), HeartbeatAt: "2000-01-01T00:00:00Z", EndedAt: "2000-01-01T00:00:00Z"}
	f.save(t)
	f.board(t, `<wb:topic name="Work"><wb:log>summary</wb:log><wb:state value="implementing" owner="executor" eta_minutes="1"/></wb:topic>`, nudgeNow)
	return f
}

func (f *nudgeFixture) save(t *testing.T) {
	t.Helper()
	nudgeWriteJSON(t, f.n.dirs().partPath(f.target.key, partKey), f.record)
	if err := wsdashboard.WriteRegistration(f.n.dir, f.row); err != nil {
		t.Fatal(err)
	}
}
func (f *nudgeFixture) board(t *testing.T, commands string, now time.Time) {
	t.Helper()
	if err := whiteboardFolder(f.target.root, f.target.key).Update(nil, func(s *wswhiteboard.State) error {
		_, p := wswhiteboard.Apply(s, commands, wswhiteboard.Env{Now: now})
		if len(p) > 0 {
			return errors.New(strings.Join(p, "; "))
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
func (f *nudgeFixture) queue(t *testing.T) []wsmailbox.Envelope {
	t.Helper()
	path, err := (wsmailbox.Paths{}).ReplyRegistry()
	if err != nil {
		t.Fatal(err)
	}
	store, err := wsmailbox.LoadReplyStore(path)
	if err != nil {
		t.Fatal(err)
	}
	return store.Queues[f.row.ReplyID]
}
func (f *nudgeFixture) hosted(t *testing.T) (wsagent.Record, *fakePiAgents) {
	t.Helper()
	a := newFakePiAgents()
	f.n.agents = a
	f.row.Hosted, f.row.HostPID, f.row.HarnessPID, f.row.EndedAt = true, os.Getpid(), 0, ""
	a.states[f.row.ReplyID] = wsagent.State{ID: f.row.ReplyID, Root: f.target.root, Lead: true, Process: true, Status: wsagent.StatusIdle, Backend: "pi"}
	rec := wsagent.Record{SchemaVersion: 1, ReplyID: f.row.ReplyID, WSKey: f.target.key, Root: f.target.root, Lead: true, HostPID: os.Getpid(), HolderID: "fixture-holder", Backend: "pi", Profile: "lead-pi"}
	f.save(t)
	if err := wsagent.WriteRecordForTest(f.n.dir, rec); err != nil {
		t.Fatal(err)
	}
	return rec, a
}

func TestWhiteboardNudgeDiscoveryBothLayoutsWithoutTouchOrResurrection(t *testing.T) {
	f := newNudgeFixture(t)
	path := f.n.dirs().partPath(f.target.key, partKey)
	old := time.Now().Add(-31 * 24 * time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	before, _ := os.Stat(path)
	legacy := nudgeTarget{"legacy-nudge-lead", f.target.root}
	nudgeWriteJSON(t, f.n.dirs().legacyPath(legacy.key), sessionKeyRecord{Root: f.target.root, Scope: string(roleLead)})
	if err := whiteboardFolder(legacy.root, legacy.key).Update(nil, func(*wswhiteboard.State) error { return nil }); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"child-nudge", "delegate-nudge", "missing-board-nudge"} {
		r := f.record
		if key == "child-nudge" {
			r.Parent = f.target.key
		}
		if key == "delegate-nudge" {
			r.Scope = string(roleDelegate)
		}
		nudgeWriteJSON(t, f.n.dirs().partPath(key, partKey), r)
	}
	want := map[nudgeTarget]time.Time{f.target: {}, legacy: {}}
	if got := f.n.discover(); !reflect.DeepEqual(got, want) {
		t.Fatalf("discovered: %+v", got)
	}
	after, _ := os.Stat(path)
	if !after.ModTime().Equal(before.ModTime()) {
		t.Fatal("discovery refreshed expired key")
	}
	// An invalid split identity cannot fall back to a stale valid legacy record.
	nudgeWriteJSON(t, f.n.dirs().partPath(legacy.key, partKey), sessionKeyRecord{SchemaVersion: 999, Root: f.target.root, Scope: string(roleLead)})
	if len(f.n.discover()) != 1 {
		t.Fatal("unsupported split key fell back to legacy")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	f.n.process(f.target, nudgeNow.Add(8*time.Hour))
	if len(f.queue(t)) != 0 {
		t.Fatal("missing key enqueued")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("expired key resurrected")
	}
	if _, err := os.Stat(whiteboardFolder(f.target.root, "missing-board-nudge").Dir); !os.IsNotExist(err) {
		t.Fatal("discovery created absent board")
	}
	empty := NewWhiteboardNudges(filepath.Join(t.TempDir(), "missing", "dashboard"), nil, io.Discard)
	if len(empty.discover()) != 0 {
		t.Fatal("empty discovery")
	}
	if _, err := os.Stat(empty.cache); !os.IsNotExist(err) {
		t.Fatal("discovery created cache/key roots")
	}
}

func TestWhiteboardNudgeTerminalPositiveProcessGate(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*nudgeFixture)
		want   bool
	}{
		{"idle-live-stale-and-ended-mcp", func(*nudgeFixture) {}, true},
		{"missing-pid", func(f *nudgeFixture) { f.row.HarnessPID = 0 }, false},
		{"dead-pid", func(f *nudgeFixture) { f.n.alive = func(int) bool { return false } }, false},
		{"contradictory-host-pid", func(f *nudgeFixture) { f.row.HostPID = os.Getpid() }, false},
		{"contradictory-stop", func(f *nudgeFixture) { f.row.HostStopped = true }, false},
		{"wrong-root", func(f *nudgeFixture) { f.row.Project = t.TempDir() }, false},
		{"wrong-harness", func(f *nudgeFixture) { f.row.Harness = "claude" }, false},
		{"unknown-row-schema", func(f *nudgeFixture) { f.row.SchemaVersion = 99 }, false},
		{"agent-row", func(f *nudgeFixture) { f.row.Agent = true }, false},
		{"unknown-session", func(f *nudgeFixture) { f.record.HarnessSession = "" }, false},
		{"unknown-harness", func(f *nudgeFixture) { f.record.Harness = "" }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newNudgeFixture(t)
			tc.change(f)
			f.save(t)
			if got := f.n.deliveryGate(f.target)(); got != tc.want {
				t.Fatalf("gate %v want %v", got, tc.want)
			}
		})
	}
	f := newNudgeFixture(t)
	if err := os.Remove(wsdashboard.RegistrationPath(f.n.dir, f.row.ReplyID)); err != nil {
		t.Fatal(err)
	}
	if f.n.deliveryGate(f.target)() {
		t.Fatal("board/key/watcher alone proved life")
	}
	// OS PID checks deliberately do not authenticate creation time/PID reuse.
	f.save(t)
	f.n.alive = func(pid int) bool { return pid == f.row.HarnessPID }
	if !f.n.deliveryGate(f.target)() {
		t.Fatal("documented positive-PID boundary changed")
	}
}

func TestWhiteboardNudgeHostedOpenOwningRunnerAndRow(t *testing.T) {
	for _, name := range []string{"live-idle", "stopped", "closed", "handed-off", "missing-snapshot", "not-owning-runner", "no-process", "wrong-key", "wrong-pid", "unknown-process", "wrong-backend"} {
		t.Run(name, func(t *testing.T) {
			f := newNudgeFixture(t)
			rec, a := f.hosted(t)
			switch name {
			case "stopped":
				f.row.HostStopped = true
				f.row.HostPID = 0
			case "closed":
				rec.ClosedAt = "closed"
				a.set(f.row.ReplyID, func(s *wsagent.State) { s.Status = wsagent.StatusClosed; s.ClosedAt = "closed" })
			case "handed-off":
				rec.HandedOffAt = "handed"
				a.set(f.row.ReplyID, func(s *wsagent.State) { s.Status = wsagent.StatusHandedOff; s.HandedOffAt = "handed" })
			case "missing-snapshot":
				delete(a.states, f.row.ReplyID)
			case "not-owning-runner":
				a.set(f.row.ReplyID, func(s *wsagent.State) { s.Root = t.TempDir() })
			case "no-process":
				a.set(f.row.ReplyID, func(s *wsagent.State) { s.Process = false })
			case "wrong-key":
				rec.WSKey = "another-key"
			case "wrong-pid":
				rec.HostPID++
			case "unknown-process":
				f.n.alive = func(int) bool { return false }
			case "wrong-backend":
				rec.Backend = "claude"
			}
			f.save(t)
			if err := wsagent.WriteRecordForTest(f.n.dir, rec); err != nil {
				t.Fatal(err)
			}
			gate := f.n.deliveryGate(f.target)
			if got := gate(); got != (name == "live-idle") {
				t.Fatalf("gate %v", got)
			}
			f.n.process(f.target, nudgeNow.Add(8*time.Hour))
			if got := len(f.queue(t)); (got == 1) != (name == "live-idle") {
				t.Fatalf("queued %d", got)
			}
		})
	}
}

func TestWhiteboardNudgeRevalidatesImmediatelyBeforeEnqueue(t *testing.T) {
	f := newNudgeFixture(t)
	_, _ = f.hosted(t)
	gate := f.n.deliveryGate(f.target)
	f.row.HostStopped = true
	f.row.HostPID = 0
	f.save(t) // automatic idle-stop after preflight
	if gate() {
		t.Fatal("stale preflight woke stopped host")
	}
	// No reverse session-store lock: scheduler delivery can run while a normal
	// session mutation owns sessionStore.mu and waits for the board transaction.
	store := newSessionStore()
	store.mu.Lock()
	defer store.mu.Unlock()
	f.row.HostStopped = false
	f.row.HostPID = os.Getpid()
	f.save(t)
	f.n.process(f.target, nudgeNow.Add(8*time.Hour))
	if len(f.queue(t)) != 1 {
		t.Fatal("resumed host did not catch up")
	}
	// Stop after enqueue does not retract already queued ordinary mail.
	f.row.HostStopped = true
	f.row.HostPID = 0
	f.save(t)
	if len(f.queue(t)) != 1 {
		t.Fatal("delivery-time lifecycle semantics changed")
	}
}

func TestWhiteboardNudgeRestartResumeIsolationAndCopy(t *testing.T) {
	f := newNudgeFixture(t)
	f.n.alive = func(int) bool { return false }
	f.n.process(f.target, nudgeNow.Add(8*time.Hour))
	if len(f.queue(t)) != 0 {
		t.Fatal("dead lead enqueued")
	}
	f.n.alive = func(int) bool { return true }
	next := f.n.process(f.target, nudgeNow.Add(8*time.Hour))
	if len(f.queue(t)) != 1 || !next.IsZero() {
		t.Fatal("catch-up accounting", next)
	}
	restarted := NewWhiteboardNudges(f.n.dir, nil, io.Discard)
	restarted.process(f.target, nudgeNow.Add(16*time.Hour))
	if len(f.queue(t)) != 1 {
		t.Fatal("restart replayed missed intervals on an unchanged board")
	}
	// A second board's clock and receipts do not share this one's suppression.
	dest := nudgeTarget{"nudge-destination-lead", f.target.root}
	nudgeWriteJSON(t, restarted.dirs().partPath(dest.key, partKey), f.record)
	row := f.row
	row.ReplyID = wsmailbox.ReplyID(dest.key)
	if err := wsdashboard.WriteRegistration(restarted.dir, row); err != nil {
		t.Fatal(err)
	}
	// Source production ceases before copying. The destination is not alive
	// until the copy is complete; no transfer command or lease is involved.
	f.n.alive = func(int) bool { return false }
	src := whiteboardFolder(f.target.root, f.target.key).Dir
	dst := whiteboardFolder(dest.root, dest.key).Dir
	if err := os.CopyFS(dst, os.DirFS(src)); err != nil {
		t.Fatal(err)
	}
	restarted.process(dest, nudgeNow.Add(8*time.Hour))
	path, _ := (wsmailbox.Paths{}).ReplyRegistry()
	q, err := wsmailbox.LoadReplyStore(path)
	if err != nil || len(q.Queues[row.ReplyID]) != 1 || len(q.Queues[f.row.ReplyID]) != 1 {
		t.Fatal("copy isolation", err, q.Queues)
	}
	restarted.process(dest, nudgeNow.Add(8*time.Hour))
	q, _ = wsmailbox.LoadReplyStore(path)
	if len(q.Queues[row.ReplyID]) != 1 {
		t.Fatal("destination catch-up repeated")
	}
	a, _ := whiteboardFolder(dest.root, dest.key).Open(nil)
	b, _ := whiteboardFolder(f.target.root, f.target.key).Open(nil)
	if !reflect.DeepEqual(a.TopicStates()["Work"].State.Run, b.TopicStates()["Work"].State.Run) {
		t.Fatal("copy changed original execution deadline")
	}
}

func TestWhiteboardNudgeActualMailboxWatchDrainAndManualRecv(t *testing.T) {
	for _, backend := range []string{"pi", "claude"} {
		t.Run(backend, func(t *testing.T) {
			f := newNudgeFixture(t)
			f.record.Harness = backend
			f.row.Harness = backend
			f.save(t)
			mail := NewLeadMailbox(f.n.dir)
			defer mail.server.Close()
			received := make(chan []string, 1)
			stop := mail.Watch(f.target.root, f.target.key, func() {
				texts, err := mail.Drain(f.target.root, f.target.key)
				if err != nil {
					t.Error(err)
				}
				if len(texts) > 0 {
					received <- texts
				}
			})
			defer stop()
			f.n.process(f.target, nudgeNow.Add(2*time.Hour))
			select {
			case texts := <-received:
				if len(texts) != 1 || !strings.Contains(texts[0], "mail from system") || !strings.Contains(texts[0], "Whiteboard lead nudge (overdue, hourly)") || !strings.Contains(texts[0], "Work; owner executor") {
					t.Fatalf("lead input %q", texts)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("mailbox-to-lead watch did not receive nudge")
			}
			if len(f.queue(t)) != 0 {
				t.Fatal("watch did not claim queue")
			}
			again, err := mail.Drain(f.target.root, f.target.key)
			if err != nil || len(again) != 0 {
				t.Fatal("duplicate drain", err)
			}
		})
	}
	// Codex/manual/off configurations keep the same queue and explicit recv;
	// scheduling neither starts a host process nor changes push configuration.
	f := newNudgeFixture(t)
	f.record.Harness = "codex"
	f.row.Harness = "codex"
	f.save(t)
	f.n.process(f.target, nudgeNow.Add(2*time.Hour))
	s := NewServer(f.target.root, "test")
	defer s.Close()
	text := toolText(t, callToolOnce(t, s, 1, "mailbox.recv", map[string]any{"session_key": f.target.key}))
	if !strings.Contains(text, "Whiteboard lead nudge") || !strings.Contains(text, "system: Whiteboard lead nudge") || len(f.queue(t)) != 0 {
		t.Fatal("manual recv", text)
	}
}

func TestWhiteboardNudgePreciseTimerAndPokeWithoutPages(t *testing.T) {
	f := newNudgeFixture(t)
	// Deadline is about 200ms away; the recovery scan is deliberately an hour.
	start := time.Now().Add(-time.Minute + 200*time.Millisecond)
	// A new run gives the precise timer an independently controlled start.
	f.board(t, `<wb:topic name="Work"><wb:state value="waiting"/><wb:state value="implementing" owner="executor" eta_minutes="1"/></wb:topic>`, start)
	f.n.interval = time.Hour
	sub := wswake.Mailbox.Subscribe()
	defer sub.Close()
	f.n.Start()
	defer f.n.Close()
	select {
	case <-sub.Hard():
	case <-time.After(5 * time.Second):
		t.Fatal("precise deadline waited for recovery scan")
	}
	if len(f.queue(t)) != 1 {
		t.Fatal("timer wake without message")
	}
	// Successful update poke invalidates a dormant schedule without pages/MCP
	// watchers. A foreign-cache poke is ignored.
	f.n.Poke(t.TempDir(), f.target.root, f.target.key)
	f.n.mu.Lock()
	foreign := len(f.n.pending)
	f.n.mu.Unlock()
	if foreign != 0 {
		t.Fatal("foreign cache poke accepted")
	}
	f.board(t, `<wb:topic name="Work"><wb:state value="waiting"/><wb:state value="implementing" owner="replacement" eta_minutes="1"/></wb:topic>`, time.Now().Add(-2*time.Minute))
	resolvedCache, err := filepath.EvalSymlinks(f.n.cache)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("poke cache at construction %s; current canonical path %s", f.n.cache, resolvedCache)
	f.n.Poke(f.n.cache, f.target.root, f.target.key)
	select {
	case <-sub.Hard():
	case <-time.After(5 * time.Second):
		t.Fatal("poke did not invalidate timer")
	}
	if len(f.queue(t)) != 2 {
		t.Fatal("new run poke inherited debounce")
	}
}

func TestWhiteboardNudgeSuccessfulUpdatePokesWithoutDashboard(t *testing.T) {
	f := newWhiteboardFixture(t)
	defer f.s.Close()
	var called int
	f.s.whiteboardChanged = func(cache, root, key string) {
		called++
		if cache != os.Getenv("WS_CACHE_HOME") || root != f.root || key != f.key {
			t.Error("wrong poke association")
		}
	}
	f.update(t, `<wb:topic name="work"><wb:log>active</wb:log><wb:state value="implementing" owner="executor" eta_minutes="40"/><wb:eta minutes="60" reason="scope"/></wb:topic>`)
	if called != 1 {
		t.Fatal("successful update did not poke")
	}
	before := called
	response := f.call(t, "whiteboard.update", map[string]any{"commands": `<wb:topic name="work"><wb:eta minutes="60" reason="same total"/></wb:topic>`})
	if !isToolError(response) || called != before {
		t.Fatal("refused batch poked")
	}
}

func TestWhiteboardNudgePokeResolvesCreatedCacheParents(t *testing.T) {
	parent := t.TempDir()
	real := filepath.Join(parent, "real")
	if err := os.Mkdir(real, 0700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(parent, "alias")
	if err := os.Symlink(real, alias); err != nil {
		t.Skipf("symlink fixture unavailable: %v", err)
	}
	n := NewWhiteboardNudges(filepath.Join(alias, "cache", "dashboard"), nil, io.Discard)
	if err := os.Mkdir(filepath.Join(real, "cache"), 0700); err != nil {
		t.Fatal(err)
	}
	target := nudgeTarget{"new-cache-lead", t.TempDir()}
	n.Poke(filepath.Join(real, "cache"), target.root, target.key)
	if !n.pending[target] {
		t.Fatal("newly-created cache canonicalization dropped update poke")
	}
	t.Logf("construction path %s; caller path %s", n.cache, filepath.Join(real, "cache"))
	n.Close()
}

func TestWhiteboardNudgeSchedulerCloseBeforeStart(t *testing.T) {
	f := newNudgeFixture(t)
	f.n.Close()
	f.n.Start()
	f.n.Close()
	select {
	case <-f.n.exited:
	default:
		t.Fatal("scheduler close did not settle")
	}
	var checks atomic.Int32
	f.n.alive = func(int) bool { checks.Add(1); return true }
	if checks.Load() != 0 {
		t.Fatal("closed scheduler processed")
	}
}
