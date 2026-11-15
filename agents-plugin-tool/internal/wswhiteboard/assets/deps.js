// Delegated once per document: topic swaps and closed-page inserts need no bind.
(function () {
  var d = document;
  if (d._wbDeps) return;
  d._wbDeps = true;
  var active = null, pinned = false, opening = 0, leaving = 0, restoring = false;
  function wrapper(node) {
    if (active && !active.isConnected) close(false);
    return node && node.closest ? node.closest('.dw') : null;
  }
  function cancel() { clearTimeout(opening); clearTimeout(leaving); }
  function isOpen(p) {
    if (p.showPopover) {
      try { return p.matches(':popover-open'); } catch (e) { /* older engine */ }
    }
    return !p.hidden;
  }
  function close(focus) {
    cancel();
    if (!active) return;
    var b = active.querySelector('.dbtn');
    var p = active.querySelector('.pop');
    if (p.hidePopover && p.isConnected) p.hidePopover();
    p.hidden = true;
    b.setAttribute('aria-expanded', 'false');
    active = null;
    pinned = false;
    if (focus && b.isConnected) {
      restoring = true;
      b.focus({ preventScroll: true });
      restoring = false;
    }
  }
  function open(w) {
    cancel();
    if (!w || !w.isConnected) return;
    if (active !== w) { close(false); active = w; }
    var b = w.querySelector('.dbtn'), p = w.querySelector('.pop');
    var wasOpen = isOpen(p);
    p.hidden = false;
    // The top layer also escapes sticky-head stacking contexts and transient
    // rail reorder transforms. Older engines retain the fixed fallback.
    if (!wasOpen && p.showPopover) p.showPopover();
    b.setAttribute('aria-expanded', 'true');
    // Fixed descendants bypass overflow:hidden on .topic. Measure each open,
    // including after swaps; clamp horizontally and flip above when needed.
    var r = b.getBoundingClientRect(), box = p.getBoundingClientRect();
    var left = Math.max(8, Math.min(r.left - 6, window.innerWidth - box.width - 8));
    var top = r.bottom + 8;
    if (top + box.height > window.innerHeight - 8) top = r.top - box.height - 8;
    top = Math.max(8, Math.min(top, window.innerHeight - box.height - 8));
    p.style.left = left + 'px';
    p.style.top = top + 'px';
    p.style.setProperty('--wb-deps-caret', Math.max(8, Math.min(r.left + r.width / 2 - left - 4, box.width - 16)) + 'px');
    p.classList.toggle('above', top < r.top);
  }
  function leave(w) {
    clearTimeout(opening);
    clearTimeout(leaving);
    leaving = setTimeout(function () {
      if (active === w && !pinned && !w.contains(d.activeElement)) close(false);
    }, 160);
  }
  d.addEventListener('mouseover', function (e) {
    var w = wrapper(e.target);
    if (!w || (e.relatedTarget && w.contains(e.relatedTarget))) return;
    clearTimeout(leaving);
    if (active === w ? isOpen(w.querySelector('.pop')) : pinned) return;
    clearTimeout(opening);
    opening = setTimeout(function () { open(w); }, 120);
  });
  d.addEventListener('mouseout', function (e) {
    var w = wrapper(e.target);
    if (w && !(e.relatedTarget && w.contains(e.relatedTarget))) leave(w);
  });
  d.addEventListener('focusin', function (e) {
    if (restoring) return;
    var w = wrapper(e.target);
    if (w) open(w);
  });
  d.addEventListener('focusout', function (e) {
    var w = wrapper(e.target);
    if (w && !(e.relatedTarget && w.contains(e.relatedTarget))) leave(w);
  });
  d.addEventListener('click', function (e) {
    var w = wrapper(e.target), b = e.target.closest && e.target.closest('.dw .dbtn');
    if (b) {
      e.preventDefault();
      e.stopPropagation();
      if (active === w && pinned && isOpen(w.querySelector('.pop'))) close(false);
      else { open(w); pinned = true; }
    } else if (!w || (e.target.closest && e.target.closest('a'))) close(false);
  }, true);
  // Reparenting can close a native popover without changing hidden. Toggle
  // does not bubble; capture it and release the stale active wrapper and pin.
  d.addEventListener('toggle', function (e) {
    if (!active || e.newState !== 'closed') return;
    var p = active.querySelector('.pop');
    if (e.target !== p) return;
    // Ignore a delayed close event if this popup has already reopened.
    try { if (p.matches(':popover-open')) return; } catch (err) { /* older engine */ }
    cancel();
    p.hidden = true;
    active.querySelector('.dbtn').setAttribute('aria-expanded', 'false');
    active = null;
    pinned = false;
  }, true);
  d.addEventListener('keydown', function (e) {
    if (e.key === 'Escape' && active) {
      e.preventDefault();
      e.stopPropagation();
      close(true);
    }
  }, true);
  d.addEventListener('scroll', function (e) {
    if (!active || !active.querySelector('.pop').contains(e.target)) close(false);
  }, true);
  window.addEventListener('resize', function () { close(false); });
})();
