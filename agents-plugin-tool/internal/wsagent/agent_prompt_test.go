// agent_prompt_test.go holds agent prompt and child authority tests.
package wsagent

import (
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wsrsrc"
)

func TestAgentRenderedPlaybookPromptReachesChildAndKeepsCaps(t *testing.T) {
	h, _ := agentHarness(t, nil)
	playbook, err := wsrsrc.Load(filepath.Join("..", "..", "..", "agents-plugin", "rsrc"), "explore", "codex", "", map[string]string{
		"ExploreAgent":  "an explorer subagent",
		"SpawnIdiom":    "creating a new Codex task",
		"ContinueIdiom": "resuming the agent using its task id",
	})
	if err != nil {
		t.Fatal(err)
	}
	if playbook.Meta.Delegates {
		t.Fatal("the shipped explore playbook still declares delegates")
	}
	if playbook.Meta.Tier != "small" {
		t.Fatalf("the shipped explore tier = %q, want small", playbook.Meta.Tier)
	}
	path := filepath.Join(h.root, "rendered-explore.md")
	if err := os.WriteFile(path, []byte(playbook.Body), 0o644); err != nil {
		t.Fatal(err)
	}

	a, _ := h.agentRunning(h.leadCaller(), AgentSpawnRequest{
		Prompt:           "explore the cache",
		Tier:             playbook.Meta.Tier,
		Authority:        "read-only",
		SystemPromptPath: "rendered-explore.md",
	})
	text := instructionsOf(t, h.backend.start(0))
	want := playbook.Body + "\n\nYour ws session key is `" + h.record(a).WSKey + "`. Pass it as the `session_key` argument of every ws tool call. " +
		"This key is bound to worktree `" + h.root + "`; if work moves to another worktree, use a key minted for that destination (from a direct spawn there or a legitimate acquisition that returns one), because this key is never rebound. " +
		"Your direct parent's mailbox reply address is `id:" + leadParent.ReplyID + "`; send it reports or questions with `mailbox.send`.\n"
	if text != want {
		t.Fatalf("the rendered playbook body was not delivered exactly: %q, want %q", text, want)
	}

	_, err = h.runner.AgentSpawn(h.holderOf(a), AgentSpawnRequest{Prompt: "x", Tier: "small", Authority: "workdir-write"})
	agentRefused(t, err, http.StatusForbidden, "authority workdir-write is above this session's own level read-only")
	_, err = h.runner.AgentSpawn(h.holderOf(a), AgentSpawnRequest{Prompt: "x", Tier: "medium", Authority: "read-only"})
	agentRefused(t, err, http.StatusForbidden, `tier "medium" is above this conversation's own tier "small"`)
}

// TestAgentChildPromptCarriesNoLeadManual: an agent.spawn child's
// appended prompt is its system prompt, its profile text, its key, and its
// direct parent reply address, as before the lead manual: the daemon's lead
// manual render is never called for it and none of its text reaches the child.
func TestAgentChildPromptCarriesNoLeadManual(t *testing.T) {
	manual := &leadManualRender{text: "# Lead workflow manual sentinel"}
	h, _ := agentHarness(t, func(o *Options) { o.LeadManual = manual.render })
	a, _ := h.agentRunning(h.leadCaller(), AgentSpawnRequest{Prompt: "go", Tier: "small", Authority: "read-only", SystemPrompt: "You are the cache probe."})
	text := instructionsOf(t, h.backend.start(0))
	want := "You are the cache probe.\n\nYour ws session key is `" + h.record(a).WSKey + "`. Pass it as the `session_key` argument of every ws tool call. " +
		"This key is bound to worktree `" + h.root + "`; if work moves to another worktree, use a key minted for that destination (from a direct spawn there or a legitimate acquisition that returns one), because this key is never rebound. " +
		"Your direct parent's mailbox reply address is `id:" + leadParent.ReplyID + "`; send it reports or questions with `mailbox.send`.\n"
	if text != want {
		t.Fatalf("the child's appended prompt = %q, want %q", text, want)
	}
	if calls := manual.list(); len(calls) != 0 {
		t.Fatalf("a child start rendered the lead manual: %v", calls)
	}
}

func TestAgentSpawnCapsAuthorityAndTier(t *testing.T) {
	h, _ := agentHarness(t, nil)
	lead := h.leadCaller()
	_, err := h.runner.AgentSpawn(lead, AgentSpawnRequest{Prompt: "x", Tier: "small"})
	agentRefused(t, err, http.StatusBadRequest, "authority is required")
	_, err = h.runner.AgentSpawn(lead, AgentSpawnRequest{Prompt: "x", Tier: "small", Authority: "admin"})
	agentRefused(t, err, http.StatusBadRequest, "authority must be read-only, workdir-write, or full")

	// A lead parent with no Runner record caps at workdir-write.
	_, err = h.runner.AgentSpawn(lead, AgentSpawnRequest{Prompt: "x", Tier: "small", Authority: "full"})
	agentRefused(t, err, http.StatusForbidden, "authority full is above this session's own level workdir-write")
	w, _ := h.agentRunning(lead, AgentSpawnRequest{Prompt: "writer", Tier: "medium", Authority: "workdir-write"})
	r, _ := h.agentRunning(lead, AgentSpawnRequest{Prompt: "reader", Tier: "small", Authority: "read-only"})
	if lvl := childLevel(wsdashboard.SessionDir(h.dir, w)); lvl != PermissionWorkdirWrite {
		t.Fatalf("the child's stored level = %q", lvl)
	}

	// A conversation parent caps at its own explicit level: at it is
	// admitted, above it refused.
	_, err = h.runner.AgentSpawn(h.holderOf(w), AgentSpawnRequest{Prompt: "x", Tier: "small", Authority: "full"})
	agentRefused(t, err, http.StatusForbidden, "above this session's own level workdir-write")
	h.agentSpawn(h.holderOf(w), AgentSpawnRequest{Prompt: "x", Tier: "small", Authority: "workdir-write"})
	_, err = h.runner.AgentSpawn(h.holderOf(r), AgentSpawnRequest{Prompt: "x", Tier: "small", Authority: "workdir-write"})
	agentRefused(t, err, http.StatusForbidden, "above this session's own level read-only")
	h.agentSpawn(h.holderOf(r), AgentSpawnRequest{Prompt: "x", Tier: "small", Authority: "read-only"})

	// A nested spawn's tier may not pass its holder's.
	_, err = h.runner.AgentSpawn(h.holderOf(r), AgentSpawnRequest{Prompt: "x", Tier: "medium", Authority: "read-only"})
	agentRefused(t, err, http.StatusForbidden, `tier "medium" is above this conversation's own tier "small"`)
	_, err = h.runner.AgentSpawn(h.holderOf(w), AgentSpawnRequest{Prompt: "x", Tier: "pi.large", Authority: "read-only"})
	agentRefused(t, err, http.StatusForbidden, `tier "large" is above this conversation's own tier "medium"`)

	// A conversation parent with no record has no level: admission fails.
	ghost := AgentCaller{Parent: ParentRef{Kind: ParentConversation, ReplyID: "nosuchconversation"}, Root: h.root, Harness: BackendClaude, Config: h.config.Config}
	_, err = h.runner.AgentSpawn(ghost, AgentSpawnRequest{Prompt: "x", Tier: "small", Authority: "read-only"})
	agentRefused(t, err, http.StatusForbidden, "permission admission", "has no record")
}

func TestAgentChildGetsATokenAndItsAuthority(t *testing.T) {
	h, _ := agentHarness(t, nil)
	lead := h.leadCaller()
	for i, tc := range []struct {
		tier    string
		level   PermissionLevel
		backend string
	}{
		{"small", PermissionWorkdirWrite, BackendClaude},
		{"pi.small", PermissionReadOnly, BackendPi},
	} {
		h.agentRunning(lead, AgentSpawnRequest{Prompt: "x", Tier: tc.tier, Authority: string(tc.level)})
		spec := h.backend.start(i)
		if spec.WSMCP == nil {
			t.Fatalf("%s: no ws-mcp", tc.tier)
		}
		env := spec.WSMCP.Env
		tools := strings.Split(env[EnvAllowedTools], ",")
		if len(env[EnvPiToken]) != 64 || env[EnvPiEndpoint] != h.endpoint || env[EnvPermissionLevel] != string(tc.level) ||
			!slices.Contains(tools, "agent.spawn") || !slices.Contains(tools, "mailbox.send") || slices.Contains(tools, "pi.spawn") || spec.Profile.Backend != tc.backend {
			t.Fatalf("%s: the child's start = %+v, env %v", tc.tier, spec, env)
		}
		if code, body := h.tokenCall(env[EnvPiToken], "agent.list", nil); code != http.StatusOK {
			t.Fatalf("%s: the child's token lists = %d %s", tc.tier, code, body)
		}
	}
}

func TestSpawnedWorkerContextNamesDirectParentReturnAddress(t *testing.T) {
	h, _ := agentHarness(t, nil)
	parent, _ := h.agentRunning(h.leadCaller(), AgentSpawnRequest{Prompt: "parent", Tier: "medium", Authority: "workdir-write"})
	h.agentRunning(h.holderOf(parent), AgentSpawnRequest{Prompt: "child", Tier: "small", Authority: "workdir-write"})

	for i, want := range []string{"id:" + leadParent.ReplyID, "id:" + parent} {
		spec := h.backend.start(i)
		raw, err := os.ReadFile(spec.Instructions)
		if err != nil {
			t.Fatal(err)
		}
		text := string(raw)
		if strings.Count(text, "direct parent's mailbox reply address is") != 1 ||
			!strings.Contains(text, "direct parent's mailbox reply address is `"+want+"`") ||
			!strings.Contains(text, "`mailbox.send`") || strings.Contains(text, "@worktree") {
			t.Fatalf("worker %d instructions do not name direct parent %q: %q", i, want, text)
		}
		if !strings.Contains(text, "This key is bound to worktree `"+spec.Root+"`") ||
			!strings.Contains(text, "direct spawn there or a legitimate acquisition that returns one") ||
			!strings.Contains(text, "this key is never rebound") {
			t.Fatalf("worker %d instructions do not diagnose cross-worktree key reuse: %q", i, text)
		}
		if i == 1 && strings.Contains(text, "direct parent's mailbox reply address is `id:"+leadParent.ReplyID+"`") {
			t.Fatalf("nested worker context names the root lead instead of its direct parent: %q", text)
		}
	}
}

func TestSpawnedWorkerKeyIsMintedForExplicitWorktree(t *testing.T) {
	h, _ := agentHarness(t, nil)
	for _, args := range [][]string{
		{"init"},
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "Test"},
		{"commit", "--allow-empty", "-m", "base"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = h.root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	destination := filepath.Join(t.TempDir(), "destination")
	cmd := exec.Command("git", "worktree", "add", "-b", "impl/test/destination", destination, "HEAD")
	cmd.Dir = h.root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git worktree add: %v\n%s", err, out)
	}
	h.agentRunning(h.leadCaller(), AgentSpawnRequest{
		Prompt: "work there", Tier: "small", Authority: "workdir-write", Worktree: destination,
	})
	spec := h.backend.start(0)
	if spec.Root != destination {
		t.Fatalf("spawn root = %q, want destination %q", spec.Root, destination)
	}
	raw, err := os.ReadFile(spec.Instructions)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "This key is bound to worktree `"+destination+"`") {
		t.Fatalf("explicit-worktree instructions do not bind the minted key to the destination: %q", raw)
	}
}

// The child's explicit level survives a daemon restart, and its next start
// is admitted and carries it again.
func TestAgentChildLevelSurvivesARestart(t *testing.T) {
	h, _ := agentHarness(t, nil)
	lead := h.leadCaller()
	r, rp := h.agentRunning(lead, AgentSpawnRequest{Prompt: "x", Tier: "small", Authority: "read-only"})
	rp.reply("done")
	eventually(t, "the turn settled", func() bool { return h.state(r).Status == StatusIdle })
	h.runner.Close()
	h.start()
	if c := findAgent(t, h.runner.AgentList(lead), r); c.Authority != PermissionReadOnly {
		t.Fatalf("after the restart = %+v", c)
	}
	if _, err := h.runner.Deliver(r, Delivery{Text: "again"}); err != nil {
		t.Fatal(err)
	}
	h.backend.waitProc(t, 1).nextSent(t)
	if lvl := h.backend.start(1).WSMCP.Env[EnvPermissionLevel]; lvl != string(PermissionReadOnly) {
		t.Fatalf("the restarted child's level = %q", lvl)
	}
}

// A child whose conversation parent's record is gone after a restart is
// refused at its next start: no guessed ceiling.
func TestAgentChildOfAGoneParentIsRefusedAtItsNextStart(t *testing.T) {
	refusals := &wakeRefusals{}
	h, _ := agentHarness(t, func(o *Options) { o.WakeRefused = refusals.add })
	a, b := settledPair(t, h)
	h.runner.Close()
	if err := os.RemoveAll(wsdashboard.SessionDir(h.dir, a)); err != nil {
		t.Fatal(err)
	}
	h.start()
	starts := h.backend.startCount()
	if _, err := h.runner.Deliver(b, Delivery{From: "senderreplyid", Text: "wake"}); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the refused start", func() bool { return strings.Contains(h.state(b).Error, "has no record") })
	time.Sleep(60 * time.Millisecond)
	if n := h.backend.startCount(); n != starts {
		t.Fatalf("a refused start launched %d", n-starts)
	}
	if got := refusals.list(); len(got) != 1 || !strings.Contains(got[0][2], "has no record") {
		t.Fatalf("the sender's notices = %v", got)
	}
}

// expiryHarness is agentHarness on a clock the test moves, with a two-day
// child-agent-waiting expiry.
