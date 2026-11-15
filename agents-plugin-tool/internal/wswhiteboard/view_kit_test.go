package wswhiteboard

import (
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

// The clock and layout are controlled independently: scroll events are delivered
// explicitly, as in browsers where a synchronous write reports asynchronously.
const viewKitPrelude = `
var bad = [];
function check(what, got, want) { if (JSON.stringify(got) !== JSON.stringify(want)) bad.push(what + ": got " + JSON.stringify(got) + ", want " + JSON.stringify(want)); }
var now = 0, timers = [], tid = 0;
function setTimeout(fn, ms) { var id = ++tid; timers.push({ id: id, at: now + (ms || 0), fn: fn }); return id; }
function clearTimeout(id) { timers = timers.filter(function (t) { return t.id !== id; }); }
function advance(ms) {
  var end = now + ms;
  for (;;) {
    timers.sort(function (a, b) { return a.at - b.at || a.id - b.id; });
    var t = timers[0]; if (!t || t.at > end) break;
    timers.shift(); now = t.at; t.fn();
  }
  now = end;
}
var all = [], writes = 0;
function El(name) {
  all.push(this); this.name = name; this.id = name; this.children = []; this.parentElement = null; this.style = {};
  this.oy = 'visible'; this.ty = 0; this.y = 0; this.top = 0; this.height = 40; this._scroll = 0;
  this.scrollHeight = 0; this.clientHeight = 0; this.clientWidth = 500; this.clientTop = 0; this.connected = true; this.on = {}; this.attrs = {};
}
Object.defineProperty(El.prototype, 'isConnected', { get: function () { return this.connected; } });
Object.defineProperty(El.prototype, 'scrollTop', { get: function () { return this._scroll; }, set: function (v) { writes++; this._scroll = v; } });
El.prototype.add = function (n) { n.parentElement = this; this.children.push(n); return n; };
El.prototype.contains = function (n) { for (; n; n = n.parentElement) if (n === this) return true; return false; };
El.prototype.closest = function (sel) { for (var n = this; n; n = n.parentElement) if (sel === '.topic' && n.isTopic || sel === '.entry-panel' && n.panel) return n; return null; };
El.prototype.matches = function (sel) { return sel === '.doc-frame' && !!this.frame; };
El.prototype.getAttribute = function (name) { return this.attrs[name] || null; };
El.prototype.querySelectorAll = function (sel) { var root = this; return all.filter(function (n) { return root !== n && root.contains(n) && n.connected && (sel.includes('.tophead') ? n.sticky : sel === '.doc-frame[data-doc-key]' ? n.frame : n.candidate); }); };
El.prototype.addEventListener = function (t, f) { (this.on[t] = this.on[t] || []).push(f); };
El.prototype.removeEventListener = function (t, f) { var l = this.on[t] || [], i = l.indexOf(f); if (i >= 0) l.splice(i, 1); };
El.prototype.fire = function (t, e) { (this.on[t] || []).slice().forEach(function (f) { f(e || {}); }); };
El.prototype.getBoundingClientRect = function () {
  var y = this.top, h = this.clientHeight || this.height;
  if (this.sc) { y = (this.sc === html ? 0 : this.sc.top) + this.y - this.sc.scrollTop; h = this.height;
    for (var p = this; p && p !== this.sc; p = p.parentElement) y += p.ty;
  }
  return { top: y, bottom: y + h, height: h, left: 0, right: 500, width: 500 };
};
var html = new El('html'), body = html.add(new El('body'));
var document = { documentElement: html, body: body, scrollingElement: html,
  querySelectorAll: function (sel) { return html.querySelectorAll(sel); },
  getElementById: function (id) { return all.find(function (n) { return n.id === id && n.connected; }) || null; }
};
var winOn = {}, ios = [], ros = [];
function IO(cb, opt) { this.cb = cb; this.opt = opt; this.obs = []; this.log = []; ios.push(this); }
IO.prototype.observe = function (n) { this.obs.push(n); this.log.push('observe:' + n.name); };
IO.prototype.unobserve = function (n) { this.obs = this.obs.filter(function (x) { return x !== n; }); this.log.push('unobserve:' + n.name); };
IO.prototype.report = function (n, ratio) { this.cb([{ target: n, intersectionRatio: ratio, boundingClientRect: n.getBoundingClientRect() }]); };
function RO(cb) { this.cb = cb; this.obs = []; this.off = false; ros.push(this); }
RO.prototype.observe = function (n) { this.obs.push(n.name); };
RO.prototype.disconnect = function () { this.off = true; };
RO.prototype.fire = function () { if (!this.off) this.cb([]); };
var win = {
  document: document, innerHeight: 800, performance: { now: function () { return now; } },
  IntersectionObserver: IO, ResizeObserver: RO, setTimeout: setTimeout, clearTimeout: clearTimeout,
  requestAnimationFrame: function (fn) { return setTimeout(function () { fn(now); }, 16); }, cancelAnimationFrame: clearTimeout,
  getComputedStyle: function (el) { return { overflowY: el.oy, scrollPaddingTop: el.padding || '0', position: el.sticky ? 'sticky' : 'static', top: '0', transform: el.ty ? 'matrix(1,0,0,1,0,' + el.ty + ')' : 'none' }; },
  addEventListener: function (t, f) { (winOn[t] = winOn[t] || []).push(f); },
  removeEventListener: function (t, f) { var l = winOn[t] || [], i = l.indexOf(f); if (i >= 0) l.splice(i, 1); }
};
function winFire(t, e) { (winOn[t] || []).slice().forEach(function (f) { f(e || {}); }); }
var bcol = body.add(new El('bcol')); bcol.oy = 'auto'; bcol.scrollHeight = 4000; bcol.clientHeight = 500; bcol.top = 50;
var board = bcol.add(new El('board'));
var bpins = body.add(new El('bpins')); bpins.oy = 'auto'; bpins.scrollHeight = 2000; bpins.clientHeight = 400; bpins.top = 60;
var plain = body.add(new El('plain'));
html.scrollHeight = 5000; html.clientHeight = 800;
function topic(parent, name, sc, y) { var n = parent.add(new El(name)); n.sc = sc; n.y = y; n.candidate = true; return n; }
`

func runViewKit(t *testing.T, cases string) {
	t.Helper()
	data, err := embeddedAsset("stream.js")
	if err != nil {
		t.Fatal(err)
	}
	kit := regexp.MustCompile(`(?s)\n  function viewKit\(win\) \{.*?\n  \}\n`).FindString(string(data))
	if kit == "" {
		t.Fatal("stream.js holds no viewKit")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	out, err := nodeScript(node, viewKitPrelude+kit+cases+`process.stdout.write(bad.join("\n"));`).CombinedOutput()
	if err != nil {
		t.Fatalf("node: %v\n%s", err, out)
	}
	if s := strings.TrimSpace(string(out)); s != "" {
		t.Errorf("view kit cases failed:\n%s", s)
	}
}

func TestViewKitUnderNode(t *testing.T) {
	runViewKit(t, `
var k = viewKit(win), a = topic(board, 'a', bcol, 70), b = topic(board, 'b', bcol, 150);
var p = topic(bpins, 'p', bpins, 100), d = topic(plain, 'd', html, 100);
[[a,bcol], [p,bpins], [d,html]].forEach(function (pair) {
  var h = k.keep(pair[0]); check('actual scroller ' + pair[0].name, h.scroller.name, pair[1].name); h.end();
});
check('owned native anchoring remains disabled', [bcol.style.overflowAnchor, bpins.style.overflowAnchor, html.style.overflowAnchor], ['none','none','none']);
// A sliver of the previous panel must not compete with a panel at the reading line.
a.y = -39; b.y = 20; var h = k.keep(a); b.y += 60; a.y += 60; writes = 0; h.apply(); h.apply();
check('gap selects next panel, one instant write', [bcol.scrollTop, writes, k.keeping(bcol)], [60,1,null]);
// Sticky clearance picks the inner panel below the head, not the earlier sliver.
bcol.scrollTop = 0; var head = topic(board, 'head', bcol, 0); head.candidate = false; head.sticky = true; head.height = 30;
a.y = 0; a.height = 29; b.y = 30; h = k.keep(a); b.y += 80; h.apply();
check('reading line below sticky head', bcol.scrollTop, 80); head.sticky = false;
// Each transaction captures afresh. Nested known mutations correct only at the outer boundary.
bcol.scrollTop = 0; a.y = 10; a.height = 80; b.y = 120;
h = k.keep(b); var nested = k.keep(a); a.y += 25; b.y += 25; writes = 0; nested.apply();
check('nested mutation has no early writer', writes, 0); h.apply();
check('outer mutation writes once', [bcol.scrollTop,writes], [25,1]);
var before = bcol.scrollTop; a.y += 50;
check('no ordinary resize session/observer', [bcol.scrollTop, ros.length], [before,0]);
// Same identity after morph/reparent, but never substitute another reference.
bcol.scrollTop = 0; a.y = 10; h = k.keep(a); a.connected = false;
var replacement = topic(board, 'new-a', bcol, 70); replacement.id = 'a'; h.apply();
check('same ID replacement resolves', bcol.scrollTop,60);
bcol.scrollTop = 0; replacement.y = 10; h = k.keep(replacement); replacement.connected = false; b.y = 80; writes=0; h.apply();
check('removed reference has no fallback writer', writes,0); a.connected=true; a.y=10;
// Visibility includes exactly one pixel, clips history, and uses the pinned root separately.
var inspect = k.keep(a,{capture:false}); a.y=-39; a.height=40;
check('one exposed pixel',inspect.visible(a),true); a.y=-40; check('fully out',inspect.visible(a),false);
var clip=board.add(new El('clip')); clip.sc=bcol; clip.y=10; clip.height=20; clip.oy='hidden';
var inside=topic(clip,'inside',bcol,35); check('history clip excludes hidden rows',inspect.visible(inside),false);
p.y=10; h=k.keep(p); p.y+=40; writes=0; h.apply();
check('independent pinned correction',[bpins.scrollTop,bcol.scrollTop,writes],[40,0,1]);
// Scroll intent cancels the capture before mutation. Momentum extends the 200 ms cooldown.
a.y=10; h=k.keep(a); bcol.fire('wheel'); a.y+=100; writes=0; h.apply();
check('intent cancels before a scroll',writes,0);
advance(150); bcol.scrollTop=20; bcol.fire('scroll'); advance(199); h=k.keep(a); a.y+=20; writes=0; h.apply();
check('momentum extends cooldown',writes,0); advance(1); a.y=30; h=k.keep(a); a.y+=30; writes=0; h.apply();
check('capture resumes after 200ms',[bcol.scrollTop,writes],[50,1]); bcol.fire('scroll');
h=k.keep(a); a.y+=10; writes=0; h.apply(); check('manager write does not start cooldown',writes,1);
['touchstart','touchmove','pointerdown'].forEach(function (type) {
  advance(201); h=k.keep(a); bcol.fire(type); a.y+=10; writes=0; h.apply(); check(type+' cancels',writes,0);
});
advance(201); h=k.keep(a); winFire('keydown',{key:'PageDown',target:a}); a.y+=10; writes=0; h.apply(); check('scroll key cancels',writes,0);
// Visibility source still retains independent subscriptions and bounded sweep.
var outs=[], w1=k.watch(a,{onOut:function(){outs.push(1);}}), io=ios[0];
io.report(a,0); io.report(a,1); var w2=k.watch(a,{onOut:function(){outs.push(2);}}); io.report(a,1); io.report(a,0);
check('shared visibility subscriptions',outs,[1,2]); w1.unwatch(); a.connected=false; k.sweep();
check('sweep removes disconnected subscriptions',io.obs.length,0);
`)
}

func TestViewKitVisibilityUnderNode(t *testing.T) {
	runViewKit(t, `
var k=viewKit(win), a=topic(board,'a',bcol,100), reports=[], outs=[];
var w1=k.watch(a,{onChange:function(r){reports.push(r.ratio);},onOut:function(){outs.push(1);}}),io=ios[0];
check('root/threshold/margin',[io.opt.root,io.opt.threshold,io.opt.rootMargin],[null,[0],'0px']);
io.report(a,0);check('initial zero is not out',outs,[]);
var w2=k.watch(a,{onOut:function(){outs.push(2);}});
check('new subscription reobserves',io.log.slice(-3),['observe:a','unobserve:a','observe:a']);
io.report(a,.5);io.report(a,0);io.report(a,0);
check('each subscriber exits once',outs,[1,2]);check('seen report',[k.seen(a).ratio,k.seen(a).visible],[0,false]);
k.unwatch(w1);io.report(a,1);check('unwatched hears no report',reports.length,4);
check('other subscriber retains observation',io.obs.includes(a),true);w2.unwatch();check('last subscriber unobserves',io.obs.length,0);
var x=topic(board,'x',bcol,100),heard=[],wy;
k.watch(x,{onChange:function(){heard.push(1);wy.unwatch();}});wy=k.watch(x,{onChange:function(){heard.push(2);}});io.report(x,1);
check('callback can remove another subscriber',heard,[1]);
var gone=topic(board,'gone',bcol,200),wide=0;
k.watch(gone);k.watch(gone,{margin:'200px',onChange:function(){wide++;}});
check('one observer per margin',[ios.length,ios[1].opt.rootMargin],[2,'200px']);
gone.connected=false;k.sweep();ios[1].report(gone,1);check('sweep drops all subscriptions',[ios[1].obs.length,wide],[0,0]);
check('no scroll-margin support means no bands',k.bands,false);
function SIO(cb,opt){IO.call(this,cb,opt);}SIO.prototype=Object.create(IO.prototype);SIO.prototype.scrollMargin='';
win.IntersectionObserver=SIO;var kb=viewKit(win);kb.watch(a,{margin:'100% 0px'});var opt=ios.at(-1).opt;
check('supported scroll margins reach inner clips',[kb.bands,opt.rootMargin,opt.scrollMargin],[true,'100% 0px','100% 0px']);
`)
}

func TestViewKitJumpFollowUnderNode(t *testing.T) {
	runViewKit(t, `
var reduced=false, animate=false, jumps=[], stopped=0;
win.matchMedia=function(){return {matches:reduced};};
El.prototype.scrollIntoView=function(opt){
  jumps.push([this.name,opt.block,opt.behavior]); if(animate && opt.behavior==='smooth')return;
  var s=this.sc, offset=opt.block==='center'?(s.clientHeight-this.height)/2:0;
  s.scrollTop=Math.max(0,Math.min(s.scrollHeight-s.clientHeight,this.getBoundingClientRect().top-(s===html?0:s.top)+s.scrollTop-offset));
};
El.prototype.scrollTo=function(opt){stopped++; this.scrollTop=opt.top;};
var k=viewKit(win), a=topic(board,'a',bcol,1000), b=topic(board,'b',bcol,2200);
var h=k.jump(a,{behavior:'smooth'}), nHooks=(winOn.keydown||[]).length;
check('initial navigation',jumps,[['a','center','smooth']]);
check('inspection remains available during jump',typeof k.keep(b,{capture:false}).visible,'function');
check('ordinary capture cannot replace jump',k.keep(b)===h,true); h.end();
advance(80); a.y+=200; ros[ros.length-1].fire();
check('late layout corrected instantly',[bcol.scrollTop,jumps[jumps.length-1]],[970,['a','center','instant']]);
advance(149); check('no settlement before150ms',!!k.keeping(bcol),true); advance(100);
check('settled jump cleans async work',[k.keeping(bcol),timers.length,bcol.style.overflowAnchor],[null,0,'none']);
// No stale pre-jump restoration after settle.
a.y+=40; advance(250); check('no follow after settlement',bcol.scrollTop,970);
// Initial smooth motion alone is not a layout mutation.
animate=true; jumps=[]; k.jump(a,{behavior:'smooth'});
for(var i=0;i<8;i++){bcol.scrollTop-=20;bcol.fire('scroll');advance(16);}
check('smooth motion is not user intent',[jumps.length,!!k.keeping(bcol)],[1,true]);
a.y+=60;advance(16);check('smooth layout corrected instantly',jumps[jumps.length-1][2],'instant');advance(250);animate=false;
// Mutations and ordinary keeps do not renew the hard cap.
k.jump(a);for(var i=0;i<24;i++){a.y+=10;k.keep(b).apply();advance(80);}
advance(79);check('before cap',!!k.keeping(bcol),true);advance(1);check('non-renewing2s cap',[k.keeping(bcol),timers.length],[null,0]);
['wheel','touchstart','touchmove','pointerdown'].forEach(function(type){
  k.jump(a,{behavior:'smooth'});bcol.fire(type);var before=bcol.scrollTop;a.y+=100;advance(250);
  check(type+' immediately cancels',[k.keeping(bcol),bcol.scrollTop,timers.length],[null,before,0]);
});
['ArrowUp','ArrowDown','ArrowLeft','ArrowRight','PageUp','PageDown','Home','End',' ','Spacebar'].forEach(function(key){
  k.jump(a);winFire('keydown',{key:'x',target:a});check('non-scroll key',!!k.keeping(bcol),true);
  winFire('keydown',{key:key,target:a});check(key+' cancels',[k.keeping(bcol),timers.length],[null,0]);
});
check('smooth navigation stopped on intent',stopped,14);
reduced=true;k.jump(a,{behavior:'smooth'});check('reduced motion',jumps[jumps.length-1][2],'instant');bcol.fire('wheel');reduced=false;
a.y=10;k.jump(a);advance(250);check('clamped edge settles',[bcol.scrollTop,k.keeping(bcol)],[0,null]);
a.y=1000;k.jump(a);advance(80);a.connected=false;
var replacement=topic(board,'replacement',bcol,1300);replacement.id='a';advance(16);
check('replacement target follows',bcol.scrollTop,1070);replacement.connected=false;advance(16);
check('removed jump target ends',[k.keeping(bcol),timers.length],[null,0]);
var d=topic(plain,'document',html,900),p=topic(bpins,'pin',bpins,700);
k.jump(d);d.y+=100;advance(16);check('document correction',html.scrollTop,620);winFire('touchstart');
k.jump(p,{block:'start'});p.y+=80;advance(16);check('pinned correction',bpins.scrollTop,780);bpins.fire('pointerdown');
check('jumps do not add permanent input hooks',(winOn.keydown||[]).length,nHooks+2);
`)
}

func TestViewKitHomeReleaseUnderNode(t *testing.T) {
	runViewKit(t, `
var reduced=false, lands=[];
win.matchMedia=function(){return {matches:reduced};};
El.prototype.scrollTo=function(opt){lands.push({top:opt.top,behavior:opt.behavior});this.scrollTop=opt.top;};
var k=viewKit(win), a=topic(board,'home',bcol,100), b=topic(board,'reading',bcol,400);
bcol.scrollTop=200;
var capture=k.keep(b), pin=k.pin(b), prepared=0;
writes=0;
k.jump(a,{top:0,behavior:'smooth',prepare:function(){
  prepared++;
  a.y+=300;b.y+=300;
  capture.apply();pin.apply();k.keep(b).apply();
  check('release morph cannot write a keep/pin correction',writes,0);
}});
check('one owner releases then navigates',[prepared,writes,lands],[1,1,[{top:0,behavior:'smooth'}]]);
a.y+=800;b.y+=800;ros.forEach(function(r){r.fire();});advance(48);
check('absolute home ignores layout during navigation',[bcol.scrollTop,writes],[0,1]);
advance(400);check('home navigation settles',k.keeping(bcol),null);
// Replacing an already-running explicit follow must end its observer before
// preparing the release; the old destination cannot write during the morph.
k.jump(b,{offset:0});
var oldRO=ros[ros.length-1];writes=0;lands=[];reduced=true;
k.jump(a,{top:0,behavior:'smooth',prepare:function(){b.y+=99;k.keep(b).apply();}});
oldRO.fire();advance(48);
check('replacement/reduced home has one instant writer',[oldRO.off,writes,lands],[true,1,[{top:0,behavior:'instant'}]]);
advance(400);
var d=topic(plain,'document-home',html,200);html.scrollTop=500;lands=[];
k.jump(d,{top:0,behavior:'smooth'});d.y+=100;advance(48);
check('document root uses the same absolute home owner',[html.scrollTop,lands],[0,[{top:0,behavior:'instant'}]]);
`)
}

// Overview's enter, pick and exit place a card at an offset right away. The
// click that asked for it already started the user-scroll cooldown, so place
// must write through it (keep would only record a capture), hold the card
// while late layout above it settles, and yield to the user's next scroll.
func TestViewKitPlaceUnderNode(t *testing.T) {
	runViewKit(t, `
win.matchMedia = function () { return { matches: false }; };
El.prototype.scrollTo = function (opt) { this.scrollTop = opt.top; };
var k = viewKit(win), a = topic(board, 'a', bcol, 900), b = topic(board, 'b', bcol, 2400);
k.keep(a, { capture: false });
bcol.fire('pointerdown');
k.place(b, { offset: 30 });
check('place writes through the click cooldown', bcol.scrollTop, 2400 - 30);
bcol.fire('scroll'); b.y += 176; advance(48);
check('late layout above is followed', bcol.scrollTop, 2576 - 30);
advance(400);
check('it settles and releases the scroller', k.keeping(bcol), null);
var h = k.keep(b); b.y += 40; writes = 0; h.apply();
check('a capture after it settles writes again', writes, 1);
k.place(a, { offset: 0 }); var held = bcol.scrollTop; bcol.fire('wheel'); a.y += 60; advance(48);
check('the user scroll ends the hold', [bcol.scrollTop, k.keeping(bcol)], [held, null]);
`)
}
