/* Daemon-local workspace memos. No markdown, HTML, or cross-tab live sync. */
(function (window) {
  "use strict";
  // Shortest column first, in list order; ties go left. keep maps a memo id to
  // the column it already holds, so saving an edit never moves other notes.
  function layout(ids, heights, count, keep) {
    var sums = [], out = {};
    for (var c = 0; c < count; c++) sums.push(0);
    ids.forEach(function (id, i) {
      var col = keep && keep[id] < count ? keep[id] : sums.indexOf(Math.min.apply(null, sums));
      out[id] = col; sums[col] += heights[i] || 0;
    });
    return out;
  }
  var KEYS_FULL = "<span><kbd>Enter</kbd> save</span><span><kbd>Shift</kbd>+<kbd>Enter</kbd> newline</span><span><kbd>Esc</kbd> cancel</span>";
  var KEYS_SHORT = "<span><kbd>↵</kbd> save</span><span><kbd>⇧↵</kbd> newline</span><span><kbd>Esc</kbd> cancel</span>";
  window.WsMemo = { layout: layout, mount: function (doc, rail, panel, close) {
    var input = doc.getElementById("memo-input"), list = doc.getElementById("memo-list");
    var title = doc.getElementById("memo-workspace"), status = doc.getElementById("memo-status");
    var counter = doc.getElementById("memo-n");
    var undo = doc.getElementById("memo-undo"), workspace = "", session = "", items = [];
    var shown = false, writable = false, busy = false, loaded = false, generation = 0, deleted = null, timer = 0, wide = false;
    // slots: memo id -> column from the last placement; placed: the list width
    // it was made for; stale: the width changed while an edit was open.
    var slots = {}, placed = -1, stale = false;
    var narrow = window.matchMedia ? window.matchMedia("(max-width: 699.98px)") : null;
    function columns() { return (narrow && narrow.matches) || list.clientWidth < 390 ? 1 : 2; }
    var pickers = [], topicGeneration = 0;
    function closePickers() {
      ++topicGeneration;
      pickers.forEach(function (picker) { if (picker) picker.close(); });
    }
    function topics(done) {
      var rid = session, gen = topicGeneration, sourceWindow = null;
      if (!rid) { done([]); return; }
      try {
        var frame = doc.querySelector(".sframe.on");
        sourceWindow = frame ? frame.contentWindow : window;
        if (sourceWindow.document.documentElement.getAttribute("data-rid") !== rid || !sourceWindow.wsCapsuleTopics) { done([]); return; }
        sourceWindow.wsCapsuleTopics(function (rows) {
          if (gen === topicGeneration && session === rid) done(rows);
        });
      } catch (_) { done([]); }
    }
    function attach(area) {
      if (!window.WsCapsules) return;
      window.WsCapsules.attach(area);
      pickers.push(window.WsCapsules.topicPicker(area, topics));
    }
    function say(text) { status.textContent = text; }
    // The server refuses trimmed text over 8000 UTF-8 bytes (a lone surrogate
    // is stored as U+FFFD, 3 bytes); maxLength counts UTF-16 units and cannot.
    function byteLength(text) {
      var n = 0;
      for (var i = 0; i < text.length; i++) {
        var c = text.charCodeAt(i);
        if (c < 0x80) n += 1;
        else if (c < 0x800) n += 2;
        else if (c >= 0xd800 && c < 0xdc00 && i + 1 < text.length && (text.charCodeAt(i + 1) & 0xfc00) === 0xdc00) { n += 4; i++; }
        else n += 3;
      }
      return n;
    }
    var overMsg = "";
    function fits(text) {
      var n = byteLength(text.trim());
      if (n <= 8000) {
        if (overMsg && status.textContent === overMsg) say("");
        overMsg = ""; return true;
      }
      overMsg = wsT("js.memo.toolong", n, 8000); say(overMsg); return false;
    }
    function badge() {
      if (!loaded) return;
      Array.prototype.forEach.call(rail.querySelectorAll(".rws"), function (ws) {
        if (ws.getAttribute("data-ws") === workspace) {
          var count = ws.querySelector(".memo-count span");
          if (count) count.textContent = String(items.length);
        }
      });
    }
    function textLinks(box, text) {
      var re = /https?:\/\/[^\s<>]+/g, from = 0, match;
      while ((match = re.exec(text))) {
        box.appendChild(doc.createTextNode(text.slice(from, match.index)));
        var a = doc.createElement("a"); a.textContent = match[0]; a.href = match[0];
        a.target = "_blank"; a.rel = "noopener noreferrer"; box.appendChild(a);
        from = re.lastIndex;
      }
      box.appendChild(doc.createTextNode(text.slice(from)));
    }
    // keep: hold each memo in its previous column (edit save, edit cancel).
    // Everything else re-places, as does a keep render after a width change.
    function render(keep) {
      closePickers(); pickers = pickers.slice(0, 1);
      list.textContent = "";
      if (window.WsCapsules && window.WsCapsules.sweep) window.WsCapsules.sweep();
      input.hidden = !writable; input.disabled = busy;
      if (counter) counter.textContent = loaded ? String(items.length) : "";
      if (loaded && !items.length) {
        var empty = doc.createElement("p"); empty.className = "memo-empty"; empty.textContent = "No memos yet.";
        list.appendChild(empty);
      }
      var count = items.length ? columns() : 1, cols = [], rows = [];
      for (var c = 0; items.length && c < count; c++) {
        cols.push(list.appendChild(doc.createElement("div"))); cols[c].className = "memo-col";
      }
      items.forEach(function (m) {
        var row = doc.createElement("article"), text = doc.createElement("div");
        row.className = "memo-note"; text.className = "memo-text";
        textLinks(text, m.text); row.appendChild(text);
        if (writable) {
          text.tabIndex = 0; text.setAttribute("role", "button"); text.setAttribute("aria-label", "Edit memo");
          function edit() {
            if (busy || row.querySelector("textarea")) return;
            var area = doc.createElement("textarea"); area.value = m.text; area.maxLength = 8000;
            area.addEventListener("input", function () { fits(area.value); });
            area.setAttribute("aria-label", "Edit memo"); text.hidden = true; row.appendChild(area); attach(area);
            var keys = doc.createElement("div"); keys.className = "memo-hint"; keys.setAttribute("aria-hidden", "true");
            keys.innerHTML = count > 1 ? KEYS_SHORT : KEYS_FULL;
            row.appendChild(keys); row.className = "memo-note memo-editing"; area.focus();
            area.addEventListener("keydown", function (e) {
              if (e.defaultPrevented || e.isComposing || e.keyCode === 229) return;
              if (e.key === "Escape") {
                // The cancelled edit's over-limit refusal no longer applies; recheck the add draft instead.
                e.preventDefault(); e.stopPropagation(); fits(input.value); render(true);
              }
              if (e.key === "Enter" && !e.shiftKey) {
                e.preventDefault();
                if (area.value.trim()) change("edit", m.id, area.value);
              }
            });
          }
          text.addEventListener("click", function (e) { if (!e.target.closest("a")) edit(); });
          text.addEventListener("keydown", function (e) { if (e.key === "Enter" && e.target === text) { e.preventDefault(); edit(); } });
          var del = doc.createElement("button"); del.className = "memo-delete";
          del.textContent = "✕"; del.setAttribute("aria-label", "Delete memo"); del.disabled = busy;
          del.addEventListener("click", function () {
            change("delete", m.id, "").then(function (ok) {
              if (!ok) return;
              clearTimeout(timer); deleted = { workspace: workspace, text: m.text }; undo.hidden = false;
              timer = setTimeout(function () { deleted = null; undo.hidden = true; }, 5000);
            });
          });
          row.appendChild(del);
        }
        cols[0].appendChild(row); rows.push(row);
      });
      // Every column has the same width, so heights measured in the first one
      // hold in any; one layout read, then the notes move to their columns.
      // Each note also costs the column's row gap, so a column of many short
      // notes is not mistaken for the shorter one.
      var gap = count > 1 && window.getComputedStyle ? parseFloat(window.getComputedStyle(cols[0]).rowGap) || 0 : 0;
      var ids = items.map(function (m) { return m.id; }), prior = keep && !stale ? slots : null, width = list.clientWidth;
      function place() {
        slots = layout(ids, count > 1 ? rows.map(function (row) { return row.offsetHeight + gap; }) : [], count, prior);
        rows.forEach(function (row, i) { cols[slots[items[i].id]].appendChild(row); });
      }
      place();
      // The one-column measuring stack can show a layout scrollbar the split
      // columns do not (or the reverse), so its heights were read at another
      // width; measure once more in place at the final width.
      if (count > 1 && list.clientWidth !== width) place();
      placed = list.clientWidth; stale = false;
      badge();
    }
    function refit() {
      if (!shown || !loaded || list.clientWidth === placed) return;
      if (list.querySelector("textarea")) stale = true; else render();
    }
    function request(url, options) {
      return window.fetch(url, options).then(function (r) {
        if (!r.ok) return r.text().then(function (text) { throw new Error(text || "Memo request failed"); });
        return r.json();
      });
    }
    function reload() {
      var gen = ++generation;
      loaded = false; writable = false; items = []; render(); say("Loading…");
      if (!workspace) { say("Choose a workspace in the session rail."); return; }
      request("/rail/memos?workspace=" + encodeURIComponent(workspace), { credentials: "same-origin" }).then(function (data) {
        if (gen !== generation || !shown) return;
        loaded = true; items = data.memos; writable = data.writable; render(); say(writable ? "" : "Read-only on LAN");
        if (writable) input.focus();
      }).catch(function (e) { if (gen === generation) say(e.message); });
    }
    function change(action, id, text) {
      if (busy || !writable) return Promise.resolve(false);
      if (action !== "delete" && !fits(text)) return Promise.resolve(false);
      var gen = generation;
      busy = true; input.disabled = true; say("");
      return request("/rail/memos", { method: "POST", credentials: "same-origin", headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ workspace: workspace, action: action, id: id, text: text })
      }).then(function (data) {
        if (gen !== generation) return false;
        items = data.memos;
        if (action === "add") input.value = "";
        return true;
      }).catch(function (e) { if (gen === generation) say(e.message); return false; }).then(function (ok) {
        busy = false; input.disabled = false;
        if (ok) render(action === "edit");
        return ok;
      });
    }
    function select(ws) {
      if (!ws) return;
      var next = ws.getAttribute("data-ws");
      if (workspace === next) return;
      closePickers();
      workspace = next; title.textContent = (ws.querySelector(".nm") || {}).textContent || next;
      title.title = next; input.value = ""; deleted = null; undo.hidden = true; clearTimeout(timer);
      if (shown) reload();
    }
    function sync(rid) {
      if (session !== rid || !workspace) {
        closePickers(); session = rid;
        var rows = rail.querySelectorAll(".rs"), ws = null;
        Array.prototype.forEach.call(rows, function (row) { if (row.getAttribute("data-rid") === rid) ws = row.closest(".rws"); });
        select(ws || rail.querySelector(".rws"));
      }
      if (shown) badge();
    }
    function position() {
      var rect = rail.getBoundingClientRect();
      panel.style.setProperty("--memo-left", (wide ? rect.right : 12 + rect.width) + "px");
      panel.style.setProperty("--memo-top", (wide ? Math.max(12, rect.top) : 12) + "px");
    }
    function open(on, isWide) {
      shown = on; wide = isWide; panel.setAttribute("aria-hidden", String(!on));
      if ("inert" in panel) panel.inert = !on;
      position();
      if (on) reload(); else { ++generation; closePickers(); input.blur(); }
    }
    attach(input);
    input.addEventListener("input", function () { fits(input.value); });
    input.addEventListener("keydown", function (e) {
      if (e.defaultPrevented || e.isComposing || e.keyCode === 229) return;
      if (e.key === "Enter" && !e.shiftKey) {
        e.preventDefault(); if (input.value.trim()) change("add", "", input.value);
      }
    });
    undo.querySelector("button").addEventListener("click", function () {
      if (!deleted || deleted.workspace !== workspace) return;
      var text = deleted.text, draft = input.value;
      change("add", "", text).then(function (ok) { if (ok) { input.value = draft; deleted = null; undo.hidden = true; clearTimeout(timer); } });
    });
    doc.getElementById("memo-close").addEventListener("click", close);
    rail.addEventListener("click", function (e) {
      var button = e.target.closest(".memo-count");
      if (button) { e.preventDefault(); e.stopPropagation(); select(button.closest(".rws")); close(true); }
    }, true);
    window.addEventListener("resize", function () { position(); refit(); });
    return { open: open, sync: sync, remember: function () {
      try { window.sessionStorage.setItem("ws.memo.resume", shown ? "1" : ""); } catch (_) {}
    }, resume: function () {
      try { var on = window.sessionStorage.getItem("ws.memo.resume") === "1"; window.sessionStorage.removeItem("ws.memo.resume"); return on; } catch (_) { return false; }
    } };
  } };
})(window);
