// Production preview state machine with a minimal card owner.
var kinds = {}, list = [], changes = 0;
document.addEventListener('ws-file-cards', () => changes++);
function fetch() { return Promise.resolve({ ok: true, json: () => ({ kind: 'text', text: 'fixture' }) }); }
function make(data) {
  var c = { data, el: new El('div'), body: new El('div'), pinned: 0,
    title() {}, actions() {}, paint() {}, save() {}, fold() {}, show() {}, bump() {},
    close() { list.splice(list.indexOf(c), 1); kinds.preview.close(c); },
    pin(on) { c.pinned = on ? 1 : 0; kinds.preview.pinned(c); }
  };
  c.name = c.el.appendChild(new El('span'));
  list.push(c); kinds.preview.open(c); return c;
}
var window = { WsCards: {
  register: (name, kind) => kinds[name] = kind,
  list: () => list,
  open: (kind, data) => make(data),
  relayout() {}
} };
eval(PREVIEW);
var P = window.WsPreview;
var a = P.open('a.txt'), b = P.open('b.txt');
check('single click replaces one preview slot', [a === b, list.length, b.data.path, b.preview], [true, 1, 'b.txt', true]);
var opened = P.open('b.txt', { keep: true });
check('double click promotes the same card', [opened === b, opened.preview, P.state('b.txt')], [true, false, 'open']);
var preview = P.open('c.txt');
check('open cards survive preview replacement', [list.length, opened.data.path, preview !== opened], [2, 'b.txt', true]);
preview.el.on.dblclick({ target: { closest: s => s === '.bc-hd' ? {} : null } });
check('header double click promotes without replacement', [preview.preview, P.state('c.txt'), list.length], [false, 'open', 2]);
var pinned = P.open('d.txt'); pinned.pin(true);
check('pin empties slot', [pinned.preview, P.state('d.txt')], [false, 'pinned']);
var next = P.open('e.txt'); pinned.pin(false);
check('unpin is open and leaves preview intact', [P.state('d.txt'), P.state('e.txt'), list.includes(next)], ['open', 'preview', true]);
check('reopening durable file never demotes or replaces preview', [P.open('d.txt') === pinned, P.state('d.txt'), P.state('e.txt')], [true, 'open', 'preview']);
next.el.on.dblclick({ target: { closest: () => ({}) } });
check('header action double click does not promote', P.state('e.txt'), 'preview');
next.close();
check('closing frees slot and row marker', [P.state('e.txt'), list.includes(next)], ['', false]);
for (var i = 0; i < 40; i++) P.open('open-' + i, { keep: true });
check('no open-card cap', list.length, 43);
var restored = make({ path: 'restored.txt' });
check('restored card has no transient preview state', [restored.preview, P.state('restored.txt')], [false, 'open']);
check('storage data is unchanged', restored.data, { path: 'restored.txt' });
check('state changes notify explorer markers', changes > 10, true);
process.stdout.write(bad.join('\n'));
