(function () {
  // Shared by standalone boards and every dashboard page, not doc-frame.js:
  // sandboxed content and terminals keep their own keyboard handling.
  var doc = document, clicked = null, pointer = null, ignoredSelection = null;
  var blocks = [
    ".doc-body", ".cbody", ".atext", ".stext", ".sbody", ".rtext", ".input-body", ".entry-text", ".panel > .body",
    ".bub", ".mbody", ".wake-body", ".cmdout", ".d-summary .d-body", ".brow", ".wbupdate-text",
    ".bcard[data-kind='preview'] .bc-body", ".gw-diff", ".gw-msg", ".memo-text"
  ].join(",");
  var dialogs = "dialog[open], [role='dialog'], .bcard[data-max], .topic[data-max], .entry-panel[data-max], .fpane[data-max]";
  var terminals = ".bcard[data-kind='terminal'], .tm-host, .xterm";

  function element(node) { return node && (node.nodeType === 1 ? node : node.parentElement); }
  function visible(el) {
    return el && el.isConnected && el.getClientRects().length && getComputedStyle(el).visibility !== "hidden";
  }
  function block(node) {
    var el = element(node);
    return el && !el.closest(terminals) ? el.closest(blocks) : null;
  }
  function lastVisible(selector) {
    var list = doc.querySelectorAll(selector);
    for (var i = list.length - 1; i >= 0; i--) if (visible(list[i])) return list[i];
    return null;
  }
  function boundary() {
    // Native modals live in the top layer, above any custom sheet. A custom
    // modal likewise wins over a background non-modal popover.
    return lastVisible("dialog:modal") || lastVisible("[aria-modal='true']") || lastVisible(dialogs);
  }
  function allowed(el, within) { return visible(el) && (!within || within.contains(el)); }

  doc.addEventListener("pointerdown", function (e) {
    if (e.button !== 0) return;
    clicked = block(e.target);
    // Clicking user-select:none chrome can leave the old DOM selection intact.
    // Disregard that unchanged selection at the next shortcut, without clearing
    // it here (a copy/attach button may still need it during its click handler).
    var sel = window.getSelection();
    ignoredSelection = !clicked && sel && sel.rangeCount ? sel.getRangeAt(0).cloneRange() : null;
  }, true);
  doc.addEventListener("pointermove", function (e) { pointer = { x: e.clientX, y: e.clientY }; }, { passive: true });
  doc.addEventListener("pointerout", function (e) { if (!e.relatedTarget) pointer = null; });
  window.addEventListener("blur", function () { pointer = null; });
  doc.addEventListener("keydown", function (e) {
    if (!(e.ctrlKey || e.metaKey) || e.altKey || e.shiftKey || e.isComposing || e.key.toLowerCase() !== "a") return;
    var target = element(e.target);
    if (target && (target.closest("input, textarea") || target.isContentEditable || target.closest(terminals))) return;

    var within = boundary(), sel = window.getSelection(), scope = null;
    if (sel && sel.rangeCount) {
      var range = sel.getRangeAt(0);
      var stale = ignoredSelection && range.compareBoundaryPoints(0, ignoredSelection) === 0 && range.compareBoundaryPoints(2, ignoredSelection) === 0;
      var start = block(sel.anchorNode), end = block(sel.focusNode);
      if (!stale && start === end && allowed(start, within)) scope = start;
    }
    ignoredSelection = null;
    if (!scope && allowed(clicked, within)) scope = clicked;
    if (!scope && pointer) {
      var under = block(doc.elementFromPoint(pointer.x, pointer.y));
      if (allowed(under, within)) scope = under;
    }
    if (!scope && within) {
      var pane = within.matches(blocks) ? within : within.querySelector(blocks);
      if (allowed(pane, within)) scope = pane;
    }
    e.preventDefault();
    if (!sel) return;
    sel.removeAllRanges();
    if (scope) {
      var all = doc.createRange();
      all.selectNodeContents(scope);
      sel.addRange(all);
    }
  }, true);
})();
