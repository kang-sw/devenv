// Offline browser-state contracts; every transport and DOM dependency is fake.
const assert = require('node:assert/strict');
const vm = require('node:vm');
const { readAssetSource } = require('./asset-source.cjs');
const source = readAssetSource('wsdashboard', 'dashboard.js');
function block(from, to) {
  const start = source.indexOf(from), end = source.indexOf(to, start);
  assert(start >= 0 && end > start, from);
  return source.slice(start, end);
}
const tick = async () => { for (let i = 0; i < 5; i++) await new Promise(r => setImmediate(r)); };
const ctx = vm.createContext({ console, Date, Promise });
vm.runInContext(block('  function searchAnswerClient(io)', '  function paletteSearch(o)'), ctx);
function fakeClient(context = '', history = []) {
  const starts = [], watches = [], cancelled = [], saved = [], pending = [];
  const io = { context, history, draw() {}, saveContext: v => saved.push(v), saveHistory: v => saved.push(v),
    cancel: id => cancelled.push(id), start: body => new Promise((resolve, reject) => { starts.push(body); pending.push({ resolve, reject }); }),
    watch: (id, receive) => { const w = { id, receive, closed: false }; watches.push(w); return () => { w.closed = true; }; } };
  return { client: ctx.searchAnswerClient(io), starts, watches, cancelled, saved, pending };
}
async function turn(f, question, id, summary, text = 'answer') {
  f.client.ask(question); await tick();
  f.pending.shift().resolve({ id, status: 'running', text: '' }); await tick();
  f.watches.at(-1).receive({ id, status: 'done', text, context: summary }); await tick();
}
async function stateCases() {
  const f = fakeClient();
  await turn(f, 'first', 'a', 'first question and answer');
  await turn(f, 'second', 'b', 'first and second questions and answers');
  await turn(f, 'third', 'c', 'all three turns');
  assert.deepEqual(f.starts.map(s => s.context), ['', 'first question and answer', 'first and second questions and answers']);
  assert.equal(f.client.state().context, 'all three turns', 'follow-ups use rolling context, not previous Q&A');
  f.client.ask('fourth'); await tick();
  f.pending.shift().resolve({ id: 'd', status: 'running', text: '' }); await tick();
  const d = f.watches.at(-1);
  d.receive({ id: 'd', status: 'running', text: 'partial', context: 'not settled' });
  assert.equal(f.client.state().context, 'all three turns', 'streamed summaries cannot promote');
  f.client.cancel();
  d.receive({ id: 'd', status: 'done', text: 'late', context: 'stale' });
  assert.equal(f.client.state().context, 'all three turns', 'cancelled callbacks cannot promote');
  assert(d.closed); assert(f.cancelled.includes('d'));
  await turn(f, 'legacy', 'e', undefined);
  assert.equal(f.client.state().context, 'all three turns', 'legacy optional-field payload retains current summary');
  f.client.ask('failed'); await tick(); f.pending.shift().resolve({ id: 'failure', status: 'running' }); await tick();
  f.watches.at(-1).receive({ id: 'failure', status: 'failed', text: '', error: 'unavailable', context: 'bad' });
  assert.equal(f.client.state().context, 'all three turns');
  await turn(f, 'unicode', 'u', '😀'.repeat(1700));
  assert.equal(Array.from(f.client.state().context).length, 1600);
  assert.equal(Array.from(f.client.state().context).at(-1), '😀', 'cap does not split surrogate pairs');
  await turn(f, 'more', 'm', 'all'); await turn(f, 'last', 'z', 'all');
  assert.equal(f.client.state().history.length, 5, 'successful history stays capped');
  const count = f.client.state().history.length;
  f.client.select(4);
  await turn(f, 'after browsing history', 'h', 'newest');
  assert.equal(f.starts.at(-1).context, 'all', 'browsing old answers never rolls context back');
  f.client.reset();
  assert.equal(f.client.state().context, ''); assert.equal(f.client.state().current, null);
  assert.equal(f.client.state().history.length, count, 'reset retains last-five history');
  await turn(f, 'after reset', 'r', 'fresh'); assert.equal(f.starts.at(-1).context, '');
  const restored = fakeClient('😀'.repeat(2000), f.client.state().history);
  restored.client.ask('restored'); await tick(); assert.equal(Array.from(restored.starts[0].context).length, 1600);

  // A delayed older creation cannot become the server's newest question.
  const race = fakeClient('stable');
  race.client.ask('older'); await tick(); race.client.ask('newer'); await tick();
  assert.equal(race.starts.length, 1, 'POST creation is serialized');
  race.pending.shift().resolve({ id: 'old', status: 'running' }); await tick();
  assert(race.cancelled.includes('old')); assert.equal(race.starts.length, 2);
  race.pending.shift().resolve({ id: 'new', status: 'running' }); await tick();
  const newest = race.watches.at(-1);
  newest.receive({ id: 'old', status: 'done', context: 'wrong id' });
  assert.equal(race.client.state().context, 'stable');
  newest.receive({ id: 'new', status: 'done', text: 'correct', context: 'current' });
  assert.equal(race.client.state().context, 'current');
  race.client.ask('reset while creating'); await tick(); race.client.reset();
  race.pending.shift().resolve({ id: 'late-reset', status: 'done', context: 'resurrected' }); await tick();
  assert(race.cancelled.includes('late-reset')); assert.equal(race.client.state().context, '');
  assert.equal(race.client.state().current, null);
}
function citationCases() {
  const rows = [{ kind: 'board-item', id: '23', topic: 'topic' }];
  assert.equal(ctx.searchCitation('[[board:23]]', rows).board, rows[0]);
  assert.equal(ctx.searchCitation('[[board:999]]', rows), null);
  assert.equal(ctx.searchCitation('[[file:docs/readme.md:2]]', rows).line, 2);
  for (const marker of ['[[file:../secret:1]]', '[[file:/tmp/secret:1]]', '[[file:C:/secret:1]]', '[[file:a/../b:1]]', '[[file:a\\b:1]]', '[[file:a//b:1]]', '[[file:a:0]]', '[[file:a:99999999999999999999]]']) {
    assert.equal(ctx.searchCitation(marker, rows), null, marker);
  }
}

// A minimal inert DOM tests host wiring rather than evaluating model markup.
class Node {
  constructor(tag = 'div') { this.tag = tag; this.children = []; this.attrs = {}; this.listeners = {}; this.value = ''; this.hidden = false; this.nodes = {}; }
  set innerHTML(s) {
    this.html = s; this.nodes = {};
    if (s.includes('<input')) this.nodes.input = new Node('input');
    if (s.includes('class="tk-qanswer-text"')) this.nodes['.tk-qanswer-text'] = new Node();
    for (const m of s.matchAll(/data-answer-cite="(\d+)"/g)) this.nodes[`[data-answer-cite="${m[1]}"]`] = new Node('span');
  }
  get innerHTML() { return this.html || ''; }
  appendChild(n) { this.children.push(n); return n; }
  replaceChildren(...ns) { this.children = ns; }
  setAttribute(k, v) { this.attrs[k] = v; }
  getAttribute(k) { return this.attrs[k]; }
  addEventListener(k, fn) { this.listeners[k] = fn; }
  querySelector(s) { return this.nodes[s] || null; }
  focus() {}
}
async function hostCases() {
  const storage = {}, reads = [], requests = [], streams = [], picks = [], previews = [], checks = [];
  let local = true;
  class EventSource {
    constructor(url) { this.url = url; streams.push(this); }
    addEventListener(k, fn) { this[k] = fn; }
    close() { this.closed = true; }
    send(status, text, context) {
      const id = this.url.split('/').at(-2);
      this.region({ data: JSON.stringify({ name: 'answer', html: `<section data-answer-id="${id}" data-answer-status="${status}"><pre>${text}</pre></section>`, context }) });
    }
  }
  const doc = { activeElement: null, createElement(tag) {
    const n = new Node(tag);
    if (tag === 'template') {
      n.content = { querySelector() {
        const id = /data-answer-id="([^"]+)"/.exec(n.innerHTML), status = /data-answer-status="([^"]+)"/.exec(n.innerHTML), pre = /<pre>([\s\S]*)<\/pre>/.exec(n.innerHTML);
        if (!id || !status) return null;
        const section = new Node(); section.attrs = { 'data-answer-id': id[1], 'data-answer-status': status[1] };
        section.nodes.pre = { textContent: pre ? pre[1] : '' }; return section;
      } };
    }
    return n;
  } };
  const host = vm.createContext({ console, Date, Promise, AbortController, EventSource, doc, rid: 'session', sbody: null,
    root: { hasAttribute: () => local }, load: k => { reads.push(k); return storage[k] || null; }, save: (k, v) => { storage[k] = v; },
    esc: s => String(s).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/"/g, '&quot;'), wsT: k => k,
    paletteRows: () => [{ kind: 'board-item', id: '23', topic: 'topic' }], palettePick: (...a) => picks.push(a),
    paletteFs: (route, params) => { checks.push(params); return Promise.resolve({ kind: 'text', text: 'one\ntwo\nthree' }); },
    window: { EventSource, addEventListener() {}, WsPreview: { open: (...a) => previews.push(a) } },
    fetch: (url, options) => { const body = options.body && JSON.parse(options.body); requests.push({ url, body }); return Promise.resolve({ ok: true, status: 201, text: () => Promise.resolve(JSON.stringify({ id: 'answer-' + requests.length, status: 'running', text: '' })) }); }
  });
  vm.runInContext(block('  function searchAnswerClient(io)', '  function tkMount()'), host);
  local = false; const remote = host.paletteSearch({});
  assert.equal(remote.searchAsk, undefined); assert.equal(reads.length, 0, 'remote views never load private history');
  local = true; host.rid = ''; const hub = host.paletteSearch({});
  assert.equal(hub.searchAsk, undefined); assert.equal(reads.length, 0, 'non-session pages have no agent state');
  host.rid = 'session';
  const o = host.paletteSearch({}), pane = new Node(); o.searchAnswerMount(pane);
  o.searchAsk('first'); await tick();
  assert.equal(requests[0].body.context, '');
  assert.equal(streams.length, 1);
  streams[0].send('running', 'Read [[board:23]] [[file:docs/readme.md:2]] [[file:../secret:1]] [[file:docs/readme.md:8]] <img onerror=evil>'); await tick();
  let body = pane.querySelector('.tk-qanswer-text');
  assert(body.innerHTML.includes('&lt;img onerror=evil>'), 'model HTML is text');
  const board = body.querySelector('[data-answer-cite="0"]').children[0]; board.listeners.click();
  assert.equal(picks[0][0].id, '23'); assert.equal(picks[0][1].keep, true);
  const file = body.querySelector('[data-answer-cite="1"]').children[0]; file.listeners.click();
  assert.equal(previews[0][0], 'docs/readme.md'); assert.equal(previews[0][1].line, 2);
  assert.equal(body.querySelector('[data-answer-cite="2"]').children.length, 0, 'outside-root citation stays literal');
  assert.equal(body.querySelector('[data-answer-cite="3"]').children.length, 0, 'out-of-range file line stays literal');
  const n = checks.length;
  streams[0].send('done', 'Read [[file:docs/readme.md:2]]', 'rolling first'); await tick();
  assert.equal(checks.length, n, 'stream redraws reuse file validation'); assert(streams[0].closed);
  o.searchAsk('second'); await tick(); assert.equal(requests.at(-1).body.context, 'rolling first');
  const old = streams[0]; old.send('done', 'late', 'stale');
  const reset = new Node('button'); reset.attrs['data-answer-reset'] = '';
  reset.hasAttribute = k => Object.hasOwn(reset.attrs, k); reset.closest = () => reset;
  pane.listeners.click({ target: reset });
  streams.at(-1).send('done', 'late cancelled', 'bad');
  o.searchAsk('after reset'); await tick();
  assert.equal(requests.at(-1).body.question, 'after reset'); assert.equal(requests.at(-1).body.context, '');
}
(async () => { await stateCases(); citationCases(); await hostCases(); console.log('search answer: cumulative context, caps, reset, legacy, stale transport, local gates and citation dispatch passed'); })().catch(err => { console.error(err); process.exit(1); });
