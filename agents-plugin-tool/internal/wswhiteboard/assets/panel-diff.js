/* Latest panel vs immediate prior version. Marks live outside the scrolling
   body; old content is server-sanitized, and overlays live on the page. */
(function () {
  function bind(api) {
    var previous = window.wbBindBoard;
    if (previous && !previous.wbPanelDiffBound) {
      window.wbBindBoard = function (root) { previous(root); api.init(root); };
      window.wbBindBoard.wbPanelDiffBound = true;
    }
  }
  if (window.wbPanelDiff) { bind(window.wbPanelDiff); window.wbPanelDiff.init(document); return; }
  var d = document, active = null, serial = 0, timer = 0, restoring = false, states = new Set();
  var framed = !!d.querySelector('meta[name="wb-doc-key"]');
  function t(key) { return window.wsT.apply(window, arguments); }
  function nodes(root) {
    var out = [];
    function visit(n) {
      if (n.nodeType === 1 && (n.matches('script,style,template,.cb-copy,.cb-wrapbtn,.ln') || (root.tagName === 'LI' && n !== root && n.matches('ul,ol')))) return;
      if (n.nodeType === 3) out.push(n);
      for (var c = n.firstChild; c; c = c.nextSibling) visit(c);
    }
    visit(root);
    return out;
  }
  // Wrap only text nodes: emphasis, links, code, and authored markup survive.
  // A diagram or script that changed its own text is left alone.
  function words(root, edits, old) {
    var list = nodes(root), text = list.map(function (n) { return n.data; }).join('');
    var expected = edits.filter(function (e) { return e.k !== (old ? 'new' : 'del'); }).map(function (e) { return e.t; }).join('');
    if (text !== expected) return;
    var pos = 0, ops = [];
    edits.forEach(function (e) {
      if (e.k === 'del' && !old) ops.push({ start: pos, end: pos, text: e.t, tag: 'del' });
      else if (e.k !== (old ? 'new' : 'del')) {
        if (e.k !== 'equal') ops.push({ start: pos, end: pos + e.t.length, tag: old ? 'del' : 'ins' });
        pos += e.t.length;
      }
    });
    var offsets = [], offset = 0;
    list.forEach(function (n) { offsets.push(offset); offset += n.length; });
    ops.reverse().forEach(function (op) {
      for (var i = list.length - 1; i >= 0; i--) {
        var n = list[i], from = offsets[i], to = from + n.length;
        if (op.start === op.end) {
          if (op.start < from || op.start > to) continue;
          var at = n.splitText(op.start - from), gone = d.createElement('del');
          gone.className = 'pdg-w'; gone.textContent = op.text;
          at.parentNode.insertBefore(gone, at);
          break;
        }
        var a = Math.max(op.start, from), b = Math.min(op.end, to);
        if (a >= b) continue;
        var part = n.splitText(a - from); part.splitText(b - a);
        var wrap = d.createElement(op.tag); wrap.className = op.tag === 'ins' ? 'pdg-ins' : 'pdg-w';
        part.parentNode.insertBefore(wrap, part); wrap.appendChild(part);
      }
    });
  }
  function close(restore) {
    clearTimeout(timer);
    if (!active) return;
    var o = active; active = null;
    o.pop.remove(); o.hit.classList.remove('hot'); o.block.classList.remove('pdg-hot');
    o.hit.setAttribute('aria-expanded', 'false'); o.hit.removeAttribute('aria-controls');
    if (restore && o.hit.isConnected) {
      restoring = true; o.hit.focus(); restoring = false;
    }
  }
  function later() {
    clearTimeout(timer);
    timer = setTimeout(function () {
      if (active && !active.pinned && !active.pop.contains(d.activeElement) && active.hit !== d.activeElement) close(false);
    }, 160);
  }
  function place(o) {
    if (!o.hit.isConnected || !o.hit.getClientRects().length || !o.hit.getBoundingClientRect().height) { close(false); return; }
    var r = o.hit.getBoundingClientRect(), block = o.block.getBoundingClientRect();
    var left = r.right + window.scrollX, top = (o.change.k === 'del' ? r.top + r.height / 2 : block.bottom) + window.scrollY + 3;
    if (o.change.k === 'new') top = r.top + window.scrollY;
    o.pop.style.left = Math.max(window.scrollX + 8, Math.min(left, window.scrollX + window.innerWidth - o.pop.offsetWidth - 8)) + 'px';
    o.pop.style.top = top + 'px';
  }
  function pin(o) {
    o.pinned = true; o.pop.classList.add('pinned');
    var b = o.pop.querySelector('.pin'); if (b) b.setAttribute('aria-pressed', 'true');
  }
  function el(tag, cls, text) {
    var n = d.createElement(tag); n.className = cls;
    if (text != null) n.textContent = text;
    return n;
  }
  function button(cls, text, title) {
    var b = el('button', cls, text); b.type = 'button'; b.title = title; b.setAttribute('aria-label', title); return b;
  }
  function label(kind) { return t(kind === 'mod' ? "js.wb.diff.modified" : kind === 'del' ? "js.wb.diff.removed" : "js.wb.diff.added"); }
  function version(v) { return 'v' + v.Version + (v.Clock ? ' ' + v.Clock : ''); }
  function show(item, pinned) {
    clearTimeout(timer);
    if (framed) return; // An opaque frame cannot place an unclipped overlay.
    if (active && active.hit === item.hit) { if (pinned && item.change.k !== 'new') pin(active); return; }
    if (active && active.pinned && !pinned) return;
    close(false);
    var change = item.change, data = item.state.data, pop;
    if (change.k === 'new') {
      pop = el('div', 'pdg-tip', label('new') + ' · ' + version(data.current));
      pop.setAttribute('role', 'tooltip');
    } else {
      pop = el('div', 'pdg-pop ' + change.k); pop.setAttribute('role', 'dialog');
      var section = t(change.k === 'mod' ? "js.wb.diff.previous" : "js.wb.diff.deleted");
      pop.setAttribute('aria-label', section); pop.tabIndex = -1;
      var head = el('div', 'pdg-ph'), pb = button('pin', '', t("js.wb.diff.pin")), cb = button('close', '×', t("js.wb.diff.close"));
      pb.innerHTML = '<svg class="i" viewBox="0 0 16 16" aria-hidden="true"><path d="M9.8 2.4l3.8 3.8-1.7.8-2.2 2.2.3 2.9-.9.9-5.4-5.4.9-.9 2.9.3 2.2-2.2zM5.9 10.1l-3.3 3.3"/></svg>';
      pb.setAttribute('aria-pressed', 'false');
      head.append(el('span', 'k', label(change.k)), el('span', 'v', version(data.prior) + ' → ' + version(data.current)), el('span', 'sp'), pb, cb);
      var content = el('div', 'pdg-pb md'); content.innerHTML = change.old || '';
      if (change.k === 'mod') words(content.firstElementChild || content, change.words || [], true);
      else content.classList.add('gone');
      if (content.firstElementChild && content.firstElementChild.tagName === 'LI') {
        var ul = d.createElement('ul'); while (content.firstChild) ul.appendChild(content.firstChild); content.appendChild(ul);
      }
      var footer = el('div', 'pdg-pf'), link = el('a', '', t("js.wb.diff.full", data.prior.Version)); link.href = '#' + data.prior.ID;
      footer.append(el('span', 'hint', t("js.wb.diff.hint")), el('span', 'sp'), link);
      pop.append(head, el('div', 'pdg-pl', section), content, footer);
      pb.addEventListener('click', function () { if (active.pinned) close(true); else pin(active); });
      cb.addEventListener('click', function () { close(true); });
      link.addEventListener('click', function (e) {
        e.preventDefault(); var id = data.prior.ID; close(false);
        if (window.wbHistory) window.wbHistory.loadTarget(d, id);
        var target = d.getElementById(id);
        if (target) { var details = target.querySelector('.entry-panel'); if (details) details.open = true; target.scrollIntoView({ block: 'center' }); }
        window.location.hash = id;
      });
    }
    pop.id = 'pdg-pop-' + (++serial); d.body.appendChild(pop);
    item.hit.setAttribute('aria-controls', pop.id); item.hit.setAttribute('aria-expanded', 'true');
    active = { pop: pop, hit: item.hit, block: item.block, change: change, pinned: false };
    item.hit.classList.add('hot'); item.block.classList.add('pdg-hot');
    pop.addEventListener('pointerenter', function () { clearTimeout(timer); });
    pop.addEventListener('pointerleave', later);
    pop.addEventListener('focusin', function () { clearTimeout(timer); });
    pop.addEventListener('focusout', later);
    if (pinned && change.k !== 'new') pin(active);
    place(active);
  }
  function layout(state) {
    var body = state.body, box = body.getBoundingClientRect(), origin = state.layer.getBoundingClientRect();
    if (!box.width) return;
    // In the host this is the existing 1px guide. In a frame its own guide
    // sits inside the document so the stable button hit boxes are not clipped.
    var gx = box.left + (framed ? parseFloat(getComputedStyle(body).paddingLeft || '0') - 7 : .5);
    state.items.forEach(function (item) {
      var r = item.block.getBoundingClientRect(), h = r.height, top = r.top;
      var gone = (item.gone || []).map(function (n) { return n.getBoundingClientRect(); }).filter(function (r) { return r.height > 0; });
      var delblock = gone.length > 0;
      if (item.change.k === 'del') {
        if (delblock) { top = gone[0].top; h = gone[gone.length - 1].bottom - top; }
        else { top = (item.change.after ? box.bottom : r.top) - 7; h = 14; }
      } else {
        var nested = item.block.querySelector(':scope > ul, :scope > ol');
        if (nested) h = nested.getBoundingClientRect().top - r.top;
      }
      item.hit.classList.toggle('delblock', !!delblock); item.hit.classList.toggle('del', item.change.k === 'del' && !delblock);
      item.hit.style.left = (gx - origin.left - 7) + 'px'; item.hit.style.top = (top - origin.top) + 'px'; item.hit.style.height = Math.max(12, h) + 'px';
    });
    if (active && state.layer.contains(active.hit)) place(active);
  }
  function initBody(body) {
    if (body.wbPanelDiff) { layout(body.wbPanelDiff); return; }
    var comments = [], walk = d.createTreeWalker(body, NodeFilter.SHOW_COMMENT), c, raw;
    while ((c = walk.nextNode())) {
      if (c.data.indexOf('wbpdg:') === 0) comments.push(c);
      else if (c.data.indexOf('wbpdg-data:') === 0) raw = c.data.slice(11);
    }
    if (!raw) return;
    var data; try { data = JSON.parse(raw); } catch (_) { return; }
    var state = { body: body, data: data, items: [], layer: el('div', 'pdg-layer') };
    body.wbPanelDiff = state; body.parentNode.insertBefore(state.layer, body.nextSibling);
    comments.forEach(function (comment) {
      var change = data.changes[+comment.data.slice(6)]; if (!change) return;
      var block = change.after ? body : comment.nextElementSibling;
      if (!block) block = body;
      var hit = button('pdg-hit ' + change.k, '', label(change.k) + ' · ' + version(data.current));
      hit.setAttribute('aria-label', label(change.k) + ' · ' + version(data.current));
      hit.setAttribute('aria-expanded', 'false');
      if (!framed) {
        hit.removeAttribute('title');
        if (change.k !== 'new') hit.setAttribute('aria-haspopup', 'dialog');
      }
      hit.appendChild(el('span', 'pdg-bar'));
      var item = { state: state, change: change, block: block, hit: hit };
      if (change.k !== 'del') {
        block.classList.add('pdg-b', change.k === 'mod' ? 'pdg-mod' : 'pdg-new');
        if (change.k === 'mod') words(block, change.words || [], false);
      } else {
        var gone = d.createElement('template'); gone.innerHTML = change.old || '';
        var elements = Array.from(gone.content.children);
        elements.forEach(function (n) {
          n.classList.add('pdg-gone');
          if (n.tagName === 'LI' && !comment.parentNode.matches('ul,ol')) {
            // A trailing removed list item belongs to the preceding list;
            // elsewhere give it its own valid list container.
            var list = change.after && comment.previousElementSibling;
            if (!list || !list.matches('ul,ol')) {
              list = d.createElement('ul'); list.className = 'pdg-gone';
              comment.parentNode.insertBefore(list, comment);
            }
            list.appendChild(n);
          } else comment.parentNode.insertBefore(n, comment);
        });
        item.gone = elements;
      }
      state.items.push(item); state.layer.appendChild(hit);
      hit.addEventListener('pointerenter', function () { show(item, false); });
      hit.addEventListener('pointerleave', later);
      hit.addEventListener('focus', function () { if (!restoring) show(item, false); });
      hit.addEventListener('blur', later);
      hit.addEventListener('click', function () { if (active && active.hit === hit && active.pinned) close(true); else show(item, true); });
      hit.addEventListener('keydown', function (e) {
        if (e.key === 'Tab' && !e.shiftKey && active && active.hit === hit && active.pop.querySelector('button')) {
          e.preventDefault(); pin(active); active.pop.querySelector('button').focus();
        }
      });
    });
    body.classList.toggle('pdg-diffview', d.documentElement.getAttribute('data-pdg-compare') === 'true');
    if (window.ResizeObserver) {
      state.observer = new ResizeObserver(function () { if (body.isConnected) layout(state); });
      state.observer.observe(body);
    }
    states.add(state);
    layout(state);
  }
  function compare(panel, on) {
    close(false); panel.classList.toggle('pdg-on', on);
    panel.querySelector('.pdg-chip').setAttribute('aria-pressed', on ? 'true' : 'false');
    var body = panel.querySelector('.entry-body'), frame = body.querySelector('.doc-frame[data-srcdoc]');
    if (frame) {
      // Reuse the existing sandbox and mounting path; no new frame messages.
      // Both the placeholder and a mounted iframe retain their source document.
      var source = frame.getAttribute('data-srcdoc').replace(/ data-pdg-compare="(?:true|false)"/, '');
      source = source.replace('<html', '<html data-pdg-compare="' + on + '"');
      frame.setAttribute('data-srcdoc', source);
      if (frame.tagName === 'IFRAME') frame.setAttribute('srcdoc', source);
    } else {
      body.classList.toggle('pdg-diffview', on);
      if (body.wbPanelDiff) layout(body.wbPanelDiff);
    }
  }
  function init(root) {
    states.forEach(function (state) {
      if (state.body.isConnected) return;
      if (state.observer) state.observer.disconnect();
      states.delete(state);
    });
    root.querySelectorAll('.doc-body:not(.framed)').forEach(initBody);
    root.querySelectorAll('.pdg-chip').forEach(function (chip) {
      if (chip.wbPanelDiff) return; chip.wbPanelDiff = true;
      var panel = chip.closest('.entry-panel');
      chip.addEventListener('click', function (e) { e.preventDefault(); e.stopPropagation(); compare(panel, chip.getAttribute('aria-pressed') !== 'true'); });
      panel.querySelector('.pdg-off').addEventListener('click', function () { compare(panel, false); });
    });
    if (active && !active.hit.isConnected) close(false);
  }
  window.wbPanelDiff = { init: init };
  bind(window.wbPanelDiff);
  d.addEventListener('wbhistoryload', function (e) { init(e.detail.root); });
  d.addEventListener('keydown', function (e) { if (e.key === 'Escape') close(true); });
  d.addEventListener('toggle', function (e) {
    if (e.target.querySelectorAll) e.target.querySelectorAll('.doc-body').forEach(function (b) { if (b.wbPanelDiff) layout(b.wbPanelDiff); });
  }, true);
  window.addEventListener('resize', function () { init(d); });
  window.addEventListener('load', function () { init(d); });
  init(d);
})();
