// Compact horizontal icon org-chart HUD browser acceptance against roster-hud-v2.
// env: HUD_BASE, HUD_MOCKUP (path to roster-hud-v2.html), HUD_SHOTS (optional dir).
const assert = require('node:assert/strict');
const { chromium } = require(process.env.WS_PLAYWRIGHT_MODULE);
const base = process.env.HUD_BASE;
const switcher = process.env.HUD_SWITCH;
const mockup = process.env.HUD_MOCKUP;
const shots = process.env.HUD_SHOTS;

async function openSession(b, scenario, { width = 1280, height = 840, scheme = 'dark' } = {}) {
  if (scenario) await fetch(`${switcher}?s=${scenario}`);
  const ctx = await b.newContext({ viewport: { width, height }, colorScheme: scheme, locale: 'ko-KR' });
  await ctx.addCookies([{ name: 'ws_dashboard_lang', value: 'ko', url: base }]);
  const tab = await ctx.newPage();
  const errors = [];
  tab.on('pageerror', e => errors.push(e.message));
  await tab.goto(`${base}/s/livesession`);
  const frame = await (await tab.locator('iframe.sframe.on').elementHandle()).contentFrame();
  await frame.locator('#agisland[data-mode="compact"]:not([hidden])').waitFor();
  await frame.waitForTimeout(350);
  return { ctx, tab, frame, errors };
}

(async () => {
  const b = await chromium.launch();
  try {
    // 1. Dark & Light themes + 12-agent 3-level geometry & computed-style checks
    for (const scheme of ['dark', 'light']) {
      const { ctx, tab, frame, errors } = await openSession(b, 'heavy', { width: 1280, scheme });
      const geom = await frame.evaluate(() => {
        const n = document.querySelector('#agcompact .hud-node');
        const e = n.querySelector('.hud-emblem');
        const toolRing = document.querySelector('#agcompact .hud-node[data-state="tool"] .hud-ring');
        const perm = document.querySelector('#agcompact .hud-badge.perm');
        const fail = document.querySelector('#agcompact .hud-badge.fail');
        const roots = document.querySelector('#agcompact .hud-roots');
        const kids = document.querySelector('#agcompact .tree-kids');
        const path = document.querySelector('#agcompact .hud-lines path');
        const nr = n.getBoundingClientRect(), er = e.getBoundingClientRect();
        const pr = perm.getBoundingClientRect(), fr = fail.getBoundingClientRect();
        const ns = getComputedStyle(n), rs = getComputedStyle(toolRing);
        const rts = getComputedStyle(roots), ks = getComputedStyle(kids), ps = getComputedStyle(path);
        const idleNode = document.querySelector('#agcompact .hud-node[data-state="idle"]');
        return {
          nodeW: Math.round(nr.width), nodeH: Math.round(nr.height), nodeRadius: ns.borderRadius,
          emblemW: Math.round(er.width), emblemH: Math.round(er.height),
          ringWidth: rs.paddingTop,
          permW: Math.round(pr.width), permH: Math.round(pr.height), permText: perm.textContent,
          failW: Math.round(fr.width), failH: Math.round(fr.height), failText: fail.textContent,
          rootsGap: rts.gap, kidsGap: ks.gap, kidsTop: ks.paddingTop,
          strokeWidth: ps.strokeWidth, pathD: path.getAttribute('d'),
          idleOpacity: Number(getComputedStyle(idleNode).opacity).toFixed(2),
          summary: document.querySelector('#agcompact .hud-summary-text').textContent.trim(),
          nodeCount: document.querySelectorAll('#agcompact .hud-node').length,
          harnesses: [...new Set([...document.querySelectorAll('#agcompact .hud-emblem')].map(x => x.getAttribute('data-h')))].sort(),
          states: [...new Set([...document.querySelectorAll('#agcompact .hud-node')].map(x => x.getAttribute('data-state')))].sort()
        };
      });
      assert.equal(geom.nodeW, 24); assert.equal(geom.nodeH, 24);
      assert.equal(geom.emblemW, 18); assert.equal(geom.emblemH, 18);
      assert.equal(geom.ringWidth, '1.5px');
      assert.equal(geom.permW, 11); assert.equal(geom.permH, 11); assert.equal(geom.permText, '!');
      assert.equal(geom.failW, 11); assert.equal(geom.failH, 11); assert.equal(geom.failText, '×');
      assert.equal(geom.rootsGap, '10px'); assert.equal(geom.kidsGap, '6px'); assert.equal(geom.kidsTop, '10px');
      assert.equal(geom.strokeWidth, '1.15px');
      assert(geom.pathD.includes('M') && geom.pathD.includes('V') && geom.pathD.includes('H'), 'orthogonal SVG connector');
      assert.equal(geom.idleOpacity, '0.42');
      assert.equal(geom.summary, '완료 2 · 피어 1');
      assert.equal(geom.nodeCount, 12);
      assert.deepEqual(geom.harnesses, ['agy', 'claude', 'codex', 'pi']);
      assert.deepEqual(geom.states, ['failed', 'holding', 'idle', 'permission', 'thinking', 'tool']);
      if (shots) await tab.screenshot({ path: `${shots}/compact-${scheme}.png` });

      if (scheme === 'dark') {
        // Tooltip clamp + own-row task from #r-tree
        await frame.locator('#agcompact .hud-node[data-id="h1"]').hover();
        await frame.locator('#hud-tooltip:not([hidden])').waitFor();
        const tt = await frame.evaluate(() => {
          const tip = document.getElementById('hud-tooltip').getBoundingClientRect();
          const convo = document.getElementById('convo').getBoundingClientRect();
          return {
            left: tip.left, right: tip.right, top: tip.top, bottom: tip.bottom,
            convoLeft: convo.left, convoRight: convo.right, convoTop: convo.top, convoBottom: convo.bottom,
            task: document.querySelector('#hud-tooltip .tt-task')?.textContent || '',
            hint: document.querySelector('#hud-tooltip .tt-hint')?.textContent || '',
            chain: document.querySelector('#hud-tooltip .tt-chain')?.textContent || ''
          };
        });
        assert(tt.left >= tt.convoLeft - 1 && tt.right <= tt.convoRight + 1, 'tooltip horizontally clamped inside #convo');
        assert(tt.top >= tt.convoTop - 1 && tt.bottom <= tt.convoBottom + 1, 'tooltip vertically clamped inside #convo');
        assert.equal(tt.task, 'Waiting on ast-scanner', 'own-row task read from #r-tree');
        assert.match(tt.hint, /트리 상세 열기/);
        if (shots) await tab.screenshot({ path: `${shots}/tooltip-dark.png` });

        // Unlinked node click -> opens #agpop & focuses details summary
        await frame.click('#agcompact .hud-node[data-id="h3"]');
        await frame.waitForTimeout(250);
        const popFocus = await frame.evaluate(() => ({
          mode: document.getElementById('agisland').getAttribute('data-mode'),
          open: document.querySelector('#agpop details[data-key="agent-h3"]')?.open,
          activeKey: document.activeElement?.closest('details')?.getAttribute('data-key')
        }));
        assert.deepEqual(popFocus, { mode: 'full', open: true, activeKey: 'agent-h3' });
        if (shots) await tab.screenshot({ path: `${shots}/full-dark.png` });
        await tab.keyboard.press('Escape');
        await frame.waitForTimeout(250);

        // Linked node click -> navigates to /s/<replyId>
        await frame.locator('#agcompact .hud-node[data-id="h2"]').hover();
        const linkedHint = await frame.textContent('#hud-tooltip .tt-hint');
        assert.match(linkedHint, /\/s\/boardsession/);
        await frame.click('#agcompact .hud-node[data-id="h2"]');
        await tab.waitForFunction(() => location.pathname.endsWith('/s/boardsession'));
      }
      assert.deepEqual(errors, [], `no pageerrors in ${scheme}`);
      await ctx.close();
    }

    // 2. Depth-4 cap +2 overflow chip & narrow 420px spacing
    {
      const { ctx, tab, frame, errors } = await openSession(b, 'depth', { width: 420, scheme: 'dark' });
      const narrow = await frame.evaluate(() => {
        const roots = document.querySelector('#agcompact .hud-roots');
        const kids = document.querySelector('#agcompact .tree-kids');
        const ov = document.querySelector('#agcompact .hud-overflow');
        const orect = ov.getBoundingClientRect();
        return {
          rootsGap: getComputedStyle(roots).gap,
          kidsGap: getComputedStyle(kids).gap,
          kidsTop: getComputedStyle(kids).paddingTop,
          overflowText: ov.textContent,
          overflowH: Math.round(orect.height),
          nodeCount: document.querySelectorAll('#agcompact .hud-node').length
        };
      });
      assert.deepEqual(narrow, { rootsGap: '7px', kidsGap: '5px', kidsTop: '9px', overflowText: '+2', overflowH: 12, nodeCount: 4 });
      if (shots) await tab.screenshot({ path: `${shots}/narrow-depth4-dark.png` });
      assert.deepEqual(errors, []);
      await ctx.close();
    }

    // 3. Wide fan-out (14 direct) horizontal scroll inside #agisland
    {
      const { ctx, tab, frame, errors } = await openSession(b, 'fanout', { width: 420, scheme: 'dark' });
      const fan = await frame.evaluate(() => {
        const island = document.getElementById('agisland').getBoundingClientRect();
        const convo = document.getElementById('convo').getBoundingClientRect();
        const sc = document.querySelector('#agcompact .hud-tree-scroll');
        return {
          nodes: document.querySelectorAll('#agcompact .hud-node').length,
          fitsConvo: island.width <= convo.width - 15,
          scrolls: sc.scrollWidth > sc.clientWidth
        };
      });
      assert.deepEqual(fan, { nodes: 14, fitsConvo: true, scrolls: true });
      if (shots) await tab.screenshot({ path: `${shots}/fanout-dark.png` });
      assert.deepEqual(errors, []);
      await ctx.close();
    }

    // 4. Idle pill when all agents are asleep/closed
    {
      await fetch(`${switcher}?s=idle`);
      const ctx = await b.newContext({ viewport: { width: 1280, height: 840 }, colorScheme: 'dark', locale: 'ko-KR' });
      await ctx.addCookies([{ name: 'ws_dashboard_lang', value: 'ko', url: base }]);
      const tab = await ctx.newPage();
      await tab.goto(`${base}/s/livesession`);
      const frame = await (await tab.locator('iframe.sframe.on').elementHandle()).contentFrame();
      await frame.locator('#agisland[data-mode="idle"]:not([hidden])').waitFor();
      await frame.waitForTimeout(250);
      assert.equal(await frame.textContent('#agidle .agidle-label'), '완료 2');
      if (shots) await tab.screenshot({ path: `${shots}/idle-dark.png` });
      await ctx.close();
    }

    // 5. Optional side-by-side mockup capture
    if (shots && mockup) {
      const ctx = await b.newContext({ viewport: { width: 1280, height: 840 }, colorScheme: 'dark' });
      const tab = await ctx.newPage();
      await tab.goto(`file://${mockup}?theme=dark&scenario=heavy&hover=s-101`);
      await tab.waitForTimeout(300);
      await tab.screenshot({ path: `${shots}/mockup-v2-dark.png` });
      await ctx.close();
    }
    console.log('hud browser acceptance ok');
  } finally {
    await b.close();
  }
})().catch(e => { console.error(e); process.exit(1); });
