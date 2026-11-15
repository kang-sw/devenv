package wsdashboard

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

func TestWakeReadsBrowser(t *testing.T) {
	if os.Getenv("WS_PLAYWRIGHT_MODULE") == "" {
		t.Skip("set WS_PLAYWRIGHT_MODULE for disposable Chromium acceptance")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal(err)
	}
	f := startPageFixture(t)
	board := &wswhiteboard.State{}
	for i := 0; i < 112; i++ {
		board.Topics = append(board.Topics, wswhiteboard.TopicDoc{Name: fmt.Sprintf("topic-%d", i), Entries: []wswhiteboard.Entry{{ID: fmt.Sprintf("e%d", i+1), Kind: wswhiteboard.EntryAgent, Text: "unchanged fixture entry", At: "2026-11-15T00:00:00Z"}}})
	}
	regions, err := wswhiteboard.RenderRegions(wswhiteboard.BuildView(board, wswhiteboard.ViewInput{Layout: wswhiteboard.LayoutFull}))
	if err != nil {
		t.Fatal(err)
	}
	page := f.src.page("livesession")
	page.Board = regions
	f.src.setPage("livesession", page)
	sd := SessionDir(f.dir, "livesession")
	if err := wslive.AppendObserved(sd, []wslive.ObservedRecord{{AgentID: "child", ModelObservation: wslive.ModelObservation{ID: "response", Model: "first-model", Source: "pi.assistant", At: 1}}}); err != nil {
		t.Fatal(err)
	}
	paths, err := filepath.Glob(filepath.Join(sd, wslive.ObservedDirName, "*.json"))
	if err != nil || len(paths) != 1 {
		t.Fatal("missing evidence fixture")
	}
	f.d.Index().Refresh()
	f.d.Index().Settle()
	var modelUpdates atomic.Int64
	updates := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		marker := r.URL.Query().Get("marker")
		if model := r.URL.Query().Get("model"); model != "" {
			// Evidence is write-once. Publish a new response, ordered across both viewports.
			n := modelUpdates.Add(1) + 1
			record := wslive.ObservedRecord{AgentID: "child", ModelObservation: wslive.ModelObservation{ID: fmt.Sprintf("response-%d", n), Model: model, Source: "pi.assistant", At: n}}
			if err := wslive.AppendObserved(sd, []wslive.ObservedRecord{record}); err != nil {
				t.Error(err)
				w.WriteHeader(500)
				return
			}
		}
		snap := wslive.Snapshot{SchemaVersion: 1, Adapter: "pi", UpdatedAt: time.Now().UnixMilli(), HeartbeatMs: 30000, Lead: wslive.Lead{Activity: wslive.Activity{Phase: wslive.PhaseReplying}, Reply: &wslive.Reply{Text: marker}}, Agents: []wslive.Agent{{ID: "child", Status: wslive.StatusRunning}}}
		f.d.snapshots.SetLive(f.dir, "livesession", snap)
		f.d.broker.wake(liveDep("livesession"))
		w.WriteHeader(http.StatusNoContent)
	}))
	defer updates.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	script := `
 const assert=require('node:assert/strict');
 const {chromium}=require(process.env.WS_PLAYWRIGHT_MODULE);
 (async()=>{const browser=await chromium.launch({headless:true});try{
  for(const width of [1440,420]){
   const context=await browser.newContext({viewport:{width,height:900},locale:'en-US'});
   const tab=await context.newPage();const errors=[];tab.on('pageerror',e=>errors.push(e.message));
   await fetch(process.env.WAKE_UPDATES+'?model=first-model&marker=initial');
   await tab.goto(process.env.WAKE_BASE+'/s/livesession');
   const frameElement=await tab.locator('iframe.sframe.on').elementHandle();
   const frame=await frameElement.contentFrame();
   await frame.locator('#r-board section.topic').first().waitFor({state:'attached'});
   await frame.waitForFunction(()=>document.querySelector('#r-tree .amodel')?.textContent.includes('first-model'));
   assert.equal(await frame.locator('#r-board section.topic').count(),112);
   await frame.evaluate(()=>{window.originalTopic=document.querySelector('#r-board section.topic')});
   for(let n=1;n<=3;n++){
    const marker='live-wake-'+n;
    await fetch(process.env.WAKE_UPDATES+'?marker='+marker);
    await frame.waitForFunction(m=>document.getElementById('r-convlive').textContent.includes(m),marker);
    assert.equal(await frame.evaluate(()=>window.originalTopic===document.querySelector('#r-board section.topic')),true,'unchanged board retained');
   }
   await fetch(process.env.WAKE_UPDATES+'?model=second-model&marker=changed-evidence');
   await frame.waitForFunction(()=>document.querySelector('#r-tree .amodel')?.textContent.includes('second-model'));
   assert.equal(await frame.evaluate(()=>window.originalTopic===document.querySelector('#r-board section.topic')),true,'evidence update retained board');
   assert.deepEqual(errors,[]);
   console.log(width+': 112 topics, live wakes and changed child evidence passed');
   await context.close();
  }
 }finally{await browser.close()}})().catch(e=>{console.error(e);process.exit(1)});
 `
	cmd := exec.CommandContext(ctx, node, "-e", script)
	cmd.Env = append(os.Environ(), "WAKE_BASE="+f.base, "WAKE_UPDATES="+updates.URL)
	out, err := cmd.CombinedOutput()
	t.Log(string(out))
	if err != nil {
		t.Fatalf("Chromium wake read acceptance: %v", err)
	}
}
