package wsagent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
}

func (b *fakeBackend) Start(_ context.Context, spec StartSpec) (Process, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.startErr != nil {
		return nil, b.startErr
	}
	p := &fakeProc{events: make(chan Event, 64), sent: make(chan string, 16), noModel: b.noModel}
	b.starts = append(b.starts, spec)
	b.procs = append(b.procs, p)
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

type fakeProc struct {
	events  chan Event
	sent    chan string
	noModel bool

	mu         sync.Mutex
	closed     bool
	interrupts int
	models     []string
	once       sync.Once
}

func (p *fakeProc) Events() <-chan Event { return p.events }

func (p *fakeProc) Send(text string) error {
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
}

func (k *fakeKeys) Mint(root string) (string, error) {
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
}

func newHarness(t *testing.T, profiles map[string]string) *harness {
	t.Helper()
	h := &harness{t: t, dir: t.TempDir(), root: t.TempDir(), backend: &fakeBackend{}, keys: &fakeKeys{}}
	values := map[string]string{}
	for name, raw := range profiles {
		values[ProfilePrefix+name] = raw
	}
	writeStructured(t, filepath.Join(h.root, ".ws-workflow", "config.json"), values)
	if err := os.WriteFile(filepath.Join(h.root, "agent.md"), []byte("Answer questions about this project."), 0o644); err != nil {
		t.Fatal(err)
	}
	h.config = Options{
		Dir:      h.dir,
		Backends: map[string]Backend{BackendClaude: h.backend},
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
