// No screenshots or shared daemon: replay the real guarded route response on
// an owned loopback listener, exercising production preview JS and CSS.
const http = require('node:http');
const assert = require('node:assert/strict');
const { chromium } = require(process.env.WS_PLAYWRIGHT_MODULE);
(async () => {
  let input = '';
  for await (const chunk of process.stdin) input += chunk;
  const config = JSON.parse(input);
  let browser, server, pdfRequests = 0;
  try {
    try {
      // This is the only launch attempt; never fall back to headless shell.
      browser = await chromium.launch({ channel: 'chromium', headless: true, timeout: 10000 });
    } catch (error) {
      console.log('SKIP: full Chromium unavailable; PDF render not run: ' + error.message);
      return;
    }
    server = http.createServer((req, res) => {
      if (req.url.startsWith('/s/rid1/fs/raw?')) {
        pdfRequests++;
        for (const [key, values] of Object.entries(config.headers)) res.setHeader(key, values);
        res.end(Buffer.from(config.pdf, 'base64'));
      } else if (req.url.startsWith('/s/rid1/fs/preview?')) {
        res.setHeader('Content-Type', 'application/json');
        res.end(JSON.stringify({ kind: 'pdf', src: '/s/rid1/fs/raw?path=doc.pdf' }));
      } else {
        res.setHeader('Content-Type', 'text/html');
        res.end('<!doctype html><html data-rid="rid1"><body><div id="card" style="height:500px"><div id="preview" style="height:100%"></div></div></body></html>');
      }
    });
    await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
    const page = await browser.newPage();
    page.setDefaultTimeout(10000);
    await page.goto('http://127.0.0.1:' + server.address().port);
    await page.addStyleTag({ content: config.css });
    await page.evaluate(() => {
      window.wsT = key => key;
      const kinds = {}, cards = [];
      window.WsCards = {
        register: (name, kind) => kinds[name] = kind,
        list: () => cards,
        open: (kind, data) => {
          const card = { data, body: document.querySelector('#preview'), el: document.querySelector('#card'),
            title() {}, actions() {}, paint() {}, bump() {}, fold() {}, show() {}, save() {} };
          cards.push(card); kinds[kind].open(card); return card;
        }
      };
    });
    await page.addScriptTag({ content: config.preview });
    const enabled = await page.evaluate(() => navigator.pdfViewerEnabled);
    assert.equal(enabled, true, 'full Chromium must advertise its native PDF viewer');
    await page.evaluate(() => window.WsPreview.open('doc.pdf'));
    await page.locator('.pv-pdf iframe').waitFor();
    // Native PDF viewer is an extension frame. Its success state is stronger
    // evidence than iframe load (which also fires for blank/error documents).
    let rendered;
    const deadline = Date.now() + 12000;
    while (Date.now() < deadline && !rendered) {
      for (const frame of page.frames()) {
        const state = await frame.evaluate(() => {
          const viewer = document.querySelector('pdf-viewer');
          if (!viewer) return null;
          const toolbar = viewer.shadowRoot && viewer.shadowRoot.querySelector('viewer-toolbar');
          return { state: viewer.loadState_, pages: toolbar && toolbar.docLength };
        }).catch(() => null);
        if (state && state.state === 'success' && state.pages === 1) rendered = state;
      }
      if (!rendered) await page.waitForTimeout(100);
    }
    assert.ok(rendered, 'native PDF viewer must load the one-page document successfully');
    assert.ok(pdfRequests > 0);
    const requestsBeforeFallback = pdfRequests;
    await page.evaluate(() => {
      Object.defineProperty(Navigator.prototype, 'pdfViewerEnabled', { configurable: true, get: () => false });
      window.WsPreview.open('doc.pdf');
    });
    await page.waitForFunction(() => document.querySelector('.pv-pdf') && !document.querySelector('.pv-pdf iframe'));
    const link = page.locator('.pv-pdf-open');
    await link.waitFor({ state: 'visible' });
    assert.equal(await link.getAttribute('href'), '/s/rid1/fs/raw?path=doc.pdf');
    assert.equal(pdfRequests, requestsBeforeFallback, 'viewer-disabled fallback must not request a frame');
    console.log(JSON.stringify({ channel: 'chromium', rendered, fallback: 'visible link; no frame request', pdfRequests }));
  } finally {
    if (browser) await browser.close();
    if (server) await new Promise(resolve => server.close(resolve));
  }
})().catch(error => { console.error(error); process.exitCode = 1; });
