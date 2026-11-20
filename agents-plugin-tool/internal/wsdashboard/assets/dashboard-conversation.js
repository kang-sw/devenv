  // --- conversation entries: merged by key ----------------------------------
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
      Array.prototype.forEach.call(box.children, function (c) { var k = c.getAttribute("data-entry"); if (k) box.wsKeys[k] = c; });
    }
    return box.wsKeys;
  }
  function resetEntries(box) { box.textContent = ""; box.wsKeys = Object.create(null); }
  // mergeEntries inserts each node of src whose name box lacks at its sorted
  // place and returns inserted/replaced nodes, in order. Existing entries
  // stay put unless their mail-wake presentation changes: a hidden mail
  // marker must replace its row (and restore it when the association expires).
  function mergeEntries(box, src, changed) {
    var have = entryKeys(box), added = [];
    Array.prototype.slice.call(src.children).forEach(function (n) {
      var k = n.getAttribute("data-entry");
      if (!k) return;
      var old = have[k];
      if (old) {
        if (!(changed && changed[k]) && old.getAttribute("data-mail-wake") === n.getAttribute("data-mail-wake")) return;
        box.replaceChild(n, old);
      } else {
        // New entries are usually the newest (live) or the oldest (history),
        // so walk back from the end only past entries that sort after k.
        var c = box.lastElementChild;
        while (c && (c.getAttribute("data-entry") || "") > k) c = c.previousElementSibling;
        box.insertBefore(n, c ? c.nextSibling : box.firstChild);
      }
      have[k] = n;
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
  // Canonical transport inputs never come from the formatted, history-extended
  // live DOM. A full event establishes the shell and newest window; a delta
  // must prove the same order and replace only complete single keyed rows.
  var ENTRY_SLOT = /<!--ws-entry-row-(\d+)-->/g;
  function entryInputFull(html) {
    if (typeof html !== "string" || html.indexOf("<!--ws-entry-row-") >= 0) return null;
    var src = frag(html), boxes = src.querySelectorAll(".centries");
    if (boxes.length !== 1) return null;
    var box = boxes[0], rows = Object.create(null), order = [];
    if (Array.from(box.childNodes).some(function (n) { return n.nodeType !== 1 && (n.nodeType !== 3 || n.textContent.trim()); })) return null;
    if (Array.from(box.children).some(function (n) {
      var key = n.getAttribute("data-entry");
      if (!key || rows[key]) return true;
      rows[key] = { html: n.outerHTML }; order.push(key);
      n.replaceWith(doc.createComment("ws-entry-row-" + (order.length - 1)));
      return false;
    })) return null;
    var holder = doc.createElement("div"); holder.appendChild(src);
    return { shell: holder.innerHTML, order: order, rows: rows };
  }
  function entryRowNode(html, key) {
    if (typeof html !== "string" || html.indexOf("<!--ws-entry-row-") >= 0) return null;
    var src = frag(html), n = src.firstElementChild;
    return src.childNodes.length === 1 && n && n.getAttribute("data-entry") === key && !n.matches(".centries") && !n.querySelector(".centries") ? n : null;
  }
  function entryInputPatch(base, ev) { return keyedInputPatch(base, ev, entryRowNode); }
  function entryInputHTML(input) { return keyedInputHTML(input, ENTRY_SLOT); }

  // mergeEntriesRegion applies an entries region event: the title bar (its
  // counts and model-history source) is synced, and the entries are merged. A window
  // past a gap starts the box over; a box at the bottom (pin) sheds its oldest
  // entries past the cap. Either way the older history is loaded again from
  // what the box then holds.
  function mergeEntriesRegion(el, html, pin, previous) {
    var next = typeof html === "string" ? null : html, box = entriesBox(), changed = Object.create(null);
    // Without a rendered baseline, the differing full's same-key rows may
    // have changed since page render (or an unparseable full). The swap owner
    // already bypasses this merge for equal hashes, preserving no-remount.
    if (next) next.order.forEach(function (key) {
      if (!previous || (previous.rows[key] && previous.rows[key].html !== next.rows[key].html)) changed[key] = true;
    });
    if (box && next && previous && next.shell === previous.shell && keyedSame(next.order, previous.order)) {
      var incoming = doc.createElement("div"), have = entryKeys(box);
      next.order.forEach(function (key) {
        if (!have[key] || changed[key]) incoming.appendChild(entryRowNode(next.rows[key].html, key));
      });
      var inserted = mergeEntries(box, incoming, changed);
      inserted.forEach(sweepOutImgs); mermaid(inserted);
      if (pin && evictEntries(box)) histReset();
      return;
    }
    html = next ? entryInputHTML(next) : html;
    var src = frag(html), head = el.querySelector(".thead"), nhead = src.querySelector(".thead");
    var nbox = src.querySelector(".centries");
    if (!box || !nbox) { el.innerHTML = html; restoreOpen(el); fmtTimes(el); sweepOutImgs(el); mermaid(el); histReset(); return; }
    if (head && nhead) { head.parentNode.replaceChild(nhead, head); fmtTimes(nhead); }
    if (gapped(box, nbox)) { resetEntries(box); histReset(); }
    var added = mergeEntries(box, nbox, changed);
    added.forEach(sweepOutImgs);
    mermaid(added);
    if (pin && evictEntries(box)) histReset();
  }

  // Model history is a click-only popover sourced by the entries title bar.
  // The popover lives outside the swapped region; syncChead calls
  // modelHistorySync after each entries refresh so an open popover keeps its
  // state while its source and anchor are replaced.
  var modelHistoryPop = null, modelHistoryOpen = false;
  function modelHistoryTrigger() { return doc.querySelector(".modelhistory-trigger"); }
  function modelHistorySource(btn) {
    return btn && btn.parentNode && btn.parentNode.querySelector("template.modelhistory-source");
  }
  function modelHistoryPlace() {
    var btn = modelHistoryTrigger();
    if (!modelHistoryPop || modelHistoryPop.hidden || !btn || !window.WsRail) return;
    var a = btn.getBoundingClientRect();
    var at = window.WsRail.popPlace(a, modelHistoryPop.offsetWidth, modelHistoryPop.offsetHeight,
      root.clientWidth || window.innerWidth, root.clientHeight || window.innerHeight);
    modelHistoryPop.style.top = at.top + "px";
    modelHistoryPop.style.left = at.left + "px";
  }
  function modelHistoryClose(refocus) {
    modelHistoryOpen = false;
    if (modelHistoryPop) modelHistoryPop.hidden = true;
    var btn = modelHistoryTrigger();
    if (btn) {
      btn.setAttribute("aria-expanded", "false");
      if (refocus) btn.focus();
    }
  }
  function modelHistorySync() {
    var btn = modelHistoryTrigger(), src = modelHistorySource(btn);
    if (!btn || !src) {
      if (modelHistoryPop) modelHistoryPop.hidden = true;
      return;
    }
    btn.setAttribute("aria-expanded", String(!!modelHistoryOpen));
    if (!modelHistoryOpen) return;
    if (!modelHistoryPop) {
      modelHistoryPop = doc.createElement("div");
      modelHistoryPop.id = "modelhistory-popover";
      modelHistoryPop.className = "rpop modelhistory-popover";
      modelHistoryPop.setAttribute("role", "dialog");
      modelHistoryPop.setAttribute("aria-label", btn.getAttribute("aria-label") || "");
      doc.body.appendChild(modelHistoryPop);
    }
    modelHistoryPop.innerHTML = src.innerHTML;
    modelHistoryPop.hidden = false;
    fmtTimes(modelHistoryPop);
    modelHistoryPlace();
  }
  function modelHistoryToggle(btn) {
    if (modelHistoryOpen) { modelHistoryClose(true); return; }
    modelHistoryOpen = true;
    btn.setAttribute("aria-expanded", "true");
    modelHistorySync();
  }
  doc.addEventListener("click", function (e) {
    var btn = e.target && e.target.closest && e.target.closest(".modelhistory-trigger");
    if (btn) { e.preventDefault(); modelHistoryToggle(btn); return; }
    if (modelHistoryOpen && (!modelHistoryPop || !modelHistoryPop.contains(e.target))) modelHistoryClose(false);
  }, true);
  window.addEventListener("keydown", function (e) {
    if (e.key !== "Escape" || !modelHistoryOpen) return;
    e.preventDefault();
    e.stopPropagation();
    modelHistoryClose(true);
  }, true);
  window.addEventListener("resize", modelHistoryPlace);
  doc.addEventListener("scroll", modelHistoryPlace, true);

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
    var epoch = ++effectJumpEpoch, seen = Object.create(null), waited = false;
    if (doc.body.classList.contains("tk-paged") && tkApi) tkApi.leavePage();
    if (grid && !tabShown("board")) setTab("board");
    function result(topic, exact) { if (epoch === effectJumpEpoch && done) done(topic, exact); }
    function step() {
      if (epoch !== effectJumpEpoch) return;
      if (window.wbPill && window.wbPill.apply && !window.wbPill.apply(step)) return;
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
          if (window.wbHistory && window.wbHistory.loadTarget) target = window.wbHistory.loadTarget(topic, id, unfold);
          else target = /^item-/.test(id) ? topic.querySelector('[id="' + id + '"]') : null;
          if (!target || target.id !== id || !inBoard(target) || target.closest("section.topic[data-topic]") !== topic ||
              (/^item-/.test(id) && !target.matches(".card.policy[data-id], .arow[data-id], .ckrow[data-id], .srow"))) target = null;
          // A target in history the page has not fetched yet: decide once
          // its topic arrives.
          var waiting = !target && !waited && window.wbHistory && window.wbHistory.loading && window.wbHistory.loading(id);
          if (waiting) { waited = true; waiting.then(reveal); return; }
        }
        var exact = !!target;
        if (!target) target = topic;
        if (unfold) unfold(target);
        // A target on a docked page not shown (or, in the narrow view, on
        // the Main Board while a docked page shows) has its page shown first.
        if (cards && cards.activate) cards.activate(target);
        for (var p = target; p && inBoard(p); p = p.parentNode) if (p.tagName === "DETAILS") p.open = true;
        // loadTarget owns lazy-history positioning, including version folds.
        if (!target.closest(".wb-history")) {
          target.scrollIntoView({ block: "start" });
          if (window.wbHistory && window.wbHistory.keep) window.wbHistory.keep(target);
          // Entry/item flashes belong to loadTarget; topic-only navigation
          // (including an absent element's fallback) uses the existing pulse.
          if (target === topic || !window.wbHistory || !window.wbHistory.loadTarget) {
            target.classList.remove("palette-flash");
            void target.offsetWidth;
            target.classList.add("palette-flash");
            setTimeout(function () { target.classList.remove("palette-flash"); }, 1600);
          }
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
    if (t.closest("[data-wb-nav]")) return;
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

  // User chips and references in committed previews share the card jump owner.
  // Delegation also covers held bubbles and subsequently paged conversation rows.
  function conversationBoardActivate(e) {
    if (e.type === "keydown" ? e.key !== "Enter" : e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return;
    var link = e.target && e.target.closest && e.target.closest("[data-wb-nav]");
    var box = entriesBox(), held = region("convlive");
    if (!link || !((box && box.contains(link)) || (held && held.contains(link)))) return;
    var selection = window.getSelection && window.getSelection();
    if (e.type !== "keydown" && selection && !selection.isCollapsed) return;
    e.preventDefault();
    var topic = link.getAttribute("data-wb-topic"), target = link.getAttribute("data-wb-nav");
    if (!topic) { toast(wsT("js.whiteboard.jump.itemmissing")); return; }
    effectJump(topic, target, function (found, exact) {
      if (!found || !exact) toast(wsT("js.whiteboard.jump.itemmissing"));
    });
  }
  doc.addEventListener("click", conversationBoardActivate);
  doc.addEventListener("keydown", conversationBoardActivate);

  // --- chat block actions (msgacts, dashboard.tmpl) ---------------------------
  // Copy fetches the block's stored text from the entry text route
  // (entry_text_route.go) and copies it quietly: the button confirms
  // (data-done, with its pill pinned) for 1.6 s, so no toast doubles it. A
  // batch's bubbles share one entry key; the route answers one text per
  // bubble, in order, and the bubble's index picks its own. Where the async
  // Clipboard API takes a pending item, the write starts inside the click,
  // so a browser that ties clipboard writes to the gesture still allows it
  // after the fetch; elsewhere the fetched text goes through copyText. A
  // disabled action (rewind, edit) only shows its tooltip.
  var ACT_CONFIRM_MS = 1600;
  function entryText(msg) {
    var holder = msg.closest("[data-entry]"), key = holder && holder.getAttribute("data-entry");
    if (!key || !window.fetch) return Promise.reject(new Error("no entry"));
    var index = holder === msg ? 0 : Array.prototype.indexOf.call(holder.querySelectorAll(".msg.user"), msg);
    return fetch("/s/" + rid + "/entry?key=" + encodeURIComponent(key), { credentials: "same-origin", cache: "no-store" }).then(function (r) {
      if (!r.ok) throw new Error("HTTP " + r.status);
      return r.json();
    }).then(function (ans) {
      var text = ans && ans.blocks && ans.blocks[index];
      if (typeof text !== "string") throw new Error("no text");
      return text;
    });
  }
  function copyEntry(msg) {
    var text = entryText(msg);
    function viaCopyText() { return text.then(function (s) { return copyText(s, true); }); }
    if (window.ClipboardItem && window.isSecureContext && navigator.clipboard && navigator.clipboard.write) {
      var item = null;
      try { item = new ClipboardItem({ "text/plain": text.then(function (s) { return new Blob([s], { type: "text/plain" }); }) }); } catch (e) { item = null; }
      if (item) return navigator.clipboard.write([item]).then(function () { return true; }, viaCopyText);
    }
    return viaCopyText();
  }
  // actConfirm shows b's tooltip (a disabled action), or its done state with
  // its pill pinned (a copy), for ACT_CONFIRM_MS.
  function actConfirm(b, done) {
    var bar = b.parentNode;
    clearTimeout(b.wsActTimer);
    b.setAttribute(done ? "data-done" : "data-tipshow", "");
    if (done) bar.setAttribute("data-pinned", "");
    b.wsActTimer = setTimeout(function () {
      b.removeAttribute("data-done");
      b.removeAttribute("data-tipshow");
      bar.removeAttribute("data-pinned");
    }, ACT_CONFIRM_MS);
  }
  function msgActClick(e) {
    var t = e.target, box = entriesBox();
    var b = t && t.closest ? t.closest(".d-act") : null;
    if (!b || !box || !box.contains(b)) return;
    e.preventDefault();
    if (b.getAttribute("aria-disabled") === "true") { actConfirm(b, false); return; }
    if (b.getAttribute("data-act") !== "copy" || b.wsActBusy) return;
    b.wsActBusy = true;
    copyEntry(b.closest(".msg")).then(function (ok) {
      b.wsActBusy = false;
      if (!ok) throw new Error("copy refused");
      actConfirm(b, true);
    }).catch(function () {
      b.wsActBusy = false;
      toast(wsT("js.entry.copyfail"));
    });
  }
  doc.addEventListener("click", msgActClick);

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
      // Pagination supersedes any WS restoration queued before this fetch
      // completed; its compensated reading position is now authoritative.
      scrollRestore.delete(convo);
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
  function boardImageStamp() {
    var marker = doc.querySelector('template[data-history-images]');
    return marker && marker.getAttribute('data-history-images');
  }
  function loadClosed(btn) {
    if (!window.fetch || btn.disabled) return;
    btn.disabled = true;
    var from = +btn.getAttribute("data-closed-next"), images = boardImageStamp();
    return fetch("/s/" + rid + "/closed?from=" + from, { credentials: "same-origin", cache: "no-store" }).then(function (r) {
      if (!r.ok) throw new Error("HTTP " + r.status);
      return r.json();
    }).then(function (res) {
      var page = frag(res.html || "").firstElementChild;
      // A board swap meanwhile replaced the control, or moved the index it
      // names (a topic closed or reopened): the page fetched may not follow
      // what shows now, and the control is clicked again.
      if (!btn.parentNode || !page || +btn.getAttribute("data-closed-next") !== from) { btn.disabled = false; return; }
      if (images !== boardImageStamp()) { btn.disabled = false; return loadClosed(btn); }
      // Pinned topics and closed topics materialized by a panel jump already
      // stand on the board; slots retain their place in the page count.
      q("section.topic[data-topic]", page).forEach(function (t) {
        var name = t.getAttribute("data-topic");
        if (!boardTopicPresent(name)) { boardPaged[name] = t.cloneNode(true); return; }
        var s = doc.createElement("span");
        s.className = "closed-slot";
        s.hidden = true;
        s.setAttribute("data-slot", name);
        t.parentNode.replaceChild(s, t);
      });
      var kept = boardKeep();
      btn.parentNode.replaceChild(page, btn);
      watchBoard(page);
      bandBoard(page);
      if (window.WsCards && window.WsCards.decorate) window.WsCards.decorate();
      restoreOpen(page);
      if (window.wbBindBoard) window.wbBindBoard(page);
      bindCapsuleNotes();
      if (window.wbAgo) window.wbAgo(page);
      mermaid(page);
      if (kept) kept.h.apply();
      return true;
    }, function () { btn.disabled = false; return false; });
  }

