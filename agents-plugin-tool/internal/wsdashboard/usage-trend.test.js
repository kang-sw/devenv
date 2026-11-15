const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const window = {};
vm.runInNewContext(fs.readFileSync(__dirname + '/assets/usage.js', 'utf8'), { window });
const start = Date.UTC(2026, 10, 15), duration = 5 * 3600000, reset = start + duration;
const sample = (elapsed, percent, source = 'Claude push', resetsAt = reset) => ({ harness: 'claude', kind: 'five_hour', source, resetsAt, asOf: start + elapsed * duration, percent });
function trend(samples, current = samples.at(-1), projection) {
  const entry = { harness: 'claude', source: current.source, asOf: current.asOf };
  const w = { kind: current.kind, percent: current.percent, resetsAt: current.resetsAt, asOf: current.asOf };
  return window.WsUsage.trend(entry, w, projection || { schema_version: 1, status: samples.length ? 'ok' : 'empty', samples });
}

test('trend splits baseline crossings exactly, with proportional deltas and no fill-to-zero', () => {
  const a = sample(.2, 10), b = sample(.4, 60), result = trend([a, b]);
  assert.equal(result.points.length, 2);
  assert.equal(result.segments.length, 2);
  const [under, over] = result.segments;
  assert.equal(under.above, false); assert.equal(over.above, true);
  assert.equal(under.a.delta, -10); assert.equal(over.b.delta, 20);
  assert.equal(under.b.delta, 0); assert.equal(over.a.delta, 0);
  assert.ok(Math.abs(under.b.asOf - (start + duration * (4 / 15))) < .001);
  assert.equal(under.b.percent, over.a.percent);
  assert.equal(under.estimated, true); assert.equal(over.estimated, true);
  // The synthetic crossing is visual geometry only, not an observed point.
  assert.ok(!result.points.some(p => p.asOf === under.b.asOf));
  assert.equal(JSON.stringify([a, b]), JSON.stringify([sample(.2, 10), sample(.4, 60)]));
  const reverse = trend([sample(.2, 40), sample(.4, 40)]);
  assert.equal(reverse.segments.length, 1); assert.equal(reverse.segments[0].above, true);
  const descendingDelta = trend([sample(.2, 30), sample(.4, 35)]);
  assert.equal(descendingDelta.segments.length, 2);
  assert.equal(descendingDelta.segments[0].above, true); assert.equal(descendingDelta.segments[1].above, false);
});

test('gap detection is strictly over ten minutes and applies to both crossing halves', () => {
  const a = sample(.1, 10), b = { ...sample(.2, 30), asOf: a.asOf + 600000 };
  assert.equal(trend([a, b]).segments[0].estimated, false);
  b.asOf++;
  assert.ok(trend([a, b]).segments.every(s => s.estimated));
});

test('no origin fabrication, no extrapolation, fallback current point with absent or unavailable history', () => {
  const a = sample(.2, 10), b = sample(.4, 60);
  const normal = trend([a, b]);
  assert.equal(normal.segments[0].a.asOf, a.asOf);
  assert.equal(normal.segments.at(-1).b.asOf, b.asOf);
  assert.equal(normal.points[0].x, 24 + .2 * 246);
  assert.equal(normal.points.at(-1).current, true);
  for (const projection of [{}, { schema_version: 1, status: 'unavailable', samples: [a] }, { schema_version: 2, status: 'ok', samples: [a] }, { schema_version: 1, status: 'empty', samples: [] }]) {
    const result = trend([], b, projection);
    assert.equal(result.points.length, 1); assert.equal(result.segments.length, 0);
    assert.equal(result.points[0].asOf, b.asOf); assert.equal(result.knownReset, true);
  }
  const missingAt = { ...b, asOf: 0 };
  assert.equal(trend([], missingAt).points.length, 0);
});

test('reset and source boundaries remain barriers even if a prior identity reappears', () => {
  const a = sample(.1, 10), b = sample(.2, 20, 'via Pi'), c = sample(.3, 30);
  assert.equal(trend([a, b, c]).segments.length, 0);
  const otherReset = sample(.2, 20, 'Claude push', reset + 1);
  assert.equal(trend([a, otherReset, c]).segments.length, 0);
  const unknown = sample(.2, 20, 'Claude push', 0);
  assert.equal(trend([a, unknown, c]).segments.length, 0);
  const result = trend([sample(.1, 10, 'Claude push', 0), unknown]);
  assert.equal(result.knownReset, false); assert.equal(result.segments.length, 0);
  assert.equal(result.points.length, 2);
  assert.ok(result.points.every(p => p.delta === null));
});

test('GPT decreases are retained but the decreasing pair never becomes cumulative growth', () => {
  const samples = [sample(.1, 20), sample(.2, 40), sample(.3, 10), sample(.4, 15)].map(s => ({ ...s, harness: 'codex', kind: 'seven_day', source: 'via Pi' }));
  const latest = samples.at(-1);
  const result = window.WsUsage.trend({ harness: 'codex', source: 'via Pi' }, { ...latest }, { schema_version: 1, status: 'ok', samples });
  assert.equal(result.points.length, 4);
  assert.ok(!result.segments.some(s => s.a.asOf === samples[1].asOf && s.b.asOf === samples[2].asOf));
  assert.ok(result.segments.some(s => s.b.asOf === latest.asOf));
});

test('history input is sorted without mutation, deduplicated and filtered to the current window', () => {
  const a = sample(.1, 10), b = sample(.2, 20), other = { ...sample(.15, 99), harness: 'codex' };
  const samples = [b, other, a, { ...a, percent: 11 }];
  const original = JSON.stringify(samples), result = trend(samples, b);
  assert.equal(JSON.stringify(samples), original);
  assert.equal(result.points.length, 2); assert.equal(result.points[0].percent, 11);
  assert.equal(result.points.at(-1).percent, 20);
  assert.equal(trend([sample(-.1, 0), a, b], b).points.length, 2);
  assert.equal(trend([a, b, sample(.3, 30)], b).points.length, 2);
});
