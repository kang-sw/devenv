// Composer image attachments and markers.
  // --- composer images -------------------------------------------------------
  // A pasted or dropped image is a thumbnail chip held in the page and
  // uploaded only with the send, so an abandoned paste never touches disk. A
  // chip a cancel returned has a ref instead of data: the file the daemon
  // already wrote, which a re-send names rather than uploads again.
  //
  // Each image has a number n, and the text names it with an "[Image #n]"
  // marker inserted at the cursor. Marker and chip stay in sync: deleting the
  // marker drops the chip, removing the chip deletes the marker. A dropped
  // image waits in imgTrash, so an undo that brings its marker back brings
  // the chip back too. Numbers are never reused or rewritten within a draft
  // (rewriting text the user is editing shakes it). Each send is its own
  // held message, numbered as the draft numbered it; a fresh draft numbers
  // from 1 again.
  //
  // The board's note pens (item notes and topic notes) take pasted images
  // too: one image list and one number sequence span the composer text and
  // every note, since one send carries them all. An image's chip sits with
  // the field whose text names it: the composer's row, or a row right after
  // that note; a note whose item left the board lives on in its answer chip,
  // and its images show in the composer's row.
  var cimgs = $("cimgs"), imgs = [], imgSeq = 0, imgTrash = {}, holdSync = false, imgLayout = "";
  var IMG_TYPES = { "image/png": 1, "image/jpeg": 1, "image/gif": 1, "image/webp": 1 };
  var IMG_MAX = 8, IMG_BYTES = 10 * 1024 * 1024;
  var MARKER = "\\[Image #([1-9][0-9]{0,5})\\]";
  function attachSrc(name) { return "/s/" + rid + "/attachment/" + encodeURIComponent(name); }
  function marker(n) { return "[Image #" + n + "]"; }
  // markerNums is the set of image numbers text's markers name.
  function markerNums(text) {
    var out = {}, re = new RegExp(MARKER, "g"), m;
    while ((m = re.exec(text))) out[+m[1]] = true;
    return out;
  }
  // boardNotes are the board's note pens: every item note and topic note
  // (a topic note is a textarea.note too).
  function boardNotes() { return boardQ("textarea.note"); }
  // strayChips are the answer chips whose item left the board: their notes
  // still go with the send, so their markers still name images.
  function strayChips() { return chipList().filter(function (c) { return c.note && !boardItem(c.id); }); }
  // allNums is the set of image numbers the markers of the whole draft name:
  // the composer text, every note on the board, and the stray chips' notes.
  function allNums() {
    var text = [ctext.value].concat(boardNotes().map(function (n) { return n.value; }), strayChips().map(function (c) { return c.note; }));
    var nums = markerNums(text.join("\n"));
    bselections.forEach(function (s) { s.images.forEach(function (n) { nums[n] = true; }); });
    if (mpend) (mpend.images || []).forEach(function (n) { nums[n] = true; });
    return nums;
  }
  // nextNum is a number no image, dropped image, or marker of this draft
  // has used.
  function nextNum() {
    Object.keys(allNums()).forEach(function (k) { imgSeq = Math.max(imgSeq, +k); });
    return ++imgSeq;
  }
  // editField replaces [start, end) of a text field's value with s through
  // the browser's editing command, so the undo stack keeps the edit, else
  // through setRangeText; either way an input event follows. A field that
  // cannot take the focus (a closed note) takes setRangeText, so the
  // command never edits whatever else holds the focus.
  function editField(el, start, end, s) {
    if (!(doc.activeElement === el || el.wsc && doc.activeElement === el.wsc.editor)) el.focus();
    el.setSelectionRange(start, end);
    var done = false;
    if (doc.activeElement === el || el.wsc && doc.activeElement === el.wsc.editor) {
      try { done = s ? doc.execCommand("insertText", false, s) : start === end || doc.execCommand("delete"); } catch (e) { done = false; }
    }
    if (!done) {
      el.setRangeText(s, start, end, "end");
      el.dispatchEvent(new Event("input", { bubbles: true }));
    }
  }
  // editText is editField on the composer's text.
  function editText(start, end, s) { editField(ctext, start, end, s); }
  // The explorer has already resolved the path against its active tab root.
  // Use the composer's mention syntax and edit path; this never submits.
  function attachExplorerPath(path) {
    if (!ctext || !window.wsMention) return;
    window.WsCards.dismissFiles();
    var v = ctext.value, a = ctext.selectionStart, b = ctext.selectionEnd;
    var s = window.wsMention.mentionParts(path).text;
    if (a > 0 && !/\s/.test(v.charAt(a - 1))) s = " " + s;
    if (b >= v.length || !/\s/.test(v.charAt(b))) s += " ";
    editText(a, b, s);
  }
  // insertMarkers puts the markers for nums at el's cursor (the composer's
  // by default), spaced from the text around them.
  function insertMarkers(nums, el) {
    el = el || ctext;
    if (!nums.length) return;
    var v = el.value, a = el.selectionStart, b = el.selectionEnd, s = nums.map(marker).join(" ");
    if (a > 0 && !/\s/.test(v.charAt(a - 1))) s = " " + s;
    if (b >= v.length || !/\s/.test(v.charAt(b))) s += " ";
    editField(el, a, b, s);
  }
  // deleteMarkers deletes each marker naming n, with the space after it,
  // wherever the draft holds one: the composer, a note, or a stray chip's
  // note. The stray chips go first: each field edit re-syncs the images,
  // which a marker left in a stray chip would bring back.
  function deleteMarkers(n) {
    bselections.forEach(function (s) { s.images = s.images.filter(function (k) { return k !== n; }); });
    if (mpend) mpend.images = (mpend.images || []).filter(function (k) { return k !== n; });
    var src = "\\[Image #" + n + "\\] ?";
    var stray = strayChips().filter(function (c) { return new RegExp(src).test(c.note); });
    stray.forEach(function (c) {
      c.note = c.note.replace(new RegExp(src, "g"), "").trim();
      if (!c.answer && !c.note) dropChip(c.id);
    });
    if (stray.length) renderChips();
    [ctext].concat(boardNotes()).forEach(function (el) {
      var re = new RegExp(src, "g"), v = el.value, spans = [], m;
      while ((m = re.exec(v))) spans.push([m.index, m.index + m[0].length]);
      spans.reverse().forEach(function (sp) { editField(el, sp[0], sp[1], ""); });
    });
  }
  // addFiles attaches image files, their markers going in at el's cursor
  // (the composer's by default).
  function addFiles(files, el) {
    var owner = el === mnote ? mpend : null;
    if (el === mnote && (!owner || owner.rid !== rid)) return;
    if (owner) lockMention();
    var refused = "", added = [];
    files.forEach(function (f) {
      if (!IMG_TYPES[f.type]) { refused = wsT("js.img.type", f.name || wsT("js.img.thefile")); return; }
      if (f.size > IMG_BYTES) { refused = wsT("js.img.big", f.name || wsT("js.img.theimage")); return; }
      if (imgs.length >= IMG_MAX) { refused = wsT("js.img.max", IMG_MAX); return; }
      var it = { name: f.name || "pasted image", url: URL.createObjectURL(f), data: null, ref: "", blob: f, n: nextNum() };
      imgs.push(it);
      added.push(it.n);
      if (owner) owner.images.push(it.n);
      var rd = new FileReader();
      rd.onload = function () { var v = String(rd.result); it.data = v.slice(v.indexOf(",") + 1); };
      rd.onerror = function () { dropImg(it); release(it); deleteMarkers(it.n); renderImgs(); noteWarn(wsT("js.img.readfail", it.name)); };
      rd.readAsDataURL(f);
    });
    renderImgs();
    if (!owner) insertMarkers(added, el);
    queueDraft();
    if (refused) noteWarn(wsT("js.img.refused", refused));
  }
  function dropImg(it) {
    var i = imgs.indexOf(it);
    if (i >= 0) imgs.splice(i, 1);
  }
  function release(it) { if (!it.ref) URL.revokeObjectURL(it.url); }
  // unattach removes a chip the user took off: its markers go, and it waits
  // in imgTrash for an undo.
  function unattach(it) {
    if (!it) return;
    dropImg(it);
    imgTrash[it.n] = it;
    renderImgs();
    deleteMarkers(it.n);
  }
  // clearTrash forgets the dropped images once no undo can bring them back.
  function clearTrash() {
    Object.keys(imgTrash).forEach(function (k) { release(imgTrash[k]); });
    imgTrash = {};
  }
  // syncMarkers follows the draft's text (allNums): an image whose marker is
  // gone is dropped (into imgTrash), and a dropped one whose marker is back
  // returns. A restore that moves markers between fields (restoreHeld)
  // holds it off until every marker has landed.
  function syncMarkers() {
    if (holdSync || (!imgs.length && !Object.keys(imgTrash).length)) return;
    var have = allNums(), changed = false;
    imgs.slice().forEach(function (it) {
      if (!have[it.n]) { dropImg(it); imgTrash[it.n] = it; changed = true; }
    });
    Object.keys(imgTrash).forEach(function (k) {
      var it = imgTrash[k];
      if (have[it.n]) { delete imgTrash[k]; imgs.push(it); changed = true; }
    });
    if (!changed) return;
    imgs.sort(function (a, b) { return a.n - b.n; });
    renderImgs();
  }
  // imgHomes is each image's field: -1 for the composer, else its index in
  // notes. The composer's text wins a marker both name, and an image no
  // field names (a stray chip's) shows in the composer's row.
  function imgHomes(notes) {
    var own = markerNums(ctext.value), names = notes.map(function (n) { return markerNums(n.value); });
    return imgs.map(function (it) {
      if (mpend && (mpend.images || []).indexOf(it.n) >= 0) return -2;
      for (var s = 0; s < bselections.length; s++) if (bselections[s].images.indexOf(it.n) >= 0) return -3 - s;
      if (own[it.n]) return -1;
      for (var i = 0; i < names.length; i++) if (names[i][it.n]) return i;
      return -1;
    });
  }
  function renderImgs() {
    if (!cimgs) return;
    var notes = boardNotes(), homes = imgHomes(notes), rows = {};
    imgLayout = JSON.stringify(homes);
    cimgs.textContent = "";
    if (mimgs) mimgs.textContent = "";
    bselections.forEach(function (s) { if (s.row) s.row.textContent = ""; });
    boardQ(".nimgs").forEach(function (r) { r.parentNode.removeChild(r); });
    imgs.forEach(function (it, i) {
      var el = doc.createElement("span"), im = doc.createElement("img"), x = doc.createElement("button"), num = doc.createElement("span");
      el.className = "ichip";
      im.src = it.url;
      im.alt = it.name;
      el.title = wsT("js.img.title", it.n, it.name);
      el.appendChild(im);
      num.className = "inum";
      num.textContent = "#" + it.n;
      el.appendChild(num);
      x.className = "unimg";
      x.setAttribute("data-unimg", String(i));
      x.title = wsT("js.img.remove");
      x.textContent = "×";
      el.appendChild(x);
      var h = homes[i];
      if (h === -2) { if (mimgs) mimgs.appendChild(el); return; }
      if (h <= -3) { var group = bselections[-3 - h]; if (group && group.row) group.row.appendChild(el); else cimgs.appendChild(el); return; }
      if (h < 0) { cimgs.appendChild(el); return; }
      // A note's row follows the note, so it hides with it (CSS) and
      // leaves with it when a board swap replaces its topic.
      if (!rows[h]) {
        rows[h] = doc.createElement("span");
        rows[h].className = "nimgs";
        var after = notes[h].wsc ? notes[h].wsc.editor : notes[h];
        after.parentNode.insertBefore(rows[h], after.nextSibling);
      }
      rows[h].appendChild(el);
    });
  }
  // followImgs moves the chips after an edit in a note that changed which
  // field names an image, and puts back the rows a board swap took.
  function followImgs() {
    if (!imgs.length && !boardQ(".nimgs").length) return;
    var homes = imgHomes(boardNotes()), rows = boardQ(".nimgs").length;
    var want = {};
    homes.forEach(function (h) { if (h >= 0) want[h] = true; });
    if (JSON.stringify(homes) !== imgLayout || rows !== Object.keys(want).length) renderImgs();
  }
  // imageFiles is the image files a clipboard or drop carries.
  function imageFiles(dt) {
    var out = [];
    if (!dt) return out;
    if (dt.items && dt.items.length) {
      Array.prototype.forEach.call(dt.items, function (item) {
        if (item.kind !== "file") return;
        var f = item.getAsFile();
        if (f) out.push(f);
      });
    } else if (dt.files) {
      out = Array.prototype.slice.call(dt.files);
    }
    return out;
  }
  // pasteImages attaches the images a paste into el carries.
  function pasteImages(e, el) {
    var files = imageFiles(e.clipboardData).filter(function (f) { return f.type.indexOf("image/") === 0; });
    if (!files.length) return;
    // A paste that carries text as well (a copied web page) still pastes
    // its text; a screenshot carries only the image. The markers go in
    // once the paste is done, after any pasted text.
    var types = Array.prototype.slice.call(e.clipboardData.types || []);
    if (types.indexOf("text/plain") < 0) e.preventDefault();
    var owner = el === mnote ? mpend : null;
    setTimeout(function () { if (el !== mnote || owner && owner === mpend && owner.rid === rid) addFiles(files, el); }, 0);
  }
  var composer = $("composer");
  if (ctext && cimgs && composer) {
    ctext.addEventListener("paste", function (e) { pasteImages(e, ctext); });
    // A board note pen pastes into the same draft; one listener on the
    // document covers the notes every board swap brings.
    doc.addEventListener("paste", function (e) {
      var el = e.target;
      if (el && el.tagName === "TEXTAREA" && el.classList.contains("note") && inBoard(el)) pasteImages(e, el);
    });
    var hasFiles = function (e) { return e.dataTransfer && Array.prototype.indexOf.call(e.dataTransfer.types || [], "Files") >= 0; };
    composer.addEventListener("dragover", function (e) {
      if (!hasFiles(e)) return;
      e.preventDefault();
      composer.classList.add("drop");
    });
    composer.addEventListener("dragleave", function (e) {
      if (!composer.contains(e.relatedTarget)) composer.classList.remove("drop");
    });
    composer.addEventListener("drop", function (e) {
      composer.classList.remove("drop");
      if (!hasFiles(e)) return;
      e.preventDefault();
      ctext.focus();
      addFiles(imageFiles(e.dataTransfer));
    });
  }
