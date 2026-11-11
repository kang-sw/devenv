  // Shared by both layouts. The full page appends live.js inside the same
  // function scope; the embed runs this part alone and never touches storage.
  var doc = document;
  function q(sel, root) { return Array.prototype.slice.call((root || doc).querySelectorAll(sel)); }
  var dirty = false, copied = false, onCopied = null;
  var ta, hint, all;
  // replyEls looks the reply bar's controls up again: a dashboard board swap
  // can replace the bar while it keeps the topics, whose controls stay wired
  // to the run of this script that bound them.
  function replyEls() {
    ta = doc.getElementById("reply-text");
    hint = doc.getElementById("reply-hint");
    all = doc.getElementById("ack-all");
  }
  replyEls();

  // --- answer form -------------------------------------------------------
  // bind wires the controls under root; the page binds the whole document
  // once, and a served page binds the closed topics it fetches later
  // (window.wbBindBoard). A dashboard board swap runs this script again
  // over a document whose unchanged topics it kept, so each control is
  // marked when wired (once) and never wired twice.
  function once(el) {
    if (el.wbBound) return false;
    el.wbBound = true;
    return true;
  }
  function bind(root) {
    q(".card.policy", root).forEach(function (card) {
      q(".opt", card).forEach(function (btn) {
        if (!once(btn)) return;
        function pick() {
          var on = btn.getAttribute("aria-pressed") === "true";
          q(".opt", card).forEach(function (b) { b.setAttribute("aria-pressed", "false"); });
          btn.setAttribute("aria-pressed", on ? "false" : "true");
          build();
        }
        // A link inside an option follows the link and picks nothing; a
        // summary inside one toggles its details and picks nothing.
        btn.addEventListener("click", function (e) { if (!e.target.closest("a, summary")) pick(); });
        // A stacked option card is a role="button" element, not a native
        // button, so it takes Enter and Space itself.
        if (btn.getAttribute("role") === "button") btn.addEventListener("keydown", function (e) {
          if (e.target !== btn || (e.key !== "Enter" && e.key !== " ")) return;
          e.preventDefault();
          pick();
        });
      });
    });
    q(".note-toggle", root).forEach(function (t) {
      if (once(t)) t.addEventListener("click", function () {
        var n = t.closest(".card, .arow, .srow").querySelector(".note");
        n.hidden = !n.hidden; if (!n.hidden) { grow(n); n.focus(); }
      });
    });
    // A topic's pen opens the note under its bar: a comment or question on
    // the topic as a whole, which the reply carries as "[topic] text".
    q(".tnote-toggle", root).forEach(function (t) {
      if (once(t)) t.addEventListener("click", function () {
        var n = t.closest(".topic").querySelector(".tnote");
        n.hidden = !n.hidden; if (!n.hidden) { grow(n); n.focus(); }
      });
    });
    q(".note, .ack input", root).forEach(function (el) {
      if (!once(el)) return;
      if (el.classList.contains("note")) el.addEventListener("input", function () { grow(el); });
      el.addEventListener("input", build);
      el.addEventListener("change", build);
    });
    inPageLinks(root);
  }
  // Size a note textarea to its content; scrollHeight is 0 while hidden.
  function grow(n) { n.style.height = "auto"; if (n.scrollHeight) n.style.height = n.scrollHeight + "px"; }
  // Bulk acknowledge: checks every ack row. Policy items stay manual.
  if (all && once(all)) all.addEventListener("change", function () {
    var on = this.checked;
    q(".ack input").forEach(function (c) { c.checked = on; });
    build();
  });
  function syncAll() {
    if (!all) return;
    var acks = q(".ack input");
    var on = acks.filter(function (c) { return c.checked; }).length;
    var total = acks.length;
    all.checked = total > 0 && on === total;
    all.indeterminate = on > 0 && on < total;
  }

  // Reply grammar: "(nn) <option>[, note: <text>]" per answered item,
  // acknowledgements grouped as "(nn), (mm) ok", parts joined by "; ", then
  // one "[topic] text" line per topic note.
  function build() {
    replyEls();
    if (!ta) return;
    var parts = [], acked = [];
    q(".card.policy").forEach(function (card) {
      var id = card.getAttribute("data-id");
      var pick = card.querySelector('[aria-pressed="true"]');
      var noteEl = card.querySelector(".note");
      var note = noteEl ? noteEl.value.trim() : "";
      var s = "";
      if (pick) s = "(" + id + ") " + pick.getAttribute("data-opt");
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
    // An assumed item reads as settled; its pen still takes a veto or a
    // comment, sent as "(nn) note: ...".
    q(".srow[data-id]").forEach(function (row) {
      var note = row.querySelector(".note").value.trim();
      if (note) parts.push("(" + row.getAttribute("data-id") + ") note: " + note);
    });
    if (acked.length) parts.push(acked.join(", ") + " ok");
    // Topic notes follow the item segments, one line each: "[topic] text".
    var topics = q(".tnote").map(function (n) {
      var note = n.value.trim();
      return note ? "[" + n.getAttribute("data-topic") + "] " + note : "";
    }).filter(Boolean);
    var text = parts.join("; ");
    if (topics.length) text += (text ? "\n" : "") + topics.join("\n");
    // A changed reply is no longer the copied one.
    if (text !== ta.value) copied = false;
    ta.value = text;
    // Topic note lines make the reply several lines tall.
    ta.style.height = "";
    if (text.indexOf("\n") >= 0) grow(ta);
    hint.textContent = "";
    dirty = text !== "";
    // A board with nothing to answer keeps its reply bar hidden until a topic
    // note gives it a line.
    var bar = ta.closest(".reply");
    if (bar.hasAttribute("data-idle")) bar.hidden = !text;
    syncAll();
  }

  // Copy: execCommand only; the async clipboard API is denied in sandboxed hosts.
  // When the command fails the reply stays selected for a manual copy.
  var copy = doc.getElementById("reply-copy");
  if (copy && once(copy)) copy.addEventListener("click", function () {
    replyEls();
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
  function inPageLinks(root) {
    q('a[href^="#item-"]', root).forEach(function (a) {
      if (once(a)) a.addEventListener("click", function (e) {
        var el = doc.getElementById(a.getAttribute("href").slice(1));
        if (!el) return;
        e.preventDefault();
        for (var d = el.parentElement; d; d = d.parentElement) if (d.tagName === "DETAILS") d.open = true;
        el.scrollIntoView({ block: "center" });
        el.classList.remove("flash"); void el.offsetWidth; el.classList.add("flash");
      });
    });
  }

  bind(doc);
  window.wbBindBoard = bind;
  build();
