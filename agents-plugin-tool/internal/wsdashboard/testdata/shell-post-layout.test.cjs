'use strict';
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const source = fs.readFileSync(path.join(__dirname, '../assets/dashboard-lifecycle.js'), 'utf8');
const init = source.match(/\n  function shFrameInit\(\) \{[\s\S]*?\n  \}\n  \/\/ shShellInit/)[0];
const c = vm.createContext({console});
vm.runInContext(`
var reads = [], writes = [], posts = [], frames = new Map(), seq = 0, observers = [], listeners = {}, attrs = {};
var menus = [], dialogs = [], overlays = [], shHidden = false, shTrackerRev = null;
var header = {innerHTML: "head", offsetHeight: 40};
Object.defineProperty(header, "offsetHeight", {get: function () { reads.push("height"); return 40; }});
var root = {
  hasAttribute: function (k) { return k in attrs; },
  setAttribute: function (k) { writes.push(k); attrs[k] = ""; },
  removeAttribute: function (k) { writes.push(k); delete attrs[k]; }
};
Object.defineProperty(root, "clientWidth", {get: function () { reads.push("width"); return 1180; }});
function getComputedStyle() { reads.push("style"); return {paddingRight: "12px", backgroundColor: "rgba(0,0,0,.5)"}; }
function requestAnimationFrame(fn) { var id = ++seq; frames.set(id, fn); return id; }
function cancelAnimationFrame(id) { frames.delete(id); }
function flush() { var batch = Array.from(frames.values()); frames.clear(); batch.forEach(function (fn) { fn(); }); }
function MutationObserver(fn) { this.fn = fn; this.nodes = []; this.observe = function (n, options) { this.nodes.push([n, options]); }; observers.push(this); }
function ResizeObserver(fn) { this.observe = function () {}; }
var window = {innerWidth: 1200, MutationObserver: MutationObserver, ResizeObserver: ResizeObserver, scrollY: 0,
  WsShell: {alpha: function () { return .5; }}, addEventListener: function (k, fn) { listeners[k] = fn; }};
var doc = {body: {}, title: "title", querySelector: function () { return header; }, addEventListener: function () {}};
var location = {origin: "http://localhost", hash: ""}, history = {};
function region() { return header; }
function $() { return null; }
function q(sel) {
  if (sel === "dialog[open]") return dialogs;
  if (sel === ".rpop:not([hidden]), .lmenu") return menus;
  return overlays;
}
function shPost(kind, m) { posts.push([kind, m]); }
` + init + '\nshFrameInit(); flush();', c);
function run(code) { return vm.runInContext(code, c); }
assert.deepEqual(Array.from(c.reads), ['style', 'width', 'height']);
run('reads = []; writes = [];');
const rootObserver = c.observers.find(o => o.nodes.some(([n]) => n === c.root));
for (let i = 0; i < 54; i++) rootObserver.fn([]);
assert.equal(c.frames.size, 1, 'root mutation burst schedules one frame');
run('flush();');
assert.deepEqual(Array.from(c.reads), [], 'no header or overlay layout reads for ordinary board/root mutations');
assert.deepEqual(Array.from(c.writes), [], 'unchanged closed overlay state does not write attributes');
run(`menus = [{getClientRects: function () { reads.push("popover"); return [{}]; }}];`);
rootObserver.fn([]); run('flush();');
assert.deepEqual(Array.from(c.reads), ['popover']);
assert.equal(run('attrs["data-lifted"]'), '');
assert.equal(run('posts[posts.length - 1][1].popover'), true);
run('reads = []; menus = [];'); rootObserver.fn([]); run('flush();');
assert.deepEqual(Array.from(c.reads), []);
assert.equal(run('root.hasAttribute("data-lifted")'), false);
run(`dialogs = [{matches: function () { return true; }}]; reads = [];`);
rootObserver.fn([]); run('flush();');
assert.deepEqual(Array.from(c.reads), ['style'], 'only an open modal needs its backdrop style');
assert.equal(run('posts[posts.length - 1][1].dim'), .5);
run('dialogs = []; reads = []; listeners.resize(); flush();');
assert.deepEqual(Array.from(c.reads), ['style', 'width', 'height'], 'resize refreshes mirrored header geometry');
const headerObserver = c.observers.find(o => o.nodes.some(([n]) => n === c.header));
run('reads = []; header.innerHTML = "new header";'); headerObserver.fn([]); run('flush();');
assert.equal(run('posts.filter(function (p) { return p[0] === "head"; }).slice(-1)[0][1].html'), 'new header');
console.log('shell post layout: cached header, closed-overlay fast path, frame coalescing and open/close geometry passed');
