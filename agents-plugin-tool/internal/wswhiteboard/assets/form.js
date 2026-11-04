  // Shared by both layouts. The full page appends live.js inside the same
  // function scope; the embed runs this part alone and never touches storage.
  var doc = document;
  function q(sel, root) { return Array.prototype.slice.call((root || doc).querySelectorAll(sel)); }
  var dirty = false, copied = false, onCopied = null;
  var ta = doc.getElementById("reply-text"), hint = doc.getElementById("reply-hint");

  // --- answer form -------------------------------------------------------
  q(".card.policy").forEach(function (card) {
    q(".opt", card).forEach(function (btn) {
      btn.addEventListener("click", function () {
        var on = btn.getAttribute("aria-pressed") === "true";
        q(".opt", card).forEach(function (b) { b.setAttribute("aria-pressed", "false"); });
        btn.setAttribute("aria-pressed", on ? "false" : "true");
        build();
      });
    });
  });
  q(".card.assumed").forEach(function (card) {
    q(".btn[data-v]", card).forEach(function (btn) {
      btn.addEventListener("click", function () {
        var on = btn.getAttribute("aria-pressed") === "true";
        q(".btn[data-v]", card).forEach(function (b) { b.setAttribute("aria-pressed", "false"); });
        btn.setAttribute("aria-pressed", on ? "false" : "true");
        build();
      });
    });
  });
  q(".note-toggle").forEach(function (t) {
    t.addEventListener("click", function () {
      var n = t.closest(".card, .arow").querySelector(".note");
      n.hidden = !n.hidden; if (!n.hidden) n.focus();
    });
  });
  q(".card.assumed .btn[data-v=no]").forEach(function (b) {
    b.addEventListener("click", function () { var n = b.closest(".card").querySelector(".note"); n.hidden = false; n.focus(); });
  });
  q(".note, .ack input").forEach(function (el) { el.addEventListener("input", build); el.addEventListener("change", build); });
  // Bulk acknowledge: checks every ack row and confirms every assumed card;
  // an assumed card already marked "no" keeps that answer. Policy items stay manual.
  var all = doc.getElementById("ack-all");
  if (all) all.addEventListener("change", function () {
    q(".ack input").forEach(function (c) { c.checked = all.checked; });
    q(".card.assumed").forEach(function (card) {
      var ok = card.querySelector(".btn[data-v=ok]"), no = card.querySelector(".btn[data-v=no]");
      if (no.getAttribute("aria-pressed") !== "true") ok.setAttribute("aria-pressed", all.checked ? "true" : "false");
    });
    build();
  });
  function syncAll() {
    if (!all) return;
    var acks = q(".ack input"), cards = q(".card.assumed");
    var on = acks.filter(function (c) { return c.checked; }).length +
      cards.filter(function (c) { return c.querySelector('[aria-pressed="true"]'); }).length;
    var total = acks.length + cards.length;
    all.checked = total > 0 && on === total;
    all.indeterminate = on > 0 && on < total;
  }

  // Reply grammar: "(nn) <option>[, note: <text>]" per answered item,
  // acknowledgements grouped as "(nn), (mm) ok", parts joined by "; ".
  function build() {
    if (!ta) return;
    var parts = [], acked = [];
    q(".card.policy, .card.assumed").forEach(function (card) {
      var id = card.getAttribute("data-id");
      var pick = card.querySelector('[aria-pressed="true"]');
      var noteEl = card.querySelector(".note");
      var note = noteEl ? noteEl.value.trim() : "";
      var s = "";
      // A confirmed assumed item without a note joins the grouped "ok" segment.
      if (!note && pick && pick.getAttribute("data-v") === "ok") { acked.push("(" + id + ")"); return; }
      if (pick) s = "(" + id + ") " + (pick.getAttribute("data-opt") || pick.getAttribute("data-v"));
      if (note) s = (s || "(" + id + ")") + (s ? ", " : " ") + "note: " + note;
      if (s) parts.push(s);
    });
    // An acknowledgement with a note leaves the group: "(nn) ok, note: ..." or "(nn) note: ...".
    q(".arow").forEach(function (row) {
      var id = row.getAttribute("data-id");
      var on = row.querySelector(".ack input").checked;
      var note = row.querySelector(".note").value.trim();
      if (note) parts.push("(" + id + ") " + (on ? "ok, " : "") + "note: " + note);
      else if (on) acked.push("(" + id + ")");
    });
    if (acked.length) parts.push(acked.join(", ") + " ok");
    var text = parts.join("; ");
    // A changed reply is no longer the copied one.
    if (text !== ta.value) copied = false;
    ta.value = text;
    hint.textContent = "";
    dirty = parts.length > 0;
    syncAll();
  }

  // Copy: execCommand only; the async clipboard API is denied in sandboxed hosts.
  // When the command fails the reply stays selected for a manual copy.
  var copy = doc.getElementById("reply-copy");
  if (copy) copy.addEventListener("click", function () {
    if (!ta.value) { hint.textContent = "∅"; return; }
    ta.focus(); ta.select();
    var ok = false;
    try { ok = doc.execCommand("copy"); } catch (e) { ok = false; }
    if (ok) {
      hint.innerHTML = '<svg class="i" aria-hidden="true"><use href="#i-check"/></svg>';
      ta.blur();
      copied = true;
      if (onCopied) onCopied();
    } else {
      hint.textContent = "Ctrl+C";
    }
  });

  // --- in-page item links: open the collapsed blocks around the target ---
  q('a[href^="#item-"]').forEach(function (a) {
    a.addEventListener("click", function (e) {
      var el = doc.getElementById(a.getAttribute("href").slice(1));
      if (!el) return;
      e.preventDefault();
      for (var d = el.parentElement; d; d = d.parentElement) if (d.tagName === "DETAILS") d.open = true;
      el.scrollIntoView({ block: "center" });
      el.classList.remove("flash"); void el.offsetWidth; el.classList.add("flash");
    });
  });

  build();
