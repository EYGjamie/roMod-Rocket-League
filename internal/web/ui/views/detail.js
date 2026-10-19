import { api, h, ico, fill, fmt, run, menu, seg, confirmDialog, pickFolder, moveTo, plName, scoreLine, visibilityNames } from '../lib.js';

const tabs = ['Überblick', 'Kern', 'Boost', 'Bewegung', 'Position', 'Demos', 'Kamera & Auto'];

// compared statistics per tab: [label, key, kind, unit]. kind int (the
// default), dec, pct or share: a part of the game, from percent_<key> or
// time_<key>, shown with its time.
const groups = {
  1: ['core', [['Punkte', 'score'], ['Tore', 'goals'], ['Vorlagen', 'assists'], ['Paraden', 'saves'], ['Schüsse', 'shots'], ['Schussquote', 'shooting_percentage', 'pct']]],
  2: ['boost', [['Boost pro Minute', 'bpm', 'int', '/min'], ['Gesammelt pro Minute', 'bcpm', 'int', '/min'], ['Ø Boost-Stand', 'avg_amount', 'pct'],
    ['Gesammelt', 'amount_collected', 'int', 'Boost'], ['… von großen Pads', 'amount_collected_big', 'int', 'Boost'], ['… von kleinen Pads', 'amount_collected_small', 'int', 'Boost'],
    ['Gestohlen', 'amount_stolen', 'int', 'Boost'], ['Große Pads', 'count_collected_big', 'int', '×'], ['Kleine Pads', 'count_collected_small', 'int', '×'],
    ['Überfüllt', 'amount_overfill', 'int', 'Boost'], ['Verbraucht bei Überschall', 'amount_used_while_supersonic', 'int', 'Boost'],
    ['Zeit ohne Boost', 'zero_boost', 'share'], ['Zeit mit vollem Boost', 'full_boost', 'share'], ['Zeit mit 0-25% Boost', 'boost_0_25', 'share'],
    ['Zeit mit 25-50% Boost', 'boost_25_50', 'share'], ['Zeit mit 50-75% Boost', 'boost_50_75', 'share'], ['Zeit mit 75-100% Boost', 'boost_75_100', 'share']]],
  3: ['movement', [['Ø Geschwindigkeit', 'avg_speed', 'dec', 'km/h'], ['Strecke', 'total_distance', 'dec', 'km'], ['Zeit mit Überschall', 'supersonic_speed', 'share'],
    ['Zeit mit Boost-Tempo', 'boost_speed', 'share'], ['Zeit langsam', 'slow_speed', 'share'], ['Zeit am Boden', 'ground', 'share'], ['Zeit an der Wand', 'wall', 'share'],
    ['Zeit tief in der Luft', 'low_air', 'share'], ['Zeit hoch in der Luft', 'high_air', 'share'], ['Powerslides', 'count_powerslide', 'int', '×'],
    ['Ø Powerslide-Dauer', 'avg_powerslide_duration', 'dec', 's']]],
  4: ['positioning', [['Ø Abstand zum Ball', 'avg_distance_to_ball', 'dec', 'm'], ['… bei eigenem Ballbesitz', 'avg_distance_to_ball_possession', 'dec', 'm'],
    ['… bei gegnerischem Ballbesitz', 'avg_distance_to_ball_no_possession', 'dec', 'm'], ['Ø Abstand zu Mitspielern', 'avg_distance_to_mates', 'dec', 'm'],
    ['Zeit im eigenen Drittel', 'defensive_third', 'share'], ['Zeit im Mitteldrittel', 'neutral_third', 'share'], ['Zeit im gegnerischen Drittel', 'offensive_third', 'share'],
    ['Zeit in der eigenen Hälfte', 'defensive_half', 'share'], ['Zeit in der gegnerischen Hälfte', 'offensive_half', 'share'], ['Zeit hinter dem Ball', 'behind_ball', 'share'],
    ['Zeit vor dem Ball', 'infront_ball', 'share'], ['Zeit als Hinterster', 'most_back', 'share'], ['Zeit als Vorderster', 'most_forward', 'share'],
    ['Zeit am nächsten am Ball', 'closest_to_ball', 'share'], ['Zeit am weitesten vom Ball', 'farthest_from_ball', 'share'],
    ['Gegentore als letzter Mann', 'goals_against_while_last_defender']]],
  5: ['demo', [['Demos verteilt', 'inflicted'], ['… in der gegnerischen Hälfte', 'inflicted_in_offensive_half'], ['… in der eigenen Hälfte', 'inflicted_in_defensive_half'],
    ['Demos kassiert', 'taken'], ['… in der eigenen Hälfte', 'taken_in_defensive_half'], ['… in der gegnerischen Hälfte', 'taken_in_offensive_half']]],
};

const refNames = { lobby: 'Lobby-Schnitt', mates: 'Mitspieler', opp: 'Gegner-Schnitt' };
let tab = 0;
let shownID = '';
let ref = 'lobby';
let deltaMode = 'rel';

const dec = (x, k = 1) => (Math.round(x * 10 ** k) / 10 ** k).toLocaleString('de-DE', { maximumFractionDigits: k });
const mmss = s => { const t = Math.round(Math.abs(s)); return Math.floor(t / 60) + ':' + String(t % 60).padStart(2, '0'); };
const fixed = (v, k) => typeof v === 'number' ? v.toLocaleString('de-DE', { minimumFractionDigits: k, maximumFractionDigits: k }) : '–';

// stat reads one statistic of a player in display units.
function stat(p, group, key, kind, dur) {
  const s = p.stats?.[group] || {};
  if (kind === 'share') return s['percent_' + key] ?? (typeof s['time_' + key] === 'number' && dur ? 100 * s['time_' + key] / dur : undefined);
  const v = s[key];
  if (typeof v !== 'number') return undefined;
  if (key === 'avg_speed') return v * 0.036;
  if (key === 'total_distance') return v / 100000;
  if (key.startsWith('avg_distance')) return v / 100;
  return v;
}

function show(x, kind, unit, dur) {
  if (kind === 'share') return `${dec(x)}% (${mmss(x / 100 * dur)})`;
  if (kind === 'pct') return dec(x) + '%';
  return (kind === 'dec' ? dec(x) : dec(x, 0)) + (!unit ? '' : unit === '×' || unit === '/min' ? unit : ' ' + unit);
}

const avg = list => list.length ? list.reduce((a, b) => a + b, 0) / list.length : undefined;

// compareRows sets the user against the lobby, the teammates or the opponents.
function compareRows(players, me, t, dur) {
  const [group, defs] = groups[t];
  const others = { lobby: players, mates: players.filter(p => p.color === me.color && p !== me), opp: players.filter(p => p.color !== me.color) }[ref];
  const out = [];
  for (const [label, key, kind = 'int', unit] of defs) {
    const of = p => stat(p, group, key, kind, dur);
    const mine = of(me);
    if (mine === undefined) continue;
    const values = players.map(of).filter(v => v !== undefined);
    const base = avg(others.map(of).filter(v => v !== undefined));
    const max = Math.max(...values.map(Math.abs)) * 1.15 || 1;
    let delta = '-';
    const d = base === undefined ? 0 : mine - base;
    if (base !== undefined) {
      const sign = Math.abs(d) < 0.05 ? '±' : d > 0 ? '+' : '−';
      if (deltaMode === 'abs') delta = kind === 'share' ? sign + mmss(d / 100 * dur) : kind === 'pct' ? sign + dec(Math.abs(d)) + ' P.' : sign + show(Math.abs(d), kind === 'int' && Math.abs(d) % 1 ? 'dec' : kind, unit, dur);
      else delta = kind === 'share' || kind === 'pct' ? sign + dec(Math.abs(d)) + ' P.' : base ? sign + Math.round(Math.abs(d) / Math.abs(base) * 100) + '%' : '-';
    }
    out.push(h('div', { class: 'cmp', title: `${label}: ${me.name} ${show(mine, kind, unit, dur)}` + (base === undefined ? '' : `, ${refNames[ref]} ${show(base, kind === 'int' ? 'dec' : kind, unit, dur)}`) },
      h('span', { class: 'n3' }, label),
      h('span', { class: 'track' }, h('span', { class: 'me', style: { width: Math.max(0, mine / max * 100) + '%' } }),
        base === undefined ? null : h('span', { class: 'ref', style: { left: Math.max(0, base / max * 100) + '%' } })),
      h('span', { class: 'r num' }, show(mine, kind, unit, dur)),
      h('span', { class: 'r num ' + (base !== undefined && d >= 0 ? 'a3' : 'n4') }, delta)));
  }
  return out;
}

// playerTable has the players as columns and the rows as [label, value(p)].
function playerTable(players, me, rows) {
  return h('table', { class: 'table' },
    h('thead', null, h('tr', null, h('th'), players.map(p => h('th', { class: 'r' }, h('span', { class: p.color }, p.name))))),
    h('tbody', null, rows.map(([label, get]) => h('tr', null, h('td', { class: 'n3' }, label),
      players.map(p => h('td', { class: 'r num' + (p === me ? ' mine' : '') }, get(p)))))));
}

const deg = v => typeof v === 'number' ? fixed(v, 0).replace('-', '−') + '°' : '–';
const carRows = [['Auto', p => p.car_name || '–'], ['Lenkempfindlichkeit', p => fixed(p.steering_sensitivity, 1)], ['Sichtfeld', p => deg(p.camera?.fov)],
  ['Höhe', p => fixed(p.camera?.height, 0)], ['Neigung', p => deg(p.camera?.pitch)], ['Abstand', p => fixed(p.camera?.distance, 0)],
  ['Steifheit', p => fixed(p.camera?.stiffness, 2)], ['Schwenkgeschwindigkeit', p => fixed(p.camera?.swivel_speed, 1)],
  ['Übergangsgeschwindigkeit', p => fixed(p.camera?.transition_speed, 1)]];

function overview(r, d, also) {
  const possession = ['blue', 'orange'].map(c => d[c]?.stats?.ball?.possession_time || 0);
  const all = possession[0] + possession[1];
  const info = [['Replay Name', r.title], ['Playlist', plName(r.playlist_id, r.playlist)], ['Map', r.map],
    ['Dauer', fmt.dur(r.duration) + (r.overtime ? ' (Verlängerung' + (d.overtime_seconds ? ' ' + fmt.dur(d.overtime_seconds) : '') + ')' : '')],
    ['Gespielt', fmt.day(r.date) + ', ' + fmt.clock(r.date)], ['Server', [d.server?.name, d.server?.region].filter(Boolean).join(', ') || '–'],
    ['Ordner', r.own ? r.group || 'Eingang' : '–'], ['Sichtbarkeit', visibilityNames[r.visibility] || r.visibility],
    ['Hochgeladen', 'von ' + r.uploader + (also.length ? ', auch von ' + also.map(a => a.uploader).join(', ') : '')]];
  const players = [...r.blue.players.map(p => ({ ...p, color: 'blue' })), ...r.orange.players.map(p => ({ ...p, color: 'orange' }))];
  const core = (p, k) => p.core ? p[k] : '–';
  return h('div', { class: 'd-over' },
    h('div', { class: 'stack', style: { gap: 'var(--space-4)' } },
      h('div', { class: 'kv' }, info.map(([k, v]) => [h('span', null, k), h('span', null, v)])),
      all ? h('div', { class: 'poss' },
        h('div', { class: 'n4' }, h('span', { class: 'grow' }, `Ballbesitz Blau ${fmt.dur(possession[0])}, ${fmt.pct(possession[0], all)}`),
          h('span', null, `${fmt.pct(possession[1], all)}, ${fmt.dur(possession[1])} Orange`)),
        h('div', { class: 'bar' }, h('i', { style: { flex: possession[0] } }), h('i', { style: { flex: possession[1] } }))) : null),
    h('table', { class: 'table' },
      h('thead', null, h('tr', null, h('th', null, 'Spieler'), ['Punkte', 'Tore', 'Vorlagen', 'Paraden', 'Schüsse'].map(t => h('th', { class: 'r' }, t)))),
      h('tbody', null, players.map(p => h('tr', { class: p.me ? 'sel' : '' },
        h('td', null, h('span', { class: p.color }, '●'), ' ', p.name, p.mvp ? [' ', h('span', { class: 'tag tag-outline' }, 'MVP')] : null),
        h('td', { class: 'r' }, p.score), h('td', { class: 'r' }, core(p, 'goals')), h('td', { class: 'r' }, core(p, 'assists')),
        h('td', { class: 'r' }, core(p, 'saves')), h('td', { class: 'r' }, core(p, 'shots')))))));
}

function actions(r, reload) {
  const items = [];
  if (r.own) {
    items.push({ label: 'In Ordner verschieben …', icon: 'folder-simple', run: async () => {
      const f = await pickFolder(document.querySelector('[data-more]'), { current: r.group_id });
      if (f && await moveTo([r.id], f)) reload();
    } });
    for (const [v, label] of Object.entries(visibilityNames)) {
      items.push({ label: 'Sichtbarkeit: ' + label, on: r.visibility === v, run: async () => {
        if (await run(() => api('/api/replays/visibility', { method: 'POST', body: { ids: [r.id], visibility: v } }), 'Sichtbarkeit geändert')) reload();
      } });
    }
  }
  if (r.borrowed) {
    const keep = r.borrowed === 'temp';
    items.push('-', { label: keep ? 'Geladene Datei behalten' : 'Geladene Datei wieder löschen lassen', run: async () => {
      if (await run(() => api('/api/replays/' + r.id + '/keep', { method: 'POST', body: { keep } }))) reload();
    } });
  }
  if (r.local) items.push({ label: 'Datei im Ordner zeigen', icon: 'file-text', run: () => run(() => api('/api/replays/' + r.id + '/reveal', { method: 'POST' })) });
  if (r.own) {
    items.push('-', { label: 'Auf ballchasing löschen …', icon: 'trash', danger: true, run: async () => {
      if (await confirmDialog('Replay löschen', 'Das Replay wird auf ballchasing endgültig gelöscht. Das lässt sich nicht rückgängig machen.', 'Endgültig löschen', true)
        && await run(() => api('/api/replays/delete', { method: 'POST', body: { ids: [r.id] } }), 'Gelöscht')) location.hash = '#/verlauf';
    } });
  }
  return items;
}

export async function mount(root, ctx) {
  const page = h('div', { class: 'page', style: { gap: 'var(--space-3)' } }, h('div', { class: 'n4', style: { fontSize: '13px' } }, 'Lade Statistiken …'));
  root.append(page);
  const id = ctx.param;
  if (id !== shownID) { tab = 0; shownID = id; }

  async function load() {
    const data = await api('/api/replays/' + encodeURIComponent(id));
    const r = data.replay;
    const d = data.detail || {};
    const meIDs = new Set(ctx.state.me || []);
    const players = ['blue', 'orange'].flatMap(c => (d[c]?.players || []).map(p => ({ ...p, color: c })));
    const me = players.find(p => p.id && meIDs.has(p.id.platform + ':' + p.id.id));
    const content = h('div', { class: 'stack', style: { gap: 'var(--space-3)' } });
    const vtabs = h('div', { class: 'vtabs', role: 'tablist', 'aria-orientation': 'vertical' });

    function render() {
      fill(vtabs, tabs.map((label, i) => h('button', { role: 'tab', 'aria-selected': String(tab === i), class: tab === i ? 'on' : '', onclick: () => { tab = i; render(); } }, label)));
      if (tab === 0) return fill(content, overview(r, d, data.also || []));
      if (tab === 6) return fill(content, players.length ? playerTable(players, me, carRows) : h('div', { class: 'empty' }, 'Für dieses Replay liefert ballchasing hier nichts.'));
      const [group, defs] = groups[tab];
      if (!me) {
        // without the user in the game, the numbers of all players side by side
        const rows = defs.map(([label, key, kind = 'int', unit]) => [label, key, kind, unit])
          .filter(([, key, kind]) => players.some(p => stat(p, group, key, kind, r.duration) !== undefined))
          .map(([label, key, kind, unit]) => [label, p => { const v = stat(p, group, key, kind, r.duration); return v === undefined ? '–' : show(v, kind, unit, r.duration); }]);
        return fill(content, rows.length ? playerTable(players, null, rows) : h('div', { class: 'empty' }, 'In diesem Bereich gibt es keine vergleichbaren Zahlen.'));
      }
      const hasMates = players.some(p => p.color === me.color && p !== me);
      if (ref === 'mates' && !hasMates) ref = 'lobby';
      const rows = compareRows(players, me, tab, r.duration);
      fill(content,
        h('div', { class: 'cmp-bar' },
          h('span', { class: 'key' }, h('span', { class: 'key-me' }), me.name),
          h('span', { class: 'key' }, h('span', { class: 'key-ref' }), refNames[ref]),
          h('span', { class: 'end' }, seg([['rel', 'Relativ'], ['abs', 'Absolut']], deltaMode, v => { deltaMode = v; render(); })),
          seg([['lobby', 'Lobby'], hasMates ? ['mates', 'Mitspieler'] : null, ['opp', 'Gegner']].filter(Boolean), ref, v => { ref = v; render(); })),
        rows.length ? rows : h('div', { class: 'n4', style: { padding: 'var(--space-4) 0', fontSize: '13px' } }, 'In diesem Bereich gibt es keine vergleichbaren Zahlen.'));
    }

    const win = r.result === 'win';
    fill(page,
      h('div', { class: 'd-name ell', title: r.title }, r.title),
      h('div', { class: 'head', style: { flexWrap: 'wrap' } },
        h('span', { class: 'd-score' }, scoreLine(r)),
        r.result ? h('span', { class: 'tag ' + (win ? 'tag-accent' : 'tag-neutral'), style: { fontSize: '13px' } }, win ? 'Sieg' : 'Niederlage') : null,
        h('span', { class: 'end actions' },
          h('a', { class: 'btn btn-secondary', href: r.link, target: '_blank', rel: 'noopener noreferrer' }, ico('arrow-square-out'), 'ballchasing'),
          h('span', { class: 'tipwrap' },
            h('button', { class: 'btn btn-secondary', 'aria-describedby': 'load-tip', onclick: async () => {
              if (await run(() => api('/api/replays/' + r.id + '/borrow', { method: 'POST' }), 'Liegt jetzt im Replay-Ordner, im Spiel unter „Replays“')) load();
            } }, ico('game-controller'), 'Ins Spiel laden'),
            h('span', { class: 'tooltip', id: 'load-tip', role: 'tooltip' }, 'Lädt die Replay-Datei von ballchasing herunter und legt sie in deinen Replay-Ordner. Danach findest du sie im Spiel unter ',
              h('span', { style: { color: 'var(--color-text)' } }, 'Replays'), ' und kannst sie dort ansehen.')),
          actions(r, load).length ? h('button', { class: 'btn btn-secondary btn-icon', 'data-more': '', 'aria-label': 'Weitere Aktionen', title: 'Weitere Aktionen',
            onclick: e => menu(e.currentTarget, actions(r, load), { width: 240 }) }, ico('dots-three')) : null)),
      h('div', { class: 'd-body' }, vtabs, content));
    render();
  }

  await load();
  return { refresh: load };
}
