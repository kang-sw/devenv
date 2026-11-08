#!/usr/bin/env node
// Golden testdata generator for the wstracker parity tests. It runs the mockup
// reference engine (ai-docs/ref/tracker-mockup/engine.js) over its fixtures
// and writes, per viewer clone, the events that clone holds with their
// protocol-owned verdicts and the expected view state. go test reads the JSON
// only; rerun this after an engine or fixture change:
//
//   node agents-plugin-tool/internal/wstracker/testdata/gen-golden.js
//
// Translations, applied in memory before the engine runs:
//   - The fixture's roles.add_member stand-in (ada.pc seq 11, adding pia under
//     raj) has no place in either Go layer: pia moves into genesis as a root
//     and the event becomes roles.set_lead {pia, [raj]}.
//   - A broken event (wrong ref, seq gap) gets the wsledger id form: the ref
//     owner's stream, the claimed seq, and a Broken tag "c<commit>:<line>".
//     Every expected string equal to the mock id is rewritten to it, and its
//     alias, stream and member follow the wsledger form.
'use strict';

const fs = require('fs');
const path = require('path');

const mockDir = path.resolve(__dirname, '../../../../ai-docs/ref/tracker-mockup');
global.window = {};
require(path.join(mockDir, 'fixtures.js'));
require(path.join(mockDir, 'engine.js'));
const E = window.TrackerEngine;
const FX = window.TRACKER_FIXTURES;

const VIEWERS = {
  'team-alpha': ['ada.pc', 'maya.pc', 'maya.lap', 'lena.pc', 'omar.pc'],
  'hana-personal': ['hana.pc'],
};
const IDENTITY = ['roles.add_admin', 'roles.remove_admin', 'roles.add_key', 'roles.revoke_key', 'roles.remove_member'];
// Rule verdicts the protocol owns for domain events (engine check() prelude).
const PROTOCOL_REASONS = [
  /^author ".*" is not a member in the state its author had seen$/,
  /^\S+ was removed from the store \(/,
  /^commit is unsigned$/,
  /^signed with .*, already revoked in its own causal past/,
  /^signer key ".*" is not a key of /,
];

function clone(x) { return JSON.parse(JSON.stringify(x)); }

function translate(def) {
  def = clone(def);
  Object.keys(def.commits || {}).forEach(function (ref) {
    def.commits[ref].forEach(function (c) {
      c.events.forEach(function (ev) {
        if (ev.type !== 'roles.add_member') return;
        const m = ev.payload.member;
        def.roles.members.push({ id: m.id, profile: m.profile, keys: m.keys, leads: [] });
        ev.type = 'roles.set_lead';
        ev.payload = { member: m.id, leads: m.leads || [] };
      });
    });
  });
  return def;
}

// What clone s holds: the engine store's input(s), rebuilt from raw().
function inputOf(raw, s) {
  const c = raw.clones[s], commits = {};
  Object.keys(raw.remote).forEach(function (ref) {
    const n = ref === s ? raw.remote[ref].length : (c.have[ref] || 0);
    commits[ref] = raw.remote[ref].slice(0, n);
  });
  if (c.local.length) commits[s] = (commits[s] || []).concat(c.local);
  return commits;
}

// The engine's flatten() id assignment, kept in step with it.
function flatten(commits) {
  const out = [];
  Object.keys(commits).forEach(function (ref) {
    let last = 0;
    commits[ref].forEach(function (c, ci) {
      (c.events || []).forEach(function (ev, ei) {
        const stream = String(c.stream || '');
        let id = stream + '#' + ev.seq, broken = null;
        if (stream !== ref || ev.seq !== last + 1) {
          id = '!' + out.length + ':' + ref + ':' + id;
          broken = 'c' + ci + ':' + ei;
        } else last = ev.seq;
        out.push({ mockId: id, ref: ref, commit: c, ev: ev, broken: broken });
      });
    });
  });
  return out;
}

function protocolVerdict(e) {
  if (e.status === 'waiting') return ['waiting', e.reason];
  if (e.ignored) return ['ignored', e.reason];
  if (e.status === 'flagged' && /^causal cycle/.test(e.reason)) return ['flagged', e.reason];
  if (IDENTITY.indexOf(e.type) >= 0) return e.status === 'applied' ? ['valid', null] : ['flagged', e.reason];
  if (e.status === 'flagged' && PROTOCOL_REASONS.some(function (re) { return re.test(e.reason); })) return ['flagged', e.reason];
  return ['valid', null];
}

function rewrite(x, map) {
  if (typeof x === 'string') return Object.prototype.hasOwnProperty.call(map, x) ? map[x] : x;
  if (Array.isArray(x)) return x.map(function (v) { return rewrite(v, map); });
  if (x && typeof x === 'object') {
    const o = {};
    Object.keys(x).forEach(function (k) { o[k] = rewrite(x[k], map); });
    return o;
  }
  return x;
}

const outDir = path.join(__dirname, 'golden');
fs.mkdirSync(outDir, { recursive: true });
FX.stores.forEach(function (raw0) {
  const def = translate(raw0);
  (VIEWERS[def.name] || []).forEach(function (viewer) {
    const store = E.openStore(def);
    const view = store.view(viewer, def.now);
    delete view.store; delete view.sync; delete view.clones;
    writeGolden(def.name, viewer, def.now, def.roles, inputOf(store.raw(), viewer), view);
  });
});

// writeGolden writes one golden file: the events viewer holds (commits, by
// ref) with protocol-owned verdicts, and the engine's view.
function writeGolden(name, viewer, now, roles, commits, view) {
    const nodes = flatten(commits), byMock = {}, idMap = {};
    nodes.forEach(function (n) {
      byMock[n.mockId] = n;
      if (n.broken) {
        const owner = n.ref, i = owner.indexOf('.');
        n.member = owner.slice(0, i); n.clone = owner.slice(i + 1);
        idMap[n.mockId] = owner + '#' + n.ev.seq + '!' + n.broken;
      } else {
        const s = String(n.commit.stream), i = s.indexOf('.');
        n.member = s.slice(0, i); n.clone = s.slice(i + 1);
      }
    });

    const events = view.events.map(function (e) {
      const n = byMock[e.id];
      if (!n) throw new Error(viewer + ': view event ' + e.id + ' not in flatten');
      const v = n.broken ? ['flagged', e.reason] : protocolVerdict(e);
      const out = { member: n.member, clone: n.clone, seq: n.ev.seq, ts: n.ev.ts || n.commit.ts, type: n.ev.type,
        payload: n.ev.payload || {}, seen: n.commit.seen || {}, verdict: v[0] };
      if (n.ev.origin) out.origin = n.ev.origin;
      if (n.broken) out.broken = n.broken;
      if (v[1]) out.reason = v[1];
      return out;
    });

    const expected = rewrite(view, idMap);
    expected.events.forEach(function (e, i) {
      const n = byMock[view.events[i].id];
      if (!n.broken) return;
      e.alias = e.id.replace('#', '-');
      e.stream = n.ref;
      e.member = n.member;
    });

    const file = path.join(outDir, name + '__' + viewer + '.json');
    const doc = { store: name, viewer: viewer, now: now, staleDays: E.STALE_DAYS, genesis: roles, events: events, expected: expected };
    fs.writeFileSync(file, JSON.stringify(doc, null, 1) + '\n');
    console.log('wrote ' + path.relative(process.cwd(), file) + ' (' + events.length + ' events)');
}

// Case specs (cases/*.json): hand-written histories with explicit seen for
// what the fixtures do not reach, run through the engine like the fixtures.
// A spec is { name, viewers, now, genesis, commits: { stream: [{ ts, seen,
// events: [[type, payload, origin?]] }] } }; seqs are assigned per stream.
const casesDir = path.join(__dirname, 'cases');
fs.readdirSync(casesDir).filter(function (f) { return f.endsWith('.json'); }).sort().forEach(function (f) {
  const spec = JSON.parse(fs.readFileSync(path.join(casesDir, f), 'utf8'));
  const commits = {};
  Object.keys(spec.commits).forEach(function (stream) {
    let seq = 0;
    commits[stream] = spec.commits[stream].map(function (c) {
      return { stream: stream, ts: c.ts, signer: stream.split('.')[0] + '-k1', seen: c.seen || {},
        events: c.events.map(function (e) {
          const ev = { seq: ++seq, type: e[0], payload: e[1] };
          if (e[2]) ev.origin = e[2];
          return ev;
        }) };
    });
  });
  spec.viewers.forEach(function (viewer) {
    const view = E.deriveViewState({ roles: spec.genesis, commits: commits }, { stream: viewer, now: spec.now });
    writeGolden('case-' + spec.name, viewer, spec.now, spec.genesis, commits, view);
  });
});

// Flagged-reason cases: run each through the engine and check the expected
// reason, so the Go unit test's strings are the engine's words.
const RC = JSON.parse(fs.readFileSync(path.join(__dirname, 'reasons.json'), 'utf8'));
let bad = 0;
function runCase(steps) {
  const commits = {}, heads = {};
  let t = Date.parse('2026-11-01T00:00:00Z');
  const ids = steps.map(function (st) {
    const stream = st[0], seq = (heads[stream] || 0) + 1, ts = new Date(t += 60000).toISOString().replace('.000Z', 'Z');
    const seen = {};
    Object.keys(heads).forEach(function (s) { if (s !== stream) seen[s] = heads[s]; });
    const ev = { seq: seq, ts: ts, type: st[1], payload: st[2] };
    if (st[3]) ev.origin = st[3];
    (commits[stream] = commits[stream] || []).push({ stream: stream, ts: ts, signer: stream.split('.')[0] + '-k1', seen: seen, events: [ev] });
    heads[stream] = seq;
    return stream + '#' + seq;
  });
  const view = E.deriveViewState({ roles: RC.genesis, commits: commits }, { stream: 'ada.pc', now: '2026-11-02T00:00:00Z' });
  const byId = {};
  view.events.forEach(function (e) { byId[e.id] = e; });
  return ids.map(function (id) { return byId[id]; });
}
RC.cases.forEach(function (c) {
  if (c.goOnly) return;
  const evs = runCase(RC.base.concat(c.steps)), last = evs[evs.length - 1];
  evs.slice(0, -1).forEach(function (e) {
    if (e.status !== 'applied') { bad++; console.error('reasons: ' + c.name + ': setup ' + e.id + ' ' + e.status + ': ' + e.reason); }
  });
  if (last.status !== 'flagged' || last.reason !== c.reason) {
    bad++;
    console.error('reasons: ' + c.name + ': engine says ' + last.status + ' "' + last.reason + '", case expects "' + c.reason + '"');
  }
});
if (bad) { console.error(bad + ' reason case(s) disagree with the engine'); process.exit(1); }
console.log('reasons.json: ' + RC.cases.length + ' cases agree with the engine');
