// Run by TestChatBlockActionsBrowser against a static render of chat blocks.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { chromium } = require(process.env.WS_PLAYWRIGHT_MODULE);
const [file, shots] = process.argv.slice(2);

const settle = page => page.waitForTimeout(250);
// A tooltip shows .35s after hover or focus, then fades in for .1s.
const tipSettle = page => page.waitForTimeout(650);

(async () => {
  const browser = await chromium.launch({ headless: true });
  try {
    const page = await browser.newPage({ viewport: { width: 800, height: 1100 }, colorScheme: 'dark' });
    const errors = [];
    page.on('pageerror', e => errors.push(e.message));
    await page.setContent(fs.readFileSync(file, 'utf8').replace('THEME', 'dark'));
    const blocks = '#frame .msg.fromlead, #frame .msg.user';
    const geometry = () => page.evaluate(sel => [...document.querySelectorAll(sel)].map(m => {
      const r = m.getBoundingClientRect();
      return [r.top, r.height];
    }), blocks);
    const pill = (sel) => page.evaluate(sel => {
      const msg = document.querySelector(sel), bub = msg.querySelector(':scope > .bub'), acts = msg.querySelector(':scope > .d-acts');
      const b = bub.getBoundingClientRect(), a = acts.getBoundingClientRect(), cs = getComputedStyle(acts);
      return { opacity: cs.opacity, events: cs.pointerEvents, inBubble: bub.contains(acts), acts: [...acts.querySelectorAll('.d-act')].map(x => x.getAttribute('data-act')),
        bubRight: b.right, bubBottom: b.bottom, right: a.right, top: a.top, bottom: a.bottom };
    }, sel);
    const shot = async name => { if (shots) await page.locator('#frame').screenshot({ path: path.join(shots, name + '.png') }); };
    const lead = '#frame .msg.fromlead:nth-child(4)', user = '#frame .msg.user:nth-child(6)';
    // The fixture's order: user, reply, tools, reply (the long one), turn, user, reply, turn.
    assert.equal(await page.locator('#frame .d-acts').count(), 5);

    // Rest: every pill is invisible and inert.
    const rest = await geometry();
    for (const sel of [lead, user]) {
      const p = await pill(sel);
      assert.equal(p.opacity, '0', sel);
      assert.equal(p.events, 'none', sel);
      assert.equal(p.inBubble, false, 'the pill must sit outside the bubble, whose paint a finished reply clips');
    }
    await shot('rest');

    // Hover a lead reply: its pill shows on the bubble's bottom-right edge, Copy only, nothing moves.
    await page.hover(lead + ' > .bub');
    await settle(page);
    let p = await pill(lead);
    assert.equal(p.opacity, '1');
    assert.deepEqual(p.acts, ['copy']);
    assert.ok(Math.abs(p.right - (p.bubRight + 8)) <= 1.5, `lead pill right ${p.right} vs bubble ${p.bubRight}`);
    assert.ok(p.top < p.bubBottom && p.bottom > p.bubBottom, 'the pill straddles the bubble bottom edge');
    assert.deepEqual(await geometry(), rest, 'hover must not reflow');
    await shot('hover-agent');

    // Hover an owner prompt: rewind, edit, copy; a disabled action's tooltip stays legible.
    // At rest the pill takes no pointer: the block's hover arms it, then the action takes the pointer.
    await page.hover(user + ' > .bub');
    await settle(page);
    await page.hover(user + ' [data-act="rewind"]');
    await tipSettle(page);
    p = await pill(user);
    assert.equal(p.opacity, '1');
    assert.deepEqual(p.acts, ['rewind', 'edit', 'copy']);
    assert.ok(Math.abs(p.right - (p.bubRight + 4)) <= 1.5, `user pill right ${p.right} vs bubble ${p.bubRight}`);
    const tip = await page.evaluate(sel => {
      const b = document.querySelector(sel + ' [data-act="rewind"]');
      const after = getComputedStyle(b, '::after');
      return { content: after.content, opacity: after.opacity, button: getComputedStyle(b).opacity, icon: getComputedStyle(b.querySelector('.i')).opacity, disabled: b.disabled, label: b.getAttribute('aria-label') };
    }, user);
    assert.equal(tip.opacity, '1');
    assert.ok(tip.content.includes('되감기'), tip.content);
    assert.equal(tip.button, '1', 'dim the icon, not the button: its tooltip must stay legible');
    assert.equal(tip.icon, '0.38');
    assert.equal(tip.disabled, false, 'aria-disabled keeps the action focusable');
    assert.deepEqual(await geometry(), rest, 'hover must not reflow');
    await shot('hover-user');

    // Copied: the copy confirms and its pill stays pinned with the pointer elsewhere.
    await page.mouse.move(790, 1090);
    await page.evaluate(sel => {
      const b = document.querySelector(sel + ' [data-act="copy"]');
      b.setAttribute('data-done', ''); b.parentNode.setAttribute('data-pinned', '');
    }, lead);
    await settle(page);
    p = await pill(lead);
    assert.equal(p.opacity, '1');
    const done = await page.evaluate(sel => {
      const b = document.querySelector(sel + ' [data-act="copy"]'), shown = c => getComputedStyle(b.querySelector(c)).display !== 'none';
      return { copy: shown('.d-act-cp'), check: shown('.d-act-ok'), label: shown('.d-act-done'), text: b.querySelector('.d-act-done').textContent };
    }, lead);
    assert.deepEqual(done, { copy: false, check: true, label: true, text: '복사됨' });
    await shot('copied');
    await page.evaluate(sel => {
      const b = document.querySelector(sel + ' [data-act="copy"]');
      b.removeAttribute('data-done'); b.parentNode.removeAttribute('data-pinned');
    }, lead);

    // Keyboard: tabbing onto the prompt's copy shows its pill with a focus ring.
    await page.mouse.click(790, 1090);
    let focused = false;
    for (let i = 0; i < 40 && !focused; i++) {
      await page.keyboard.press('Tab');
      focused = await page.evaluate(sel => document.activeElement === document.querySelector(sel + ' [data-act="copy"]'), user);
    }
    assert.ok(focused, 'the prompt copy action is reachable by Tab');
    await tipSettle(page);
    p = await pill(user);
    assert.equal(p.opacity, '1');
    const ring = await page.evaluate(() => { const cs = getComputedStyle(document.activeElement); return cs.outlineStyle + ' ' + cs.outlineWidth; });
    assert.equal(ring, 'solid 2px');
    const tips = await page.evaluate(() => [...document.querySelectorAll('.d-act')].filter(b => getComputedStyle(b, '::after').opacity !== '0').map(b => b.getAttribute('data-act')));
    assert.deepEqual(tips, ['copy'], 'tabbing past rewind and edit leaves no tooltip behind');
    await shot('focus');
    await page.evaluate(() => document.activeElement.blur());

    // Narrow: the pill stays inside the column.
    await page.evaluate(() => { document.getElementById('frame').style.width = '420px'; });
    await page.hover(lead + ' > .bub');
    await settle(page);
    p = await pill(lead);
    const frameRight = await page.evaluate(() => document.getElementById('frame').getBoundingClientRect().right);
    assert.equal(p.opacity, '1');
    assert.ok(p.right <= frameRight, `pill ${p.right} overflows the 420px column ${frameRight}`);
    await shot('narrow');

    assert.deepEqual(errors, []);
    console.log('chat actions: ok');
  } finally {
    await browser.close();
  }
})().catch(e => { console.error(e); process.exit(1); });
