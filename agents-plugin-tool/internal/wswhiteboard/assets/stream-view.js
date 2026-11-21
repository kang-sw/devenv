// Viewport visibility, scroll ownership, and explicit navigation controls.
  // viewKit holds the dashboard board's visibility source and keep-in-place
  // helper. win gives the globals it uses, so the kit runs under a fake
  // window too.
  //
  // watch(node, {onChange, onOut, margin}) subscribes to node's viewport
  // visibility: one IntersectionObserver per rootMargin, all with root null,
  // so whichever ancestor scrolls (a column, the pinned column, the
  // document) clips it. A margin reaches past an inner scroller's clip
  // only as a scroll margin (a root margin widens the viewport alone), so
  // each observer takes it as both where the browser has scroll margins;
  // bands tells whether it has, that is whether a margin means the same
  // distance from the view whichever ancestor scrolls. Each call is its own
  // subscription. onChange gets every report {node, ratio, visible, rect};
  // onOut runs once per change from ratio > 0 to 0 (a first report of 0
  // has not left view). seen(node) reads the node's last report without a
  // layout. sweep() drops the subscriptions of nodes no longer on the page.
  //
  // keep(node) captures the reading-line panel on node's actual scroller.
  // Nested synchronous mutations share one transaction; the last apply()
  // resolves that identity and writes at most once. There is no ordinary
  // resize session or substituted reference. capture:false only inspects
  // exposed geometry. Owners retain native-anchor suppression and input
  // listeners; input/momentum suppress captures until 200 ms after scrolling.
  // jump(node, {block, behavior}) alone follows asynchronous layout, until
  // 150 ms / five stable frames or its non-renewing 2 s cap. It has priority
  // over ordinary transactions, and input cancels it immediately.
  function viewKit(win) {
    var doc = win.document, observers = {}, reports = new WeakMap(), sessions = new Map(), owners = new Map();
    var watchNodes = new Set(), watchFrame = false;
    var IO = win.IntersectionObserver, bands = !!(IO && IO.prototype && "scrollMargin" in IO.prototype);
    function observer(margin) {
      if (observers[margin]) return observers[margin];
      var o = observers[margin] = { subs: new Map(), io: null };
      o.io = new win.IntersectionObserver(function (entries) {
        entries.forEach(function (e) {
          var list = o.subs.get(e.target);
          if (!list) return;
          var r = { node: e.target, ratio: e.intersectionRatio, visible: e.intersectionRatio > 0, rect: e.boundingClientRect };
          var s = reports.get(e.target) || { by: {} };
          s.by[margin] = r;
          s.last = r;
          reports.set(e.target, s);
          list.slice().forEach(function (sub) {
            if (sub.gone) return;
            var was = sub.ratio;
            sub.ratio = r.ratio;
            if (sub.onChange) sub.onChange(r);
            if (sub.onOut && was > 0 && r.ratio === 0) sub.onOut(r);
          });
        });
      }, bands ? { root: null, rootMargin: margin, scrollMargin: margin, threshold: [0] } : { root: null, rootMargin: margin, threshold: [0] });
      return o;
    }
    function drop(sub) {
      if (sub.gone) return;
      sub.gone = true;
      if (sub.node.wbWatch === sub.handle) {
        delete sub.node.wbWatch;
        delete sub.node.wbViewedExposure;
      }
      var o = observers[sub.margin], list = o && o.subs.get(sub.node);
      if (!list) return;
      list.splice(list.indexOf(sub), 1);
      if (!list.length) { o.subs.delete(sub.node); o.io.unobserve(sub.node); }
    }
    function watch(node, opt) {
      opt = opt || {};
      var sub = { node: node, margin: opt.margin || "0px", onChange: opt.onChange, onOut: opt.onOut, ratio: undefined, gone: false };
      var handle = { node: node, unwatch: function () { drop(sub); } };
      sub.handle = handle;
      if (!node || !win.IntersectionObserver) { sub.gone = true; return handle; }
      watchNodes.add(node);
      if (!watchFrame) {
        watchFrame = true;
        win.requestAnimationFrame(function () {
          watchFrame = false;
          var scrollers = new Set();
          watchNodes.forEach(function (n) { if (rendered(n)) scrollers.add(scrollerOf(n)); });
          watchNodes.clear();
          scrollers.forEach(owner);
        });
      }
      var o = observer(sub.margin), list = o.subs.get(node);
      // A node already observed reports only on a change: observing it
      // again gives the new subscription its first report.
      if (list) { list.push(sub); o.io.unobserve(node); } else o.subs.set(node, [sub]);
      o.io.observe(node);
      return handle;
    }
    function unwatch(handle) { if (handle && handle.unwatch) handle.unwatch(); }
    function sweep() {
      Object.keys(observers).forEach(function (m) {
        var gone = [];
        observers[m].subs.forEach(function (list, node) { if (!node.isConnected) gone = gone.concat(list); });
        gone.forEach(drop);
      });
    }
    function seen(node, margin) {
      var s = reports.get(node);
      return s ? (margin ? s.by[margin] || null : s.last) : null;
    }
    // A node's nearest scroller: the first ancestor whose overflow scrolls
    // and whose content overflows it, else the document's.
    function docScroller() { return doc.scrollingElement || doc.documentElement; }
    function scrollerOf(node) {
      for (var p = node && node.parentElement; p && p !== doc.body && p !== doc.documentElement; p = p.parentElement) {
        var oy = win.getComputedStyle(p).overflowY;
        if ((oy === "auto" || oy === "scroll" || oy === "overlay") && p.scrollHeight > p.clientHeight) return p;
      }
      return docScroller();
    }
    function shiftY(el) {
      var t = win.getComputedStyle(el).transform, m = t && /^matrix(3d)?\((.*)\)$/.exec(t);
      if (!m) return 0;
      var v = m[2].split(",").map(parseFloat);
      return (m[1] ? v[13] : v[5]) || 0;
    }
    // layoutTop is node's top in the viewport with the transforms between
    // it and its scroller taken out; scrollerTop is the scroller's content
    // top in the viewport.
    function layoutTop(node, scroller) {
      var y = node.getBoundingClientRect().top;
      for (var p = node; p && p !== scroller; p = p.parentElement) y -= shiftY(p);
      return y;
    }
    function scrollerTop(scroller) { return scroller === docScroller() ? 0 : scroller.getBoundingClientRect().top + (scroller.clientTop || 0); }
    function viewport(scroller, read) {
      read = read || function (n) { return n.getBoundingClientRect(); };
      var main = scroller === docScroller(), r = main ? { top: 0, bottom: win.innerHeight || scroller.clientHeight } : read(scroller);
      var top = Math.max(0, r.top + (main ? 0 : scroller.clientTop || 0));
      var bottom = Math.min(win.innerHeight || scroller.clientHeight, main ? r.bottom : r.top + (scroller.clientTop || 0) + scroller.clientHeight);
      var style = win.getComputedStyle(scroller);
      top += parseFloat(style.scrollPaddingTop) || 0;
      // The rail publishes its clearance on sticky heads, not on the host.
      Array.from((main ? doc : scroller).querySelectorAll('.tophead, .bc-hd, .wb-rail-wrap.up .wb-rail-stack')).filter(rendered).forEach(function (head) {
        var hs = win.getComputedStyle(head), h = read(head);
        if ((hs.position === 'sticky' || head.matches('.wb-rail-stack')) && h.height && h.top <= top + (parseFloat(hs.top) || 0) + 1 && h.bottom > top)
          top = Math.max(top, h.bottom);
      });
      return { top: top, bottom: bottom, left: main ? 0 : Math.max(0, r.left + (scroller.clientLeft || 0)),
        right: Math.min(win.innerWidth || r.right || Infinity, main ? win.innerWidth || Infinity : r.left + (scroller.clientLeft || 0) + scroller.clientWidth) };
    }
    // A geometry read inside closed details can itself force a display-lock
    // update and full-document layout. Reject hidden ancestry BEFORE any rect,
    // client box or scroller inspection; the details' own summary still shows.
    function rendered(node) {
      if (!node || node.isConnected === false) return false;
      for (var p = node, branch = node; p; branch = p, p = p.parentElement) {
        if (p.hidden) return false;
        if (p !== node && p.tagName === 'DETAILS' && !p.open) {
          var summary = Array.from(p.children || []).find(function (n) { return n.tagName === 'SUMMARY'; });
          if (branch !== summary) return false;
        }
      }
      if (node.checkVisibility) return node.checkVisibility({ visibilityProperty: true });
      // Older engines lack checkVisibility. Style reads do not open a hidden
      // details subtree's forced geometry scope (already rejected above).
      for (var p = node; p; p = p.parentElement) {
        var s = win.getComputedStyle(p);
        if (s.display === 'none' || (p === node && (s.visibility === 'hidden' || s.visibility === 'collapse')) || s.contentVisibility === 'hidden') return false;
      }
      return true;
    }
    function exposed(node, scroller, view, read) {
      if (!rendered(node)) return false;
      read = read || function (n) { return n.getBoundingClientRect(); };
      var v = view || viewport(scroller, read), r = read(node), top = Math.max(v.top, r.top), bottom = Math.min(v.bottom, r.bottom);
      var left = Math.max(v.left, r.left), right = Math.min(v.right, r.right);
      if (!(r.height > 0 && bottom - top >= 1 && right - left >= 1)) return false;
      for (var p = node.parentElement; p && p !== scroller && p !== doc.body; p = p.parentElement) {
        if (/hidden|clip|auto|scroll/.test(win.getComputedStyle(p).overflowY)) {
          var box = read(p); top = Math.max(top, box.top); bottom = Math.min(bottom, box.bottom);
          left = Math.max(left, box.left); right = Math.min(right, box.right);
        }
      }
      return r.height > 0 && bottom - top >= 1 && right - left >= 1;
    }
    function reading(scroller) {
      // One synchronous capture has one viewport. Re-scanning every sticky
      // head for every candidate makes history initialization/resize cubic
      // in board size; reject offscreen boxes before walking their ancestry.
      var root = scroller === docScroller() ? doc : scroller, view = viewport(scroller), line = view.top + 1;
      var selector = '.entry-panel, .wb-entry:not(.panel), .doc-frame, [data-card]', rects = new Map();
      function read(n) { if (!rects.has(n)) rects.set(n, n.getBoundingClientRect()); return rects.get(n); }
      function eligible(n) {
        if (!rendered(n) || (n.matches('.doc-frame') && n.closest('.entry-panel'))) return false;
        return exposed(n, scroller, view, read) && scrollerOf(n) === scroller;
      }
      // Frame-height batches normally capture the unit under the reading line
      // in constant work, rather than enumerating the entire long board.
      if (doc.elementFromPoint) {
        var hit = doc.elementFromPoint((view.left + view.right) / 2, line), path = [];
        for (var p = hit; p && p !== root; p = p.parentElement) {
          if (p.matches && p.matches(selector) && eligible(p)) path.push(p);
        }
        path.sort(function (a, b) { return read(a).top - read(b).top; });
        var atLine = path.find(function (n) { var r = read(n); return r.top <= line && r.bottom > line; });
        if (atLine) return atLine;
      }
      // Gaps, overlays and old engines retain the exact previous selection
      // rule, but hidden history/system runs are never measured and each
      // candidate rect is read once, including sorting/line selection.
      var nodes = Array.from(root.querySelectorAll(selector)).filter(rendered).filter(eligible);
      nodes.sort(function (a, b) { return read(a).top - read(b).top; });
      return nodes.find(function (n) { var r = read(n); return r.top <= line && r.bottom > line; }) ||
        nodes.find(function (n) { return read(n).top > line; }) || null;
    }
    function owner(scroller) {
      var o = owners.get(scroller);
      if (o) return o;
      var main = scroller === docScroller(), target = main ? win : scroller;
      o = { state: 'idle', until: 0, wrote: scroller.scrollTop, transaction: null };
      owners.set(scroller, o);
      scroller.wbScrollOwner = true;
      (main ? doc.documentElement : scroller).style.overflowAnchor = 'none';
      function clock() { return win.performance ? win.performance.now() : Date.now(); }
      function intent() {
        o.state = 'user-scrolling'; o.until = clock() + 200;
        if (o.transaction) o.transaction.cancel();
        if (o.pin) o.pin.end();
        var s = sessions.get(scroller);
        if (s && s.follow) { scroller.scrollTo({ top: scroller.scrollTop, left: scroller.scrollLeft, behavior: 'instant' }); s.cancel(); }
      }
      ['wheel', 'touchstart', 'touchmove', 'pointerdown'].forEach(function (type) { target.addEventListener(type, intent, { passive: true, capture: true }); });
      win.addEventListener('keydown', function (e) {
        if (!main && e.target && !scroller.contains(e.target) && doc.activeElement && !scroller.contains(doc.activeElement)) return;
        if (/^(ArrowUp|ArrowDown|ArrowLeft|ArrowRight|PageUp|PageDown|Home|End| |Spacebar)$/.test(e.key)) intent();
      }, true);
      target.addEventListener('scroll', function () {
        if (Math.abs(scroller.scrollTop - o.wrote) <= 1) return;
        var s = sessions.get(scroller);
        if (s && s.follow) return; // Native initial smooth navigation is not input.
        if (o.state === 'user-scrolling') o.until = clock() + 200;
        else intent();
        o.wrote = scroller.scrollTop;
      }, { passive: true });
      o.ready = function () {
        if (o.state === 'user-scrolling' && clock() >= o.until) o.state = 'idle';
        return o.state === 'idle';
      };
      return o;
    }
    // pin(ref) holds ref's place in its scroller for a settle window: apply()
    // scrolls by however far ref moved since the call, and a ResizeObserver
    // re-applies it before paint while late content (images, panel frames)
    // sizes. The window ends at its non-renewing cap, on owner input, or when
    // a jump takes the scroller; after that apply() is a no-op and ordinary
    // keep() transactions own the scroller again. Writes go through the scroll
    // owner so they are not taken for user input.
    var pins = new Set(), PIN_MS = 1500;
    function pin(ref) {
      var scroller = scrollerOf(ref), o = owner(scroller), top = layoutTop(ref, scroller);
      if (o.pin) o.pin.end();
      var p = { done: false, ro: null, cap: 0 };
      p.apply = function () {
        if (p.done) return;
        if (!ref.isConnected) { p.end(); return; }
        var delta = layoutTop(ref, scroller) - top;
        if (Math.abs(delta) >= 0.5) scroller.scrollTop += delta;
        o.wrote = scroller.scrollTop;
      };
      p.end = function () {
        if (p.done) return;
        p.done = true; pins.delete(p);
        if (o.pin === p) o.pin = null;
        if (p.ro) p.ro.disconnect();
        win.clearTimeout(p.cap);
      };
      o.pin = p; pins.add(p);
      p.cap = win.setTimeout(p.end, PIN_MS);
      if (win.ResizeObserver) {
        p.ro = new win.ResizeObserver(p.apply);
        var content = scroller === docScroller() ? [doc.body] : Array.prototype.slice.call(scroller.children);
        content.concat([ref.parentElement]).forEach(function (el) { if (el) p.ro.observe(el); });
      }
      return p;
    }
    function pinning() { return pins.size; }
    // idle(scroller): no owner input in the last 200 ms, no jump following
    // and no pin holding the scroller, so a keep() transaction would write
    // without cutting a settle window short.
    function idle(scroller) { var o = owner(scroller); return o.ready() && !sessions.get(scroller) && !o.pin; }
    function keep(node, opt) {
      opt = opt || {};
      var scroller = scrollerOf(node), o = owner(scroller), s = sessions.get(scroller);
      var inspect = { scroller: scroller, visible: function (n) { return exposed(n, scroller); }, viewport: function () { return viewport(scroller); }, apply: function () {}, end: function () {} };
      if (opt.capture === false) return inspect;
      if (s && s.follow) return s.handle;
      if (!o.ready()) return inspect;
      var t = o.transaction;
      if (!t) {
        var ref = typeof opt.offset === 'number' ? node : reading(scroller);
        var id = ref && ref.id, topic = ref && ref.closest('.topic'), name = topic && topic.getAttribute('data-topic');
        var key = ref && ref.getAttribute('data-doc-key');
        var offset = ref && (typeof opt.offset === 'number' ? opt.offset + scrollerTop(scroller) : layoutTop(ref, scroller));
        t = o.transaction = { depth: 0, done: false };
        t.cancel = function () { t.done = true; o.transaction = null; sessions.delete(scroller); };
        t.apply = function () {
          if (t.done || --t.depth > 0) return;
          t.cancel();
          if (!o.ready() || !ref) return;
          if (!ref.isConnected) {
            ref = id ? doc.getElementById(id) : null;
            if (!ref && key) ref = Array.from(doc.querySelectorAll('.doc-frame[data-doc-key]')).find(function (n) {
              var p = n.closest('.topic'); return n.getAttribute('data-doc-key') === key && p && p.getAttribute('data-topic') === name;
            });
          }
          if (!ref || !ref.isConnected || scrollerOf(ref) !== scroller || (name && (!ref.closest('.topic') || ref.closest('.topic').getAttribute('data-topic') !== name))) return;
          var delta = layoutTop(ref, scroller) - offset;
          if (Math.abs(delta) >= 0.5) scroller.scrollTop += delta;
          o.wrote = scroller.scrollTop;
          if (opt.onEnd) opt.onEnd('settled');
        };
        sessions.set(scroller, { handle: inspect });
      }
      t.depth++;
      var used = false;
      inspect.apply = function () { if (!used) { used = true; t.apply(); } };
      inspect.end = function () { if (!used) { used = true; if (!t.done && --t.depth === 0) t.cancel(); } };
      return inspect;
    }
    // Explicit Overview placement uses the navigation owner, including its
    // late-layout follow and input cancellation, never an ordinary capture.
    // readingOffset follows the sticky reading line as a reloaded board mounts;
    // ordinary offset stays relative to the scroller. Both use the same jump.
    function place(node, opt) {
      return jump(node, { offset: opt && typeof opt.offset === 'number' ? opt.offset : 0,
        readingOffset: opt && opt.readingOffset, behavior: 'instant' });
    }
    function jump(node, opt) {
      var scroller = scrollerOf(node), s = sessions.get(scroller);
      if (opt && opt.prepare && s) {
        if (s.cancel) s.cancel(); else s.handle.end();
        s = null;
      }
      if (!s || !s.follow) s = start(scroller);
      if (opt && opt.onEnd) s.ends.push(opt.onEnd);
      if (opt && opt.prepare) opt.prepare();
      s.jump(node, opt || {});
      return s.handle;
    }
    function start(scroller) {
      var o = owner(scroller);
      if (o.transaction) o.transaction.cancel();
      if (o.pin) o.pin.end();
      o.state = 'explicit-jump-follow';
      var s = { ends: [], ro: null, done: false };
      var main = scroller === docScroller();
      function apply() {
        if (s.done) return;
        if (s.follow) s.follow.align();
        o.wrote = scroller.scrollTop;
      }
      function end(reason) {
        if (s.done) return;
        s.done = true;
        if (s.follow) s.follow.off();
        sessions.delete(scroller);
        if (s.ro) s.ro.disconnect();
        if (o.state === 'explicit-jump-follow') o.state = 'idle';
        o.wrote = scroller.scrollTop;
        s.ends.forEach(function (fn) { fn(reason); });
      }
      s.cancel = function () { end('user'); };
      s.jump = function (node, opt) {
        if (s.follow) s.follow.off();
        var block = opt.block || "center", raf = 0, cap = 0, stable = 0, quiet = 0, last, position;
        function off() {
          win.cancelAnimationFrame(raf);
          win.clearTimeout(cap);
        }
        function measure() {
          if (typeof opt.top === 'number') return opt.top;
          var r = node.getBoundingClientRect();
          var offset = typeof opt.readingOffset === 'number' ? viewport(scroller).top + 1 - scrollerTop(scroller) + opt.readingOffset :
            typeof opt.offset === 'number' ? opt.offset : block === "center" ? (scroller.clientHeight - r.height) / 2 : 0;
          return r.top - scrollerTop(scroller) + scroller.scrollTop - offset;
        }
        function align() {
          // A board morph may replace the item but retain its stable ID.
          if (!node.isConnected && node.id) node = doc.getElementById(node.id) || node;
          if (!node.isConnected || scrollerOf(node) !== scroller) { end("ended"); return; }
          var next = measure();
          // Smooth scrolling changes the viewport rect, not this content-space
          // destination. Only layout movement interrupts it with an instant fix.
          if (Math.abs(next - position) >= 0.5) {
            land('instant');
            stable = 0; quiet = 0;
            position = next;
          }
        }
        function frame(t) {
          raf = 0;
          align();
          if (s.done) return;
          var current = scroller.scrollTop;
          if (last !== undefined && Math.abs(current - last) < 0.5) {
            if (!stable++) quiet = t;
            if (stable >= 5 && t - quiet >= 150) { end("settled"); return; }
          } else { stable = 0; quiet = t; }
          last = current;
          raf = win.requestAnimationFrame(frame);
        }
        function land(behavior) {
          if (typeof opt.top === 'number' || typeof opt.offset === 'number' || typeof opt.readingOffset === 'number') scroller.scrollTo({ top: measure(), behavior: behavior });
          else node.scrollIntoView({ block: block, behavior: behavior });
          o.wrote = scroller.scrollTop;
        }
        s.follow = { align: align, off: off };
        position = measure();
        var reduced = win.matchMedia && win.matchMedia("(prefers-reduced-motion: reduce)").matches;
        land(reduced ? "instant" : opt.behavior || "instant");
        cap = win.setTimeout(function () { end("cap"); }, 2000);
        raf = win.requestAnimationFrame(frame);
      };
      // A swap settling on another anchor must not end explicit navigation.
      s.handle = { scroller: scroller, apply: apply, end: function () { if (!s.follow) end("ended"); } };
      if (win.ResizeObserver) {
        s.ro = new win.ResizeObserver(function () { if (!s.done) apply(); });
        (main ? [doc.body] : Array.prototype.slice.call(scroller.children)).forEach(function (el) { if (el) s.ro.observe(el); });
      }
      sessions.set(scroller, s);
      return s;
    }
    function keeping(scroller) { var s = sessions.get(scroller); return s ? s.handle : null; }
    // snapshot() is a read-only exposure view for one synchronous pass with
    // no DOM write inside it: each scroller's viewport and each ancestor's
    // scroller are measured once. Per-node keep(capture:false).visible
    // rescans every sticky head, which over a whole board's panels froze
    // every board update for ~1 s (100 topics).
    function snapshot() {
      var views = new Map(), under = new Map(), rects = new Map(), visibility = new Map();
      function read(node) { if (!rects.has(node)) rects.set(node, node.getBoundingClientRect()); return rects.get(node); }
      function scroller(node) {
        var chain = [], found = null;
        for (var p = node && node.parentElement; p && p !== doc.body && p !== doc.documentElement; p = p.parentElement) {
          if (under.has(p)) { found = under.get(p); break; }
          chain.push(p);
          var oy = win.getComputedStyle(p).overflowY;
          if ((oy === "auto" || oy === "scroll" || oy === "overlay") && p.scrollHeight > p.clientHeight) { found = p; break; }
        }
        if (!found) found = docScroller();
        chain.forEach(function (c) { under.set(c, found); });
        return found;
      }
      function visible(node) {
        if (visibility.has(node)) return visibility.get(node);
        if (!rendered(node)) { visibility.set(node, false); return false; }
        var s = scroller(node), v = views.get(s);
        if (!v) { v = viewport(s, read); views.set(s, v); }
        var result = exposed(node, s, v, read);
        visibility.set(node, result);
        return result;
      }
      return { scroller: scroller, visible: visible };
    }
    return { watch: watch, unwatch: unwatch, sweep: sweep, seen: seen, keep: keep, pin: pin, pinning: pinning, idle: idle, place: place, keeping: keeping, jump: jump, bands: bands, snapshot: snapshot, rendered: rendered };
  }
