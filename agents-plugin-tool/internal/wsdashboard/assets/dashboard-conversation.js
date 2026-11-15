  // --- conversation entries: merged by key, never re-rendered ----------------
  // Each entry element carries data-entry, its ring entry name; names sort in
  // time order. Only the entries container is searched: held bubbles in the
  // convlive region carry data-entry too (their inbox id) and are not
  // entries. A box keeps the names it holds (entryKeys) across merges, so a
  // merge reads only the incoming entries.
  function entriesBox() { var r = region("entries"); return r && r.querySelector(".centries"); }
  function frag(html) { var t = doc.createElement("template"); t.innerHTML = html; return t.content; }
  function entryKeys(box) {
    if (!box.wsKeys) {
      box.wsKeys = Object.create(null);
      Array.prototype.forEach.call(box.children, function (c) { var k = c.getAttribute("data-entry"); if (k) box.wsKeys[k] = true; });
    }
    return box.wsKeys;
  }
  function resetEntries(box) { box.textContent = ""; box.wsKeys = Object.create(null); }
  // mergeEntries inserts each node of src whose name box lacks at its sorted
  // place and returns the inserted nodes, in order; an entry already shown
  // is kept as it is.
  function mergeEntries(box, src) {
    var have = entryKeys(box), added = [];
    Array.prototype.slice.call(src.children).forEach(function (n) {
      var k = n.getAttribute("data-entry");
      if (!k || have[k]) return;
      // New entries are usually the newest (live) or the oldest (history),
      // so walk back from the end only past entries that sort after k.
      var c = box.lastElementChild;
      while (c && (c.getAttribute("data-entry") || "") > k) c = c.previousElementSibling;
      box.insertBefore(n, c ? c.nextSibling : box.firstChild);
      have[k] = true;
      added.push(n);
      restoreOpen(n);
      fmtTimes(n);
    });
    return added;
  }
  // gapped reports a newest window (nbox) that does not reach what box
  // holds: its oldest entry is newer than box's newest, so entries between
  // them, missed while a stream was down, are in neither.
  function gapped(box, nbox) {
    var last = box.lastElementChild, first = nbox.firstElementChild;
    return !!(last && first && (first.getAttribute("data-entry") || "") > (last.getAttribute("data-entry") || ""));
  }
  // ENTRY_CAP bounds a conversation box while the user keeps it at the
  // bottom: three of the server's newest windows (its ring size, 50).
  // evictEntries drops the oldest entries past it and reports whether it
  // dropped any.
  var ENTRY_CAP = 150;
  function evictEntries(box) {
    var n = box.children.length - ENTRY_CAP, keys = entryKeys(box);
    if (n <= 0) return false;
    for (; n > 0; n--) {
      var c = box.firstElementChild;
      delete keys[c.getAttribute("data-entry")];
      box.removeChild(c);
    }
    return true;
  }
  // mergeEntriesRegion applies an entries region event: the title bar (its
  // counts) is replaced, and the entries are merged. A window past a gap
  // starts the box over; a box at the bottom (pin) sheds its oldest entries
  // past the cap. Either way the older history is loaded again from what the
  // box then holds.
  function mergeEntriesRegion(el, html, pin) {
    var src = frag(html), box = entriesBox(), head = el.querySelector(".thead"), nhead = src.querySelector(".thead");
    var nbox = src.querySelector(".centries");
    if (!box || !nbox) { el.innerHTML = html; restoreOpen(el); fmtTimes(el); sweepOutImgs(el); mermaid(el); histReset(); return; }
    if (head && nhead) { head.parentNode.replaceChild(nhead, head); fmtTimes(nhead); }
    if (gapped(box, nbox)) { resetEntries(box); histReset(); }
    var added = mergeEntries(box, nbox);
    added.forEach(sweepOutImgs);
    mermaid(added);
    if (pin && evictEntries(box)) histReset();
  }

  // Conversation cards navigate only this session's board and validated IDs.
  // The existing card/filter and history owners retain all geometry decisions.
  var effectJumpEpoch = 0;
  function effectTopic(id) {
    if (typeof id !== "string" || !/^topic-(?:[0-9a-f]{2})+$/.test(id)) return null;
    return boardQ("section.topic[data-topic]").filter(function (n) { return n.id === id && inBoard(n); })[0] || null;
  }
  function effectJump(topicID, id, done) {
    if (typeof topicID !== "string" || !/^topic-(?:[0-9a-f]{2})+$/.test(topicID) ||
        typeof id !== "string" || (id !== topicID && !/^(?:item-[1-9][0-9]{0,5}|[ep][1-9][0-9]*)$/.test(id))) return;
    var epoch = ++effectJumpEpoch, seen = Object.create(null);
    if (doc.body.classList.contains("tk-paged") && tkApi) tkApi.leavePage();
    if (grid && !tabShown("board")) setTab("board");
    function result(topic, exact) { if (epoch === effectJumpEpoch && done) done(topic, exact); }
    function step() {
      if (epoch !== effectJumpEpoch) return;
      var topic = effectTopic(topicID);
      if (!topic) {
        // Closed topics already have a paginated route. Do not invent a new
        // anchor/store API or search another session when a target is absent.
        var more = boardQ("button.closed-more[data-closed-next]")[0];
        var cursor = more && more.getAttribute("data-closed-next");
        if (!more || more.disabled || seen[cursor]) { result(false, false); return; }
        seen[cursor] = true;
        var pending = loadClosed(more);
        if (!pending || !pending.then) { result(false, false); return; }
        pending.then(function (loaded) { if (loaded) step(); else result(false, false); });
        return;
      }
      var cards = window.WsCards, unfold = cards && cards.unfoldTopic;
      function reveal() {
        if (epoch !== effectJumpEpoch) return;
        // SSE may have replaced the topic while overview/filter restore ran.
        if (effectTopic(topicID) !== topic) { step(); return; }
        var target = topic;
        if (id !== topicID) {
          if (/^[ep]/.test(id)) target = window.wbHistory && window.wbHistory.loadTarget ? window.wbHistory.loadTarget(topic, id, unfold) : null;
          else target = topic.querySelector('[id="' + id + '"]');
          if (!target || target.id !== id || !inBoard(target) || target.closest("section.topic[data-topic]") !== topic ||
              (/^item-/.test(id) && !target.matches(".card.policy[data-id], .arow[data-id], .ckrow[data-id], .srow"))) target = null;
        }
        var exact = !!target;
        if (!target) target = topic;
        if (unfold) unfold(target);
        for (var p = target; p && inBoard(p); p = p.parentNode) if (p.tagName === "DETAILS") p.open = true;
        if (target.matches(".ckrow.done, .ckrow.no") && root.getAttribute("data-ck-hide") === "1") {
          var toggle = topic.querySelector("button.ckhide[aria-pressed=true]");
          if (toggle) toggle.click();
        }
        // loadTarget owns lazy-history positioning, including version folds.
        if (!target.closest(".wb-history")) {
          target.scrollIntoView({ block: "start" });
          if (window.wbHistory && window.wbHistory.keep) window.wbHistory.keep(target);
        }
        target.setAttribute("tabindex", "-1");
        target.focus({ preventScroll: true });
        result(true, exact);
      }
      if (cards && cards.revealTopic) cards.revealTopic(topic, reveal); else reveal();
    }
    step();
  }
  function effectJumpClick(e) {
    var t = e.target, box = entriesBox();
    if (!t || !t.closest || !box || e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return;
    var card = t.closest(".wbupdate-topic[data-wb-topic]");
    if (!card || !box.contains(card)) return;
    var link = t.closest("a[data-wb-jump]");
    if (!link && t.closest("a, summary, button, input, textarea, select")) return;
    var selection = window.getSelection && window.getSelection();
    if (!link && selection && !selection.isCollapsed) return;
    e.preventDefault();
    var row = t.closest(".wbupdate-row[data-wb-element]");
    var topicID = card.getAttribute("data-wb-topic");
    var id = link ? link.getAttribute("data-wb-jump") : row ? row.getAttribute("data-wb-element") : topicID;
    effectJump(topicID, id, function (found, exact) {
      var status = card.querySelector(".wbupdate-status");
      if (status) status.textContent = !found ? wsT("js.whiteboard.jump.missing") : !exact ? wsT("js.whiteboard.jump.elementmissing") : "";
    });
  }
  doc.addEventListener("click", effectJumpClick);

  // --- older history: loaded a page at a time when scrolled to the top -------
  // The region draws the newest entries; older ones come from the history
  // route and are prepended with the scroll position kept. histBefore is the
  // next page's cursor ("" asks for the newest page, so a page that drew no
  // entries can still start loading); null takes it from the box's oldest
  // entry. histReset forgets the cursor and the end once the box's oldest
  // entry changed under it (evicted, or the box started over), and its epoch
  // drops a page still in flight, which would no longer join what the box
  // holds.
  var histBefore = null, histEnd = false, histBusy = false, histEpoch = 0;
  function histReset() { histBefore = null; histEnd = false; histBusy = false; histEpoch++; }
  function loadHistory() {
    if (histEnd || histBusy || !window.fetch || !convo || !visible(convo)) return;
    var box = entriesBox();
    if (!box) return;
    if (histBefore === null) {
      var first = box.firstElementChild;
      histBefore = first ? first.getAttribute("data-entry") || "" : "";
    }
    histBusy = true;
    var epoch = histEpoch;
    var url = "/s/" + rid + "/history?n=50" + (histBefore ? "&before=" + encodeURIComponent(histBefore) : "");
    fetch(url, { credentials: "same-origin", cache: "no-store" }).then(function (r) {
      if (!r.ok) throw new Error("HTTP " + r.status);
      return r.json();
    }).then(function (res) {
      if (epoch !== histEpoch) return;
      histBusy = false;
      var h = convo.scrollHeight, top = convo.scrollTop;
      var added = mergeEntries(box, frag(res.html || ""));
      added.forEach(sweepOutImgs);
      // Keep what the user was reading in place: the prepended height goes
      // above the scroll offset.
      convo.scrollTop = top + (convo.scrollHeight - h);
      mermaid(added);
      if (res.before) histBefore = res.before;
      if (res.end || !res.before) histEnd = true;
      nearTop();
    }, function () {
      // A failed fetch (daemon replaced, network) is retried on the next
      // scroll to the top, after a pause.
      if (epoch === histEpoch) setTimeout(function () { if (epoch === histEpoch) histBusy = false; }, 3000);
    });
  }
  // nearTop loads older entries when the conversation is scrolled to (or
  // too short to scroll away from) its top.
  function nearTop() { if (convo && visible(convo) && convo.scrollTop < 160) loadHistory(); }
  if (convo) convo.addEventListener("scroll", nearTop);

  // mermaid draws every undrawn mermaid block, loading the shared script
  // once; a call while it loads is covered by the draw its onload runs, and
  // a failed load (daemon restarting) is retried by the next call. roots
  // (an element or a list of them: what a swap just inserted) limits the
  // look for undrawn blocks to them; omitted, the whole page is looked at.
  var mermaidLoading = false;
  function mermaid(roots) {
    var list = roots == null ? [doc] : Array.isArray(roots) ? roots : [roots];
    if (!window.wbMermaidFile || !list.some(function (r) { return !!r.querySelector("pre.mermaid:not([data-processed])"); })) return;
    if (window.mermaid) { window.wbMermaid(); return; }
    if (mermaidLoading) return;
    mermaidLoading = true;
    var s = doc.createElement("script");
    s.src = window.wbMermaidFile; // relative: the server serves it beside the page
    s.onload = function () { window.wbMermaid(); };
    s.onerror = function () { mermaidLoading = false; s.parentNode.removeChild(s); };
    doc.head.appendChild(s);
  }

  // --- closed topics past the board's cap: fetched on request ---------------
  // The board renders the most recently closed topics up to its cap, then a
  // show-more control naming the next index (data-closed-next); a click
  // fetches the next page from the closed route into the control's place,
  // ending with the next control when more remain. The fetched pages stay
  // across board swaps (morphBoard).
  function boardTopicPresent(name) {
    return boardQ("section.topic[data-topic]").some(function (shown) { return shown.getAttribute("data-topic") === name; }) ||
      !!(window.WsCards && window.WsCards.pinnedTopic(name));
  }
  function loadClosed(btn) {
    if (!window.fetch || btn.disabled) return;
    btn.disabled = true;
    var from = +btn.getAttribute("data-closed-next");
    return fetch("/s/" + rid + "/closed?from=" + from, { credentials: "same-origin", cache: "no-store" }).then(function (r) {
      if (!r.ok) throw new Error("HTTP " + r.status);
      return r.json();
    }).then(function (res) {
      var page = frag(res.html || "").firstElementChild;
      // A board swap meanwhile replaced the control, or moved the index it
      // names (a topic closed or reopened): the page fetched may not follow
      // what shows now, and the control is clicked again.
      if (!btn.parentNode || !page || +btn.getAttribute("data-closed-next") !== from) { btn.disabled = false; return; }
      // Pinned topics and closed topics materialized by a panel jump already
      // stand on the board; slots retain their place in the page count.
      q("section.topic[data-topic]", page).forEach(function (t) {
        var name = t.getAttribute("data-topic");
        if (!boardTopicPresent(name)) return;
        var s = doc.createElement("span");
        s.className = "closed-slot";
        s.hidden = true;
        s.setAttribute("data-slot", name);
        t.parentNode.replaceChild(s, t);
      });
      btn.parentNode.replaceChild(page, btn);
      watchBoard(page);
      bandBoard(page);
      if (window.WsCards && window.WsCards.decorate) window.WsCards.decorate();
      restoreOpen(page);
      if (window.wbBindBoard) window.wbBindBoard(page);
      if (window.wbAgo) window.wbAgo(page);
      mermaid(page);
      return true;
    }, function () { btn.disabled = false; return false; });
  }

