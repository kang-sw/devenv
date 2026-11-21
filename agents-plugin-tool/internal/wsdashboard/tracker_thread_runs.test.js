const test = require('node:test');
const assert = require('node:assert/strict');
const vm = require('node:vm');
const { readAssetSource } = require('./testdata/asset-source.cjs');

const source = readAssetSource('wsdashboard', 'tracker.js');

function section(start, end) {
  const a = source.indexOf(start), b = source.indexOf(end, a);
  assert.ok(a >= 0 && b > a, `missing section ${start} .. ${end}`);
  return source.slice(a, b);
}

function harness() {
  const members = {
    ada: { name: 'Ada Park', profile: { org: 'Platform', family: 'Park', given: 'Ada' } },
    maya: { name: 'Maya Ortiz', profile: { org: 'Search', family: 'Ortiz', given: 'Maya' } },
    raj: { name: 'Raj Rao', profile: { org: 'Infra', family: 'Rao', given: 'Raj' } }
  };
  const ui = { store: 's1', item: 'i1' };
  const EV = {};
  const VS = {
    members,
    items: { i1: { id: 'i1', kind: 'task', title: 'Main', thread: [] } },
    comments: {},
    priority: { names: ['Low', 'Normal', 'High'], default: 1 },
    events: []
  };
  const c = {
    ui, EV, VS,
    esc: s => String(s == null ? '' : s).replace(/[&<>"]/g, ch => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' }[ch])),
    attr: (k, v) => ` data-${k}="${String(v)}"`,
    tipAttr: t => ` data-tip="${String(t)}"`,
    fmtTs: ts => ts ? String(ts).slice(5, 16).replace('T', ' ') : '-',
    nm: id => (members[id] ? members[id].name : id),
    whoName: id => {
      const m = members[id];
      return m && m.profile && m.profile.org ? `<span class="org">${m.profile.org}</span> ${m.name}` : (m ? m.name : id);
    },
    originChip: o => (o === 'agent' ? '<span class="chip agent">agent</span>' : o === 'agent-auto' ? '<span class="chip agent">agent</span><span class="chip auto">auto</span>' : ''),
    describe: (n, inThread) => `${n.type}${inThread === 'parent' ? '(parent)' : ''}${n.payload && n.payload.status ? ' <span class="chip st-' + n.payload.status + '">' + n.payload.status + '</span>' : ''}`,
    flags: n => (n.status === 'flagged' ? '<span class="chip flag">flagged</span>' : n.status === 'waiting' ? '<span class="chip">waiting</span>' : '') + (n.markers && n.markers.length ? '<span class="chip conc-c">concurrent</span>' : ''),
    markerHtml: n => (n.markers || []).map(m => `<div class="conc">${m.kind}</div>`).join(''),
    supHtml: () => '',
    commentHtml: n => `<li class="cmt" data-ev="${n.id}">${n.payload && n.payload.body || ''}</li>`,
    wsT: (k, ...args) => {
      const tbl = {
        'js.tk.win.thread': 'Thread',
        'js.tk.tr.expand': 'Expand',
        'js.tk.tr.collapse': 'Collapse',
        'js.tk.tr.expandall': 'Expand all logs',
        'js.tk.tr.collapseall': 'Collapse all logs',
        'js.tk.tr.tip.open': '{0} · Click to expand',
        'js.tk.tr.tip.close': '{0} · Click to collapse',
        'js.tk.tr.k.create': 'created',
        'js.tk.tr.k.edit': 'edited',
        'js.tk.tr.k.cc': 'CC added',
        'js.tk.tr.k.uncc': 'CC removed',
        'js.tk.tr.k.assign': 'assigned',
        'js.tk.tr.k.close': 'closed',
        'js.tk.tr.k.confirm': 'approved',
        'js.tk.tr.k.reject': 'sent back',
        'js.tk.tr.k.reopen': 'reopened',
        'js.tk.tr.k.misc': 'other'
      };
      const s = tbl[k] || k;
      return s.replace(/\{(\d+)\}/g, (_, i) => args[Number(i)] != null ? String(args[Number(i)]) : `{${i}}`);
    },
    wsTn: (k, n, ...rest) => {
      const args = [n, ...rest];
      const tbl = {
        'js.tk.tr.count.one': '{0} system event',
        'js.tk.tr.count.other': '{0} system events',
        'js.tk.tr.others.one': '{1} +{0} other',
        'js.tk.tr.others.other': '{1} +{0} others'
      };
      const s = tbl[k + (n === 1 ? '.one' : '.other')] || k;
      return s.replace(/\{(\d+)\}/g, (_, i) => args[Number(i)] != null ? String(args[Number(i)]) : `{${i}}`);
    }
  };
  vm.createContext(c);
  vm.runInContext(section('  // Thread run grouping and compact system lines', '  // The edit mode (ui.iedit)'), c);
  return c;
}

function ev(id, type, extra = {}) {
  return {
    id,
    alias: id.replace('#', '-'),
    type,
    member: extra.member || 'ada',
    ts: extra.ts || '2026-11-19T01:09:00Z',
    thread: extra.thread !== undefined ? extra.thread : 'i1',
    parentThread: extra.parentThread || null,
    status: extra.status || 'applied',
    ignored: !!extra.ignored,
    origin: extra.origin || 'human',
    markers: extra.markers || [],
    payload: extra.payload || {}
  };
}

test('comment boundaries, 2+ run threshold, and non-item comments', () => {
  const c = harness();
  const list = [
    ev('e1', 'item.create'),
    ev('e2', 'comment.add', { thread: 'i1', payload: { body: 'hi' } }),
    ev('e3', 'item.cc'),
    ev('e4', 'item.cc'),
    ev('e5', 'comment.add', { thread: 'other-item', payload: { body: 'child' } }),
    ev('e6', 'comment.add', { thread: 'i1', payload: { body: 'bye' } }),
    ev('e7', 'item.edit')
  ];
  const g = c.groupThreadEvents(list, 'i1');
  assert.deepEqual(Array.from(g, x => x.kind), ['single', 'comment', 'run', 'comment', 'single']);
  assert.equal(g[0].ev.id, 'e1');
  assert.equal(g[2].id, 'e3');
  assert.deepEqual(Array.from(g[2].events, x => x.id), ['e3', 'e4', 'e5']);
  assert.equal(g[4].ev.id, 'e7');
});

test('4+ default collapse; flagged or concurrency-marked stays open; reject and waiting do not', () => {
  const c = harness();
  const r2 = [ev('a1', 'item.cc'), ev('a2', 'item.cc')];
  const r3 = [...r2, ev('a3', 'item.cc')];
  const r4 = [...r3, ev('a4', 'item.cc')];
  assert.equal(c.runDefaultCollapsed(r2), false);
  assert.equal(c.runDefaultCollapsed(r3), false);
  assert.equal(c.runDefaultCollapsed(r4), true);

  // Flagged and ignored-flagged keep a 4+ run open.
  assert.equal(c.runDefaultCollapsed([...r3, ev('f1', 'item.close', { status: 'flagged' })]), false);
  assert.equal(c.runDefaultCollapsed([...r3, ev('f2', 'item.close', { status: 'flagged', ignored: true })]), false);
  // Concurrency markers keep a 4+ run open.
  assert.equal(c.runDefaultCollapsed([...r3, ev('m1', 'item.edit', { markers: [{ kind: 'field' }] })]), false);

  // item.reject and status=waiting do NOT keep a 4+ run open (Decision 4).
  assert.equal(c.runDefaultCollapsed([...r3, ev('j1', 'item.reject')]), true);
  assert.equal(c.runDefaultCollapsed([...r3, ev('w1', 'item.edit', { status: 'waiting' })]), true);
});

test('item.status breaks runs; other lifecycle events and child item.create do not', () => {
  const c = harness();
  const list = [
    ev('e1', 'item.create'),
    ev('e2', 'item.status', { payload: { status: 'doing' } }),
    ev('e3', 'item.assign'),
    ev('e4', 'item.close'),
    ev('e5', 'item.reopen'),
    ev('e6', 'item.confirm'),
    ev('e7', 'item.reject'),
    ev('e8', 'item.create', { parentThread: 'i1', thread: 'child1' }),
    ev('e9', 'item.status', { payload: { status: 'blocked' } }),
    ev('e10', 'item.cc'),
    ev('e11', 'item.cc')
  ];
  const g = c.groupThreadEvents(list, 'i1');
  assert.deepEqual(Array.from(g, x => x.kind), ['single', 'status', 'run', 'status', 'run']);
  assert.equal(g[0].ev.id, 'e1');
  assert.equal(g[1].ev.id, 'e2');
  assert.equal(g[2].id, 'e3');
  assert.deepEqual(Array.from(g[2].events, x => x.id), ['e3', 'e4', 'e5', 'e6', 'e7', 'e8']);
  assert.equal(c.runDefaultCollapsed(g[2].events), true);
  assert.equal(g[3].ev.id, 'e9');
  assert.equal(g[4].id, 'e10');
  assert.deepEqual(Array.from(g[4].events, x => x.id), ['e10', 'e11']);
});

test('fold state keyed by first event id: untoggled recomputes default; explicit toggle survives re-render', () => {
  const c = harness();
  const foldMap = Object.create(null);

  // Untoggled run starts with 3 events -> open.
  let run = { id: 'e1', events: [ev('e1', 'item.cc'), ev('e2', 'item.cc'), ev('e3', 'item.cc')] };
  assert.equal(c.isRunCollapsed(run, foldMap), false);

  // Run grows to 4 events while still untoggled -> recomputes default -> collapsed.
  run = { id: 'e1', events: [...run.events, ev('e4', 'item.cc')] };
  assert.equal(c.isRunCollapsed(run, foldMap), true);

  // Run gains a flagged event while still untoggled -> recomputes default -> open.
  run = { id: 'e1', events: [...run.events, ev('e5', 'item.close', { status: 'flagged' })] };
  assert.equal(c.isRunCollapsed(run, foldMap), false);

  // Explicit toggle on e1 -> stays collapsed even with flagged event and across growth.
  c.setRunCollapsed(foldMap, 'e1', true);
  assert.equal(c.isRunCollapsed(run, foldMap), true);
  run = { id: 'e1', events: [...run.events, ev('e6', 'item.cc')] };
  assert.equal(c.isRunCollapsed(run, foldMap), true);

  // Expand-all writes explicit toggles only for currently shown runs; a later run follows default.
  const runB = { kind: 'run', id: 'b1', events: [ev('b1', 'item.cc'), ev('b2', 'item.cc'), ev('b3', 'item.cc'), ev('b4', 'item.cc')] };
  const groups1 = [{ kind: 'run', id: 'e1', events: run.events }, runB];
  assert.equal(c.hasCollapsedRun(groups1, foldMap), true);
  c.setAllRunsCollapsed(groups1, foldMap, false);
  assert.equal(c.isRunCollapsed(groups1[0], foldMap), false);
  assert.equal(c.isRunCollapsed(runB, foldMap), false);
  assert.equal(c.hasCollapsedRun(groups1, foldMap), false);

  // A new 4-event run appearing later is not in foldMap, so it follows its collapsed default.
  const runC = { kind: 'run', id: 'c1', events: [ev('c1', 'item.cc'), ev('c2', 'item.cc'), ev('c3', 'item.cc'), ev('c4', 'item.cc')] };
  const groups2 = [...groups1, runC];
  assert.equal(c.isRunCollapsed(runC, foldMap), true);
  assert.equal(c.hasCollapsedRun(groups2, foldMap), true);
});

test('rendered thread HTML: compact lines, same-author suppression, HH:MM in runs, summary, status break, and evLine untouched', () => {
  const c = harness();
  const events = [
    ev('e1', 'item.create', { member: 'ada', ts: '2026-11-19T00:41:00Z' }),
    ev('e2', 'item.cc', { member: 'ada', ts: '2026-11-19T00:41:00Z' }),
    ev('e3', 'comment.add', { member: 'maya', ts: '2026-11-19T00:58:00Z', payload: { body: 'hello' } }),
    ev('e4', 'item.cc', { member: 'ada', ts: '2026-11-19T01:09:00Z' }),
    ev('e5', 'item.cc', { member: 'ada', ts: '2026-11-19T01:10:00Z' }),
    ev('e6', 'item.assign', { member: 'maya', ts: '2026-11-19T01:11:00Z' }),
    ev('e7', 'item.edit', { member: 'maya', ts: '2026-11-19T01:12:00Z' }),
    ev('e8', 'item.status', { member: 'raj', ts: '2026-11-19T09:30:00Z', payload: { status: 'doing' } }),
    ev('e9', 'item.uncc', { member: 'raj', ts: '2026-11-19T09:35:00Z' })
  ];
  events.forEach(e => { c.EV[e.id] = e; });
  c.VS.items.i1.thread = events.map(e => e.id);

  const html = c.threadSectionHtml('ada', c.VS.items.i1);
  // Header shows Expand all logs because e4..e7 is collapsed by default.
  assert.match(html, /data-act="trfold-all" data-all="open"/);
  assert.match(html, /Expand all logs/);
  // Run 1 (e1..e2): expanded (no shut class), e2 has .same (same author ada), HH:MM times.
  assert.match(html, /<li class="tr-run" data-run="e1" data-n="2">/);
  assert.match(html, /<li class="tr-sl" data-ev="e1"[^>]*><span class="tr-t">00:41<\/span>/);
  assert.match(html, /<li class="tr-sl same" data-ev="e2"[^>]*><span class="tr-t">00:41<\/span><span class="tr-g cc" aria-hidden="true">\+<\/span>/);
  // Run 2 (e4..e7): collapsed (.shut), summary has count, Ada Park +1 other, CC added 2, assigned 1, edited 1, and 01:09–01:12.
  assert.match(html, /<li class="tr-run shut" data-run="e4" data-n="4">/);
  assert.match(html, /4 system events/);
  assert.match(html, /Ada Park \+1 other/);
  assert.match(html, /CC added 2, assigned 1, edited 1/);
  assert.match(html, /01:09\u201301:12/);
  // Inside Run 2: e6 changes author from ada to maya -> no .same; e7 repeats maya -> has .same and edit glyph.
  assert.match(html, /<li class="tr-sl" data-ev="e6"[^>]*><span class="tr-t">01:11<\/span><span class="tr-g as" aria-hidden="true">\u2192<\/span>/);
  assert.match(html, /<li class="tr-sl same" data-ev="e7"[^>]*><span class="tr-t">01:12<\/span><span class="tr-g ed" aria-hidden="true">\u270e<\/span>/);
  // Status break (e8): standalone .wide .st-break .tr-st-doing with full MM-DD HH:MM timestamp.
  assert.match(html, /<li class="tr-sl wide st-break tr-st-doing" data-ev="e8"[^>]*><span class="tr-t">11-19 09:30<\/span>/);
  // Lone system event (e9): standalone .wide with minus CC glyph and full timestamp.
  assert.match(html, /<li class="tr-sl wide" data-ev="e9"[^>]*><span class="tr-t">11-19 09:35<\/span><span class="tr-g cc" aria-hidden="true">\u2212<\/span>/);

  // Activity feed and wiki still call evLine(n, false) unchanged (Decision 8).
  assert.match(source, /function evLine\(n, inThread\)/);
  assert.match(source, /return x \+ evLine\(n, false\);/);
  assert.match(source, /return evLine\(n, false\);/);
});
