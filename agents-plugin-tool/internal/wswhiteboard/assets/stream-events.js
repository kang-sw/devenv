// Recap jumps and latest-agent arrival effects.

/* Recap jump: a topic recap's clock chip (.rat, a keyboard button) names the
   log row the recap summarizes (the recap's data-entry, its latest agent
   line). Activating the chip, or a plain click on the recap text, reveals
   that row through the shared history loader (opening its disclosures,
   clearing the sticky head), whose flash is the one highlight. A text click
   waits out a double-click so selecting a word never jumps; a drag, a
   selection, or a modified click is left alone, and controls inside the
   recap keep their own behavior. Document-level, so board swaps need no
   rebinding. */
(function () {
  if (window.wbRecapJump) return;
  // DOUBLE stands in for the platform double-click interval, which a page
  // cannot read; FLASH outlasts page.css .flash (1.6s).
  var CHIP = '.trecap[data-entry] > .rat[role="button"]', DOUBLE = 500, FLASH = 2000, press = null, pending = 0;
  function recapOf(n) {
    var recap = n && n.closest ? n.closest('.trecap[data-entry]') : null;
    return recap && recap.querySelector(':scope > .rat[role="button"]') ? recap : null;
  }
  function cancel() { window.clearTimeout(pending); pending = 0; }
  function selecting(recap) {
    var sel = window.getSelection && window.getSelection();
    return !!(sel && !sel.isCollapsed && sel.rangeCount && sel.getRangeAt(0).intersectsNode(recap));
  }
  function jump(recap) {
    cancel();
    var topic = recap.closest('.topic'), entry = recap.getAttribute('data-entry'), history = window.wbHistory;
    if (!topic || !history || !history.loadTarget) return null;
    if (window.wbPill && window.wbPill.apply && !window.wbPill.apply(function () { jump(recap); })) return null;
    if (topic.isConnected === false) topic = topic.id ? document.getElementById(topic.id) : null;
    if (!topic) return null;
    // A dashboard card fold hides the whole topic; reveal it before geometry.
    var unfold = window.WsCards && window.WsCards.unfoldTopic;
    var row = history.loadTarget(topic, entry, unfold);
    if (row) {
      // The arrival blink's more specific animation would mask the flash and
      // replay it when the blink ends, so neither may outlive this one.
      row.classList.remove('wb-log-blink');
      window.clearTimeout(row._wbRecapFlash);
      row._wbRecapFlash = window.setTimeout(function () { row.classList.remove('flash'); }, FLASH);
    }
    return row;
  }
  // Every press cancels a pending text jump: it may be a double-click's
  // second press, and its own click decides again.
  document.addEventListener('pointerdown', function (e) {
    cancel();
    press = recapOf(e.target) ? { x: e.clientX, y: e.clientY } : null;
  }, true);
  document.addEventListener('click', function (e) {
    var recap = recapOf(e.target), at = press;
    press = null;
    if (!recap || e.defaultPrevented || e.button || e.altKey || e.ctrlKey || e.metaKey || e.shiftKey) return;
    if (e.detail > 1) { cancel(); return; }
    var inner = e.target.closest('a[href], button, input, textarea, select, summary, label, [contenteditable]');
    if (inner && recap.contains(inner)) return;
    if (at && Math.abs(e.clientX - at.x) + Math.abs(e.clientY - at.y) > 4) return;
    if (selecting(recap)) return;
    var chip = e.target.closest('.rat[role="button"]');
    if (chip && recap.contains(chip)) { jump(recap); return; }
    pending = window.setTimeout(function () {
      pending = 0;
      if (recap.isConnected && !selecting(recap)) jump(recap);
    }, DOUBLE);
  });
  document.addEventListener('keydown', function (e) {
    if (e.defaultPrevented || (e.key !== 'Enter' && e.key !== ' ' && e.key !== 'Spacebar')) return;
    if (e.isComposing || e.repeat || e.altKey || e.ctrlKey || e.metaKey || e.shiftKey) return;
    if (!e.target || !e.target.matches || !e.target.matches(CHIP)) return;
    e.preventDefault();
    jump(e.target.parentElement);
  });
  window.wbRecapJump = { jump: jump };
})();

/* Latest agent arrival: state belongs to the page, not to replaceable board
   nodes. Initial content (including inert history) is never an arrival. */
(function () {
  if (!window.wbLatestLog) {
    var started = Date.now(), topics = new Set(), seen = new Set(), watched = new Map();
    // Seeing the header recap must not consume the lower log's first sight.
    var selector = '.wb-entry.agent.current';
    function key(node) {
      var topic = node.closest('.topic');
      return topic && topic.getAttribute('data-topic') + '\n' + node.getAttribute('data-entry');
    }
    function baseline(root) {
      root.querySelectorAll('[data-kind="agent"][data-entry]').forEach(function (node) {
        var k = key(node);
        if (k) seen.add(k);
      });
      root.querySelectorAll('template').forEach(function (t) { if (t.content) baseline(t.content); });
    }
    baseline(document);
    function reduced() { return window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches; }
    var observer = window.IntersectionObserver && new window.IntersectionObserver(function (entries) {
      entries.forEach(function (entry) {
        var node = entry.target, k = watched.get(node);
        if (!entry.isIntersecting || !entry.intersectionRatio || !k || !node.isConnected) return;
        // The observer accounts for viewport and history-clip intersection;
        // closed disclosures must not consume a pending arrival either.
        for (var p = node.parentElement; p; p = p.parentElement) {
          if (p.tagName === 'DETAILS' && !p.open) return;
        }
        if (!seen.has(k)) {
          seen.add(k);
          if (!reduced()) {
            node.classList.add('wb-log-blink');
            window.setTimeout(function () { node.classList.remove('wb-log-blink'); }, 3000);
          }
        }
        observer.unobserve(node);
        watched.delete(node);
      });
    });
    function init() {
      var candidates = new Map();
      document.querySelectorAll(selector).forEach(function (node) {
        var k = key(node);
        if (!k) return;
        candidates.set(node, k);
        var name = node.closest('.topic').getAttribute('data-topic');
        if (!topics.has(name)) {
          // A closed topic fetched later is old content, not a new log. New
          // topics still qualify when their log was written since page load.
          var time = node.querySelector('time[datetime]');
          if (!time || !(Date.parse(time.getAttribute('datetime')) > started)) seen.add(k);
        }
      });
      candidates.forEach(function (k, node) {
        topics.add(node.closest('.topic').getAttribute('data-topic'));
      });
      watched.forEach(function (k, node) {
        if (!candidates.has(node) || candidates.get(node) !== k || seen.has(k)) {
          observer.unobserve(node);
          watched.delete(node);
          node.classList.remove('wb-log-blink');
        }
      });
      candidates.forEach(function (k, node) {
        if (!observer || seen.has(k) || watched.has(node)) return;
        watched.set(node, k);
        observer.observe(node);
      });
    }
    window.wbLatestLog = { init: init };
    // Board swaps normally bind explicitly; child-list changes also cover
    // history admission and deferred closed-topic insertion. Do not observe
    // classes: our own animation must not trigger another binding pass.
    if (window.MutationObserver) new window.MutationObserver(function (changes) {
      // Ignore unrelated host activity, such as streaming terminal output.
      if (changes.some(function (change) {
        if (change.target.closest && change.target.closest('.topic')) return true;
        return Array.from(change.addedNodes).concat(Array.from(change.removedNodes)).some(function (node) {
          return node.nodeType === 1 && (node.matches('.topic') || node.querySelector('.topic'));
        });
      })) init();
    }).observe(document.body, { childList: true, subtree: true });
  }
  var bind = window.wbBindBoard;
  if (bind && !bind.wbLatestLogBound) {
    window.wbBindBoard = function (root) { bind(root); window.wbLatestLog.init(); };
    window.wbBindBoard.wbLatestLogBound = true;
  }
  window.wbLatestLog.init();
})();
