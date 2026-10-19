import { api, h, ico, fill, fmt, debounce, seg, pickFolder, moveTo, plName, resultBadge, scoreLine, mates } from '../lib.js';

let mode = 'sessions';
let query = '';
const closed = new Set(); // ends of folded sessions

// sessionsOf groups replays, newest first, into runs with at most 30
// minutes between two games, as the server does.
function sessionsOf(items) {
  const out = [];
  for (const r of items) {
    const t = new Date(r.date);
    const s = out[out.length - 1];
    if (s && s.start - t <= 30 * 60e3) {
      s.items.push(r);
      s.start = t;
    } else {
      out.push({ start: t, end: t, items: [r] });
    }
  }
  return out;
}

const open = r => () => { location.hash = '#/verlauf/' + r.id; };

function row(r, when) {
  return h('div', { class: 'row g-hist', onclick: open(r) },
    h('span'), resultBadge(r),
    h('div', { class: 'ell' }, scoreLine(r), h('span', { class: 'sm n5' }, ' ' + mates(r))),
    h('span', { class: 'ell n3', title: r.title }, r.title),
    h('span', { class: 'ell' }, plName(r.playlist_id, r.playlist)),
    h('span', { class: 'ell n4' }, r.map),
    h('span', { class: 'n4 num' }, fmt.dur(r.duration)),
    h('span', null, h('span', { class: 'tag tag-neutral', title: r.own ? r.group || 'Eingang' : 'hochgeladen von ' + r.uploader },
      r.own ? r.group || 'Eingang' : 'von ' + r.uploader)),
    h('span', { class: 'sm n5 num nowrap r' }, when));
}

export async function mount(root, ctx) {
  const list = h('div', { style: { display: 'contents' } });
  const search = h('input', { type: 'search', placeholder: 'Map, Playlist, Mitspieler', value: query, 'aria-label': 'Suche',
    oninput: debounce(e => { query = e.target.value.trim(); load(); }, 300) });
  root.append(h('div', { class: 'page' },
    h('div', { class: 'head tight' }, h('h1', null, 'Verlauf'),
      h('span', { style: { marginLeft: 'var(--space-4)' } }, seg([['sessions', 'Sessions'], ['list', 'Liste']], mode, v => { mode = v; render(); })),
      h('label', { class: 'search end', style: { width: '280px' } }, ico('magnifying-glass'), search)),
    list));
  let items = [];
  let total = 0;

  function session(s) {
    const key = s.end.toISOString();
    const shut = closed.has(key) && !query;
    const wins = s.items.filter(r => r.result === 'win').length;
    const own = s.items.filter(r => r.own).map(r => r.id);
    return h('div', null,
      h('div', { class: 'sess' },
        h('button', { 'aria-expanded': String(!shut), onclick: () => { closed.has(key) ? closed.delete(key) : closed.add(key); render(); } },
          ico(shut ? 'caret-right' : 'caret-down'), `${fmt.title(s.end)}, ${s.items.length > 1 ? fmt.clock(s.start) + '-' : ''}${fmt.clock(s.end)}`),
        h('span', { class: 'n4 num nowrap' }, `${s.items.length} ${s.items.length > 1 ? 'Spiele' : 'Spiel'}, ${wins}-${s.items.filter(r => r.result === 'loss').length}`),
        h('span', { class: 'streak' }, s.items.slice().reverse().map(r => h('i', { class: r.result || '', title: r.result === 'win' ? 'Sieg' : r.result === 'loss' ? 'Niederlage' : 'ohne Ergebnis' }))),
        own.length ? h('button', { class: 'btn btn-ghost sm end', onclick: async e => {
          const f = await pickFolder(e.currentTarget);
          if (f && await moveTo(own, f)) load();
        } }, 'Alle verschieben') : null),
      shut ? null : s.items.map(r => row(r, fmt.clock(r.date))));
  }

  function render() {
    fill(list,
      mode === 'sessions' ? sessionsOf(items).map(session) : h('div', { class: 'rows' }, items.map(r => row(r, fmt.when(r.date)))),
      items.length ? null : h('div', { class: 'empty', style: { padding: 'var(--space-6) 0' } }, query ? `Keine Replays passen zu „${query}“.` : 'Noch keine Replays.'),
      items.length < total ? h('button', { class: 'btn btn-secondary more', onclick: () => load(true) }, 'Mehr laden') : null);
  }

  async function load(more = false) {
    const q = new URLSearchParams({ status: 'uploaded', offset: more ? items.length : 0, limit: more ? 200 : Math.max(200, items.length) });
    if (query) q.set('q', query);
    const res = await api('/api/replays?' + q);
    items = more ? items.concat(res.items) : res.items;
    total = res.total;
    render();
  }

  await load();
  const soon = debounce(() => load(), 400);
  ctx.on('replays', soon);
  ctx.on('groups', soon);
  return { refresh: () => load() };
}
