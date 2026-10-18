import { api, h, fill, svg, fmt, debounce } from '../lib.js';

export const title = 'Statistik';

const series = [
  ['wins', 'Siege', 'win'],
  ['losses', 'Niederlagen', 'loss'],
  ['other', 'ohne Ergebnis', 'none'],
];

// tooltip shows one shared tip next to the pointer or the focused mark.
const tip = h('div', { class: 'tip', hidden: true, role: 'tooltip' });
function showTip(anchor, titleText, rows, ev) {
  if (!tip.isConnected) document.body.append(tip);
  fill(tip, h('div', { class: 't' }, titleText),
    ...rows.map(([value, label, cls]) => h('div', { class: 'r' },
      cls ? h('span', { class: 'key', style: { background: `var(--${cls})` } }) : null, h('b', null, value), h('span', { class: 'muted' }, label))));
  tip.hidden = false;
  const r = anchor.getBoundingClientRect();
  const x = ev && ev.clientX ? ev.clientX : r.left + r.width / 2;
  const y = ev && ev.clientY ? ev.clientY : r.top;
  const w = tip.offsetWidth, th = tip.offsetHeight;
  tip.style.left = Math.max(8, Math.min(x + 14, innerWidth - w - 8)) + 'px';
  tip.style.top = Math.max(8, y - th - 12 < 8 ? y + 18 : y - th - 12) + 'px';
}
const hideTip = () => { tip.hidden = true; };

function hoverable(el, titleText, rows) {
  el.setAttribute('tabindex', '0');
  el.addEventListener('pointermove', e => showTip(el, titleText, rows, e));
  el.addEventListener('pointerleave', hideTip);
  el.addEventListener('focus', () => showTip(el, titleText, rows));
  el.addEventListener('blur', hideTip);
  return el;
}

// niceTicks returns clean integer ticks from 0 up to at least max.
function niceTicks(max) {
  const raw = max / 4;
  const pow = 10 ** Math.floor(Math.log10(Math.max(raw, 1)));
  const step = Math.max(1, [1, 2, 5, 10].map(m => m * pow).find(s => s >= raw));
  const ticks = [];
  for (let v = 0; v <= max + step - 1; v += step) ticks.push(v);
  if (ticks[ticks.length - 1] < max) ticks.push(ticks[ticks.length - 1] + step);
  return ticks;
}

// topRounded draws a bar with a 4px rounded top and a square base.
function barPath(x, y, w, hgt, rounded) {
  if (!rounded || hgt < 5) return `M${x},${y}h${w}v${hgt}h${-w}Z`;
  const r = Math.min(4, w / 2);
  return `M${x},${y + hgt}V${y + r}Q${x},${y} ${x + r},${y}H${x + w - r}Q${x + w},${y} ${x + w},${y + r}V${y + hgt}Z`;
}

// columnChart draws stacked columns of wins, losses and games without a
// result per period, sized to the container.
function columnChart(container, periods, unit) {
  const W = Math.max(320, container.clientWidth || 760);
  const H = 240, padL = 34, padR = 8, padT = 10, padB = 26;
  const totals = periods.map(p => p.wins + p.losses + p.other);
  const ticks = niceTicks(Math.max(1, ...totals));
  const top = ticks[ticks.length - 1];
  const band = (W - padL - padR) / Math.max(1, periods.length);
  const bw = Math.max(3, Math.min(24, band * 0.62));
  const y = v => padT + (H - padT - padB) * (1 - v / top);
  const every = Math.ceil(periods.length / Math.max(1, Math.floor((W - padL) / 56)));
  const root = svg('svg', { viewBox: `0 0 ${W} ${H}`, width: W, height: H, role: 'img',
    'aria-label': `Spiele pro ${unit === 'day' ? 'Tag' : 'Monat'}: Siege und Niederlagen` });
  root.append(svg('g', { class: 'chart-grid' }, ticks.map(t => svg('line', { x1: padL, x2: W - padR, y1: y(t), y2: y(t) }))));
  root.append(svg('g', null, ticks.map(t => svg('text', { x: padL - 6, y: y(t) + 4, 'text-anchor': 'end', class: 'num' }, String(t)))));
  periods.forEach((p, i) => {
    const x = padL + band * i + (band - bw) / 2;
    const g = svg('g', { class: 'col', 'aria-label': `${p.label}: ${p.wins} Siege, ${p.losses} Niederlagen` + (p.other ? `, ${p.other} ohne Ergebnis` : '') });
    g.append(svg('rect', { class: 'hit', x: padL + band * i, y: padT, width: band, height: H - padT - padB, fill: 'transparent' }));
    let base = 0;
    const parts = series.filter(([k]) => p[k] > 0);
    parts.forEach(([k, , cls], j) => {
      const y0 = y(base), y1 = y(base + p[k]);
      const gap = j > 0 ? 2 : 0; // 2px surface gap between stacked segments
      const hgt = Math.max(1, y0 - y1 - gap);
      g.append(svg('path', { class: 'mark mark-' + cls, d: barPath(x, y1, bw, hgt, j === parts.length - 1) }));
      base += p[k];
    });
    hoverable(g, p.label, series.map(([k, label, cls]) => [p[k], label, cls]).filter(([v], j) => v > 0 || j < 2));
    root.append(g);
    if (i % every === 0) root.append(svg('text', { x: padL + band * i + band / 2, y: H - 8, 'text-anchor': 'middle' }, p.label));
  });
  root.append(svg('line', { class: 'chart-axis', x1: padL, x2: W - padR, y1: y(0) + 0.5, y2: y(0) + 0.5 }));
  fill(container, root);
}

function legend(withOther) {
  return h('div', { class: 'legend' }, series.filter(([k]) => withOther || k !== 'other').map(([, label, cls]) =>
    h('span', null, h('i', { class: 'swatch', style: { background: `var(--${cls})` } }), label)));
}

function tableToggle(build) {
  const box = h('div', { hidden: true, style: { marginTop: '12px', overflowX: 'auto' } });
  const btn = h('button', { class: 'small ghost', onclick: () => {
    box.hidden = !box.hidden;
    btn.textContent = box.hidden ? 'Als Tabelle' : 'Tabelle ausblenden';
    if (!box.hidden) fill(box, build());
  } }, 'Als Tabelle');
  return [btn, box];
}

const rate = t => t.wins + t.losses > 0 ? t.wins / (t.wins + t.losses) : 0;

export async function mount(root, ctx) {
  let days = Number((() => { try { return sessionStorage.getItem('statistik-tage'); } catch { return null; } })() ?? 7);
  if (![7, 14, 30, 0].includes(days)) days = 7;
  const seg = h('div', { class: 'seg', role: 'group', 'aria-label': 'Zeitraum' });
  const content = h('div', { class: 'stack' });
  root.append(h('div', { class: 'row' }, seg), content);
  let data = null;
  let chartBox = null;

  function renderSeg() {
    fill(seg, ...[[7, '7 Tage'], [14, '14 Tage'], [30, 'Monat'], [0, 'Gesamt']].map(([d, label]) =>
      h('button', { class: days === d ? 'active' : '', 'aria-pressed': String(days === d), onclick: () => {
        days = d;
        try { sessionStorage.setItem('statistik-tage', String(d)); } catch { /* private mode */ }
        renderSeg();
        load();
      } }, label)));
  }

  function render() {
    const s = data;
    const decided = s.wins + s.losses;
    const withOther = s.per_period.some(p => p.other > 0);
    chartBox = h('div', { class: 'chart' });
    const [periodBtn, periodTable] = tableToggle(() => h('table', { class: 'stats' },
      h('thead', null, h('tr', null, ['Zeitraum', 'Siege', 'Niederlagen', 'ohne Ergebnis'].map(t => h('th', null, t)))),
      h('tbody', null, s.per_period.map(p => h('tr', null, h('td', null, p.label), h('td', { class: 'num' }, p.wins), h('td', { class: 'num' }, p.losses), h('td', { class: 'num' }, p.other))))));
    const maxGames = Math.max(1, ...s.playlists.map(p => p.games));
    fill(content, 
      h('div', { class: 'grid grid-4' },
        h('div', { class: 'card kpi' }, h('div', { class: 's' }, 'Spiele'), h('div', { class: 'v' }, s.games), h('div', { class: 's' }, 'mit dir im Zeitraum')),
        h('div', { class: 'card kpi' }, h('div', { class: 's' }, 'Siegquote'), h('div', { class: 'v' }, fmt.pct(s.wins, decided)), h('div', { class: 's' }, `aus ${decided} entschiedenen Spielen`)),
        h('div', { class: 'card kpi' }, h('div', { class: 's' }, 'Bilanz'), h('div', { class: 'v' }, `${s.wins}–${s.losses}`), h('div', { class: 's' }, 'Siege – Niederlagen')),
        h('div', { class: 'card kpi' }, h('div', { class: 's' }, 'Uploads'), h('div', { class: 'v' }, s.uploads), h('div', { class: 's' }, 'von deinem Konto'))),
      h('div', { class: 'card' },
        h('div', { class: 'card-head' }, h('h2', null, s.unit === 'day' ? 'Spiele pro Tag' : 'Spiele pro Monat'), legend(withOther)),
        s.games ? chartBox : h('div', { class: 'empty' }, 'Keine Spiele in diesem Zeitraum.'),
        h('div', { class: 'row' }, periodBtn), periodTable),
      h('div', { class: 'grid grid-2' },
        h('div', { class: 'card' },
          h('div', { class: 'card-head' }, h('h2', null, 'Nach Playlist'), h('span', { class: 'muted' }, 'Spiele · Siegquote')),
          legend(s.playlists.some(p => p.games > p.wins + p.losses)),
          s.playlists.length ? h('div', null, s.playlists.map(p => {
            const other = p.games - p.wins - p.losses;
            const row = h('div', { class: 'hbar', 'aria-label': `${p.name}: ${p.games} Spiele, ${p.wins} Siege, ${p.losses} Niederlagen` },
              h('span', { class: 'ellipsis' }, p.name),
              h('div', { class: 'track', style: { width: Math.max(4, 100 * p.games / maxGames) + '%', gap: '2px', background: 'none' } },
                [['wins', 'win'], ['losses', 'loss']].map(([k, cls]) => p[k] ? h('i', { style: { flex: p[k], background: `var(--${cls})`, borderRadius: '2px' } }) : null),
                other ? h('i', { style: { flex: other, background: 'var(--none)', borderRadius: '2px' } }) : null),
              h('span', { class: 'num muted nowrap' }, `${p.games} · ${fmt.pct(p.wins, p.wins + p.losses)}`));
            return hoverable(row, p.name, [[p.games, 'Spiele'], [p.wins, 'Siege', 'win'], [p.losses, 'Niederlagen', 'loss'], [fmt.pct(p.wins, p.wins + p.losses), 'Siegquote']]);
          })) : h('div', { class: 'empty' }, 'Keine Spiele.')),
        h('div', { class: 'card' },
          h('div', { class: 'card-head' }, h('h2', null, 'Beste Maps'), h('span', { class: 'muted' }, 'Siegquote, ab 3 Spielen')),
          s.maps.length ? h('div', null, s.maps.map(m => {
            const r = rate(m);
            const row = h('div', { class: 'hbar', 'aria-label': `${m.name}: Siegquote ${Math.round(100 * r)} Prozent aus ${m.games} Spielen` },
              h('span', { class: 'ellipsis' }, m.name),
              h('div', { class: 'track' }, h('i', { style: { width: Math.max(2, 100 * r) + '%', background: 'var(--win)', borderRadius: '0 4px 4px 0' } })),
              h('span', { class: 'num muted nowrap' }, `${Math.round(100 * r)} % · ${m.games}`));
            return hoverable(row, m.name, [[Math.round(100 * r) + ' %', 'Siegquote'], [m.wins, 'Siege', 'win'], [m.losses, 'Niederlagen', 'loss'], [m.games, 'Spiele']]);
          })) : h('div', { class: 'empty' }, 'Noch keine Map mit mindestens 3 Spielen.'))));
    if (s.games) requestAnimationFrame(() => columnChart(chartBox, s.per_period, s.unit));
  }

  async function load() {
    content.style.opacity = data ? '.55' : '';
    data = await api('/api/stats?days=' + days);
    content.style.opacity = '';
    render();
  }

  renderSeg();
  await load();
  const onResize = debounce(() => { if (data?.games && chartBox?.isConnected) columnChart(chartBox, data.per_period, data.unit); }, 150);
  addEventListener('resize', onResize);
  ctx.on('replays', debounce(load, 800));
  return { refresh: load, destroy: () => { removeEventListener('resize', onResize); hideTip(); } };
}
