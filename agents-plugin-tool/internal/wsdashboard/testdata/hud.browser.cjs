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
  const ctx = await b.newContext({ viewport: { width, height }, deviceScaleFactor: 2, colorScheme: scheme, locale: 'ko-KR' });
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

async function chooseTheme(tab, frame, theme) {
  const openedRail = !await tab.locator('#gear').isVisible();
  if (openedRail) await tab.locator('#railbtn').click();
  await tab.locator('#gear').click();
  await tab.locator(`#gmenu [data-theme-set="${theme}"]`).click();
  await tab.locator(`html[data-theme="${theme}"]`).waitFor();
  await frame.locator(`html[data-theme="${theme}"]`).waitFor();
  await tab.keyboard.press('Escape');
  if (openedRail) {
    await tab.keyboard.press('Escape');
    await tab.locator('html:not(.rail-open)').waitFor();
  }
}

async function checkKeyedRebuild(frame) {
  const result = await frame.evaluate(async () => {
    const tree = document.getElementById('r-tree');
    const carrier = tree.querySelector('.ag-hud-data');
    const compact = document.getElementById('agcompact');
    const idle = document.getElementById('agidle');
    const pop = document.getElementById('agpop');
    const original = carrier.textContent;
    const source = JSON.parse(original);
    const node = id => compact.querySelector(`.hud-node[data-id="${id}"]`);
    const project = data => {
      carrier.textContent = JSON.stringify(data);
      window.wsHud.project(tree, compact, idle, pop, () => {});
    };
    const h3 = node('h3'), h4 = node('h4'), h5 = node('h5');
    const ring = h3.querySelector('.hud-ring');
    const animation = ring.getAnimations()[0];
    const startTime = animation?.currentTime;
    const delay = id => parseFloat(getComputedStyle(node(id).querySelector('.hud-ring')).animationDelay) * 1000;
    const stagger = [delay('h3'), delay('h5')];
    await new Promise(resolve => setTimeout(resolve, 65));
    project(source);
    const same = node('h3') === h3 && node('h4') === h4 && node('h5') === h5 &&
      node('h3').querySelector('.hud-ring') === ring && ring.getAnimations()[0] === animation;
    const advanced = animation?.currentTime - startTime;
    const changed = structuredClone(source);
    const h2 = changed.roots.find(r => r.id === 'h1').children.find(c => c.id === 'h2');
    h2.children.reverse();
    h2.children.find(c => c.id === 'h3').state = 'thinking';
    h2.children.find(c => c.id === 'h3').label = 'sym-extract-updated';
    project(changed);
    const reordered = [...node('h2').closest('.tree-node').querySelector('.tree-kids').children]
      .map(w => w.querySelector('.hud-node').getAttribute('data-id'));
    const updated = node('h3') === h3 && node('h3').querySelector('.hud-ring') === ring &&
      node('h3').getAttribute('data-state') === 'thinking' &&
      node('h3').getAttribute('aria-label').includes('sym-extract-updated') &&
      node('h4') === h4 && node('h5') === h5;
    const thinkingDelay = delay('h3');
    const oldH4Animation = h4.querySelector('.hud-ring').getAnimations()[0];
    const oldH4Time = oldH4Animation?.currentTime;
    const oldClock = document.timeline.currentTime;
    h2.children = h2.children.filter(c => c.id !== 'h4');
    project(changed);
    const removed = !node('h4') && node('h3') === h3;
    project(source);
    await new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve)));
    const newH4Time = node('h4').querySelector('.hud-ring').getAnimations()[0]?.currentTime;
    const elapsed = document.timeline.currentTime - oldClock;
    const period = 1150;
    const drift = Number.isFinite(oldH4Time) && Number.isFinite(newH4Time)
      ? Math.abs((((newH4Time - oldH4Time - elapsed) % period) + period * 1.5) % period - period / 2)
      : null;
    const remounted = node('h4') !== h4 && node('h3') === h3 && node('h5') === h5;
    carrier.textContent = original;
    return { same, advanced, stagger, reordered, updated, thinkingDelay, removed, remounted, drift,
      animationName: getComputedStyle(ring).animationName };
  });
  assert.equal(result.same, true, 'stable keys preserve buttons, rings and running CSS animation');
  assert(result.advanced > 30 && result.advanced < 300, `animation advances across unchanged rebuild: ${result.advanced}`);
  assert(result.stagger.every(x => Number.isFinite(x) && x <= 0 && x > -900), `tool delays are bounded: ${result.stagger}`);
  assert.notEqual(result.stagger[0], result.stagger[1], 'different agent keys stagger equal-period rings');
  assert.deepEqual(result.reordered, ['h4', 'h3'], 'sibling order follows new source order');
  assert.equal(result.updated, true, 'state and label update in place while siblings keep identity');
  assert(result.thinkingDelay <= 0 && result.thinkingDelay > -1150, 'state change restamps within thinking period');
  assert.equal(result.removed, true, 'removed keyed node disappears');
  assert.equal(result.remounted, true, 'reappearing node is new while surviving nodes stay mounted');
  assert(Number.isFinite(result.drift) && result.drift < 80, `remounted ring resumes phase within 80ms: ${result.drift}`);
  assert.match(result.animationName, /hud-spin-tool/, 'restored state uses tool animation');
}

(async () => {
  const b = await chromium.launch();
  try {
    // 1. Dark & Light themes + 12-agent 3-level geometry & computed-style checks
    for (const scheme of ['dark', 'light']) {
      const { ctx, tab, frame, errors } = await openSession(b, 'heavy', { width: 1280, scheme: scheme === 'dark' ? 'light' : 'dark' });
      await chooseTheme(tab, frame, scheme);
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
      assert.equal(geom.nodeCount, 12);
      assert.deepEqual(geom.harnesses, ['agy', 'claude', 'codex', 'pi']);
      assert.deepEqual(geom.states, ['failed', 'holding', 'idle', 'permission', 'thinking', 'tool']);
      const flat = await frame.evaluate(() => {
        const panel = document.getElementById('agisland'), compact = document.getElementById('agcompact');
        return { shadow: getComputedStyle(panel).boxShadow, border: getComputedStyle(panel).borderWidth,
          background: getComputedStyle(panel).backgroundColor, footer: !!compact.querySelector('.hud-footer'),
          expand: !!compact.querySelector('.hud-expand-btn'), text: compact.textContent };
      });
      assert.equal(flat.shadow, 'none'); assert.equal(flat.border, '0px');
      assert.match(flat.background, /^color\(|^rgba\(/);
      assert.equal(flat.footer, false); assert.equal(flat.expand, false); assert(!flat.text.includes('완료'));
      if (shots) await frame.locator('#agisland').screenshot({ path: `${shots}/hud-compact-${scheme}.png` });
      await frame.click('#agcompact', { position: { x: 2, y: 2 } });
      await frame.locator('#agisland[data-mode="full"]').waitFor();
      assert.equal(await frame.getAttribute('#agcompact', 'aria-expanded'), 'true');
      if (shots && scheme === 'dark') await tab.screenshot({ path: `${shots}/hud-expanded-dark.png` });
      await frame.click('#agclose');
      await frame.locator('#agisland[data-mode="compact"]').waitFor();
      await frame.locator('#agcompact').focus();
      await frame.locator('#agcompact').press('Enter');
      await frame.locator('#agisland[data-mode="full"]').waitFor();
      await frame.click('#agclose');
      await frame.locator('#agisland[data-mode="compact"]').waitFor();
      if (shots) await tab.screenshot({ path: `${shots}/compact-${scheme}.png` });
      if (shots) await frame.locator('#agcompact').screenshot({ path: `${shots}/hud-${scheme}.png` });

      // Node and close controls keep their own actions in each theme.
      await frame.click('#agcompact .hud-node[data-id="h3"]');
      await frame.locator('#agisland[data-mode="full"]').waitFor();
      assert.equal(await frame.locator('#agpop details[data-key="agent-h3"]').evaluate(el => el.open), true);
      await frame.click('#agclose');
      await frame.locator('#agisland[data-mode="compact"]').waitFor();
      await frame.locator('#agcompact .hud-node[data-id="h2"]').hover();
      await frame.locator('#agcompact .node-slot:has(.hud-node[data-id="h2"]) .hud-close').click();
      const closeDialog = frame.locator('dialog.rdeldlg[open]');
      await closeDialog.waitFor();
      assert.equal(await frame.getAttribute('#agisland', 'data-mode'), 'compact');
      await closeDialog.locator('.pbtn:not(.danger)').click();

      if (process.env.WS_SUBAGENT_SHOTS) {
        const dir = process.env.WS_SUBAGENT_SHOTS;
        let closes = 0;
        await tab.route('**/agents/boardsession/close', async route => {
          closes++;
          await route.fulfill({ status: 200, body: '{}' });
        });
        const node = frame.locator('#agcompact .hud-node[data-id="h2"]');
        await node.hover();
        const close = frame.locator('#agcompact .node-slot:has(.hud-node[data-id="h2"]) .hud-close');
        await close.waitFor({ state: 'visible' });
        await frame.waitForTimeout(250);
        await tab.screenshot({ path: `${dir}/hud-hover-${scheme}.png` });
        for (const [id, state] of [['h11', 'failed'], ['h8', 'idle']]) {
          const target = frame.locator(`#agcompact .hud-node[data-id="${id}"]`);
          assert.equal(await target.getAttribute('data-state'), state);
          await target.hover();
          await frame.locator(`#agcompact .node-slot:has(.hud-node[data-id="${id}"]) .hud-close`).waitFor({ state: 'visible' });
          await frame.waitForTimeout(250);
          await tab.screenshot({ path: `${dir}/hud-${state}-hover-${scheme}.png` });
        }
        await node.hover();
        await close.click();
        const dialog = frame.locator('dialog.rdeldlg[open]');
        await dialog.waitFor();
        assert.equal(closes, 0, 'active child waits for confirmation');
        await tab.screenshot({ path: `${dir}/close-modal-${scheme}.png` });
        await dialog.locator('.pbtn:not(.danger)').click();
        assert.equal(closes, 0, 'cancel sends no request');
        await frame.click('#agcompact', { position: { x: 2, y: 2 } });
        await frame.locator('#agpop details[data-key="agent-h1"] > summary').click();
        await frame.waitForTimeout(500);
        const row = frame.locator('#agpop .agent:has(> details[data-key="agent-h2"])');
        await row.locator('> details > summary').hover();
        const rowClose = row.locator('> details > summary > .aclose');
        await rowClose.waitFor({ state: 'visible' });
        await frame.waitForTimeout(250);
        await tab.screenshot({ path: `${dir}/agpop-hover-${scheme}.png` });
        await rowClose.click();
        await dialog.waitFor();
        await dialog.locator('.pbtn.danger').click();
        await frame.waitForTimeout(150);
        assert.equal(closes, 1, 'confirmed close sends one request');
        await tab.unroute('**/agents/boardsession/close');
        if (await frame.locator('#agclose').isVisible()) await frame.locator('#agclose').click();
      }

      if (scheme === 'dark') {
        await checkKeyedRebuild(frame);
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
    for (const scheme of ['dark', 'light']) {
      await fetch(`${switcher}?s=idle`);
      const ctx = await b.newContext({ viewport: { width: 1280, height: 840 }, colorScheme: 'dark', locale: 'ko-KR' });
      await ctx.addCookies([{ name: 'ws_dashboard_lang', value: 'ko', url: base }]);
      const tab = await ctx.newPage();
      await tab.goto(`${base}/s/livesession`);
      const frame = await (await tab.locator('iframe.sframe.on').elementHandle()).contentFrame();
      await frame.locator('#agisland[data-mode="idle"]:not([hidden])').waitFor();
      await frame.waitForTimeout(250);
      await chooseTheme(tab, frame, scheme);
      const idleStyle = await frame.evaluate(() => ({ shadow: getComputedStyle(document.getElementById('agisland')).boxShadow,
        border: getComputedStyle(document.getElementById('agisland')).borderWidth, text: document.getElementById('agidle').textContent }));
      assert.equal(idleStyle.shadow, 'none'); assert.equal(idleStyle.border, '0px'); assert(!idleStyle.text.includes('완료'));
      if (shots) await frame.locator('#agisland').screenshot({ path: `${shots}/hud-idle-${scheme}.png` });
      await frame.click('#agidle');
      await frame.locator('#agisland[data-mode="full"]').waitFor();
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
