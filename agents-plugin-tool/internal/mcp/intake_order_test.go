package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
)

func orderRequest(kind string, v1 bool) request {
	ev := map[string]any{"kind": kind}
	if v1 {
		ev["reasoningSummary"] = map[string]any{"v": 1}
	}
	p, _ := json.Marshal(map[string]any{"name": dashboardIntakeTool, "arguments": map[string]any{"events": []any{ev}}})
	return request{Method: "tools/call", Params: p}
}

func TestSummaryIngressTracksAbandonedLegacyAndActualDeferredWrite(t *testing.T) {
	o := &intakeOrder{}
	s := &Server{}
	writeDone := make(chan struct{})
	ls := &liveSession{effects: map[uint64]chan struct{}{1: writeDone}, stopped: true}
	s.live.sessions = map[string]*liveSession{"key": ls}
	legacy := o.ingress(orderRequest("reset", false))
	first := o.ingress(orderRequest("agent", true))
	result := make(chan error, 1)
	go func() { result <- first.await(s) }()
	// A local timeout/abandon is not completion of the remote operation.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	<-ctx.Done()
	select {
	case err := <-result:
		t.Fatalf("abandon bypassed legacy handler: %v", err)
	default:
	}
	legacy.finish()
	select {
	case err := <-result:
		t.Fatalf("handler response bypassed deferred write: %v", err)
	default:
	}
	// Only this represents the write callback actually returning.
	close(writeDone)
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("actual quiescence did not admit v1")
	}
	first.finish()
}

func TestSummaryIngressFailsClosedWithinAdmissionBudget(t *testing.T) {
	o := &intakeOrder{}
	s := &Server{}
	legacy := o.ingress(orderRequest("reset", false))
	defer legacy.finish()
	first := o.ingress(orderRequest("agent", true))
	defer first.finish()
	started := time.Now()
	if err := first.await(s); !errors.Is(err, errSummaryAdmission) {
		t.Fatalf("admission error = %v", err)
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("unbounded admission: %v", elapsed)
	}
	later := o.ingress(orderRequest("turn_end", false))
	defer later.finish()
	if err := later.await(s); !errors.Is(err, errSummaryAdmission) {
		t.Fatal("revoked connection admitted later mutation", err)
	}
}

func TestSummaryIngressFIFOAndUnrelatedToolsRemainIndependent(t *testing.T) {
	o := &intakeOrder{}
	s := &Server{}
	first := o.ingress(orderRequest("agent", true))
	if err := first.await(s); err != nil {
		t.Fatal(err)
	}
	queued := o.ingress(orderRequest("reset", false))
	third := o.ingress(orderRequest("turn_start", false))
	if op := o.ingress(request{Method: "ping"}); op != nil {
		t.Fatal("unrelated request queued")
	}
	secondReady := make(chan error, 1)
	thirdReady := make(chan error, 1)
	go func() { secondReady <- queued.await(s) }()
	go func() { thirdReady <- third.await(s) }()
	select {
	case <-secondReady:
		t.Fatal("queued mutation overtook v1")
	default:
	}
	first.finish()
	if err := <-secondReady; err != nil {
		t.Fatal(err)
	}
	select {
	case <-thirdReady:
		t.Fatal("third mutation overtook queued predecessor")
	default:
	}
	queued.finish()
	if err := <-thirdReady; err != nil {
		t.Fatal(err)
	}
	third.finish()
}

func TestSummaryQuiescenceWaitsForRunningSnapshotCallback(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	old := setLiveSeams(func(s *liveSeams) {
		s.writeSnapshot = func(string, []byte) error { once.Do(func() { close(entered) }); <-release; return nil }
		s.readRepo = func(string) wslive.Repo { return wslive.Repo{} }
	})
	t.Cleanup(func() { setLiveSeams(func(s *liveSeams) { *s = old }) })
	ls := &liveSession{state: wslive.NewState("key"), dir: t.TempDir(), active: func() bool { return true }, nudge: func() {}, debug: func(string, map[string]any) {}}
	ls.mu.Lock()
	ls.scheduleLocked(time.Millisecond)
	ls.mu.Unlock()
	<-entered
	done := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		done <- ls.quiesce(ctx)
	}()
	select {
	case err := <-done:
		t.Fatalf("released while callback can still write: %v", err)
	default:
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	ls.halt()
}
