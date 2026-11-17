'use strict';
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const prelude = fs.readFileSync(path.join(__dirname, '../pill_segments_test.go'), 'utf8').match(/const pillSegPrelude = `([\s\S]*?)\n`/)[1];
const source = fs.readFileSync(path.join(__dirname, '../assets/pill.js'), 'utf8');
const c = vm.createContext({console, setTimeout() { throw Error('scroll must not arm a timer'); }});
vm.runInContext(prelude + `
var audit = [], listeners = {}, animations = [];
window.scrollY = 20;
window.addEventListener = function (name, fn) { listeners[name] = fn; };
function instrument(e) {
  ["hidden", "textContent"].forEach(function (key) {
    var value = e[key];
    Object.defineProperty(e, key, { get: function () { return value; }, set: function (v) { audit.push("write:" + key); value = v; } });
  });
  e.style = new Proxy(e.style, {set: function (obj, key, value) { audit.push("write:style"); obj[key] = value; return true; }});
  var toggle = e.classList.toggle;
  e.classList.toggle = function (key, on) { audit.push("write:class"); toggle(key, on); };
  e.animate = function (frames) { audit.push("write:animate"); animations.push(frames); return {cancel: function () { audit.push("write:cancel"); }}; };
  return e;
}
var BaseE = E;
E = function (tag) { return instrument(new BaseE(tag)); };
E.prototype = BaseE.prototype;
E.prototype.cloneNode = function () { var copy = new E(this.tag); copy.cls = Object.assign({}, this.cls); copy.style = Object.assign({}, this.style); this.children.forEach(function (child) { copy.appendChild(child.cloneNode(true)); }); return copy; };
E.prototype.removeChild = function (child) { this.children.splice(this.children.indexOf(child), 1); child.parent = null; };
Object.defineProperty(E.prototype, "parentNode", {get: function () { return this.parent; }});
[host, html].concat(items).forEach(instrument);
var rect = E.prototype.getBoundingClientRect, attr = E.prototype.setAttribute;
E.prototype.getBoundingClientRect = function () { audit.push("read:rect"); return rect.call(this); };
E.prototype.setAttribute = function (key, value) { audit.push("write:attr"); attr.call(this, key, value); };
Object.defineProperty(E.prototype, "offsetWidth", {get: function () { audit.push("read:width"); return this.hidden ? 0 : 100 + this.textContent.length; }});
` + source, c);
function frame() {
  vm.runInContext('audit = []; frame();', c);
  const events = Array.from(c.audit);
  const write = events.findIndex(e => e.startsWith('write:'));
  assert.ok(write < 0 || !events.slice(write).some(e => e.startsWith('read:')), events.join(', '));
  return events;
}
frame(); // Both pills change; neither width is read after a write.
frame(); // Deferred intrinsic-width measurement and animation.
vm.runInContext('window.wbPill.update();', c);
assert.deepEqual(frame(), ['read:rect', 'read:rect', 'read:rect', 'read:rect', 'read:rect', 'read:width', 'read:width']);
vm.runInContext('for (var i = 0; i < 54; i++) listeners.scroll();', c);
assert.equal(c.frames.length, 1, 'scroll burst retains one pending rAF');
assert.ok(!frame().some(e => e.startsWith('write:')), 'unchanged visible set/labels makes no DOM writes');
vm.runInContext('items[0].top = 950; items[4].top = -50; window.wbPill.update();', c);
assert.ok(!frame().some(e => e.startsWith('write:')), 'same counts retain labels without DOM writes');
vm.runInContext('press(pill(0), "open");', c);
assert.equal(c.jumped[c.jumped.length - 1], 'b2', 'same-count membership changes still refresh navigation targets');
vm.runInContext('items[4].top = 950; window.wbPill.update();', c);
frame(); frame();
assert.equal(vm.runInContext('shown(pill(0))[0][2]', c), '1', 'counts follow current item geometry');
vm.runInContext('window.wbPill.off();', c);
console.log('pill layout: read-before-write, unchanged display, scroll coalescing and count refresh passed');
