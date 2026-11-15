// Minimal read-only DOM adapter for the Go-rendered search fixture. It models
// template content separately; it does not claim native layout or frame execution.
const assert = require('node:assert/strict');
const vm = require('node:vm');
module.exports = function renderedEntries(source, fixture) {
  class Node {
    constructor(data = {}, parent = null) {
      this.tagName = (data.tag || '').toUpperCase();
      this.text = data.text || '';
      this.attrs = { ...data.attrs };
      this.parentElement = parent;
      this.children = (data.children || []).map(n => new Node(n, this));
      if (this.tagName === 'TEMPLATE') this.content = new Node({ children: data.content });
    }
    get id() { return this.attrs.id || ''; }
    get textContent() { return this.text + this.children.map(n => n.textContent).join(''); }
    getAttribute(name) { return this.attrs[name] ?? null; }
    hasAttribute(name) { return name in this.attrs; }
    matches(selector) {
      const attr = /\[([\w-]+)(\^=|=)?["']?([^\]"']*)["']?\]/.exec(selector);
      if (attr && (!this.hasAttribute(attr[1]) || (attr[2] === '=' && this.attrs[attr[1]] !== attr[3]) ||
        (attr[2] === '^=' && !this.attrs[attr[1]].startsWith(attr[3])))) return false;
      const base = selector.replace(/\[[^\]]*\]/g, ''), tag = /^[\w-]+/.exec(base);
      return (!tag || this.tagName === tag[0].toUpperCase()) &&
        [...base.matchAll(/\.([\w-]+)/g)].every(m => (this.attrs.class || '').split(/\s+/).includes(m[1]));
    }
    closest(selector) { for (let n = this; n; n = n.parentElement) if (n.matches(selector)) return n; return null; }
    querySelectorAll(selector) {
      if (selector.startsWith(':scope > ')) {
        let nodes = [this];
        for (const part of selector.slice(9).split(' > ')) nodes = nodes.flatMap(n => n.children.filter(c => c.matches(part)));
        return nodes;
      }
      const selectors = selector.split(',').map(s => s.trim().split(/\s+/));
      const out = [];
      function visit(n) {
        for (const child of n.children) {
          if (child.tagName && selectors.some(parts => {
            if (!child.matches(parts[parts.length - 1])) return false;
            let parent = child.parentElement;
            for (let i = parts.length - 2; i >= 0; i--) {
              while (parent && !parent.matches(parts[i])) parent = parent.parentElement;
              if (!parent) return false;
              parent = parent.parentElement;
            }
            return true;
          })) out.push(child);
          visit(child);
        }
      }
      visit(this);
      return out;
    }
    querySelector(selector) { return this.querySelectorAll(selector)[0] || null; }
    snapshot() {
      return { tag: this.tagName.toLowerCase(), text: this.text, attrs: this.attrs,
        children: this.children.map(n => n.snapshot()), content: this.content && this.content.children.map(n => n.snapshot()) };
    }
    cloneNode() { return new Node(this.snapshot()); }
    remove() { const list = this.parentElement.children; list.splice(list.indexOf(this), 1); }
    set innerHTML(value) {
      assert.equal(this.tagName, 'TEMPLATE', 'frame parsing stays inert');
      assert(fixture.frames[value], 'frame source came from the rendered board');
      this.content = new Node(fixture.frames[value]);
    }
  }
  const root = new Node(fixture.root);
  const q = (selector, node = root) => node.querySelectorAll(selector);
  root.createElement = tag => { assert.equal(tag, 'template'); return new Node({ tag }); };
  const ctx = vm.createContext({ q, framed: false, boardQ: selector => q(selector), doc: root, readItem: () => null });
  const start = source.indexOf('  function paletteText('), end = source.indexOf('  function paletteClosed(', start);
  assert(start >= 0 && end > start);
  vm.runInContext(source.slice(start, end), ctx);
  const pages = q('template[data-history-gap]');
  assert(pages.length > 0, 'fixture has unloaded timeline gaps');
  assert(pages.some(p => q('details.system-run', p.content).length > 0), 'a gap holds a folded system run');
  assert(q('details.panel-versions').length > 0 || pages.some(p => q('details.panel-versions', p.content).length > 0), 'fixture has an earlier panel version');
  const before = JSON.stringify(root.snapshot()), rows = JSON.parse(JSON.stringify(ctx.paletteRows()));
  assert.equal(JSON.stringify(root.snapshot()), before, 'indexing does not mutate the rendered DOM or templates');
  const entries = rows.filter(r => r.group === 5);
  const find = text => entries.filter(r => r.title.includes(text));
  for (const text of ['Older-page needle', 'Live log needle', 'Historical payload needle', 'Latest payload needle']) {
    assert.equal(find(text).length, 1, `one entry for ${text}`);
  }
  assert(!find('Historical payload needle')[0].title.includes('Latest payload needle'), 'version text is owned by its version');
  assert(!find('Latest payload needle')[0].title.includes('Historical payload needle'), 'newer panel excludes nested versions');
  assert(find('Latest payload needle')[0].title.includes('Latest panel recap'), 'panel recap and body share one row');
  assert(!entries.some(r => r.title.includes('forbiddenScriptNeedle')), 'frame scripts are not indexed');
  assert(!rows.some(r => r.group === 2 && r.topic === 'stream'), 'bodyless topic cannot inherit panel text or recap');
  assert(rows.some(r => r.group === 2 && r.topic === 'body' && r.title === 'Topic-owned body needle'));
  assert.equal(new Set(entries.map(r => r.entry)).size, entries.length, 'every entry/version has its own row');
  const allArticles = q('article.wb-entry').concat(pages.flatMap(p => q('article.wb-entry', p.content)));
  assert.equal(entries.length, allArticles.length, 'live and inert entries are all indexed');
  for (const article of allArticles) {
    const row = entries.find(r => r.entry === article.id);
    assert(row, article.id);
    const time = q('time.entry-at', article).find(t => t.closest('.wb-entry') === article);
    if (time) {
      assert(row.hint.endsWith(' · ' + time.textContent));
      if (article.getAttribute('data-kind') !== 'panel') assert(!row.title.includes(time.textContent), 'log/system time is not entry text');
    }
    if (article.getAttribute('data-kind') === 'system') assert(row.title.length > 0, 'folded system rows are searchable');
  }
  // Mounting changes the source attribute, not what search can read.
  const frame = q('.doc-frame').concat(pages.flatMap(p => q('.doc-frame', p.content)))[0], sourceDoc = frame.attrs['data-srcdoc'];
  assert(sourceDoc);
  frame.attrs.srcdoc = sourceDoc; delete frame.attrs['data-srcdoc'];
  assert.deepEqual(JSON.parse(JSON.stringify(ctx.paletteRows())), rows);
};
