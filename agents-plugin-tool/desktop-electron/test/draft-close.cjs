'use strict';
const assert = require('node:assert/strict');

const image = 'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jZ9kAAAAASUVORK5CYII=';
const text = 'preserve this draft';
const name = 'draft.png';

async function draftFrame(page, origin) {
  await page.goto(origin + '/s/electronfixture');
  const frame = page.frameLocator('iframe.sframe.on');
  await frame.locator('#grid:not([inert]) #ctext').waitFor({ state: 'attached' });
  return frame;
}

async function attachImageDraft(frame) {
  const input = frame.locator('#ctext');
  await input.evaluate((field, { image, text, name }) => {
    field.value = text;
    field.dispatchEvent(new Event('input', { bubbles: true }));
    const bytes = Uint8Array.from(atob(image), c => c.charCodeAt(0));
    const transfer = new DataTransfer();
    transfer.items.add(new File([bytes], name, { type: 'image/png' }));
    field.dispatchEvent(new ClipboardEvent('paste', { clipboardData: transfer, bubbles: true, cancelable: true }));
  }, { image, text, name });
  await frame.locator('#cimgs img[alt="draft.png"]').waitFor();
  assert.equal(await input.evaluate(() => window.wsSaveSessionDraft()), true);
}

async function assertImageDraft(frame) {
  assert.match(await frame.locator('#ctext').inputValue(), /preserve this draft.*\[Image #1\]/s);
  const pixels = await frame.locator('#cimgs img[alt="draft.png"]').evaluate(async img => {
    const blob = await (await fetch(img.src)).blob();
    return Array.from(new Uint8Array(await blob.arrayBuffer()));
  });
  assert.deepEqual(pixels, [...Buffer.from(image, 'base64')]);
}

async function openLegacyGuard(page, origin) {
  const url = origin + '/electron-legacy-close';
  await page.route(url, route => route.fulfill({
    contentType: 'text/html', body: '<h1>Legacy draft</h1><textarea aria-label="Draft">Legacy draft with an unpersisted image</textarea>'
  }));
  await page.goto(url);
  await page.locator('textarea').click();
  await page.evaluate(image => {
    const bytes = Uint8Array.from(atob(image), c => c.charCodeAt(0));
    const unsavedImage = new File([bytes], 'legacy.png', { type: 'image/png' });
    window.addEventListener('beforeunload', event => {
      if (!unsavedImage.size) return;
      event.preventDefault();
      event.returnValue = '';
    });
  }, image);
}

module.exports = { draftFrame, attachImageDraft, assertImageDraft, openLegacyGuard };
