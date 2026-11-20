// Draws a topic body's change marks as thin bars in the gutter left of its
// text column: green beside a new block, blue beside a rewritten one, and a
// short red tick where a block was deleted. The server leaves the marks in
// the body as comments (changes.go): <!--wbchg:new-->, <!--wbchg:mod-->, and
// <!--wbchg:del--> go before the block they mark, <!--wbchg:delafter--> after
// the last block. The bars live in a zero-height layer after the body, so the
// body's own layout and :first-child/:last-child rules stay as written.
function wbChangeBars(body) {
  var marks = [], walk = document.createTreeWalker(body, NodeFilter.SHOW_COMMENT), c;
  while ((c = walk.nextNode())) if (c.data.indexOf("wbchg:") === 0) marks.push(c);
  var layer = body.nextElementSibling;
  if (layer && !layer.classList.contains("wb-chg-layer")) layer = null;
  function hideTip() {
    if (!layer || !layer.wbTip) return;
    layer.wbTip.hidden = true;
    if (layer.wbTip.wbBodyAgeEnd) layer.wbTip.wbBodyAgeEnd();
  }
  hideTip();
  if (!marks.length) { if (layer) layer.remove(); return; }
  if (!layer) {
    layer = document.createElement("div");
    layer.className = "wb-chg-layer";
    body.parentNode.insertBefore(layer, body.nextSibling);
  }
  // A body whose topic moved on after its edit (chg-stale) keeps its bars,
  // dimmed. Set here rather than in page.css because it is runtime state.
  layer.style.opacity = body.classList.contains("chg-stale") ? ".35" : "";
  layer.textContent = "";
  var origin = layer.getBoundingClientRect(), box = body.getBoundingClientRect();
  if (!box.width) return;
  // 9px left of the text column; the body's padding is the gutter.
  var x = box.left + parseFloat(getComputedStyle(body).paddingLeft || "0") - 9 - origin.left;
  function bar(kind, top, height, wide) {
    var b = document.createElement("div");
    b.className = "wb-chg-bar " + kind;
    if (height) b.style.height = height + "px";
    // Keep the painted bar's resting geometry. A separate, stable hit box
    // grows neither on hover nor into the text column.
    var hit = document.createElement("div"), hitHeight = Math.max(12, height || 2), pad = (hitHeight - (height || 2)) / 2;
    hit.className = "wb-chg-hit";
    var paintedLeft = wide ? x - 3 : x;
    var hitLeft = wide ? Math.max(box.left - origin.left, x - 8) : x - 5;
    hit.style.left = hitLeft + "px";
    hit.style.top = (top - pad) + "px";
    hit.style.height = hitHeight + "px";
    hit.style.width = "12px";
    b.style.left = (paintedLeft - hitLeft) + "px";
    b.style.top = pad + "px";
    hit.appendChild(b);
    layer.appendChild(hit);
    hit.addEventListener("pointerenter", function () {
      hideTip();
      var tip = layer.wbTip;
      if (!tip) {
        tip = document.createElement("div");
        tip.className = "wb-chg-tip";
        tip.hidden = true;
        tip.setAttribute("role", "tooltip");
        tip.setAttribute("data-body-age", "");
        document.body.appendChild(tip);
        layer.wbTip = tip;
      }
      // Same validated view value: the unframed topic body metadata, or the
      // frame's own metadata. Never read overall topic activity or parent.
      var root = body.closest(".topic") || document;
      var source = root.querySelector(".tdoc[data-body-at], meta[name=wb-body-at]");
      var at = source ? source.getAttribute("data-body-at") : "";
      tip.setAttribute("data-body-at", at);
      window.wbBodyAgeHover(tip, function (value) {
        tip.textContent = value;
        tip.hidden = false;
        // Fixed overlay, clamped inside this document (including short opaque
        // frames). It never changes body height or asks the host to scroll.
        tip.style.left = "0px";
        tip.style.top = "0px";
        var r = hit.getBoundingClientRect(), tr = tip.getBoundingClientRect();
        tip.style.left = Math.max(0, Math.min(r.right + 6, window.innerWidth - tr.width)) + "px";
        tip.style.top = Math.max(0, Math.min(r.top, window.innerHeight - tr.height)) + "px";
      });
    });
    hit.addEventListener("pointerleave", hideTip);
  }
  marks.forEach(function (m) {
    var kind = m.data.slice(6), el = kind === "delafter" ? m.previousElementSibling : m.nextElementSibling;
    if (!el) return;
    var r = el.getBoundingClientRect();
    if (kind === "new" || kind === "mod") {
      // A list item's bar stops where its nested list starts: the nested
      // items carry their own marks.
      var h = r.height, nested = el.querySelector(":scope > ul, :scope > ol");
      if (nested) h = nested.getBoundingClientRect().top - r.top;
      if (h > 0) bar(kind, r.top - origin.top, h, false);
    } else {
      bar("del", (kind === "del" ? r.top : r.bottom) - origin.top - 1, 0, true);
    }
  });
}

// wbWatchChanges keeps body's bars in step with its layout: a body that opens,
// resizes, or loads an image lays out again.
function wbWatchChanges(body) {
  wbChangeBars(body);
  if (window.ResizeObserver) new ResizeObserver(function () { wbChangeBars(body); }).observe(body);
  window.addEventListener("load", function () { wbChangeBars(body); });
}
