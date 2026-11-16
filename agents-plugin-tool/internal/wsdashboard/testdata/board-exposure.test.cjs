// Offline contracts: only shipped JS in node:vm, no daemon or child process.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const { readAssetSource } = require('./asset-source.cjs');
const sharedGo = fs.readFileSync(path.join(__dirname, '../../wswhiteboard/view_kit_test.go'), 'utf8');
const prelude = sharedGo.match(/const viewKitPrelude = `([\s\S]*?)\n`/)[1];
function funcs(source, names) {
  return names.map(name => {
    const found = source.match(new RegExp('\\n  function ' + name + '\\([^\\n]*\\{[^\\n]*\\}\\n|\\n  function ' + name + '\\([\\s\\S]*?\\n  \\}\\n'));
    assert.ok(found, 'missing shipped function ' + name);
    return found[0];
  }).join('');
}
function fixture(stream = readAssetSource('wswhiteboard', 'stream.js')) {
  const c = vm.createContext({ assert, console, Map, Set, WeakSet });
  vm.runInContext(prelude + funcs(stream, ['viewKit']) + '\nvar k = viewKit(win);', c);
  return c;
}
function hiddenContract(stream) {
  const c = fixture(stream);
  vm.runInContext(`
    var hiddenReads = 0;
    var versions = board.add(new El('versions')); versions.tagName='DETAILS'; versions.open=false; versions.sc=bcol; versions.y=10;
    var summary = versions.add(new El('summary')); summary.tagName='SUMMARY';
    summary.sc=bcol; summary.y=10; summary.candidate=true;
    var caption=topic(summary,'summary-caption',bcol,10);
    var inner=versions.add(new El('hidden-content'));
    var hidden=topic(inner,'hidden-version',bcol,10);
    hidden.getBoundingClientRect=function(){hiddenReads++;throw Error('hidden geometry was measured');};
    var inspect=k.keep(summary,{capture:false});
    assert.equal(inspect.visible(hidden),false,'reject closed ancestry before geometry');
    assert.equal(k.snapshot().visible(hidden),false,'snapshot rejects hidden nodes before scroller lookup');
    var oldStyle=win.getComputedStyle;
    win.getComputedStyle=function(n){assert.notEqual(n,hidden);assert.notEqual(n,inner);return oldStyle(n);};
    k.watch(hidden); // Native IO may observe closed content without inspecting its scroller.
    win.getComputedStyle=oldStyle;
    assert.equal(inspect.visible(summary),true,'the closed details own summary still renders');
    assert.equal(inspect.visible(caption),true,'nested content in the own summary still renders');
    assert.equal(inspect.visible(versions),true,'a closed details box itself still renders');
    var second=versions.add(new El('second-summary'));second.tagName='SUMMARY';second.candidate=true;
    second.getBoundingClientRect=hidden.getBoundingClientRect;
    assert.equal(inspect.visible(second),false,'only the first own summary is displayed');
    var nested=inner.add(new El('nested-details'));nested.tagName='DETAILS';nested.open=false;
    var nestedSummary=nested.add(new El('nested-summary'));nestedSummary.tagName='SUMMARY';
    nestedSummary.getBoundingClientRect=hidden.getBoundingClientRect;
    assert.equal(inspect.visible(nestedSummary),false,'an inner summary cannot escape an outer closed content subtree');
    var gone=topic(board,'display-none',bcol,20);gone.checkVisibility=function(){return false;};
    gone.getBoundingClientRect=hidden.getBoundingClientRect;
    assert.equal(inspect.visible(gone),false,'style-only checkVisibility rejects non-rendered content');
    var displayParent=board.add(new El('display-none-parent'));displayParent.display='none';
    var displayChild=topic(displayParent,'display-none-child',bcol,30);displayChild.getBoundingClientRect=hidden.getBoundingClientRect;
    var computed=win.getComputedStyle;
    win.getComputedStyle=function(n){return Object.assign(computed(n),{display:n.display||'block',visibility:n.visibility||'visible',contentVisibility:n.contentVisibility||'visible'});};
    assert.equal(inspect.visible(displayChild),false,'old-engine fallback rejects display:none ancestry');
    var locked=board.add(new El('display-lock'));locked.contentVisibility='hidden';
    var lockChild=topic(locked,'locked-child',bcol,30);lockChild.getBoundingClientRect=hidden.getBoundingClientRect;
    assert.equal(inspect.visible(lockChild),false,'old-engine fallback rejects display-locked ancestry');
    var hiddenInk=board.add(new El('hidden-ink'));hiddenInk.visibility='hidden';
    var visibleInk=topic(hiddenInk,'visible-ink',bcol,30);visibleInk.visibility='visible';
    assert.equal(inspect.visible(visibleInk),true,'visibility:visible descendant can override inherited hidden ink');
    var attrHidden=board.add(new El('hidden-attribute'));attrHidden.hidden=true;
    var child=topic(attrHidden,'hidden-child',bcol,30);child.getBoundingClientRect=hidden.getBoundingClientRect;
    assert.equal(inspect.visible(child),false);
    // This enters the shared reading() path used by frame-height capture.
    var visible=topic(board,'visible',bcol,100);
    var h=k.keep(visible);h.end();
    assert.equal(hiddenReads,0,'reading fallback never probes hidden versions/system runs');
    versions.open=true;second.candidate=false;
    // Hide the throwing nodes explicitly before testing the newly opened path.
    hidden.hidden=true;nestedSummary.hidden=true;
    assert.equal(k.rendered(caption),true);
  `, c);
}
function scrollContract(boardSource = readAssetSource('wsdashboard', 'dashboard.js')) {
  const c = fixture();
  vm.runInContext(`
    var reads=0, checks=0, flushes=0;
    var window={IntersectionObserver:IO,wbHistory:k};
    var doc=document, boardPending={A:true}, boardReleaseFrame=0, boardScrollTimer=0, boardHomeWrites=new WeakSet();
    function $(id){return id==='bcol'?bcol:null;}
    function boardPendingNames(){return [];}
    function boardPendingBadge(){}
    function boardFlash(){}
    function boardReleaseDue(){checks++;return false;}
    function boardFlushViewed(){flushes++;}
    function requestAnimationFrame(fn){return win.requestAnimationFrame(fn);}
    function cancelAnimationFrame(id){win.cancelAnimationFrame(id);}
    var oldRect=El.prototype.getBoundingClientRect;
    El.prototype.getBoundingClientRect=function(){reads++;return oldRect.call(this);};
    for(var i=0;i<1000;i++)topic(board,'unit'+i,bcol,40*i);
    bcol.scrollTop=50;
  ` + funcs(boardSource, ['boardViewedScroll'].concat(boardSource.includes('function boardViewedTrailing') ? ['boardViewedTrailing'] : [])) + `
    // Raw handlers do not scan; a trailing check after 120 ms is allowed.
    for(var i=0;i<100;i++)boardViewedScroll({type:'scroll',target:bcol});
    advance(16);
    assert.equal(reads,0,'1000-entry board: zero geometry reads from raw scroll ticks');
    assert.equal(checks,0,'raw scroll ticks must not recompute exposure');
    assert.equal(flushes,0,'raw scroll ticks must not re-swap the board');
  `, c);
  vm.runInContext(funcs(boardSource, ['boardViewedExposure', 'watchBoard']) + `
    board.nodeType=1;
    function q(sel,n){return n.querySelectorAll(sel);}
    watchBoard(board);
    var unit=all.find(function(n){return n.name==='unit0';});
    var observer=ios[0];
    assert.ok(observer.obs.includes(unit),'the shipped watcher subscribes release units');
    function report(ratio){observer.cb([{target:unit,intersectionRatio:ratio,boundingClientRect:{top:0,bottom:40,height:40,left:0,right:500,width:500}}]);}
    report(1);report(1);report(0);
    assert.equal(checks,0,'observer reports coalesce before exact release checking');
    advance(16);assert.equal(checks,1);assert.equal(flushes,0);
    var previousHandle=unit.wbWatch;
    unit.connected=false;k.sweep();
    assert.equal(unit.wbWatch,undefined,'sweep clears the dead board handle');
    assert.equal(unit.wbViewedExposure,undefined,'fresh subscriptions replay their first exposure report');
    unit.connected=true;watchBoard(board);
    assert.ok(unit.wbWatch && unit.wbWatch!==previousHandle,'materialized rows can be re-watched');
    assert.ok(observer.obs.includes(unit));
    // The old-engine fallback still coalesces scrolls and uses hidden-safe reads.
    window.IntersectionObserver=null;
    for(var i=0;i<100;i++)boardViewedScroll({type:'scroll',target:bcol});
    advance(16);assert.equal(checks,2);
  `, c);
}
function trailingStickyContract() {
  const source = readAssetSource('wsdashboard', 'dashboard.js');
  for (const nativeScrollend of [true, false]) {
    const c = fixture();
    c.nativeScrollend = nativeScrollend;
    vm.runInContext(`
      var window={IntersectionObserver:IO,wbHistory:k};
      var doc=document,boardPending={},boardReleaseFrame=0,boardScrollTimer=0,boardSnap=null;
      var boardHomeWrites=new WeakSet(),checks=0,flushes=0;
      if(nativeScrollend)doc.onscrollend=null;
      function $(id){return id==='bcol'?bcol:id==='bpins'?bpins:null;}
      function inBoard(n){return board.contains(n);}
      function boardPendingNames(){return [];}
      function boardPendingBadge(){}
      function boardFlash(){}
      function boardFlushViewed(){flushes++;boardPending={};}
      function requestAnimationFrame(fn){return win.requestAnimationFrame(fn);}
      function cancelAnimationFrame(id){win.cancelAnimationFrame(id);}
      var head=topic(board,'stuck-head',bcol,0);head.sticky=true;head.candidate=false;
      head.getBoundingClientRect=function(){return {top:50,bottom:130,height:80,left:0,right:500,width:500};};
      var blocker=topic(board,'blocker',bcol,120);
      k.watch(blocker);ios[0].report(blocker,1);
      assert.equal(k.keep(blocker,{capture:false}).visible(blocker),true);
      boardPending={A:[blocker]};
    ` + funcs(source, ['boardExposed', 'boardWithSnap', 'boardReleaseDue', 'boardViewedScroll', 'boardViewedTrailing', 'boardViewedExposure']) + `
      var exactReleaseDue=boardReleaseDue;
      boardReleaseDue=function(){checks++;return exactReleaseDue();};
      bcol.scrollTop=100;
      assert.equal(k.seen(blocker).visible,true,'IO still intersects the raw viewport');
      assert.equal(k.keep(blocker,{capture:false}).visible(blocker),false,'blocker is entirely under the sticky head');
      // No further IO report: one trailing check must release this blocker.
      for(var i=0;i<100;i++)boardViewedScroll({type:'scroll',target:bcol});
      if(nativeScrollend) {
        advance(200);assert.equal(checks,0,'native branch waits for scrollend');
        boardViewedTrailing({type:'scrollend',target:bcol});
      } else {
        assert.equal(timers.length,1,'one debounced trailing check per gesture');
        advance(119);assert.equal(checks,0,'no exposure scan during the gesture');
        advance(1);
      }
      advance(16);
      assert.equal(checks,1,'one exact sticky-boundary check at gesture end');
      assert.equal(flushes,1,'a due release cannot stall awaiting another IO edge');
      advance(200);assert.equal(checks,1);
    `, c);
  }
}
function reviewWiringContract() {
  const source = readAssetSource('wsdashboard', 'dashboard.js');
  const swap = funcs(source, ['swap']);
  assert.ok(swap.indexOf('window.wbHistory.restore(n, history);') < swap.indexOf('watchBoard(n);'), 'watch rows admitted by init/restore');
  const history = source.match(/doc\.addEventListener\("wbhistoryload", function \(e\) \{[\s\S]*?\n  \}\);/)[0];
  assert.ok(history.includes('watchBoard(el);'), 'load-older/materialized rows are watched');
  assert.ok(source.includes('doc.addEventListener("scrollend", boardViewedTrailing, { capture: true, passive: true });'));
  const toggle = source.match(/doc\.addEventListener\("toggle", function \(e\) \{\n    var n = e.target;[\s\S]*?\n  \}, true\);/)[0];
  const c = vm.createContext({ assert });
  vm.runInContext(`
    var toggleHandler,watched=0,releases=0;
    var doc={addEventListener:function(kind,fn){toggleHandler=fn;}};
    function inBoard(n){return !!n.board;}
    function watchBoard(){watched++;}
    function boardViewedExposure(){releases++;}
  ` + toggle + `
    toggleHandler({target:{tagName:'DETAILS',open:true,board:false}});
    assert.equal(watched,0);assert.equal(releases,0,'unrelated tree/drawer disclosures do not release board updates');
    toggleHandler({target:{tagName:'DETAILS',open:true,board:true}});
    assert.equal(watched,1);assert.equal(releases,1);
    var window={wbHistory:{rendered:function(){return false;},keep:function(){throw Error('hidden scroller inspected');}}};
    var boardSnap=null;
  ` + funcs(source, ['boardExposed', 'boardScroller']) + `
    assert.equal(boardExposed({}),false);
    assert.equal(boardScroller({}),null);
  `, c);
}
function readingCount() {
  const c = fixture();
  vm.runInContext(`
    var reads=0, hiddenReads=0, styleReads=0;
    var oldRect=El.prototype.getBoundingClientRect,oldStyle=win.getComputedStyle;
    El.prototype.getBoundingClientRect=function(){reads++;return oldRect.call(this);};
    win.getComputedStyle=function(n){styleReads++;return oldStyle(n);};
    var units=[];
    for(var i=0;i<1000;i++) {
      var parent=board;
      if(i>=950){parent=board.add(new El('closed'+i));parent.tagName='DETAILS';parent.open=false;}
      var n=topic(parent,'entry'+i,bcol,i*40);units.push(n);
      n.matches=function(s){return s.includes('.entry-panel')?this.candidate:false;};
      if(i>=950)n.getBoundingClientRect=function(){hiddenReads++;throw Error('hidden version layout');};
    }
    document.elementFromPoint=function(){return units[0];};
    var window={wbHistory:k};
    function boardBoxes(){return [board];}
    var frameSizes=new Map(),frameSizeFrame=0,themes=0;
    function postTheme(){themes++;}
    units.forEach(function(n){n.style.height='40px';frameSizes.set(n,120);});
    units[0].wbGrow=true;
  ` + funcs(readAssetSource('wsdashboard', 'dashboard.js'), ['boardKeep', 'applyFrameHeights']) + `
    applyFrameHeights();
    assert.equal(themes,1000,'actual frame-height batch applied');
    assert.ok(units.every(function(n){return n.style.height==='120px';}));
    assert.equal(hiddenReads,0);
    assert.ok(reads<=8,'1000-frame capture uses one reading hit-test, not a full scan: '+reads);
    assert.ok(styleReads<8000,'batch shares ancestor/scroller lookup: '+styleReads);
    globalThis.countReceipt={units:1000,closed:50,rawScrollReads:0,readingCaptureReads:reads};
  `, c);
  return JSON.parse(JSON.stringify(c.countReceipt));
}
function existingViewContracts() {
  const cases = [...sharedGo.matchAll(/runViewKit\(t, `([\s\S]*?)`\)/g)].map(m => m[1]);
  assert.ok(cases.length >= 4);
  for (const code of cases) {
    const c = fixture();
    vm.runInContext(code, c);
    assert.deepEqual(JSON.parse(JSON.stringify(c.bad)), [], 'existing shared viewport/keep/watch/pin/jump contract');
  }
}
function existingViewedContracts() {
  const source = readAssetSource('wsdashboard', 'dashboard.js');
  const go = fs.readFileSync(path.join(__dirname, '../board_viewed_test.go'), 'utf8');
  const model = fs.readFileSync(path.join(__dirname, 'viewed-panels.cjs'), 'utf8');
  const names = [...go.match(/return string\(data\) \+ dashboardFuncs\(t, ([^\n]+)\)/)[1].matchAll(/"([^"]+)"/g)].map(m => m[1]);
  const cases = [...go.matchAll(/viewedHarness\(t\)\+`([\s\S]*?)`\)/g)].map(m => m[1]).filter(code => !code.includes('string(data)'));
  assert.ok(cases.length >= 6);
  for (const code of cases) {
    const c = vm.createContext({ process: { stdout: { write() {} } }, console });
    vm.runInContext(model + funcs(source, names) + code, c);
    assert.deepEqual(JSON.parse(JSON.stringify(c.bad)), [], 'existing viewed rendering/order/release/top-origin/owner receipt semantics');
  }
}
function candidatesContract() {
  const source = readAssetSource('wsdashboard', 'dashboard.js');
  const c = vm.createContext({ assert, console });
  vm.runInContext(`
    var window={wbHistory:{rendered:function(n){return !n.hidden;}}};
    var yes={id:'shown',hidden:false,classList:{contains:function(){return false;}}};
    var no={id:'closed-version',hidden:true,classList:yes.classList};
    var box={children:[yes,no]};
    function q(selector){return selector.includes('closed-page')?[]:[yes,no];}
    function boardBoxes(){return [box];}
    function boardExposed(n){assert.notEqual(n,no,'hidden candidates must be removed before exposure');return true;}
  ` + funcs(source, ['boardLivePanels', 'boardExposureUnits']) + `
    assert.equal(boardLivePanels(box).length,1);
    assert.ok(boardExposureUnits().every(function(n){return n===yes;}));
  `, c);
}
function run() {
  new vm.Script(readAssetSource('wsdashboard', 'dashboard.js'));
  new vm.Script(readAssetSource('wswhiteboard', 'stream.js'));
  hiddenContract();
  scrollContract();
  trailingStickyContract();
  reviewWiringContract();
  candidatesContract();
  existingViewContracts();
  existingViewedContracts();
  return readingCount();
}
module.exports = run;
module.exports.hiddenContract = hiddenContract;
module.exports.scrollContract = scrollContract;
if (require.main === module) {
  const test = require('node:test');
  test('hidden geometry, observer-driven release and existing viewed semantics', () => {
    console.log('layout-read counts:', run());
  });
}
