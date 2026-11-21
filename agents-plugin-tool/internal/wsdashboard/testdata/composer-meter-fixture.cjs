const fs = require('node:fs');
const path = require('node:path');
const { readAssetSource } = require('./asset-source.cjs');

const LUCIDE_DIR = path.resolve(__dirname, '../assets/lucide/icons');
const EN_TABLE = JSON.parse(fs.readFileSync(path.resolve(__dirname, '../../wsi18n/tables/en.json'), 'utf8'));

function lucideSpriteHtml() {
  const syms = fs.readdirSync(LUCIDE_DIR).filter(n => n.endsWith('.svg')).sort().map(n => {
    const raw = fs.readFileSync(path.join(LUCIDE_DIR, n), 'utf8');
    const body = raw.match(/<svg\b[^>]*>([\s\S]*)<\/svg>/)[1].replace(/\s*\n\s*/g, '').trim();
    return `<symbol id="l-${n.slice(0, -4)}" viewBox="0 0 24 24">${body}</symbol>`;
  });
  return `<svg width="0" height="0" style="position:absolute" aria-hidden="true">${syms.join('')}</svg>`;
}

const fmtTok = n => n == null ? '?' : n >= 1000000 ? (n / 1000000).toFixed(1) + 'M' : n >= 1000 && n % 1000 === 0 ? (n / 1000) + 'K' : n.toLocaleString('en-US');

function meterMarkup({ cur, soft, hard, native, cache }) {
  const denom = hard > 0 ? hard : native;
  const unknown = cur == null || !denom;
  const m1 = soft > 0 && hard > soft ? Math.round(soft + (hard - soft) / 3) : 0;
  const m2 = soft > 0 && hard > soft ? Math.round(soft + (2 * (hard - soft)) / 3) : 0;
  const exact = unknown ? 0 : (cur / denom) * 100;
  const pct = unknown ? null : Math.round(exact);
  const fillW = unknown ? '0.0' : Math.min(100, Math.max(0, exact)).toFixed(1);
  const pSoft = soft > 0 && denom > 0 ? ((soft / denom) * 100).toFixed(1) : '';
  const pM1 = m1 > 0 && denom > 0 ? ((m1 / denom) * 100).toFixed(1) : '';
  const pM2 = m2 > 0 && denom > 0 ? ((m2 / denom) * 100).toFixed(1) : '';
  let cls = unknown ? 'd-unknown' : '', floatHtml = '';
  if (!unknown && hard > 0) {
    if (cur >= hard) { cls = 'crit'; floatHtml = `<span class="sb-float at-hard" style="--fc:var(--wb-fail)" title="Hard compaction (${fmtTok(hard)})"><svg class="li" aria-hidden="true"><use href="#l-zap"/></svg></span>`; }
    else if (m2 && cur >= m2) { cls = 'd-hot'; floatHtml = `<span class="sb-float" style="left:${pM2}%;--fc:var(--wb-hot)" title="Nudge 2/3 (${fmtTok(m2)})"><svg class="li" aria-hidden="true"><use href="#l-bell-ring"/></svg></span>`; }
    else if (m1 && cur >= m1) { cls = 'warn'; floatHtml = `<span class="sb-float" style="left:${pM1}%;--fc:var(--wb-warn)" title="Nudge 1/3 (${fmtTok(m1)})"><svg class="li" aria-hidden="true"><use href="#l-bell"/></svg></span>`; }
    else if (soft && cur >= soft) { cls = 'warn'; floatHtml = `<span class="sb-float" style="left:${pSoft}%;--fc:var(--wb-warn)" title="Soft advisory (${fmtTok(soft)})"><svg class="li" aria-hidden="true"><use href="#l-info"/></svg></span>`; }
  }
  const natHtml = native && denom && native !== denom ? `<span class="sb-native"> (${fmtTok(native)})</span>` : '';
  const cacheHtml = cache ? `<span class="sb-cache" title="prompt cache hit rate of the last response">CH ${cache}</span>` : '';
  return `<div class="sb sb-top"><div class="sb-ctx${cls ? ' ' + cls : ''}" tabindex="0" role="meter" aria-label="context usage" aria-valuemin="0" aria-valuemax="100"${unknown ? '' : ` aria-valuenow="${Math.min(100, Math.max(0, pct))}"`}><span class="sb-pct">${unknown ? '?' : pct + '%'}</span><div class="sb-bar-wrap">${floatHtml}<div class="sb-bar" aria-hidden="true"><i class="fill" style="width: ${fillW}%"></i>${pSoft ? `<u class="m-soft" style="left: ${pSoft}%"></u>` : ''}${pM1 ? `<u class="m-tick" style="left: ${pM1}%"></u>` : ''}${pM2 ? `<u class="m-tick" style="left: ${pM2}%"></u>` : ''}</div></div><span class="sb-tok">${unknown ? '? / ' + fmtTok(denom) : cur.toLocaleString('en-US') + ' / ' + fmtTok(denom)}${natHtml}</span>${cacheHtml}</div><div class="ctx-tip" role="tooltip"><div class="ctx-tip-grid"><span>Current</span><span>${unknown ? 'Unknown' : cur.toLocaleString('en-US')}</span>${soft ? `<span>Soft advisory</span><span>${fmtTok(soft)}</span>` : ''}${m1 ? `<span>Nudge 1/3</span><span>${fmtTok(m1)}</span><span>Nudge 2/3</span><span>${fmtTok(m2)}</span>` : ''}${hard ? `<span>Hard compact</span><span>${fmtTok(hard)}</span>` : ''}<span>Native window</span><span>${fmtTok(native || denom)}</span></div></div></div>`;
}

function pillsMarkup({ model, effort, isChild, canEdit }) {
  const tailIcon = isChild ? '<svg class="i li lock" aria-hidden="true"><use href="#l-lock"/></svg>' : '<svg class="i li chev" aria-hidden="true"><use href="#l-chevron-down"/></svg>';
  const dis = !canEdit ? ' disabled' : '';
  const mTip = isChild ? EN_TABLE['js.lead.controls.child'] : (!canEdit ? EN_TABLE['js.lead.controls.host'] : EN_TABLE['js.lead.controls.model']);
  const eTip = isChild ? EN_TABLE['js.lead.controls.child'] : (!canEdit ? EN_TABLE['js.lead.controls.host'] : EN_TABLE['js.lead.controls.effort']);
  return `<div class="fpills" id="composer-pills"><div class="fpill-wrap"><button type="button" class="fpill" data-pop="model" aria-haspopup="listbox" aria-expanded="false"${dis} title="${mTip}"><svg class="i li ico" aria-hidden="true"><use href="#l-cpu"/></svg><span class="pv">${model || 'default'}</span>${tailIcon}</button></div><div class="fpill-wrap"><button type="button" class="fpill" data-pop="effort" aria-haspopup="listbox" aria-expanded="false"${dis} title="${eTip}"><svg class="i li ico" aria-hidden="true"><use href="#l-gauge"/></svg><span class="pv">${effort || 'default'}</span>${tailIcon}</button></div></div>`;
}

function assetBundle() {
  const css = [
    readAssetSource('wswhiteboard', 'page.css'),
    readAssetSource('wsdashboard', 'dashboard.css'),
    readAssetSource('wsdashboard', 'usage.css'),
  ].join('\n');
  return {
    css,
    sprite: lucideSpriteHtml(),
    en: EN_TABLE,
    railJs: readAssetSource('wsdashboard', 'rail.js'),
    usageJs: readAssetSource('wsdashboard', 'usage.js'),
    dashJs: readAssetSource('wsdashboard', 'dashboard.js'),
  };
}

module.exports = { assetBundle, meterMarkup, pillsMarkup, EN_TABLE };
