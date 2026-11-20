// agent_worktree_test.go holds concurrent spawn and worktree child tests.
package wsagent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"slices"
	"sync"
	"testing"

	"github.com/kang-sw/devenv/internal/wsgit"
)

func TestAgentSpawnWidthCapAtomicUnderConcurrentSpawns(t *testing.T) {
	h, _ := agentHarness(t, nil)
	lead := h.leadCaller()
	const remaining = 5
	for i := range MaxSpawnWidth - remaining {
		h.agentSpawn(lead, AgentSpawnRequest{Prompt: fmt.Sprintf("pre %d", i), Tier: "small", Authority: "read-only"})
	}
	const contenders = 20
	start := make(chan struct{})
	errs := make(chan error, contenders)
	var wg sync.WaitGroup
	for i := range contenders {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			<-start
			_, err := h.runner.AgentSpawn(lead, AgentSpawnRequest{Prompt: fmt.Sprintf("race %d", n), Tier: "small", Authority: "read-only"})
			errs <- err
		}(i)
	}
	close(start)
	wg.Wait()
	close(errs)

	succeeded, refused := 0, 0
	for err := range errs {
		if err == nil {
			succeeded++
			continue
		}
		agentRefused(t, err, http.StatusForbidden, "the spawn width cap is 99 awake direct children")
		refused++
	}
	if succeeded != remaining || refused != contenders-remaining {
		t.Fatalf("concurrent spawn results: succeeded=%d refused=%d, want %d and %d", succeeded, refused, remaining, contenders-remaining)
	}
	if n := len(h.runner.AgentList(lead)); n != MaxSpawnWidth {
		t.Fatalf("AgentList count = %d, want %d", n, MaxSpawnWidth)
	}
	if n := h.runner.awakeChildren(leadParent); n != MaxSpawnWidth {
		t.Fatalf("awakeChildren = %d, want %d", n, MaxSpawnWidth)
	}
	h.runner.mu.Lock()
	pending := len(h.runner.pendingSpawns)
	h.runner.mu.Unlock()
	if pending != 0 {
		t.Fatalf("pendingSpawns leaked %d entries", pending)
	}
}

// TestWorktreeActiveChildrenAndSpawnReleaseReservation verifies daemon-wide
// bound-child discovery (direct, nested grandchild, sleeping/waiting non-closed
// children, and closed exclusion), the /agents/by-token/worktree.children
// token route, and mutual exclusion between AgentSpawn(worktree: ...) and the
// per-worktree release lock.
func TestWorktreeActiveChildrenAndSpawnReleaseReservation(t *testing.T) {
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
	wt := filepath.Join(t.TempDir(), "pooled-wt")
	cmd := exec.Command("git", "worktree", "add", "-b", "impl/test/bound", wt, "HEAD")
	cmd.Dir = h.root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git worktree add: %v\n%s", err, out)
	}

	lead := h.leadCaller()
	// While a release holds the exclusive per-worktree lock, AgentSpawn into
	// that worktree must be refused without registering a child.
	releaseLock, err := wsgit.WorktreeReleaseLock(context.Background(), wsgit.ExecRunner{}, wt)
	if err != nil {
		t.Fatal(err)
	}
	if err := releaseLock.Lock(); err != nil {
		t.Fatal(err)
	}
	_, err = h.runner.AgentSpawn(lead, AgentSpawnRequest{
		Prompt: "raced spawn", Tier: "small", Authority: "workdir-write", Worktree: wt,
	})
	agentRefused(t, err, http.StatusConflict, "currently being released")
	if got := h.runner.ActiveChildrenInWorktree(wt); len(got) != 0 {
		t.Fatalf("refused spawn left bound children: %+v", got)
	}
	if err := releaseLock.Unlock(); err != nil {
		t.Fatal(err)
	}

	// Direct child spawned into wt and nested grandchild inheriting wt.
	directID, directProc := h.agentRunning(lead, AgentSpawnRequest{
		Prompt: "direct work", Tier: "medium", Authority: "workdir-write", Worktree: wt,
	})
	directSpec := h.backend.start(0)
	directToken := directSpec.WSMCP.Env[EnvPiToken]
	if directToken == "" {
		t.Fatal("direct child did not receive capability token")
	}
	directRec, ok := h.runner.recordOf(directID)
	if !ok {
		t.Fatal("missing direct child record")
	}
	grandID, grandProc := h.agentRunning(h.runner.holderCaller(directRec), AgentSpawnRequest{
		Prompt: "nested work", Tier: "small", Authority: "read-only",
	})

	// Daemon-wide query sees both direct and nested children (and ignores an
	// unrelated child running on h.root).
	otherID, otherProc := h.agentRunning(lead, AgentSpawnRequest{
		Prompt: "other work", Tier: "small", Authority: "read-only",
	})
	defer otherProc.reply("done")

	bound := h.runner.ActiveChildrenInWorktree(wt)
	if len(bound) != 2 {
		t.Fatalf("ActiveChildrenInWorktree = %+v, want 2 children", bound)
	}
	addrs := []string{bound[0].Address, bound[1].Address}
	if !slices.Contains(addrs, AgentAddress(directID)) || !slices.Contains(addrs, AgentAddress(grandID)) || slices.Contains(addrs, AgentAddress(otherID)) {
		t.Fatalf("unexpected bound addresses: %v (want %s and %s, not %s)", addrs, AgentAddress(directID), AgentAddress(grandID), AgentAddress(otherID))
	}

	// Token route exposes the same daemon-wide query to a token-bearing worker.
	body, _ := json.Marshal(map[string]string{"worktree": wt})
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:12345"+RoutePrefix+"/"+tokenRoute+"/worktree.children", bytes.NewReader(body))
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+directToken)
	rec := httptest.NewRecorder()
	h.runner.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("token route status = %d: %s", rec.Code, rec.Body.String())
	}
	var tokenAnswer struct {
		Children []BoundWorktreeChild `json:"children"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &tokenAnswer); err != nil {
		t.Fatalf("decode token answer: %v", err)
	}
	if len(tokenAnswer.Children) != 2 {
		t.Fatalf("token route children = %+v, want 2", tokenAnswer.Children)
	}

	// Settled/sleeping (waiting) children are still non-closed and still block.
	// Direct child holds until grandchild reports, then receives grandchild's
	// report as its next turn.
	directProc.reply("waiting on nested")
	grandProc.reply("nested done")
	eventually(t, "grandchild settled", func() bool { return h.state(grandID).Status == StatusIdle })
	directProc.nextSent(t)
	directProc.reply("all done")
	eventually(t, "direct child settled", func() bool { return h.state(directID).Status == StatusIdle })
	if _, err := h.runner.Stop(grandID); err != nil {
		t.Fatal(err)
	}
	eventually(t, "grandchild waiting", func() bool { return AgentStatus(h.state(grandID)) == AgentWaiting })
	if got := h.runner.ActiveChildrenInWorktree(wt); len(got) != 2 {
		t.Fatalf("waiting non-closed grandchild dropped from bound children: %+v", got)
	}

	// Closing the grandchild leaves only the direct child; closing both clears it.
	if _, err := h.runner.CloseConversation(grandID); err != nil {
		t.Fatal(err)
	}
	if got := h.runner.ActiveChildrenInWorktree(wt); len(got) != 1 || got[0].Address != AgentAddress(directID) {
		t.Fatalf("after closing grandchild = %+v, want only %s", got, AgentAddress(directID))
	}
	if _, err := h.runner.CloseConversation(directID); err != nil {
		t.Fatal(err)
	}
	if got := h.runner.ActiveChildrenInWorktree(wt); len(got) != 0 {
		t.Fatalf("after closing both children = %+v, want empty", got)
	}

	// Shared spawn reservations allow concurrent spawns while blocking an
	// exclusive release lock until all in-flight spawns release it.
	u1, err := wsgit.ReserveWorktreeSpawn(wt)
	if err != nil {
		t.Fatalf("first spawn reservation: %v", err)
	}
	u2, err := wsgit.ReserveWorktreeSpawn(wt)
	if err != nil {
		u1()
		t.Fatalf("second concurrent spawn reservation: %v", err)
	}
	if locked, err := releaseLock.TryLock(); err != nil || locked {
		if locked {
			_ = releaseLock.Unlock()
		}
		u2()
		u1()
		t.Fatalf("exclusive release lock succeeded while spawn reservations held (locked=%v err=%v)", locked, err)
	}
	u1()
	u2()
	if locked, err := releaseLock.TryLock(); err != nil || !locked {
		t.Fatalf("exclusive release lock failed after spawn reservations released (locked=%v err=%v)", locked, err)
	}
	_ = releaseLock.Unlock()
}
