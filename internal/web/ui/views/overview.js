import { api, h, fill, fmt, run, debounce, stateNames, sw, replayRow, uploadFiles, system } from '../lib.js';

export const title = 'Übersicht';

export async function mount(root, ctx) {
  const banners = h('div', { class: 'stack' });
  const status = h('div', { class: 'card' });
  const session = h('div', { class: 'card' });
  const queue = h('div', { class: 'card' });
  const recent = h('div', { class: 'card' });
  const picker = h('input', { type: 'file', accept: '.replay', multiple: true, hidden: true, onchange: e => { uploadFiles(e.target.files); e.target.value = ''; } });
  root.append(banners, h('div', { class: 'grid grid-2' }, status, session), queue, recent, picker);

  function renderStatus() {
    const s = ctx.state;
    const sys = system(s);
    const acc = s.account || {};
    const paused = s.paused || {};
    const toggle = (key, value) => run(() => api('/api/pause', { method: 'POST', body: { [key]: !value } }));
    fill(banners, 
      acc.status === 'none' ? h('div', { class: 'banner warn' }, 'Noch nicht mit ballchasing verbunden. ',
        h('a', { href: '#/einrichtung' }, 'Jetzt einrichten')) : null,
      acc.status === 'invalid' || s.queue?.bad_token ? h('div', { class: 'banner bad' }, 'Der ballchasing-Token ist ungültig, Uploads warten. ',
        h('a', { href: '#/einstellungen' }, 'Neuen Token eintragen')) : null,
      s.queue?.offline ? h('div', { class: 'banner warn' }, 'ballchasing ist gerade nicht erreichbar. Uploads werden automatisch nachgeholt.') : null);
    fill(status, 
      h('div', { class: 'card-head' }, h('h2', null, 'Status')),
      h('div', { class: 'stack' },
        h('div', { class: 'row' }, h('span', { class: 'grow' }, 'ballchasing'),
          h('span', { class: acc.status === 'ok' ? '' : 'muted' }, acc.status === 'ok' ? 'angemeldet als ' + acc.name : { none: 'nicht verbunden', invalid: 'Token ungültig', offline: 'nicht erreichbar' }[acc.status] || '…')),
        h('div', { class: 'row' }, h('span', { class: 'grow' }, 'Rocket League'),
          h('span', { class: s.stats_api ? '' : 'muted' }, s.in_match ? 'im Match' : s.stats_api ? 'verbunden' : 'nicht verbunden')),
        h('hr', { class: 'sep' }),
        h('div', { class: 'row' }, h('div', { class: 'grow' }, 'Ordner überwachen und hochladen'), sw(!paused.watch, () => toggle('watch', paused.watch))),
        sys.save_key ? h('div', { class: 'row' }, h('div', { class: 'grow' }, 'Replays automatisch speichern'), sw(!paused.mark, () => toggle('mark', paused.mark))) : null,
        sys.hotkeys ? h('div', { class: 'faint', style: { fontSize: '12px' } }, 'Strg+Alt+P pausiert beides zusammen.') : null),
      h('div', { class: 'row', style: { marginTop: '14px' } },
        h('button', { onclick: () => picker.click() }, 'Replays hochladen …'),
        h('button', { onclick: () => run(() => api('/api/open-folder', { method: 'POST' })) }, 'Replay-Ordner öffnen')));
  }

  function renderSession(s) {
    if (!s) {
      fill(session, h('div', { class: 'card-head' }, h('h2', null, 'Session')), h('div', { class: 'empty' }, 'Noch keine Spiele mit dir.'));
      return;
    }
    const decided = s.wins + s.losses;
    fill(session, 
      h('div', { class: 'card-head' }, h('h2', null, s.active ? 'Aktuelle Session' : 'Letzte Session'),
        h('span', { class: 'muted' }, fmt.day(s.start) + ', ' + fmt.clock(s.start) + '–' + fmt.clock(s.end))),
      h('div', { class: 'grid grid-3' },
        h('div', null, h('div', { class: 'muted' }, 'Spiele'), h('div', { class: 'num', style: { fontSize: '22px', fontWeight: 700 } }, s.games)),
        h('div', null, h('div', { class: 'muted' }, 'Bilanz'), h('div', { class: 'num', style: { fontSize: '22px', fontWeight: 700 } }, `${s.wins}–${s.losses}`)),
        h('div', null, h('div', { class: 'muted' }, 'Siegquote'), h('div', { class: 'num', style: { fontSize: '22px', fontWeight: 700 } }, fmt.pct(s.wins, decided)))),
      h('div', { style: { marginTop: '12px' } }, h('div', { class: 'muted', style: { marginBottom: '6px' } }, 'Verlauf' + (s.streak > 1 ? ` · längste Siegesfolge ${s.streak}` : '')),
        h('div', { class: 'streak', 'aria-label': s.results.map(r => r === 'win' ? 'Sieg' : r === 'loss' ? 'Niederlage' : 'offen').join(', ') },
          s.results.map(r => h('i', { class: r, title: r === 'win' ? 'Sieg' : r === 'loss' ? 'Niederlage' : 'ohne Ergebnis' })))));
  }

  function renderQueue(q) {
    const active = q.items.filter(f => !['failed', 'rejected'].includes(f.state));
    const failed = q.items.filter(f => ['failed', 'rejected'].includes(f.state));
    const retry = name => run(() => api('/api/queue/retry', { method: 'POST', body: { name } }), 'Wird erneut versucht');
    const line = f => h('div', { class: 'item file' },
      h('span', { class: 'res ' + (['failed', 'rejected'].includes(f.state) ? 'loss' : 'none') }, ['failed', 'rejected'].includes(f.state) ? '!' : '↑'),
      h('div', { class: 'grow' }, h('div', { class: 'ellipsis' }, f.name),
        h('div', { class: 'faint', style: { fontSize: '12px' } },
          stateNames[f.state] || f.state,
          f.state === 'retry' || (f.state === 'queued' && f.error) ? ' · ' + fmt.rel(f.next_try) : '',
          f.error ? ' · ' + f.error : '',
          f.attempts ? ` · Versuch ${f.attempts} von 3` : '')),
      h('div', { class: 'when' }, fmt.date(f.added)),
      ['failed', 'rejected', 'retry'].includes(f.state) ? h('button', { class: 'small', onclick: () => retry(f.name) }, 'Jetzt erneut') : h('span'));
    fill(queue, 
      h('div', { class: 'card-head' }, h('h2', null, 'Warteschlange'),
        failed.length ? h('button', { class: 'small', onclick: () => run(() => api('/api/queue/retry-all', { method: 'POST' }), r => `${r.queued} Uploads neu eingereiht`) }, 'Alle fehlgeschlagenen erneut senden') : null),
      active.length + failed.length ? h('div', { class: 'list' }, [...failed, ...active].slice(0, 30).map(line))
        : h('div', { class: 'empty' }, 'Keine offenen oder fehlgeschlagenen Uploads.'));
  }

  function renderRecent(list) {
    fill(recent, 
      h('div', { class: 'card-head' }, h('h2', null, 'Letzte Uploads'), h('a', { href: '#/verlauf' }, 'Ganzer Verlauf')),
      list.length ? h('div', { class: 'list' }, list.map(r => replayRow(r, { changed: load }))) : h('div', { class: 'empty' }, 'Noch nichts hochgeladen.'));
  }

  async function load() {
    const [q, o] = await Promise.all([api('/api/queue'), api('/api/overview')]);
    renderQueue(q);
    renderRecent(o.recent || []);
    renderSession(o.session);
  }

  renderStatus();
  await load();
  const soon = debounce(load, 300);
  ctx.on('ui:state', renderStatus);
  ctx.on('queue', soon);
  ctx.on('replays', soon);
  ctx.on('groups', soon);
  const tick = setInterval(soon, 15000); // countdowns
  return { refresh: load, destroy: () => clearInterval(tick) };
}
