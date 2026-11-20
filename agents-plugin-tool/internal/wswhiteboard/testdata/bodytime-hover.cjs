// Finite offline browser probe. Usage: node bodytime-hover.cjs FIXTURE_DIR CAPTURE_DIR
// Requires Playwright (or BODYTIME_PLAYWRIGHT_MODULE) and its Chromium. No live
// daemon/session endpoint is read. Rest/hover captures are not live acceptance.
'use strict';
const assert = require('node:assert/strict');
const fs = require('node:fs/promises');
const path = require('node:path');
const http = require('node:http');
const syncFS = require('node:fs');
const { execFileSync } = require('node:child_process');
const { chromium } = require(process.env.BODYTIME_PLAYWRIGHT_MODULE || 'playwright');
const [fixtureArg, captureArg] = process.argv.slice(2);
if (!fixtureArg || !captureArg) throw Error('FIXTURE_DIR and CAPTURE_DIR are required');
const fixture = path.resolve(fixtureArg), captures = path.resolve(captureArg);
const expected = { minutes: '본문 5분 전 갱신', unknown: '변경 시각 모름', 'stale-years': '본문 2년 전 갱신', months: '본문 2개월 전 갱신' };
let browser, browserServer, browserProcess, server, cleanupPromise;
// The next supervisor supplies a private append-only receipt path. This is
// instrumentation only; it grants no execution authority and never scans PIDs.
function receipt(event, data = {}) {
  if (!process.env.BODYTIME_RECEIPT_PATH) return;
  const fd = syncFS.openSync(process.env.BODYTIME_RECEIPT_PATH, 'a');
  try {
    syncFS.writeSync(fd, JSON.stringify({ event, wall: Date.now() / 1000, source: 'offline-cjs', ...data }) + '\n');
    syncFS.fsyncSync(fd);
  } finally { syncFS.closeSync(fd); }
}
function killOwnedBrowser() {
  if (browserProcess && browserProcess.exitCode === null && browserProcess.signalCode === null) {
    try {
      process.kill(-browserProcess.pid, 'SIGKILL');
      receipt('signal', { pid: browserProcess.pid, pgid: browserProcess.pid, signal: 'SIGKILL', result: 'sent', identity_basis: 'live unreaped BrowserServer child' });
    } catch (err) {
      receipt('signal', { pid: browserProcess.pid, signal: 'SIGKILL', result: err.code || String(err) });
      if (err.code !== 'ESRCH') console.error(err);
    }
  }
}
function bounded(promise, ms, label) {
  let timer;
  return Promise.race([promise, new Promise((_, reject) => {
    timer = setTimeout(() => reject(Error(label + ' exceeded ' + ms + 'ms')), ms);
  })]).finally(() => clearTimeout(timer));
}
function cleanup() {
  if (cleanupPromise) return cleanupPromise;
  cleanupPromise = (async () => {
    clearTimeout(watchdog);
    // Each closer is finite. Chromium is a separate, explicitly recorded
    // owned process group; a stuck protocol close must not leave it running.
    if (server) {
      receipt('fixture_close_requested');
      server.closeAllConnections();
      await bounded(new Promise(resolve => server.close(resolve)), 1000, 'fixture server close')
        .then(() => receipt('fixture_closed')).catch(err => { receipt('cleanup_block', { reason: String(err) }); console.error(err); });
    }
    if (browser) {
      receipt('browser_close_requested');
      await bounded(browser.close(), 4000, 'browser close')
        .then(() => receipt('browser_connection_closed')).catch(err => { receipt('cleanup_block', { reason: String(err) }); console.error(err); });
    }
    if (browserServer) {
      receipt('browser_server_close_requested');
      await bounded(browserServer.close(), 4000, 'browser server close')
        .then(() => receipt('browser_server_closed')).catch(err => { receipt('cleanup_block', { reason: String(err) }); console.error(err); });
    }
    killOwnedBrowser();
  })();
  return cleanupPromise;
}
function interrupt(reason) {
  console.error(reason);
  receipt('probe_interrupted', { reason });
  process.exitCode = 1;
  const hard = setTimeout(() => { killOwnedBrowser(); process.exit(1); }, 11000);
  cleanup().finally(() => { clearTimeout(hard); process.exit(1); });
}
const probeDeadline = performance.now() + 330000;
const watchdog = setTimeout(() => interrupt('body-time probe exceeded its 330s finite budget'), 330000);
process.once('SIGTERM', () => interrupt('body-time probe received SIGTERM'));
process.once('SIGINT', () => interrupt('body-time probe received SIGINT'));

function hostGeometry() {
  const rect = el => { const r = el.getBoundingClientRect(); return [r.x, r.y, r.width, r.height]; };
  return {
    scroll: [scrollX, scrollY, document.scrollingElement.scrollWidth, document.scrollingElement.scrollHeight],
    focus: [document.activeElement.tagName, document.activeElement.id],
    topics: Array.from(document.querySelectorAll('.topic'), rect),
    frames: Array.from(document.querySelectorAll('iframe.doc-frame'), rect),
    anchors: Array.from(document.querySelectorAll('.topic [id]'), rect)
  };
}

// Read-only browser evidence. Coordinates in a frame stay in that frame's
// own document; the host receiver is measured separately, without parent access.
async function hoverSnapshot(page, hit, point, errors, errorTimeline, iframe) {
  const frame = iframe ? await iframe.evaluate(el => {
    const r = el.getBoundingClientRect();
    return { x: r.x + el.clientLeft, y: r.y + el.clientTop, width: r.width, height: r.height };
  }) : null;
  const localPoint = point ? { x: point.x - (frame ? frame.x : 0), y: point.y - (frame ? frame.y : 0) } : null;
  const local = await hit.evaluate((el, localPoint) => {
    const rect = n => { if (!n) return null; const r = n.getBoundingClientRect(); return { x: r.x, y: r.y, width: r.width, height: r.height }; };
    const describe = n => n ? { tag: n.tagName, id: n.id, classes: n.className,
      capsule: !!n.closest('.wb-cap'), rect: rect(n) } : null;
    const receiver = localPoint ? document.elementFromPoint(localPoint.x, localPoint.y) : null;
    const layer = el.parentElement, tip = layer.wbTip;
    return { performanceMs: performance.now(), marker: rect(el.querySelector('.wb-chg-bar')), hit: rect(el),
      body: rect(layer.previousElementSibling), point: localPoint, receiver: describe(receiver),
      receivesPoint: !!receiver && (receiver === el || el.contains(receiver)),
      viewport: [innerWidth, innerHeight], scroll: [scrollX, scrollY],
      tip: tip ? { hidden: tip.hidden, display: getComputedStyle(tip).display, text: tip.textContent, rect: rect(tip) } : null };
  }, localPoint);
  const host = await page.evaluate(point => {
    const receiver = point ? document.elementFromPoint(point.x, point.y) : null;
    const r = receiver && receiver.getBoundingClientRect();
    return { performanceMs: performance.now(), viewport: [innerWidth, innerHeight], scroll: [scrollX, scrollY],
      receiver: receiver ? { tag: receiver.tagName, id: receiver.id, classes: receiver.className,
        capsule: !!receiver.closest('.wb-cap'), rect: [r.x, r.y, r.width, r.height] } : null };
  }, point);
  return { monotonicNs: process.hrtime.bigint().toString(), point, frame, local, host,
    pageErrors: errors.slice(), errorTimeline: errorTimeline.slice() };
}
function retainHover(row) {
  const fd = syncFS.openSync(path.join(captures, 'hover-timeline.jsonl'), 'a');
  try { syncFS.writeSync(fd, JSON.stringify(row) + '\n'); syncFS.fsyncSync(fd); }
  finally { syncFS.closeSync(fd); }
}

async function run() {
  await fs.mkdir(captures, { recursive: true });
  await fs.mkdir(path.join(captures, 'canary'), { recursive: true });
  await fs.access(path.join(fixture, 'framed.html'));
  server = http.createServer(async (req, res) => {
    try {
      const name = new URL(req.url, 'http://localhost').pathname.slice(1);
      if (!/^(?:framed\.html|wb-frame-[a-f0-9]{12}\.(?:css|js))$/.test(name)) { res.writeHead(404).end(); return; }
      const data = await fs.readFile(path.join(fixture, name));
      res.setHeader('Content-Type', name.endsWith('.js') ? 'text/javascript' : name.endsWith('.css') ? 'text/css' : 'text/html');
      res.end(data);
    } catch (err) { res.writeHead(500).end(String(err)); }
  });
  await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
  receipt('fixture_listening', { pid: process.pid, address: server.address() });
  // A crash during launch must leave explicit unknown ownership, not an
  // absent record that an outer runner could misread as clean termination.
  syncFS.writeFileSync(path.join(captures, 'browser-owner.json'), JSON.stringify({ phase: 'launching' }));
  receipt('browser_launch_requested', { parent_pid: process.pid });
  browserServer = await chromium.launchServer({ headless: true, timeout: 15000 });
  browserProcess = browserServer.process();
  // Public BrowserServer.process() supplies the exact launched identity.
  // The outer finite runner verifies start time before any fallback group kill.
  const owner = {
    pid: browserProcess.pid,
    start: execFileSync('/bin/ps', ['-p', String(browserProcess.pid), '-o', 'lstart='], { timeout: 1000, encoding: 'utf8' }).trim(),
    args: browserProcess.spawnargs
  };
  syncFS.writeFileSync(path.join(captures, 'browser-owner.json'), JSON.stringify(owner, null, 2));
  receipt('browser_registered', owner);
  browserProcess.once('exit', (code, signal) => receipt('browser_child_exit', { pid: browserProcess.pid, code, signal }));
  browser = await chromium.connect(browserServer.wsEndpoint(), { timeout: 15000 });
  const results = [], canaryResults = [];
  const scenarios = [{ width: 360, mode: 'framed', canary: true },
    ...[1280, 800, 360].map(width => ({ width, mode: 'framed', canary: false }))];
  for (const { width, mode, canary } of scenarios) {
      if (!canary) assert.equal(canaryResults.length, 1, 'mandatory canary passed before matrix');
      const captureDir = canary ? path.join(captures, 'canary') : captures;
      const page = await browser.newPage({ viewport: { width, height: 900 }, colorScheme: 'light', reducedMotion: 'reduce' });
      page.setDefaultTimeout(5000);
      const errors = [], errorTimeline = [];
      page.on('pageerror', err => {
        errors.push(String(err));
        errorTimeline.push({ monotonicNs: process.hrtime.bigint().toString(), error: String(err) });
      });
      await page.goto(`http://127.0.0.1:${server.address().port}/${mode}.html`, { waitUntil: 'networkidle', timeout: 15000 });
      // Missing selectors/timestamps/marks are a structural failure, not a cue
      // to adapt the test by redesigning or silently skipping a case.
      assert.equal(await page.locator('.topic').count(), 4, 'fixture topic contract');
      assert.equal(await page.locator('.tdoc[data-body-at]').count(), 4, 'distinct body metadata contract');
      assert.equal(await page.locator('.tbar > time.tago').count(), 4, 'activity remains separately present and first');
      assert.equal(await page.locator('iframe.doc-frame').count(), 4);
      await page.locator('#probe-focus').focus();
      for (const [topic, want] of Object.entries(expected).filter(([topic]) => !canary || topic === 'unknown')) {
        const card = page.locator(`.topic[data-topic="${topic}"]`);
        const metadata = card.locator('.tdoc[data-body-at]');
        await assert.doesNotReject(() => metadata.waitFor({ state: 'attached' }));
        assert.equal(await card.locator('.tbar .body-age').count(), 0, 'body age is not rendered in the topic header');
        assert.notEqual(await card.locator('.tbar > time.tago').textContent(), want);
        let scope = card;
        if (mode === 'framed') {
          const iframe = card.locator('iframe.doc-frame');
          assert.equal(await iframe.getAttribute('sandbox'), 'allow-scripts allow-popups allow-popups-to-escape-sandbox');
          scope = card.frameLocator('iframe.doc-frame');
          await scope.locator('.wb-chg-hit').first().waitFor();
          // Evaluate in the frame through browser automation, not product
          // parent access. The frame must remain opaque to its own script.
          const blocked = await scope.locator('body').evaluate(() => {
            try { void parent.document.body; return false; } catch (_) { return true; }
          });
          assert.equal(blocked, true, 'opaque frame origin');
        }
        const hits = scope.locator('.wb-chg-hit');
        assert.equal(await hits.count(), 4, `${topic}: new/mod/del/end-del`);
        assert.deepEqual(await scope.locator('.wb-chg-bar').evaluateAll(els => els.map(e => e.className)),
          ['wb-chg-bar new', 'wb-chg-bar mod', 'wb-chg-bar del', 'wb-chg-bar del']);
        const local = mode === 'framed' ? scope.locator('body') : card;
        // Measure in the selected topic/frame's own document, never by
        // reaching through a product parent/child origin boundary.
        const localState = () => local.evaluate(el => {
          const body = el.matches('.topic') ? el.querySelector('.doc-body:not(.framed)') : el.querySelector('.doc-body');
          if (!body) throw Error('missing local doc-body contract');
          const r = n => { const b = n.getBoundingClientRect(); return [b.x, b.y, b.width, b.height]; };
          return { body: r(body), blocks: Array.from(body.children, r), anchors: Array.from(body.querySelectorAll('[id], a[href]'), r),
            scroll: [scrollX, scrollY, document.documentElement.scrollWidth, document.documentElement.scrollHeight, body.scrollLeft, body.scrollTop, body.scrollWidth, body.scrollHeight],
            focus: [document.activeElement.tagName, document.activeElement.id] };
        });
        for (const i of canary ? [3] : [0, 1, 2, 3]) {
          const hit = hits.nth(i), bar = hit.locator('.wb-chg-bar');
          const identity = { lane: canary ? 'canary' : 'matrix', width, mode, topic, mark: i };
          const caseDeadline = performance.now() + 60000;
          let point = null;
          const trace = async phase => {
            const row = { ...identity, phase, ...await hoverSnapshot(page, hit, point, errors, errorTimeline,
              card.locator('iframe.doc-frame')) };
            retainHover(row);
            return row;
          };
          await bounded((async () => {
          try {
          await card.locator('iframe.doc-frame').scrollIntoViewIfNeeded();
          await hit.scrollIntoViewIfNeeded();
          await page.mouse.move(width - 2, 2);
          // Let frame size and fonts settle before the measured hover.
          await page.evaluate(() => new Promise(r => requestAnimationFrame(() => requestAnimationFrame(r))));
          const hostBefore = await page.evaluate(hostGeometry), bodyBefore = await localState();
          const hitBefore = await hit.boundingBox(), rest = await bar.boundingBox();
          assert.ok(hitBefore.width >= 12 && hitBefore.height >= 12, 'stable useful hit area');
          const color = await bar.evaluate(el => getComputedStyle(el).backgroundColor);
          assert.equal(color, i === 0 ? 'rgb(26, 127, 55)' : i === 1 ? 'rgb(9, 105, 218)' : 'rgb(207, 34, 46)');
          assert.equal(await scope.locator('.wb-chg-layer').evaluate(el => getComputedStyle(el).opacity), topic === 'stale-years' ? '0.35' : '1');
          assert.equal(rest.width, i < 2 ? 2 : 8);
          if (i >= 2) assert.equal(rest.height, 2);
          if (i === 0 || canary) await page.screenshot({ path: path.join(captureDir, `${width}-${mode}-${topic}-rest.png`) });
          // Keep the exact original transparent-edge point, not a painted-line
          // shortcut. A single receiver assertion diagnoses coverage, not age.
          point = { x: hitBefore.x + 1, y: hitBefore.y + Math.min(6, hitBefore.height / 2) };
          const beforeHover = await trace('before-hover');
          assert.ok(beforeHover.local.receivesPoint,
            `transparent gutter point must reach intended hit: ${JSON.stringify(beforeHover.local.receiver)}`);
          // Hit its outer transparent gutter, not only the 2px painted line.
          await page.mouse.move(hitBefore.x + 1, hitBefore.y + Math.min(6, hitBefore.height / 2));
          await trace('after-move');
          const tip = scope.locator('.wb-chg-tip');
          await tip.waitFor({ state: 'visible' });
          assert.equal(await tip.textContent(), want, 'header/gutter same value');
          assert.doesNotMatch(await tip.textContent(), /\d{1,2}:\d{2}|\d{4}[-/.]\d/, 'relative only, no date or clock');
          assert.equal(await bar.getAttribute('title'), null, 'no native absolute tooltip');
          const hover = await bar.boundingBox();
          assert.equal(hover.width, i < 2 ? rest.width * 2 : rest.width);
          assert.equal(hover.height, i < 2 ? rest.height : rest.height * 2);
          assert.equal(hover.x + hover.width / 2, rest.x + rest.width / 2, 'paint anchor x');
          assert.equal(hover.y + hover.height / 2, rest.y + rest.height / 2, 'paint anchor y');
          assert.deepEqual(await hit.boundingBox(), hitBefore, 'hit area does not move on hover');
          assert.equal(await bar.evaluate(el => getComputedStyle(el).backgroundColor), color, 'color unchanged');
          assert.deepEqual(await page.evaluate(hostGeometry), hostBefore, 'host no reflow/scroll/focus');
          assert.deepEqual(await localState(), bodyBefore, 'body no reflow/anchor/scroll/focus');
          const tipBox = await tip.boundingBox();
          assert.ok(tipBox.width > 0 && tipBox.height > 0, 'tooltip visible');
          await page.screenshot({ path: path.join(captureDir, `${width}-${mode}-${topic}-hover-${i}.png`) });
          await trace('hover-verified');
          await page.mouse.move(width - 2, 2);
          await tip.waitFor({ state: 'hidden' });
          (canary ? canaryResults : results).push({ width, mode, topic, mark: i, text: want, rest, hover, hit: hitBefore });
          } catch (err) {
            // One evidence attempt inside the unchanged case/browser budgets;
            // diagnostics never replace the first failure or consume cleanup.
            try {
            const budget = Math.min(2000, caseDeadline - performance.now(), probeDeadline - performance.now());
            if (budget > 0) {
              const evidenceDeadline = performance.now() + budget;
              try {
                await bounded((async () => {
                  await trace('failure');
                  if (performance.now() >= evidenceDeadline) throw Error('failure snapshot exhausted evidence budget');
                  await page.screenshot({ path: path.join(captureDir, `${width}-${mode}-${topic}-failure-${i}.png`),
                    timeout: Math.max(1, evidenceDeadline - performance.now()) });
                })(), budget, 'single failure evidence');
              } catch (evidenceError) {
                retainHover({ ...identity, phase: 'failure-evidence-incomplete', point, error: String(err),
                  evidenceError: String(evidenceError), pageErrors: errors.slice(), errorTimeline: errorTimeline.slice() });
              }
            } else {
              retainHover({ ...identity, phase: 'failure-evidence-omitted-budget', point, error: String(err),
                pageErrors: errors.slice(), errorTimeline: errorTimeline.slice() });
            }
            receipt('hover_case_failure', { ...identity, point, error: String(err), retry: false });
            } catch (diagnosticError) { console.error('failure evidence retention failed:', diagnosticError); }
            throw err;
          }
          })(), 60000, `${width}/${mode}/${topic}/${i}`);
        }
        // Preserve the established inline recap/log DOM and first-line placement.
        const inline = await card.evaluate(el => {
          const recap = el.querySelector('.trecap'), entry = el.querySelector('.wb-entry.agent');
          if (!recap || !entry) return null;
          const rat = recap.querySelector('.rat'), text = recap.querySelector('.rtext');
          const stamp = entry.querySelector('.entry-at'), line = entry.querySelector('.entry-text');
          const walk = document.createTreeWalker(text, NodeFilter.SHOW_TEXT);
          let node; while ((node = walk.nextNode()) && !node.textContent.trim()) {}
          const range = document.createRange(); range.setStart(node, 0); range.setEnd(node, 1);
          return { recapInline: getComputedStyle(text).display, recapLead: rat.compareDocumentPosition(text) & Node.DOCUMENT_POSITION_FOLLOWING,
            logInline: line.contains(stamp), textTop: range.getBoundingClientRect().top, timeTop: rat.getBoundingClientRect().top };
        });
        assert.ok(inline, 'retained recap/log fixture contract');
        assert.equal(inline.recapInline, 'inline');
        assert.ok(inline.recapLead && inline.logInline, 'author/time lead body inline');
        assert.ok(Math.abs(inline.textTop - inline.timeTop) < 6, 'recap text follows on first line');
      }
      assert.deepEqual(errors, [], 'no browser script errors');
      if (canary) {
        assert.equal(canaryResults.length, 1, 'mandatory end-deletion canary completed');
        await fs.writeFile(path.join(captureDir, 'geometry.json'), JSON.stringify(canaryResults, null, 2));
        receipt('canary_pass', { width, mode, topic: 'unknown', mark: 3, hover_cases: 1, captures: 2 });
      }
      await page.close();
  }
  assert.equal(canaryResults.length, 1, 'mandatory canary completed');
  assert.equal(results.length, 96, 'finite matrix completed');
  await fs.writeFile(path.join(captures, 'geometry.json'), JSON.stringify(results, null, 2));
  console.log(`PASS: mandatory canary (1 hover, 2 captures) then ${results.length} matrix hover cases, 120 matrix rest/hover captures; ${captures}`);
}
run().catch(err => { receipt('probe_failure', { error: String(err) }); console.error(err); process.exitCode = 1; }).finally(cleanup);
