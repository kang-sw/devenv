  // --- ticket map View C geometry: pure, no DOM ---------------------------
  // The constellation's deterministic math, kept apart from the view
  // (dashboard-ticketmap-graph.js, right after this file) so the node test
  // (testdata/ticketmap-graph.test.cjs) loads this whole file through vm:
  // the seeded rng, the cluster-center and node force layout and its
  // iteration scaling, the convex hull with its smoothed path, top and
  // point-in-polygon test, the soup's candidate offsets and centroid, the
  // label spatial hash and planner (text width is injected), the
  // similarity-edge pick, the tooltip clamp, the camera math (wheel,
  // zoomAt, fit), the Ego Ring geometry, and the pointer hit resolver.
  ticketMap.geom = (function () {
    var K_MIN = 0.18, K_MAX = 3.8, TAU = 6.2832;
    function clamp(v, lo, hi) { return Math.max(lo, Math.min(hi, v)); }
    // rng is mulberry32: the same seed gives the same sequence.
    function rng(seed) {
      return function () {
        seed |= 0;
        seed = seed + 0x6D2B79F5 | 0;
        var t = Math.imul(seed ^ seed >>> 15, 1 | seed);
        t = t + Math.imul(t ^ t >>> 7, 61 | t) ^ t;
        return ((t ^ t >>> 14) >>> 0) / 4294967296;
      };
    }
    // radius is a star's radius: status, degree and epic for open
    // tickets, a fixed 4.5 for closed refs.
    var R_ST = { ready: 7.5, todo: 6.5, idea: 5.5 };
    function radius(status, deg, epic) {
      if (!R_ST[status]) return 4.5;
      return R_ST[status] + Math.min(5, Math.sqrt(deg || 0) * 1.1) + (epic ? 2.5 : 0);
    }
    function clusterRadius(count) { return 34 + 30 * Math.sqrt(count); }
    // iterations scales the node simulation down past 400 stars.
    function iterations(N) { return N <= 400 ? 420 : Math.max(120, Math.floor(420 * Math.pow(400 / N, 2))); }

    // layout settles the constellation. nodes[i] = {k: cluster index in
    // [0, K), r: radius, x: [[j, w]] explicit links (w 1, or .5 for
    // related-only), sim: [[j, s]] similar tickets, most similar first}.
    // It returns the settled positions x, y and the cluster centers cx, cy
    // and radii cr. The same input and seed give bitwise-equal output.
    function layout(nodes, K, seed) {
      var rnd = rng(seed), N = nodes.length, a, b, it;
      var cn = new Array(K).fill(0);
      nodes.forEach(function (n) { cn[n.k]++; });
      var cr = cn.map(clusterRadius);
      // Inter-cluster affinity: explicit cross-cluster links, plus twice
      // the score of each top-3 similarity of .2 or more.
      var Wc = [];
      for (a = 0; a < K; a++) Wc.push(new Array(K).fill(0));
      nodes.forEach(function (n) {
        n.x.forEach(function (l) { if (nodes[l[0]].k !== n.k) Wc[n.k][nodes[l[0]].k] += 1; });
        n.sim.slice(0, 3).forEach(function (s) { if (s[1] >= 0.2 && nodes[s[0]].k !== n.k) Wc[n.k][nodes[s[0]].k] += s[1] * 2; });
      });
      // Cluster centers: biggest first on a golden-angle spiral, then 900
      // rounds of overlap separation and affinity pull, damped flatter in y.
      var cx = new Array(K).fill(0), cy = new Array(K).fill(0);
      var ord = [];
      for (a = 0; a < K; a++) ord.push(a);
      ord.sort(function (p, q) { return cn[q] - cn[p] || p - q; });
      ord.forEach(function (c, ix) {
        var ang = ix * 2.39996, rr = 60 * Math.sqrt(ix + 0.5) * 3;
        cx[c] = Math.cos(ang) * rr * 1.7;
        cy[c] = Math.sin(ang) * rr * 0.8;
      });
      for (it = 0; it < 900; it++) {
        var al = 1 - it / 900;
        for (a = 0; a < K; a++) {
          for (b = a + 1; b < K; b++) {
            var dx = cx[b] - cx[a], dy = cy[b] - cy[a], d = Math.hypot(dx, dy) || 0.01, minD = cr[a] + cr[b] + 64, f;
            if (d < minD) {
              f = (minD - d) / d * 0.5;
              cx[a] -= dx * f; cy[a] -= dy * f; cx[b] += dx * f; cy[b] += dy * f;
            } else if (Wc[a][b] + Wc[b][a] > 0) {
              f = Math.min(0.04, (Wc[a][b] + Wc[b][a]) * 0.0025) * al * (d - minD) / d;
              cx[a] += dx * f; cy[a] += dy * f; cx[b] -= dx * f; cy[b] -= dy * f;
            }
          }
        }
        for (a = 0; a < K; a++) { cx[a] *= 1 - 0.006 * al; cy[a] *= 1 - 0.022 * al; }
      }
      // Nodes start at a random spot inside their cluster.
      var X = new Float64Array(N), Y = new Float64Array(N), Rr = new Float64Array(N), Kc = new Int32Array(N);
      nodes.forEach(function (n, i) {
        var ang = rnd() * Math.PI * 2, rr = Math.sqrt(rnd()) * cr[n.k] * 0.7;
        X[i] = cx[n.k] + Math.cos(ang) * rr;
        Y[i] = cy[n.k] + Math.sin(ang) * rr;
        Rr[i] = n.r;
        Kc[i] = n.k;
      });
      // Springs: each explicit pair once, then top-3 similarities of .2 or
      // more not already linked.
      var la = [], lb = [], lw = [], seen = new Set();
      function link(i, j, w) {
        var key = Math.min(i, j) * N + Math.max(i, j);
        if (i === j || seen.has(key)) return;
        seen.add(key);
        la.push(i); lb.push(j); lw.push(w);
      }
      nodes.forEach(function (n, i) {
        n.x.forEach(function (l) { link(i, l[0], l[1]); });
        n.sim.slice(0, 3).forEach(function (s) { if (s[1] >= 0.2) link(i, s[0], s[1]); });
      });
      settle(X, Y, Rr, Kc, la, lb, lw, cx, cy, cr, iterations(N), rnd);
      return { x: X, y: Y, cx: cx, cy: cy, cr: cr };
    }

    // settle runs the node simulation in place: pair repulsion (skipped
    // across clusters past 120px), the springs, gravity to the cluster
    // center and containment in CR - r - 10. It declares no closure, so
    // V8 keeps its hot loop's arrays in registers.
    function settle(X, Y, Rr, Kc, la, lb, lw, cx, cy, cr, IT, rnd) {
      var N = X.length, L = la.length, it, a, b, k;
      for (it = 0; it < IT; it++) {
        var al2 = 0.35 + 0.65 * (1 - it / IT);
        for (a = 0; a < N; a++) {
          var xa = X[a], ya = Y[a], ka = Kc[a], ra = Rr[a] + 30;
          for (b = a + 1; b < N; b++) {
            var ex = X[b] - xa, ey = Y[b] - ya, d2 = ex * ex + ey * ey;
            if (d2 < 0.01) { ex = rnd() - 0.5; ey = rnd() - 0.5; d2 = 0.5; }
            var same = ka === Kc[b];
            if (!same && d2 > 14400) continue;
            var dd = Math.sqrt(d2), mag = (same ? 2600 : 1200) / d2 * al2, mn = ra + Rr[b];
            if (dd < mn) mag += (mn - dd) * 0.5;
            if (mag > 30) mag = 30;
            var ux = ex / dd * mag * 0.5, uy = ey / dd * mag * 0.5;
            xa -= ux; ya -= uy;
            X[b] += ux; Y[b] += uy;
          }
          X[a] = xa; Y[a] = ya;
        }
        for (k = 0; k < L; k++) {
          var p = la[k], q = lb[k], sm = Kc[p] === Kc[q];
          var sx = X[q] - X[p], sy = Y[q] - Y[p], sd = Math.hypot(sx, sy) || 0.01;
          var sf = (sd - (sm ? 58 : 140)) / sd * (sm ? 0.045 : 0.006) * lw[k] * al2;
          X[p] += sx * sf; Y[p] += sy * sf; X[q] -= sx * sf; Y[q] -= sy * sf;
        }
        for (a = 0; a < N; a++) {
          var c = Kc[a];
          X[a] += (cx[c] - X[a]) * 0.012 * al2;
          Y[a] += (cy[c] - Y[a]) * 0.012 * al2;
          var gx = X[a] - cx[c], gy = Y[a] - cy[c], gd = Math.hypot(gx, gy), lim = cr[c] - Rr[a] - 10;
          if (gd > lim) { X[a] = cx[c] + gx / gd * lim; Y[a] = cy[c] + gy / gd * lim; }
        }
      }
    }

    // --- hulls -------------------------------------------------------------
    // ring is the 16 hull points around a star at r + 20.
    function ring(x, y, r, out) {
      var pad = r + 20;
      for (var a = 0; a < 16; a++) out.push([x + Math.cos(a / 16 * TAU) * pad, y + Math.sin(a / 16 * TAU) * pad]);
      return out;
    }
    // convexHull is the monotone chain over the distinct points: collinear
    // points keep only the two ends, a single point stays one point.
    function convexHull(pts) {
      var s = pts.slice().sort(function (p, q) { return p[0] - q[0] || p[1] - q[1]; }), u = [];
      s.forEach(function (p) { var l = u[u.length - 1]; if (!l || l[0] !== p[0] || l[1] !== p[1]) u.push(p); });
      if (u.length < 3) return u;
      function cr(o, a, b) { return (a[0] - o[0]) * (b[1] - o[1]) - (a[1] - o[1]) * (b[0] - o[0]); }
      var lo = [], up = [], i;
      for (i = 0; i < u.length; i++) {
        while (lo.length >= 2 && cr(lo[lo.length - 2], lo[lo.length - 1], u[i]) <= 0) lo.pop();
        lo.push(u[i]);
      }
      for (i = u.length - 1; i >= 0; i--) {
        while (up.length >= 2 && cr(up[up.length - 2], up[up.length - 1], u[i]) <= 0) up.pop();
        up.push(u[i]);
      }
      up.pop(); lo.pop();
      return lo.concat(up);
    }
    // hullPath smooths the hull: quadratic curves through the midpoints,
    // the corners as control points.
    function hullPath(h) {
      function f(v) { return v.toFixed(1); }
      var d = "", n = h.length;
      for (var i = 0; i < n; i++) {
        var p = h[i], q = h[(i + 1) % n];
        d += (i ? " Q" : "M" + f((h[n - 1][0] + p[0]) / 2) + "," + f((h[n - 1][1] + p[1]) / 2) + " Q") + f(p[0]) + "," + f(p[1]) + " " + f((p[0] + q[0]) / 2) + "," + f((p[1] + q[1]) / 2);
      }
      return d + "Z";
    }
    // hullTop is where a cluster header sits: the center x, top and bottom
    // y, and the width.
    function hullTop(h) {
      var x0 = Infinity, x1 = -Infinity, y0 = Infinity, y1 = -Infinity;
      h.forEach(function (p) { x0 = Math.min(x0, p[0]); x1 = Math.max(x1, p[0]); y0 = Math.min(y0, p[1]); y1 = Math.max(y1, p[1]); });
      return { x: (x0 + x1) / 2, y: y0, by: y1, w: x1 - x0 };
    }
    function inPoly(poly, x, y) {
      var ins = false;
      for (var i = 0, j = poly.length - 1; i < poly.length; j = i++) {
        var xi = poly[i][0], yi = poly[i][1], xj = poly[j][0], yj = poly[j][1];
        if ((yi > y) !== (yj > y) && x < (xj - xi) * (y - yi) / (yj - yi) + xi) ins = !ins;
      }
      return ins;
    }

    // --- labels, soup, tooltip ----------------------------------------------
    // soupOffsets are a soup word's candidate nudges from its centroid:
    // dx in 6px steps within +-(w*.8 + 24), dy in h/2 steps within
    // +-h*2.6, nearest first by hypot(dx, 1.35*dy), cut to that ellipse,
    // at most 220.
    function soupOffsets(w, h) {
      var o = [], mx = w * 0.8 + 24, my = h * 2.6, cut = Math.max(mx, my * 1.35);
      for (var dy = -my; dy <= my + 1e-6; dy += h * 0.5) {
        for (var dx = -mx; dx <= mx + 1e-6; dx += 6) o.push([dx, dy, Math.hypot(dx, dy * 1.35)]);
      }
      o.sort(function (p, q) { return p[2] - q[2]; });
      return o.filter(function (p) { return p[2] <= cut; }).slice(0, 220);
    }
    // grid is the label spatial hash: AABB boxes {x, y, w, h} in cells of
    // cs px; hits tells whether a box overlaps one already put.
    function grid(cs) {
      var m = new Map();
      function cells(b, fn) {
        for (var gx = Math.floor(b.x / cs); gx <= Math.floor((b.x + b.w) / cs); gx++) {
          for (var gy = Math.floor(b.y / cs); gy <= Math.floor((b.y + b.h) / cs); gy++) if (fn(gx + "," + gy) === false) return false;
        }
        return true;
      }
      return {
        hits: function (b) {
          return !cells(b, function (key) {
            var l = m.get(key);
            if (l) for (var i = 0; i < l.length; i++) { var o = l[i]; if (b.x < o.x + o.w && b.x + b.w > o.x && b.y < o.y + o.h && b.y + b.h > o.y) return false; }
          });
        },
        put: function (b) { cells(b, function (key) { var l = m.get(key); if (!l) m.set(key, l = []); l.push(b); }); }
      };
    }
    // tipPos places a w x h tooltip in a W x H host: at the cursor +16/+14,
    // flipped left or above when it would cross the right or bottom margin,
    // then clamped 12px inside.
    function tipPos(mx, my, w, h, W, H) {
      var M = 12, x = mx + 16, y = my + 14;
      if (x + w > W - M) x = mx - w - 14;
      if (y + h > H - M) y = my - h - 12;
      return { x: clamp(x, M, Math.max(M, W - w - M)), y: clamp(y, M, Math.max(M, H - h - M)) };
    }

    // fitText shortens a title to maxLen characters and maxPx (width(s)
    // measures), by binary search, ending in an ellipsis.
    function fitText(t, maxPx, maxLen, width) {
      var s = t.length > maxLen ? t.slice(0, maxLen - 1).trimEnd() + "…" : t;
      if (width(s) <= maxPx) return s;
      var lo = 4, hi = Math.min(t.length, maxLen);
      while (lo < hi) {
        var mid = (lo + hi + 1) >> 1;
        if (width(t.slice(0, mid).trimEnd() + "…") <= maxPx) lo = mid; else hi = mid - 1;
      }
      return t.slice(0, lo).trimEnd() + "…";
    }
    var offCache = new Map();
    function cachedOffsets(w, h) {
      var key = w + "x" + h, o = offCache.get(key);
      if (!o) offCache.set(key, o = soupOffsets(w, h));
      return o;
    }
    // planLabels places one frame's labels greedily, in screen px, through
    // the spatial hash: the overlay controls (o.blocks) first, then the
    // framed stars (priority >= 9e6), the cluster headers (the full label
    // above the hull, the first term above, then both below, clamped 4px
    // inside), the ego captions (clamped likewise), and the soup and the
    // other stars (the soup first when o.soupFirst). o.stars are the
    // candidates, highest priority
    // first: {i, x, y, r (screen), p, title, framed, pre, side, inner,
    // skip}; o.obst the stars a title must not cover; o.heads {id, x, y,
    // by, full, short, cnt} in world units; o.sects [{x, y, text}|null];
    // o.soup {j, cl, x, y (screen centroid), w, h, poly (world)} in soup
    // order; o.text(s, weight, size, mono) measures. A soup word stays in
    // the viewport, inside its hull and off every other label, at most 8
    // per cluster.
    function planLabels(o) {
      var g = grid(80), W = o.W, H = o.H, k = o.k, ego = o.ego;
      var out = { stars: new Map(), heads: new Map(), sects: new Map(), soup: new Map() };
      o.blocks.forEach(function (b) { g.put(b); });
      function inView(b) { return b.x >= 2 && b.y >= 2 && b.x + b.w <= W - 2 && b.y + b.h <= H - 2; }
      var obst = o.obst.map(function (n) { var r = n.r + 1.5; return { x: n.x - r, y: n.y - r, w: r * 2, h: r * 2, i: n.i }; });
      function covers(b, own) {
        return obst.some(function (q) { return q.i !== own && b.x < q.x + q.w && b.x + b.w > q.x && b.y < q.y + q.h && b.y + b.h > q.y; });
      }
      function star(n) {
        if (n.skip) return;
        var wt = n.framed ? 620 : 400, preW = n.pre ? o.text(n.pre, 600) + 6 : 0, lim = Math.min(W * 0.42, 420);
        if (ego) {
          var r0 = n.r + 7, room = n.side === "l" ? n.x - r0 - 6 : n.side === "r" ? W - n.x - r0 - 6 : 2 * ego.R1 * k - 24;
          lim = Math.min(340, room);
          if (n.inner && ego.outer[n.side]) lim = Math.min(lim, ego.gap * k - 26);
          lim = Math.max(100, lim);
        }
        var pad = n.framed ? 20 : 10;
        var txt = fitText(n.title, lim - pad - preW, o.maxLen, function (s) { return o.text(s, wt); });
        var w = o.text(txt, wt) + pad + preW, h = n.framed ? 20 : 18, r = n.r;
        var at = { r: [n.x + r + 4, n.y - h / 2], l: [n.x - r - 4 - w, n.y - h / 2], b: [n.x - w / 2, n.y + r + 3], t: [n.x - w / 2, n.y - r - 3 - h] };
        var order = n.side === "l" ? "lrbt" : n.side === "t" ? "trlb" : "rlbt";
        for (var pass = 0; pass < (n.p >= 9e6 ? 2 : 1); pass++) {
          for (var q = 0; q < 4; q++) {
            var b = { x: at[order[q]][0], y: at[order[q]][1], w: w, h: h };
            if (!inView(b) || g.hits(b) || (!pass && covers(b, n.i))) continue;
            g.put(b);
            out.stars.set(n.i, { b: b, txt: txt, pre: n.pre });
            return;
          }
        }
      }
      function soup() {
        var per = {};
        o.soup.forEach(function (it) {
          if ((per[it.cl] || 0) >= 8 || it.x < -it.w || it.x > W + it.w || it.y < -it.h || it.y > H + it.h) return;
          var offs = cachedOffsets(it.w, it.h);
          for (var q = 0; q < offs.length; q++) {
            var cx = it.x + offs[q][0], cy = it.y + offs[q][1], b = { x: cx - it.w / 2, y: cy - it.h / 2, w: it.w, h: it.h };
            if (!inView(b) || !inPoly(it.poly, (cx - o.cx) / k, (cy - o.cy) / k) || g.hits(b)) continue;
            g.put(b);
            out.soup.set(it.j, b);
            per[it.cl] = (per[it.cl] || 0) + 1;
            return;
          }
        });
      }
      function rest() { o.stars.forEach(function (n) { if (n.p < 9e6) star(n); }); }
      o.stars.forEach(function (n) { if (n.p >= 9e6) star(n); });
      o.heads.forEach(function (hd) {
        [[hd.full, 0], [hd.short, 0], [hd.full, 1], [hd.short, 1]].some(function (c) {
          var w = Math.ceil(o.text(c[0], 680) + 36 + hd.cnt), h = 22, x = hd.x * k + o.cx, y = (c[1] ? hd.by : hd.y) * k + o.cy;
          var b = { x: x - w / 2, y: c[1] ? y - 2 : y - h + 2, w: w, h: h };
          if (b.x + b.w < 0 || b.x > W || b.y + b.h < 0 || b.y > H) return true;
          b.x = clamp(b.x, 4, W - w - 4);
          b.y = clamp(b.y, 4, H - h - 4);
          if (g.hits(b)) return false;
          g.put(b);
          out.heads.set(hd.id, { b: b, txt: c[0] });
          return true;
        });
      });
      o.sects.forEach(function (a, ix) {
        if (!a) return;
        var w = Math.ceil(o.text(a.text, 700, 11) + 30), b = { x: a.x * k + o.cx - w / 2, y: a.y * k + o.cy - 10, w: w, h: 20 };
        // Like a header, a caption past the edge is clamped 4px inside.
        b.x = clamp(b.x, 4, W - w - 4);
        b.y = clamp(b.y, 4, H - 24);
        if (!g.hits(b)) { g.put(b); out.sects.set(ix, b); }
      });
      if (o.soupFirst) { soup(); rest(); } else { rest(); soup(); }
      return out;
    }
    // simPick greedily picks the similarity edges at or above th, most
    // similar first, at most 3 per star; lists[i] holds node i's similar
    // tickets {j, s, terms}. Each pair is [s, a, b, terms].
    function simPick(lists, th) {
      var cnt = new Map(), pairs = [], seen = new Set(), out = [];
      lists.forEach(function (l, i) {
        l.forEach(function (s) {
          var key = Math.min(i, s.j) + ":" + Math.max(i, s.j);
          if (s.s < th || seen.has(key)) return;
          seen.add(key);
          pairs.push([s.s, i, s.j, s.terms || []]);
        });
      });
      pairs.sort(function (p, q) { return q[0] - p[0]; });
      pairs.forEach(function (p) {
        if ((cnt.get(p[1]) || 0) >= 3 || (cnt.get(p[2]) || 0) >= 3) return;
        cnt.set(p[1], (cnt.get(p[1]) || 0) + 1);
        cnt.set(p[2], (cnt.get(p[2]) || 0) + 1);
        out.push(p);
      });
      return out;
    }
    // bounds is the world box of the points P[i] for which ok(i) holds,
    // padded for their labels; null when none does.
    function bounds(P, ok) {
      var b = { x0: Infinity, y0: Infinity, x1: -Infinity, y1: -Infinity };
      P.forEach(function (p, i) {
        if (!p || !ok(i)) return;
        b.x0 = Math.min(b.x0, p.x - 30); b.y0 = Math.min(b.y0, p.y - 40);
        b.x1 = Math.max(b.x1, p.x + 30); b.y1 = Math.max(b.y1, p.y + 30);
      });
      return b.x0 <= b.x1 ? b : null;
    }
    // centroid is a soup word's anchor: its carrying tickets that ok(i)
    // keeps, weighted max(.15, tw), with their count n.
    function centroid(ids, tw, P, ok) {
      var sx = 0, sy = 0, sw = 0, n = 0;
      ids.forEach(function (i, ix) {
        if (!P[i] || !ok(i)) return;
        var w = Math.max(0.15, +tw[ix] || 0);
        sx += P[i].x * w; sy += P[i].y * w; sw += w; n++;
      });
      return { x: sw ? sx / sw : 0, y: sw ? sy / sw : 0, n: n };
    }

    // --- camera ---------------------------------------------------------------
    function clampK(k) { return clamp(k, K_MIN, K_MAX); }
    // zoomAt is the camera at zoom k2 (clamped) that keeps the world point
    // under the screen point (mx, my) fixed.
    function zoomAt(cam, mx, my, k2) {
      k2 = clampK(k2);
      var wx = (mx - cam.x) / cam.k, wy = (my - cam.y) / cam.k;
      return { x: mx - wx * k2, y: my - wy * k2, k: k2 };
    }
    // wheelDy normalizes a wheel delta to pixels (deltaMode 1 lines, 2
    // pages) and clamps it to +-320; wheelK is the zoom it asks for.
    function wheelDy(deltaY, mode) { return clamp(deltaY * (mode === 1 ? 32 : mode === 2 ? 400 : 1), -320, 320); }
    function wheelK(k, dy, ctrl) { return k * Math.exp(-dy * (ctrl ? 0.008 : 0.0028)); }
    // fitCam frames the world box b (x0, y0, x1, y1) with 30px sides and
    // the top and bottom reserves, at most maxK.
    function fitCam(b, W, H, top, bottom, maxK) {
      var k = clamp(Math.min((W - 60) / Math.max(1e-6, b.x1 - b.x0), (H - top - bottom) / Math.max(1e-6, b.y1 - b.y0)), K_MIN, maxK);
      return { k: k, x: W / 2 - (b.x0 + b.x1) / 2 * k, y: top + (H - top - bottom) / 2 - (b.y0 + b.y1) / 2 * k };
    }
    // The semantic zoom: stage 1 soup (k < .85), 2 crossfade (< 1.35), 3
    // titles; the Ego Ring is stage 3 with no soup. nodeScale keeps far
    // stars legible.
    function stage(k, ego) { return ego ? 3 : k < 0.85 ? 1 : k < 1.35 ? 2 : 3; }
    function soupAlpha(k, ego, sel) { return ego ? 0 : clamp((1.25 - k) / 0.4, 0, 1) * (sel ? 0.45 : 1); }
    function nodeScale(k) { return k < 0.85 ? Math.min(1.9, Math.sqrt(0.85 / k)) : 1; }
    function soupFont(nw) { return Math.round((11 + 5 * Math.pow(nw, 0.7)) * 2) / 2; }
    function easeOut(u) { return 1 - Math.pow(1 - u, 3); }
    function easeInOut(u) { return u < 0.5 ? 4 * u * u * u : 1 - Math.pow(-2 * u + 2, 3) / 2; }
    // edgePath trims an edge to both stars' radii; related and similarity
    // edges bend by min(30, .12 d).
    function edgePath(p, q, ra, rb, bent) {
      function f(v) { return v.toFixed(1); }
      var dx = q.x - p.x, dy = q.y - p.y, d = Math.hypot(dx, dy) || 1;
      var x1 = p.x + dx / d * ra, y1 = p.y + dy / d * ra, x2 = q.x - dx / d * rb, y2 = q.y - dy / d * rb;
      if (!bent) return "M" + f(x1) + "," + f(y1) + " L" + f(x2) + "," + f(y2);
      var bend = Math.min(30, d * 0.12);
      return "M" + f(x1) + "," + f(y1) + " Q" + f((x1 + x2) / 2 - dy / d * bend) + "," + f((y1 + y2) / 2 + dx / d * bend) + " " + f(x2) + "," + f(y2);
    }

    // --- Ego Ring -----------------------------------------------------------
    // ego places a ring around c0 in a W-wide viewport. m holds the
    // members: up (parent, blocked-by), down (child, blocks), rel (related),
    // in their display order, and sim ([[j, s]], the outer ring's similar
    // tickets). Related tickets alternate, then rebalance so the inner
    // columns match: left is up, a spacer, related-left; right is
    // related-right, a spacer, down. The outer ring alternates right and
    // left. Each item sits at x = +-R sqrt(max(.12, 1 - (y/Ry)^2)).
    function ego(m, W, c0) {
      var SP = 32, R1 = clamp(W * 0.1, 90, 190), R2 = R1 + clamp((W / 2 - R1) / 2 + 14, 170, 340);
      var relL = [], relR = [], flip = 0;
      m.rel.forEach(function (j) { (flip++ % 2 ? relL : relR).push(j); });
      var all = relR.concat(relL);
      relL = []; relR = [];
      all.forEach(function (j) { (m.up.length + relL.length <= m.down.length + relR.length ? relL : relR).push(j); });
      var left = m.up.concat(m.up.length && relL.length ? [null] : [], relL);
      var right = relR.concat(relR.length && m.down.length ? [null] : [], m.down);
      var rows = Math.max(left.length, right.length);
      var Ry1 = Math.max(R1 * 0.9, (rows - 1) / 2 * SP * 1.22 + 30);
      var out = { pos: new Map(), side: new Map(), rank: new Map(), tag: new Map(), pct: new Map(), R1: R1, R2: R2, Ry1: Ry1, Ry2: 0, rows: rows, left: left, right: right };
      out.pos.set(m.i, { x: c0.x, y: c0.y });
      out.side.set(m.i, "t");
      function place(items, sign, R, Ry, tag) {
        var n = items.length;
        items.forEach(function (j, ix) {
          if (j === null) return;
          var y = (ix - (n - 1) / 2) * SP, x = sign * R * Math.sqrt(Math.max(0.12, 1 - (y / Ry) * (y / Ry)));
          out.pos.set(j, { x: c0.x + x, y: c0.y + y });
          out.side.set(j, sign < 0 ? "l" : "r");
          out.rank.set(j, (tag === "x" ? 2000 : 1000) - ix);
          out.tag.set(j, tag);
        });
      }
      place(left, -1, R1, Ry1, "x");
      place(right, 1, R1, Ry1, "x");
      var sL = [], sR = [];
      m.sim.forEach(function (s, ix) { (ix % 2 ? sL : sR).push(s); });
      out.Ry2 = Math.max(Ry1 + 70, (Math.max(sL.length, sR.length) - 1) / 2 * SP * 1.22 + 40);
      place(sL.map(function (s) { return s[0]; }), -1, R2, out.Ry2, "s");
      place(sR.map(function (s) { return s[0]; }), 1, R2, out.Ry2, "s");
      m.sim.forEach(function (s) { out.pct.set(s[0], Math.round(s[1] * 100)); out.rank.set(s[0], 500 + s[1] * 100); });
      out.outer = { l: sL.length > 0, r: sR.length > 0 };
      // Sector captions: up, down, related left, related right, similar.
      function at(j, dx, dy) { var p = out.pos.get(j); return p ? { x: p.x + dx, y: p.y + dy } : null; }
      var simSide = sR.length >= sL.length ? sR : sL;
      out.sect = [
        m.up.length ? at(m.up[0], -60, -30) : null,
        m.down.length ? at(m.down[m.down.length - 1], 60, 30) : null,
        relL.length ? at(relL[0], -60, -30) : null,
        relR.length ? at(relR[0], 60, -30) : null,
        simSide.length ? at(simSide[0][0], simSide === sR ? 80 : -80, -38) : null
      ];
      return out;
    }
    // egoCam frames the ring: 70 above, 60 below, k at most 1, centered
    // on the ego ticket's x.
    function egoCam(pts, c0, W, H) {
      var y0 = Infinity, y1 = -Infinity;
      pts.forEach(function (p) { y0 = Math.min(y0, p.y); y1 = Math.max(y1, p.y); });
      y0 -= 46; y1 += 20;
      var k = clamp(Math.min(1, (H - 130) / Math.max(1, y1 - y0)), K_MIN, 1);
      return { k: k, x: W / 2 - c0.x * k, y: 70 + (H - 130) / 2 - (y0 + y1) / 2 * k };
    }

    // resolveHit picks the pointer's target: a soup word over a star
    // yields to the star (stack holds what lies under the pointer, top
    // first); otherwise the target's own kind (label, soup word,
    // similarity edge, background) stands.
    function resolveHit(target, stack) {
      if (target.type === "soup") {
        for (var i = 0; i < stack.length; i++) if (stack[i].type === "node") return stack[i];
      }
      return target;
    }

    return {
      clamp: clamp, rng: rng, radius: radius, clusterRadius: clusterRadius, iterations: iterations, layout: layout,
      ring: ring, convexHull: convexHull, hullPath: hullPath, hullTop: hullTop, inPoly: inPoly,
      soupOffsets: soupOffsets, grid: grid, tipPos: tipPos, fitText: fitText, planLabels: planLabels,
      simPick: simPick, bounds: bounds, centroid: centroid,
      clampK: clampK, zoomAt: zoomAt, wheelDy: wheelDy, wheelK: wheelK, fitCam: fitCam,
      stage: stage, soupAlpha: soupAlpha, nodeScale: nodeScale, soupFont: soupFont,
      easeOut: easeOut, easeInOut: easeInOut, edgePath: edgePath,
      ego: ego, egoCam: egoCam, resolveHit: resolveHit
    };
  })();
