package wsdashboard

import (
	"os/exec"
	"testing"
)

func TestClosedHistoryRetriesAcrossImageStampChangesUnderNode(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	asset, err := dashboardTestAsset("assets/dashboard.js")
	if err != nil {
		t.Fatal(err)
	}
	script := whiteboardBridgeFunctions(t, string(asset), "boardImageStamp", "loadClosed") + `
var stamp='one', requests=[], installed=[], rid='r', boardPaged={}, done=false;
process.on('beforeExit',function(){if(!done){console.error('async closed-history test did not complete');process.exitCode=1;}});
var window={fetch:true}, doc={querySelector:function(){return {getAttribute:function(){return stamp;}};}};
function fetch(href){return new Promise(resolve=>requests.push(resolve));}
function frag(html){return {firstElementChild:{version:html}};}
function q(){return [];}
function boardKeep(){return null;}
function watchBoard(){} function bandBoard(){} function restoreOpen(){} function bindCapsuleNotes(){} function mermaid(){}
var btn={disabled:false,getAttribute:function(){return '30';},parentNode:{replaceChild:function(page){installed.push(page.version);}}};
function boardTopicPresent(){return false;}
async function drain(){for(var i=0;i<12;i++)await Promise.resolve();}
(async function(){
  var pending=loadClosed(btn);stamp='two';
  requests[0]({ok:true,json:function(){return Promise.resolve({html:'old-image'});}});await drain();
  if(installed.length || requests.length!==2)throw Error('stale closed page was installed rather than refetched');
  requests[1]({ok:true,json:function(){return Promise.resolve({html:'new-image'});}});
  if(await pending!==true || installed.join(',')!=='new-image')throw Error('refetched closed page did not land');
  done=true;
})().catch(e=>{console.error(e);process.exitCode=1;});`
	if out, err := nodeScript(node, script).CombinedOutput(); err != nil {
		t.Fatalf("closed image stamp: %v\n%s", err, out)
	}
}
