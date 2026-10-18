import { api, h, fill, debounce, loadGroups, tree } from '../lib.js';
import { createReplayList } from './list.js';

export const title = 'Verlauf';

// folderOptions lists the folders indented for a select.
export function folderOptions(groups, selected, extra = []) {
  const out = extra.map(([v, label]) => h('option', { value: v, selected: v === selected }, label));
  const walk = (nodes, depth) => {
    for (const g of nodes) {
      out.push(h('option', { value: g.id, selected: g.id === selected }, '  '.repeat(depth) + g.name));
      walk(g.children, depth + 1);
    }
  };
  walk(tree(groups), 0);
  return out;
}

export async function mount(root, ctx) {
  const saved = (() => { try { return JSON.parse(sessionStorage.getItem('verlauf') || '{}'); } catch { return {}; } })();
  const f = { q: '', status: 'all', playlist: '', days: '0', group: '', origin: 'all', ...saved };
  const [{ playlists }, { groups }] = await Promise.all([api('/api/rules'), loadGroups()]);

  const list = createReplayList({
    query: () => new URLSearchParams(Object.entries(f).filter(([, v]) => v && v !== 'all' && v !== '0')),
    empty: 'Keine Replays passen zu den Filtern.',
  });
  const change = (key, value) => {
    f[key] = value;
    try { sessionStorage.setItem('verlauf', JSON.stringify(f)); } catch { /* private mode */ }
    list.reset();
  };
  const select = (key, options, label) => h('select', { 'aria-label': label, onchange: e => change(key, e.target.value) },
    options.map(([v, text]) => h('option', { value: v, selected: f[key] === v }, text)));

  root.append(
    h('div', { class: 'row' },
      h('input', { type: 'search', placeholder: 'Suchen nach Map, Spieler oder Datei …', value: f.q, style: { flex: '1 1 260px' },
        'aria-label': 'Suche', oninput: debounce(e => change('q', e.target.value.trim()), 300) }),
      select('status', [['all', 'Alle'], ['uploaded', 'Hochgeladen'], ['failed', 'Fehler']], 'Status'),
      select('playlist', [['', 'Alle Playlists'], ...playlists.map(p => [p.id, p.name])], 'Playlist'),
      select('days', [['0', 'Gesamter Zeitraum'], ['1', 'Heute'], ['7', 'Letzte 7 Tage'], ['14', 'Letzte 14 Tage'], ['30', 'Letzte 30 Tage']], 'Zeitraum'),
      h('select', { 'aria-label': 'Ordner', onchange: e => change('group', e.target.value) },
        folderOptions(groups, f.group, [['', 'Alle Ordner'], ['unsorted', 'Eingang (ohne Ordner)']])),
      select('origin', [['all', 'Eigene und von Mitspielern'], ['own', 'Nur eigene'], ['mates', 'Von Mitspielern']], 'Herkunft')),
    list.el);
  await list.load();
  const soon = debounce(() => list.load(), 400);
  ctx.on('replays', soon);
  ctx.on('queue', soon);
  return { refresh: () => list.load() };
}
