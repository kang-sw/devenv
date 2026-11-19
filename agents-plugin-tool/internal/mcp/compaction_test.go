package mcp

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/kang-sw/devenv/internal/wsagent"
	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wslive"
)

func TestCompactionRegistryManifestParity(t *testing.T) {
	legacy, _ := loadDeclaredConfig(piManifestPath).lookup("pi.compaction_advisory_percent")
	if legacy.Pattern != wsconfig.ThresholdPattern {
		t.Fatal("threshold grammar drifted from shipped manifests")
	}
	for key, def := range wsconfig.CompactionDefaults {
		e, ok := resolveConfigEntryForKey(key)
		if !ok || e.HarnessBucketed() || !e.HarnessStored() || !e.RepoScoped() {
			t.Fatal(key, e)
		}
		if builtinConfigDefaults()[key] != def {
			t.Fatal(key)
		}
		for _, v := range thresholdAccepted {
			if _, err := e.Normalize(v); err != nil {
				t.Fatalf("%s rejected %s", key, v)
			}
		}
		for _, v := range thresholdRejected {
			if _, err := e.Normalize(v); err == nil {
				t.Fatalf("%s accepted %s", key, v)
			}
		}
	}
}

func TestCompactionConfigGetTuneAndLegacy(t *testing.T) {
	s, _, key := manifestServer(t)
	s.observeHarness("test", "pi")
	tune := func(k, v, harness, scope string) {
		t.Helper()
		r := keyedCall(t, s, 1, key, "config.tune", map[string]any{"key": k, "value": v, "harness": harness, "scope": scope})
		if toolIsError(t, r) {
			t.Fatal(r)
		}
	}
	// New harness values remain scalar overrides, including agy.
	tune("compaction.lead_advisory", "70%", "pi", "global")
	tune("compaction.lead_advisory", "40%", "*", "project")
	result, err := s.resolveConfigGet(key, "compaction.lead_advisory")
	if err != nil || result.Value != "40%" || result.Scope != "project" {
		t.Fatal(result, err)
	}
	tune("compaction.worker_hard", "90%", "agy", "project")
	resolver := s.sessionResolver(key, builtinConfigDefaults())
	rv, err := resolver.Get(key, "compaction.worker_hard.agy")
	if err != nil || rv.Value != "90%" {
		t.Fatal(rv, err)
	}
	// Unknown tool refusal remains the older-daemon contract (no dispatch).
	r := keyedCall(t, s, 2, key, "compact", map[string]any{})
	if toolIsError(t, r) || !strings.Contains(toolText(t, r), "unsupported") {
		t.Fatal(r)
	}
}

type compactionLiveBackend struct {
	mu     sync.Mutex
	starts int
	sends  []string
	proc   *lifecycleProcess
}

func (b *compactionLiveBackend) Start(context.Context, wsagent.StartSpec) (wsagent.Process, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.starts++
	p := &lifecycleProcess{
		backend: &lifecycleBackend{},
		id:      fmt.Sprintf("compact-holder-%d", b.starts),
		events:  make(chan wsagent.Event, 32),
	}
	b.proc = p
	return &compactionLiveProcess{lifecycleProcess: p, parent: b}, nil
}

func (b *compactionLiveBackend) counts() (int, []string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.starts, append([]string(nil), b.sends...)
}

func (b *compactionLiveBackend) emit(ev wsagent.Event) {
	b.mu.Lock()
	p := b.proc
	b.mu.Unlock()
	p.events <- ev
}

type compactionLiveProcess struct {
	*lifecycleProcess
	parent *compactionLiveBackend
}

func (p *compactionLiveProcess) Send(text string) error {
	p.parent.mu.Lock()
	p.parent.sends = append(p.parent.sends, text)
	p.parent.mu.Unlock()
	return nil
}

func startCompactionTestRunner(t *testing.T, s *Server) (*wsagent.Runner, *compactionLiveBackend) {
	t.Helper()
	keys := NewAgentKeyStore()
	backend := &compactionLiveBackend{}
	runner := wsagent.NewRunner(wsagent.Options{
		Dir:      t.TempDir(),
		Config:   s.configOptions(),
		Keys:     keys,
		LeadKeys: keys,
		Holders:  lifecycleHolders{},
		Backends: map[string]wsagent.BackendRegistration{
			wsagent.BackendClaude: {
				Backend:         backend,
				Catalog:         wsagent.ClaudeCatalog(),
				LeadProfile:     wsagent.LeadProfileClaude,
				PermissionModes: wsagent.LeadPermissionModes,
			},
		},
	})
	runner.Start(nil)
	t.Cleanup(runner.Close)
	return runner, backend
}

func TestCompactionConfigTuneLiveReloadsRunningConversation(t *testing.T) {
	s, root, key := manifestServer(t)
	s.observeHarness("test", wsagent.BackendClaude)
	tune := func(id int, args map[string]any) {
		t.Helper()
		r := keyedCall(t, s, id, key, "config.tune", args)
		if toolIsError(t, r) {
			t.Fatalf("config.tune(%v) failed: %s", args, r)
		}
	}

	// Start conversation with global hard threshold X = 80%.
	tune(1, map[string]any{"key": "compaction.lead_hard", "value": "80%", "harness": "*", "scope": "global"})
	runner, backend := startCompactionTestRunner(t, s)
	st, err := runner.CreateLead(wsagent.BackendClaude, root, wsagent.LeadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runner.Send(st.ID, "first"); err != nil {
		t.Fatal(err)
	}
	lifecycleEventually(t, "first turn sent", func() bool { _, sends := backend.counts(); return len(sends) == 1 })

	// Turn 1 ends at 60% usage: below X=80%, so no compaction triggers.
	tokens := int64(60000)
	backend.emit(wsagent.Event{Live: &wslive.Event{Kind: wslive.KindStatus, Context: &wslive.UsageContext{Tokens: &tokens, Window: 100000}}})
	backend.emit(wsagent.Event{TurnEnd: true})
	lifecycleEventually(t, "idle after turn 1", func() bool {
		got, err := runner.Get(st.ID)
		return err == nil && got.Status == wsagent.StatusIdle
	})
	if starts, sends := backend.counts(); starts != 1 || len(sends) != 1 {
		t.Fatalf("unexpected start/compact at 60%% < 80%%: starts=%d sends=%q", starts, sends)
	}

	// Tune global hard threshold to Y = 55% via config.tune without restarting.
	tune(2, map[string]any{"key": "compaction.lead_hard", "value": "55%", "harness": "*", "scope": "global"})

	// Next threshold check uses Y=55% and queues /compact on the same running process.
	backend.emit(wsagent.Event{Live: &wslive.Event{Kind: wslive.KindStatus, Context: &wslive.UsageContext{Tokens: &tokens, Window: 100000}}})
	lifecycleEventually(t, "compact sent at tuned threshold Y=55%", func() bool {
		_, sends := backend.counts()
		return len(sends) == 2 && sends[1] == "/compact"
	})
	if starts, _ := backend.counts(); starts != 1 {
		t.Fatalf("conversation restarted on live reload: starts=%d, want 1", starts)
	}
}

func TestCompactionConfigTuneProjectShadowsGlobalLiveReload(t *testing.T) {
	s, root, key := manifestServer(t)
	s.observeHarness("test", wsagent.BackendClaude)
	tune := func(id int, args map[string]any) {
		t.Helper()
		r := keyedCall(t, s, id, key, "config.tune", args)
		if toolIsError(t, r) {
			t.Fatalf("config.tune(%v) failed: %s", args, r)
		}
	}

	// Project-scope hard threshold 90% set before conversation starts.
	tune(1, map[string]any{"key": "compaction.lead_hard", "value": "90%", "harness": "*", "scope": "project"})
	runner, backend := startCompactionTestRunner(t, s)
	st, err := runner.CreateLead(wsagent.BackendClaude, root, wsagent.LeadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runner.Send(st.ID, "first"); err != nil {
		t.Fatal(err)
	}
	lifecycleEventually(t, "first turn sent", func() bool { _, sends := backend.counts(); return len(sends) == 1 })

	tokens60 := int64(60000)
	backend.emit(wsagent.Event{Live: &wslive.Event{Kind: wslive.KindStatus, Context: &wslive.UsageContext{Tokens: &tokens60, Window: 100000}}})
	backend.emit(wsagent.Event{TurnEnd: true})
	lifecycleEventually(t, "idle after turn 1", func() bool {
		got, err := runner.Get(st.ID)
		return err == nil && got.Status == wsagent.StatusIdle
	})

	// Global tune to 55% must still be shadowed by project scope 90%.
	tune(2, map[string]any{"key": "compaction.lead_hard", "value": "55%", "harness": "*", "scope": "global"})
	if _, err := runner.Send(st.ID, "second"); err != nil {
		t.Fatal(err)
	}
	// kick runs compactIdle before draining inbox: receiving "second" instead of
	// "/compact" proves 60% did not trigger under the shadowed global 55%.
	lifecycleEventually(t, "second turn sent without compact", func() bool {
		_, sends := backend.counts()
		return len(sends) == 2
	})
	if starts, sends := backend.counts(); starts != 1 || !strings.Contains(sends[1], "second") {
		t.Fatalf("global tune bypassed project shadow: starts=%d sends=%q", starts, sends)
	}
	backend.emit(wsagent.Event{Live: &wslive.Event{Kind: wslive.KindStatus, Context: &wslive.UsageContext{Tokens: &tokens60, Window: 100000}}})
	backend.emit(wsagent.Event{TurnEnd: true})
	lifecycleEventually(t, "idle after turn 2", func() bool {
		got, err := runner.Get(st.ID)
		return err == nil && got.Status == wsagent.StatusIdle
	})

	// Resetting the project override via config.tune exposes the global 55% live.
	tune(3, map[string]any{"key": "compaction.lead_hard", "reset": true, "harness": "*", "scope": "project"})
	backend.emit(wsagent.Event{Live: &wslive.Event{Kind: wslive.KindStatus, Context: &wslive.UsageContext{Tokens: &tokens60, Window: 100000}}})
	lifecycleEventually(t, "compact sent after project reset exposes global 55%", func() bool {
		_, sends := backend.counts()
		return len(sends) == 3 && sends[2] == "/compact"
	})
	if starts, _ := backend.counts(); starts != 1 {
		t.Fatalf("conversation restarted: starts=%d, want 1", starts)
	}
}
