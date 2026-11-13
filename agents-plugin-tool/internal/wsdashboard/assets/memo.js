/* Daemon-local workspace memos. No markdown, HTML, or cross-tab live sync. */
(function (window) {
  "use strict";
  window.WsMemo = { mount: function (doc, rail, panel, close) {
    var input = doc.getElementById("memo-input"), list = doc.getElementById("memo-list");
    var title = doc.getElementById("memo-workspace"), status = doc.getElementById("memo-status");
    var undo = doc.getElementById("memo-undo"), workspace = "", session = "", items = [];
    var shown = false, writable = false, busy = false, loaded = false, generation = 0, deleted = null, timer = 0, wide = false;
    function say(text) { status.textContent = text; }
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
    function render() {
      list.textContent = "";
      input.hidden = !writable; input.disabled = busy;
      items.forEach(function (m) {
        var row = doc.createElement("article"), text = doc.createElement("div");
        row.className = "memo-note"; text.className = "memo-text";
        textLinks(text, m.text); row.appendChild(text);
        if (writable) {
          text.tabIndex = 0; text.setAttribute("role", "button"); text.setAttribute("aria-label", "Edit memo");
          function edit() {
            if (busy || row.querySelector("textarea")) return;
            var area = doc.createElement("textarea"); area.value = m.text; area.maxLength = 8000;
            area.setAttribute("aria-label", "Edit memo"); text.hidden = true; row.appendChild(area); area.focus();
            area.addEventListener("keydown", function (e) {
              if (e.isComposing) return;
              if (e.key === "Escape") { e.preventDefault(); e.stopPropagation(); render(); }
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
        list.appendChild(row);
      });
      badge();
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
        if (ok) render();
        return ok;
      });
    }
    function select(ws) {
      if (!ws) return;
      var next = ws.getAttribute("data-ws");
      if (workspace === next) return;
      workspace = next; title.textContent = (ws.querySelector(".nm") || {}).textContent || next;
      title.title = next; input.value = ""; deleted = null; undo.hidden = true; clearTimeout(timer);
      if (shown) reload();
    }
    function sync(rid) {
      if (session !== rid || !workspace) {
        session = rid;
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
      if (on) reload(); else { ++generation; input.blur(); }
    }
    input.addEventListener("keydown", function (e) {
      if (e.key === "Enter" && !e.shiftKey && !e.isComposing) {
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
    window.addEventListener("resize", position);
    return { open: open, sync: sync, remember: function () {
      try { window.sessionStorage.setItem("ws.memo.resume", shown ? "1" : ""); } catch (_) {}
    }, resume: function () {
      try { var on = window.sessionStorage.getItem("ws.memo.resume") === "1"; window.sessionStorage.removeItem("ws.memo.resume"); return on; } catch (_) { return false; }
    } };
  } };
})(window);
