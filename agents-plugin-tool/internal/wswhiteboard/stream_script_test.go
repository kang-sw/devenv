package wswhiteboard

import (
	"os/exec"
	"strings"
	"testing"
)

func TestHistoryScriptPagingRevealAndSwapAnchorsUnderNode(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	asset, err := embeddedAsset("stream.js")
	if err != nil {
		t.Fatal(err)
	}
	// Keep the history DOM and bubbling model local to this stream test.
	script := `var BOARD = '', BODY_CLASS = '';` + formPrelude + `
Object.defineProperty(E.prototype, 'id', { get: function () { return this.attrs.id || ''; } });
Object.defineProperty(E.prototype, 'tagName', { get: function () { return this.tag.toUpperCase(); } });
Object.defineProperty(E.prototype, 'parentElement', { get: function () { return this.parent; } });
Object.defineProperty(E.prototype, 'firstChild', { get: function () { return this.children[0] || null; } });
Object.defineProperty(E.prototype, 'nextElementSibling', { get: function () { return this.parent && this.parent.children[this.parent.children.indexOf(this)+1] || null; } });
Object.defineProperty(E.prototype, 'open', { get: function () { return 'open' in this.attrs; }, set: function (v) { if (v) this.attrs.open = ''; else delete this.attrs.open; } });
E.prototype.matches = function (s) { return one(this, s); };
E.prototype.remove = function () { this.parent.children.splice(this.parent.children.indexOf(this), 1); this.parent = null; };
E.prototype.cloneNode = function () { var n = new E(this.tag, Object.assign({}, this.attrs)); this.children.forEach(function (c) { var child = c.cloneNode(true); child.parent = n; n.children.push(child); }); if (this.content) n.content = this.content.cloneNode(true); return n; };
E.prototype.insertBefore = function (node, before) { var list = node.tag === '#fragment' ? node.children.slice() : [node], self = this; list.forEach(function(c) { if(c.parent) c.remove(); }); var i = before ? this.children.indexOf(before) : this.children.length; this.children.splice.apply(this.children, [i, 0].concat(list)); list.forEach(function (c) { c.parent = self; }); };
Object.defineProperty(E.prototype, 'scrollHeight', { get: function () { var live = this.querySelector('.history-live'); return this.cls().includes('wb-history') && live ? live.children.length * 20 : (this._height || 20); }, set: function (n) { this._height = n; } });
Object.defineProperty(E.prototype, 'scrollTop', { get: function () { return this._top || 0; }, set: function (n) { this._top = Math.max(0, Math.min(n, Math.max(0, this.scrollHeight - (this.clientHeight || 0)))); } });
E.prototype.getBoundingClientRect = function () {
  if (this.cls().includes('wb-history')) return { top: 0, bottom: 100, height: 100 };
  var h = this.closest('.wb-history');
  if (!h) return { top: 0, bottom: 0, height: 0 };
  var article = this.closest('.wb-entry'), live = h.querySelector('.history-live'), index = live.children.indexOf(article);
  var top = index * 20 - h.scrollTop;
  return { top: top, bottom: top + 20, height: index >= 0 ? 20 : 0 };
};
var loads = 0;
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
  var start = Math.max(1, n-49), out = '<details class="topic-stream" open><summary>history</summary><div class="wb-history" data-history="t"><button class="history-older">older</button><div class="history-live">';
  for (var i = start; i <= n; i++) out += row(i);
  out += '</div>';
  for (var end = start-1; end > 0;) { var from = Math.max(1, end-49); out += '<template data-history-page>'; for (var j = from; j <= end; j++) out += row(j); out += '</template>'; end = from-1; }
  out += '</div></details>';
  root = parse(out);
  root.querySelectorAll('template').forEach(function (t) { t.content = new E('#fragment'); t.content.children = t.children; t.content.children.forEach(function(c) { c.parent = t.content; }); t.children = []; });
  var h = root.querySelector('.wb-history'); h.clientHeight = 100; return h;
}
var bound = 0; window.wbBindBoard = function () { bound++; };
var h = board(123);
` + string(asset) + `
check('initial window is 50', h.querySelectorAll('.wb-entry').length, 50);
check('initial reader follows bottom', h.scrollTop, 900);
window.wbHistory.init(document);
check('init wires container once', (h.on.click || []).length, 1);
h.scrollTop = 220; fire(h, 'scroll');
var anchor = window.wbHistory.capture(document)[0].position.anchor;
click(h.querySelector('.history-older'));
check('one older page adds fifty', h.querySelectorAll('.wb-entry').length, 100);
check('older prepend preserves entry offset', window.wbHistory.capture(document)[0].position.anchor, anchor);
check('older prepend shifts internal scroll only', h.scrollTop, 1220);
var snapshot = window.wbHistory.capture(document), oldTop = h.scrollTop;
var next = board(133);
window.wbHistory.restore(document, snapshot);
check('swap preserves reading anchor', window.wbHistory.capture(document)[0].position.anchor, anchor);
check('swap rematerializes loaded history boundary', next.querySelector('.wb-entry').id, 'e1');
var before = next.querySelectorAll('.wb-entry').length;
check('unknown target stays absent', window.wbHistory.loadTarget(document, 'p999'), null);
check('forged selector refused', window.wbHistory.loadTarget(document, 'p1"] script'), null);
check('bad targets load nothing', next.querySelectorAll('.wb-entry').length, before);
next = board(123); window.wbHistory.init(document);
var target = window.wbHistory.loadTarget(document, 'p1');
check('panel target loaded from oldest template', [target.id, target.open, next.querySelectorAll('.wb-entry').length], ['p1', true, 123]);
check('chronological first/last after reveal', [next.querySelector('.wb-entry').id, next.querySelectorAll('.wb-entry').slice(-1)[0].id], ['e1', 'e123']);
root.querySelector('.topic-stream').open = false;
window.wbHistory.loadTarget(document, 'e1');
check('historical entry reveal opens containing fold', root.querySelector('.topic-stream').open, true);
check('entry reveal opens its panel', next.querySelector('.entry-panel').open, true);
next.scrollTop = next.scrollHeight; fire(next, 'scroll');
var bottom = window.wbHistory.capture(document);
next = board(133); window.wbHistory.restore(document, bottom);
check('bottom reader follows new entries after swap', next.scrollTop, next.scrollHeight-next.clientHeight);
// Simulated asynchronous frame growth: a bottom reader follows, while a
// history reader retains the visible entry and offset.
next.scrollTop = 400; fire(next, 'scroll');
var reading = window.wbHistory.capture(document)[0].position.anchor;
observers[observers.length-1].fn();
check('asynchronous resize preserves reading anchor', window.wbHistory.capture(document)[0].position.anchor, reading);
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
var template = root.querySelector('template'); template.content = new E('#fragment'); template.content.children = template.children; template.content.children.forEach(function(c) { c.parent = template.content; }); template.children=[];
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
template = root.querySelector('template'); template.content = new E('#fragment'); template.content.children = template.children; template.content.children.forEach(function(c) { c.parent = template.content; }); template.children=[];
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
root.querySelectorAll('template').reverse().forEach(function(t) { t.content=new E('#fragment'); t.content.children=t.children; t.content.children.forEach(function(c) { c.parent=t.content; }); t.children=[]; });
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
h.scrollTop = 587; fire(h, 'scroll');
var snapshot = window.wbHistory.capture(document), anchor = snapshot[0].position.anchor;
check('reader is anchored inside retained panel', anchor, { id: 'e140', run: false, offset: -7 });
check('reader is not following bottom', snapshot[0].position.bottom, false);
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
window.wbHistory.restore(document, snapshot);
check('graft restoration keeps one delegated listener', (h.on.click || []).length, 1);
check('graft restoration preserves viewport anchor', window.wbHistory.capture(document)[0].position.anchor, anchor);
check('restoration materializes just the saved boundary page', [h.querySelectorAll('.wb-entry').length, h.querySelector('.wb-entry').id], [100, 'e62']);
check('restoration keeps panel disclosure open', h.querySelector('.entry-panel').open, true);
var count = h.querySelectorAll('.wb-entry').length, top = h.scrollTop, beforeLoads = loads;
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
check('older click after graft loads exactly fifty', h.querySelectorAll('.wb-entry').length, count + 50);
check('older click after graft preserves chronological bounds', [h.querySelector('.wb-entry').id, h.querySelectorAll('.wb-entry').slice(-1)[0].id], ['e12', 'e161']);
check('older click after graft preserves viewport anchor', window.wbHistory.capture(document)[0].position.anchor, anchor);
check('older click after graft shifts only internal scroll', h.scrollTop, top + 1000);
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
	if !strings.Contains(string(a.css), "max-height: min(65vh, 720px)") {
		t.Fatal("history must have a bounded-height scroll area")
	}
	if strings.Contains(string(a.css), "grid-template-columns: minmax(0, 1fr) 320px") {
		t.Fatal("full page still reserves a side rail")
	}
}
