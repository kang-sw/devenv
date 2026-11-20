'use strict';

// Ticket map geometry, pure: drRoom's right-edge handle offsets
// (assets/dashboard-drawer.js) for every combination of present and hidden
// handles.
// Run: node --test internal/wsdashboard/testdata/ticketmap-*.test.cjs
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

const drawer = fs.readFileSync(path.resolve(__dirname, '..', 'assets', 'dashboard-drawer.js'), 'utf8');
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
