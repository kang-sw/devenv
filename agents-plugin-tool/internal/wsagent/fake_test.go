package wsagent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
)

// fakeBackend records each start and hands out fakeProcs a test drives.
type fakeBackend struct {
	mu       sync.Mutex
	starts   []StartSpec
	procs    []*fakeProc
	startErr error
	// noModel makes SetModel unsupported, like an effort change.
	noModel bool
	// exe is the executable Executable reports (ChoiceLister); empty is
	// "fake".
	exe string
	// startGate, when set, holds every Start until released (gateStarts).
	startGate *gate
	// failSends is how many of the next sends fail, as a write to a closed
	// pipe does; negative fails every send.
	failSends int
	// steers makes its processes take a child's report mid-turn
	// (reportSteerer).
	steers bool
}

// takeSendFailure reports whether the next send fails (failSends).
func (b *fakeBackend) takeSendFailure() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.failSends > 0 {
		b.failSends--
		return true
	}
	return b.failSends < 0
}

func (b *fakeBackend) setFailSends(n int) {
	b.mu.Lock()
	b.failSends = n
	b.mu.Unlock()
}

// gate holds the calls of a fake until released, announcing each on
// entered; release also disarms it, so later calls pass.
type gate struct {
	entered chan struct{}
	ch      chan struct{}
	once    sync.Once
	disarm  func()
}

func newGate(t *testing.T, disarm func()) *gate {
	g := &gate{entered: make(chan struct{}, 16), ch: make(chan struct{}), disarm: disarm}
	t.Cleanup(g.release)
	return g
}

func (g *gate) wait() {
	g.entered <- struct{}{}
	<-g.ch
}

// awaitEntered waits until a held call began.
func (g *gate) awaitEntered(t *testing.T) {
	t.Helper()
	select {
	case <-g.entered:
	case <-time.After(5 * time.Second):
		t.Fatalf("no held call began")
	}
}

func (g *gate) release() {
	g.once.Do(func() {
		g.disarm()
		close(g.ch)
	})
}

// gateStarts holds every Start until the gate is released.
func (b *fakeBackend) gateStarts(t *testing.T) *gate {
	g := newGate(t, func() {
		b.mu.Lock()
		b.startGate = nil
		b.mu.Unlock()
	})
	b.mu.Lock()
	b.startGate = g
	b.mu.Unlock()
	return g
}

func (b *fakeBackend) Executable() string {
	if b.exe == "" {
		return "fake"
	}
	return b.exe
}

func (b *fakeBackend) Choices(context.Context) ([]ModelChoice, []string, error) {
	return []ModelChoice{{ID: b.Executable() + "-model"}}, []string{"low", "high"}, nil
}

func (b *fakeBackend) Start(_ context.Context, spec StartSpec) (Process, error) {
	b.mu.Lock()
	g := b.startGate
	b.mu.Unlock()
	if g != nil {
		g.wait()
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.startErr != nil {
		return nil, b.startErr
	}
	p := &fakeProc{backend: b, events: make(chan Event, 64), sent: make(chan string, 16), noModel: b.noModel}
	b.starts = append(b.starts, spec)
	b.procs = append(b.procs, p)
	if b.steers {
		return fakeSteerProc{p}, nil
	}
	return p, nil
}

func (b *fakeBackend) startCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.starts)
}

func (b *fakeBackend) start(i int) StartSpec {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.starts[i]
}

func (b *fakeBackend) proc(i int) *fakeProc {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.procs[i]
}

// waitProc waits for the i-th process to start (a send starts it in the
// conversation's loop, after the send returned) and returns it.
func (b *fakeBackend) waitProc(t *testing.T, i int) *fakeProc {
	t.Helper()
	eventually(t, fmt.Sprintf("process %d to start", i), func() bool { return b.startCount() > i })
	return b.proc(i)
}

type fakeProc struct {
	backend *fakeBackend
	events  chan Event
	sent    chan string
	noModel bool

	mu         sync.Mutex
	closed     bool
	interrupts int
	models     []string
	once       sync.Once
	// closeGate, when set, holds Close until released (gateClose).
	closeGate *gate
	// steers are the texts a fakeSteerProc was steered, and cleared counts
	// its ClearSteered calls.
	steers  []string
	cleared int
}

// fakeSteerProc is a fakeProc that takes a child's report mid-turn
// (reportSteerer); the test answers each steer (SteerAnswer, UserTaken).
type fakeSteerProc struct{ *fakeProc }

func (p fakeSteerProc) Steer(text string) error {
	p.mu.Lock()
	p.steers = append(p.steers, text)
	p.mu.Unlock()
	return nil
}

func (p fakeSteerProc) ClearSteered() error {
	p.mu.Lock()
	p.cleared++
	p.mu.Unlock()
	return nil
}

// steered is what the process was steered, and how often it was asked to
// drop its queued steers.
func (p *fakeProc) steered() ([]string, int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.steers...), p.cleared
}

// gateClose holds the process's Close until the gate is released.
func (p *fakeProc) gateClose(t *testing.T) *gate {
	g := newGate(t, func() {
		p.mu.Lock()
		p.closeGate = nil
		p.mu.Unlock()
	})
	p.mu.Lock()
	p.closeGate = g
	p.mu.Unlock()
	return g
}

func (p *fakeProc) Events() <-chan Event { return p.events }

func (p *fakeProc) Send(text string) error {
	if p.backend != nil && p.backend.takeSendFailure() {
		return errors.New("write |1: broken pipe")
	}
	p.sent <- text
	return nil
}

func (p *fakeProc) Interrupt() error {
	p.mu.Lock()
	p.interrupts++
	p.mu.Unlock()
	return nil
}

func (p *fakeProc) SetModel(model string) error {
	if p.noModel {
		return ErrUnsupported
	}
	p.mu.Lock()
	p.models = append(p.models, model)
	p.mu.Unlock()
	return nil
}

func (p *fakeProc) SetEffort(string) error { return ErrUnsupported }

func (p *fakeProc) Close() error {
	p.mu.Lock()
	g := p.closeGate
	p.mu.Unlock()
	if g != nil {
		g.wait()
	}
	p.once.Do(func() {
		p.mu.Lock()
		p.closed = true
		p.mu.Unlock()
		close(p.events)
	})
	return nil
}

func (p *fakeProc) isClosed() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.closed
}

// exit ends the process from its side, as a crash would.
func (p *fakeProc) exit() { p.once.Do(func() { close(p.events) }) }

// reply sends a reply and ends the turn.
func (p *fakeProc) reply(text string) {
	p.events <- Event{Live: &wslive.Event{Kind: wslive.KindReply, Text: text, Final: true}}
	p.events <- Event{TurnEnd: true}
}

func (p *fakeProc) nextSent(t *testing.T) string {
	t.Helper()
	select {
	case s := <-p.sent:
		return s
	case <-time.After(5 * time.Second):
		t.Fatalf("no turn was sent")
		return ""
	}
}

// fakeKeys mints sequential keys and records Ensure calls.
type fakeKeys struct {
	mu      sync.Mutex
	n       int
	ensured []string
	// mintGates hold the Mint calls for a root until released (gateMint).
	mintGates map[string]*gate
}

// gateMint holds Mint for root until the gate is released.
func (k *fakeKeys) gateMint(t *testing.T, root string) *gate {
	g := newGate(t, func() {
		k.mu.Lock()
		delete(k.mintGates, root)
		k.mu.Unlock()
	})
	k.mu.Lock()
	if k.mintGates == nil {
		k.mintGates = map[string]*gate{}
	}
	k.mintGates[root] = g
	k.mu.Unlock()
	return g
}

func (k *fakeKeys) Mint(root string) (string, error) {
	k.mu.Lock()
	g := k.mintGates[root]
	k.mu.Unlock()
	if g != nil {
		g.wait()
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	k.n++
	return fmt.Sprintf("fake-agent-key-%d-%d", time.Now().UnixNano(), k.n), nil
}

func (k *fakeKeys) Ensure(root, key string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	if key == "" {
		return errors.New("empty key")
	}
	k.ensured = append(k.ensured, key)
	return nil
}

// harness is a runner over a temporary daemon directory, a working root
// with its own profiles, and a fake claude backend.
type harness struct {
	t       *testing.T
	dir     string
	root    string
	config  Options
	backend *fakeBackend
	keys    *fakeKeys
	runner  *Runner
	// endpoint, when set, is the runner's SetEndpoint at each start.
	endpoint string
}

// suiteBackend is the backend the runner suite's profiles name and the fake
// is registered under; TestRunnerSuiteOnPi runs the suite again with pi.
// A profile written with "backend":"@other" names the backend left
// unregistered. Tests in this package never run in parallel.
var suiteBackend = BackendClaude

func otherBackend() string {
	if suiteBackend == BackendClaude {
		return BackendPi
	}
	return BackendClaude
}

func newHarness(t *testing.T, profiles map[string]string) *harness {
	t.Helper()
	h := &harness{t: t, dir: t.TempDir(), root: t.TempDir(), backend: &fakeBackend{}, keys: &fakeKeys{}}
	values := map[string]string{}
	for name, raw := range profiles {
		raw = strings.ReplaceAll(raw, `"backend":"claude"`, `"backend":"`+suiteBackend+`"`)
		raw = strings.ReplaceAll(raw, `"backend":"@other"`, `"backend":"`+otherBackend()+`"`)
		values[ProfilePrefix+name] = raw
	}
	writeStructured(t, filepath.Join(h.root, ".ws-workflow", "config.json"), values)
	if err := os.WriteFile(filepath.Join(h.root, "agent.md"), []byte("Answer questions about this project."), 0o644); err != nil {
		t.Fatal(err)
	}
	h.config = Options{
		Dir:      h.dir,
		Backends: map[string]Backend{suiteBackend: h.backend},
		Keys:     h.keys,
		Config:   testConfig(t),
		WSMCP: func(root string) (MCPCommand, error) {
			return MCPCommand{Command: "/bin/ws-mcp", Args: []string{"serve", "--stdio", "--root", root}}, nil
		},
		heartbeat: 50 * time.Millisecond,
	}
	h.start()
	return h
}

// start starts a fresh runner over the same directory, as a daemon
// (re)start does.
func (h *harness) start() {
	h.runner = NewRunner(h.config)
	if h.endpoint != "" {
		h.runner.SetEndpoint(h.endpoint)
	}
	h.runner.Start(nil)
	h.t.Cleanup(h.runner.Close)
}

func (h *harness) create(profile string) State {
	h.t.Helper()
	st, err := h.runner.Create(profile, h.root)
	if err != nil {
		h.t.Fatalf("Create(%s): %v", profile, err)
	}
	return st
}

func (h *harness) state(id string) State {
	h.t.Helper()
	st, err := h.runner.Get(id)
	if err != nil {
		h.t.Fatalf("Get: %v", err)
	}
	return st
}

// eventually polls cond until it holds or five seconds pass.
func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// readEventually returns read's first success within a few seconds and
// fails the test with its last error after that. On Windows a read that
// meets the runner's replace of the file in flight fails (a sharing
// violation); the next attempt does not meet it.
func readEventually[T any](t *testing.T, what string, read func() (T, error)) T {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		v, err := read()
		if err == nil {
			return v
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s: %v", what, err)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// nopReplies drops a mapper's answers and lines.
type nopReplies struct{}

func (nopReplies) answer(string, controlAnswer) {}
func (nopReplies) send(any)                     {}
