package wsagent

import (
	"encoding/json"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsinbox"
	"github.com/kang-sw/devenv/internal/wslive"
)

func agyPromptTexts(h *leadHarness, holder string) []string {
	var out []string
	for _, raw := range h.holders.acpInputs(holder, "session/prompt") {
		var params struct {
			Prompt []struct{ Type, Text string } `json:"prompt"`
		}
		_ = json.Unmarshal(raw, &params)
		if len(params.Prompt) == 1 && params.Prompt[0].Type == "text" {
			out = append(out, params.Prompt[0].Text)
		}
	}
	return out
}

type ambientKeyStore struct {
	value string
	reads atomic.Int32
}

func (s *ambientKeyStore) Mint(string) (string, error) { return "key", nil }
func (s *ambientKeyStore) Ensure(string, string) error { return nil }
func (s *ambientKeyStore) Root(string) (string, bool)  { return "", false }
func (s *ambientKeyStore) GetOverride(string, string) (string, bool) {
	s.reads.Add(1)
	return s.value, s.value != ""
}

func TestWorkflowLanguageInstruction(t *testing.T) {
	if got := WorkflowLanguageInstruction("  Korean  "); got != "Respond to the user in Korean. Write ticket frontmatter titles in that language. Keep all internal reasoning, subagent prompts, code comments, and other AI-authored artifacts in English." {
		t.Fatalf("instruction = %q", got)
	}
	if got := WorkflowLanguageInstruction("  "); got != "" {
		t.Fatalf("empty instruction = %q", got)
	}
}

func TestTurnInputAmbientOrderingAndRoles(t *testing.T) {
	wantBlock := ambientBlock(WorkflowLanguageInstruction("Korean"))
	for _, backend := range []string{BackendClaude, BackendCodex, BackendAgy, BackendPi} {
		t.Run(backend+" lead", func(t *testing.T) {
			c := &conv{rec: Record{Lead: true, Backend: backend}, ambient: wantBlock, compactNote: "restore once"}
			got := c.turnInput("ordinary")
			if got != "restore once\n\nordinary\n\n"+wantBlock {
				t.Fatalf("turn input = %q", got)
			}
			if got := c.turnInput("playbook body"); got != "restore once\n\nplaybook body\n\n"+wantBlock {
				t.Fatalf("composition must not consume the note itself: %q", got)
			}
		})
	}
	worker := &conv{rec: Record{Backend: BackendCodex}, compactNote: "restore once"}
	if got := worker.turnInput("ordinary"); got != "restore once\n\nordinary" {
		t.Fatalf("worker input = %q", got)
	}
}

func TestHostedLeadAmbientReachesEveryBackendSend(t *testing.T) {
	type backendCase struct {
		name   string
		new    func(*testing.T) (*leadHarness, func(string))
		start  func(*leadHarness)
		inputs func(*leadHarness, string) []string
	}
	cases := []backendCase{
		{
			name: BackendClaude,
			new: func(t *testing.T) (*leadHarness, func(string)) {
				return newLeadHarness(t), func(string) {}
			},
			start:  func(h *leadHarness) { h.start() },
			inputs: (*leadHarness).userInputs,
		},
		{
			name: BackendPi,
			new: func(t *testing.T) (*leadHarness, func(string)) {
				return newLeadHarness(t), func(string) {}
			},
			start:  func(h *leadHarness) { h.start() },
			inputs: piPrompts,
		},
		{
			name: BackendCodex,
			new: func(t *testing.T) (*leadHarness, func(string)) {
				h, fake := newCodexLeadHarness(t)
				return h, func(id string) { fake.replyID = id }
			},
			start:  func(h *leadHarness) { h.startAt(codexSyntheticEndpoint) },
			inputs: codexUserInputs,
		},
		{
			name: BackendAgy,
			new: func(t *testing.T) (*leadHarness, func(string)) {
				h, fake := newAgyLeadHarness(t)
				return h, func(id string) { fake.replyID = id }
			},
			start:  func(h *leadHarness) { h.startAt("http://127.0.0.1:1") },
			inputs: agyPromptTexts,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h, bind := tc.new(t)
			resolver := wsconfig.NewResolver(h.opts.Config, nil, nil, nil)
			if err := resolver.Set(wsconfig.ItemWorkflowLang, "Korean", wsconfig.SetOptions{ExplicitScope: wsconfig.ScopeGlobal}); err != nil {
				t.Fatal(err)
			}
			tc.start(h)
			id := h.lead(tc.name, LeadOptions{}).ID
			bind(id)
			h.send(id, "hello")
			holder := h.holder(id)
			want := h.envelope("hello") + "\n\n" + ambientBlock(WorkflowLanguageInstruction("Korean"))
			eventually(t, tc.name+" ambient input", func() bool {
				return slices.Equal(tc.inputs(h, holder), []string{want})
			})
		})
	}
}

func TestAmbientCacheReadsOnlyOnRefresh(t *testing.T) {
	store := &ambientKeyStore{value: "Korean"}
	r := &Runner{opts: Options{Keys: store, Config: wsconfig.Options{}}, convs: map[string]*conv{}}
	c := &conv{r: r, rec: Record{Lead: true, WSKey: "lead-key", Root: t.TempDir(), ReplyID: "lead"}}
	c.refreshAmbient()
	if got := store.reads.Load(); got != 1 {
		t.Fatalf("initial config reads = %d, want 1", got)
	}
	for range 5 {
		got := c.turnInput("hello")
		if strings.Count(got, "<"+ambientBlockName+">") != 1 {
			t.Fatalf("ambient count = %d in %q", strings.Count(got, "<"+ambientBlockName+">"), got)
		}
	}
	if got := store.reads.Load(); got != 1 {
		t.Fatalf("steady-state config reads = %d, want 1", got)
	}
	store.value = "Japanese"
	c.refreshAmbient()
	if got := store.reads.Load(); got != 2 {
		t.Fatalf("refresh config reads = %d, want 2", got)
	}
	if got := c.turnInput("hello"); !strings.Contains(got, "Respond to the user in Japanese.") || strings.Count(got, "<"+ambientBlockName+">") != 1 {
		t.Fatalf("refreshed input = %q", got)
	}
}

func TestHostedLeadAmbientSendRefreshRetryAndHistory(t *testing.T) {
	h := newLeadHarness(t)
	reg := h.opts.Backends[BackendClaude]
	reg.PlaybookCommands = true
	h.opts.Backends[BackendClaude] = reg
	h.opts.PlaybookCommands = func() ([]wslive.SlashCommand, error) {
		return []wslive.SlashCommand{{Name: "lead-run", Description: "Run a ticket"}}, nil
	}
	h.opts.ExpandCommand = func(_, _, _, _ string) (string, error) {
		return "# Lead Run\nbody", nil
	}
	resolver := wsconfig.NewResolver(h.opts.Config, nil, nil, nil)
	if err := resolver.Set(wsconfig.ItemWorkflowLang, "Korean", wsconfig.SetOptions{ExplicitScope: wsconfig.ScopeGlobal}); err != nil {
		t.Fatal(err)
	}
	h.start()

	id := h.lead(BackendClaude, LeadOptions{}).ID
	h.send(id, "one")
	holder := h.holder(id)
	korean := ambientBlock(WorkflowLanguageInstruction("Korean"))
	eventually(t, "ordinary turn ambient", func() bool {
		return slices.Equal(h.userInputs(holder), []string{h.envelope("one") + "\n\n" + korean})
	})
	h.holders.out(holder, turnLines("sess-1", "u1", "first")...)
	h.idle(id)

	if err := resolver.Set(wsconfig.ItemWorkflowLang, "Japanese", wsconfig.SetOptions{ExplicitScope: wsconfig.ScopeGlobal}); err != nil {
		t.Fatal(err)
	}
	h.runner.RefreshAmbient()
	h.sendCommand(id, "lead-run", "ticket-x")
	playbook := playbookCommandTurn(wsinbox.Command{Name: "lead-run", Args: "ticket-x"}, "# Lead Run\nbody")
	japanese := ambientBlock(WorkflowLanguageInstruction("Japanese"))
	eventually(t, "expanded playbook turn ambient", func() bool {
		return slices.Equal(h.userInputs(holder), []string{
			h.envelope("one") + "\n\n" + korean,
			playbook + "\n\n" + japanese,
		})
	})
	h.holders.out(holder, turnLines("sess-1", "u2", "ran")...)
	h.idle(id)

	// A failed send keeps the original input and retries it on a fresh process;
	// composition is repeated from the unwrapped queue value, so ambient appears
	// exactly once rather than nesting around the prior attempt.
	h.holders.failInputs("user", 1)
	h.send(id, "retry")
	var retryHolder string
	eventually(t, "retried turn ambient", func() bool {
		retryHolder = h.record(id).HolderID
		if retryHolder == "" || retryHolder == holder {
			return false
		}
		got := h.userInputs(retryHolder)
		return slices.Equal(got, []string{h.envelope("retry") + "\n\n" + japanese}) &&
			strings.Count(got[0], "<"+ambientBlockName+">") == 1
	})
	h.holders.out(retryHolder, turnLines("sess-2", "u3", "retried")...)
	h.idle(id)

	// The dashboard/history projection retains only the original user text and
	// typed command; model-only ambient never becomes a visible user message.
	if got, want := ringTexts(h.ring(id), wslive.RingUser), []string{h.envelope("one"), "/lead-run ticket-x", h.envelope("retry")}; !slices.Equal(got, want) {
		t.Fatalf("visible user history = %q, want %q", got, want)
	}
}
