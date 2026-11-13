package wswhiteboard

import (
	"os/exec"
	"strings"
	"testing"
)

func TestHistoryScriptHeightRevealAndSwapUnderNode(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	asset, err := embeddedAsset("stream.js")
	if err != nil {
		t.Fatal(err)
	}
	// Exercise stream policy with deterministic layout and a recording shared kit.
	// The real kit's geometry, visibility transitions, and timers have their own tests.
	asset = []byte(strings.Replace(string(asset), "var kit = viewKit(window);", "var kit = window.historyKit;", 1))
	// Keep the history DOM and bubbling model local to this stream test.
	script := `var BOARD = '', BODY_CLASS = '';` + formPrelude + `
var originalOne=one;
one=function(e,s){ return originalOne(e,s.replace(/#([\w-]+)/g,'[id="$1"]')); };
Object.defineProperty(E.prototype, 'id', { get: function () { return this.attrs.id || ''; } });
Object.defineProperty(E.prototype, 'tagName', { get: function () { return this.tag.toUpperCase(); } });
Object.defineProperty(E.prototype, 'parentElement', { get: function () { return this.parent; } });
Object.defineProperty(E.prototype, 'firstChild', { get: function () { return this.children[0] || null; } });
Object.defineProperty(E.prototype, 'nextElementSibling', { get: function () { return this.parent && this.parent.children[this.parent.children.indexOf(this)+1] || null; } });
Object.defineProperty(E.prototype, 'open', { get: function () { return 'open' in this.attrs; }, set: function (v) { if (v) this.attrs.open = ''; else delete this.attrs.open; } });
E.prototype.matches = function (s) { return one(this, s); };
E.prototype.remove = function () { this.parent.children.splice(this.parent.children.indexOf(this), 1); this.parent = null; };
E.prototype.cloneNode = function () { var n = new E(this.tag, Object.assign({}, this.attrs)); layout(n); this.children.forEach(function (c) { var child = c.cloneNode(true); child.parent = n; n.children.push(child); }); if (this.content) n.content = this.content.cloneNode(true); return n; };
E.prototype.insertBefore = function (node, before) { var list = node.tag === '#fragment' ? node.children.slice() : [node], self = this; list.forEach(function(c) { if(c.parent) c.remove(); }); var i = before ? this.children.indexOf(before) : this.children.length; this.children.splice.apply(this.children, [i, 0].concat(list)); list.forEach(function (c) { c.parent = self; }); };
Object.defineProperty(E.prototype, 'isConnected', { get: function () { for(var p=this;p;p=p.parent) if(p===root) return true; return false; } });
E.prototype.contains = function (n) { for (;n;n=n.parent) if(n===this) return true; return false; };
E.prototype.matches = function (s) { var self=this; return s.split(',').some(function(p) { return one(self,p.trim()); }); };
Object.defineProperty(E.prototype, 'scrollHeight', { get: function () { return this.cls().includes('history-live') ? (this._liveHeight === undefined ? this.querySelectorAll('.wb-entry').length * 20 : this._liveHeight) : (this._height || 20); }, set: function (n) { this._height = n; } });
Object.defineProperty(E.prototype, 'scrollTop', { get: function () { return this._top || 0; }, set: function (n) { this._top = Math.max(0, Math.min(n, Math.max(0, this.scrollHeight - (this.clientHeight || 0)))); } });
E.prototype.getBoundingClientRect = function () {
  var h = this.closest('.wb-history');
  if (!h) return { top: 0, bottom: 100, height: 100 };
  var live = h.querySelector('.history-live'), height = Math.min(live.scrollHeight, (h._wbHistorySteps || 1)*640);
  if (this === h || this.cls().includes('history-clip')) return { top: 0, bottom: height, height: height };
  if (this === live) return { top: height-live.scrollHeight, bottom: height, height: live.scrollHeight };
  var article = this.closest('.wb-entry'), index = live.querySelectorAll('.wb-entry').indexOf(article);
  var top = height-live.scrollHeight + index * 20;
  return { top: top, bottom: top + 20, height: index >= 0 ? 20 : 0 };
};
var loads = 0, keeps = [], timers = [], motions = [];
E.prototype.getAnimations = function () { return motions; };
document.getAnimations = function () { return motions; };
window.setTimeout = function (fn) { timers.push(fn); };
document.addEventListener = function (type, fn) { (document.on[type] || (document.on[type]=[])).push(fn); };
document.on = {};
window.historyKit = {
  watch: function (node, opt) { return {node:node, opt:opt, ratio:undefined}; },
  unwatch: function (w) { if(w) w.gone=true; },
  keep: function (node, opt) { var k={node:node,opt:opt,applied:0,apply:function(){this.applied++;}}; keeps.push(k); return k; }
};
function visible(h, ratio) { var w=h._wbHistoryWatch, was=w.ratio; w.ratio=ratio; w.opt.onChange({visible:ratio>0}); if(was>0 && ratio===0) w.opt.onOut(); }
function resize(h) { h._wbHistoryObserver.fn(); }
function layout(h) { h.style.setProperty=function(k,v){this[k]=v;}; }
function prepare() {
  root.querySelectorAll('template').reverse().forEach(function(t) { t.content=new E('#fragment'); t.content.children=t.children; t.content.children.forEach(function(c){c.parent=t.content;}); t.children=[]; });
  root.querySelectorAll('.wb-history').forEach(function(h) { var live=h.querySelector('.history-live'), clip=new E('div',{'class':'history-clip'}); h.insertBefore(clip,live); clip.insertBefore(live,null); layout(h); });
}
function CustomEvent(type, init) { init = init || {}; this.type = type; this.detail = init.detail; this.bubbles = !!init.bubbles; this.cancelBubble = false; }
CustomEvent.prototype.stopPropagation = function () { this.cancelBubble = true; };
E.prototype.dispatchEvent = function (event) {
  event.target = this;
  if (event.type === 'wbhistoryload') { loads++; check('load event root', event.detail.root === this, true); }
  var path = [this];
  if (event.bubbles) for (var p = this.parent; p; p = p.parent) path.push(p);
  path.some(function (node) {
    event.currentTarget = node;
    (node.on[event.type] || []).slice().forEach(function (fn) { fn.call(node, event); });
    return event.cancelBubble;
  });
  event.currentTarget = null;
  return true;
};
fire = function (element, type) { element.dispatchEvent(new CustomEvent(type, { bubbles: type === 'click' })); };
var observers = [];
function ResizeObserver(fn) { this.fn = fn; observers.push(this); }
ResizeObserver.prototype.observe = function () {}; ResizeObserver.prototype.disconnect = function () {};
function board(n, panelAt) {
  panelAt = panelAt || 1;
  function row(i) {
    var panel = i === panelAt;
    return '<article class="wb-entry '+(panel ? 'panel' : 'agent')+'" id="e'+i+'" data-entry="e'+i+'" data-history-anchor="e'+i+'"'+(panel ? ' data-panel="p1"' : '')+'>'+
      (panel ? '<details class="entry-panel" id="p1" data-key="entry:e'+i+'"><summary>panel</summary><iframe class="doc-frame" data-doc="t:panel:e'+i+'"></iframe></details>' : 'line '+i)+'</article>';
  }
  var start = Math.max(1, n-49), out = '<section class="topic" data-topic="t"><details class="topic-stream" open><summary>history</summary><div class="wb-history" data-history="t"><button class="history-older">older</button><div class="history-live">';
  for (var i = start; i <= n; i++) out += row(i);
  out += '</div>';
  for (var end = start-1; end > 0;) { var from = Math.max(1, end-49); out += '<template data-history-page>'; for (var j = from; j <= end; j++) out += row(j); out += '</template>'; end = from-1; }
  out += '</div></details></section>';
  root = parse(out);
  prepare();
  return root.querySelector('.wb-history');
}
var bound = 0; window.wbBindBoard = function () { bound++; };
var h = board(123);
` + string(asset) + `
check('initial window is 50', h.querySelectorAll('.wb-entry').length, 50);
check('initial one-step height', [h._wbHistorySteps,h.style['--wb-history-height']], [1,'640px']);
resize(h);
check('initial resize never loads', h.querySelectorAll('.wb-entry').length, 50);
window.wbHistory.init(document);
check('init wires container once', (h.on.click || []).length, 1);
check('templates keep control visible', [h.querySelector('.history-older').hidden,h.classList.contains('history-more')], [false,true]);
var priorWatch=h._wbHistoryWatch;
click(h.querySelector('.history-older'));
check('one older page adds fifty', h.querySelectorAll('.wb-entry').length, 100);
check('expansion increments height and rearms', [h._wbHistorySteps,priorWatch.gone,h._wbHistoryWatch!==priorWatch], [2,true,true]);
check('expansion uses shared keep then apply', [keeps.slice(-1)[0].opt.observe[0]===h.querySelector('.history-live'),keeps.slice(-1)[0].applied], [true,1]);
var snapshot = window.wbHistory.capture(document);
var next = board(133);
window.wbHistory.restore(document, snapshot);
check('swap preserves expansion steps', next._wbHistorySteps, 2);
check('swap rematerializes loaded history boundary', next.querySelector('.wb-entry').id, 'e1');
var before = next.querySelectorAll('.wb-entry').length;
check('unknown target stays absent', window.wbHistory.loadTarget(document, 'p999'), null);
check('forged selector refused', window.wbHistory.loadTarget(document, 'p1"] script'), null);
check('bad targets load nothing', next.querySelectorAll('.wb-entry').length, before);
var impostor=new E('div',{id:'p999'}); next.insertBefore(impostor,null);
check('valid ID on non-panel is refused',window.wbHistory.loadTarget(document,'p999'),null);
impostor.attrs={id:'e999','class':'wb-entry','data-entry':'e998'};
check('entry identity mismatch is refused',window.wbHistory.loadTarget(document,'e999'),null);
impostor.remove();
next = board(123); window.wbHistory.init(document);
var target = window.wbHistory.loadTarget(document, 'p1');
check('panel target loaded from oldest template', [target.id, target.open, next.querySelectorAll('.wb-entry').length], ['p1', true, 123]);
check('chronological first/last after reveal', [next.querySelector('.wb-entry').id, next.querySelectorAll('.wb-entry').slice(-1)[0].id], ['e1', 'e123']);
root.querySelector('.topic-stream').open = false;
var targetWatch=next._wbHistoryWatch;
window.wbHistory.loadTarget(document, 'e1');
check('repeated deep-link rearms visibility', [targetWatch.gone,next._wbHistoryWatch!==targetWatch], [true,true]);
check('historical entry reveal opens containing fold', root.querySelector('.topic-stream').open, true);
check('entry reveal opens its panel', next.querySelector('.entry-panel').open, true);
check('target reveals sufficient height', next._wbHistorySteps >= Math.ceil(123*20/640), true);
check('target keep has navigation offset', [keeps.slice(-1)[0].opt.offset,keeps.slice(-1)[0].node.id], [12,'e1']);
keeps.slice(-1)[0].opt.onEnd();
var expanded = next._wbHistorySteps;
next.querySelector('.history-live').insertBefore(new E('article',{'class':'wb-entry agent',id:'e124','data-entry':'e124'}),null);
resize(next);
check('live append leaves expansion stable', next._wbHistorySteps, expanded);
// Control states distinguish clipped materialized content from inert pages.
next = board(50); window.wbHistory.init(document);
check('clipped live rows show control without templates', next.querySelector('.history-older').hidden, false);
click(next.querySelector('.history-older'));
check('fully revealed rows hide control', [next.querySelector('.history-older').hidden,next.classList.contains('history-more')], [true,false]);
next = board(123); next.querySelector('.history-live')._liveHeight=3000; window.wbHistory.init(document);
click(next.querySelector('.history-older')); resize(next);
check('tall live content expands without materialization', [next._wbHistorySteps,next.querySelectorAll('.wb-entry').length], [2,50]);
next = board(10); next.scrollHeight=9999; window.wbHistory.init(document);
check('short initial live rows hide control', next.querySelector('.history-older').hidden, true);
next = board(223); next.querySelector('.history-live')._liveHeight=0; window.wbHistory.init(document);
resize(next);
check('zero initial height still does not load', next.querySelectorAll('.wb-entry').length, 50);
click(next.querySelector('.history-older'));
check('short click materializes at most one page', next.querySelectorAll('.wb-entry').length, 100);
resize(next);
check('short resize materializes at most one further page', next.querySelectorAll('.wb-entry').length, 150);
var fold=next.closest('.topic-stream'); fold.open=false;
var shortSnapshot=window.wbHistory.capture(document);
var shortRoot=root, restored=board(223); window.wbHistory.restore(document,shortSnapshot);
check('capture restores expansion and containing disclosure', [restored._wbHistorySteps,restored.closest('.topic-stream').open], [2,false]);
root=shortRoot;
// Visibility is delivered by the independently tested kit: initial zero is
// not onOut, and every expansion establishes a fresh subscription.
visible(next,0);
check('initial zero does not collapse', next._wbHistorySteps,2);
visible(next,1); visible(next,0);
check('positive to zero collapses height only', [next._wbHistorySteps,next.querySelectorAll('.wb-entry').length,fold.open], [1,150,false]);
resize(next);
check('collapse stops resize loading', next.querySelectorAll('.wb-entry').length,150);
next=board(123); window.wbHistory.init(document); click(next.querySelector('.history-older'));
var input=new E('textarea'); next.closest('.topic').insertBefore(input,null); document.activeElement=input;
visible(next,1); visible(next,0);
check('editing in topic defers collapse',next._wbHistorySteps,2);
document.activeElement=null; document.on.focusout[0](); timers.splice(0).forEach(function(fn){fn();});
check('focusout retries deferred collapse',next._wbHistorySteps,1);
click(next.querySelector('.history-older')); document.activeElement=next.querySelector('.wb-entry');
visible(next,1); visible(next,0);
check('focus inside history defers collapse',next._wbHistorySteps,2);
document.activeElement=null; visible(next,1); next.closest('.topic').setAttribute('data-max',''); visible(next,0);
check('maximized topic defers collapse',next._wbHistorySteps,2);
next.closest('.topic').removeAttribute('data-max');
var finishMotion; motions=[{playState:'running',effect:{target:next.closest('.topic')},finished:new Promise(function(resolve){finishMotion=resolve;})}];
visible(next,1); visible(next,0);
check('running board motion defers collapse', [next._wbHistorySteps,next._wbHistoryWaiting], [2,true]);
var motionHistory=next, motionRoot=root; motions=[];
// Resolve after the synchronous regression fixtures below complete.
Promise.resolve().then(function(){ root=motionRoot; finishMotion(); });
Promise.resolve().then(function(){return Promise.resolve();}).then(function(){
  check('motion completion retries collapse',motionHistory._wbHistorySteps,1);
  if(bad.length) throw new Error(bad.join('\n'));
});
var beforeBound = bound;
window.wbBindBoard(root);
check('board binding wrapper preserves existing host hook', bound, beforeBound + 1);
check('load notification emitted', loads > 0, true);
// Re-posting changes a prior latest version's disclosure key, so automatic
// refresh cannot restore its former open state beside the new latest card.
next = board(123); window.wbHistory.init(document);
window.wbHistory.loadTarget(document, 'p1');
var latestOpen = window.wbHistory.capture(document);
next = board(124); var historicalPanel = window.wbHistory.loadTarget(document, 'p1');
historicalPanel.setAttribute('data-key', 'entry:e1:history'); historicalPanel.open = false;
window.wbHistory.restore(document, latestOpen);
check('repost folds previously expanded latest version', historicalPanel.open, false);
historicalPanel.open = true;
var historicalOpen = window.wbHistory.capture(document);
next = board(125); historicalPanel = window.wbHistory.loadTarget(document, 'p1');
historicalPanel.setAttribute('data-key', 'entry:e1:history'); historicalPanel.open = false;
window.wbHistory.restore(document, historicalOpen);
check('manual historical disclosure survives later swaps', historicalPanel.open, true);
// A single system run crosses the fifty-entry page boundary. Preserve the
// existing disclosure and chronological child entries when the chips merge.
function systems(from, end, open) { var out = '<details class="system-run" data-key="system:e'+from+'" data-history-anchor="e'+from+'"'+(open ? ' open' : '')+'><summary>system events '+(end-from+1)+'</summary>'; for(var i=from;i<=end;i++) out += '<article class="wb-entry system" id="e'+i+'" data-entry="e'+i+'" data-history-anchor="e'+i+'">event</article>'; return out+'</details>'; }
root = parse('<div class="wb-history" data-history="t"><button class="history-older">older</button><div class="history-live">'+systems(51,80,true)+'<article class="wb-entry agent" id="e81" data-entry="e81">boundary</article></div><template data-history-page>'+systems(1,50,false)+'</template></div>');
prepare();
h = root.querySelector('.wb-history'); h.clientHeight=100;
window.wbHistory.init(document);
var disclosure = h.querySelector('.system-run');
click(h.querySelector('.history-older'));
check('adjacent page fragments merge into one system chip', h.querySelectorAll('.system-run').length, 1);
check('existing system disclosure preserved', [h.querySelector('.system-run') === disclosure, disclosure.open], [true,true]);
check('merged count', disclosure.children[0].textContent, 'system events 80');
check('merged chronological range', [disclosure.querySelectorAll('.wb-entry')[0].id, disclosure.querySelectorAll('.wb-entry').slice(-1)[0].id], ['e1','e80']);
check('merge does not swallow following agent', h.querySelector('.history-live').children.slice(-1)[0].id, 'e81');
// A log run crosses the boundary too: the older page's folded rows join the
// newer page's run fold, which keeps its node, open state, and visible tail,
// and the system chips that meet at the boundary merge inside it.
function agents(from, end) { var out = ''; for (var i=from;i<=end;i++) out += '<article class="wb-entry agent" id="e'+i+'" data-entry="e'+i+'" data-history-anchor="e'+i+'">line</article>'; return out; }
function run(from, inner, open) { return '<details class="run-fold" data-key="run:e'+from+'" data-history-anchor="e'+from+'"'+(open ? ' open' : '')+'><summary><span class="rl"></span><span class="lbl">log</span></summary>'+inner+'</details>'; }
root = parse('<div class="wb-history" data-history="t"><button class="history-older">older</button><div class="history-live">'+run(60, systems(60,61,false)+agents(62,96), true)+agents(97,99)+'</div><template data-history-page><article class="wb-entry panel" id="e30" data-entry="e30" data-panel="p1"><details class="entry-panel" id="p1"><summary>p1</summary></details></article>'+run(31, agents(31,57)+systems(58,59,false), false)+'</template></div>');
prepare();
h = root.querySelector('.wb-history'); h.clientHeight=100;
window.wbHistory.init(document);
var runFold = h.querySelector('.run-fold'), newerChip = runFold.querySelector('.system-run');
click(h.querySelector('.history-older'));
check('boundary run folds join into the newer fold', [h.querySelectorAll('.run-fold').length, h.querySelector('.run-fold') === runFold, runFold.open], [1, true, true]);
check('joined fold keeps the stream order', h.querySelector('.history-live').children.map(function(c) { return c.id || c.getAttribute('data-key'); }), ['e30', 'run:e31', 'e97', 'e98', 'e99']);
check('joined fold relabels', [runFold.getAttribute('data-history-anchor'), runFold.querySelector('.lbl').textContent], ['e31', 'log 66']);
check('boundary system chips merge inside the fold', [runFold.querySelectorAll('.system-run').length, runFold.querySelector('.system-run') === newerChip, newerChip.querySelectorAll('.wb-entry').length], [1, true, 4]);
// A closed topic beyond the initial cap is itself inert, with an older
// panel page inside. A direct target loads only that topic, then its page.
root = parse('<template data-history-topic="old"><section class="topic" data-topic="old"><details class="topic-stream"><summary>history</summary><div class="wb-history" data-history="old"><button class="history-older">older</button><div class="history-live"><article class="wb-entry agent" id="e2" data-entry="e2">new</article></div><template data-history-page><article class="wb-entry panel" id="e1" data-entry="e1" data-panel="p1"><details class="entry-panel" id="p1"><summary>old</summary></details></article></template></div></details></section></template>');
// Detach inner templates before their outer owner, as native template
// contents are separate document fragments at both nesting levels.
prepare();
check('deferred topic is not initially live', document.querySelectorAll('.topic').length, 0);
target = window.wbHistory.loadTarget(document, 'p1');
check('navigation loads deferred topic and panel page', [document.querySelectorAll('.topic').length, target && target.id, target && target.open], [1,'p1',true]);
check('deferred topic keeps actual key', document.querySelector('.topic').getAttribute('data-topic'), 'old');
// Retaining a panel frame during a log append must not strand the new
// older-page button on an already-bound history container.
function framedBoard(n) {
  var h = board(n, 140), fold = h.closest('.topic-stream');
  var topic = new E('section', { 'class': 'topic', 'data-topic': 't' });
  root.insertBefore(topic, fold);
  topic.insertBefore(fold, null);
  return h;
}
// Model frameGraft's child replacement down the retained frame ancestry:
// keep the section/history/live/panel path, replace its siblings, and put
// the incoming frame beside the old one pending its asynchronous reveal.
function graftFrame(oldTopic, newTopic) {
  var oldFrame = oldTopic.querySelector('iframe.doc-frame'), newFrame = newTopic.querySelector('iframe.doc-frame');
  function path(frame, topic) { var out = []; for (var p = frame; p; p = p.parentElement) { out.push(p); if (p === topic) break; } return out; }
  var oldPath = path(oldFrame, oldTopic), newPath = path(newFrame, newTopic);
  check('graft paths have matching shape', oldPath.map(function (n) { return n.tagName; }), newPath.map(function (n) { return n.tagName; }));
  newFrame.setAttribute('class', 'doc-frame doc-frame-next');
  for (var i = oldPath.length - 1; i > 0; i--) {
    var kept = oldPath[i - 1], incoming = newPath[i - 1], parent = oldPath[i], before = true;
    parent.attrs = Object.assign({}, newPath[i].attrs);
    parent.children.slice().forEach(function (child) { if (child !== kept) child.remove(); });
    newPath[i].children.slice().forEach(function (child) {
      if (child === incoming) { before = false; if (child === newFrame) parent.insertBefore(child, null); }
      else parent.insertBefore(child, before ? kept : null);
    });
  }
}
h = framedBoard(160);
window.wbHistory.init(document);
check('graft fixture starts with fifty entries', h.querySelectorAll('.wb-entry').length, 50);
var retainedRoot = root, topic = h.closest('.topic'), live = h.querySelector('.history-live');
var frame = h.querySelector('iframe.doc-frame'), oldButton = h.querySelector('.history-older'), oldLine = h.querySelector('[id="e150"]');
check('first live frame is a late panel', frame.closest('.wb-entry').id, 'e140');
h.querySelector('.entry-panel').open = true;
click(h.querySelector('.history-older'));
var snapshot = window.wbHistory.capture(document);
check('reader expansion captured', snapshot[0].steps, 2);
// An agent-log append shifts the fifty-entry tail by one. Unlike a whole
// board replacement, the graft leaves _wbHistoryBound on the live container.
var fresh = framedBoard(161), replacementButton = fresh.querySelector('.history-older');
graftFrame(topic, fresh.closest('.topic'));
root = retainedRoot;
check('history and live containers retained', [topic.querySelector('.wb-history') === h, h.querySelector('.history-live') === live, h._wbHistoryBound], [true, true, true]);
check('graft replaces older button and ordinary live entries', [h.querySelector('.history-older') === replacementButton, oldButton.parentElement, h.querySelector('[id="e150"]') !== oldLine], [true, null, true]);
check('graft retains old frame beside pending incoming frame', [h.querySelector('iframe.doc-frame') === frame, h.querySelectorAll('iframe.doc-frame').length], [true, 2]);
window.wbHistory.init(topic);
window.wbHistory.restore(topic, snapshot);
check('graft restoration keeps one delegated listener', (h.on.click || []).length, 1);
check('graft restoration preserves expansion', window.wbHistory.capture(document)[0].steps, 2);
check('restoration rematerializes saved boundary', [h.querySelectorAll('.wb-entry').length, h.querySelector('.wb-entry').id], [150, 'e12']);
check('restoration keeps panel disclosure open', h.querySelector('.entry-panel').open, true);
var count = h.querySelectorAll('.wb-entry').length, beforeLoads = loads;
h.querySelector('.history-live')._liveHeight=0;
click(h.querySelector('[id="e150"]'));
check('unrelated live clicks do not load history', h.querySelectorAll('.wb-entry').length, count);
var label = new E('span'); replacementButton.insertBefore(label, null);
var clickSeen = null, loadSeen = null, scrolls = 0;
retainedRoot.addEventListener('click', function (event) { clickSeen = [event.target === label, event.currentTarget === retainedRoot, event.bubbles]; });
retainedRoot.addEventListener('wbhistoryload', function (event) { loadSeen = [event.target === h, event.currentTarget === retainedRoot, event.detail.root === h]; });
retainedRoot.addEventListener('scroll', function () { scrolls++; });
fire(h, 'scroll');
check('scroll events do not bubble in history harness', scrolls, 0);
click(label);
check('replacement button descendant click bubbles with original target', clickSeen, [true, true, true]);
check('older click after graft loads remaining page', h.querySelectorAll('.wb-entry').length, count + 11);
check('older click after graft preserves chronological bounds', [h.querySelector('.wb-entry').id, h.querySelectorAll('.wb-entry').slice(-1)[0].id], ['e1', 'e161']);
check('older click after graft expands height', h._wbHistorySteps, 3);
check('older click after graft emits one bubbling load', [loads - beforeLoads, loadSeen], [1, [true, true, true]]);
if (bad.length) throw new Error(bad.join('\n'));
`
	cmd := nodeScript(node, script)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("history script: %v\n%s", err, out)
	}
}

func TestHistoryScriptWiredOnlyIntoFullPagesAndSharedAssets(t *testing.T) {
	a, err := loadAssets()
	if err != nil {
		t.Fatal(err)
	}
	shared, err := PageAssets()
	if err != nil || !strings.Contains(string(shared.FormScript), a.stream) {
		t.Fatalf("shared history script missing: %v", err)
	}
	for _, layout := range []Layout{LayoutFull, LayoutEmbed} {
		p := newPageData(BuildView(streamFixture(), ViewInput{Layout: layout}), a, false)
		if got := strings.Contains(string(p.Script), a.stream); got != (layout == LayoutFull) {
			t.Errorf("layout %d history wiring = %v", layout, got)
		}
	}
	if !strings.Contains(string(a.css), "--wb-history-height") || !strings.Contains(string(a.css), ".history-clip") || !strings.Contains(string(a.css), "flex-end") {
		t.Fatal("history must use a bottom-aligned height clip")
	}
	if strings.Contains(string(a.css), "grid-template-columns: minmax(0, 1fr) 320px") {
		t.Fatal("full page still reserves a side rail")
	}
}
