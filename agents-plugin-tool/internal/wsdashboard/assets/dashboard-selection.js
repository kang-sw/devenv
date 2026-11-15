  // --- the selection popover ---
  function supPop(c, open) {
    c.pop = open; c.pending = null;
    c.popEl.hidden = !open;
    c.chip.setAttribute("aria-expanded", String(open));
    if (!open) { c.popEl.innerHTML = ""; return; }
    c.q = ""; c.qi = -1;
    supDrawPop(c);
    var f = c.popEl.querySelector(".spq");
    if (f) f.focus({ preventScroll: true });
    if (!supChoicesBusy) {
      supChoicesBusy = true;
      tkFetch("/support/choices", undefined, function (status, v) {
        supChoicesBusy = false;
        if (status === 200 && Array.isArray(v.harnesses)) supChoices = v;
        else if (!supChoices) supChoices = { harnesses: [], error: tkErrText(status, v) };
        Object.keys(supChats).forEach(function (id) { if (supChats[id].pop) supDrawPop(supChats[id]); });
      });
    }
  }
  function supHarness(name) {
    var hs = (supChoices && supChoices.harnesses) || [];
    for (var i = 0; i < hs.length; i++) if (hs[i].harness === name) return hs[i];
    return null;
  }
  // supFuzzy scores id against query q: every character of q in order, a
  // run of adjacent ones and a start at a word boundary scoring higher; -1
  // when q is not a subsequence.
  function supFuzzy(id, qq) {
    if (!qq) return 0;
    var s = id.toLowerCase(), qs = qq.toLowerCase().replace(/\s+/g, ""), at = -1, score = 0, run = 0;
    for (var i = 0; i < qs.length; i++) {
      var j = s.indexOf(qs[i], at + 1);
      if (j < 0) return -1;
      run = j === at + 1 ? run + 1 : 0;
      score += 1 + run * 2 + (j === 0 || /[^a-z0-9]/.test(s[j - 1]) ? 3 : 0);
      at = j;
    }
    return score - s.length / 100;
  }
  function supModels(c) {
    var h = supHarness(supCur(c).harness), list = [{ id: "", label: wsT("js.sup.default") }];
    ((h && h.models) || []).forEach(function (m) { list.push({ id: m.id, label: m.id, thinking: m.thinking }); });
    if (!c.q) return list;
    return list.filter(function (m) { return m.id; }).map(function (m) { return { m: m, s: supFuzzy(m.id, c.q) }; })
      .filter(function (x) { return x.s >= 0; }).sort(function (a, b) { return b.s - a.s; }).map(function (x) { return x.m; });
  }
  function supDrawPop(c) {
    if (!c.pop) return;
    var cur = supCur(c), h = supHarness(cur.harness), hs = (supChoices && supChoices.harnesses) || [];
    var html = '<div class="spsec"><div class="splbl">' + wsT("js.sup.harness") + "</div>";
    if (!supChoices) html += '<p class="spnote">' + wsT("js.sup.loading") + "</p>";
    else if (supChoices.error) html += '<p class="spnote err">' + supEsc(supChoices.error) + "</p>";
    else {
      html += '<div class="spseg" role="radiogroup" aria-label="' + supEsc(wsT("js.sup.harness")) + '">' + hs.map(function (x) {
        var on = x.harness === cur.harness;
        return '<button type="button" role="radio" aria-checked="' + on + '" data-harness="' + supEsc(x.harness) + '"' + (x.available ? "" : " disabled") +
          (x.available ? "" : ' title="' + supEsc(wsT("js.sup.unavail", x.harness, x.error || "")) + '"') + ">" + supEsc(x.harness) + "</button>";
      }).join("") + "</div>";
    }
    if (c.pending) {
      html += '<div class="spwarn" role="alert"><span>' + supEsc(wsT("js.sup.switch.warn", c.pending)) + '</span><span class="spacts">' +
        '<button type="button" class="btn mini primary" data-sp="switch">' + wsT("js.sup.switch.ok") + "</button>" +
        '<button type="button" class="btn mini" data-sp="cancel">' + wsT("js.sup.switch.cancel") + "</button></span></div>";
    }
    html += "</div>";
    html += '<div class="spsec"><div class="splbl">' + wsT("js.sup.model") + '</div><input class="spq" type="search" autocomplete="off" spellcheck="false" placeholder="' +
      supEsc(wsT("js.sup.search")) + '" value="' + supEsc(c.q) + '"><ul class="splist" role="listbox" aria-label="' + supEsc(wsT("js.sup.model")) + '"></ul></div>';
    var levels = (h && h.efforts) || [], m = null;
    ((h && h.models) || []).forEach(function (x) { if (x.id === cur.model) m = x; });
    var noEffort = !levels.length || (m && m.thinking === false);
    html += '<div class="spsec"><div class="splbl">' + wsT("js.sup.effort") + '<span class="spval">' + supEsc(cur.effort || wsT("js.sup.default")) + "</span>" +
      (cur.effort && !noEffort ? '<button type="button" class="spreset" data-sp="effort-reset">' + wsT("js.sup.effort.reset") + "</button>" : "") + "</div>";
    if (noEffort) html += '<p class="spnote">' + wsT(levels.length ? "js.sup.noeffort" : "js.sup.noefforts") + "</p>";
    else {
      var at = levels.indexOf(cur.effort);
      html += '<div class="knob" role="slider" tabindex="0" aria-label="' + supEsc(wsT("js.sup.effort")) + '" aria-valuemin="0" aria-valuemax="' + (levels.length - 1) +
        '" aria-valuenow="' + Math.max(at, 0) + '" aria-valuetext="' + supEsc(cur.effort || wsT("js.sup.default")) + '"' + (at < 0 ? ' data-none=""' : "") + '><div class="ktrack">' +
        levels.map(function (lv, i) { return '<span class="kdot' + (i === at ? " on" : "") + '" style="left:' + supPct(i, levels.length) + '%" title="' + supEsc(lv) + '"></span>'; }).join("") +
        (at >= 0 ? '<span class="kthumb" style="left:' + supPct(at, levels.length) + '%"></span>' : "") + '</div><div class="kends"><span>' + supEsc(levels[0]) + "</span><span>" +
        supEsc(levels[levels.length - 1]) + "</span></div></div>";
    }
    if (c.conv) html += '<p class="spnote">' + wsT("js.sup.next") + "</p>";
    if (c.err) html += '<p class="spnote err">' + supEsc(c.err) + "</p>";
    var fq = doc.activeElement && doc.activeElement.classList && doc.activeElement.classList.contains("spq") && c.popEl.contains(doc.activeElement);
    var fk = doc.activeElement && doc.activeElement.classList && doc.activeElement.classList.contains("knob") && c.popEl.contains(doc.activeElement);
    var caret = fq ? [doc.activeElement.selectionStart, doc.activeElement.selectionEnd] : null;
    c.popEl.innerHTML = html;
    // the popover opens upward over the log and never past the chat's top
    var comp = c.popEl.parentNode;
    if (comp.offsetTop > 0) c.popEl.style.maxHeight = Math.max(160, comp.offsetTop - 12) + "px";
    supDrawModels(c);
    var kn = c.popEl.querySelector(".knob");
    if (kn) supKnob(c, kn, levels);
    if (fq) { var nq = c.popEl.querySelector(".spq"); nq.focus({ preventScroll: true }); try { nq.setSelectionRange(caret[0], caret[1]); } catch (x) { /* search fields */ } }
    if (fk && kn) kn.focus({ preventScroll: true });
  }
  function supPct(i, n) { return n < 2 ? 50 : i / (n - 1) * 100; }
  function supDrawModels(c) {
    var ul = c.popEl.querySelector(".splist");
    if (!ul) return;
    var cur = supCur(c).model, list = supModels(c);
    if (c.qi >= list.length) c.qi = list.length - 1;
    ul.innerHTML = list.length ? list.map(function (m, i) {
      return '<li role="option" aria-selected="' + (m.id === cur) + '" class="' + (i === c.qi ? "hi" : "") + '" data-model="' + supEsc(m.id) + '">' +
        (m.id ? supEsc(m.label) : "<i>" + supEsc(m.label) + "</i>") + "</li>";
    }).join("") : '<li class="spnone">' + wsT("js.sup.nomatch") + "</li>";
    var hi = ul.querySelector("li.hi");
    if (hi) hi.scrollIntoView({ block: "nearest" });
  }
  function supPopClick(c, e) {
    var t = e.target, b;
    if ((b = t.closest("[data-harness]"))) {
      var name = b.getAttribute("data-harness");
      if (name === supCur(c).harness) { c.pending = null; supDrawPop(c); return; }
      // switching an open conversation's harness replaces the conversation
      if (c.conv) { c.pending = name; supDrawPop(c); return; }
      supSelect(c, { harness: name });
      return;
    }
    if ((b = t.closest("[data-sp]"))) {
      var k = b.getAttribute("data-sp");
      if (k === "switch" && c.pending) { var p = c.pending; c.pending = null; supSelect(c, { harness: p }); }
      else if (k === "cancel") { c.pending = null; supDrawPop(c); }
      else if (k === "effort-reset") supSelect(c, { effort: "" });
      return;
    }
    if ((b = t.closest("li[data-model]"))) supPickModel(c, b.getAttribute("data-model"));
  }
  function supPickModel(c, id) {
    c.q = ""; c.qi = -1;
    if (id === supCur(c).model) { supDrawPop(c); return; }
    supSelect(c, { model: id });
  }
  function supPopKey(c, e) {
    if (e.key === "Escape") { e.preventDefault(); supPop(c, false); c.chip.focus(); return; }
    if (!e.target.classList.contains("spq") || e.isComposing) return;
    var list = supModels(c);
    if (e.key === "ArrowDown" || e.key === "ArrowUp") {
      e.preventDefault();
      if (!list.length) return;
      c.qi = c.qi < 0 ? (e.key === "ArrowDown" ? 0 : list.length - 1) : (c.qi + (e.key === "ArrowDown" ? 1 : list.length - 1)) % list.length;
      supDrawModelsHi(c, list);
    } else if (e.key === "Enter") {
      e.preventDefault();
      if (c.qi >= 0 && list[c.qi]) supPickModel(c, list[c.qi].id);
    }
  }
  function supDrawModelsHi(c, list) {
    q(".splist li", c.popEl).forEach(function (li, i) { li.classList.toggle("hi", i === c.qi && !!list[i]); });
    var hi = c.popEl.querySelector(".splist li.hi");
    if (hi) hi.scrollIntoView({ block: "nearest" });
  }
  // supKnob wires the effort knob: a drag moves the thumb, snapping to the
  // nearest level, and its release (or an arrow key) selects that level.
  function supKnob(c, kn, levels) {
    var track = kn.querySelector(".ktrack"), drag = null;
    function at(x) {
      var r = track.getBoundingClientRect();
      return Math.round(Math.max(0, Math.min(1, (x - r.left) / (r.width || 1))) * (levels.length - 1));
    }
    function show(i) {
      var th = track.querySelector(".kthumb");
      if (!th) { th = doc.createElement("span"); th.className = "kthumb"; track.appendChild(th); }
      th.style.left = supPct(i, levels.length) + "%";
      q(".kdot", track).forEach(function (d, j) { d.classList.toggle("on", j === i); });
      kn.removeAttribute("data-none");
      kn.setAttribute("aria-valuenow", String(i));
      kn.setAttribute("aria-valuetext", levels[i]);
      var v = c.popEl.querySelector(".spval");
      if (v) v.textContent = levels[i];
    }
    function pick(i) { if (levels[i] !== supCur(c).effort) supSelect(c, { effort: levels[i] }); }
    kn.addEventListener("pointerdown", function (e) {
      if (e.button) return;
      drag = { i: at(e.clientX) };
      try { kn.setPointerCapture(e.pointerId); } catch (x) { /* the moves still land */ }
      show(drag.i);
      e.preventDefault();
      kn.focus({ preventScroll: true });
    });
    kn.addEventListener("pointermove", function (e) {
      if (!drag) return;
      var i = at(e.clientX);
      if (i !== drag.i) { drag.i = i; show(i); }
    });
    function end() { if (!drag) return; var i = drag.i; drag = null; pick(i); }
    kn.addEventListener("pointerup", end);
    kn.addEventListener("pointercancel", function () { drag = null; supDrawPop(c); });
    kn.addEventListener("keydown", function (e) {
      var cur = levels.indexOf(supCur(c).effort), i = cur;
      if (e.key === "ArrowRight" || e.key === "ArrowUp") i = Math.min(levels.length - 1, cur + 1);
      else if (e.key === "ArrowLeft" || e.key === "ArrowDown") i = cur < 0 ? 0 : Math.max(0, cur - 1);
      else if (e.key === "Home") i = 0;
      else if (e.key === "End") i = levels.length - 1;
      else return;
      e.preventDefault();
      if (i !== cur) { show(i); pick(i); }
    });
  }
  // A click outside an open popover (and outside its chip) closes it.
  doc.addEventListener("click", function (e) {
    Object.keys(supChats).forEach(function (id) {
      var c = supChats[id];
      if (c.pop && !c.popEl.contains(e.target) && !c.chip.contains(e.target) && e.target.isConnected) supPop(c, false);
    });
  });

