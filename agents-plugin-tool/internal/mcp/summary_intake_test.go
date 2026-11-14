package mcp

import (
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wslive"
)

// The fake supplies controlled ownership/faults; the separate real daemon slot
// fixture pins LockOwner against the actual daemon's generation table.
type summaryIntakeHost struct{slot *summaryIntakeSlot}
type summaryIntakeSlot struct{fakeHostedSlot; lease sync.Mutex; owned bool}
func(h summaryIntakeHost)OpenHostedSlot(string)(wsdashboard.HostedSlot,bool){return h.slot,true}
func(s *summaryIntakeSlot)LockOwner()(func(),bool){s.lease.Lock();if !s.owned{s.lease.Unlock();return nil,false};return s.lease.Unlock,true}

func intakeSummaryFixture() *wslive.ReasoningSummary {
	return &wslive.ReasoningSummary{V:1,Producer:"33333333-3333-4333-8333-333333333333",Turn:1,Revision:1,ActiveRequest:&wslive.SummaryPair{Request:1,Attempt:1},Requests:[]wslive.SummaryAttempt{{Request:1,Attempt:1,Provider:"openai",API:"openai-responses",Model:"model",Status:wslive.SummaryStreaming,Parts:[]wslive.SummaryPart{{Text:"exact prefix",ObservedBytes:12,RetainedBytes:12,Status:wslive.SummaryStreaming}}}}}
}

func summaryIntake(t *testing.T,s *Server,key string,ev map[string]any) response {
	t.Helper();args:=map[string]any{"session_key":key,"adapter":"pi","events":[]any{ev}}
	p,_:=json.Marshal(map[string]any{"name":dashboardIntakeTool,"arguments":args})
	op:=s.intake.ingress(request{Method:"tools/call",Params:p});defer op.finish()
	if err:=op.await(s);err!=nil{return toolTextResponse(json.RawMessage("1"),"",err)}
	return s.handleDashboardIntake(json.RawMessage("1"),args)
}

func summaryIntakeText(t *testing.T,r response)string{t.Helper();raw,_:=json.Marshal(r);return toolText(t,string(raw))}

func newSummaryIntakeFixture(t *testing.T)(liveFixture,*summaryIntakeSlot){
	t.Helper();f:=newLiveFixture(t,true)
	t.Setenv(envHarnessPID,strconv.Itoa(os.Getpid()))
	f.server.ownerPID=os.Getpid();f.server.intake=&intakeOrder{};f.server.sessionHarness="pi"
	slot:=&summaryIntakeSlot{owned:true};f.server.live.host=summaryIntakeHost{slot}
	row,err:=wsdashboard.ReadRegistration(f.stateDir,f.replyA);if err!=nil{t.Fatal(err)}
	row.EndedAt="";row.HarnessPID=os.Getpid();row.MCPOwnerPID=os.Getpid();row.Harness="pi"
	if err:=wsdashboard.WriteRegistration(f.stateDir,row);err!=nil{t.Fatal(err)}
	t.Cleanup(f.server.Close)
	return f,slot
}

func TestSummaryIntakeNegotiatesOnlyOwnedDaemonReceiver(t *testing.T){
	f:=newLiveFixture(t,true)
	r:=summaryIntake(t,f.server,f.keyA,map[string]any{"kind":"turn_start"})
	if strings.Contains(summaryIntakeText(t,r),wslive.ReasoningSummaryCapability){t.Fatal("file-only receiver advertised incomplete support")}
	r=summaryIntake(t,f.server,f.keyA,map[string]any{"kind":"agent","reasoningSummary":intakeSummaryFixture()})
	if !responseIsError(r){t.Fatal("file-only receiver accepted optional summary")}
}

func TestSummaryIntakeFreezesRealEndBeforeAcceptanceAndRetriesOnlyCandidate(t *testing.T){
	f,_:=newSummaryIntakeFixture(t)
	start:=summaryIntake(t,f.server,f.keyA,map[string]any{"kind":"turn_start","at":1000})
	if !strings.Contains(summaryIntakeText(t,start),wslive.ReasoningSummaryCapability){t.Fatal("owned receiver did not negotiate")}
	progress:=summaryIntake(t,f.server,f.keyA,map[string]any{"kind":"agent","reasoningSummary":intakeSummaryFixture()})
	if responseIsError(progress){t.Fatal(summaryIntakeText(t,progress))}
	// The actual filesystem fault occurs after live admission but before the
	// natural boundary. Acceptance must freeze in receiver RAM, not drop it.
	ringDir:=wslive.RingDir(f.sessionDir(f.replyA))
	if err:=os.WriteFile(ringDir,[]byte("block ring directory"),0o600);err!=nil{t.Fatal(err)}
	final:=intakeSummaryFixture();final.Revision=2;final.ActiveRequest=nil;final.Requests[0].Status=wslive.SummaryAborted;final.Requests[0].Parts[0].Status=wslive.SummaryAborted
	r:=summaryIntake(t,f.server,f.keyA,map[string]any{"kind":"turn_end","at":2000,"reason":"aborted","reasoningSummary":final})
	if responseIsError(r)||!strings.HasPrefix(summaryIntakeText(t,r),"ok: 1 of 1 events applied"){t.Fatal("real end not accepted into RAM",summaryIntakeText(t,r))}
	store:=wslive.SummaryStore(ringDir)
	if store.Available()==nil{t.Fatal("I/O failure not reflected in admission")}
	if strings.Contains(summaryIntakeText(t,r),wslive.ReasoningSummaryCapability){t.Fatal("storage failure retained acceptance capability")}
	if err:=os.Remove(ringDir);err!=nil{t.Fatal(err)}
	if n,err:=store.Retry(ringDir);err!=nil||n!=1{t.Fatalf("frozen retry: %d %v",n,err)}
	rows:=readLiveRing(t,f.sessionDir(f.replyA));if len(rows)!=1||rows[0].DurationMs!=1000||rows[0].Reason!="aborted"||rows[0].SummaryHistory==nil||rows[0].SummaryHistory.Requests[0].Parts[0].Text!="exact prefix"{t.Fatalf("candidate changed/recounted: %+v",rows)}
	// Losing the ordinary response and retrying cannot recreate a closed turn.
	r=summaryIntake(t,f.server,f.keyA,map[string]any{"kind":"turn_end","at":2000,"reason":"aborted","reasoningSummary":final})
	if !strings.Contains(summaryIntakeText(t,r),"skipped"){t.Fatal("closed source turn reapplied")}
	if rows:=readLiveRing(t,f.sessionDir(f.replyA));len(rows)!=1{t.Fatal("uncertain retry duplicated ring turn")}
}

func TestSummaryIntakeRetiredConnectionAndHostedTakeoverCannotWrite(t *testing.T){
	f,slot:=newSummaryIntakeFixture(t)
	summaryIntake(t,f.server,f.keyA,map[string]any{"kind":"turn_start"})
	summaryIntake(t,f.server,f.keyA,map[string]any{"kind":"agent","reasoningSummary":intakeSummaryFixture()})
	slot.lease.Lock();slot.owned=false;slot.lease.Unlock()
	r:=summaryIntake(t,f.server,f.keyA,map[string]any{"kind":"turn_end","reasoningSummary":intakeSummaryFixture()})
	if !responseIsError(r){t.Fatal("retired slot ended turn")}
	if rows:=readLiveRing(t,f.sessionDir(f.replyA));len(rows)!=0{t.Fatal("retired slot wrote ring")}
	slot.lease.Lock();slot.owned=true;slot.lease.Unlock()
	row,err:=wsdashboard.ReadRegistration(f.stateDir,f.replyA);if err!=nil{t.Fatal(err)}
	row.Hosted=true;row.HostPID=777
	if err:=wsdashboard.WriteRegistration(f.stateDir,row);err!=nil{t.Fatal(err)}
	r=summaryIntake(t,f.server,f.keyA,map[string]any{"kind":"turn_end","reasoningSummary":intakeSummaryFixture()})
	if !strings.Contains(summaryIntakeText(t,r),"ownership changed"){t.Fatal("takeover was not fenced")}
	if rows:=readLiveRing(t,f.sessionDir(f.replyA));len(rows)!=0{t.Fatal("terminal-to-hosted takeover wrote stale candidate")}
}
