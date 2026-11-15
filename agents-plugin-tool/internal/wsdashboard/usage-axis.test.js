const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const { execFileSync } = require('node:child_process');
const window = {};
vm.runInNewContext(fs.readFileSync(__dirname + '/assets/usage.js', 'utf8'), { window });
const axis = window.WsUsage.weeklyAxis;

test('weekly calendar grid follows local midnights across both DST transitions', () => {
  // A separate process makes the timezone explicit without mutating the runner.
  const script = `
    const assert = require('node:assert/strict');
    const fs = require('node:fs'), vm = require('node:vm'), window = {};
    vm.runInNewContext(fs.readFileSync(process.argv[1], 'utf8'), { window });
    for (const month of [2, 10]) {
      const reset = new Date(2026, month, month === 2 ? 12 : 5, 17, 30).getTime();
      const start = reset - 7 * 86400000;
      const result = window.WsUsage.weeklyAxis(start, reset, 'en', () => 20);
      assert.equal(result.grid.length, 7);
      const times = result.grid.map(x => Math.round(start + (x - 24) / 246 * (reset - start)));
      times.forEach(at => {
        const date = new Date(at);
        assert.equal(date.getHours(), 0); assert.equal(date.getMinutes(), 0);
        assert.ok(at > start && at < reset);
      });
      const gaps = times.slice(1).map((at, i) => (at - times[i]) / 3600000);
      assert.ok(gaps.includes(month === 2 ? 23 : 25));
      assert.ok(result.labels.every(label => {
        const at = Math.round(start + (label.x - 24) / 246 * (reset - start));
        return label.text === new Intl.DateTimeFormat('en', { weekday: 'short' }).format(at);
      }));
    }
  `;
  execFileSync(process.execPath, ['-e', script, __dirname + '/assets/usage.js'], { env: { ...process.env, TZ: 'America/New_York' } });
});

test('midnight-aligned windows label each full day without duplicate edge labels', () => {
  const reset = new Date(2026, 10, 15).getTime(), start = reset - 7 * 86400000;
  const result = axis(start, reset, 'en', () => 20);
  assert.equal(result.grid.length, 6);
  assert.equal(result.labels.length, 7);
  assert.ok(result.grid.every(x => x > 24 && x < 270));
});

test('day centers stay natural while clipped and colliding labels are omitted', () => {
  const reset = new Date(2026, 10, 15, 17, 30).getTime(), start = reset - 7 * 86400000;
  for (const width of [18, 30, 60, 300]) {
    const result = axis(start, reset, 'ko', () => width);
    let right = -Infinity;
    for (const label of result.labels) {
      const left = label.x - width / 2;
      assert.ok(left >= 24 && label.x + width / 2 <= 270);
      assert.ok(left >= right + 3);
      right = label.x + width / 2;
      const at = Math.round(start + (label.x - 24) / 246 * (reset - start));
      assert.equal(new Date(at).getHours(), 12);
      assert.equal(label.text, new Intl.DateTimeFormat('ko', { weekday: 'short' }).format(at));
    }
    // The first center precedes the window; the final label's right half clips.
    if (width === 18) assert.equal(result.labels.length, 6);
    if (width === 300) assert.equal(result.labels.length, 0);
  }
  const css = fs.readFileSync(__dirname + '/assets/usage.css', 'utf8');
  assert.match(css, /\.usage-day-grid \{ stroke: var\(--wb-line\); stroke-width: \.5; opacity: \.35; \}/);
});
