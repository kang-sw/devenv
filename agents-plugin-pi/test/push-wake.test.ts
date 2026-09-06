import { test, afterEach } from 'node:test';
import assert from 'node:assert/strict';
import { registerGoalLoop } from '../src/goal-loop.ts';
import { registerLeadBootstrap } from '../src/lead-bootstrap.ts';
import { mkdtempSync, writeFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { heldPushQueue, leadIdleRef, leadCompactingRef, leadWakeStartPendingRef, pushToLead, registerPushFlush } from '../src/spawner.ts';

function harness(withGoal = false) {
  const handlers = new Map<string, Function[]>();
  const timers = new Map<number, Function>();
  let id = 0;
  let idle = true;
  let throws = false;
  let handledInput = false;
  let systemPrompt: string | undefined;
  const users: any[] = [], custom: any[] = [];
  const commands = new Map<string, any>(), tools = new Map<string, any>();
  const notices: string[] = [];
  const ctx: any = { isIdle: () => idle, getContextUsage: () => undefined, compact() {}, ui: {notify(text: string) {notices.push(text);}, setStatus() {}} };
  const clock = {
    scheduleTimer(cb: Function) { timers.set(++id, cb); return id as any; },
    clearTimer(handle: any) { timers.delete(handle); },
  };
  const pi: any = {
    on(event: string, fn: Function) { handlers.set(event, [...(handlers.get(event) ?? []), fn]); },
    registerCommand(name: string, def: any) { commands.set(name, def); },
    registerTool(def: any) { tools.set(def.name, def); },
    getCommands: () => [],
    sendUserMessage(content: unknown, options: unknown) {
      users.push({content, options});
      if (throws) throw Error('preflight');
      if (handledInput) return; // input hook handled the prompt: no bootstrap or start
      for (const hook of handlers.get('before_agent_start') ?? []) {
        systemPrompt = hook({systemPrompt: systemPrompt ?? 'base', prompt: content}, ctx)?.systemPrompt ?? systemPrompt;
      }
    },
    sendMessage(message: unknown, options: unknown) { assert.equal(idle, false, 'custom sends never start idle runs'); custom.push({message, options}); },
  };
  leadIdleRef.current = () => idle;
  const goal = withGoal ? registerGoalLoop(pi, { goalLoopConfigPath: '/nonexistent/push-wake.json', ...clock }) : undefined;
  registerPushFlush(pi, { delayMs: () => 10, ...clock });
  const emit = (event: string, payload: any = {}) => { let result; for (const fn of handlers.get(event) ?? []) result = fn(payload, ctx) ?? result; return result; };
  return {pi, users, custom, timers, emit, commands, tools, ctx, goal, notices,
    modelCall: () => systemPrompt,
    start() { idle = false; emit('agent_start'); },
    settle() { idle = true; emit('agent_settled'); },
    busy() { idle = false; }, fail() { throws = true; }, handleInput() { handledInput = true; },
    tick() { const [key, cb] = [...timers][0]!; timers.delete(key); cb(); },
    push(mode: 'steer'|'followUp', report = mode) { pushToLead(pi, undefined, undefined, 'ws-agent-report', {report}, mode); },
  };
}
afterEach(() => { heldPushQueue.length = 0; leadIdleRef.current = undefined; leadCompactingRef.current = false; });
for (const modes of [['followUp','steer'], ['steer','followUp']] as const) {
  test(`idle ${modes.join('/')} coalesces until confirmed start`, () => {
    const h = harness(); h.push(modes[0]); h.push(modes[1]);
    assert.equal(h.users.length, 1); assert.match(h.users[0].content, /1.*waiting/);
    assert.equal(h.custom.length, 0); assert.equal(heldPushQueue.length, 2);
    h.start(); assert.equal(h.custom.length, 2);
    assert.deepEqual(h.custom.map(x => x.options), modes.map(deliverAs => ({deliverAs, triggerTurn: true})));
    assert.equal(h.timers.size, 0); h.emit('session_shutdown');
  });
}
for (const failure of ['handled', 'throw']) test(`no-event ${failure} retries without losing pushes`, () => {
  const h = harness(); if (failure === 'throw') h.fail(); else h.handleInput();
  h.push('followUp'); h.push('steer'); h.tick();
  assert.equal(h.users.length, 2); assert.match(h.users[1].content, /2.*waiting/);
  assert.equal(heldPushQueue.length, 2); assert.equal(h.timers.size, 1);
  h.emit('session_shutdown'); assert.equal(h.timers.size, 0); assert.equal(heldPushQueue.length, 0);
  assert.equal(leadWakeStartPendingRef.current, false);
});
test('start cannot release compaction hold; busy release waits for settle', () => {
  const h = harness(); leadCompactingRef.current = true; h.push('steer'); h.start();
  assert.equal(h.custom.length, 0); assert.equal(h.users.length, 0);
  leadCompactingRef.current = false; h.settle(); assert.equal(h.custom.length, 0);
  h.start(); assert.equal(h.custom.length, 1); h.emit('session_shutdown');
});
test('ordinary busy steer is immediate and followUp waits for settled wake', () => {
  const h = harness(); h.busy(); h.push('followUp'); h.push('steer');
  assert.equal(h.custom.length, 1); h.settle(); assert.equal(h.users.length, 1);
  h.start(); assert.equal(h.custom.length, 2); h.emit('session_shutdown');
});

for (const order of ['reminder-first', 'push-first']) test(`${order}: one reservation, pushes do not spend reminder streak`, async () => {
  const h = harness(true);
  await h.commands.get('goal').handler('ship', h.ctx);
  h.settle();
  if (order === 'reminder-first') { h.tick(); h.push('followUp'); }
  else { h.push('followUp'); h.tick(); }
  assert.equal(h.users.length, 2, 'announcement plus just one wake');
  assert.equal(h.custom.length, 0);
  h.start(); assert.equal(h.custom.length, 1);
  const remainingReminders = order === 'reminder-first' ? 8 : 9;
  for (let i = 0; i < remainingReminders; i++) { h.settle(); h.tick(); h.start(); }
  assert.equal(h.notices.length, 0, 'push wake did not spend a reminder streak');
  h.settle(); h.tick();
  assert.match(h.notices[0], /Goal loop force-stopped/);
  assert.equal(h.users.length, 2 + remainingReminders);
  h.goal!.resetCompactionStateForShutdown(); h.emit('session_shutdown');
});

test('carry survives push wake and deferred compaction release after start', async () => {
  const h = harness(true);
  await h.commands.get('goal').handler('ship', h.ctx);
  const carry = '  Ω\t\r\n한글 🦦\n ';
  await h.tools.get('goal-compact-and-continue').execute('x', {carry_forward: carry}, undefined, undefined, h.ctx);
  h.push('followUp'); h.start();
  assert.equal(leadCompactingRef.current, true);
  assert.equal(h.custom.length, 0);
  h.emit('session_compact'); await new Promise(resolve => setImmediate(resolve));
  assert.equal(leadCompactingRef.current, false);
  assert.equal(h.custom.length, 0, 'busy release waits for settle');
  h.settle(); h.tick();
  assert.equal(h.users.length, 2, 'only counted push wake, no reminder');
  assert.ok(!h.users[1].content.includes(carry));
  h.start(); h.settle(); h.tick();
  assert.ok(h.users[2].content.endsWith(carry));
  h.goal!.resetCompactionStateForShutdown(); h.emit('session_shutdown');
});

test('idle compaction release cannot cancel held-push timeout', async () => {
  const h = harness(true);
  await h.commands.get('goal').handler('ship', h.ctx);
  await h.tools.get('goal-compact-and-continue').execute('x', {carry_forward: ''}, undefined, undefined, h.ctx);
  h.push('followUp'); h.emit('session_compact'); await new Promise(resolve => setImmediate(resolve));
  assert.equal(h.timers.size, 2, 'independent settle and wake recovery ownership');
  h.tick(); assert.equal(h.users.length, 3, 'push retry survives settle arming');
  h.tick(); assert.equal(h.custom.length, 0);
  assert.equal(h.timers.size, 1, 'only the pending push recovery remains');
  h.start(); h.goal!.resetCompactionStateForShutdown(); h.emit('session_shutdown');
});

for (const role of ['fork', 'worker', 'explore']) test(`${role} wake role containment`, () => {
  const previous = process.env.WS_PI_SPAWN_ROLE;
  process.env.WS_PI_SPAWN_ROLE = role;
  try {
    const h = harness(); h.push('followUp');
    assert.equal(h.users.length, role === 'fork' ? 1 : 0);
    assert.equal(h.timers.size, role === 'fork' ? 1 : 0);
    if (role === 'fork') { h.tick(); assert.equal(h.users.length, 2); h.start(); }
    h.emit('session_shutdown');
  } finally { if (previous === undefined) delete process.env.WS_PI_SPAWN_ROLE; else process.env.WS_PI_SPAWN_ROLE = previous; }
});

test('push user preflight runs actual bootstrap with live skill paths; override persists across model calls', () => {
  const dir = mkdtempSync(join(tmpdir(), 'push-bootstrap-'));
  try {
    const h = harness();
    registerLeadBootstrap(h.pi, {current: {manualSnapshot: 'manual snapshot', guideText: 'adapter guide'}}, {current: undefined});
    h.emit('before_agent_start', {systemPrompt: 'base', prompt: 'prior owner turn'}); // cache the old empty path set
    const skill = join(dir, 'SKILL.md');
    writeFileSync(skill, '---\nname: newly-discovered\ndescription: Fresh skill\n---\nBody');
    h.pi.getCommands = () => [{name: 'newly-discovered', source: 'skill', sourceInfo: {path: skill}}];
    h.push('followUp');
    assert.match(h.modelCall()!, /manual snapshot/); assert.match(h.modelCall()!, /adapter guide/);
    assert.match(h.modelCall()!, /newly-discovered/);
    h.start();
    const modelCalls = [h.modelCall(), h.modelCall()];
    assert.ok(modelCalls.every(prompt => prompt!.includes(skill)));
    h.emit('session_shutdown');
  } finally { rmSync(dir, {recursive: true, force: true}); }
});
