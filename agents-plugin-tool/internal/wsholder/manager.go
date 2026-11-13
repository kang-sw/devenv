package wsholder

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Sink receives every event of every attached holder, in order per holder,
// on that holder's reader goroutine. Records carry their sequence numbers:
// after a daemon restart the whole ring is replayed, so a consumer drops
// the records it already holds.
type Sink interface {
	HolderEvent(id string, ev Event)
}

// Options configures a Manager.
type Options struct {
	// Dir is the holders directory (DirName under the dashboard state
	// directory); it is created when absent.
	Dir string
	// Python is the interpreter that runs a holder (PythonEnv's value);
	// empty means python3 on PATH.
	Python string
	// Log receives diagnostics; nil discards them.
	Log io.Writer
	// RescanInterval is Run's period; zero means DefaultRescanInterval.
	RescanInterval time.Duration
	// Sink receives the attached holders' events; nil drops them. AddSink
	// adds another.
	Sink Sink
	// RingBytes is the size of the output ring a new holder keeps; zero is
	// the holder's own default.
	RingBytes int
	// OnExit persists a stopped holder's exit before its directory is removed.
	// An error retains the directory for a later scan to retry.
	OnExit func(Descriptor, Exit) error

	// holderArgs are extra holder arguments, so a test shortens its
	// periods. startTimeout bounds the wait for a started holder's
	// descriptor; zero means defaultStartTimeout. The rest replace OS
	// calls: lookPath (exec.LookPath), start (startDetached), breakaway
	// (breakawayApplies), procStart (processStartTime), procTicks
	// (processStartTicks), alive (processAlive), and kill (killHolder).
	holderArgs   []string
	startTimeout time.Duration
	lookPath     func(string) (string, error)
	start        func(exe string, args []string, dir string, breakaway bool) (*exec.Cmd, error)
	breakaway    *bool
	procStart    func(pid int) (time.Time, error)
	procTicks    func(pid int) (uint64, error)
	alive        func(pid int) bool
	kill         func(pid int) error
}

const (
	defaultStartTimeout = 20 * time.Second
	// startTimeTolerance is how far a holder's recorded start may sit from
	// the OS creation time of its PID for a guarded kill to proceed.
	startTimeTolerance = 2 * time.Second
	// stopWait bounds the wait for a holder to go after a stop or a kill.
	stopWait = 20 * time.Second
	// maxFailures and failureAge bound the start failures a listing keeps.
	maxFailures = 5
	failureAge  = time.Hour
)

var (
	// ErrUnknownHolder is Stop's answer for an id no holder has.
	ErrUnknownHolder = errors.New("unknown holder")
	// ErrStartTimeMismatch refuses a guarded kill: the process under the
	// holder's PID is not the one that wrote the descriptor.
	ErrStartTimeMismatch = errors.New("the process under the holder's PID does not match its recorded start time; not killed")
)

// Info is one holder as the daemon sees it.
type Info struct {
	ID  string `json:"id"`
	PID int    `json:"pid"`
	// Attached reports that this daemon holds the holder's attach slot;
	// Cause says why not otherwise.
	Attached bool  `json:"attached"`
	Cause    Cause `json:"cause,omitempty"`
	// Protocol is the version the holder reports.
	Protocol int `json:"protocol"`
	// StartedAt is the holder's start in epoch milliseconds.
	StartedAt int64 `json:"started_at"`
	// RestartSafe is false for a holder that may end with the daemon's
	// host (Descriptor.RestartSafe).
	RestartSafe bool            `json:"restart_safe"`
	Session     json.RawMessage `json:"session,omitempty"`
	// Child is the child's last known status, for an attached holder.
	Child *ChildStatus `json:"child,omitempty"`
	// Lost counts the records a replay reported gone from the ring.
	Lost uint64 `json:"lost,omitempty"`
}

// Exit is the optional exit.json written by a holder before it ends.
type Exit struct {
	ExitCode  *int   `json:"exit_code"`
	Signal    *int   `json:"signal"`
	Reason    string `json:"reason"`
	StoppedAt string `json:"stopped_at"`
}

// StartFailure is a holder that could not be started.
type StartFailure struct {
	// At is the failure's time in epoch milliseconds.
	At      int64           `json:"at"`
	Message string          `json:"message"`
	Session json.RawMessage `json:"session,omitempty"`
}

// Listing is every holder the manager knows and its recent start failures.
type Listing struct {
	Holders  []Info         `json:"holders"`
	Failures []StartFailure `json:"failures"`
}

type entry struct {
	desc   Descriptor
	client *Client
	cause  Cause
	// lastSeq is the last record sequence seen from this holder, kept in
	// memory only: a reattach in this process continues after it, and a
	// new daemon process replays the whole ring.
	lastSeq uint64
	lost    uint64
	child   ChildStatus
}

// Manager finds, starts, attaches to, and stops session holders for one
// holders directory. The daemon never ends a live holder on its own: only
// Stop does.
type Manager struct {
	opts Options
	// opMu serializes scans, launches, and stops, which attach and detach.
	opMu sync.Mutex
	mu   sync.Mutex
	// entries are the known holders by id.
	entries map[string]*entry
	// sinks receive the attached holders' events (Options.Sink, AddSink).
	sinks    []Sink
	failures []StartFailure
	closed   bool
	stop     chan struct{}
	stopOnce sync.Once
}

// New returns a manager for opts.Dir. It touches nothing until Scan, Run,
// or Launch.
func New(opts Options) *Manager {
	if opts.Log == nil {
		opts.Log = io.Discard
	}
	if opts.RescanInterval <= 0 {
		opts.RescanInterval = DefaultRescanInterval
	}
	if opts.startTimeout <= 0 {
		opts.startTimeout = defaultStartTimeout
	}
	if opts.lookPath == nil {
		opts.lookPath = exec.LookPath
	}
	if opts.start == nil {
		opts.start = startDetached
	}
	if opts.breakaway == nil {
		applies := breakawayApplies
		opts.breakaway = &applies
	}
	if opts.procStart == nil {
		opts.procStart = processStartTime
	}
	if opts.procTicks == nil {
		opts.procTicks = processStartTicks
	}
	if opts.alive == nil {
		opts.alive = processAlive
	}
	if opts.kill == nil {
		opts.kill = killHolder
	}
	m := &Manager{opts: opts, entries: map[string]*entry{}, stop: make(chan struct{})}
	if opts.Sink != nil {
		m.sinks = []Sink{opts.Sink}
	}
	return m
}

// AddSink adds a receiver of the attached holders' events, after the ones
// already there. A consumer that needs the manager itself (to answer an
// event with an operation) adds itself here before the first Scan or Run,
// so it misses no event.
func (m *Manager) AddSink(sink Sink) {
	m.mu.Lock()
	m.sinks = append(m.sinks[:len(m.sinks):len(m.sinks)], sink)
	m.mu.Unlock()
}

func (m *Manager) logf(format string, args ...any) {
	fmt.Fprintf(m.opts.Log, "holders: "+format+"\n", args...)
}

// Run scans at once, removes the staged scripts no live holder runs, and
// then rescans every RescanInterval until Close.
func (m *Manager) Run() {
	m.Scan()
	m.PruneScripts()
	ticker := time.NewTicker(m.opts.RescanInterval)
	defer ticker.Stop()
	for {
		select {
		case <-m.stop:
			return
		case <-ticker.C:
			m.Scan()
		}
	}
}

// Close ends the rescans and every attach connection. The holders and
// their children keep running for the next daemon.
func (m *Manager) Close() {
	m.stopOnce.Do(func() { close(m.stop) })
	m.mu.Lock()
	m.closed = true
	var clients []*Client
	for _, e := range m.entries {
		if e.client != nil {
			clients = append(clients, e.client)
		}
	}
	m.mu.Unlock()
	for _, c := range clients {
		c.Close()
	}
}

// Scan reads the holders directory and attaches to every holder this
// daemon is not attached to. A descriptor whose process is gone is removed
// with its directory, as is a directory left without a descriptor. So is one
// whose PID now belongs to another process: its endpoint does not answer and
// the process under the PID did not start when the descriptor says.
func (m *Manager) Scan() {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	m.scan()
}

func (m *Manager) scan() {
	if m.isClosed() {
		return
	}
	dirs, err := os.ReadDir(m.opts.Dir)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			m.logf("scan: %v", err)
		}
		dirs = nil
	}
	seen := map[string]bool{}
	for _, d := range dirs {
		id := d.Name()
		if !d.IsDir() || !ValidID(id) {
			continue
		}
		dir := filepath.Join(m.opts.Dir, id)
		desc, err := ReadDescriptor(dir)
		if err != nil {
			if info, statErr := os.Stat(dir); statErr == nil && time.Since(info.ModTime()) > orphanDirGrace {
				_ = os.RemoveAll(dir)
			}
			continue
		}
		seen[id] = true
		m.mu.Lock()
		e := m.entries[id]
		attached := e != nil && e.client != nil && e.desc.PID == desc.PID && e.desc.Port == desc.Port
		m.mu.Unlock()
		if attached {
			continue
		}
		if !m.opts.alive(desc.PID) {
			m.logf("holder %s (pid %d) is gone; removing its descriptor", id, desc.PID)
			if m.cleanup(desc, "unknown") == nil {
				seen[id] = false
			}
			continue
		}
		// A holder that died without removing its directory (a reboot, a
		// kill) leaves a descriptor whose PID the OS hands to another
		// process. Nothing could ever stop that entry, since the guarded
		// kill rightly refuses the stranger, so it is cleared here. Both
		// signs are required: a start mismatch alone is not proof on an OS
		// matched by wall clock, and a live holder answers its endpoint.
		if err := m.attach(desc); err != nil && attachCause(err) == CauseNoAnswer && !errors.Is(err, ErrDetached) &&
			errors.Is(m.sameProcess(desc), ErrStartTimeMismatch) {
			m.logf("holder %s is gone and pid %d belongs to another process; removing its descriptor", id, desc.PID)
			if m.cleanup(desc, "unknown") == nil {
				seen[id] = false
			}
		}
	}
	// A holder whose directory vanished stays while attached (it rewrites
	// its descriptor); an unattached one is forgotten.
	m.mu.Lock()
	for id, e := range m.entries {
		if !seen[id] && e.client == nil {
			delete(m.entries, id)
		}
	}
	m.mu.Unlock()
}

// cleanup persists an available exit before removing the holder directory.
// Old or abruptly killed holders may have no exit file.
func (m *Manager) cleanup(desc Descriptor, fallback string) error {
	dir := filepath.Join(m.opts.Dir, desc.ID)
	raw, err := os.ReadFile(filepath.Join(dir, "exit.json"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		m.logf("holder %s: read exit: %v", desc.ID, err)
		return err
	}
	exit := Exit{Reason: fallback, StoppedAt: time.Now().UTC().Format(time.RFC3339)}
	if err == nil {
		if err := json.Unmarshal(raw, &exit); err != nil {
			m.logf("holder %s: invalid exit: %v", desc.ID, err)
			return err
		}
	}
	if m.opts.OnExit != nil {
		if err := m.opts.OnExit(desc, exit); err != nil {
			m.logf("holder %s: persist exit: %v", desc.ID, err)
			return err
		}
	}
	code, signal := "unknown", "none"
	if exit.ExitCode != nil {
		code = strconv.Itoa(*exit.ExitCode)
	}
	if exit.Signal != nil {
		signal = strconv.Itoa(*exit.Signal)
	}
	m.logf("stopped holder %s (pid %d) at %s: exit %s, signal %s, reason %s", desc.ID, desc.PID, exit.StoppedAt, code, signal, exit.Reason)
	if err := os.RemoveAll(dir); err != nil {
		m.logf("holder %s: remove directory: %v", desc.ID, err)
		return err
	}
	m.forget(desc.ID)
	return nil
}

func (m *Manager) isClosed() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.closed
}

func (m *Manager) forget(id string) {
	m.mu.Lock()
	e := m.entries[id]
	delete(m.entries, id)
	m.mu.Unlock()
	if e != nil && e.client != nil {
		e.client.Close()
	}
}

// attachCause is why a failed attach left the holder unattached.
func attachCause(err error) Cause {
	var attachErr *AttachError
	if errors.As(err, &attachErr) {
		return attachErr.Cause
	}
	return CauseNoAnswer
}

// attach attaches to the holder desc names and starts its record stream,
// after the last sequence this process saw from it. The outcome lands in
// the holder's entry.
func (m *Manager) attach(desc Descriptor) error {
	m.mu.Lock()
	e := m.entries[desc.ID]
	if e == nil {
		e = &entry{}
		m.entries[desc.ID] = e
	}
	if e.client != nil {
		old := e.client
		e.client = nil
		defer old.Close()
	}
	e.desc = desc
	from := uint64(0)
	if e.lastSeq > 0 {
		from = e.lastSeq + 1
	}
	m.mu.Unlock()

	id := desc.ID
	ctx, cancel := context.WithTimeout(context.Background(), attachDialTimeout+2*attachStepTimeout)
	defer cancel()
	client, info, err := Attach(ctx, desc, func(ev Event) { m.event(id, ev) })
	if err != nil {
		cause := attachCause(err)
		m.mu.Lock()
		changed := e.cause != cause
		e.cause = cause
		m.mu.Unlock()
		if changed {
			m.logf("holder %s (pid %d) is unattached: %v", id, desc.PID, err)
		}
		return err
	}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		client.Close()
		return ErrDetached
	}
	e.client, e.cause, e.child = client, "", info.Child
	m.mu.Unlock()
	m.logf("attached to holder %s (pid %d, protocol %d)", id, desc.PID, info.Protocol)
	go func() {
		<-client.Done()
		m.mu.Lock()
		if e.client == client {
			e.client, e.cause = nil, CauseNoAnswer
		}
		m.mu.Unlock()
	}()
	if _, _, err := client.Replay(from); err != nil {
		client.Close()
		return err
	}
	return nil
}

// event records what an attached holder sent and passes it to the sink.
func (m *Manager) event(id string, ev Event) {
	var newlyLost uint64
	m.mu.Lock()
	if e := m.entries[id]; e != nil {
		switch ev.Event {
		case "output":
			if ev.Seq > e.lastSeq {
				e.lastSeq = ev.Seq
			}
		case "exit":
			if ev.Seq > e.lastSeq {
				e.lastSeq = ev.Seq
			}
			e.child.State, e.child.ExitCode, e.child.Signal = "exited", ev.ExitCode, ev.Signal
		case "gap":
			// Only records this process never received are lost: a
			// replay a consumer asked for from an earlier sequence
			// (Client.Replay) reports again the ones long gone.
			from := ev.From
			if from <= e.lastSeq {
				from = e.lastSeq + 1
			}
			if ev.To > from {
				newlyLost = ev.To - from
				e.lost += newlyLost
			}
			if ev.To > 0 && ev.To-1 > e.lastSeq {
				e.lastSeq = ev.To - 1
			}
		}
	}
	sinks := m.sinks
	m.mu.Unlock()
	if newlyLost > 0 {
		m.logf("holder %s lost %d records before %d: they left its ring while no daemon was attached", id, newlyLost, ev.To)
	}
	for _, sink := range sinks {
		sink.HolderEvent(id, ev)
	}
}

// Client returns the attach connection to holder id, nil when unattached.
func (m *Manager) Client(id string) *Client {
	m.mu.Lock()
	defer m.mu.Unlock()
	if e := m.entries[id]; e != nil {
		return e.client
	}
	return nil
}

// List returns the known holders, ordered by start, and the recent start
// failures, newest first.
func (m *Manager) List() Listing {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := Listing{Holders: []Info{}, Failures: []StartFailure{}}
	for _, e := range m.entries {
		info := Info{
			ID:          e.desc.ID,
			PID:         e.desc.PID,
			Attached:    e.client != nil,
			Protocol:    e.desc.Protocol,
			StartedAt:   int64(e.desc.StartedAt * 1000),
			RestartSafe: e.desc.RestartSafe,
			Session:     e.desc.Session,
			Lost:        e.lost,
		}
		if info.Attached {
			child := e.child
			info.Child = &child
		} else {
			info.Cause = e.cause
			if info.Cause == "" {
				info.Cause = CauseNoAnswer
			}
		}
		out.Holders = append(out.Holders, info)
	}
	sort.Slice(out.Holders, func(i, j int) bool {
		a, b := out.Holders[i], out.Holders[j]
		if a.StartedAt != b.StartedAt {
			return a.StartedAt < b.StartedAt
		}
		return a.ID < b.ID
	})
	cutoff := time.Now().Add(-failureAge).UnixMilli()
	for i := len(m.failures) - 1; i >= 0; i-- {
		if m.failures[i].At >= cutoff {
			out.Failures = append(out.Failures, m.failures[i])
		}
	}
	return out
}

// python resolves the interpreter a holder runs under.
func (m *Manager) python() (string, error) {
	if m.opts.Python != "" {
		return m.opts.Python, nil
	}
	path, err := m.opts.lookPath("python3")
	if err != nil {
		return "", fmt.Errorf("no interpreter for the session holder: %s is unset and python3 is not on PATH: %w", PythonEnv, err)
	}
	return path, nil
}

// LaunchSpec is one holder start.
type LaunchSpec struct {
	// Session is a JSON object the holder records in its descriptor, nil
	// for none.
	Session json.RawMessage
	// RingBytes is the size of this holder's output ring; zero is
	// Options.RingBytes.
	RingBytes int
}

// Launch starts a new holder for session (a JSON object the holder records
// in its descriptor, nil for none), waits for its descriptor, and attaches.
// A failure is also kept for the listing.
func (m *Manager) Launch(session json.RawMessage) (string, error) {
	return m.LaunchWith(LaunchSpec{Session: session})
}

// LaunchWith is Launch for a holder the spec tunes.
func (m *Manager) LaunchWith(spec LaunchSpec) (string, error) {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	id, err := m.launch(spec)
	if err != nil {
		m.logf("holder start failed: %v", err)
		m.mu.Lock()
		m.failures = append(m.failures, StartFailure{At: time.Now().UnixMilli(), Message: err.Error(), Session: spec.Session})
		if len(m.failures) > maxFailures {
			m.failures = m.failures[len(m.failures)-maxFailures:]
		}
		m.mu.Unlock()
		return "", err
	}
	return id, nil
}

func (m *Manager) launch(spec LaunchSpec) (string, error) {
	if m.isClosed() {
		return "", errors.New("the holder manager is closed")
	}
	session := spec.Session
	if len(session) == 0 {
		session = json.RawMessage("{}")
	}
	python, err := m.python()
	if err != nil {
		return "", err
	}
	scriptPath, err := StageScript(m.opts.Dir)
	if err != nil {
		return "", fmt.Errorf("stage the holder script: %w", err)
	}
	raw := make([]byte, 6)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	id := "h" + hex.EncodeToString(raw)
	dir := filepath.Join(m.opts.Dir, id)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	args := []string{scriptPath, "--dir", dir, "--id", id, "--session", string(session)}
	ring := spec.RingBytes
	if ring <= 0 {
		ring = m.opts.RingBytes
	}
	if ring > 0 {
		args = append(args, "--ring-bytes", strconv.Itoa(ring))
	}
	args = append(args, m.opts.holderArgs...)
	// The holder runs in the holders directory, not its own, so it can
	// remove its own directory when it exits.
	cmd, err := m.opts.start(python, args, m.opts.Dir, *m.opts.breakaway)
	if err != nil && *m.opts.breakaway {
		// The starter's job forbids breakaway: the holder still starts,
		// inside the job, and records that it may end with it.
		cmd, err = m.opts.start(python, append(args, "--restart-unsafe"), m.opts.Dir, false)
	}
	if err != nil {
		_ = os.RemoveAll(dir)
		return "", fmt.Errorf("start the session holder with %s: %w", python, err)
	}
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	deadline := time.Now().Add(m.opts.startTimeout)
	var desc Descriptor
	for {
		if desc, err = ReadDescriptor(dir); err == nil {
			break
		}
		select {
		case waitErr := <-exited:
			detail := logTail(dir)
			_ = os.RemoveAll(dir)
			return "", fmt.Errorf("the session holder (%s) ended before it registered: %v%s", python, exitText(waitErr), detail)
		case <-time.After(50 * time.Millisecond):
		}
		if time.Now().After(deadline) {
			_ = cmd.Process.Kill()
			detail := logTail(dir)
			_ = os.RemoveAll(dir)
			return "", fmt.Errorf("the session holder (%s) wrote no descriptor within %s%s", python, m.opts.startTimeout, detail)
		}
	}
	if err := m.attach(desc); err != nil {
		return "", fmt.Errorf("attach to the new session holder %s: %w", id, err)
	}
	return id, nil
}

func exitText(err error) string {
	if err == nil {
		return "exit status 0"
	}
	return err.Error()
}

// logTail is the end of a holder's own log, for a start failure's message.
func logTail(dir string) string {
	raw, err := os.ReadFile(filepath.Join(dir, logName))
	if err != nil || len(raw) == 0 {
		return ""
	}
	if len(raw) > 1024 {
		raw = raw[len(raw)-1024:]
	}
	return ": " + strings.TrimSpace(string(raw))
}

// Stop ends holder id and its child. An attached holder is asked through
// the protocol. One the daemon cannot attach to is killed with its process
// group (POSIX) or process tree (Windows), but only while the process under its PID still matches the start
// time its descriptor records, which guards against a reused PID;
// ErrStartTimeMismatch refuses the kill otherwise.
func (m *Manager) Stop(id string) error {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	m.mu.Lock()
	e := m.entries[id]
	var desc Descriptor
	var client *Client
	if e != nil {
		desc, client = e.desc, e.client
	}
	m.mu.Unlock()
	if e == nil {
		return ErrUnknownHolder
	}
	if client != nil {
		if err := client.Stop(); err == nil {
			select {
			case <-client.Done():
			case <-time.After(stopWait):
				client.Close()
			}
			if m.waitGone(desc.PID) {
				if err := m.cleanup(desc, "stop"); err != nil {
					return err
				}
				return nil
			}
		}
		// The holder did not stop through the protocol; fall through.
	}
	if !m.opts.alive(desc.PID) {
		return m.cleanup(desc, "unknown")
	}
	if err := m.sameProcess(desc); err != nil {
		return err
	}
	if err := m.opts.kill(desc.PID); err != nil {
		m.logf("kill holder %s (pid %d): %v", id, desc.PID, err)
	}
	if !m.waitGone(desc.PID) {
		return fmt.Errorf("holder process %d did not end", desc.PID)
	}
	if err := m.cleanup(desc, "killed"); err != nil {
		return err
	}
	m.logf("killed holder %s (pid %d) and its child", id, desc.PID)
	return nil
}

// sameProcess returns nil when the process under desc.PID is the holder
// that wrote desc, and the reason a kill is refused otherwise. The boot-clock
// start decides where the holder recorded one and the OS reports one; the
// wall-clock start, within startTimeTolerance, decides everywhere else.
func (m *Manager) sameProcess(desc Descriptor) error {
	if desc.StartTicks != 0 {
		ticks, err := m.opts.procTicks(desc.PID)
		if err != nil {
			return fmt.Errorf("the start time of process %d could not be read, so it was not killed: %w", desc.PID, err)
		}
		if ticks != 0 {
			if ticks != desc.StartTicks {
				return ErrStartTimeMismatch
			}
			return nil
		}
	}
	started, err := m.opts.procStart(desc.PID)
	if err != nil {
		return fmt.Errorf("the start time of process %d could not be read, so it was not killed: %w", desc.PID, err)
	}
	recorded := time.Unix(0, int64(desc.StartedAt*float64(time.Second)))
	if diff := started.Sub(recorded); math.Abs(float64(diff)) > float64(startTimeTolerance) {
		return ErrStartTimeMismatch
	}
	return nil
}

// waitGone waits for pid to end, up to stopWait.
func (m *Manager) waitGone(pid int) bool {
	deadline := time.Now().Add(stopWait)
	for m.opts.alive(pid) {
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(25 * time.Millisecond)
	}
	return true
}

// PruneScripts removes the staged scripts that no live holder's descriptor
// names. Run calls it once after its first scan.
func (m *Manager) PruneScripts() {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	keep := map[string]bool{}
	m.mu.Lock()
	for _, e := range m.entries {
		if e.desc.ScriptHash != "" {
			keep[e.desc.ScriptHash] = true
		}
	}
	m.mu.Unlock()
	pruneStagedScripts(m.opts.Dir, keep, time.Now())
}
