package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"
)

// intakeOrder belongs to one opened MCP connection. Legacy handlers retain
// their concurrency, but every canonical operation is registered before its
// goroutine starts. Cancellation never closes an unfinished operation.
type intakeOrder struct {
	mu sync.Mutex
	legacy []<-chan struct{}
	tail <-chan struct{}
	ordered bool
	revoked bool
}

const summaryAdmissionBudget = 750 * time.Millisecond

var errSummaryAdmission = errors.New("reasoning summary admission unavailable: earlier intake effects have not quiesced")

type intakeOperation struct {
	order *intakeOrder
	done chan struct{}
	before []<-chan struct{}
	first, ordered bool
}

func canonicalIntake(req request) (canonical, v1 bool) {
	if req.Method!="tools/call" { return }
	var p struct { Name string `json:"name"`; Arguments struct { Events []struct { ReasoningSummary json.RawMessage `json:"reasoningSummary"` } `json:"events"` } `json:"arguments"` }
	if json.Unmarshal(req.Params,&p)!=nil || p.Name!=dashboardIntakeTool { return }
	canonical=true
	for _,ev:=range p.Arguments.Events {
		var s struct { V int `json:"v"` }
		if json.Unmarshal(ev.ReasoningSummary,&s)==nil && s.V==1 { v1=true; break }
	}
	return
}

func (o *intakeOrder) ingress(req request) *intakeOperation {
	canonical,v1:=canonicalIntake(req)
	if !canonical { return nil }
	o.mu.Lock(); defer o.mu.Unlock()
	// Completed legacy operations have no handler effects left; their deferred
	// writes remain tracked by liveSession until the first admission barrier.
	live:=o.legacy[:0]
	for _,done:=range o.legacy { select { case <-done: default: live=append(live,done) } }
	o.legacy=live
	op:=&intakeOperation{order:o,done:make(chan struct{})}
	if !o.ordered && v1 {
		o.ordered=true; op.first=true
		op.before=append(op.before,o.legacy...)
		o.legacy=nil
	} else if o.ordered && o.tail!=nil { op.before=append(op.before,o.tail) }
	op.ordered=o.ordered
	if op.ordered { o.tail=op.done } else { o.legacy=append(o.legacy,op.done) }
	return op
}

func (op *intakeOperation) finish() { if op!=nil { close(op.done) } }

func (op *intakeOperation) await(s *Server) error {
	if op==nil || !op.ordered { return nil }
	o:=op.order
	o.mu.Lock(); revoked:=o.revoked; o.mu.Unlock()
	if revoked { return errSummaryAdmission }
	// A queued cancellation may skip mutation only after its predecessor is
	// finished. Otherwise a later operation could overtake that predecessor.
	ctx:=context.Background()
	var cancel context.CancelFunc
	if op.first { ctx,cancel=context.WithTimeout(ctx,summaryAdmissionBudget); defer cancel() }
	for _,done:=range op.before {
		select { case <-done: case <-ctx.Done(): o.revoke(); return errSummaryAdmission }
	}
	if op.first {
		if err:=s.live.quiesce(ctx); err!=nil { o.revoke(); return errSummaryAdmission }
	}
	return nil
}

func (o *intakeOrder) enabled() bool { o.mu.Lock(); defer o.mu.Unlock(); return o.ordered }

func (o *intakeOrder) revoke() { o.mu.Lock(); o.revoked=true; o.mu.Unlock() }

// quiesce waits for the actual scheduled callbacks, not just handler replies.
// A timeout leaves their completion channels live; it never fakes completion.
func (h *liveHub) quiesce(ctx context.Context) error {
	h.mu.Lock()
	list:=make([]*liveSession,0,len(h.sessions))
	for _,ls:=range h.sessions { list=append(list,ls) }
	h.mu.Unlock()
	for _,ls:=range list {
		if err:=ls.quiesce(ctx); err!=nil { return err }
	}
	return nil
}

func (ls *liveSession) quiesce(ctx context.Context) error {
	ls.mu.Lock()
	ls.quiescing=true
	if ls.timer!=nil && ls.timer.Stop() {
		if done,ok:=ls.effects[ls.timerEffect]; ok { close(done); delete(ls.effects,ls.timerEffect) }
	}
	ls.due=time.Time{}
	pending:=make([]<-chan struct{},0,len(ls.effects))
	for _,done:=range ls.effects { pending=append(pending,done) }
	ls.mu.Unlock()
	defer func() { ls.mu.Lock(); ls.quiescing=false; if !ls.stopped { ls.scheduleLocked(currentLiveSeams().heartbeat) }; ls.mu.Unlock() }()
	for _,done:=range pending { select { case <-done: case <-ctx.Done(): return ctx.Err() } }
	return nil
}
