import { api, h, fill, fmt, run, menu, debounce, confirmDialog, promptDialog, pickFolder, loadGroups, tree, groupPath, replayRow } from '../lib.js';
import { createReplayList } from './list.js';

export const title = 'Ordner';

const open = new Set();

// moveTo moves replays and reports the result.
async function moveTo(ids, group, name) {
  const res = await run(() => api('/api/replays/move', { method: 'POST', body: { ids, group } }));
  if (!res) return;
  if (res.errors.length) run(async () => { throw new Error(`${res.done} verschoben, ${res.errors.length} fehlgeschlagen: ${res.errors[0]}`); });
  else run(async () => {}, `${res.done} Replays nach „${name}“ verschoben`);
}

// sessionBlock shows one session of unsorted replays with its actions.
export function sessionBlock(s, { selected, onSelect, changed, current }) {
  const ids = s.items.map(it => it.replay.id);
  const split = s.split.filter(sp => sp.group_id);
  return h('div', { class: 'list' },
    h('div', { class: 'list-head', style: { flexWrap: 'wrap' } },
      h('strong', { style: { color: 'var(--text)' } }, 'Session ' + fmt.day(s.start) + ', ' + fmt.clock(s.start) + '–' + fmt.clock(s.end)),
      h('span', null, `${s.items.length} Spiele · ${s.wins}–${s.losses}`),
      h('span', { class: 'grow' }),
      h('button', { class: 'small', onclick: async () => {
        const f = await pickFolder(`${ids.length} Replays verschieben`, { inbox: false });
        if (f) { await moveTo(ids, f.id, f.name); changed(); }
      } }, 'Alle nach …'),
      split.length ? h('button', { class: 'small', title: split.map(sp => `${sp.count}× ${sp.playlist} → ${sp.group}`).join('\n'), onclick: async () => {
        const ok = await confirmDialog('Session aufteilen', h('div', { class: 'stack' },
          h('div', null, 'Die Replays werden je Playlist in diese Ordner verschoben:'),
          h('ul', null, split.map(sp => h('li', null, `${sp.count}× ${sp.playlist} → ${sp.group}`)))), 'Aufteilen');
        if (!ok) return;
        for (const sp of split) await moveTo(sp.ids, sp.group_id, sp.group);
        changed();
      } }, 'Aufteilen') : null),
    s.items.map(it => replayRow(it.replay, { selected, onSelect, changed, current: current === it.replay.id })));
}

export async function mount(root, ctx) {
  let selectedNode = (() => { try { return sessionStorage.getItem('ordner'); } catch { return null; } })() || 'unsorted';
  const treeBox = h('div', { class: 'tree', role: 'tree', 'aria-label': 'Ordner' });
  const right = h('div', { class: 'stack' });
  const syncBtn = h('button', { class: 'small', onclick: () => run(() => api('/api/sync', { method: 'POST', body: { full: true } }), 'Abgleich mit ballchasing gestartet') }, 'Aktualisieren');
  root.append(h('div', { class: 'layout-tree' },
    h('div', { class: 'card', style: { padding: '10px' } },
      h('div', { class: 'row', style: { padding: '2px 4px 8px' } }, h('h2', { class: 'grow' }, 'Ordner'),
        h('button', { class: 'small', onclick: () => newFolder('') }, '+ Neu'), syncBtn),
      treeBox),
    right));

  let groups = [];
  let unsorted = 0;

  async function newFolder(parent) {
    const name = await promptDialog(parent ? 'Neuer Unterordner' : 'Neuer Ordner', parent ? 'Name des Unterordners in „' + groupPath(groups, parent) + '“' : 'Name des Ordners');
    if (!name) return;
    const g = await run(() => api('/api/groups', { method: 'POST', body: { name, parent } }), 'Ordner angelegt');
    if (g) { if (parent) open.add(parent); select(g.id); }
  }

  function dropTarget(el, id, name) {
    el.addEventListener('dragover', e => { if (e.dataTransfer.types.includes('application/x-replays')) { e.preventDefault(); el.classList.add('drop'); } });
    el.addEventListener('dragleave', () => el.classList.remove('drop'));
    el.addEventListener('drop', async e => {
      el.classList.remove('drop');
      const raw = e.dataTransfer.getData('application/x-replays');
      if (!raw) return;
      e.preventDefault();
      await moveTo(JSON.parse(raw), id, name);
    });
  }

  function node(g, depth) {
    const expanded = open.has(g.id);
    const el = h('div', { class: 'tree-node' + (selectedNode === g.id ? ' active' : ''), role: 'treeitem', tabindex: 0,
      'aria-expanded': g.children.length ? String(expanded) : null, style: { paddingLeft: 8 + depth * 14 + 'px' },
      title: `${g.direct} direkt, ${g.indirect} mit Unterordnern`,
      onclick: () => select(g.id),
      onkeydown: e => {
        if (e.key === 'Enter') select(g.id);
        if (e.key === 'ArrowRight' && g.children.length) { open.add(g.id); renderTree(); }
        if (e.key === 'ArrowLeft') { open.delete(g.id); renderTree(); }
      } },
      h('span', { class: 'twisty', onclick: e => { e.stopPropagation(); expanded ? open.delete(g.id) : open.add(g.id); renderTree(); } },
        g.children.length ? (expanded ? '▾' : '▸') : ''),
      h('span', { class: 'ellipsis' }, g.name),
      h('span', { class: 'count' }, g.indirect),
      h('button', { class: 'icon-btn', style: { padding: '0 6px' }, 'aria-label': 'Aktionen für ' + g.name, onclick: e => {
        e.stopPropagation();
        menu(e.currentTarget, [
          { label: 'Neuer Unterordner …', run: () => newFolder(g.id) },
          { label: 'Auf ballchasing öffnen', run: () => window.open('https://ballchasing.com/group/' + g.id, '_blank', 'noopener') },
          '-',
          { label: 'Ordner löschen …', danger: true, run: async () => {
            if (g.indirect > 0 || g.children.length) return run(async () => { throw new Error('Nur leere Ordner ohne Unterordner lassen sich löschen.'); });
            if (await confirmDialog('Ordner löschen', `„${g.name}“ wird auf ballchasing gelöscht.`, 'Löschen', true)) {
              await run(() => api('/api/groups/' + g.id, { method: 'DELETE' }), 'Ordner gelöscht');
              if (selectedNode === g.id) select('unsorted');
            }
          } },
        ]);
      } }, '⋯'));
    dropTarget(el, g.id, groupPath(groups, g.id));
    return [el, expanded ? h('div', { role: 'group' }, g.children.map(c => node(c, depth + 1))) : null];
  }

  function renderTree() {
    const inbox = h('div', { class: 'tree-node' + (selectedNode === 'unsorted' ? ' active' : ''), role: 'treeitem', tabindex: 0,
      onclick: () => select('unsorted'), onkeydown: e => { if (e.key === 'Enter') select('unsorted'); } },
      h('span', { class: 'twisty' }, '•'), h('span', null, 'Eingang (unsortiert)'), h('span', { class: 'count' }, unsorted));
    dropTarget(inbox, '', 'Eingang');
    fill(treeBox, inbox, h('hr', { class: 'sep', style: { margin: '6px 0' } }),
      groups.length ? tree(groups).map(g => node(g, 0)) : h('div', { class: 'empty' }, 'Noch keine Ordner.'));
  }

  // --- right side ---
  let list = null;
  const selected = new Set();

  async function renderUnsorted() {
    const [{ tips }, { sessions }] = await Promise.all([api('/api/suggestions'), api('/api/inbox')]);
    const onSelect = (id, on) => { on ? selected.add(id) : selected.delete(id); renderUnsorted(); };
    const bulk = selected.size ? h('div', { class: 'bulk' }, h('strong', null, `${selected.size} ausgewählt`),
      h('button', { class: 'primary', onclick: async () => {
        const f = await pickFolder(`${selected.size} Replays verschieben`, { inbox: false });
        if (f) { await moveTo([...selected], f.id, f.name); selected.clear(); }
      } }, 'In Ordner verschieben …'), h('span', { class: 'grow' }),
      h('button', { class: 'ghost', onclick: () => { selected.clear(); renderUnsorted(); } }, 'Auswahl aufheben')) : null;
    fill(right, 
      h('div', { class: 'row' }, h('h2', { class: 'grow' }, 'Eingang'), h('a', { href: '#/eingang' }, 'Mit Tastatur einordnen')),
      tips.length ? h('div', { class: 'card stack' }, h('h3', null, 'Vorschläge'), tips.map(t => h('div', { class: 'row' },
        h('span', { class: 'grow' }, t.text),
        h('button', { class: 'small primary', onclick: () => run(() => api('/api/suggestions/apply', { method: 'POST', body: { key: t.key } }), 'Übernommen') }, 'Übernehmen'),
        h('button', { class: 'small ghost', onclick: () => run(() => api('/api/suggestions/ignore', { method: 'POST', body: { key: t.key } })) }, 'Ignorieren')))) : null,
      sessions.length ? sessions.map(s => sessionBlock(s, { selected, onSelect, changed: renderUnsorted }))
        : h('div', { class: 'card empty' }, 'Alles einsortiert.'),
      bulk);
  }

  async function renderGroup(g) {
    list = createReplayList({ query: () => new URLSearchParams({ group: g.id, origin: 'own' }), empty: 'In diesem Ordner liegen keine Replays direkt.' });
    fill(right, 
      h('div', { class: 'row' }, h('h2', { class: 'grow' }, groupPath(groups, g.id)),
        h('span', { class: 'muted' }, `${g.direct} direkt · ${g.indirect} mit Unterordnern`),
        h('a', { class: 'btn', href: 'https://ballchasing.com/group/' + g.id, target: '_blank', rel: 'noopener noreferrer' }, 'Auf ballchasing ↗')),
      list.el);
    await list.load();
  }

  async function renderRight() {
    const g = groups.find(x => x.id === selectedNode);
    if (!g) { selectedNode = 'unsorted'; }
    return g ? renderGroup(g) : renderUnsorted();
  }

  function select(id) {
    selectedNode = id;
    try { sessionStorage.setItem('ordner', id); } catch { /* private mode */ }
    selected.clear();
    renderTree();
    renderRight();
  }

  async function load() {
    const data = await loadGroups();
    groups = data.groups;
    unsorted = data.unsorted;
    syncBtn.disabled = data.syncing;
    syncBtn.textContent = data.syncing ? 'Abgleich läuft …' : 'Aktualisieren';
    // open the path to the selected folder
    for (let g = groups.find(x => x.id === selectedNode); g && g.parent; g = groups.find(x => x.id === g.parent)) open.add(g.parent);
    renderTree();
  }

  await load();
  await renderRight();
  const reloadTree = debounce(async () => { await load(); }, 300);
  const reloadRight = debounce(() => { selectedNode === 'unsorted' ? renderUnsorted() : list?.load(); }, 400);
  ctx.on('groups', reloadTree);
  ctx.on('state', reloadTree);
  ctx.on('replays', reloadRight);
  ctx.on('inbox', reloadRight);
  return { refresh: async () => { await load(); reloadRight(); } };
}
