'use strict';
const { createPublicKey, verify } = require('node:crypto');

function decode(value) {
  const bytes = Buffer.from(value, 'base64');
  if (!bytes.length || bytes.toString('base64') !== value) throw new Error('Invalid base64 in Tauri signature or key');
  return bytes;
}

function publicKey(encoded) {
  const lines = decode(encoded).toString('utf8').trimEnd().split(/\r?\n/);
  const packet = decode(lines[1] || '');
  if (lines.length !== 2 || !lines[0].startsWith('untrusted comment: ') ||
      packet.length !== 42 || packet.toString('ascii', 0, 2) !== 'Ed') {
    throw new Error('Invalid Tauri updater public key');
  }
  // Minisign stores the raw Ed25519 key after its algorithm and 8-byte key ID.
  const der = Buffer.concat([Buffer.from('302a300506032b6570032100', 'hex'), packet.subarray(10)]);
  return { id: packet.subarray(2, 10), key: createPublicKey({ key: der, format: 'der', type: 'spki' }) };
}

function verifySignature({ id, key }, signature, digest, version) {
  const lines = decode(signature).toString('utf8').trimEnd().split(/\r?\n/);
  const packet = decode(lines[1] || '');
  const global = decode(lines[3] || '');
  const prefix = 'trusted comment: ';
  if (lines.length !== 4 || !lines[0].startsWith('untrusted comment: ') || !lines[2].startsWith(prefix) ||
      packet.length !== 74 || packet.toString('ascii', 0, 2) !== 'ED' || global.length !== 64 ||
      !id.equals(packet.subarray(2, 10))) {
    throw new Error('Invalid version-bound Tauri updater signature or mismatched key');
  }
  // Current Tauri signatures use Minisign's prehashed mode. Verify the
  // payload's BLAKE2b-512 digest and the trusted comment independently.
  const rawSignature = packet.subarray(10);
  const comment = lines[2].slice(prefix.length);
  if (!verify(null, digest, key, rawSignature) ||
      !verify(null, Buffer.concat([rawSignature, Buffer.from(comment)]), key, global)) {
    throw new Error('Could not verify Tauri updater signature');
  }
  const versions = comment.split('\t').filter(field => field.startsWith('version:'));
  if (versions.length !== 1 || versions[0] !== `version:${version}`) {
    throw new Error(`Tauri updater signature does not bind version ${version}`);
  }
}

module.exports = { publicKey, verifySignature };
