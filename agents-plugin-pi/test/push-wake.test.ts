import { test, afterEach } from 'node:test';
import assert from 'node:assert/strict';
import { registerGoalLoop } from '../src/goal-loop.ts';
import { registerLeadBootstrap } from '../src/lead-bootstrap.ts';
import { mkdtempSync, writeFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { heldPushQueue, leadIdleRef, leadCompactingRef, leadWakeStartPendingRef, pushToLead, registerPushFlush, sendToLead } from '../src/spawner.ts';
import { PUSH_BATCH_CUSTOM_TYPE } from '../src/push-protocol.ts';

function harness(withGoal = false, steeringMode: 'one-at-a-time' | 'all' = 'one-at-a-time') {
  const handlers = new Map<string, Function[]>();
  const timers = new Map<number, Function>();
  let id = 0;
  let idle = true;
  let throws = false;
  let customThrows = false;
  let handledInput = false;
  let systemPrompt: string | undefined;
  const users: any[] = [], custom: any[] = [];
  let nextUserStart = 0;
  const steering: any[] = [], followUps: any[] = [], modelTimeline: string[] = [];
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
      if (!(options as {deliverAs?: string} | undefined)?.deliverAs) nextUserStart = users.length;
      if (throws) throw Error('preflight');
      if (handledInput) return; // input hook handled the prompt: no bootstrap or start
      for (const hook of handlers.get('before_agent_start') ?? []) {
        systemPrompt = hook({systemPrompt: systemPrompt ?? 'base', prompt: content}, ctx)?.systemPrompt ?? systemPrompt;
      }
    },
    sendMessage(message: unknown, options: unknown) {
      assert.equal(idle, false, 'custom sends never start idle runs');
      if (customThrows) throw Error('custom preflight');
      custom.push({message, options});
      ((options as {deliverAs?: string}).deliverAs === 'steer' ? steering : followUps).push(message);
    },
  };
  // Pi's default `one-at-a-time` queues drain one steering message before
  // each response; follow-ups drain only once that steering loop stops.
  const label = (message: unknown) => {
    const candidate = message as {customType?: string; details?: {report?: string; items?: Array<{customType?: string; details?: {report?: string; cmd_id?: string}}>}};
    return candidate.customType === PUSH_BATCH_CUSTOM_TYPE
      ? `batch:${candidate.details?.items?.map((item) => item.details?.report ?? item.details?.cmd_id ?? item.customType).join(',')}`
      : candidate.details?.report;
  };
  const drain = (queue: any[], mode: string) => {
    if (steeringMode === 'all' && queue.length) {
      modelTimeline.push(...queue.splice(0).map((message) => `${mode}:${label(message)}`), 'model-response');
      return;
    }
    while (queue.length) {
      modelTimeline.push(`${mode}:${label(queue.shift())}`);
      modelTimeline.push('model-response');
    }
  };
  const drainPi = () => { drain(steering, 'steer'); drain(followUps, 'followUp'); };
  leadIdleRef.current = () => idle;
  const registry = new Map();
  const goal = withGoal ? registerGoalLoop(pi, {
    goalLoopConfigPath: '/nonexistent/push-wake.json',
    rpcRegistryRef: {current: registry},
    ...clock,
  }) : undefined;
  registerPushFlush(pi, { delayMs: () => 10, ...clock });
  const emit = (event: string, payload: any = {}) => { let result; for (const fn of handlers.get(event) ?? []) result = fn(payload, ctx) ?? result; return result; };
  return {pi, users, custom, timers, emit, commands, tools, ctx, goal, notices, modelTimeline, registry,
    modelCall: () => systemPrompt,
    start() {
      idle = false;
      emit('agent_start');
      const admitted = users[nextUserStart++];
      if (admitted) emit('message_start', {message: {role: 'user', content: admitted.content, timestamp: Date.now()}});
      drainPi();
    },
    end() { emit('agent_end'); drainPi(); },
    settle() { idle = true; emit('agent_settled'); },
    busy() { idle = false; }, fail() { throws = true; }, failCustom() { customThrows = true; }, allowCustom() { customThrows = false; }, handleInput() { handledInput = true; },
    tick() { const [key, cb] = [...timers][0]!; timers.delete(key); cb(); },
    push(mode: 'steer'|'followUp', report = mode) { pushToLead(pi, undefined, undefined, 'ws-agent-report', {report}, mode); },
  };
}
afterEach(() => { heldPushQueue.length = 0; leadIdleRef.current = undefined; leadCompactingRef.current = false; leadWakeStartPendingRef.current = false; });
for (const modes of [['followUp','steer'], ['steer','followUp']] as const) {
  test(`idle ${modes.join('/')} coalesces until confirmed start`, () => {
    const h = harness(); h.push(modes[0]); h.push(modes[1]);
    assert.equal(h.users.length, 1); assert.match(h.users[0].content, /1.*waiting/);
    assert.equal(h.custom.length, 0); assert.equal(heldPushQueue.length, 2);
    h.start(); assert.equal(h.custom.length, 1);
    assert.equal(h.custom[0].message.customType, PUSH_BATCH_CUSTOM_TYPE);
    assert.deepEqual(h.custom[0].message.details.items.map((item: any) => item.details.report), [...modes]);
    assert.deepEqual(h.custom.map(x => x.options), [{deliverAs: 'steer', triggerTurn: true}]);
    assert.deepEqual(h.modelTimeline, [`steer:batch:${modes.join(',')}`, 'model-response'], 'one batch yields one response even in one-at-a-time steering mode');
    assert.equal(h.timers.size, 0); h.emit('session_shutdown');
  });
}
for (const order of ['raw-family', 'family-raw'] as const) {
  test(`confirmed-start steering preserves ${order} shared FIFO order`, () => {
    const h = harness();
    const raw = () => sendToLead(h.pi, {customType: 'ws-thread-summary', content: 'raw', display: true, details: {source: 'raw'}}, 'followUp');
    const family = () => h.push('followUp', 'family');
    if (order === 'raw-family') { raw(); family(); } else { family(); raw(); }

    assert.equal(heldPushQueue.length, 2);
    h.start();
    assert.deepEqual(h.custom.map((entry) => (entry.message as {customType?: string}).customType), [PUSH_BATCH_CUSTOM_TYPE]);
    assert.deepEqual(
      h.custom[0].message.details.items.map((item: any) => item.customType),
      order === 'raw-family' ? ['ws-thread-summary', 'ws-agent-report'] : ['ws-agent-report', 'ws-thread-summary'],
    );
    assert.deepEqual(h.custom.map((entry) => entry.options), [{deliverAs: 'steer', triggerTurn: true}]);
    h.emit('session_shutdown');
  });
}
test('an independent user start clears the pending wake reservation and releases steering', () => {
  const h = harness();
  h.push('followUp');
  assert.equal(leadWakeStartPendingRef.current, true);
  assert.equal(h.custom.length, 0);

  h.start(); // A user-started run wins before the queued wake itself starts.

  assert.equal(leadWakeStartPendingRef.current, false);
  assert.deepEqual(h.custom[0].options, {deliverAs: 'steer', triggerTurn: true});
  assert.equal(h.users.length, 1, 'the pending reservation did not create a second wake');
  h.emit('session_shutdown');
});

test('/goal stop leaves a running child untouched and preserves its eventual report wake', async () => {
  const h = harness(true);
  const childStops: string[] = [];
  const child = {
    client: {
      abort: async () => { childStops.push('abort'); },
      stop: async () => { childStops.push('stop'); },
    },
    running: true,
    streaming: true,
        threadBound: false,
    reportLog: [],
  };
  h.registry.set('child-1', child);
  await h.commands.get('goal').handler('ship', h.ctx);

  await h.commands.get('goal').handler('stop', h.ctx);
  assert.deepEqual(childStops, [], 'goal stopping never aborts or stops a running child');
  assert.equal(h.registry.get('child-1'), child, 'the live child remains registered unchanged');

  child.running = false;
  child.streaming = false;
  h.push('followUp', 'child finished');
  assert.equal(heldPushQueue.length, 1, 'the eventual child report remains queued for delivery');
  assert.equal(leadWakeStartPendingRef.current, true, 'child-report wake ownership is independent of goal state');
  assert.equal(h.timers.size, 1, 'child-report wake recovery is pending');

  h.start();
  assert.equal(h.custom.length, 1);
  assert.equal((h.custom[0].message as any).details.items[0].details.report, 'child finished');
  assert.equal(heldPushQueue.length, 0);
  assert.equal(h.users.length, 2, 'only the goal announcement and child-report wake were submitted');
  h.settle();
  assert.equal(h.timers.size, 0, 'stopped goal does not schedule a reminder after child delivery');
  h.goal!.resetCompactionStateForShutdown(); h.emit('session_shutdown');
});
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
test('ordinary busy steer with no older hold stays an immediate individual custom message', () => {
  const h = harness(); h.busy(); h.push('steer');
  assert.equal(h.custom.length, 1);
  assert.equal(h.custom[0].message.customType, 'ws-agent-report');
  assert.notEqual(h.custom[0].message.customType, PUSH_BATCH_CUSTOM_TYPE);
  assert.deepEqual(h.custom[0].options, {deliverAs: 'steer', triggerTurn: true});
  assert.equal(heldPushQueue.length, 0);
  h.settle(); h.emit('session_shutdown');
});

test('approval and question steer with no older hold keep their individual custom types', () => {
  const h = harness(); h.busy();
  const approvalRecord: any = {agentId: 'approval-agent', workGeneration: 1, pendingApproval: {cmdId: 'cmd-1'}, reportLog: []};
  const questionRecord: any = {agentId: 'question-agent', workGeneration: 1, reportLog: [{kind: 'question', at: 1}]};
  h.registry.set(approvalRecord.agentId, approvalRecord);
  h.registry.set(questionRecord.agentId, questionRecord);
  pushToLead(h.pi, h.registry, approvalRecord, 'ws-agent-approval', {cmd_id: 'cmd-1', request: 'approve'}, 'steer');
  pushToLead(h.pi, h.registry, questionRecord, 'ws-agent-question', {question: 'continue?'}, 'steer');
  assert.deepEqual(h.custom.map((entry) => entry.message.customType), ['ws-agent-approval', 'ws-agent-question']);
  assert.ok(h.custom.every((entry) => entry.message.customType !== PUSH_BATCH_CUSTOM_TYPE));
  assert.equal(heldPushQueue.length, 0);
  h.settle(); h.emit('session_shutdown');
});

test('an actionable steer joins an older held followUp instead of overtaking FIFO', () => {
  const h = harness(); h.busy();
  const approvalRecord: any = {agentId: 'approval-agent', workGeneration: 1, pendingApproval: {cmdId: 'cmd-1'}, reportLog: []};
  h.registry.set(approvalRecord.agentId, approvalRecord);
  h.push('followUp', 'progress first');
  pushToLead(h.pi, h.registry, approvalRecord, 'ws-agent-approval', {cmd_id: 'cmd-1', request: 'approve second'}, 'steer');
  assert.equal(h.custom.length, 0, 'the later steer cannot overtake the held FIFO prefix');
  h.end();
  assert.equal(h.custom.length, 1);
  assert.equal(h.custom[0].message.customType, PUSH_BATCH_CUSTOM_TYPE);
  assert.deepEqual(h.custom[0].message.details.items.map((item: any) => item.customType), ['ws-agent-report', 'ws-agent-approval']);
  assert.deepEqual(h.custom[0].message.details.items.map((item: any) => item.state), ['informational', 'actionable']);
  assert.deepEqual(h.custom[0].options, {deliverAs: 'followUp', triggerTurn: true});
  assert.equal(h.users.length, 0, 'the accepted boundary batch needs no counted fallback wake');
  h.settle(); assert.equal(h.users.length, 0);
  h.emit('session_shutdown');
});

for (const steeringMode of ['one-at-a-time', 'all'] as const) test(`agent_end batch preserves FIFO and XML escaping in ${steeringMode} mode`, () => {
  const h = harness(false, steeringMode);
  h.busy();
  h.push('followUp', 'first <tag> & "quote"');
  sendToLead(h.pi, {customType: 'ws-thread-summary', content: "owner's > summary", display: true, details: {source: 'owner'}}, 'followUp');
  h.push('followUp', 'third');

  h.end();

  assert.equal(h.custom.length, 1);
  const batch = h.custom[0];
  assert.equal(batch.message.customType, PUSH_BATCH_CUSTOM_TYPE);
  assert.deepEqual(batch.options, {deliverAs: 'followUp', triggerTurn: true});
  assert.deepEqual(batch.message.details.items.map((item: any) => item.customType), ['ws-agent-report', 'ws-thread-summary', 'ws-agent-report']);
  const content = batch.message.content as string;
  assert.ok(content.indexOf('first') < content.indexOf("owner&apos;s") && content.indexOf("owner&apos;s") < content.indexOf('third'));
  assert.match(content, /first &lt;tag&gt; &amp; &quot;quote&quot;/);
  assert.ok(content.indexOf('<action-summary>') > content.lastIndexOf('</message>'), 'action summary trails the ordered body');
  assert.equal(h.modelTimeline.filter((entry: string) => entry === 'model-response').length, 1, 'one snapshot creates one continuation');
  h.settle(); h.emit('session_shutdown');
});

test('snapshot-time validation marks stale approval and question controls superseded', () => {
  const h = harness(); h.busy();
  const approvalRecord: any = {agentId: 'approval-agent', workGeneration: 1, pendingApproval: {cmdId: 'cmd-1'}, reportLog: []};
  const questionEntry = {kind: 'question', at: 1};
  const questionRecord: any = {agentId: 'question-agent', workGeneration: 2, reportLog: [questionEntry]};
  h.registry.set(approvalRecord.agentId, approvalRecord);
  h.registry.set(questionRecord.agentId, questionRecord);
  pushToLead(h.pi, h.registry, approvalRecord, 'ws-agent-approval', {cmd_id: 'cmd-1', request: 'approve'}, 'followUp');
  pushToLead(h.pi, h.registry, questionRecord, 'ws-agent-question', {question: 'continue?'}, 'followUp');
  approvalRecord.pendingApproval = {cmdId: 'cmd-2'};
  questionRecord.workGeneration = 3;

  h.end();

  const batch = h.custom[0].message;
  assert.deepEqual(batch.details.items.map((item: any) => item.state), ['superseded', 'superseded']);
  assert.match(batch.content, /type="ws-agent-approval" state="superseded" agent-id="approval-agent" command-id="cmd-1"/);
  assert.match(batch.content, /type="ws-agent-question" state="superseded" agent-id="question-agent"/);
  assert.match(batch.content, /<action-summary>\n    none\n  <\/action-summary>/);
  h.settle(); h.emit('session_shutdown');
});

test('one boundary batch carries every push family plus the owner summary in strict arrival order', () => {
  const h = harness(); h.busy(); leadCompactingRef.current = true;
  const approvalRecord: any = {agentId: 'approval-agent', workGeneration: 1, pendingApproval: {cmdId: 'cmd-1'}, reportLog: []};
  const questionEntry = {kind: 'question', at: 1};
  const questionRecord: any = {agentId: 'question-agent', workGeneration: 1, reportLog: [questionEntry]};
  h.registry.set(approvalRecord.agentId, approvalRecord);
  h.registry.set(questionRecord.agentId, questionRecord);
  pushToLead(h.pi, h.registry, undefined, 'ws-agent-report', {report: 'progress'}, 'followUp');
  pushToLead(h.pi, h.registry, approvalRecord, 'ws-agent-approval', {cmd_id: 'cmd-1', request: 'approve'}, 'steer');
  pushToLead(h.pi, h.registry, questionRecord, 'ws-agent-question', {question: 'continue?'}, 'steer');
  pushToLead(h.pi, h.registry, undefined, 'ws-agent-advisory', {advisory: 'stalled'}, 'followUp');
  pushToLead(h.pi, h.registry, undefined, 'ws-agent-report', {report: 'done'}, 'followUp');
  pushToLead(h.pi, h.registry, undefined, 'ws-agent-settled', {reason: 'idle'}, 'followUp');
  pushToLead(h.pi, h.registry, undefined, 'ws-agent-orphaned', {count: 1}, 'followUp');
  sendToLead(h.pi, {customType: 'ws-thread-summary', content: 'owner summary', display: true, details: {threadId: 'q1'}}, 'followUp');
  leadCompactingRef.current = false;

  h.end();

  assert.deepEqual(h.custom[0].message.details.items.map((item: any) => item.customType), [
    'ws-agent-report', 'ws-agent-approval', 'ws-agent-question', 'ws-agent-advisory',
    'ws-agent-report', 'ws-agent-settled', 'ws-agent-orphaned', 'ws-thread-summary',
  ]);
  assert.equal(h.custom.length, 1);
  h.settle(); h.emit('session_shutdown');
});

test('snapshot-time validation keeps current controls actionable with a trailing identity summary', () => {
  const h = harness(); h.busy();
  const approvalRecord: any = {agentId: 'approval-agent', workGeneration: 1, pendingApproval: {cmdId: 'cmd-1'}, reportLog: []};
  const questionEntry = {kind: 'question', at: 1};
  const questionRecord: any = {agentId: 'question-agent', workGeneration: 2, reportLog: [questionEntry]};
  h.registry.set(approvalRecord.agentId, approvalRecord);
  h.registry.set(questionRecord.agentId, questionRecord);
  pushToLead(h.pi, h.registry, approvalRecord, 'ws-agent-approval', {cmd_id: 'cmd-1', request: 'approve'}, 'followUp');
  pushToLead(h.pi, h.registry, questionRecord, 'ws-agent-question', {question: 'continue?'}, 'followUp');

  h.end();

  const batch = h.custom[0].message;
  assert.deepEqual(batch.details.items.map((item: any) => item.state), ['actionable', 'actionable']);
  assert.match(batch.content, /<action type="approval" agent-id="approval-agent" command-id="cmd-1">/);
  assert.match(batch.content, /<action type="question" agent-id="question-agent">/);
  h.settle(); h.emit('session_shutdown');
});

test('rejected boundary submission retains the exact snapshot and retries once on the fallback wake', () => {
  const h = harness(); h.busy();
  h.push('followUp', 'first'); h.push('followUp', 'second');
  h.failCustom(); h.end();
  assert.equal(h.custom.length, 0);
  assert.equal(heldPushQueue.length, 2, 'synchronous rejection removes nothing');
  h.allowCustom(); h.settle();
  assert.equal(h.users.length, 1, 'settlement re-arms the existing counted wake');
  h.start();
  assert.equal(h.custom.length, 1);
  assert.deepEqual(h.custom[0].message.details.items.map((item: any) => item.details.report), ['first', 'second']);
  assert.equal(heldPushQueue.length, 0);
  h.emit('session_shutdown');
});

test('terminal obligations move only on accepted batch delivery and restore on rejection', () => {
  const h = harness(); h.busy();
  const calls: string[] = [];
  const terminal: any = {
    afterEnqueue: () => calls.push('after'),
  };
  pushToLead(h.pi, h.registry, undefined, 'ws-agent-report', {report: 'done'}, 'followUp', terminal);
  assert.equal(terminal.state, 'held');
  h.failCustom(); h.end();
  assert.deepEqual(calls, []);
  assert.equal(terminal.state, 'held');
  h.allowCustom(); h.settle(); h.start();
  assert.deepEqual(calls, ['after']);
  assert.equal(terminal.state, 'enqueued');
  h.emit('session_shutdown');
});

test('late arrivals after a boundary snapshot remain queued for the settled fallback', () => {
  const h = harness(); h.busy(); h.push('followUp', 'boundary'); h.end();
  h.push('followUp', 'late');
  assert.equal(heldPushQueue.length, 1);
  h.settle(); assert.equal(h.users.length, 1);
  h.start();
  assert.equal(h.custom.length, 2);
  assert.equal(h.custom[1].message.details.items[0].details.report, 'late');
  h.emit('session_shutdown');
});

test('batch content stays uncapped for an oversized report', () => {
  const h = harness(); h.busy();
  const large = '界<&'.repeat(40_000);
  h.push('followUp', large); h.end();
  const content = h.custom[0].message.content as string;
  assert.ok(content.length > large.length, 'escaping grows rather than truncates the payload');
  assert.match(content, /界&lt;&amp;界&lt;&amp;/);
  assert.ok(content.endsWith(`</${PUSH_BATCH_CUSTOM_TYPE}>`));
  h.settle(); h.emit('session_shutdown');
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
    assert.equal(h.users.length, 1, `${role} owns persistent child pushes`);
    assert.equal(h.timers.size, 1);
    h.tick(); assert.equal(h.users.length, 2); h.start();
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
