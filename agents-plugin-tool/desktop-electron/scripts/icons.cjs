'use strict';
// Dev-time only: node scripts/icons.cjs [/path/to/cached/playwright]
// Packaging copies the checked-in outputs and never runs Chromium.
const fs = require('node:fs');
const path = require('node:path');
const SIZES = [16, 24, 32, 48, 64, 128, 256];

function packICO(images) {
  const header = Buffer.alloc(6 + images.length * 16);
  header.writeUInt16LE(1, 2);
  header.writeUInt16LE(images.length, 4);
  let offset = header.length;
  images.forEach(({ size, png }, index) => {
    const entry = 6 + index * 16;
    header[entry] = header[entry + 1] = size === 256 ? 0 : size;
    header.writeUInt16LE(1, entry + 4);
    header.writeUInt16LE(32, entry + 6);
    header.writeUInt32LE(png.length, entry + 8);
    header.writeUInt32LE(offset, entry + 12);
    offset += png.length;
  });
  return Buffer.concat([header, ...images.map(image => image.png)]);
}

async function generate(playwright = require('playwright')) {
  const svg = fs.readFileSync(path.resolve(__dirname, '../../../ai-docs/ref/brand/werkschaft-mark.svg'), 'utf8');
  const browser = await playwright.chromium.launch({ headless: true });
  try {
    const page = await browser.newPage({ deviceScaleFactor: 1 });
    const images = [];
    for (const size of SIZES) {
      await page.setViewportSize({ width: size, height: size });
      await page.setContent(`<style>html,body{margin:0;background:transparent}svg{display:block;width:${size}px;height:${size}px}</style>${svg}`);
      images.push({ size, png: await page.screenshot({ omitBackground: true }) });
    }
    const icons = path.resolve(__dirname, '../icons');
    fs.mkdirSync(icons, { recursive: true });
    fs.writeFileSync(path.join(icons, 'icon.png'), images.at(-1).png);
    fs.writeFileSync(path.join(icons, 'icon.ico'), packICO(images));
    console.log(`Generated transparent PNG-entry ICO (${SIZES.join(', ')}px) and 256px icon.png`);
  } finally { await browser.close(); }
}
if (require.main === module) {
  generate(require(process.argv[2] || 'playwright')).catch(error => { console.error(error); process.exitCode = 1; });
}
module.exports = { SIZES, packICO, generate };
