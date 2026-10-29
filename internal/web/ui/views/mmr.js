// MMR (developer mode): the current lobby with ranks and wins, how often a
// smurf was in the lobby, the user's MMR over the matches and where the
// opponents stood against it.
import { api, h, ico, fill, fmt, run, seg, debounce, plName } from '../lib.js';

const tierNames = ['Unranked', 'Bronze I', 'Bronze II', 'Bronze III', 'Silber I', 'Silber II', 'Silber III', 'Gold I', 'Gold II', 'Gold III',
  'Platin I', 'Platin II', 'Platin III', 'Diamant I', 'Diamant II', 'Diamant III', 'Champion I', 'Champion II', 'Champion III',
  'Grand Champion I', 'Grand Champion II', 'Grand Champion III', 'Supersonic Legend'];

// tierShort is a rank as trackers abbreviate it: C2 is Champion II.
function tierShort(t) {
  if (t <= 0) return 'Unr.';
  if (t >= 22) return 'SSL';
  if (t >= 19) return 'GC' + (t - 18);
  return 'BSGPDC'[Math.floor((t - 1) / 3)] + ((t - 1) % 3 + 1);
}
const tierClass = t => t > 0 ? 'tier-' + Math.min(8, Math.floor((t - 1) / 3) + 1) : '';
const sizeName = n => n ? `${n}v${n}` : '–';
const signed = v => (v > 0 ? '+' : v < 0 ? '−' : '±') + Math.round(Math.abs(v));
let size = 2;
let days = 30;

// rankCell shows a rank as a colored square, its short name and the MMR.
function rankCell(r, looked) {
  if (!r) return h('span', { class: 'n5' }, looked ? '–' : '…');
  return h('span', { class: 'rank ' + tierClass(r.tier), title: `${tierNames[r.tier] || r.tier}, Division ${['I', 'II', 'III', 'IV'][r.div] || r.div + 1}, ${r.played} Spiele` },
    h('i'), tierShort(r.tier), h('small', null, Math.round(r.mmr)));
}

// lobbyTable lists the players of a lobby by team, the user highlighted.
function lobbyTable(l, me) {
  const isMe = p => p.me || me.includes(pidOf(p.id));
  const team = (n, label) => {
    const rows = l.players.filter(p => p.team === n).sort((a, b) => isMe(b) - isMe(a) || a.left - b.left);
    if (!rows.length) return null;
    return [h('tr', { class: 'team' + (n ? ' orange' : '') }, h('th', { colspan: 5 }, label)),
      rows.map(p => h('tr', { class: p.left ? 'left' : '', title: p.left ? 'Hat das Match verlassen' : null },
        h('td', { class: 'ell' + (isMe(p) ? ' me' : ''), style: { maxWidth: '220px' } }, p.name),
        p.ranks.map(r => h('td', null, rankCell(r, p.looked))),
        h('td', { class: 'r n4 num' }, p.wins >= 0 ? fmt.num(p.wins) : '–')))];
  };
  return h('table', { class: 'table lobby' },
    h('thead', null, h('tr', null, h('th', null, 'Spieler'), h('th', null, '1s'), h('th', null, '2s'), h('th', null, '3s'), h('th', { class: 'r' }, 'Siege'))),
    h('tbody', null, team(0, 'Blau'), team(1, 'Orange')));
}

// pidOf turns Steam|7656…|0 into steam:7656…, as the user's accounts are
// written.
function pidOf(id) {
  const [platform, rest] = id.split('|');
  const p = platform.toLowerCase();
  return (p === 'xboxone' ? 'xbox' : p) + ':' + rest;
}

const smurfButton = (l, done) => h('button', { class: 'btn ' + (l.smurf ? 'btn-primary' : 'btn-secondary') + ' sm', 'aria-pressed': String(l.smurf),
  title: l.smurf ? 'Markierung zurücknehmen' : 'Smurf in dieser Lobby markieren',
  onclick: async e => {
    e.stopPropagation();
    if (await run(() => api(`/api/lobbies/${encodeURIComponent(l.guid)}/smurf`, { method: 'POST', body: { smurf: !l.smurf } }))) done();
  } }, l.smurf ? [ico('check'), 'Smurf'] : 'Kein Smurf');

const svgNS = 'http://www.w3.org/2000/svg';
function svg(tag, attrs, ...kids) {
  const el = document.createElementNS(svgNS, tag);
  for (const [k, v] of Object.entries(attrs || {})) if (v != null) el.setAttribute(k, v);
  for (const c of kids.flat(Infinity)) if (c != null) el.append(c instanceof Node ? c : document.createTextNode(String(c)));
  return el;
}

// niceStep is a round tick distance for a range shown with about n ticks.
function niceStep(range, n) {
  const raw = range / n;
  const mag = 10 ** Math.floor(Math.log10(raw));
  return [1, 2, 2.5, 5, 10].map(f => f * mag).find(s => s >= raw) || mag * 10;
}

// lineChart draws the user's MMR and the opponents' average per match, one
// axis for both, with a crosshair and tooltip on hover.
function lineChart(points) {
  const box = h('div', { class: 'lchart' });
  const tip = h('div', { class: 'tip', role: 'tooltip' });
  const draw = () => {
    const W = box.clientWidth || 800, H = 220;
    const pad = { l: 44, r: 92, t: 10, b: 24 };
    const vals = points.flatMap(p => [p.me, p.opp]).filter(v => v != null);
    let lo = Math.min(...vals), hi = Math.max(...vals);
    if (hi - lo < 40) { lo -= 20; hi += 20; }
    const step = niceStep(hi - lo, 4);
    lo = Math.floor(lo / step) * step;
    hi = Math.ceil(hi / step) * step;
    const n = points.length;
    const x = i => pad.l + (n === 1 ? (W - pad.l - pad.r) / 2 : i * (W - pad.l - pad.r) / (n - 1));
    const y = v => pad.t + (hi - v) / (hi - lo) * (H - pad.t - pad.b);
    const path = key => {
      let d = '';
      let pen = false;
      points.forEach((p, i) => {
        if (p[key] == null) { pen = false; return; }
        d += (pen ? 'L' : 'M') + x(i).toFixed(1) + ' ' + y(p[key]).toFixed(1);
        pen = true;
      });
      return d;
    };
    const ticks = [];
    for (let v = lo; v <= hi + 1e-9; v += step) ticks.push(v);
    const dateFmt = new Intl.DateTimeFormat('de-DE', { day: 'numeric', month: 'numeric' });
    const xTicks = [...new Set([0, Math.round((n - 1) / 2), n - 1])];
    const lastOf = key => { for (let i = n - 1; i >= 0; i--) if (points[i][key] != null) return i; return -1; };
    const ends = [['me', 'Du'], ['opp', 'Gegner Ø']].map(([key, label]) => ({ key, label, i: lastOf(key) })).filter(e => e.i >= 0);
    // keep the two end labels apart
    const ey = ends.map(e => y(points[e.i][e.key]));
    if (ey.length === 2 && Math.abs(ey[0] - ey[1]) < 14) {
      const mid = (ey[0] + ey[1]) / 2, up = ey[0] <= ey[1] ? 0 : 1;
      ey[up] = mid - 7; ey[1 - up] = mid + 7;
    }
    const cross = svg('line', { class: 'cross', y1: pad.t, y2: H - pad.b, visibility: 'hidden' });
    const s = svg('svg', { viewBox: `0 0 ${W} ${H}`, 'aria-label': 'MMR pro Match: du und der Schnitt der Gegner', role: 'img' },
      ticks.map(v => [svg('line', { class: v === lo ? 'base' : 'grid', x1: pad.l, x2: W - pad.r, y1: y(v), y2: y(v) }),
        svg('text', { x: pad.l - 8, y: y(v) + 4, 'text-anchor': 'end' }, fmt.num(v))]),
      xTicks.map(i => svg('text', { x: x(i), y: H - 6, 'text-anchor': n === 1 ? 'middle' : i === 0 ? 'start' : i === n - 1 ? 'end' : 'middle' }, dateFmt.format(new Date(points[i].at)))),
      points.map((p, i) => p.smurf ? svg('rect', { class: 'smurf', x: x(i) - 1, y: H - pad.b + 3, width: 2, height: 5 }) : null),
      cross,
      svg('path', { class: 'line opp', d: path('opp') }),
      svg('path', { class: 'line me', d: path('me') }),
      ends.map((e, k) => [svg('circle', { class: e.key, cx: x(e.i), cy: y(points[e.i][e.key]), r: 4 }),
        svg('text', { class: 'end', x: W - pad.r + 10, y: ey[k] + 4 }, `${e.label} ${Math.round(points[e.i][e.key])}`)]));
    const hover = svg('g');
    s.append(hover);
    s.addEventListener('mousemove', ev => {
      const rect = s.getBoundingClientRect();
      const px = (ev.clientX - rect.left) * W / rect.width;
      const i = Math.max(0, Math.min(n - 1, Math.round(n === 1 ? 0 : (px - pad.l) / ((W - pad.l - pad.r) / (n - 1)))));
      const p = points[i];
      cross.setAttribute('x1', x(i));
      cross.setAttribute('x2', x(i));
      cross.setAttribute('visibility', 'visible');
      hover.replaceChildren(...[['me', p.me], ['opp', p.opp]].filter(([, v]) => v != null).map(([k, v]) => svg('circle', { class: k, cx: x(i), cy: y(v), r: 4 })));
      fill(tip, h('b', null, fmt.when(p.at)),
        h('span', null, h('span', null, h('i', { style: { background: 'var(--series-me)' } }), 'Du'), h('span', { class: 'num' }, Math.round(p.me))),
        p.opp != null ? h('span', null, h('span', null, h('i', { style: { background: 'var(--series-opp)' } }), 'Gegner Ø'), h('span', { class: 'num' }, `${Math.round(p.opp)} (${signed(p.opp - p.me)})`)) : null,
        p.mates != null ? h('span', null, h('span', null, 'Mitspieler Ø'), h('span', { class: 'num' }, Math.round(p.mates))) : null,
        h('span', null, h('span', null, 'Ergebnis'), h('span', null, ['Niederlage', 'Sieg'][p.result] || 'unbekannt')),
        p.smurf ? h('span', null, h('span', null, 'Smurf in der Lobby'), h('span', null, 'ja')) : null);
      tip.classList.add('on');
      const left = x(i) / W * rect.width;
      tip.style.left = Math.min(Math.max(0, left + 12), rect.width - tip.offsetWidth) + 'px';
    });
    s.addEventListener('mouseleave', () => { tip.classList.remove('on'); cross.setAttribute('visibility', 'hidden'); hover.replaceChildren(); });
    box.replaceChildren(s, tip);
  };
  const redraw = debounce(draw, 100);
  let drawn = false;
  new ResizeObserver(() => { if (drawn) redraw(); else { drawn = true; draw(); } }).observe(box);
  return box;
}

export async function mount(root, ctx) {
  const content = h('div', { style: { display: 'contents' } });
  const page = h('div', { class: 'page mmr', style: { gap: 'var(--space-6)' } },
    h('div', { class: 'head' }, h('h1', null, 'MMR'),
      h('span', { class: 'end actions' },
        seg([[1, '1s'], [2, '2s'], [3, '3s']], size, v => { size = v; load(); }),
        seg([[7, '7 Tage'], [30, 'Monat'], [0, 'Gesamt']], days, v => { days = v; load(); }))),
    content);
  root.append(page);
  const open = new Set(); // lobbies unfolded in the list

  function render(st, cur, settings) {
    const epic = settings.epic || {};
    const rate = (a, b) => fmt.pct(a.wins, a.wins + a.losses);
    const first = st.points[0], last = st.points[st.points.length - 1];
    const l = cur.lobby;
    fill(content,
      !ctx.state.lobby ? h('div', { class: 'note warn' }, 'Die MMR-Abfrage ist aus. Einschalten unter ', h('a', { href: '#/einstellungen' }, 'Einstellungen → Entwickler'), '.') : null,
      ctx.state.lobby && epic.status !== 'ok' ? h('div', { class: 'note warn' },
        { none: 'Kein Epic-Account angemeldet', expired: 'Die Epic-Anmeldung ist abgelaufen', error: 'Die MMR-Abfrage hat gerade ein Problem: ' + epic.error }[epic.status] || 'Epic-Account unbekannt',
        '. Lobbys und Smurfs werden trotzdem erfasst, nur ohne Ränge. ', h('a', { href: '#/einstellungen' }, 'Zu den Einstellungen')) : null,
      st.lobbies && !st.me_known ? h('div', { class: 'note' }, 'Dein Spieler ist in keiner Lobby erkannt, deshalb fehlen eigene MMR und Ergebnis. Lege ihn unter ',
        h('a', { href: '#/einstellungen' }, 'Einstellungen → Dein Spieler'), ' fest.') : null,

      h('div', { class: 'stack' },
        h('div', { class: 'head tight' }, h('div', { class: 'caps', style: { margin: 0 } }, 'Aktuelle Lobby'),
          l ? h('span', { class: 'sm n4' }, `${sizeName(l.size)}, ${fmt.when(l.started)}`) : null,
          cur.busy ? h('span', { class: 'sm n4' }, 'Ränge werden geladen …') : null,
          cur.error ? h('span', { class: 'sm err' }, cur.error) : null,
          l ? h('span', { class: 'end' }, smurfButton(l, load)) : null),
        l ? lobbyTable(l, ctx.state.me || []) : h('div', { class: 'empty', style: { padding: 0 } }, 'Noch kein Online-Match seit dem Start. Die Lobby erscheint, sobald eins beginnt.')),

      h('div', { class: 'kpis' },
        h('div', { class: 'kpi' }, h('b', null, st.lobbies), `Lobbys ${sizeName(size)}`),
        h('div', { class: 'kpi' }, h('b', null, st.smurfs, h('small', null, ' ' + fmt.pct(st.smurfs, st.lobbies))), 'mit Smurf'),
        h('div', { class: 'kpi', title: 'Schnitt der Gegner minus deine MMR, über alle Matches mit Rängen' }, h('b', null, st.compared ? signed(st.opp_diff) : '–'), 'Gegner im Schnitt'),
        h('div', { class: 'kpi', title: 'Anteil der Matches, in denen die Gegner im Schnitt über dir lagen' }, h('b', null, fmt.pct(st.opp_above, st.compared)), 'Gegner stärker'),
        st.points.length > 1 ? h('div', { class: 'kpi', title: 'Veränderung vom ersten zum letzten Match im Zeitraum' }, h('b', null, signed(last.me - first.me)), 'deine MMR') : null),

      st.points.length ? h('div', { class: 'stack' },
        h('div', { class: 'head tight' }, h('div', { class: 'caps', style: { margin: 0 } }, `MMR pro Match, ${sizeName(size)}`),
          h('div', { class: 'legend end' }, h('span', null, h('i', { class: 'line me' }), 'Du'), h('span', null, h('i', { class: 'line opp' }), 'Gegner Ø'),
            st.smurfs ? h('span', null, h('i', { style: { width: '2px', height: '8px', background: 'var(--color-neutral-400)' } }), 'Smurf') : null)),
        lineChart(st.points))
        : h('div', { class: 'empty' }, st.lobbies ? 'Noch keine Lobby mit deiner MMR in diesem Zeitraum.' : `Noch keine ${sizeName(size)}-Lobby in diesem Zeitraum.`),

      h('div', { class: 'duo' },
        h('div', null, h('div', { class: 'caps' }, 'Siegquote'),
          [['Mit Smurf', st.smurf_record], ['Ohne Smurf', st.clean_record]].map(([label, wl]) => h('div', { class: 'bar-row', style: { gridTemplateColumns: '110px minmax(0, 1fr) 130px' }, title: `${wl.wins} Siege, ${wl.losses} Niederlagen` },
            h('span', null, label),
            h('span', { class: 'track rate' }, h('span', { class: 'me', style: { width: wl.wins + wl.losses ? rate(wl) : '0' } })),
            h('span', { class: 'r n4 num' }, `${rate(wl)}, ${wl.wins}W-${wl.losses}L`)))),
        h('div', null, h('div', { class: 'caps' }, 'Im Vergleich zu dir'),
          h('div', { class: 'bar-row' }, h('span', null, 'Gegner Ø'), h('span', { class: 'sm n4' }, st.compared ? `in ${st.compared} Matches` : 'noch keine Daten'),
            h('span', { class: 'r num' }, st.compared ? signed(st.opp_diff) + ' MMR' : '–')),
          h('div', { class: 'bar-row' }, h('span', null, 'Mitspieler Ø'), h('span', { class: 'sm n4' }, size === 1 ? 'keine im 1s' : ''),
            h('span', { class: 'r num' }, st.mate_diff != null ? signed(st.mate_diff) + ' MMR' : '–')))),

      st.recent.length ? h('div', { class: 'stack' },
        h('div', { class: 'caps' }, 'Lobbys'),
        h('table', { class: 'table' },
          h('thead', null, h('tr', null, h('th', null, 'Wann'), h('th', null, 'Playlist'), h('th', { class: 'c' }, 'Ergebnis'),
            h('th', { class: 'r' }, 'Du'), h('th', { class: 'r' }, 'Gegner Ø'), h('th', { class: 'r' }, 'Smurf'))),
          h('tbody', null, st.recent.map(row => {
            const self = row.players.find(p => p.me || (ctx.state.me || []).includes(pidOf(p.id)));
            const own = self?.ranks[size - 1];
            const opp = row.players.filter(p => self && p.team !== self.team && p.ranks[size - 1]?.tier > 0);
            const oppAvg = opp.length ? opp.reduce((a, p) => a + p.ranks[size - 1].mmr, 0) / opp.length : null;
            const unfolded = open.has(row.guid);
            const toggle = () => { unfolded ? open.delete(row.guid) : open.add(row.guid); render(st, cur, settings); };
            return [h('tr', { class: 'link' + (unfolded ? ' open' : ''), tabindex: 0, onclick: toggle, onkeydown: e => { if (e.key === 'Enter') toggle(); } },
              h('td', { class: 'nowrap' }, ico(unfolded ? 'caret-down' : 'caret-right', 11), ' ', fmt.when(row.started)),
              h('td', { class: 'n4' }, row.playlist ? plName(row.playlist) : sizeName(row.size)),
              h('td', { class: 'c' }, h('span', { class: 'res ' + (['loss', 'win'][row.result] || 'none'), style: { display: 'inline-grid' } }, ['N', 'S'][row.result] || '–')),
              h('td', { class: 'r' }, own ? rankCell(own, true) : '–'),
              h('td', { class: 'r num' }, oppAvg != null ? `${Math.round(oppAvg)}` : '–', own && oppAvg != null ? h('span', { class: 'n5' }, ` (${signed(oppAvg - own.mmr)})`) : null),
              h('td', { class: 'r' }, smurfButton(row, load))),
            unfolded ? h('tr', null, h('td', { class: 'sub', colspan: 6 }, lobbyTable(row, ctx.state.me || []))) : null];
          }))))
        : null);
  }

  const load = async () => {
    const [st, cur, settings] = await Promise.all([api(`/api/mmr?size=${size}&days=${days}`), api('/api/lobby'), api('/api/settings')]);
    render(st, cur, settings);
  };
  await load();
  ctx.on('lobby', debounce(load, 400));
  ctx.on('settings', debounce(load, 400));
  return { refresh: load };
}
