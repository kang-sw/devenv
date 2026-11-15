const assert = require("node:assert/strict");
const { readAssetSource } = require('./asset-source.cjs');
const vm = require("node:vm");
const source = readAssetSource('wsdashboard', 'dashboard.js');
const start = source.indexOf("  function hdTerms(seq) {");
const end = source.indexOf("  function hdFail(msg) {", start);
assert(start > 0 && end > start);

class El {
  constructor(tag, cls, text) { this.tag = tag; this.className = cls; this.textContent = text || ""; this.children = []; }
  appendChild(child) { this.children.push(child); return child; }
  addEventListener() {}
}
const anchor = { id: "hanchor", cwd: "/work/<b>plain</b>", session: "host0", state: "running" };
function fixture(reply = { anchor: true, terminals: [anchor] }) {
  const requests = [];
  const context = {
    window: { WsTerm: { list: cb => cb(null, { orphans: [{ id: "horphan", cwd: "/gone" }] }) } },
    fetch: (url, options) => { requests.push([url, options]); return Promise.resolve({ ok: true, json: () => Promise.resolve(reply) }); },
    hdSeq: 1, hdlist: new El("div"), hdnote: null,
    mk: (tag, cls, text) => new El(tag, cls, text), wsT: key => key,
    wtAge: () => "now", wtStamp: () => "now"
  };
  vm.createContext(context);
  vm.runInContext(source.slice(start, end), context);
  return { context, requests };
}
const flush = () => new Promise(resolve => setImmediate(resolve));
(async () => {
  const { context: c, requests } = fixture();
  c.hdTerms(1);
  await flush();
  assert.equal(requests[0][0], "/s/host0/terminals"); // no folder filter: holders shows every anchor terminal
  assert.equal(requests[0][1].credentials, "same-origin");
  assert.equal(requests[0][1].cache, "no-store");
  assert.equal(c.hdlist.children.length, 2);
  const box = c.hdlist.children[1];
  assert.equal(box.children[0].textContent, "js.hd.term.anchor");
  const row = box.children[1];
  assert.equal(row.title, "hanchor");
  const meta = row.children.find(el => el.className === "hdmeta");
  assert.equal(meta.children[0].textContent, anchor.cwd); // folder is plain text, never HTML
  assert.equal(meta.children[0].title, anchor.cwd);

  const stale = fixture().context;
  stale.hdTerms(1);
  stale.hdSeq = 2;
  await flush();
  assert.equal(stale.hdlist.children.length, 1); // only the synchronous orphan response, no stale anchor box

  for (const reply of [{ anchor: false, terminals: [anchor] }, { anchor: true, terminals: [] }]) {
    const c = fixture(reply).context;
    c.hdTerms(1);
    await flush();
    assert.equal(c.hdlist.children.length, 1);
  }
  console.log("holders anchor label, folder text and stale-response checks passed");
})().catch(err => { console.error(err); process.exitCode = 1; });
