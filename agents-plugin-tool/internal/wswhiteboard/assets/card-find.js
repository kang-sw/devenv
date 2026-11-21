;(function () {
  // In-card find bar: intercepts Ctrl/Cmd+F only while focus or the pointer
  // is inside a board card (panel, topic, or non-terminal board card). Outside
  // a card the browser or desktop default stays untouched. Works across both
  // host DOM text and sandboxed doc-frame iframes inside the card.
  var doc = document;
  if (window.wbCardFind) {
    if (window.wbCardFind.sync) window.wbCardFind.sync();
    return;
  }

  var TERMINALS = ".bcard[data-kind='terminal'], .tm-host, .xterm";
  var SKIP_TAGS = { SCRIPT: 1, STYLE: 1, TEMPLATE: 1, TEXTAREA: 1, INPUT: 1, SELECT: 1, SVG: 1, NOSCRIPT: 1 };

  function elOf(node) { return node && (node.nodeType === 1 ? node : node.parentElement); }
  function visible(el) {
    if (!el || el.isConnected === false) return false;
    if (el.getClientRects && !el.getClientRects().length) return false;
    if (window.getComputedStyle && getComputedStyle(el).visibility === "hidden") return false;
    return true;
  }
  function cardOf(node) {
    var el = elOf(node);
    if (!el || !el.closest || el.closest(TERMINALS)) return null;
    var panel = el.closest("details.entry-panel[open]:not(.msg)");
    if (panel) return panel;
    var bcard = el.closest(".bcard");
    if (bcard) return bcard.getAttribute("data-kind") === "terminal" ? null : bcard;
    var side = el.closest("details.panel");
    if (side) return side;
    var topic = el.closest("section.topic");
    if (topic) return topic;
    return null;
  }
  function nonCardModalOpen(d) {
    var list = d.querySelectorAll("dialog:modal, [aria-modal='true']");
    for (var i = list.length - 1; i >= 0; i--) {
      if (visible(list[i]) && !cardOf(list[i])) return true;
    }
    return false;
  }
  function resolveTarget(activeEl, eventTarget, ptr, hoveredFrame, d) {
    d = d || doc;
    if (nonCardModalOpen(d)) return null;
    var ae = elOf(activeEl), et = elOf(eventTarget);
    if (ae && ae.closest && ae.closest(TERMINALS)) return null;
    if (et && et.closest && et.closest(TERMINALS)) return null;
    var inBar = (ae && ae.closest && ae.closest(".wb-find-bar")) || (et && et.closest && et.closest(".wb-find-bar"));
    if (inBar) return cardOf(inBar);
    var c = cardOf(et) || cardOf(ae);
    if (c && visible(c)) return c;
    if (hoveredFrame && hoveredFrame.isConnected !== false) {
      c = cardOf(hoveredFrame);
      if (c && visible(c)) return c;
    }
    if (ptr && d.elementFromPoint) {
      var under = d.elementFromPoint(ptr.x, ptr.y);
      c = cardOf(under);
      if (c && visible(c)) return c;
    }
    return null;
  }
  function stepIndex(current, total, delta) {
    if (!total || total <= 0) return -1;
    if (current < 0 || current >= total) return delta < 0 ? total - 1 : 0;
    return ((current + delta) % total + total) % total;
  }
  function hiddenInCard(el, root) {
    for (var p = el; p && p !== root; p = p.parentElement) {
      if (p.hidden || (p.hasAttribute && p.hasAttribute("hidden"))) return true;
      if (p.parentElement && p.parentElement.tagName === "DETAILS" && !p.parentElement.open && p.tagName !== "SUMMARY") return true;
    }
    return false;
  }
  function clearHighlights(root) {
    if (!root || !root.querySelectorAll) return;
    var marks = Array.prototype.slice.call(root.querySelectorAll("mark.wb-find-hit"));
    var parents = new Set();
    marks.forEach(function (m) {
      var p = m.parentNode;
      if (!p) return;
      parents.add(p);
      while (m.firstChild) p.insertBefore(m.firstChild, m);
      p.removeChild(m);
    });
    parents.forEach(function (p) { if (p.normalize) p.normalize(); });
  }
  function collectTextNodes(root, skipEl) {
    var nodes = [];
    if (!root || !doc.createTreeWalker) return nodes;
    var walker = doc.createTreeWalker(root, 4 /* NodeFilter.SHOW_TEXT */, {
      acceptNode: function (node) {
        var p = node.parentElement;
        if (!p || !node.nodeValue || !node.nodeValue.trim()) return 2 /* FILTER_REJECT */;
        if (skipEl && (p === skipEl || skipEl.contains(p))) return 2;
        if (SKIP_TAGS[p.tagName]) return 2;
        if (p.closest && p.closest(".wb-find-bar, script, style, template, textarea, input, select, svg")) return 2;
        if (hiddenInCard(p, root)) return 2;
        return 1 /* FILTER_ACCEPT */;
      }
    });
    var n;
    while ((n = walker.nextNode())) nodes.push(n);
    return nodes;
  }
  function highlightNode(root, query, skipEl) {
    clearHighlights(root);
    if (!root || !query) return [];
    var needle = query.toLowerCase(), len = needle.length;
    if (!len) return [];
    var textNodes = collectTextNodes(root, skipEl);
    var marks = [];
    textNodes.forEach(function (node) {
      var val = node.nodeValue, lower = val.toLowerCase();
      var idx = lower.indexOf(needle);
      if (idx < 0) return;
      var frag = doc.createDocumentFragment(), pos = 0;
      while (idx >= 0) {
        if (idx > pos) frag.appendChild(doc.createTextNode(val.slice(pos, idx)));
        var mark = doc.createElement("mark");
        mark.className = "wb-find-hit";
        mark.textContent = val.slice(idx, idx + len);
        frag.appendChild(mark);
        marks.push(mark);
        pos = idx + len;
        idx = lower.indexOf(needle, pos);
      }
      if (pos < val.length) frag.appendChild(doc.createTextNode(val.slice(pos)));
      if (node.parentNode) node.parentNode.replaceChild(frag, node);
    });
    return marks;
  }

  var state = {
    card: null,
    bar: null,
    input: null,
    countEl: null,
    prevBtn: null,
    nextBtn: null,
    query: "",
    seq: 0,
    hostMarks: [],
    frames: [],       // [{ el, count }]
    items: [],        // [{ kind: "host", el } | { kind: "frame", frame, local }]
    index: -1,
    restoreFocus: null
  };
  var pointer = null, hoveredFrame = null;

  function cardFrames(card) {
    if (!card || !card.querySelectorAll) return [];
    return Array.prototype.slice.call(card.querySelectorAll("iframe.doc-frame")).filter(function (f) {
      return !hiddenInCard(f, card) && f.contentWindow;
    });
  }
  function postToFrame(frame, msg) {
    if (window.wbCardFindFrame && window.wbCardFindFrame.post) {
      window.wbCardFindFrame.post(frame, msg);
      return true;
    }
    return false;
  }
  function insertBar(card, bar) {
    if (card.matches("details.entry-panel, details.panel")) {
      var sum = card.querySelector(":scope > summary");
      if (sum && sum.nextSibling) card.insertBefore(bar, sum.nextSibling);
      else card.appendChild(bar);
      return;
    }
    if (card.matches("section.topic")) {
      var head = card.querySelector(":scope > .tophead");
      if (head && head.nextSibling) card.insertBefore(bar, head.nextSibling);
      else card.insertBefore(bar, card.firstChild);
      return;
    }
    if (card.matches(".bcard")) {
      var inner = card.querySelector(":scope > .bc-in") || card;
      var hd = inner.querySelector(":scope > .bc-hd");
      if (hd && hd.nextSibling) inner.insertBefore(bar, hd.nextSibling);
      else inner.insertBefore(bar, inner.firstChild);
      return;
    }
    card.insertBefore(bar, card.firstChild);
  }
  function buildBar() {
    var bar = doc.createElement("div");
    bar.className = "wb-find-bar";
    bar.setAttribute("role", "search");
    bar.setAttribute("aria-label", window.wsT ? wsT("js.find.label") : "Find in card");

    var input = doc.createElement("input");
    input.type = "search";
    input.className = "wb-find-input";
    input.placeholder = window.wsT ? wsT("js.find.placeholder") : "Find in card…";
    input.setAttribute("aria-label", bar.getAttribute("aria-label"));
    input.autocomplete = "off";
    input.spellcheck = false;

    var count = doc.createElement("span");
    count.className = "wb-find-count";
    count.setAttribute("aria-live", "polite");

    function mkBtn(cls, label, svg) {
      var b = doc.createElement("button");
      b.type = "button";
      b.className = "wb-find-btn " + cls;
      b.title = label;
      b.setAttribute("aria-label", label);
      b.innerHTML = svg;
      return b;
    }
    var prev = mkBtn("wb-find-prev", window.wsT ? wsT("js.find.prev") : "Previous match (Shift+Enter)",
      '<svg class="i" viewBox="0 0 16 16" aria-hidden="true"><path d="M4 10l4-4 4 4"/></svg>');
    var next = mkBtn("wb-find-next", window.wsT ? wsT("js.find.next") : "Next match (Enter)",
      '<svg class="i" viewBox="0 0 16 16" aria-hidden="true"><path d="M4 6l4 4 4-4"/></svg>');
    var closeBtn = mkBtn("wb-find-close", window.wsT ? wsT("js.find.close") : "Close find bar (Esc)",
      '<svg class="i" viewBox="0 0 16 16" aria-hidden="true"><path d="M4.2 4.2l7.6 7.6M11.8 4.2l-7.6 7.6"/></svg>');

    input.addEventListener("input", function () { runSearch(input.value, true); });
    input.addEventListener("keydown", function (e) {
      if (e.isComposing || e.keyCode === 229) return;
      if (e.key === "Enter") {
        e.preventDefault();
        step(e.shiftKey ? -1 : 1);
      } else if (e.key === "Escape") {
        e.preventDefault();
        e.stopPropagation();
        close(true);
      }
    });
    prev.addEventListener("click", function () { step(-1); input.focus(); });
    next.addEventListener("click", function () { step(1); input.focus(); });
    closeBtn.addEventListener("click", function () { close(true); });

    bar.appendChild(input);
    bar.appendChild(count);
    bar.appendChild(prev);
    bar.appendChild(next);
    bar.appendChild(closeBtn);
    return { bar: bar, input: input, count: count, prev: prev, next: next };
  }

  function rebuildIndex(resetPos) {
    var list = [];
    // Order matches in DOM order by comparing host marks and frame elements.
    var parts = [];
    state.hostMarks.forEach(function (m) { parts.push({ kind: "host", node: m, el: m }); });
    state.frames.forEach(function (fr) {
      if (fr.count > 0) parts.push({ kind: "frame", node: fr.el, frame: fr.el, count: fr.count });
    });
    parts.sort(function (a, b) {
      if (a.node === b.node) return 0;
      if (!a.node.compareDocumentPosition) return 0;
      return (a.node.compareDocumentPosition(b.node) & 4) ? -1 : 1;
    });
    parts.forEach(function (p) {
      if (p.kind === "host") list.push({ kind: "host", el: p.el });
      else for (var i = 0; i < p.count; i++) list.push({ kind: "frame", frame: p.frame, local: i });
    });
    state.items = list;
    if (!list.length) state.index = -1;
    else if (resetPos || state.index < 0 || state.index >= list.length) state.index = 0;
    paintCount();
    focusCurrent(resetPos);
  }
  function paintCount() {
    if (!state.countEl) return;
    var q = state.query, total = state.items.length;
    if (!q) {
      state.countEl.textContent = "";
      state.countEl.classList.remove("wb-find-none");
    } else {
      state.countEl.textContent = (total ? (state.index + 1) : 0) + "/" + total;
      state.countEl.classList.toggle("wb-find-none", total === 0);
    }
    var dis = total === 0;
    if (state.prevBtn) state.prevBtn.disabled = dis;
    if (state.nextBtn) state.nextBtn.disabled = dis;
  }
  function focusCurrent(scroll) {
    state.hostMarks.forEach(function (m) { m.classList.remove("wb-find-cur"); });
    var cur = state.index >= 0 ? state.items[state.index] : null;
    state.frames.forEach(function (fr) {
      if (!cur || cur.kind !== "frame" || cur.frame !== fr.el) {
        postToFrame(fr.el, { wbDoc: "find-focus", seq: state.seq, index: -1 });
      }
    });
    if (!cur) return;
    if (cur.kind === "host") {
      cur.el.classList.add("wb-find-cur");
      if (scroll && cur.el.scrollIntoView) cur.el.scrollIntoView({ block: "center", behavior: "auto" });
    } else if (cur.kind === "frame") {
      postToFrame(cur.frame, { wbDoc: "find-focus", seq: state.seq, index: cur.local, scroll: !!scroll });
    }
  }
  function scrollFrameMatch(frame, top, height) {
    if (!frame || frame.isConnected === false) return;
    var fr = frame.getBoundingClientRect();
    var targetY = fr.top + top + (height || 0) / 2;
    // Scroll the closest scrollable ancestor of the frame so the match centers.
    var s = frame.parentElement;
    while (s && s !== doc.body && s !== doc.documentElement) {
      if (s.scrollHeight > s.clientHeight + 2 && /auto|scroll/.test(getComputedStyle(s).overflowY)) {
        var sr = s.getBoundingClientRect();
        s.scrollTop += targetY - (sr.top + sr.height / 2);
        return;
      }
      s = s.parentElement;
    }
    if (window.scrollBy) window.scrollBy(0, targetY - window.innerHeight / 2);
  }
  function runSearch(query, resetPos) {
    if (!state.card) return;
    state.query = query;
    state.seq++;
    var seq = state.seq;
    state.hostMarks = highlightNode(state.card, query, state.bar);
    var frames = cardFrames(state.card);
    state.frames = frames.map(function (f) { return { el: f, count: 0 }; });
    frames.forEach(function (f) {
      if (!query) postToFrame(f, { wbDoc: "find-clear", seq: seq });
      else postToFrame(f, { wbDoc: "find", query: query, seq: seq });
    });
    rebuildIndex(resetPos);
  }
  function step(delta) {
    if (!state.items.length) return;
    state.index = stepIndex(state.index, state.items.length, delta);
    paintCount();
    focusCurrent(true);
  }
  function close(restore) {
    if (!state.card) return;
    var prevFocus = restore ? state.restoreFocus : null;
    clearHighlights(state.card);
    state.frames.forEach(function (fr) { postToFrame(fr.el, { wbDoc: "find-clear", seq: state.seq + 1 }); });
    if (state.bar && state.bar.parentNode) state.bar.parentNode.removeChild(state.bar);
    state.card.classList.remove("wb-find-open");
    state.card = null;
    state.bar = null;
    state.input = null;
    state.countEl = null;
    state.prevBtn = null;
    state.nextBtn = null;
    state.hostMarks = [];
    state.frames = [];
    state.items = [];
    state.index = -1;
    state.restoreFocus = null;
    if (prevFocus && prevFocus.isConnected && prevFocus.focus) {
      try { prevFocus.focus({ preventScroll: true }); } catch (e) {}
    }
  }
  function open(card) {
    if (!card) return false;
    if (state.card === card && state.bar && state.bar.isConnected) {
      state.input.focus();
      state.input.select();
      return true;
    }
    var prevQuery = state.query;
    var restoreEl = doc.activeElement && doc.activeElement !== doc.body ? doc.activeElement : null;
    close(false);
    var ui = buildBar();
    state.card = card;
    state.bar = ui.bar;
    state.input = ui.input;
    state.countEl = ui.count;
    state.prevBtn = ui.prev;
    state.nextBtn = ui.next;
    state.restoreFocus = restoreEl;
    card.classList.add("wb-find-open");
    insertBar(card, ui.bar);
    if (prevQuery) {
      ui.input.value = prevQuery;
      runSearch(prevQuery, true);
    } else {
      paintCount();
    }
    ui.input.focus();
    ui.input.select();
    return true;
  }
  function openFor(node) {
    var c = cardOf(node);
    return c ? open(c) : false;
  }
  function onFrameResult(frame, seq, count) {
    if (!state.card || seq !== state.seq) return;
    var entry = state.frames.filter(function (f) { return f.el === frame; })[0];
    if (!entry) return;
    entry.count = Math.max(0, count | 0);
    rebuildIndex(state.index < 0);
  }
  function onFrameScroll(frame, seq, top, height) {
    if (!state.card || seq !== state.seq) return;
    scrollFrameMatch(frame, top, height);
  }
  function setHoveredFrame(frame, source) {
    if (frame) hoveredFrame = frame;
    else if (!source || hoveredFrame === source) hoveredFrame = null;
  }
  function sync() {
    if (state.card && !state.card.isConnected) close(false);
  }

  doc.addEventListener("pointermove", function (e) {
    pointer = { x: e.clientX, y: e.clientY };
    var t = elOf(e.target);
    hoveredFrame = t && t.tagName === "IFRAME" && t.classList.contains("doc-frame") ? t : null;
  }, { passive: true });
  doc.addEventListener("pointerover", function (e) {
    var t = elOf(e.target);
    if (t && t.tagName === "IFRAME" && t.classList.contains("doc-frame")) hoveredFrame = t;
  }, { passive: true });
  doc.addEventListener("pointerout", function (e) {
    if (!e.relatedTarget) { pointer = null; hoveredFrame = null; }
    else if (hoveredFrame && e.target === hoveredFrame) hoveredFrame = null;
  });
  window.addEventListener("blur", function () { pointer = null; });

  doc.addEventListener("keydown", function (e) {
    if (e.isComposing || e.keyCode === 229) return;
    if (e.key === "Escape" && state.card) {
      e.preventDefault();
      e.stopPropagation();
      close(true);
      return;
    }
    if (!(e.ctrlKey || e.metaKey) || e.altKey || e.shiftKey || (e.key || "").toLowerCase() !== "f") return;
    var card = resolveTarget(doc.activeElement, e.target, pointer, hoveredFrame, doc);
    if (!card) return; // outside a card: leave browser/Electron default untouched
    e.preventDefault();
    e.stopPropagation();
    open(card);
  }, true);

  window.wbCardFind = {
    cardOf: cardOf,
    resolveTarget: resolveTarget,
    highlightNode: highlightNode,
    clearHighlights: clearHighlights,
    stepIndex: stepIndex,
    open: open,
    openFor: openFor,
    close: close,
    step: step,
    sync: sync,
    setHoveredFrame: setHoveredFrame,
    onFrameResult: onFrameResult,
    onFrameScroll: onFrameScroll,
    state: state
  };
})();
