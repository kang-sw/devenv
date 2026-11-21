const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

function animationContext() {
  const source = fs.readFileSync(path.join(__dirname, '../assets/dashboard-animation.js'), 'utf8');
  const root = { paused: false, hasAttribute(name) { return name === 'data-animation-paused' && this.paused; } };
  const doc = { documentElement: root, timeline: { currentTime: 0 } };
  const context = vm.createContext({ doc, Math });
  vm.runInContext(source, context);
  return { context, doc, root };
}

function element(owner) {
  return {
    style: { animationDelay: '' },
    closest() { return owner; }
  };
}

test('animation period table matches CSS durations', () => {
  const { context } = animationContext();
  const conversation = fs.readFileSync(path.join(__dirname, '../assets/dashboard-conversation.css'), 'utf8');
  const dashboard = fs.readFileSync(path.join(__dirname, '../assets/dashboard.css'), 'utf8');
  const rules = [
    ['thinking', conversation, '.hud-node[data-state="thinking"] .hud-ring', 'hud-spin-think'],
    ['tool', conversation, '.hud-node[data-state="tool"] .hud-ring', 'hud-spin-tool'],
    ['holding', conversation, '.hud-node[data-state="holding"] .hud-ring', 'hud-hold-breathe'],
    ['permission', conversation, '.hud-node[data-state="permission"] .hud-ring', 'hud-perm-pulse'],
    ['hbr', dashboard, '.hb .hbr', 'hbspin'],
    ['spin', dashboard, '.spin', 'dash-spin']
  ];
  for (const [key, css, selector, name] of rules) {
    const escaped = selector.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
    const match = css.match(new RegExp(`${escaped}\\s*\\{[^}]*animation:\\s*${name}\\s+(\\d*\\.?\\d+)(ms|s)`));
    assert(match, `${key}: CSS animation ${name} exists on ${selector}`);
    const duration = Number(match[1]) * (match[2] === 's' ? 1000 : 1);
    assert.equal(context.ANIMATION_PERIOD_MS[key], duration, `${key}: helper period matches CSS`);
  }
});

test('phase offsets are deterministic and bounded for each animation period', () => {
  const { context: a } = animationContext();
  const { context: b } = animationContext();
  assert.equal(a.animationPhaseOffset('agent-h3', 900), b.animationPhaseOffset('agent-h3', 900));
  assert.equal(a.animationPhaseOffset('', 900), 0);
  for (const period of Object.values(a.ANIMATION_PERIOD_MS)) {
    for (const key of ['agent-h3', 'agent-h5', 'session-123', '한글']) {
      const offset = a.animationPhaseOffset(key, period);
      assert(Number.isInteger(offset) && offset >= 0 && offset < period, `${key}: ${offset}/${period}`);
    }
  }
  assert.notEqual(a.animationPhaseOffset('agent-h3', 900), a.animationPhaseOffset('agent-h5', 900));
});

test('a remounted ring resumes the global phase while a stable ring keeps its delay', () => {
  const { context, doc, root } = animationContext();
  const old = element(null), fresh = element(null);
  const period = context.ANIMATION_PERIOD_MS.tool;
  doc.timeline.currentTime = 123.25;
  context.animationPhaseDelay(old, period, 'h3', 'tool');
  const originalDelay = Number.parseFloat(old.style.animationDelay);
  assert(originalDelay <= 0 && originalDelay > -period);
  doc.timeline.currentTime = 345.25;
  context.animationPhaseDelay(old, period, 'h3', 'tool');
  assert.equal(Number.parseFloat(old.style.animationDelay), originalDelay, 'same stamp does not restart animation');
  context.animationPhaseDelay(fresh, period, 'h3', 'tool');
  const resumedPhase = ((-Number.parseFloat(fresh.style.animationDelay)) % period + period) % period;
  const oldPhase = ((-originalDelay + 222) % period + period) % period;
  assert(Math.abs(resumedPhase - oldPhase) < 0.11, `${resumedPhase} vs ${oldPhase}`);
  context.animationPhaseDelay(old, context.ANIMATION_PERIOD_MS.thinking, 'h3', 'thinking');
  assert.match(old.wsAnimationStamp, /^thinking\|h3\|1150$/);
  assert(Number.parseFloat(old.style.animationDelay) > -1150);
  root.paused = true;
  const paused = element(null);
  context.animationPhaseDelay(paused, period, 'h3', 'tool');
  assert.equal(Number.parseFloat(paused.style.animationDelay), -context.animationPhaseOffset('h3', period));
});

test('shared spinner sync seeds from owner identity and respects existing stamps', () => {
  const { context, doc } = animationContext();
  doc.timeline.currentTime = 401;
  const owner = { getAttribute(name) { return { 'data-sid': 'session-a', 'data-key': 'other-key' }[name] || null; } };
  const hbr = element(owner), spin = element(owner);
  const root = {
    matches() { return false; },
    querySelectorAll(selector) { return selector === '.hb .hbr' ? [hbr] : selector === '.spin' ? [spin] : []; }
  };
  context.syncSpinnerPhases(root);
  assert.equal(hbr.wsAnimationStamp, 'hbr|session-a|1100');
  assert.equal(spin.wsAnimationStamp, 'spin|session-a|800');
  assert.notEqual(hbr.style.animationDelay, spin.style.animationDelay);
  const prior = spin.style.animationDelay;
  doc.timeline.currentTime = 700;
  context.syncSpinnerPhases(root);
  assert.equal(spin.style.animationDelay, prior);
});
