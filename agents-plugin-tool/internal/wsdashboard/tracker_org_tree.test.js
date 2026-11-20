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

const ctx = {};
vm.createContext(ctx);
vm.runInContext(section('  function orgTree(', '  // Live, doing or blocked'), ctx);
const orgTree = (members, leadsOverride) => JSON.parse(JSON.stringify(ctx.orgTree(members, leadsOverride)));

test('orgTree nests a 3-level forest with subtree counts and unplaced members', () => {
  const members = {
    'kim-hn': { leads: [], children: ['lee-dy', 'park-sy'], removed: false },
    'lee-dy': { leads: ['kim-hn'], children: ['choi-jw', 'jung-mj', 'kang-yr'], removed: false },
    'choi-jw': { leads: ['lee-dy'], children: [], removed: false },
    'jung-mj': { leads: ['lee-dy'], children: [], removed: false },
    'kang-yr': { leads: ['lee-dy'], children: [], removed: false },
    'park-sy': { leads: ['kim-hn'], children: ['yoon-th', 'han-sa'], removed: false },
    'yoon-th': { leads: ['park-sy'], children: [], removed: false },
    'han-sa': { leads: ['park-sy'], children: [], removed: false },
    'oh-sj': { leads: [], children: [], removed: false },
    'song-hj': { leads: ['kim-hn'], children: [], removed: true },
  };
  const ot = orgTree(members);
  assert.equal(ot.roots.length, 1);
  assert.equal(ot.roots[0].id, 'kim-hn');
  assert.equal(ot.roots[0].depth, 0);
  assert.equal(ot.roots[0].count, 7);
  assert.deepEqual(ot.roots[0].children.map(n => [n.id, n.depth, n.count]), [
    ['lee-dy', 1, 3],
    ['park-sy', 1, 2],
  ]);
  assert.deepEqual(ot.roots[0].children[0].children.map(n => [n.id, n.depth, n.count]), [
    ['choi-jw', 2, 0],
    ['jung-mj', 2, 0],
    ['kang-yr', 2, 0],
  ]);
  assert.deepEqual(ot.unplaced, ['oh-sj']);
  assert.deepEqual(ot.removed, ['song-hj']);
  assert.deepEqual(ot.unreachable, []);
  assert.equal(ot.depthOf['kim-hn'], 0);
  assert.equal(ot.depthOf['lee-dy'], 1);
  assert.equal(ot.depthOf['choi-jw'], 2);
  assert.equal(ot.depthOf['oh-sj'], 0);
  assert.equal(ot.countOf['kim-hn'], 7);
  assert.equal(ot.countOf['lee-dy'], 3);
  assert.equal(ot.countOf['oh-sj'], 0);
});

test('orgTree handles removed leads, unplaced rule, and cycles', () => {
  const members = {
    boss: { leads: [], removed: true },
    solo: { leads: ['boss'], removed: false }, // removed lead + no followers -> unplaced
    mgr: { leads: ['boss'], removed: false },  // removed lead + follower -> tree root
    rep: { leads: ['mgr'], removed: false },
    cycA: { leads: ['cycB'], removed: false },
    cycB: { leads: ['cycA'], removed: false },
    self: { leads: ['self'], removed: false },
  };
  const ot = orgTree(members);
  assert.deepEqual(ot.roots.map(r => [r.id, r.count]), [['mgr', 1]]);
  assert.deepEqual(ot.unplaced, ['solo']);
  assert.deepEqual(ot.removed, ['boss']);
  assert.deepEqual(ot.unreachable, ['cycA', 'cycB', 'self']);
});

test('orgTree applies leadsOverride for projected editor trees', () => {
  const members = {
    a: { leads: [], children: ['b'], removed: false },
    b: { leads: ['a'], children: [], removed: false },
    c: { leads: [], children: [], removed: false },
  };
  // Promote b to root, move a under b, place c under a
  const ot = orgTree(members, { b: '', a: 'b', c: 'a' });
  assert.equal(ot.roots.length, 1);
  assert.equal(ot.roots[0].id, 'b');
  assert.equal(ot.roots[0].count, 2);
  assert.equal(ot.roots[0].children[0].id, 'a');
  assert.equal(ot.roots[0].children[0].count, 1);
  assert.equal(ot.roots[0].children[0].children[0].id, 'c');
  assert.deepEqual(ot.unplaced, []);
  assert.deepEqual(ot.depthOf, { b: 0, a: 1, c: 2 });
  // Clear b's lead and leave everyone unplaced
  const flat = orgTree(members, { b: '' });
  assert.deepEqual(flat.roots, []);
  assert.deepEqual(flat.unplaced, ['a', 'b', 'c']);
});
