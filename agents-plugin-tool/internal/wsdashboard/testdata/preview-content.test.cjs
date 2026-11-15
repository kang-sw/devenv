var kinds = {}, cards = [], replies = [], calls = [];
function fetch(url, options) {
  calls.push({ url, options });
  var reply = replies.shift();
  return Promise.resolve({ ok: true, json: () => reply });
}
var window = { navigator: { pdfViewerEnabled: true }, WsCards: {
  register: (name, kind) => kinds[name] = kind,
  list: () => cards,
  open: (kind, data) => {
    var card = { data, body: new El('div'), el: new El('div'),
      title(v) { this.head = v; }, actions() {}, paint() {}, bump() {}, fold() {}, show() {}, save() {} };
    cards.push(card); kinds[kind].open(card); return card;
  }
} };
eval(PREVIEW);
async function open(path, answer, options) {
  replies.push(answer);
  var card = window.WsPreview.open(path, options);
  for (var i = 0; i < 8; i++) await Promise.resolve();
  return card;
}
(async function () {
  var source = 'package main\n\n// <script>alert(1)</script>\n', highlighted = [
    [{ s: 'package', c: 'k' }, { s: ' main' }], [], [{ s: '// <script>alert(1)</script>', c: 'c' }], []
  ];
  var card = await open('main.go', { kind: 'text', text: source, highlighted_lines: highlighted }, { line: 3 });
  var pre = find(card.body, 'PRE');
  check('highlighting retains exact source and blank lines', pre.textContent, source);
  check('tokens use existing chroma styles', [pre.className, pre.children[0].className], ['pv-txt chroma', 't-k']);
  check('markup is text, never executable DOM', [find(pre, 'SCRIPT'), pre.innerHTML], [null, undefined]);
  check('line navigation still asks for text and marks fixed line height', [calls[0].url, card.body.children[0].children[0].children[1].style.top], ['/s/rid1/fs/preview?path=main.go&text=1', '36px']);
  for (var answer of [{ kind: 'text', text: 'unknown' }, { kind: 'text', text: 'capped' },
    { kind: 'text', text: 'normalized\r\n', highlighted_lines: [[{ s: 'normalized', c: 'k' }], []] }]) {
    card = await open('plain.txt', answer);
    pre = find(card.body, 'PRE');
    check('absent or mismatched tokens fall back to source', [pre.className, pre.textContent], ['pv-txt', answer.text]);
  }
  card = await open('empty.go', { kind: 'text' });
  check('omitted empty source stays empty', find(card.body, 'PRE').textContent, '');
  check('PDF links stay opaque; only server pdf answers opt in', [window.WsPreview.takes('doc.pdf'), window.WsPreview.takes('/outside/DOC.PDF')], [false, false]);
  var pdf = { kind: 'pdf', src: '/s/rid1/fs/raw?path=doc.pdf' };
  card = await open('doc.pdf', pdf);
  var box = card.body.children[0], frame = find(box, 'IFRAME'), link = find(box, 'A');
  check('native PDF uses same-origin raw URL without HTML sandbox', [box.className, frame.src, frame.attrs.sandbox, frame.attrs.referrerpolicy], ['pv-pdf', pdf.src, undefined, 'no-referrer']);
  check('fallback is always available inside existing preview', [link.href, link.target, link.rel, link.textContent, box.children.length], [pdf.src, '_blank', 'noopener noreferrer', 'js.preview.pdf.open', 2]);
  check('PDF badge and fullscreen category', [card.head.badge, card.el.attrs['data-preview-kind']], ['js.preview.kind.pdf', 'pdf']);
  frame.on.error();
  check('frame failure leaves link, not a blank frame', [find(box, 'IFRAME'), find(box, 'A') === link, box.hasAttribute('data-viewer')], [null, true, false]);
  for (var supported of [false, undefined]) {
    window.navigator.pdfViewerEnabled = supported;
    card = await open('doc.pdf', pdf);
    check('absent viewer offers link without requesting PDF frame', [find(card.body, 'IFRAME'), find(card.body, 'A').href, card.body.children[0].children.length], [null, pdf.src, 1]);
  }
  // A late error from a replaced PDF frame is confined to its detached box.
  card = await open('main.go', { kind: 'text', text: 'current' });
  frame.on.error();
  check('late PDF event cannot replace another preview', find(card.body, 'PRE').textContent, 'current');
  wsT = function (key, reason) { return key === 'js.preview.none' ? key + ':' + reason : key; };
  root.setAttribute('data-sysopen', '');
  for (var kind of ['video', 'audio']) {
    var mediaAnswer = { kind, src: '/s/rid1/fs/raw?path=media' };
    card = await open('media', mediaAnswer);
    var player = find(card.body, kind.toUpperCase());
    check(kind + ' uses native controls and metadata only', [player.src, player.controls, player.preload], [mediaAnswer.src, true, 'metadata']);
    check(kind + ' has kind badge and sizing category', [card.head.badge, card.el.attrs['data-preview-kind']], ['js.preview.kind.' + kind, kind]);
    player.on.error();
    check(kind + ' error uses existing localized none fallback', [card.body.children[0].className, card.body.children[0].children[0].textContent, card.el.attrs['data-preview-kind'], card.head.icon], ['pv-none', 'js.preview.none:js.preview.media.unsupported', 'none', 'file']);
    check(kind + ' error still offers native open', find(card.body, 'BUTTON').textContent, 'js.preview.openext');
    card = await open('current.txt', { kind: 'text', text: 'current' });
    player.on.error();
    check(kind + ' late error cannot replace current preview', find(card.body, 'PRE').textContent, 'current');
    card = await open('media', mediaAnswer);
    player = find(card.body, kind.toUpperCase());
    // Starting another load invalidates the old player even before its answer.
    replies.push({ kind: 'text', text: 'reloaded' });
    kinds.preview.open(card);
    player.on.error();
    check(kind + ' pending reload ignores old error', card.body.children[0].className, kind === 'video' ? 'pv-vid' : 'pv-audio');
    for (var i = 0; i < 8; i++) await Promise.resolve();
    check(kind + ' reload completes normally', find(card.body, 'PRE').textContent, 'reloaded');
  }
  process.stdout.write(bad.join('\n'));
})().catch(e => { console.error(e); process.exitCode = 1; });
