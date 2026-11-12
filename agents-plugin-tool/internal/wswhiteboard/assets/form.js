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
  // Item fields hold lead-authored HTML whose class attribute survives the
  // sanitizer, so every control here is selected by what a field cannot
  // carry: a data-* attribute, or an element the sanitizer drops (textarea,
  // button, label).
  function bind(root) {
    q(".card.policy[data-id]", root).forEach(function (card) {
      q(".opt[data-opt]", card).forEach(function (btn) {
        if (!once(btn)) return;
        function pick() {
          var on = btn.getAttribute("aria-pressed") === "true";
          q(".opt[data-opt]", card).forEach(function (b) { b.setAttribute("aria-pressed", "false"); });
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
    // A check row answers one way at a time: its x stages "no" and clears
    // the box, opening the note for the reason; ticking the box stages "ok"
    // and releases the x.
    q(".ckrow[data-id] button.no-toggle", root).forEach(function (b) {
      if (once(b)) b.addEventListener("click", function () {
        var row = b.closest(".ckrow"), on = b.getAttribute("aria-pressed") !== "true";
        setNo(row, on);
        if (on) {
          var box = row.querySelector("label.ck > input"), n = row.querySelector("textarea.note");
          if (box) box.checked = false;
          if (n) { n.hidden = false; grow(n); n.focus(); }
        }
        build();
      });
    });
    q(".ckrow[data-id] label.ck > input", root).forEach(function (c) {
      if (once(c)) c.addEventListener("change", function () {
        if (c.checked) setNo(c.closest(".ckrow"), false);
        build();
      });
    });
    q("button.note-toggle", root).forEach(function (t) {
      if (once(t)) t.addEventListener("click", function () {
        var n = t.closest(".card, .arow, .ckrow, .srow").querySelector("textarea.note");
        n.hidden = !n.hidden; if (!n.hidden) { grow(n); n.focus(); }
      });
    });
    // A topic's pen opens the note under its bar: a comment or question on
    // the topic as a whole, which the reply carries as a "[topic]^text" line.
    q("button.tnote-toggle", root).forEach(function (t) {
      if (once(t)) t.addEventListener("click", function () {
        var n = t.closest(".topic").querySelector("textarea.tnote");
        n.hidden = !n.hidden; if (!n.hidden) { grow(n); n.focus(); }
      });
    });
    q("button[data-bulk]", root).forEach(function (chip) {
      if (once(chip)) chip.addEventListener("click", function () { bulkAnswer(chip); });
    });
    q("textarea.note, label.ack > input", root).forEach(function (el) {
      if (!once(el)) return;
      if (el.classList.contains("note")) el.addEventListener("input", function () { grow(el); });
      el.addEventListener("input", build);
      el.addEventListener("change", build);
    });
    inPageLinks(root);
  }
  // A topic bar's count chip, where it is a button (the dashboard board),
  // answers every open item of its topic that it counts, as if each control
  // were pressed: the ack chip checks each acknowledgement, the policy chip
  // picks each card's recommended option. It only stages, like any pick; a
  // card already picked keeps its pick, and a card with no recommendation
  // is left for a real choice.
  function bulkAnswer(chip) {
    var topic = chip.closest(".topic");
    if (!topic) return;
    if (chip.getAttribute("data-bulk") === "ack") {
      q(".arow[data-id] label.ack > input", topic).forEach(function (c) { c.checked = true; });
    } else {
      q(".card.policy[data-id]", topic).forEach(function (card) {
        var opts = q(".opt[data-opt]", card);
        if (opts.some(function (b) { return b.getAttribute("aria-pressed") === "true"; })) return;
        var rec = opts.filter(function (b) { return b.classList.contains("rec"); })[0];
        if (rec) rec.setAttribute("aria-pressed", "true");
      });
    }
    build();
  }
  // setNo presses or releases a check row's x; the row carries data-no while
  // it stages "no".
  function setNo(row, on) {
    var b = row.querySelector("button.no-toggle");
    if (b) b.setAttribute("aria-pressed", on ? "true" : "false");
    if (on) row.setAttribute("data-no", ""); else row.removeAttribute("data-no");
  }
  // Size a note textarea to its content; scrollHeight is 0 while hidden.
  function grow(n) { n.style.height = "auto"; if (n.scrollHeight) n.style.height = n.scrollHeight + "px"; }
  // Bulk acknowledge: checks every ack row. Policy items stay manual.
  if (all && once(all)) all.addEventListener("change", function () {
    var on = this.checked;
    q("label.ack > input").forEach(function (c) { c.checked = on; });
    build();
  });
  function syncAll() {
    if (!all) return;
    var acks = q("label.ack > input");
    var on = acks.filter(function (c) { return c.checked; }).length;
    var total = acks.length;
    all.checked = total > 0 && on === total;
    all.indeterminate = on > 0 && on < total;
  }

  // The reply is the answer block (answers.js) of the answered items, in
  // board order, and the topic notes.
  function build() {
    replyEls();
    if (!ta) return;
    var items = [];
    q(".card.policy[data-id]").forEach(function (card) {
      var pick = card.querySelector('.opt[data-opt][aria-pressed="true"]');
      var noteEl = card.querySelector("textarea.note");
      items.push({ id: card.getAttribute("data-id"), answer: pick ? pick.getAttribute("data-opt") : "", note: noteEl ? noteEl.value : "" });
    });
    // A check is "ok" by its box or "no" by its x, one at a time; an assumed
    // check, locked like a settled row, sends its note alone.
    q(".ckrow[data-id]").forEach(function (row) {
      var box = row.querySelector("label.ck > input"), noteEl = row.querySelector("textarea.note");
      var answer = row.hasAttribute("data-no") ? "no" : box && box.checked ? "ok" : "";
      var note = noteEl ? noteEl.value : "";
      row.classList.toggle("staged", !!(answer || note.trim()));
      items.push({ id: row.getAttribute("data-id"), answer: answer, note: note });
    });
    // An acknowledgement is "ok"; with a note it leaves the ok group.
    q(".arow[data-id]").forEach(function (row) {
      var on = row.querySelector("label.ack > input").checked;
      items.push({ id: row.getAttribute("data-id"), answer: on ? "ok" : "", note: row.querySelector("textarea.note").value });
    });
    // An assumed item reads as settled; its pen still takes a veto or a
    // comment, sent as a note alone.
    q(".srow[data-id]").forEach(function (row) {
      items.push({ id: row.getAttribute("data-id"), answer: "", note: row.querySelector("textarea.note").value });
    });
    var topics = q("textarea.tnote").map(function (n) {
      return { topic: n.getAttribute("data-topic"), note: n.value };
    });
    var text = window.wbAnswers.block(items, topics);
    // A changed reply is no longer the copied one.
    if (text !== ta.value) copied = false;
    ta.value = text;
    // The block makes the reply several lines tall.
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

  // --- class tag tooltips ----------------------------------------------
  // An item's class tag (policy / technical / announced) says what that
  // class asks of the reader in a native tooltip. The title is set on first
  // hover rather than rendered, so the text ships once in the client table
  // instead of on every row. One handler on the document covers tags a live
  // update or a board swap inserts later; the dashboard re-runs this script
  // on every board swap, so the handler attaches once.
  if (window.wsT && !window.wbTagTips) {
    window.wbTagTips = true;
    var tagKeys = { policy: "js.wb.tag.policy", technical: "js.wb.tag.technical", announced: "js.wb.tag.announced" };
    doc.addEventListener("mouseover", function (e) {
      var el = e.target;
      if (!el || !el.classList || !el.classList.contains("tag") || el.hasAttribute("title")) return;
      for (var c in tagKeys) if (el.classList.contains(c)) { el.setAttribute("title", wsT(tagKeys[c])); return; }
    });
  }

  bind(doc);
  window.wbBindBoard = bind;
  build();
