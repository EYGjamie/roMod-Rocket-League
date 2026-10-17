import { api, h, fill, fmt, run, debounce, promptDialog, pickFolder, resultBadge, scoreLine } from '../lib.js';

export const title = 'Eingang';

export async function mount(root, ctx) {
  let sessions = [];
  let items = [];
  let currentID = null;
  const box = h('div', { class: 'stack' });
  root.append(
    h('div', { class: 'card row', style: { gap: '14px', fontSize: '13px' } },
      h('span', null, h('kbd', null, '1'), '–', h('kbd', null, '4'), ' Ordner'),
      h('span', null, h('kbd', null, 'Enter'), ' Vorschlag übernehmen'),
      h('span', null, h('kbd', null, 'N'), ' neuer Ordner'),
      h('span', null, h('kbd', null, 'E'), ' überspringen'),
      h('span', null, h('kbd', null, 'J'), ' / ', h('kbd', null, 'K'), ' nächstes / voriges'),
      h('span', null, h('kbd', null, 'O'), ' auf ballchasing öffnen')),
    box);

  const index = () => Math.max(0, items.findIndex(it => it.replay.id === currentID));
  const current = () => items[index()];

  async function moveCurrent(group, name) {
    const it = current();
    if (!it) return;
    const next = items[index() + 1] || items[index() - 1];
    const res = await run(() => api('/api/replays/move', { method: 'POST', body: { ids: [it.replay.id], group } }));
    if (res && !res.errors.length) {
      run(async () => {}, 'Nach „' + name + '“');
      currentID = next ? next.replay.id : null;
    } else if (res) {
      run(async () => { throw new Error(res.errors[0]); });
    }
    await load();
  }

  async function newFolder() {
    const name = await promptDialog('Neuer Ordner', 'Name des Ordners (auf oberster Ebene)');
    if (!name) return;
    const g = await run(() => api('/api/groups', { method: 'POST', body: { name, parent: '' } }));
    if (g) moveCurrent(g.id, name);
  }

  function step(delta) {
    if (!items.length) return;
    const i = Math.min(items.length - 1, Math.max(0, index() + delta));
    currentID = items[i].replay.id;
    render();
    box.querySelector('.item.current')?.scrollIntoView({ block: 'nearest' });
  }

  function row(it) {
    const r = it.replay;
    const isCur = r.id === currentID;
    const el = h('div', { class: 'item' + (isCur ? ' current' : ''), style: { gridTemplateColumns: 'auto 1fr auto auto' },
      onclick: () => { currentID = r.id; render(); } },
      resultBadge(r),
      h('div', { class: 'grow' }, scoreLine(r),
        h('div', { class: 'faint ellipsis', style: { fontSize: '12px' } }, [r.playlist, r.map, fmt.dur(r.duration)].join(' · '))),
      h('div', { class: 'when' }, fmt.clock(r.date)),
      h('a', { class: 'icon-btn', href: r.link, target: '_blank', rel: 'noopener noreferrer', 'aria-label': 'Auf ballchasing öffnen', onclick: e => e.stopPropagation() }, '↗'),
      isCur ? h('div', { class: 'item-detail', style: { paddingLeft: '34px' } },
        h('div', { class: 'row' },
          it.suggestions.length ? it.suggestions.slice(0, 4).map((s, i) => h('button', { class: i === 0 ? 'primary' : '', title: s.reason,
            onclick: e => { e.stopPropagation(); moveCurrent(s.group_id, s.name); } },
            h('kbd', null, String(i + 1)), ' ', s.name, h('span', { class: i === 0 ? '' : 'muted', style: { fontSize: '12px', opacity: .8 } }, ' · ' + s.reason)))
            : h('span', { class: 'muted' }, 'Noch keine Vorschläge – ordne ein paar Replays von Hand ein.'),
          h('button', { onclick: async e => {
            e.stopPropagation();
            const f = await pickFolder('Ordner wählen', { inbox: false });
            if (f) moveCurrent(f.id, f.name);
          } }, 'Anderer Ordner …'),
          h('button', { class: 'ghost', onclick: e => { e.stopPropagation(); newFolder(); } }, h('kbd', null, 'N'), ' Neuer Ordner'),
          h('button', { class: 'ghost', onclick: e => { e.stopPropagation(); step(1); } }, h('kbd', null, 'E'), ' Überspringen'))) : null);
    return el;
  }

  function render() {
    if (!items.length) {
      fill(box, h('div', { class: 'card empty' }, 'Alles einsortiert. Neue Uploads ohne passende Regel landen hier.'));
      return;
    }
    if (!items.some(it => it.replay.id === currentID)) currentID = items[0].replay.id;
    fill(box, ...sessions.map(s => h('div', { class: 'list' },
      h('div', { class: 'list-head' }, h('strong', { style: { color: 'var(--text)' } }, 'Session ' + fmt.day(s.start) + ', ' + fmt.clock(s.start) + '–' + fmt.clock(s.end)),
        h('span', null, `${s.items.length} Spiele · ${s.wins}–${s.losses}`)),
      s.items.map(row))));
  }

  async function load() {
    const data = await api('/api/inbox');
    sessions = data.sessions;
    items = sessions.flatMap(s => s.items);
    render();
  }

  function onKey(e) {
    if (e.ctrlKey || e.altKey || e.metaKey || e.target.matches('input, select, textarea') || document.querySelector('.overlay-bg, .menu')) return;
    const it = current();
    const k = e.key.toLowerCase();
    if (k === 'j' || e.key === 'ArrowDown') step(1);
    else if (k === 'k' || e.key === 'ArrowUp') step(-1);
    else if (k === 'e') step(1);
    else if (k === 'n') newFolder();
    else if (k === 'o' && it) window.open(it.replay.link, '_blank', 'noopener');
    else if (e.key === 'Enter' && it?.suggestions[0]) moveCurrent(it.suggestions[0].group_id, it.suggestions[0].name);
    else if (/^[1-4]$/.test(e.key) && it?.suggestions[Number(e.key) - 1]) {
      const s = it.suggestions[Number(e.key) - 1];
      moveCurrent(s.group_id, s.name);
    } else return;
    e.preventDefault();
  }

  await load();
  document.addEventListener('keydown', onKey);
  ctx.on('inbox', debounce(load, 300));
  return { refresh: load, destroy: () => document.removeEventListener('keydown', onKey) };
}
