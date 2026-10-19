import { api, h, fill, fmt, debounce, seg, plName } from '../lib.js';

const shortMonths = ['Jan', 'Feb', 'Mär', 'Apr', 'Mai', 'Jun', 'Jul', 'Aug', 'Sep', 'Okt', 'Nov', 'Dez'];
const dayFmt = new Intl.DateTimeFormat('de-DE', { weekday: 'short', day: 'numeric', month: 'numeric' });
const monthFmt = new Intl.DateTimeFormat('de-DE', { month: 'long', year: 'numeric' });
let days = 14;

// columns draws the games per period as stacked bars: wins over losses
// over games without a result, with a tooltip each.
function columns(s) {
  const n = s.per_period.length;
  const max = Math.max(1, ...s.per_period.map(p => p.wins + p.losses + p.other));
  const px = v => Math.round(v / max * 168);
  const every = n > 14 ? 3 : 1;
  const cls = 'chart' + (n > 14 ? ' dense' : '');
  const cols = s.per_period.map((p, i) => {
    const d = new Date(p.start);
    const games = p.wins + p.losses + p.other;
    const lines = games ? [['Spiele', games], ['Bilanz', `${p.wins}W-${p.losses}L`], ['Siegquote', fmt.pct(p.wins, p.wins + p.losses)], p.other ? ['ohne Ergebnis', p.other] : null] : [['Nicht gespielt', '']];
    return h('div', { class: 'col', tabindex: 0, 'aria-label': `${s.unit === 'day' ? dayFmt.format(d) : monthFmt.format(d)}: ${p.wins} Siege, ${p.losses} Niederlagen` },
      h('div', { class: 'tip' + (i < 2 ? ' l' : i > n - 3 ? ' r' : ''), role: 'tooltip', style: { bottom: px(p.wins) + px(p.losses) + px(p.other) + 10 + 'px' } },
        h('b', null, s.unit === 'day' ? dayFmt.format(d) : monthFmt.format(d)),
        lines.filter(Boolean).map(([k, v]) => h('span', null, h('span', null, k), h('span', { class: 'num' }, v)))),
      [['w', p.wins], ['', p.losses], ['o', p.other]].map(([c, v]) => v ? h('span', { class: c, style: { height: px(v) + 'px' } }) : null));
  });
  return [h('div', { class: cls }, cols),
    h('div', { class: cls.replace('chart', 'chart-x') }, s.per_period.map((p, i) => h('span', null, i % every ? '' : s.unit === 'day' ? new Date(p.start).getDate() + '.' : shortMonths[new Date(p.start).getMonth()])))];
}

export async function mount(root, ctx) {
  const content = h('div', { style: { display: 'contents' } });
  root.append(h('div', { class: 'page', style: { gap: 'var(--space-6)' } },
    h('div', { class: 'head' }, h('h1', null, 'Statistik'),
      h('span', { class: 'end' }, seg([[7, '7 Tage'], [14, '14 Tage'], [30, 'Monat'], [0, 'Gesamt']], days, v => { days = v; load(); }))),
    content));

  function render(s) {
    const other = s.per_period.some(p => p.other > 0);
    const plMax = Math.max(1, ...s.playlists.map(p => p.games));
    fill(content,
      h('div', { class: 'kpis' },
        h('div', { class: 'kpi' }, h('b', null, s.games), 'Spiele'),
        h('div', { class: 'kpi' }, h('b', null, fmt.pct(s.wins, s.wins + s.losses)), 'Siegquote'),
        h('div', { class: 'kpi' }, h('b', null, s.wins, h('small', null, 'W'), '-', s.losses, h('small', null, 'L')), 'Bilanz')),
      s.games ? h('div', { class: 'stack' },
        h('div', { class: 'legend' }, h('span', null, h('i', { class: 'w' }), 'Siege'), h('span', null, h('i'), 'Niederlagen'),
          other ? h('span', null, h('i', { class: 'o' }), 'ohne Ergebnis') : null),
        columns(s)) : h('div', { class: 'empty' }, 'Keine Spiele in diesem Zeitraum.'),
      h('div', { class: 'duo' },
        h('div', null, h('div', { class: 'caps' }, 'Playlists'),
          s.playlists.length ? s.playlists.map(p => {
            const name = plName(p.id, p.name);
            const rest = p.games - p.wins - p.losses;
            return h('div', { class: 'bar-row', title: `${name}: ${p.wins} Siege, ${p.losses} Niederlagen` + (rest ? `, ${rest} ohne Ergebnis` : '') },
              h('span', { class: 'ell' }, name),
              h('span', { class: 'split', style: { width: Math.round(100 * p.games / plMax) + '%' } },
                [['w', p.wins], ['', p.losses], ['o', rest]].map(([c, v]) => v ? h('i', { class: c, style: { flex: v } }) : null)),
              h('span', { class: 'r n4 num' }, `${p.games}, ${fmt.pct(p.wins, p.wins + p.losses)}`));
          }) : h('div', { class: 'n4', style: { fontSize: '13px' } }, 'Keine Spiele.')),
        h('div', null, h('div', { class: 'caps' }, 'Beste Maps, ab 3 Spielen'),
          s.maps.length ? s.maps.map(m => {
            const rate = fmt.pct(m.wins, m.wins + m.losses);
            return h('div', { class: 'bar-row map', title: `${m.name}: ${m.wins} Siege, ${m.losses} Niederlagen` },
              h('span', { class: 'ell' }, m.name),
              h('span', { class: 'track rate' }, h('span', { class: 'me', style: { width: m.wins + m.losses ? rate : '0' } })),
              h('span', { class: 'r n4 num' }, `${rate}, ${m.games}`));
          }) : h('div', { class: 'n4', style: { fontSize: '13px' } }, 'Noch keine Map mit mindestens 3 Spielen.'))));
  }

  const load = async () => render(await api('/api/stats?days=' + days));

  await load();
  ctx.on('replays', debounce(load, 800));
  return { refresh: load };
}
