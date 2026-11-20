package wsagent

import (
	"strings"
	"sync/atomic"
	"testing"

	"github.com/kang-sw/devenv/internal/wsconfig"
)

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
