// The analysis of a folder: what ballchasing computes over its replays, per
// player as a total or an average per game.
import { h, fill, fmt, seg } from '../lib.js';

// columns per tab: [label, group, key, kind]. kind count (summed over the
// games: per game or in total), rate (an average either way), pct, wins
// or share (a part of the playing time, in total as a duration).
const tabs = [
  ['Kern', [['Siege', '', '', 'wins'], ['Punkte', 'core', 'score'], ['Tore', 'core', 'goals'], ['Vorlagen', 'core', 'assists'],
    ['Paraden', 'core', 'saves'], ['Schüsse', 'core', 'shots'], ['Schussquote', 'core', 'shooting_percentage', 'pct'], ['MVP', 'core', 'mvp']]],
  ['Boost', [['Pro Minute', 'boost', 'bpm', 'rate'], ['Ø Stand', 'boost', 'avg_amount', 'pct'], ['Gesammelt', 'boost', 'amount_collected'],
    ['Gestohlen', 'boost', 'amount_stolen'], ['Große Pads', 'boost', 'count_collected_big'], ['Kleine Pads', 'boost', 'count_collected_small'],
    ['Ohne Boost', 'boost', 'zero_boost', 'share'], ['Voller Boost', 'boost', 'full_boost', 'share']]],
  ['Bewegung', [['Ø Tempo km/h', 'movement', 'avg_speed', 'rate'], ['Strecke km', 'movement', 'total_distance'],
    ['Überschall', 'movement', 'supersonic_speed', 'share'], ['Am Boden', 'movement', 'ground', 'share'],
    ['Hoch in der Luft', 'movement', 'high_air', 'share'], ['Powerslides', 'movement', 'count_powerslide']]],
  ['Position', [['Ø Abstand zum Ball m', 'positioning', 'avg_distance_to_ball', 'rate'], ['Eigenes Drittel', 'positioning', 'defensive_third', 'share'],
    ['Gegnerisches Drittel', 'positioning', 'offensive_third', 'share'], ['Hinter dem Ball', 'positioning', 'behind_ball', 'share'],
    ['Als Hinterster', 'positioning', 'most_back', 'share'], ['Am nächsten am Ball', 'positioning', 'closest_to_ball', 'share'],
    ['Gegentore als Letzter', 'positioning', 'goals_against_while_last_defender']]],
  ['Demos', [['Verteilt', 'demo', 'inflicted'], ['In gegnerischer Hälfte', 'demo', 'inflicted_in_offensive_half'],
    ['Kassiert', 'demo', 'taken'], ['In eigener Hälfte', 'demo', 'taken_in_defensive_half']]],
];

let tab = 0;
let total = false; // totals instead of averages per game
let everyone = false; // also players with a single game
let sortBy = -1; // column, -1 for games with you on top
let asc = false;

const dec = (x, k = 1) => (Math.round(x * 10 ** k) / 10 ** k).toLocaleString('de-DE', { maximumFractionDigits: k });
const hms = s => { const t = Math.round(s); return t >= 3600 ? `${Math.floor(t / 3600)}:${String(Math.floor(t / 60) % 60).padStart(2, '0')}:${String(t % 60).padStart(2, '0')}` : fmt.dur(t); };

// in display units, as on the replay page
function scale(key, v) {
  if (key === 'avg_speed') return v * 0.036;
  if (key === 'total_distance') return v / 100000;
  if (key.startsWith('avg_distance')) return v / 100;
  return v;
}

function value(p, [, group, key, kind = 'count']) {
  const c = p.cumulative || {};
  const a = p.game_average || {};
  if (kind === 'wins') return c.games ? 100 * c.wins / c.games : undefined;
  if (kind === 'share') {
    const t = c[group]?.['time_' + key];
    if (total) return t;
    return typeof t === 'number' && c.play_duration ? 100 * t / c.play_duration : a[group]?.['percent_' + key];
  }
  const v = (kind === 'count' && total ? c : a)[group]?.[key];
  return typeof v === 'number' ? scale(key, v) : undefined;
}

function show(v, [, , key, kind = 'count']) {
  if (v === undefined) return '–';
  if (kind === 'wins' || kind === 'pct' || kind === 'share' && !total) return dec(v) + '%';
  if (kind === 'share') return hms(v);
  return dec(v, total && kind === 'count' && !['total_distance'].includes(key) ? 0 : 1);
}

// render fills el with the analysis. data is the reply of
// /api/groups/{id}/stats, me the IDs of the user ("steam:…"), replays the
// number of replays in the folder and reload asks ballchasing again.
export function render(el, data, { me = [], replays = 0, reload }) {
  const g = data.group || {};
  const mine = new Set(me);
  const players = (g.players || []).map(p => ({ ...p, me: mine.has(p.platform + ':' + p.id) }));
  const you = players.find(p => p.me);
  const games = p => p.cumulative?.games || 0;
  const few = players.filter(p => !p.me && games(p) < 2).length;
  const many = players.length > 12 && few > 0;
  const shown = players.filter(p => everyone || !many || p.me || games(p) >= 2);
  const cols = tabs[tab][1];
  const sorted = shown.slice().sort((a, b) => {
    if (sortBy < 0) return (b.me - a.me) || games(b) - games(a) || a.name.localeCompare(b.name, 'de');
    const x = value(a, cols[sortBy]) ?? -Infinity;
    const y = value(b, cols[sortBy]) ?? -Infinity;
    return asc ? x - y : y - x;
  });
  const redraw = () => render(el, data, { me, replays, reload });
  const sortOn = i => () => { asc = sortBy === i && i >= 0 && !asc; sortBy = i; redraw(); };
  const head = (label, i, cls) => h('th', { class: cls, 'aria-sort': sortBy === i ? (asc ? 'ascending' : 'descending') : null },
    h('button', { class: 'sort', onclick: sortOn(i), title: 'Sortieren' }, label));

  const c = you?.cumulative;
  fill(el,
    g.status === 'pending_replays' ? h('div', { class: 'note' }, 'ballchasing wertet noch Replays in diesem Ordner aus, die Zahlen können sich noch ändern. ',
      h('button', { class: 'link', onclick: reload }, 'Neu laden')) : null,
    g.failed_replays?.length ? h('div', { class: 'note warn' }, `${g.failed_replays.length} Replays konnte ballchasing nicht auswerten, sie fehlen in den Zahlen.`) : null,
    h('div', { class: 'kpis' }, c ? [
      h('div', { class: 'kpi' }, h('b', null, c.games), c.games === 1 ? 'Spiel' : 'Spiele'),
      h('div', { class: 'kpi' }, h('b', null, fmt.pct(c.wins, c.games)), 'Siegquote'),
      h('div', { class: 'kpi' }, h('b', null, c.wins, h('small', null, 'W'), '-', c.games - c.wins, h('small', null, 'L')), 'Bilanz'),
      c.play_duration ? h('div', { class: 'kpi' }, h('b', null, hms(c.play_duration)), 'Spielzeit') : null,
    ] : [
      h('div', { class: 'kpi' }, h('b', null, replays), 'Replays'),
      h('div', { class: 'kpi' }, h('b', null, players.length), 'Spieler'),
    ]),
    players.length ? [
      h('div', { class: 'tabs' },
        tabs.map(([label], i) => h('button', { class: tab === i ? 'on' : '', 'aria-pressed': String(tab === i), onclick: () => { tab = i; sortBy = -1; redraw(); } }, label)),
        h('span', { class: 'end actions' },
          seg([[false, 'Ø pro Spiel'], [true, 'Gesamt']], total, v => { total = v; redraw(); }, true),
          many ? seg([[false, 'ab 2 Spielen'], [true, `alle ${players.length}`]], everyone, v => { everyone = v; redraw(); }, true) : null)),
      h('div', { class: 'xscroll' }, h('table', { class: 'table gstats' },
        h('thead', null, h('tr', null, h('th', null, 'Spieler'), head('Spiele', -1, 'r'), cols.map((col, i) => head(col[0], i, 'r')))),
        h('tbody', null, sorted.map(p => h('tr', { class: p.me ? 'sel' : '' },
          h('td', null, h('div', { class: 'ell', title: `${p.name} (${p.platform})` }, p.name)),
          h('td', { class: 'r num n4' }, games(p)),
          cols.map(col => h('td', { class: 'r num' }, show(value(p, col), col)))))))),
      h('div', { class: 'sm n5' }, `Berechnet von ballchasing, Stand ${fmt.clock(data.fetched)}. `,
        h('button', { class: 'link', onclick: reload }, 'Neu laden')),
    ] : h('div', { class: 'empty' }, 'In diesem Ordner gibt es noch nichts auszuwerten.'));
}
