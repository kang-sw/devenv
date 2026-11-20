(() => {
  const HARNESS = {
    claude: { label: 'Claude', permSupported: true, permTip: '',
      models: [['opus-4.6', '200K/1M'], ['sonnet-4.6', '200K/1M'], ['haiku-4.5', '200K']],
      efforts: [['low', 'fast'], ['medium', 'balanced'], ['high', 'deep'], ['max', 'full']],
      perms: [['auto', 'auto'], ['default', 'ask'], ['acceptEdits', 'edits'], ['plan', 'read-only'], ['bypassPermissions', 'bypass']] },
    codex: { label: 'Codex', permSupported: true, permTip: '',
      models: [['gpt-5.4', '1M'], ['gpt-5.3-codex', '400K'], ['o4-mini', '200K']],
      efforts: [['low', 'quick'], ['medium', 'standard'], ['high', 'deep'], ['xhigh', 'max']],
      perms: [['read-only', 'read-only'], ['workspace-write', 'workspace'], ['danger-full-access', 'unrestricted']] },
    agy: { label: 'AGY', permSupported: true, permTip: '',
      models: [['gemini-4-hydrogen', '1M'], ['claude-opus-4.6', '1M'], ['gemini-3.5-flash', '1M']],
      efforts: [['low', 'fast'], ['medium', 'balanced'], ['high', 'deep']],
      perms: [['default', 'ask'], ['accept-edits', 'edits'], ['auto', 'auto'], ['plan', 'plan'], ['bypass', 'bypass']] },
    pi: { label: 'Pi', permSupported: false, permTip: 'Pi has no native permission mode',
      models: [['pi-sonnet-4.6', '200K'], ['pi-opus-4.6', '1M'], ['pi-local-qwen', '128K']],
      efforts: [['off', 'none'], ['minimal', 'lite'], ['medium', 'std'], ['high', 'deep']],
      perms: [['—', 'none']] }
  };
  const S = {
    theme: 'dark', width: 'normal', harness: 'claude', role: 'lead', tokens: 51293, openPop: null,
    sel: {
      claude: { model: 'opus-4.6', effort: 'high', perm: 'acceptEdits' },
      codex:  { model: 'gpt-5.4', effort: 'high', perm: 'workspace-write' },
      agy:    { model: 'gemini-4-hydrogen', effort: 'high', perm: 'accept-edits' },
      pi:     { model: 'pi-sonnet-4.6', effort: 'medium', perm: '—' }
    }
  };
  const svg = (d, cls = 'ico') => `<svg class="${cls}" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">${d}</svg>`;
  const IC = {
    cpu: svg('<rect x="4" y="4" width="16" height="16" rx="2"/><rect x="9" y="9" width="6" height="6"/><path d="M9 1v3M15 1v3M9 20v3M15 20v3M20 9h3M20 14h3M1 9h3M1 14h3"/>'),
    gauge: svg('<path d="m12 14 4-4"/><path d="M3.34 19a10 10 0 1 1 17.32 0"/>'),
    shield: svg('<path d="M20 13c0 5-3.5 7.5-7.66 8.95a1 1 0 0 1-.67-.01C7.5 20.5 4 18 4 13V6a1 1 0 0 1 1-1c2 0 4.5-1.2 6.24-2.72a1.17 1.17 0 0 1 1.52 0C14.51 3.81 17 5 19 5a1 1 0 0 1 1 1z"/>'),
    shieldOff: svg('<path d="m2 2 20 20"/><path d="M5 5a1 1 0 0 0-1 1v7c0 5 3.5 7.5 7.67 8.94a1 1 0 0 0 .67.01c2.35-.82 4.48-1.97 5.9-3.71"/><path d="M9.309 3.652A12.252 12.252 0 0 0 11.24 2.28a1.17 1.17 0 0 1 1.52 0C14.51 3.81 17 5 19 5a1 1 0 0 1 1 1v7a9.784 9.784 0 0 1-.08 1.264"/>'),
    chev: svg('<path d="m6 9 6 6 6-6"/>', 'chev'),
    lock: svg('<rect x="3" y="11" width="18" height="11" rx="2"/><path d="M7 11V7a5 5 0 0 1 10 0v4"/>', 'lock'),
    info: svg('<circle cx="12" cy="12" r="10"/><path d="M12 16v-4"/><path d="M12 8h.01"/>', ''),
    bell: svg('<path d="M6 8a6 6 0 0 1 12 0c0 7 3 9 3 9H3s3-2 3-9"/><path d="M10.3 21a1.94 1.94 0 0 0 3.4 0"/>', ''),
    bellRing: svg('<path d="M6 8a6 6 0 0 1 12 0c0 7 3 9 3 9H3s3-2 3-9"/><path d="M10.3 21a1.94 1.94 0 0 0 3.4 0"/><path d="M4 2C2.8 3.7 2 5.7 2 8"/><path d="M22 8c0-2.3-.8-4.3-2-6"/>', ''),
    zap: svg('<path d="M4 14a1 1 0 0 1-.78-1.63l9.9-10.2a.5.5 0 0 1 .86.46l-1.92 6.02A1 1 0 0 0 13 10h7a1 1 0 0 1 .78 1.63l-9.9 10.2a.5.5 0 0 1-.86-.46l1.92-6.02A1 1 0 0 0 11 14z"/>', '')
  };
  const fmtTok = n => n == null ? '?' : n >= 1000000 ? (n / 1000000).toFixed(n % 1000000 === 0 ? 1 : 2) + 'M' : n >= 1000 && n % 1000 === 0 ? (n / 1000) + 'K' : n.toLocaleString('en-US');
  const limits = () => S.role === 'child' ? { soft: 90000, hard: 150000, native: 1000000 } : { soft: 110000, hard: 180000, native: 1000000 };

  function meterHtml(opts) {
    const { cur, soft, hard, native, compact, label } = opts;
    const unknown = cur == null;
    const n1 = Math.round(soft + (hard - soft) / 3), n2 = Math.round(soft + (2 * (hard - soft)) / 3);
    const pct = unknown ? null : Math.round((cur / hard) * 100);
    const fillPct = unknown ? 0 : Math.min(100, Math.max(0, (cur / hard) * 100));
    const pSoft = (soft / hard) * 100, pN1 = (n1 / hard) * 100, pN2 = (n2 / hard) * 100;
    let cls = unknown ? 'unknown' : '', floatHtml = '';
    if (!unknown) {
      if (cur >= hard) { cls = 'crit'; floatHtml = `<span class="sb-float at-hard" style="--fc:var(--wb-fail)" title="Hard compaction (${fmtTok(hard)})">${IC.zap}</span>`; }
      else if (cur >= n2) { cls = 'hot'; floatHtml = `<span class="sb-float" style="left:${pN2.toFixed(1)}%;--fc:var(--wb-hot)" title="Nudge 2/3 (${fmtTok(n2)})">${IC.bellRing}</span>`; }
      else if (cur >= n1) { cls = 'warn'; floatHtml = `<span class="sb-float" style="left:${pN1.toFixed(1)}%;--fc:var(--wb-warn)" title="Nudge 1/3 (${fmtTok(n1)})">${IC.bell}</span>`; }
      else if (cur >= soft) { cls = 'warn'; floatHtml = `<span class="sb-float" style="left:${pSoft.toFixed(1)}%;--fc:var(--wb-warn)" title="Soft advisory (${fmtTok(soft)})">${IC.info}</span>`; }
    }
    const nativeSpan = native && native !== hard ? `<span class="sb-native"> (${fmtTok(native)})</span>` : '';
    const tip = `<div class="ctx-tip" role="tooltip"><div class="ctx-tip-grid">
      <span>Current</span><span>${unknown ? 'Unknown' : fmtTok(cur)}</span>
      <span>Soft advisory</span><span>${fmtTok(soft)}</span>
      <span>Nudge 1/3</span><span>${fmtTok(n1)}</span>
      <span>Nudge 2/3</span><span>${fmtTok(n2)}</span>
      <span>Hard compact</span><span>${fmtTok(hard)}</span>
      <span>Native window</span><span>${fmtTok(native || hard)}</span>
    </div></div>`;
    return `<div class="r-ctx${compact ? ' is-compact' : ''}">
      <div class="sb-ctx ${cls}" tabindex="0" aria-label="${label || 'Context usage'}">
        <span class="sb-pct">${unknown ? '?' : pct + '%'}</span>
        <div class="sb-bar-wrap">${floatHtml}<div class="sb-bar" aria-hidden="true">
          <i class="fill" style="width:${fillPct.toFixed(1)}%"></i>
          <u class="m-soft" style="left:${pSoft.toFixed(1)}%"></u>
          <u class="m-tick" style="left:${pN1.toFixed(1)}%"></u>
          <u class="m-tick" style="left:${pN2.toFixed(1)}%"></u>
        </div></div>
        <span class="sb-tok">${unknown ? '? / ' + fmtTok(hard) : fmtTok(cur) + ' / ' + fmtTok(hard)}${nativeSpan}</span>
      </div>${tip}
    </div>`;
  }

  function footerPillsHtml(hKey, role, interactive) {
    const h = HARNESS[hKey], sel = S.sel[hKey], isChild = role === 'child';
    const childTip = 'Child settings are fixed at spawn (read-only preview)';
    const slots = [
      { key: 'model', label: 'Model', icon: IC.cpu, val: sel.model, dis: isChild, tip: isChild ? childTip : 'Switch model', opts: h.models },
      { key: 'effort', label: 'Effort', icon: IC.gauge, val: sel.effort, dis: isChild, tip: isChild ? childTip : 'Switch effort', opts: h.efforts },
      { key: 'perm', label: 'Permission', icon: !h.permSupported ? IC.shieldOff : IC.shield, val: sel.perm, dis: isChild || !h.permSupported, piNone: !h.permSupported, tip: isChild ? childTip : (h.permSupported ? 'Switch permission mode' : h.permTip), opts: h.perms }
    ];
    return slots.map(sl => {
      const open = interactive && !sl.dis && S.openPop === sl.key;
      const tail = isChild ? IC.lock : (sl.piNone ? '' : IC.chev);
      return `<div class="fpill-wrap"><button type="button" class="fpill${sl.piNone ? ' pi-none' : ''}"${interactive ? ` data-pop="${sl.key}"` : ''} aria-expanded="${open}"${sl.dis ? ' disabled' : ''} title="${sl.tip}">${sl.icon}<span class="pv">${sl.val}</span>${tail}</button></div>`;
    }).join('');
  }

  function renderPop() {
    const pop = document.getElementById('fpop-portal');
    if (!S.openPop) { pop.hidden = true; pop.innerHTML = ''; return; }
    const btn = document.querySelector(`#primary-pills [data-pop="${S.openPop}"]`);
    const h = HARNESS[S.harness], map = { model: ['Model', h.models], effort: ['Effort', h.efforts], perm: ['Permission', h.perms] };
    const [title, opts] = map[S.openPop], cur = S.sel[S.harness][S.openPop];
    pop.innerHTML = `<div class="fpop-hd">${h.label} ${title}</div>` + opts.map(([v, sub]) => `<button type="button" class="fpop-opt" data-slot="${S.openPop}" data-val="${v}" role="option" aria-selected="${v === cur}"><span>${v}</span><span class="fpop-sub">${sub}</span></button>`).join('');
    pop.hidden = false;
    const r = btn.getBoundingClientRect(), pw = pop.offsetWidth, ph = pop.offsetHeight;
    pop.style.left = Math.max(8, Math.min(r.left, window.innerWidth - pw - 8)) + 'px';
    pop.style.top = Math.max(8, r.top - ph - 6) + 'px';
  }

  function renderPrimary() {
    const lim = limits(), shell = document.getElementById('primary-shell');
    shell.className = 'composer-shell' + (S.width === 'narrow' ? ' w-narrow' : S.width === 'rail' ? ' w-rail' : '');
    const isChild = S.role === 'child';
    document.getElementById('primary-ctx').innerHTML = meterHtml({ cur: S.tokens, ...lim, compact: S.width !== 'normal', label: 'Primary context meter' });
    document.getElementById('primary-pills').innerHTML = footerPillsHtml(S.harness, S.role, true);
    const ta = document.getElementById('primary-input'), send = document.getElementById('primary-send');
    ta.disabled = isChild; send.disabled = isChild;
    ta.placeholder = isChild ? 'Child preview — locked at spawn' : `Message ${HARNESS[S.harness].label} lead...`;
    document.getElementById('slider-val').textContent = `${fmtTok(S.tokens)} / ${fmtTok(lim.hard)}`;
    document.getElementById('stage-meta').innerHTML = `<span>Role: <strong>${isChild ? 'Child Preview (worker thresholds)' : 'Lead (editable)'}</strong></span><span>Soft: <strong>${fmtTok(lim.soft)}</strong></span><span>Hard: <strong>${fmtTok(lim.hard)}</strong></span><span>Native: <strong>${fmtTok(lim.native)}</strong></span>`;
    renderPop();
  }

  function renderGallery() {
    const states = [
      ['Normal', '28% — no floating icon', { cur: 51293, soft: 110000, hard: 180000, native: 1000000 }],
      ['Soft Crossed', '66% — info icon over Soft', { cur: 118800, soft: 110000, hard: 180000, native: 1000000 }],
      ['Nudge 1/3', '78% — bell over 1/3 tick', { cur: 140400, soft: 110000, hard: 180000, native: 1000000 }],
      ['Nudge 2/3', '91% — bell-ring over 2/3 tick', { cur: 163800, soft: 110000, hard: 180000, native: 1000000 }],
      ['Overflow', '105% — zap over Hard edge', { cur: 189000, soft: 110000, hard: 180000, native: 1000000 }],
      ['Hard == Native', '45% — no parenthetical', { cur: 90000, soft: 120000, hard: 200000, native: 200000 }],
      ['Unknown', '? — uninitialized', { cur: null, soft: 110000, hard: 180000, native: 1000000 }],
      ['Narrow 320px', '78% — native hidden, bell 1/3', { cur: 140400, soft: 110000, hard: 180000, native: 1000000, compact: true }]
    ];
    document.getElementById('state-gallery').innerHTML = states.map(([t, s, o]) => `<div class="srow"><div class="srow-lbl">${t}<small>${s}</small></div><div class="${o.compact ? 'srow-compact' : ''}">${meterHtml(o)}</div></div>`).join('');
    const frows = [['Claude', 'Lead editable', 'claude', 'lead'], ['Codex', 'Lead editable', 'codex', 'lead'], ['AGY', 'Lead editable', 'agy', 'lead'], ['Pi', 'Perm disabled + shield-off', 'pi', 'lead'], ['Child Preview', 'All 3 locked read-only', 'claude', 'child']];
    document.getElementById('foot-gallery').innerHTML = frows.map(([t, s, h, r]) => `<div class="frow"><div class="srow-lbl">${t}<small>${s}</small></div><div class="fpills">${footerPillsHtml(h, r, false)}</div></div>`).join('');
    document.getElementById('after-demo-ctx').innerHTML = meterHtml({ cur: 140400, soft: 110000, hard: 180000, native: 1000000 });
    document.getElementById('after-demo-pills').innerHTML = footerPillsHtml('claude', 'lead', false);
  }

  function syncButtons() {
    document.documentElement.dataset.theme = S.theme;
    document.querySelectorAll('[data-bind]').forEach(b => b.setAttribute('aria-pressed', String(S[b.dataset.bind] === b.dataset.val)));
  }

  document.addEventListener('click', e => {
    const b = e.target.closest('[data-bind]');
    if (b) { S[b.dataset.bind] = b.dataset.val; S.openPop = null; syncButtons(); renderPrimary(); return; }
    const popBtn = e.target.closest('[data-pop]');
    if (popBtn && !popBtn.disabled) { S.openPop = S.openPop === popBtn.dataset.pop ? null : popBtn.dataset.pop; renderPrimary(); return; }
    const opt = e.target.closest('.fpop-opt');
    if (opt) { S.sel[S.harness][opt.dataset.slot] = opt.dataset.val; S.openPop = null; renderPrimary(); return; }
    if (S.openPop && !e.target.closest('#fpop-portal') && !e.target.closest('.fpill-wrap')) { S.openPop = null; renderPrimary(); }
  });
  document.addEventListener('keydown', e => { if (e.key === 'Escape' && S.openPop) { S.openPop = null; renderPrimary(); } });
  document.getElementById('tok-slider').addEventListener('input', e => { S.tokens = Number(e.target.value); renderPrimary(); });
  window.addEventListener('resize', () => { if (S.openPop) renderPop(); });
  syncButtons(); renderPrimary(); renderGallery();
})();
