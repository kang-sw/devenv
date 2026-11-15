// Captures the landing guide's screenshots (assets/guide) from the real
// dashboard, run by TestLandingGuideCapture over disposable fixture sessions
// on an ephemeral port (never the shared daemon). Regenerate after a UI
// change, from agents-plugin-tool:
//
//   WS_GUIDE_CAPTURE=$PWD/internal/wsdashboard/assets/guide \
//   WS_PLAYWRIGHT_MODULE=<node_modules>/playwright \
//   PLAYWRIGHT_BROWSERS_PATH=$HOME/Library/Caches/ms-playwright \
//   go test ./internal/wsdashboard -run TestLandingGuideCapture -count=1
//
// Each step is shot in both themes at 2x, cropped to its region, marked
// (a ring around the control and a short callout, drawn into the page
// before the shot), and written as WebP: <lang>-<theme>-<step>.webp, plus
// guide.json with every image's CSS size, which the landing reads for its
// <img> width and height. Korean captures serve Korean pages; English ones
// serve every other language.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { chromium } = require(process.env.WS_PLAYWRIGHT_MODULE);
const [base, lang, out, main, fresh, ask, home] = process.argv.slice(2);

const SCALE = 2;
// The widest image kept, in device pixels: 800 CSS px at 2x, about the
// guide's column.
const MAX_W = 1600;
const QUALITY = 0.8;
const ACCENT = '#e8552f';
// The folder the add-workspace step types (the fixture makes it).
const WS_PATH = '~/projects/web-shop';
// The window the whole-page steps (board, answer) are shot in.
const NARROW = { width: 1120, height: 720 };

// The callouts, per language.
const CALL = {
  ko: { list: '세션 목록', wsAdd: '워크스페이스 추가', wsPath: '폴더 경로', wsGo: '추가', plus: '새 세션', start: '시작', send: '입력 후 Enter', card: '보드 업데이트', board: '화이트보드', pick: '선택', sendAns: '전송', files: '파일', gear: '설정' },
  en: { list: 'Sessions', wsAdd: 'Add workspace', wsPath: 'Folder path', wsGo: 'Add', plus: 'New', start: 'Start', send: 'Type, then Enter', card: 'Board update', board: 'Whiteboard', pick: 'Pick', sendAns: 'Send', files: 'Files', gear: 'Settings' }
}[lang];

const on = '#frames iframe.on';
const sizes = {};

// tidy shows the fixture's home folder as ~, so the shots carry no
// temporary path.
async function tidy(page) {
  for (const f of page.frames()) {
    await f.evaluate(home => {
      const swap = s => s.split(home).join('~');
      const walk = document.createTreeWalker(document.body || document.documentElement, NodeFilter.SHOW_TEXT);
      for (let n = walk.nextNode(); n; n = walk.nextNode()) if (n.nodeValue.includes(home)) n.nodeValue = swap(n.nodeValue);
      document.querySelectorAll('[title]').forEach(el => { if (el.title.includes(home)) el.title = swap(el.title); });
    }, home).catch(() => {});
  }
}

// rectOf is a selector's box in the top page's coordinates, whether it is
// in the top page or in the shown session frame.
async function rectOf(page, sel, inFrame) {
  const loc = inFrame ? page.frameLocator(on).locator(sel).first() : page.locator(sel).first();
  await loc.waitFor({ state: 'visible' });
  const r = await loc.boundingBox();
  assert.ok(r, 'no box for ' + sel);
  return r;
}

// mark draws a ring around each box and its callout beside it, in the top
// page over everything (the shell's frame among it). A mark's top, when
// set, places its callout at that height instead, keeping it off what lies
// between.
async function mark(page, marks) {
  await page.evaluate(([marks, accent]) => {
    document.querySelectorAll('.guide-mark').forEach(el => el.remove());
    for (const m of marks) {
      const pad = m.pad ?? 4, r = m.box;
      const ring = document.body.appendChild(document.createElement('div'));
      ring.className = 'guide-mark';
      Object.assign(ring.style, {
        position: 'fixed', zIndex: 2147483647, pointerEvents: 'none', boxSizing: 'border-box',
        left: (r.x - pad) + 'px', top: (r.y - pad) + 'px', width: (r.width + 2 * pad) + 'px', height: (r.height + 2 * pad) + 'px',
        border: '3px solid ' + accent, borderRadius: (m.round ?? 9) + 'px', boxShadow: '0 0 0 4px rgba(232,85,47,.22)'
      });
      const tag = document.body.appendChild(document.createElement('div'));
      tag.className = 'guide-mark';
      if (m.n) {
        const n = tag.appendChild(document.createElement('b'));
        n.textContent = m.n;
        Object.assign(n.style, { display: 'inline-grid', placeItems: 'center', width: '18px', height: '18px', marginRight: '6px', borderRadius: '50%', background: '#fff', color: accent, fontSize: '12px' });
      }
      tag.appendChild(document.createTextNode(m.label));
      Object.assign(tag.style, {
        position: 'fixed', zIndex: 2147483647, pointerEvents: 'none', whiteSpace: 'nowrap', display: 'flex', alignItems: 'center',
        font: '600 14px/18px -apple-system, "Segoe UI", "Apple SD Gothic Neo", "Noto Sans KR", sans-serif',
        color: '#fff', background: accent, padding: '5px 11px 5px ' + (m.n ? '5px' : '11px'), borderRadius: '999px', boxShadow: '0 2px 10px rgba(0,0,0,.35)'
      });
      const t = tag.getBoundingClientRect(), gap = 10;
      let x, y;
      switch (m.side || 'right') {
        case 'right': x = r.x + r.width + pad + gap; y = r.y + r.height / 2 - t.height / 2; break;
        case 'left': x = r.x - pad - gap - t.width; y = r.y + r.height / 2 - t.height / 2; break;
        case 'below': x = r.x + r.width / 2 - t.width / 2; y = r.y + r.height + pad + gap; break;
        case 'above': x = r.x + r.width / 2 - t.width / 2; y = r.y - pad - gap - t.height; break;
        case 'inside': x = r.x + r.width - t.width - 12; y = r.y + r.height - t.height - 12; break;
      }
      tag.style.left = x + 'px';
      tag.style.top = (m.top ?? y) + 'px';
    }
  }, [marks, ACCENT]);
}

// save shoots clip, scales it to at most MAX_W, and writes it as WebP.
async function save(page, theme, step, clip) {
  const png = await page.screenshot({ clip, animations: 'disabled' });
  const { data, w, h } = await page.evaluate(async ([b64, maxW, q]) => {
    const img = new Image();
    img.src = 'data:image/png;base64,' + b64;
    await img.decode();
    const s = Math.min(1, maxW / img.naturalWidth);
    const c = document.createElement('canvas');
    c.width = Math.round(img.naturalWidth * s);
    c.height = Math.round(img.naturalHeight * s);
    const g = c.getContext('2d');
    g.imageSmoothingQuality = 'high';
    g.drawImage(img, 0, 0, c.width, c.height);
    const url = c.toDataURL('image/webp', q);
    return { data: url.slice(url.indexOf(',') + 1), w: c.width, h: c.height };
  }, [png.toString('base64'), MAX_W, QUALITY]);
  assert.ok(data.length > 0, step + ': no WebP');
  const name = lang + '-' + theme + '-' + step + '.webp';
  fs.writeFileSync(path.join(out, name), Buffer.from(data, 'base64'));
  sizes[name] = { w: Math.round(w / SCALE), h: Math.round(h / SCALE) };
  await page.evaluate(() => document.querySelectorAll('.guide-mark').forEach(el => el.remove()));
  console.log(name, w + 'x' + h, fs.statSync(path.join(out, name)).size + ' bytes');
}

async function openRail(page) {
  await page.locator('#railbtn').click();
  await page.locator('#rail').evaluate(el => new Promise(r => {
    const t = () => (getComputedStyle(el).visibility === 'visible' && el.getBoundingClientRect().left >= 0 ? r() : requestAnimationFrame(t));
    t();
  }));
  await page.waitForTimeout(350);
}

async function closeRail(page) {
  for (let i = 0; i < 3 && await railShown(page); i++) {
    await page.keyboard.press('Escape');
    await page.waitForTimeout(300);
    if (await railShown(page)) await page.locator('#rscrim').click({ force: true }).catch(() => {});
    await page.waitForTimeout(300);
  }
  assert.equal(await railShown(page), false, 'the session list stays open');
}

function railShown(page) {
  return page.locator('#rail').evaluate(el => getComputedStyle(el).visibility === 'visible' && el.getBoundingClientRect().right > 0);
}

async function settle(page, ms = 500) {
  await page.waitForTimeout(ms);
  await tidy(page);
}

async function shows(page, pathname) {
  await page.waitForFunction(p => {
    const el = document.querySelector('#frames iframe.on');
    if (location.pathname !== p || !el) return false;
    try { return el.contentWindow.location.pathname === p && el.contentWindow.document.readyState === 'complete'; } catch (e) { return false; }
  }, pathname);
}

async function capture(browser, theme) {
  const c = await browser.newContext({ viewport: { width: 1440, height: 900 }, deviceScaleFactor: SCALE, colorScheme: theme, locale: lang === 'ko' ? 'ko-KR' : 'en-US' });
  await c.addCookies([{ name: 'ws_dashboard_lang', value: lang, domain: new URL(base).hostname, path: '/' }]);
  const page = await c.newPage();
  const errors = [];
  page.on('pageerror', e => errors.push(e.message));

  // 1. The session list's button, top left.
  await page.goto(base + '/');
  await page.frameLocator(on).locator('.dlanding').waitFor();
  await settle(page);
  await mark(page, [{ box: await rectOf(page, '#railbtn'), label: CALL.list, side: 'right', pad: 5 }]);
  await save(page, theme, 'open', { x: 0, y: 0, width: 420, height: 60 });

  // 2. The + on the session list's header opens the add-workspace popover:
  // type the project folder's path, and Add. The callouts for the path and
  // Add sit under the popover, off the preview of what Add will add.
  await openRail(page);
  const wsAdd = '#rail .rhd .radd[data-add="ws"]';
  await page.locator(wsAdd).click();
  const wsPop = '.rpop:not([hidden])';
  await page.locator(wsPop + ' .spq').waitFor({ state: 'visible' });
  await page.locator(wsPop + ' .spq').fill(WS_PATH);
  await page.locator(wsPop + ' .spres code').waitFor({ state: 'visible' });
  await settle(page, 400);
  const wpop = await rectOf(page, wsPop);
  const wrail = await rectOf(page, '#rail');
  await mark(page, [
    { box: await rectOf(page, wsAdd), label: CALL.wsAdd, n: '1', side: 'right', pad: 3 },
    { box: await rectOf(page, wsPop + ' .spq'), label: CALL.wsPath, n: '2', side: 'below', pad: 3, top: wpop.y + wpop.height + 8 },
    { box: await rectOf(page, wsPop + ' .pbtn.go'), label: CALL.wsGo, n: '3', side: 'below', pad: 3, top: wpop.y + wpop.height + 8 }
  ]);
  const wsRight = Math.max(wrail.x + wrail.width + 2, wpop.x + wpop.width + 8);
  await save(page, theme, 'workspace', { x: 0, y: 0, width: Math.round(Math.min(1440, wsRight)), height: Math.round(Math.min(900, wpop.y + wpop.height + 48)) });
  await closeRail(page);

  // 3. A worktree row's + opens the new-session popover; Start spawns.
  await openRail(page);
  const wt = '#rail .rwt[data-g$="my-app-login"]';
  await page.locator(wt + ' .wthd').hover();
  await page.locator(wt + ' .rnew').click();
  await page.locator('.rpop:not([hidden]) .pbtn.go').waitFor({ state: 'visible' });
  await settle(page, 400);
  const pop = await rectOf(page, '.rpop:not([hidden])');
  await mark(page, [
    { box: await rectOf(page, wt + ' .rnew'), label: CALL.plus, n: '1', side: 'right', pad: 3 },
    { box: await rectOf(page, '.rpop:not([hidden]) .pbtn.go'), label: CALL.start, n: '2', side: 'left', pad: 4 }
  ]);
  const railBox = await rectOf(page, '#rail');
  await save(page, theme, 'new', { x: 0, y: 0, width: Math.round(railBox.x + railBox.width + 2), height: Math.round(Math.min(900, pop.y + pop.height + 16)) });

  // 4. Start opens the new session; ask for the whiteboard in its
  // message box.
  await page.locator('.rpop:not([hidden]) .pbtn.go').click();
  await shows(page, '/s/' + fresh);
  await closeRail(page);
  const frame = page.frameLocator(on);
  await frame.locator('#composer .wsc-editor').click();
  await page.keyboard.insertText(ask);
  await settle(page, 600);
  const comp = await rectOf(page, '#composer', true);
  await mark(page, [{ box: comp, label: CALL.send, side: 'above', pad: 4 }]);
  const top3 = Math.round(comp.y - 150);
  await save(page, theme, 'ask', { x: 0, y: top3, width: Math.round(comp.x + comp.width + 16), height: 900 - top3 });

  // 5. The agent's board: the update card in the conversation, and the
  // board beside it. A narrower window keeps the whole page legible at the
  // guide's column width.
  await page.setViewportSize({ width: NARROW.width, height: NARROW.height });
  await page.goto(base + '/s/' + main);
  await shows(page, '/s/' + main);
  await frame.locator('.wbupdate').waitFor();
  await settle(page, 900);
  const topic = await rectOf(page, '#bcol section.topic', true);
  const bottom4 = Math.round(Math.min(NARROW.height, topic.y + topic.height + 14));
  await mark(page, [
    { box: await rectOf(page, '.wbupdate', true), label: CALL.card, n: '1', side: 'below', pad: 4 },
    { box: { x: topic.x, y: topic.y, width: topic.width, height: Math.min(topic.height, bottom4 - 10 - topic.y) }, label: CALL.board, n: '2', side: 'inside', pad: 3 }
  ]);
  await save(page, theme, 'board', { x: 0, y: 44, width: NARROW.width, height: bottom4 - 44 });

  // 6. Answer: pick an option, which stages it on the message box, and
  // send.
  await frame.locator('[data-opt="a"]').first().click();
  await frame.locator('#cchips .achip').waitFor();
  await settle(page, 400);
  await mark(page, [
    { box: await rectOf(page, '[data-opt="a"]', true), label: CALL.pick, n: '1', side: 'below', pad: 4 },
    { box: await rectOf(page, '#csend', true), label: CALL.sendAns, n: '2', side: 'above', pad: 4 }
  ]);
  const top5 = Math.round((await rectOf(page, '#bcol section.topic .wb-entry.timeline-ask, #bcol section.topic [data-entry-item]', true)).y - 2);
  await save(page, theme, 'answer', { x: 0, y: top5, width: NARROW.width, height: NARROW.height - top5 });
  await page.setViewportSize({ width: 1440, height: 900 });
  await page.goto(base + '/s/' + main);
  await shows(page, '/s/' + main);

  // 7. Files: the tab above the conversation shows the worktree.
  const ftab = 'button[data-ctab="files"]';
  await frame.locator(ftab).click();
  await frame.locator(ftab + '[aria-pressed="true"]').waitFor();
  await settle(page, 1200);
  const tab = await rectOf(page, ftab, true);
  await mark(page, [{ box: tab, label: CALL.files, side: 'right', pad: 3 }]);
  const top6 = Math.round(tab.y - 14);
  await save(page, theme, 'files', { x: 0, y: top6, width: 612, height: 250 });

  // 8. Settings: the gear at the bottom of the session list.
  await page.goto(base + '/');
  await page.frameLocator(on).locator('.dlanding').waitFor();
  await openRail(page);
  await page.locator('#gear').click();
  await page.locator('#gmenu').waitFor({ state: 'visible' });
  await settle(page, 400);
  const menu = await rectOf(page, '#gmenu');
  await mark(page, [{ box: await rectOf(page, '#gear'), label: CALL.gear, side: 'right', pad: 4, round: 999 }]);
  const rail = await rectOf(page, '#rail');
  const top7 = Math.round(menu.y - 3);
  await save(page, theme, 'settings', { x: 0, y: top7, width: Math.round(rail.x + rail.width + 2), height: 900 - top7 });

  assert.deepEqual(errors, [], theme + ': page errors');
  await c.close();
}

(async () => {
  const browser = await chromium.launch({ headless: true });
  try {
    for (const theme of ['dark', 'light']) await capture(browser, theme);
    const file = path.join(out, 'guide.json');
    const all = fs.existsSync(file) ? JSON.parse(fs.readFileSync(file, 'utf8')) : {};
    for (const k of Object.keys(all)) if (k.startsWith(lang + '-')) delete all[k];
    Object.assign(all, sizes);
    const lines = Object.keys(all).sort().map(k => '  ' + JSON.stringify(k) + ': ' + JSON.stringify({ w: all[k].w, h: all[k].h }));
    fs.writeFileSync(file, '{\n' + lines.join(',\n') + '\n}\n');
  } finally {
    await browser.close();
  }
})().catch(e => { console.error(e); process.exitCode = 1; });
