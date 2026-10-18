import { api, h, fill, fmt, menu, replayActions, playerTable, resultBadge, scoreLine, visibilityNames } from '../lib.js';

export const title = 'Replay';

const groups = [
  ['core', 'Kern', {
    score: 'Punkte', goals: 'Tore', assists: 'Vorlagen', saves: 'Paraden', shots: 'Schüsse', shooting_percentage: 'Schussquote',
    goals_against: 'Gegentore', shots_against: 'Schüsse aufs eigene Tor', epic_saves: 'Glanzparaden', clears: 'Befreiungsschläge',
    centers: 'Flanken', aerial_hits: 'Treffer in der Luft', first_touches: 'Erste Berührungen', flip_resets: 'Flip-Resets',
    crossbar_hits: 'Lattentreffer', bicycle_hits: 'Fallrückzieher', juggle_hits: 'Jongliertreffer', mvp: 'MVP',
  }],
  ['boost', 'Boost', {
    bpm: 'Boost pro Minute', bcpm: 'Gesammelt pro Minute', avg_amount: 'Ø Boost', amount_collected: 'Gesammelt',
    amount_collected_big: '… von großen Pads', amount_collected_small: '… von kleinen Pads', amount_stolen: 'Gestohlen',
    amount_stolen_big: '… große Pads', amount_stolen_small: '… kleine Pads', count_collected_big: 'Große Pads',
    count_collected_small: 'Kleine Pads', count_stolen_big: 'Große Pads gestohlen', count_stolen_small: 'Kleine Pads gestohlen',
    amount_overfill: 'Überfüllt', amount_overfill_stolen: 'Überfüllt (gestohlen)', amount_used_while_supersonic: 'Verbraucht bei Überschall',
    time_zero_boost: 'Zeit ohne Boost', percent_zero_boost: 'Anteil ohne Boost', time_full_boost: 'Zeit mit vollem Boost',
    percent_full_boost: 'Anteil mit vollem Boost', time_boost_0_25: 'Zeit mit 0–25 %', percent_boost_0_25: 'Anteil 0–25 %',
    time_boost_25_50: 'Zeit mit 25–50 %', percent_boost_25_50: 'Anteil 25–50 %', time_boost_50_75: 'Zeit mit 50–75 %',
    percent_boost_50_75: 'Anteil 50–75 %', time_boost_75_100: 'Zeit mit 75–100 %', percent_boost_75_100: 'Anteil 75–100 %',
  }],
  ['movement', 'Bewegung', {
    avg_speed: 'Ø Geschwindigkeit', avg_speed_percentage: 'Ø Geschwindigkeit (vom Maximum)', total_distance: 'Strecke',
    time_supersonic_speed: 'Zeit mit Überschall', percent_supersonic_speed: 'Anteil Überschall', time_boost_speed: 'Zeit mit Boost-Tempo',
    percent_boost_speed: 'Anteil Boost-Tempo', time_slow_speed: 'Zeit langsam', percent_slow_speed: 'Anteil langsam',
    time_ground: 'Zeit am Boden', percent_ground: 'Anteil am Boden', time_wall: 'Zeit an der Wand', percent_wall: 'Anteil an der Wand',
    time_low_air: 'Zeit tief in der Luft', percent_low_air: 'Anteil tief in der Luft', time_high_air: 'Zeit hoch in der Luft',
    percent_high_air: 'Anteil hoch in der Luft', time_powerslide: 'Zeit im Powerslide', count_powerslide: 'Powerslides',
    avg_powerslide_duration: 'Ø Powerslide-Dauer',
  }],
  ['positioning', 'Position', {
    avg_distance_to_ball: 'Ø Abstand zum Ball', avg_distance_to_ball_possession: '… bei eigenem Ballbesitz',
    avg_distance_to_ball_no_possession: '… bei gegnerischem Ballbesitz', avg_distance_to_mates: 'Ø Abstand zu Mitspielern',
    time_defensive_third: 'Zeit im eigenen Drittel', percent_defensive_third: 'Anteil eigenes Drittel',
    time_neutral_third: 'Zeit im Mitteldrittel', percent_neutral_third: 'Anteil Mitteldrittel',
    time_offensive_third: 'Zeit im gegnerischen Drittel', percent_offensive_third: 'Anteil gegnerisches Drittel',
    time_defensive_half: 'Zeit in der eigenen Hälfte', percent_defensive_half: 'Anteil eigene Hälfte',
    time_offensive_half: 'Zeit in der gegnerischen Hälfte', percent_offensive_half: 'Anteil gegnerische Hälfte',
    time_behind_ball: 'Zeit hinter dem Ball', percent_behind_ball: 'Anteil hinter dem Ball',
    time_infront_ball: 'Zeit vor dem Ball', percent_infront_ball: 'Anteil vor dem Ball',
    time_most_back: 'Zeit als Hinterster', percent_most_back: 'Anteil als Hinterster',
    time_most_forward: 'Zeit als Vorderster', percent_most_forward: 'Anteil als Vorderster',
    time_closest_to_ball: 'Zeit am nächsten am Ball', percent_closest_to_ball: 'Anteil am nächsten am Ball',
    time_farthest_from_ball: 'Zeit am weitesten vom Ball', percent_farthest_from_ball: 'Anteil am weitesten vom Ball',
    goals_against_while_last_defender: 'Gegentore als letzter Mann',
  }],
  ['demo', 'Demos', {
    inflicted: 'Demos verteilt', inflicted_in_defensive_half: '… in der eigenen Hälfte', inflicted_in_offensive_half: '… in der gegnerischen Hälfte',
    taken: 'Demos kassiert', taken_in_defensive_half: '… in der eigenen Hälfte', taken_in_offensive_half: '… in der gegnerischen Hälfte',
  }],
];

const cameraLabels = { fov: 'Sichtfeld', height: 'Höhe', pitch: 'Neigung', distance: 'Abstand', stiffness: 'Steifheit', swivel_speed: 'Schwenkgeschwindigkeit', transition_speed: 'Übergangsgeschwindigkeit' };

// value formats a statistic by its kind.
function value(key, v) {
  if (v === undefined || v === null) return '–';
  if (typeof v === 'boolean') return v ? '✓' : '';
  if (key.startsWith('time_') || key === 'avg_powerslide_duration') return key === 'avg_powerslide_duration' ? fmt.num(v) + ' s' : fmt.dur(v);
  if (key.startsWith('percent_') || key.endsWith('_percentage')) return fmt.num(v) + ' %';
  if (key === 'total_distance') return fmt.num(v / 100000) + ' km';
  if (key === 'avg_speed') return fmt.num(v * 0.036) + ' km/h';
  if (key.startsWith('avg_distance')) return fmt.num(v / 100) + ' m';
  return fmt.num(v);
}

function isPercent(key) { return key.startsWith('percent_') || key.endsWith('_percentage'); }

// statTable shows one group: rows are statistics, columns the players and
// the team totals.
function statTable(detail, group, labels, meIDs) {
  const players = [];
  for (const color of ['blue', 'orange']) for (const p of detail[color]?.players || []) players.push({ ...p, color });
  const teams = ['blue', 'orange'].map(c => ({ color: c, stats: detail[c]?.stats?.[group] || {} })).filter(t => Object.keys(t.stats).length > 0);
  const keys = Object.keys(labels).filter(k => players.some(p => p.stats?.[group]?.[k] !== undefined) || teams.some(t => t.stats[k] !== undefined));
  for (const p of players) for (const k of Object.keys(p.stats?.[group] || {})) if (!keys.includes(k)) keys.push(k);
  if (!keys.length) return h('div', { class: 'empty' }, 'Für dieses Replay liefert ballchasing hier nichts.');
  const pid = p => p.id ? p.id.platform + ':' + p.id.id : '';
  return h('div', { style: { overflowX: 'auto' } }, h('table', { class: 'stats' },
    h('thead', null, h('tr', null, h('th', null, ''),
      players.map(p => h('th', { class: meIDs.has(pid(p)) ? 'me' : '' }, h('span', { class: p.color === 'blue' ? 'b' : 'o' }, p.name))),
      teams.map(t => h('th', null, t.color === 'blue' ? 'Team Blau' : 'Team Orange')))),
    h('tbody', null, keys.map(k => h('tr', null,
      h('td', null, labels[k] || k),
      players.map(p => {
        const v = p.stats?.[group]?.[k];
        return h('td', { class: 'num', style: meIDs.has(pid(p)) ? { fontWeight: 600 } : null },
          isPercent(k) && typeof v === 'number' ? h('div', { class: 'row nowrap', style: { justifyContent: 'flex-end' } },
            h('div', { class: 'minibar', style: { width: '50px' } }, h('i', { style: { width: Math.min(100, v) + '%' } })), value(k, v)) : value(k, v));
      }),
      teams.map(t => h('td', { class: 'num muted' }, t.stats[k] !== undefined ? value(k, t.stats[k]) : '')))))));
}

function cameraTable(detail) {
  const players = ['blue', 'orange'].flatMap(c => (detail[c]?.players || []).map(p => ({ ...p, color: c })));
  const rows = [['Auto', p => p.car_name || '–'], ['Lenkempfindlichkeit', p => fmt.num(p.steering_sensitivity)],
    ...Object.entries(cameraLabels).map(([k, label]) => [label, p => fmt.num(p.camera?.[k])])];
  return h('div', { style: { overflowX: 'auto' } }, h('table', { class: 'stats' },
    h('thead', null, h('tr', null, h('th', null, ''), players.map(p => h('th', null, h('span', { class: p.color === 'blue' ? 'b' : 'o' }, p.name))))),
    h('tbody', null, rows.map(([label, get]) => h('tr', null, h('td', null, label), players.map(p => h('td', { class: 'num' }, get(p))))))));
}

export async function mount(root, ctx) {
  const id = ctx.param;
  root.append(h('div', { class: 'muted' }, 'Lade Statistiken …'));
  const data = await api('/api/replays/' + encodeURIComponent(id));
  const r = data.replay;
  const d = data.detail || {};
  const meIDs = new Set(ctx.state.me || []);
  ctx.setTitle(r.title || 'Replay');
  let tab = 'overview';
  const body = h('div');

  const info = [
    ['Gespielt', fmt.date(r.date)], ['Playlist', r.playlist], ['Map', r.map],
    ['Dauer', fmt.dur(r.duration) + (r.overtime ? ' (Verlängerung' + (d.overtime_seconds ? ` ${fmt.dur(d.overtime_seconds)}` : '') + ')' : '')],
    ['Server', d.server ? [d.server.name, d.server.region].filter(Boolean).join(', ') : '–'], ['Saison', d.season ?? '–'],
    ['Sichtbarkeit', visibilityNames[r.visibility] || r.visibility], ['Ordner', r.own ? (r.group || 'Eingang') : '–'],
    ['Hochgeladen', fmt.date(r.created) + ' von ' + r.uploader],
  ];
  const possession = ['blue', 'orange'].map(c => d[c]?.stats?.ball?.possession_time || 0);

  const tabs = [['overview', 'Überblick'], ...groups.map(([k, label]) => [k, label]), ['camera', 'Kamera & Auto']];
  const tabBar = h('div', { class: 'tabs', role: 'tablist' });
  function render() {
    fill(tabBar, ...tabs.map(([k, label]) => h('button', { role: 'tab', 'aria-selected': String(tab === k), class: tab === k ? 'active' : '',
      onclick: () => { tab = k; render(); } }, label)));
    if (tab === 'overview') {
      fill(body, h('div', { class: 'grid grid-2' },
        h('div', { class: 'card' }, h('h2', null, 'Spiel'), h('table', { class: 'stats' }, h('tbody', null,
          info.map(([k, v]) => h('tr', null, h('td', { class: 'muted' }, k), h('td', null, v)))))),
        h('div', { class: 'card' }, h('h2', null, 'Ballbesitz'),
          possession[0] + possession[1] > 0 ? h('div', { class: 'stack' },
            h('div', { class: 'row', style: { justifyContent: 'space-between' } },
              h('span', null, h('i', { class: 'swatch', style: { background: 'var(--blue)' } }), 'Blau ', h('strong', null, fmt.dur(possession[0])),
                h('span', { class: 'muted' }, ' · ' + fmt.pct(possession[0], possession[0] + possession[1]))),
              h('span', null, h('span', { class: 'muted' }, fmt.pct(possession[1], possession[0] + possession[1]) + ' · '), h('strong', null, fmt.dur(possession[1])), ' Orange',
                h('i', { class: 'swatch', style: { background: 'var(--orange)', margin: '0 0 0 6px' } }))),
            h('div', { class: 'row nowrap', style: { gap: '2px', height: '10px' } },
              h('i', { style: { flex: possession[0], height: '100%', background: 'var(--blue)', borderRadius: '4px 0 0 4px' } }),
              h('i', { style: { flex: possession[1], height: '100%', background: 'var(--orange)', borderRadius: '0 4px 4px 0' } })))
            : h('div', { class: 'muted' }, 'Keine Angabe')),
        h('div', { class: 'card', style: { gridColumn: '1 / -1' } }, h('h2', null, 'Spieler'), playerTable(r))));
    } else if (tab === 'camera') {
      fill(body, h('div', { class: 'card' }, cameraTable(d)));
    } else {
      const [, , labels] = groups.find(([k]) => k === tab);
      fill(body, h('div', { class: 'card' }, statTable(d, tab, labels, meIDs)));
    }
  }

  fill(root, 
    h('div', { class: 'row' }, h('a', { href: '#/verlauf' }, '← Verlauf'), h('span', { class: 'grow' }),
      h('a', { class: 'btn', href: r.link, target: '_blank', rel: 'noopener noreferrer' }, 'Auf ballchasing öffnen ↗'),
      h('button', { onclick: e => menu(e.currentTarget, replayActions(r, () => location.reload())) }, 'Aktionen ⋯')),
    h('div', { class: 'card row', style: { gap: '16px' } }, resultBadge(r), h('div', { style: { fontSize: '20px' } }, scoreLine(r)),
      h('span', { class: 'muted' }, r.playlist + ' · ' + r.map),
      r.result ? h('span', { class: 'chip ' + (r.result === 'win' ? 'ok' : 'bad') }, h('span', { class: 'dot' }), r.result === 'win' ? 'Sieg' : 'Niederlage') : null),
    data.also?.length ? h('div', { class: 'banner info' }, 'Dasselbe Spiel hat auch ', data.also.map(a => a.uploader).join(', '), ' hochgeladen.') : null,
    tabBar, body);
  render();
}
