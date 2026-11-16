// Pure Node: reuse the existing whiteboard DOM fixture, never a ws executable.
const { test } = require('node:test');
const fs = require('node:fs');
const assert = require('node:assert/strict');
const vm = require('node:vm');
const path = require('node:path');
const wb = path.join(__dirname, '../wswhiteboard');
const prelude = fs.readFileSync(path.join(wb, 'check_form_test.go'), 'utf8').split('const formPrelude = `')[1].split('`')[0];
const contract = fs.readFileSync(path.join(wb, 'stream_script_test.go'), 'utf8');
const rawSetup = contract.split('script := `')[1].split('` + string(asset) + `')[0];
const setup = rawSetup.slice(rawSetup.indexOf('\nObject.defineProperty'));
const regression = contract.split('` + string(asset) + `')[1].split('\n`')[0];
const asset = fs.readFileSync(path.join(wb, 'assets/stream.js'), 'utf8').replace('var kit = viewKit(window);', 'var kit = window.historyKit;');
function run(source) { vm.runInNewContext(source, { console }, { timeout: 10000 }); }

test('history retains gap, pending-height, navigation, restore and auto-fold contracts', () => {
  run("var BOARD='',BODY_CLASS='';" + prelude + setup + asset.replace('h._wbHistoryFilling = true; fill(h);', '') + regression);
});

test('watchBoard subscriptions defer visibility and scroller probes into one read phase', () => {
  const frames = [], order = [];
  const style = { set overflowAnchor(value) { order.push('write:owner'); } };
  const scroller = { style, scrollTop: 0, addEventListener() {} };
  const body = {}, documentElement = { style };
  const window = { document: { body, documentElement, scrollingElement: scroller },
    requestAnimationFrame(fn) { frames.push(fn); }, addEventListener() {},
    IntersectionObserver: class { observe() {} unobserve() {} },
    getComputedStyle() { order.push('read:scroller'); return { overflowY: 'auto' }; }
  };
  const context = { window }; vm.createContext(context);
  vm.runInContext(asset.slice(asset.indexOf('  function viewKit(win)'), asset.indexOf('  // Current panel folds')) + '\nthis.kit = viewKit(window);', context);
  const parent = { parentElement: body, scrollHeight: 1000, clientHeight: 100 };
  for (let i = 0; i < 27; i++) context.kit.watch({ isConnected: true, parentElement: parent, checkVisibility() { order.push('read:visibility'); return true; } });
  assert.deepEqual(order, []);
  assert.equal(frames.length, 1);
  parent.style = style; parent.scrollTop = 0; parent.addEventListener = () => {};
  frames.shift()();
  assert.equal(order.filter(x => x === 'read:visibility').length, 27);
  assert.equal(order.at(-1), 'write:owner');
  assert.equal(order.filter(x => x === 'write:owner').length, 1);
});

test('27-topic initial history batches writes before reads and coalesces late resumes', () => {
  const fixture = setup.slice(setup.indexOf('\nObject.defineProperty'));
  run("var BOARD='',BODY_CLASS='';" + prelude + fixture + `
var histories=[];for(var i=0;i<27;i++)histories.push(board(gap(rows(1,200,7),true)+row(201,20)));
root=new E('div');histories.forEach(function(h){root.appendChild(h.closest('.topic'));});
var dirty=false,flushes=0,reads=0,baseInsert=E.prototype.insertBefore,baseRect=E.prototype.getBoundingClientRect;
E.prototype.insertBefore=function(n,b){dirty=true;return baseInsert.call(this,n,b);};
E.prototype.getBoundingClientRect=function(){reads++;if(dirty){flushes++;dirty=false;}return baseRect.call(this);};
` + asset + `
check('27 exact bottom windows',histories.map(function(h){return liveIDs(h).length;}),Array(27).fill(141));
check('batch layout flushes bounded across topics',flushes<=7,true);
flush();check('one history event per topic',loads,27);
var prior=loads;histories.forEach(function(h){h._wbHistoryObserver.fn();h._wbHistoryObserver.fn();});flush();flush();
check('settled resize does not add rows or notifications',loads,prior);
// A pending frame stays the final admitted row; repeated resize reports share one rAF.
var panel='<article class="wb-entry panel" id="e12" data-entry="e12"><details class="entry-panel" open><iframe class="doc-frame"></iframe></details></article>';
h=board(gap(row(11,600)+row(12,500,panel)+row(13,100),true)+row(14,50));window.wbHistory.init(document);
check('unknown height returns speculative older rows',liveIDs(h),['e12','e13','e14']);
flush();var start=frames.length;h.querySelector('.doc-frame').style.height='500px';h._wbHistoryObserver.fn();h._wbHistoryObserver.fn();
check('late height reports coalesce',frames.length-start,1);flush();flush();
check('late height reaches exact window',liveIDs(h),['e11','e12','e13','e14']);
if(bad.length)throw Error(bad.join('\\n'));
`);
});
