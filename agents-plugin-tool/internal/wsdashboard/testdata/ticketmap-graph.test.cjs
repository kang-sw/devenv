'use strict';

// Ticket map View C geometry, pure (assets/dashboard-ticketmap-geom.js,
// loaded whole through vm): the seeded layout's determinism, containment
// and iteration scaling, the camera's zoomAt and wheel math, the soup's
// candidate offsets, the convex hull and point-in-polygon test on
// degenerate input, the tooltip clamp, the Ego Ring geometry, the label
// planner, the similarity-edge pick and the pointer hit resolver.
// Run: node --test internal/wsdashboard/testdata/ticketmap-*.test.cjs
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

const src = fs.readFileSync(path.resolve(__dirname, '..', 'assets', 'dashboard-ticketmap-geom.js'), 'utf8');
const ctx = vm.createContext({ ticketMap: {} });
vm.runInContext(src, ctx);
const geo = ctx.ticketMap.geom;
// plain re-creates a value from the vm's realm in this one, so deepEqual
// compares structure, not prototypes.
const plain = v => JSON.parse(JSON.stringify(v));

// board builds a synthetic constellation: K clusters of m stars, a chain of
// explicit links inside each cluster, one related link across neighbouring
// clusters, and similarity lists from a fixed formula.
function board(K, m) {
  const nodes = [];
  for (let k = 0; k < K; k++) for (let a = 0; a < m; a++) nodes.push({ k, r: geo.radius(['ready', 'todo', 'idea'][a % 3], a % 5, a === 0), x: [], sim: [] });
  const link = (i, j, w) => { nodes[i].x.push([j, w]); nodes[j].x.push([i, w]); };
  for (let k = 0; k < K; k++) {
    for (let a = 1; a < m; a++) link(k * m + a - 1, k * m + a, 1);
    if (k) link((k - 1) * m, k * m, 0.5);
  }
  nodes.forEach((n, i) => {
    n.sim = nodes.map((o, j) => [j, i === j ? 0 : Math.round(1000 * (o.k === n.k ? 0.5 - Math.abs(i - j) * 0.03 : 0.12 + ((i * 7 + j * 3) % 10) / 100)) / 1000])
      .filter(s => s[1] > 0.08).sort((a, b) => b[1] - a[1] || a[0] - b[0]).slice(0, 25);
  });
  return nodes;
}

test('the layout is deterministic for a seed and differs across seeds', () => {
  const nodes = board(6, 14);
  const a = geo.layout(nodes, 6, 42), b = geo.layout(nodes, 6, 42), c = geo.layout(nodes, 6, 7);
  assert.equal(a.x.length, nodes.length);
  assert.deepEqual(Array.from(a.x), Array.from(b.x));
  assert.deepEqual(Array.from(a.y), Array.from(b.y));
  assert.deepEqual(a.cx, b.cx);
  for (let i = 0; i < nodes.length; i++) assert.ok(Number.isFinite(a.x[i]) && Number.isFinite(a.y[i]), 'finite ' + i);
  assert.notDeepEqual(Array.from(a.x), Array.from(c.x));
  // The rng is mulberry32: the first draws for seed 42 (the v5 reference's).
  const r = geo.rng(42);
  assert.deepEqual([r(), r(), r()].map(v => v.toFixed(9)), ['0.601103752', '0.448290559', '0.852465793']);
});

test('every star settles inside its cluster: within CR - r - 10 of the center', () => {
  for (const [K, m] of [[1, 1], [1, 2], [3, 9], [8, 20]]) {
    const nodes = board(K, m), L = geo.layout(nodes, K, 42);
    const count = new Array(K).fill(0);
    nodes.forEach(n => count[n.k]++);
    nodes.forEach((n, i) => {
      const lim = geo.clusterRadius(count[n.k]) - n.r - 10, d = Math.hypot(L.x[i] - L.cx[n.k], L.y[i] - L.cy[n.k]);
      assert.equal(L.cr[n.k], 34 + 30 * Math.sqrt(count[n.k]));
      assert.ok(d <= lim + 1e-9, `K=${K} m=${m} star ${i}: ${d} > ${lim}`);
    });
  }
});

test('star radius and iteration scaling follow the formulas', () => {
  assert.equal(geo.radius('ready', 0, false), 7.5);
  assert.equal(geo.radius('todo', 4, false), 6.5 + Math.min(5, 2 * 1.1));
  assert.equal(geo.radius('idea', 100, true), 5.5 + 5 + 2.5);
  assert.equal(geo.radius('done', 9, true), 4.5);
  assert.equal(geo.radius('dropped', 0, false), 4.5);
  for (const N of [0, 1, 179, 300, 400]) assert.equal(geo.iterations(N), 420, 'N=' + N);
  for (const N of [401, 500, 800, 1000, 5000]) assert.equal(geo.iterations(N), Math.max(120, Math.floor(420 * Math.pow(400 / N, 2))), 'N=' + N);
  assert.equal(geo.iterations(500), 268);
  assert.equal(geo.iterations(2000), 120);
});

test('zoomAt keeps the world point under the anchor fixed and clamps k', () => {
  const cams = [{ x: 0, y: 0, k: 1 }, { x: -413.25, y: 287.5, k: 0.37 }, { x: 1200.125, y: -66.75, k: 2.9 }];
  const anchors = [[0, 0], [800, 450], [1599, 959], [13.5, 707.25]];
  const deltas = [[-120, 0], [120, 0], [3, 1], [-3, 1], [1, 2], [-1, 2], [5000, 0], [-5000, 0]];
  for (const cam of cams) for (const [mx, my] of anchors) for (const [dy, mode] of deltas) for (const ctrl of [false, true]) {
    const d = geo.wheelDy(dy, mode), k2 = geo.wheelK(cam.k, d, ctrl), next = geo.zoomAt(cam, mx, my, k2);
    const wx = (mx - cam.x) / cam.k, wy = (my - cam.y) / cam.k;
    assert.ok(Math.abs(d) <= 320);
    assert.ok(next.k >= 0.18 && next.k <= 3.8, 'k clamped ' + next.k);
    assert.equal(next.k, Math.min(3.8, Math.max(0.18, k2)));
    assert.ok(Math.hypot(wx * next.k + next.x - mx, wy * next.k + next.y - my) < 1e-9, `drift at ${mx},${my} mode ${mode}`);
  }
  assert.equal(geo.wheelDy(3, 1), 96);
  assert.equal(geo.wheelDy(1, 2), 320);
  assert.equal(geo.wheelDy(-2000, 0), -320);
  assert.ok(geo.wheelK(1, -120, false) > 1.3);
  assert.ok(1 / geo.wheelK(1, 96, false) > 1.25);
  assert.equal(geo.zoomAt({ x: 5, y: 5, k: 1 }, 0, 0, 100).k, 3.8);
  assert.equal(geo.zoomAt({ x: 5, y: 5, k: 1 }, 0, 0, 0.001).k, 0.18);
});

test('the soup offsets run nearest first inside the ellipse, at most 220', () => {
  for (const [w, h] of [[30, 14], [64, 18], [120, 21], [8, 4]]) {
    const o = geo.soupOffsets(w, h), mx = w * 0.8 + 24, my = h * 2.6, cut = Math.max(mx, my * 1.35);
    assert.ok(o.length > 0 && o.length <= 220, `${w}x${h}: ${o.length}`);
    assert.ok(Math.abs(o[0][0]) <= 3 && Math.abs(o[0][1]) <= h * 0.25, 'the nearest grid point leads');
    for (let i = 0; i < o.length; i++) {
      const [dx, dy, d] = o[i];
      assert.equal(d, Math.hypot(dx, dy * 1.35));
      assert.ok(d <= cut + 1e-9 && Math.abs(dx) <= mx + 1e-6 && Math.abs(dy) <= my + 1e-6);
      if (i) assert.ok(o[i - 1][2] <= d, 'sorted');
    }
  }
  assert.equal(geo.soupOffsets(120, 21).length, 220);
});

test('the convex hull and inPoly handle degenerate input', () => {
  const sq = geo.convexHull([[0, 0], [10, 0], [10, 10], [0, 10], [5, 5], [5, 5], [0, 0], [10, 10]]);
  assert.equal(sq.length, 4);
  assert.ok(geo.inPoly(sq, 5, 5) && geo.inPoly(sq, 0.1, 9.9));
  assert.ok(!geo.inPoly(sq, -0.1, 5) && !geo.inPoly(sq, 5, 10.1) && !geo.inPoly(sq, 20, 20));
  // Collinear points keep only their ends; nothing is inside a segment.
  const line = geo.convexHull([[0, 0], [1, 1], [2, 2], [3, 3], [2, 2]]);
  assert.deepEqual(plain(line), [[0, 0], [3, 3]]);
  assert.ok(!geo.inPoly(line, 1, 1) && !geo.inPoly(line, 1.5, 1));
  // Duplicates of one point stay one point.
  assert.deepEqual(plain(geo.convexHull([[4, 4], [4, 4], [4, 4]])), [[4, 4]]);
  assert.deepEqual(plain(geo.convexHull([])), []);
  assert.ok(!geo.inPoly([], 0, 0) && !geo.inPoly([[1, 1]], 1, 1));
  // A star's 16 hull points at r + 20 make a hull around it with its top.
  const h = geo.convexHull(geo.ring(100, 50, 6, []));
  assert.equal(h.length, 16);
  assert.ok(geo.inPoly(h, 100, 50) && !geo.inPoly(h, 100, 50 + 27));
  const top = geo.hullTop(h);
  assert.ok(Math.abs(top.x - 100) < 1e-6 && Math.abs(top.y - 24) < 1e-6 && Math.abs(top.by - 76) < 1e-6 && Math.abs(top.w - 52) < 1e-6, JSON.stringify(top));
  assert.match(geo.hullPath(h), /^M[-\d.]+,[-\d.]+ Q.*Z$/);
});

test('the tooltip flips and clamps in all four corners', () => {
  const W = 1000, H = 700, w = 260, h = 120;
  const inside = p => p.x >= 12 && p.y >= 12 && p.x + w <= W - 12 && p.y + h <= H - 12;
  let p = geo.tipPos(10, 10, w, h, W, H);
  assert.deepEqual([p.x, p.y], [26, 24]);
  p = geo.tipPos(990, 10, w, h, W, H);
  assert.deepEqual([p.x, p.y], [990 - w - 14, 24]);
  p = geo.tipPos(10, 690, w, h, W, H);
  assert.deepEqual([p.x, p.y], [26, 690 - h - 12]);
  p = geo.tipPos(990, 690, w, h, W, H);
  assert.deepEqual([p.x, p.y], [990 - w - 14, 690 - h - 12]);
  for (const [mx, my] of [[0, 0], [W, 0], [0, H], [W, H], [W / 2, H / 2], [W - 1, 5], [3, H - 1]]) assert.ok(inside(geo.tipPos(mx, my, w, h, W, H)), `${mx},${my}`);
  // A tip larger than the host pins to the top-left margin.
  p = geo.tipPos(500, 500, 2000, 900, W, H);
  assert.deepEqual([p.x, p.y], [12, 12]);
});

test('the Ego Ring balances its columns and places every member on its ellipse', () => {
  const c0 = { x: 40, y: -20 };
  for (const W of [900, 1260, 1600]) {
    for (let up = 0; up <= 20; up += 4) for (let down = 0; down + up <= 20; down += 3) for (let rel = 0; rel + up + down <= 20; rel += 5) for (const ns of [0, 1, 10]) {
      let j = 1;
      const take = n => Array.from({ length: n }, () => j++);
      const m = { i: 0, up: take(up), down: take(down), rel: take(rel), sim: take(ns).map((k, ix) => [k, 0.5 - ix / 100]) };
      const g = geo.ego(m, W, c0);
      const R1 = Math.min(190, Math.max(90, W * 0.1));
      assert.equal(g.R1, R1);
      assert.equal(g.pos.size, 1 + up + down + rel + ns);
      assert.deepEqual(plain(g.pos.get(0)), c0);
      const left = g.left.filter(x => x !== null), right = g.right.filter(x => x !== null);
      assert.equal(left.length + right.length, up + down + rel);
      // Up heads the left column and down ends the right one; related
      // tickets fill the shorter column first.
      assert.deepEqual(plain(left.slice(0, up)), m.up);
      assert.deepEqual(plain(right.slice(right.length - down)), m.down);
      if (rel >= Math.abs(up - down)) assert.ok(Math.abs(left.length - right.length) <= 1, `balance ${up}/${down}/${rel}: ${left.length} vs ${right.length}`);
      assert.equal(g.rows, Math.max(g.left.length, g.right.length));
      for (const [col, sign] of [[g.left, -1], [g.right, 1]]) {
        col.forEach((k, ix) => {
          if (k === null) return;
          const p = g.pos.get(k), y = (ix - (col.length - 1) / 2) * 32;
          assert.ok(Math.abs(p.y - c0.y - y) < 1e-9);
          assert.ok(Math.abs(p.x - c0.x - sign * R1 * Math.sqrt(Math.max(0.12, 1 - (y / g.Ry1) ** 2))) < 1e-9);
          assert.equal(g.side.get(k), sign < 0 ? 'l' : 'r');
          assert.ok(Math.abs(y) < g.Ry1);
        });
      }
      m.sim.forEach(([k], ix) => {
        const p = g.pos.get(k);
        assert.equal(g.side.get(k), ix % 2 ? 'l' : 'r');
        assert.ok(Math.abs(Math.abs(p.x - c0.x) - g.R2 * Math.sqrt(Math.max(0.12, 1 - ((p.y - c0.y) / g.Ry2) ** 2))) < 1e-9);
        assert.equal(g.pct.get(k), Math.round(m.sim[ix][1] * 100));
      });
      assert.ok(g.Ry2 >= g.Ry1 + 70 && g.R2 > g.R1);
      assert.equal(g.sect[0] === null, up === 0);
      assert.equal(g.sect[1] === null, down === 0);
      assert.equal(g.sect[4] === null, ns === 0);
    }
  }
});

test('the label planner never overlaps labels, controls or the viewport edge', () => {
  const text = (s, wt, size) => s.length * (size || 12) * 0.55 + (wt > 500 ? 2 : 0);
  const stars = [];
  for (let i = 0; i < 60; i++) stars.push({ i, x: 40 + (i % 10) * 70, y: 60 + Math.floor(i / 10) * 48, r: 5, p: i === 0 ? 1e7 : 1e4 - i, title: 'Ticket title number ' + i, framed: i === 0, pre: '', side: 'r', inner: false, skip: i % 7 === 3 });
  const poly = [[0, 0], [800, 0], [800, 400], [0, 400]];
  const plan = geo.planLabels({
    W: 800, H: 400, k: 1, cx: 0, cy: 0, ego: null, soupFirst: true, text, maxLen: 46,
    blocks: [{ x: 0, y: 0, w: 300, h: 40 }], obst: stars, stars,
    heads: [{ id: 0, x: 400, y: 30, by: 380, full: 'Alpha · Beta · Gamma', short: 'Alpha', cnt: 13 }],
    sects: [], soup: [0, 1, 2, 3, 4, 5, 6, 7, 8, 9].map(j => ({ j, cl: 0, x: 400, y: 200, w: 50, h: 16, poly }))
  });
  const boxes = [...plan.stars.values()].map(s => s.b).concat([...plan.heads.values()].map(h => h.b), [...plan.soup.values()], [{ x: 0, y: 0, w: 300, h: 40 }]);
  for (let a = 0; a < boxes.length; a++) for (let b = a + 1; b < boxes.length; b++) {
    const p = boxes[a], q = boxes[b];
    assert.ok(!(p.x < q.x + q.w && p.x + p.w > q.x && p.y < q.y + q.h && p.y + p.h > q.y), 'overlap ' + a + ' ' + b);
  }
  for (const b of boxes.slice(0, -1)) assert.ok(b.x >= 0 && b.y >= 0 && b.x + b.w <= 800 && b.y + b.h <= 400);
  assert.ok(plan.stars.has(0) && !plan.stars.has(1), 'the framed star places first, its neighbour yields');
  assert.ok(!plan.stars.has(3) && !plan.stars.has(10), 'skipped stars stay unlabelled');
  assert.equal(plan.soup.size, 8, 'at most 8 soup words per cluster');
  // An ego caption anchored past the right edge is clamped 4px inside.
  const cap = geo.planLabels({ W: 800, H: 400, k: 1, cx: 0, cy: 0, ego: null, soupFirst: true, text, maxLen: 46, blocks: [], obst: [], stars: [], heads: [], soup: [],
    sects: [{ x: 790, y: 395, text: 'Semantically similar (body %)' }] }).sects.get(0);
  assert.ok(cap && cap.x + cap.w === 796 && cap.y + cap.h === 396, 'clamped caption ' + JSON.stringify(cap));
  assert.equal(geo.fitText('abcdefghij', 1e9, 6, s => s.length), 'abcde…');
  assert.equal(geo.fitText('abcdefghij', 7, 46, s => s.length), 'abcdef…');
});

test('similarity edges: most similar first, at most 3 per star, at or above the threshold', () => {
  const lists = [];
  for (let i = 0; i < 8; i++) lists.push([]);
  const add = (a, b, s) => { lists[a].push({ j: b, s, terms: [] }); lists[b].push({ j: a, s, terms: [] }); };
  [[0, 1, 0.9], [0, 2, 0.8], [0, 3, 0.7], [0, 4, 0.6], [1, 2, 0.5], [5, 6, 0.23], [6, 7, 0.24]].forEach(p => add(...p));
  const got = plain(geo.simPick(lists, 0.24).map(p => [p[1], p[2], p[0]]));
  assert.deepEqual(got, [[0, 1, 0.9], [0, 2, 0.8], [0, 3, 0.7], [1, 2, 0.5], [6, 7, 0.24]]);
});

test('bounds, centroid, fit and the semantic zoom stages', () => {
  const P = [{ x: 0, y: 0 }, { x: 100, y: 50 }, null, { x: -40, y: 10 }];
  assert.deepEqual(plain(geo.bounds(P, i => i !== 3)), { x0: -30, y0: -40, x1: 130, y1: 80 });
  assert.equal(geo.bounds(P, () => false), null);
  const c = geo.centroid([0, 1, 3], [0.5, 0.1, 0.02], P, i => i !== 3);
  assert.equal(c.n, 2);
  assert.ok(Math.abs(c.x - 100 * 0.15 / 0.65) < 1e-9);
  const one = geo.fitCam({ x0: -30, y0: -40, x1: 30, y1: 30 }, 1600, 900, 64, 54, 1.6);
  assert.equal(one.k, 1.6, 'one star fits at maxK');
  assert.deepEqual([geo.stage(0.5, false), geo.stage(0.85, false), geo.stage(1.34, false), geo.stage(1.35, false), geo.stage(0.3, true)], [1, 2, 2, 3, 3]);
  assert.equal(geo.soupAlpha(0.8, false, false), 1);
  assert.ok(Math.abs(geo.soupAlpha(1.0, false, false) - 0.625) < 1e-12);
  assert.ok(Math.abs(geo.soupAlpha(1.0, false, true) - 0.625 * 0.45) < 1e-12);
  assert.equal(geo.soupAlpha(1.4, false, false), 0);
  assert.equal(geo.soupAlpha(0.2, true, false), 0);
  assert.equal(geo.nodeScale(1), 1);
  assert.equal(geo.nodeScale(0.1), 1.9);
  assert.deepEqual([0, 0.5, 1].map(nw => geo.soupFont(nw)), [11, 14, 16]);
});

test('the hit resolver lets a star under a soup word win', () => {
  const soup = { type: 'soup', j: 4 }, node = { type: 'node', i: 9 }, label = { type: 'label', i: 2 }, edge = { type: 'edge' }, bg = { type: 'bg' };
  assert.equal(geo.resolveHit(soup, [soup, label, node, bg]), node);
  assert.equal(geo.resolveHit(soup, [soup, bg]), soup);
  assert.equal(geo.resolveHit(label, [label, node]), label);
  assert.equal(geo.resolveHit(edge, [edge, node]), edge);
  assert.equal(geo.resolveHit(bg, []), bg);
});
