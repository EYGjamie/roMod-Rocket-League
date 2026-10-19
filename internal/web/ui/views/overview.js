import { api, h, ico, fill, fmt, run, debounce, seg, stateNames, uploadFiles, plName, resultBadge, scoreLine } from '../lib.js';

const failed = f => f.state === 'failed' || f.state === 'rejected';
const kindOf = f => f.state === 'ignored' ? 'ign' : failed(f) ? 'err' : 'wait';
let tab = 'all';

const retry = names => run(() => Promise.all(names.map(name => api('/api/queue/retry', { method: 'POST', body: { name } }))),
  names.length === 1 ? 'Wird erneut hochgeladen' : `${names.length} Uploads neu eingereiht`);
const ignore = body => run(() => api('/api/queue/ignore', { method: 'POST', body }),
  r => r.ignored === 1 ? 'Wird ignoriert, steht jetzt unter „Ignoriert“' : `${r.ignored} Replays werden ignoriert`);

function fileRow(f, paused) {
  const kind = kindOf(f);
  const bad = kind !== 'wait';
  const tip = [stateNames[f.state] || f.state, f.error, f.attempts && kind !== 'ign' ? `Versuch ${f.attempts} von 3` : '',
    f.state === 'retry' ? 'nächster ' + fmt.rel(f.next_try) : ''].filter(Boolean).join(', ');
  const action = (icon, label, onclick) => h('button', { class: 'btn btn-ghost btn-icon n4', 'aria-label': label, title: label, onclick }, ico(icon));
  return h('tr', { class: kind === 'ign' ? 'off' : null },
    h('td', null, h('span', { class: 'res ' + (bad ? 'fail' : 'wait') }, bad ? '!' : '↑')),
    h('td', { class: 'nowrap n5' }, kind === 'ign' ? 'ignoriert' : 'wird ermittelt'),
    h('td', null, h('div', { class: 'ell mono n3', style: { maxWidth: '180px' }, title: f.name }, f.name)),
    bad ? h('td', { colspan: 2 }, h('div', { class: 'ell sm n4', style: { maxWidth: '260px' }, title: f.error }, f.error)) : [h('td'), h('td')],
    h('td', null, h('span', { class: 'state ' + (bad ? 'fail' : 'wait'), role: 'img', title: tip, 'aria-label': tip },
      f.state === 'uploading' && !paused ? h('span', { class: 'ring' }) : null, ico(bad ? 'warning' : paused ? 'pause' : 'arrow-up'))),
    h('td', { class: 'c nowrap' }, bad ? [
      action('arrow-clockwise', kind === 'ign' ? 'Doch hochladen' : 'Erneut versuchen', () => retry([f.name])),
      kind === 'err' ? action('eye-slash', 'Ignorieren, nicht mehr hochladen', () => ignore({ names: [f.name] })) : null,
    ] : null),
    h('td', { class: 'c nowrap n4 num' }, fmt.when(f.added, true)));
}

function doneRow(r) {
  return h('tr', { class: 'link', onclick: () => { location.hash = '#/verlauf/' + r.id; } },
    h('td', null, resultBadge(r)),
    h('td', { class: 'nowrap' }, scoreLine(r)),
    h('td', null, h('div', { class: 'ell n3', style: { maxWidth: '180px' }, title: r.title }, r.title)),
    h('td', { class: 'nowrap' }, plName(r.playlist_id, r.playlist)),
    h('td', { class: 'nowrap n4' }, r.map),
    h('td', null, h('span', { class: 'state', role: 'img', title: 'Hochgeladen', 'aria-label': 'Hochgeladen' }, ico('check'))),
    h('td', { class: 'c' }, h('span', { class: 'tag tag-neutral', title: r.group || 'Eingang' }, r.group || 'Eingang')),
    h('td', { class: 'c nowrap n4 num' }, fmt.when(r.created, true)));
}

export async function mount(root, ctx) {
  const picker = h('input', { type: 'file', accept: '.replay', multiple: true, hidden: true, onchange: e => { uploadFiles(e.target.files); e.target.value = ''; } });
  const page = h('div', { class: 'page' });
  root.append(page, picker);
  let files = [];
  let recent = [];

  function render() {
    const paused = ctx.state.paused || {};
    const any = paused.watch || paused.mark;
    const rows = [...files.map(f => ({ kind: kindOf(f), el: () => fileRow(f, paused.watch) })),
      ...recent.map(r => ({ kind: 'done', el: () => doneRow(r) }))];
    const inTab = k => x => k === 'all' ? x.kind !== 'ign' : x.kind === k;
    const ignored = rows.filter(inTab('ign')).length;
    if (tab === 'ign' && !ignored) tab = 'all';
    const shown = rows.filter(inTab(tab));
    const anyFailed = files.some(failed);
    fill(page,
      h('div', { class: 'head' }, h('h1', null, 'Übersicht'),
        h('span', { class: 'end' }, seg([[false, 'Läuft'], [true, 'Pausiert']], !!any,
          v => run(() => api('/api/pause', { method: 'POST', body: { watch: v, mark: v } }))))),
      h('div', { class: 'tabs' },
        [['all', 'Alle'], ['wait', 'Wartet'], ['err', 'Fehler'], ['done', 'Hochgeladen'], ignored ? ['ign', 'Ignoriert'] : null].filter(Boolean).map(([k, label]) =>
          h('button', { class: tab === k ? 'on' : '', 'aria-pressed': String(tab === k), onclick: () => { tab = k; render(); } },
            label, h('span', { class: 'count' }, rows.filter(inTab(k)).length))),
        h('span', { class: 'end actions' },
          tab === 'ign' ? h('button', { class: 'btn btn-ghost', onclick: () => retry(files.filter(f => f.state === 'ignored').map(f => f.name)) },
            ico('arrow-clockwise'), 'Alle doch hochladen') : [
            anyFailed ? h('button', { class: 'btn btn-ghost n4', title: 'Nicht mehr hochladen und aus der Fehlerliste nehmen',
              onclick: () => ignore({ all: true }) }, ico('eye-slash'), 'Fehlgeschlagene ignorieren') : null,
            h('button', { class: 'btn btn-ghost', disabled: !anyFailed,
              onclick: () => run(() => api('/api/queue/retry-all', { method: 'POST' }), r => `${r.queued} Uploads neu eingereiht`) },
            ico('arrow-clockwise'), 'Fehlgeschlagene erneut')],
          h('button', { class: 'btn btn-secondary', onclick: () => picker.click() }, ico('upload-simple'), 'Hochladen'))),
      h('table', { class: 'table' },
        h('thead', null, h('tr', null, h('th', { style: { width: '28px' } }), ['Spiel', 'Replay Name', 'Playlist', 'Map'].map(t => h('th', null, t)),
          ['Status', 'Ordner', 'Zeit'].map(t => h('th', { class: 'c' }, t)))),
        h('tbody', null, shown.map(x => x.el()))),
      shown.length ? null : h('div', { class: 'empty' }, 'Nichts in dieser Ansicht.'));
  }

  // update renders only what changed: a table built anew between mouse
  // down and up loses the click, and background syncs send many events.
  let drawn = '';
  function update(force) {
    const paused = ctx.state.paused || {};
    const key = JSON.stringify([files, recent, paused.watch, paused.mark]);
    if (force || key !== drawn) render();
    drawn = key;
  }

  async function load(force) {
    const [q, o] = await Promise.all([api('/api/queue'), api('/api/overview')]);
    files = q.items;
    recent = o.recent || [];
    update(force === true);
  }

  await load(true);
  const soon = debounce(load, 300);
  ctx.on('ui:state', () => update());
  ctx.on('queue', soon);
  ctx.on('replays', soon);
  ctx.on('groups', soon);
  const tick = setInterval(() => load(true), 15000); // retry countdowns
  return { refresh: () => load(true), destroy: () => clearInterval(tick) };
}
