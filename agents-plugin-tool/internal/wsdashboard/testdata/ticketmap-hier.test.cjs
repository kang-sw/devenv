'use strict';

// Ticket map geometry, pure: drRoom's right-edge handle offsets
// (assets/dashboard-drawer.js) for every combination of present and hidden
// handles, and View B's layoutHier column solver
// (assets/dashboard-ticketmap-viewb.js, the block between "tm:hier-begin"
// and "tm:hier-end").
// Run: node --test internal/wsdashboard/testdata/ticketmap-*.test.cjs
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

const drawer = fs.readFileSync(path.resolve(__dirname, '..', 'assets', 'dashboard-drawer.js'), 'utf8');
const viewb = fs.readFileSync(path.resolve(__dirname, '..', 'assets', 'dashboard-ticketmap-viewb.js'), 'utf8');
function section(text, start, end) {
  const i = text.indexOf(start), j = text.indexOf(end, i + start.length);
  if (i < 0 || j < 0) throw new Error('missing section ' + start);
  return text.slice(i, j);
}

// room runs drRoom over one handle set: each width is the handle's measured
// width, or 0 for an absent one (the panels handle with no panel, no git
// region, no search handle, a hidden ticket map handle).
function room(h) {
  const props = {}, toggles = {};
  const box = width => ({ getBoundingClientRect: () => ({ width }) });
  const ctx = vm.createContext({
    drHas: h.drawer > 0, drRoomW: -1, drGhOff: -1, drFoldW: -1, drFoldGw: -1,
    drhandle: box(h.drawer), gitRegion: box(h.git),
    shandle: h.search ? box(h.search) : null,
    thandle: h.map === undefined ? null : Object.assign(box(h.map || 35), { hidden: !h.map }),
    ghandle: () => (h.git ? {} : null),
    drheader: { classList: { toggle: (k, on) => { toggles[k] = on; } } },
    doc: { documentElement: { style: { setProperty: (k, v) => { props[k] = v; } } } }
  });
  vm.runInContext(section(drawer, '\n  function drRoom() {', '\n  // drFoldShow') + '\ndrRoom();', ctx);
  return { props, room: toggles['dr-room'] };
}

// expect is an independent model: right to left, panels, search, ticket
// map, git; each present handle sits 6px left of the previous one.
function expect(h) {
  const order = [['drawer', null], ['search', '--shr'], ['map', '--thr'], ['git', '--ghr']];
  const want = { '--shr': '0px', '--thr': '0px', '--ghr': '0px' };
  let acc = 0;
  for (const [name, prop] of order) {
    const w = h[name] || 0;
    if (!w) continue;
    const at = acc ? acc + 6 : 0;
    if (prop) want[prop] = at + 'px';
    acc = at + w;
  }
  want['--drhw'] = acc + 'px';
  return want;
}

test('drRoom places every handle combination', () => {
  let n = 0;
  for (const drawerW of [0, 178]) {
    for (const search of [0, 35]) {
      for (const map of [undefined, 0, 35]) {
        for (const git of [0, 90]) {
          const h = { drawer: drawerW, search, map, git };
          const got = room(h), want = expect(h);
          const label = JSON.stringify(h);
          for (const k of ['--shr', '--thr', '--ghr', '--drhw']) assert.equal(got.props[k], want[k], label + ' ' + k);
          assert.equal(got.room, want['--drhw'] !== '0px', label + ' header room');
          n++;
        }
      }
    }
  }
  assert.equal(n, 24);
});

test('the ticket map handle sits 6px left of the search handle, and the header reserves it', () => {
  const got = room({ drawer: 178, search: 35, map: 35, git: 90 }).props;
  assert.equal(got['--shr'], '184px');
  assert.equal(got['--thr'], '225px', '178 + 6 + 35 + 6');
  assert.equal(got['--ghr'], '266px', '225 + 35 + 6');
  assert.equal(got['--drhw'], '356px');
  const hidden = room({ drawer: 178, search: 35, map: 0, git: 90 }).props;
  assert.equal(hidden['--thr'], '0px', 'a hidden handle takes no room');
  assert.equal(hidden['--ghr'], '225px', 'git follows the search handle directly');
  const noDrawer = room({ drawer: 0, search: 35, map: 35, git: 0 }).props;
  assert.equal(noDrawer['--thr'], '41px', 'with the drawer handle hidden it is still 6px left of search');
  assert.equal(noDrawer['--drhw'], '76px');
});

const hier = vm.createContext({});
vm.runInContext(section(viewb, '// tm:hier-begin', '// tm:hier-end'), hier);
// cols runs the solver; rows splits each group's n cards into its rows.
function cols(W1, ns) {
  const r = JSON.parse(JSON.stringify(hier.tmHierCols(W1, ns)));
  r.rows = ns.map((n, ix) => {
    const out = [], c = r.cols[ix];
    for (let left = Math.min(n, 99); left > 0; left -= c) out.push(Math.min(c, left));
    return out;
  });
  return r;
}
// width is the tier's width at the solved columns.
const width = r => r.cols.reduce((s, k) => s + k * r.cw + (k - 1) * 16, 0) + 40 * (r.cols.length - 1);

test('layoutHier balances five cards as 3+2, never 4+1', () => {
  const r = cols(1000, [5]);
  assert.equal(r.cw, 232);
  assert.deepEqual(r.rows, [[3, 2]], 'four columns fit 1000px, rebalanced to 3+2');
  assert.deepEqual(cols(1600, [5]).rows, [[5]], 'all five fit one row at 1600px');
  assert.deepEqual(cols(560, [5]).rows, [[2, 2, 1]], 'only two columns fit 560px: the rebalance never adds one');
});

test('layoutHier lays out an epic with 11 children at W1 = 1200 and 800', () => {
  // Collapsed: 8 shown behind the +3 toggle.
  let r = cols(1200, [8]);
  assert.deepEqual(r.rows, [[4, 4]]);
  assert.ok(width(r) <= 1200);
  r = cols(1200, [11]);
  assert.deepEqual(r.rows, [[4, 4, 3]]);
  r = cols(800, [8]);
  assert.deepEqual(r.rows, [[3, 3, 2]]);
  assert.ok(width(r) <= 800);
  r = cols(800, [11]);
  assert.deepEqual(r.rows, [[3, 3, 3, 2]]);
  // Two groups share the tier: the group with the most rows grows first.
  r = cols(1200, [8, 2]);
  assert.equal(r.cw, 232);
  assert.ok(width(r) <= 1200, 'fits: ' + JSON.stringify(r));
  assert.deepEqual(r.rows, [[3, 3, 2], [1, 1]]);
});

test('layoutHier narrows cards to share a tight column and caps n at 99', () => {
  const r = cols(300, [1, 1]);
  assert.equal(r.cw, 130, 'floor((300 - 40) / 2)');
  assert.deepEqual(r.cols, [1, 1]);
  const big = cols(1200, [150]);
  assert.deepEqual(big.cols, [4]);
  assert.equal(big.rows[0].length, 25, '99 cards in 4 columns');
  assert.deepEqual(cols(1200, [150]), cols(1200, [99]));
  assert.deepEqual(cols(1200, [0]).cols, [1], 'an empty group keeps one column');
});
