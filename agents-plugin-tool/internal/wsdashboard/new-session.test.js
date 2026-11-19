const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');

test('new session saves only successful starts and restores available per-harness choices', () => {
  class Element {
    constructor(tag) { this.tag = tag; this.children = []; this.attrs = {}; this.events = {}; this.classList = { toggle() {} }; this._value = ''; }
    appendChild(c) { this.children.push(c); return c; }
    set textContent(v) { this.children = []; this.text = v; }
    get textContent() { return this.text || ''; }
    set value(v) { this._value = v; }
    get value() { return this._value || (this.tag === 'select' && this.children[0] ? this.children[0].value : ''); }
    setAttribute(k, v) { this.attrs[k] = v; }
    getAttribute(k) { return this.attrs[k] || ''; }
    addEventListener(k, f) { this.events[k] = f; }
    focus() {}
  }
  let raw = '{bad', pending, reply, writes = 0;
  const hs = [ { harness: 'claude', lead: true, available: true, models: [{ id: 'opus' }], efforts: ['high'], permission_modes: ['auto', 'plan'] }, { harness: 'pi', lead: true, available: true, models: [{ id: 'gpt' }], efforts: ['low'], permission_modes: [] } ];
  const c = { mode: 'index', root: { hasAttribute: () => false }, supChoices: null,
    localStorage: { getItem: () => raw, setItem(k, v) { assert.equal(k, 'ws.dashboard.newSession'); raw = v; writes++; } },
    sessionStorage: { setItem() {} }, RENAME_NEW: 'rename',
    supHarness(k) { return c.supChoices && c.supChoices.harnesses.find(h => h.harness === k); },
    tkFetch(path, opt, cb) { pending = cb; }, wsT: s => s, mk: (tag) => new Element(tag), rpPop: () => new Element('div'), rpBtn: () => new Element('button'), rpId: () => 'id',
    gIcon: () => new Element('icon'), rpLive: () => true, railPopPlace() {}, railPopClose() {}, railOpen() {}, saveDraft() {}, notifAsk() {}, go() {}, toast() {},
    WsRail: { leadBody: (root, v) => v }, railPost(path, body, cb) { reply = cb; }
  };
  vm.createContext(c);
  const source = fs.readFileSync(__dirname + '/assets/dashboard-leads.js', 'utf8');
  vm.runInContext(source.slice(0, source.indexOf('  // The session menu of the viewed hosted session')), c);
  const p = c.rpLeadMake(), btn = new Element('button');
  const find = (el, pred) => pred(el) ? el : el.children.map(x => find(x, pred)).find(Boolean);
  const all = (el, tag) => [ ...(el.tag === tag ? [el] : []), ...el.children.flatMap(x => all(x, tag)) ];
  const click = el => el.events.click({ target: el });
  p.open('root', 'name', btn);
  assert.equal(writes, 0);
  pending(200, { harnesses: hs });
  let selects = all(p.el, 'select');
  selects[0].value = 'opus'; selects[1].value = 'plan'; selects[2].value = 'high';
  const start = p.el.children.at(-1).children.at(-1);
  click(start); reply(500, {}, 'error'); assert.equal(writes, 0);
  click(start); reply(201, { id: '1' }); assert.equal(writes, 1);
  assert.deepEqual(JSON.parse(raw), { harness: 'claude', harnesses: { claude: { model: 'opus', mode: 'plan', effort: 'high', iso: 'default' } } });
  p.open('root', 'name', btn);
  selects = all(p.el, 'select'); assert.equal(selects[0].value, 'opus'); assert.equal(selects[1].value, 'plan');
  const segment = find(p.el, e => e.attrs['aria-label'] === 'js.sup.harness');
  const pi = find(segment, e => e.attrs['data-harness'] === 'pi');
  segment.events.click({ target: { closest: () => pi } });
  selects[0].value = 'gpt'; selects[2].value = 'low'; click(start); reply(201, { id: '2' });
  assert.equal(JSON.parse(raw).harnesses.claude.model, 'opus');
  c.supChoices = null;
  p.open('root', 'name', btn); assert.equal(start.disabled, true);
  hs[1].models = []; hs[1].efforts = ['high']; pending(200, { harnesses: hs });
  assert.equal(pi.attrs['aria-checked'], 'true');
  assert.equal(selects[0].value, ''); assert.equal(selects[2].value, '');
  const before = raw;
  const cancel = p.el.children.at(-1).children.at(-2);
  click(cancel); assert.equal(raw, before);
  p.open('root', 'name', btn); assert.equal(raw, before);
  raw = JSON.stringify({ harness: 'claude', harnesses: { claude: { model: 'gone', mode: 'gone', effort: 'gone', iso: 'gone' } } });
  p.open('root', 'name', btn);
  assert.equal(selects[0].value, ''); assert.equal(selects[1].value, 'auto'); assert.equal(selects[2].value, '');
  c.localStorage.getItem = () => { throw Error('blocked'); };
  c.localStorage.setItem = () => { throw Error('blocked'); };
  p.open('root', 'name', btn); click(start); reply(201, { id: '3' });
});

test('new session popover rereads the harness choices on every open and keeps the form', () => {
  class Element {
    constructor(tag) { this.tag = tag; this.children = []; this.attrs = {}; this.events = {}; this.classList = { toggle() {} }; this._value = ''; }
    appendChild(c) { this.children.push(c); return c; }
    set textContent(v) { this.children = []; this.text = v; }
    get textContent() { return this.text || ''; }
    set value(v) { this._value = v; }
    get value() { return this._value || (this.tag === 'select' && this.children[0] ? this.children[0].value : ''); }
    setAttribute(k, v) { this.attrs[k] = v; }
    getAttribute(k) { return this.attrs[k] || ''; }
    addEventListener(k, f) { this.events[k] = f; }
    focus() {}
  }
  const pending = [];
  const harness = models => ({ harness: 'codex', lead: true, available: true, models: models.map(id => ({ id })), efforts: ['low'], permission_modes: ['read-only'] });
  const c = { mode: 'index', root: { hasAttribute: () => false }, supChoices: { harnesses: [harness(['gpt-old'])] },
    localStorage: { getItem: () => JSON.stringify({ harness: 'codex' }), setItem() {} }, sessionStorage: { setItem() {} }, RENAME_NEW: 'rename',
    supHarness(k) { return c.supChoices && c.supChoices.harnesses.find(h => h.harness === k); },
    tkFetch(path, opt, cb) { assert.equal(path, '/support/choices'); pending.push(cb); }, wsT: s => s, mk: (tag) => new Element(tag), rpPop: () => new Element('div'), rpBtn: () => new Element('button'), rpId: () => 'id',
    gIcon: () => new Element('icon'), rpLive: () => true, railPopPlace() {}, railPopClose() {}, railOpen() {}, saveDraft() {}, notifAsk() {}, go() {}, toast() {},
    WsRail: { leadBody: (root, v) => v }, railPost() {}
  };
  vm.createContext(c);
  const source = fs.readFileSync(__dirname + '/assets/dashboard-leads.js', 'utf8');
  vm.runInContext(source.slice(0, source.indexOf('  // The session menu of the viewed hosted session')), c);
  const p = c.rpLeadMake(), btn = new Element('button');
  const all = (el, tag) => [ ...(el.tag === tag ? [el] : []), ...el.children.flatMap(x => all(x, tag)) ];
  const options = el => el.children.map(o => o.value);
  p.open('root', 'name', btn);
  // The held list answers at once; the read is still sent.
  assert.equal(pending.length, 1);
  const model = all(p.el, 'select')[0];
  assert.deepEqual(options(model), ['', 'gpt-old']);
  model.value = 'gpt-old';
  pending.shift()(200, { harnesses: [harness(['gpt-old', 'gpt-new'])] });
  assert.deepEqual(options(model), ['', 'gpt-old', 'gpt-new']);
  assert.equal(model.value, 'gpt-old', 'a pick survives the refresh');
  // A failed read keeps the held list.
  p.open('root', 'name', btn);
  assert.equal(pending.length, 1);
  pending.shift()(500, {});
  assert.deepEqual(options(model), ['', 'gpt-old', 'gpt-new']);
});
