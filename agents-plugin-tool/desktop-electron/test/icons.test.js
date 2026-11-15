'use strict';
const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const zlib = require('node:zlib');
const { SIZES, packICO } = require('../scripts/icons.cjs');

function verifyPNG(png, size) {
  assert.equal(png.subarray(0, 8).toString('hex'), '89504e470d0a1a0a');
  assert.equal(png.readUInt32BE(16), size);
  assert.equal(png.readUInt32BE(20), size);
  assert.equal(png[24], 8, '8-bit samples');
  assert.equal(png[25], 6, 'RGBA, not an opaque screenshot');
  const chunks = [];
  for (let offset = 8; offset < png.length;) {
    const length = png.readUInt32BE(offset);
    if (png.toString('ascii', offset + 4, offset + 8) === 'IDAT') chunks.push(png.subarray(offset + 8, offset + 8 + length));
    offset += length + 12;
  }
  const pixels = zlib.inflateSync(Buffer.concat(chunks));
  // At 16px the rounded corner is antialiased, so partial alpha is expected.
  assert(pixels[4] < 255, 'corner retains transparency (initial PNG filter predictors are zero)');
  if (size === 256) assert.equal(pixels[4], 0);
}

test('committed brand ICO embeds every required transparent PNG size and matches icon.png at 256px', () => {
  const ico = fs.readFileSync(path.resolve(__dirname, '../icons/icon.ico'));
  assert.equal(ico.readUInt16LE(0), 0);
  assert.equal(ico.readUInt16LE(2), 1);
  assert.equal(ico.readUInt16LE(4), SIZES.length);
  const images = SIZES.map((size, index) => {
    const entry = 6 + index * 16;
    assert.equal(ico[entry] || 256, size);
    assert.equal(ico[entry + 1] || 256, size);
    assert.equal(ico.readUInt16LE(entry + 4), 1);
    assert.equal(ico.readUInt16LE(entry + 6), 32);
    const length = ico.readUInt32LE(entry + 8), offset = ico.readUInt32LE(entry + 12);
    const png = ico.subarray(offset, offset + length);
    verifyPNG(png, size);
    return { size, png };
  });
  assert.deepEqual(packICO(images), ico);
  assert.deepEqual(images.at(-1).png, fs.readFileSync(path.resolve(__dirname, '../icons/icon.png')));
});
