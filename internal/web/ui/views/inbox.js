import { api, h, ico, fill, fmt, flash, busy, debounce, pickFolder, moveTo, plName, resultBadge, scoreLine, mates } from '../lib.js';

const chosen = new Map(); // replay ID -> {id, name} picked by hand

export async function mount(root, ctx) {
  const page = h('div', { class: 'page' });
  root.append(page);
  let items = [];
  const sel = new Set();

  // target is the folder a row goes to: picked by hand, else the first suggestion.
  const target = it => chosen.get(it.replay.id) || (it.suggestions[0] && { id: it.suggestions[0].group_id, name: it.suggestions[0].name });

  async function move(ids, f, quiet) {
    const n = await moveTo(ids, f, quiet);
    ids.forEach(id => { sel.delete(id); chosen.delete(id); });
    return n;
  }

  // acceptAll moves every row to its target, one request per folder.
  async function acceptAll(list) {
    const by = new Map();
    for (const it of list) {
      const t = target(it);
      if (!by.has(t.id)) by.set(t.id, { f: t, ids: [] });
      by.get(t.id).ids.push(it.replay.id);
    }
    let n = 0;
    const done = busy(`${list.length} Replays werden einsortiert`);
    try {
      for (const { f, ids } of by.values()) n += await move(ids, f, true);
    } finally {
      done();
    }
    if (n) flash(`${n} Replays einsortiert`);
    load();
  }

  function row(it) {
    const r = it.replay;
    const t = target(it);
    const on = sel.has(r.id);
    return h('tr', { class: on ? 'sel' : '' },
      h('td', null, h('input', { type: 'checkbox', 'aria-label': 'Auswählen', checked: on, onchange: e => { e.target.checked ? sel.add(r.id) : sel.delete(r.id); render(); } })),
      h('td', null, resultBadge(r)),
      h('td', { class: 'nowrap' }, scoreLine(r), h('div', { class: 'sm n5' }, [mates(r), fmt.when(r.date)].filter(Boolean).join(', '))),
      h('td', null, h('div', { class: 'ell n3', style: { maxWidth: '160px' }, title: r.title }, r.title)),
      h('td', { class: 'nowrap n4' }, plName(r.playlist_id, r.playlist)),
      h('td', null, h('button', { class: 'picker' + (t ? ' has' : ''), title: it.suggestions[0]?.reason || 'kein Vorschlag', 'aria-haspopup': 'menu',
        onclick: async e => {
          const f = await pickFolder(e.currentTarget, { suggestions: it.suggestions, current: t?.id });
          if (f) { chosen.set(r.id, f); render(); }
        } }, ico('folder-simple'), h('span', { class: 'grow ell', style: { textAlign: 'left' } }, t ? t.name : 'Ordner wählen'), ico('caret-down'))),
      h('td', { class: 'r' }, h('button', { class: 'btn btn-ghost sm', disabled: !t, onclick: async () => { await move([r.id], t); load(); } }, ico('check'), 'OK')));
  }

  function render() {
    const withTarget = items.filter(target);
    const all = items.length > 0 && items.every(it => sel.has(it.replay.id));
    fill(page,
      h('div', { class: 'head tight' }, h('h1', null, 'Eingang'),
        h('span', { class: 'nowrap n4', style: { fontSize: '13px' } }, `${items.length} Replays, ${withTarget.length} mit Vorschlag`),
        h('span', { class: 'end actions' },
          h('button', { class: 'btn btn-secondary', disabled: !sel.size, onclick: async e => {
            const f = await pickFolder(e.currentTarget);
            if (f) { await move([...sel], f); load(); }
          } }, sel.size ? `${sel.size} verschieben` : 'Auswahl verschieben'),
          h('button', { class: 'btn btn-primary', disabled: !withTarget.length, onclick: () => acceptAll(withTarget) },
            ico('checks'), `${withTarget.length} Vorschläge übernehmen`))),
      items.length ? h('table', { class: 'table' },
        h('thead', null, h('tr', null,
          h('th', { style: { width: '24px' } }, h('input', { type: 'checkbox', 'aria-label': 'Alle auswählen', checked: all,
            onchange: () => { all ? sel.clear() : items.forEach(it => sel.add(it.replay.id)); render(); } })),
          h('th', { style: { width: '28px' } }), ['Spiel', 'Replay Name', 'Playlist', 'Ordner'].map(t => h('th', null, t)), h('th', { style: { width: '70px' } }))),
        h('tbody', null, items.map(row)))
        : h('div', { class: 'empty' }, ico('check-circle'), 'Alles einsortiert. Neue Uploads ohne passende Regel landen hier.'));
  }

  async function load() {
    const { sessions } = await api('/api/inbox');
    items = sessions.flatMap(s => s.items);
    const present = new Set(items.map(it => it.replay.id));
    for (const id of sel) if (!present.has(id)) sel.delete(id);
    render();
  }

  await load();
  const soon = debounce(load, 300);
  ctx.on('inbox', soon);
  ctx.on('groups', soon);
  return { refresh: load };
}
