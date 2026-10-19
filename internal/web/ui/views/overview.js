import { api, h, ico, fill, fmt, run, debounce, seg, stateNames, uploadFiles, plName, resultBadge, scoreLine } from '../lib.js';

const failed = f => f.state === 'failed' || f.state === 'rejected';
let tab = 'all';

function fileRow(f, paused) {
  const bad = failed(f);
  const tip = [stateNames[f.state] || f.state, f.error, f.attempts ? `Versuch ${f.attempts} von 3` : '',
    f.state === 'retry' ? 'nächster ' + fmt.rel(f.next_try) : ''].filter(Boolean).join(', ');
  return h('tr', null,
    h('td', null, h('span', { class: 'res ' + (bad ? 'fail' : 'wait') }, bad ? '!' : '↑')),
    h('td', { class: 'nowrap n5' }, 'wird ermittelt'),
    h('td', null, h('div', { class: 'ell mono n3', style: { maxWidth: '180px' }, title: f.name }, f.name)),
    h('td'), h('td'),
    h('td', null, h('span', { class: 'state ' + (bad ? 'fail' : 'wait'), role: 'img', title: tip, 'aria-label': tip },
      f.state === 'uploading' && !paused ? h('span', { class: 'ring' }) : null, ico(bad ? 'warning' : paused ? 'pause' : 'arrow-up'))),
    h('td'),
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
    const rows = [...files.map(f => ({ kind: failed(f) ? 'err' : 'wait', el: () => fileRow(f, paused.watch) })),
      ...recent.map(r => ({ kind: 'done', el: () => doneRow(r) }))];
    const shown = rows.filter(x => tab === 'all' || x.kind === tab);
    fill(page,
      h('div', { class: 'head' }, h('h1', null, 'Übersicht'),
        h('span', { class: 'end' }, seg([[false, 'Läuft'], [true, 'Pausiert']], !!any,
          v => run(() => api('/api/pause', { method: 'POST', body: { watch: v, mark: v } }))))),
      h('div', { class: 'tabs' },
        [['all', 'Alle'], ['wait', 'Wartet'], ['err', 'Fehler'], ['done', 'Hochgeladen']].map(([k, label]) =>
          h('button', { class: tab === k ? 'on' : '', 'aria-pressed': String(tab === k), onclick: () => { tab = k; render(); } },
            label, h('span', { class: 'count' }, rows.filter(x => k === 'all' || x.kind === k).length))),
        h('span', { class: 'end actions' },
          h('button', { class: 'btn btn-ghost', disabled: !files.some(failed),
            onclick: () => run(() => api('/api/queue/retry-all', { method: 'POST' }), r => `${r.queued} Uploads neu eingereiht`) },
          ico('arrow-clockwise'), 'Fehlgeschlagene erneut'),
          h('button', { class: 'btn btn-secondary', onclick: () => picker.click() }, ico('upload-simple'), 'Hochladen'))),
      h('table', { class: 'table' },
        h('thead', null, h('tr', null, h('th', { style: { width: '28px' } }), ['Spiel', 'Replay Name', 'Playlist', 'Map'].map(t => h('th', null, t)),
          ['Status', 'Ordner', 'Zeit'].map(t => h('th', { class: 'c' }, t)))),
        h('tbody', null, shown.map(x => x.el()))),
      shown.length ? null : h('div', { class: 'empty' }, 'Nichts in dieser Ansicht.'));
  }

  async function load() {
    const [q, o] = await Promise.all([api('/api/queue'), api('/api/overview')]);
    files = q.items;
    recent = o.recent || [];
    render();
  }

  await load();
  const soon = debounce(load, 300);
  ctx.on('ui:state', render);
  ctx.on('queue', soon);
  ctx.on('replays', soon);
  ctx.on('groups', soon);
  const tick = setInterval(soon, 15000); // retry countdowns
  return { refresh: load, destroy: () => clearInterval(tick) };
}
