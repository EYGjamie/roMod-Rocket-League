import { api, h, fill, run, flash, sw, debounce, confirmDialog, visibilityNames, system, tierNames, uploadQuota } from '../lib.js';

export const title = 'Einstellungen';

// keyName turns a KeyboardEvent.code into a readable name.
export function keyName(code) {
  if (!code) return '–';
  const names = { Backspace: 'Rücktaste (Backspace)', Enter: 'Eingabe', Space: 'Leertaste', Tab: 'Tab', Delete: 'Entf', Insert: 'Einfg',
    Home: 'Pos1', End: 'Ende', PageUp: 'Bild ↑', PageDown: 'Bild ↓', ArrowUp: 'Pfeil ↑', ArrowDown: 'Pfeil ↓', ArrowLeft: 'Pfeil ←', ArrowRight: 'Pfeil →',
    ShiftLeft: 'Umschalt links', ShiftRight: 'Umschalt rechts', ControlLeft: 'Strg links', ControlRight: 'Strg rechts', AltLeft: 'Alt', AltRight: 'Alt Gr', CapsLock: 'Feststell' };
  if (names[code]) return names[code];
  if (code.startsWith('Key')) return code.slice(3);
  if (code.startsWith('Digit')) return code.slice(5);
  if (code.startsWith('Numpad')) return 'Num ' + code.slice(6);
  return code;
}

// keyCapture records a single key for the game's save key.
export function keyCapture(current, onChange) {
  let code = current;
  let listening = false;
  const btn = h('button', { class: 'keycap' });
  const show = () => { btn.textContent = listening ? 'Taste drücken … (Esc bricht ab)' : keyName(code); btn.classList.toggle('listening', listening); };
  const onKey = e => {
    e.preventDefault();
    e.stopPropagation();
    stop();
    if (e.code !== 'Escape') { code = e.code; onChange(code); }
    show();
  };
  const stop = () => { listening = false; document.removeEventListener('keydown', onKey, true); };
  btn.onclick = () => { if (listening) { stop(); show(); return; } listening = true; show(); document.addEventListener('keydown', onKey, true); };
  btn.addEventListener('blur', () => { if (listening) { stop(); show(); } });
  show();
  return btn;
}

// hotkeyCapture records a combination like Ctrl+Alt+B; Entf clears it.
// meta names the Windows key: Win, or Super on Linux.
export function hotkeyCapture(current, onChange, meta = 'Win') {
  let value = current;
  let listening = false;
  const btn = h('button', { class: 'keycap' });
  const pretty = v => v ? v.replace('Ctrl', 'Strg').replace('Shift', 'Umschalt') : 'kein Hotkey';
  const show = () => { btn.textContent = listening ? 'Kombination drücken …' : pretty(value); btn.classList.toggle('listening', listening); };
  const onKey = e => {
    e.preventDefault();
    e.stopPropagation();
    if (e.code === 'Escape') { stop(); show(); return; }
    if (e.code === 'Delete' || e.code === 'Backspace') { stop(); value = ''; onChange(value); show(); return; }
    let key = '';
    if (e.code.startsWith('Key')) key = e.code.slice(3);
    else if (e.code.startsWith('Digit')) key = e.code.slice(5);
    else if (/^F\d{1,2}$/.test(e.code)) key = e.code;
    if (!key) return; // a modifier alone, wait for the key
    const mods = [e.ctrlKey && 'Ctrl', e.altKey && 'Alt', e.shiftKey && 'Shift', e.metaKey && meta].filter(Boolean);
    if (!mods.length && !key.startsWith('F')) { flash(`Bitte mit Strg, Alt, Umschalt oder ${meta} kombinieren`, 'err'); return; }
    stop();
    value = [...mods, key].join('+');
    onChange(value);
    show();
  };
  const stop = () => { listening = false; document.removeEventListener('keydown', onKey, true); };
  btn.onclick = () => { if (listening) { stop(); show(); return; } listening = true; show(); document.addEventListener('keydown', onKey, true); };
  btn.addEventListener('blur', () => { if (listening) { stop(); show(); } });
  show();
  return btn;
}

// testKeyButton presses the save key after a countdown.
export function testKeyButton() {
  const out = h('span', { class: 'muted', role: 'status' });
  const btn = h('button', { onclick: async () => {
    btn.disabled = true;
    let left = 5;
    out.textContent = `Wechsle jetzt in ein Match in Rocket League … ${left}`;
    const t = setInterval(() => { left--; out.textContent = left > 0 ? `Wechsle jetzt in ein Match in Rocket League … ${left}` : 'Taste wird gedrückt …'; }, 1000);
    try {
      const r = await api('/api/test-key', { method: 'POST', body: { delay: 5 }, allowError: true });
      out.textContent = r.error ? r.error : 'Taste gedrückt. Im Spiel sollte jetzt stehen, dass das Replay gespeichert wird.';
    } catch (e) {
      out.textContent = e.message;
    } finally {
      clearInterval(t);
      btn.disabled = false;
    }
  } }, 'Testen');
  return h('span', { class: 'row' }, btn, out);
}

// tokenForm connects to ballchasing; onDone receives the account. Only
// Windows encrypts the token (DPAPI); elsewhere the file permissions guard it.
export function tokenForm(onDone, sys) {
  const input = h('input', { type: 'password', placeholder: 'Upload-Token von ballchasing.com/upload', style: { flex: '1 1 280px' }, autocomplete: 'off',
    onkeydown: e => { if (e.key === 'Enter') connect(); } });
  const reveal = h('button', { class: 'ghost small', onclick: () => { input.type = input.type === 'password' ? 'text' : 'password'; reveal.textContent = input.type === 'password' ? 'Anzeigen' : 'Verbergen'; } }, 'Anzeigen');
  const msg = h('div', { class: 'muted', role: 'status' });
  const btn = h('button', { class: 'primary', onclick: () => connect() }, 'Verbinden');
  async function connect() {
    const token = input.value.trim();
    if (!token) return input.focus();
    btn.disabled = true;
    msg.textContent = 'Prüfe …';
    try {
      const me = await api('/api/token', { method: 'POST', body: { token } });
      msg.textContent = 'Angemeldet als ' + me.name;
      input.value = '';
      onDone?.(me);
    } catch (e) {
      msg.textContent = e.message;
    } finally {
      btn.disabled = false;
    }
  }
  return h('div', { class: 'stack' }, h('div', { class: 'row' }, input, reveal, btn), msg,
    h('div', { class: 'faint', style: { fontSize: '12px' } }, 'Den Token findest du auf ', h('a', { href: 'https://ballchasing.com/upload', target: '_blank', rel: 'noopener noreferrer' }, 'ballchasing.com/upload'),
      ' (nach dem Anmelden mit Steam). ', sys?.windows === false ? 'Er wird in der Konfiguration gespeichert, die nur dein Benutzer lesen kann.' : 'Er wird verschlüsselt gespeichert.'));
}

// identityEditor shows and edits the user's player accounts.
export async function identityEditor(onChange) {
  const box = h('div', { class: 'stack' });
  async function render() {
    const { current, candidates, players } = await api('/api/identities');
    const set = async pids => {
      await run(() => api('/api/identities', { method: 'PUT', body: { pids } }), 'Gespeichert');
      onChange?.();
      render();
    };
    const pids = current.map(c => c.pid);
    const add = h('select', { onchange: e => { if (e.target.value) set([...pids, e.target.value]); } },
      h('option', { value: '' }, 'Account hinzufügen …'),
      players.filter(p => !pids.includes(p.pid)).map(p => h('option', { value: p.pid }, `${p.name} (${p.games} Spiele, ${p.pid})`)));
    fill(box, 
      current.length ? h('div', { class: 'list' }, current.map(c => h('div', { class: 'item', style: { gridTemplateColumns: '1fr auto auto' } },
        h('div', null, h('strong', null, c.name || c.pid), h('div', { class: 'faint', style: { fontSize: '12px' } }, c.pid)),
        c.confirmed ? h('span', { class: 'chip ok' }, h('span', { class: 'dot' }), 'bestätigt') : h('button', { class: 'small primary', onclick: () => set(pids) }, 'Das bin ich'),
        h('button', { class: 'icon-btn', 'aria-label': 'Entfernen', onclick: () => set(pids.filter(p => p !== c.pid)) }, '✕'))))
        : h('div', { class: 'muted' }, candidates.length ? 'Noch nicht festgelegt.' : 'Wird nach den ersten Uploads erkannt.'),
      !current.length && candidates.length ? h('div', { class: 'row' }, h('span', null, 'Vorschlag: ', candidates.map(c => c.name).join(' und ')),
        h('button', { class: 'small primary', onclick: () => set(candidates.map(c => c.pid)) }, 'Das bin ich')) : null,
      players.length ? add : null);
  }
  await render();
  return box;
}

// saveKeyMissing explains why the save key is not pressed on this system.
export function saveKeyMissing(sys) {
  return h('div', { class: 'banner info' }, sys.mac
    ? 'Auf dem Mac drückt das Tool die Speichern-Taste nicht (macOS erlaubt das nur mit Bedienungshilfen-Rechten). '
    : 'Ohne X11 oder XWayland kann das Tool die Speichern-Taste nicht drücken. ',
  'Am Matchende ertönt ein Hinweiston: dann im Spiel „Replay speichern“ wählen, hochgeladen wird trotzdem automatisch.');
}

export async function mount(root, ctx) {
  const sys = system(ctx.state);
  let s = await api('/api/settings');
  const save = async patch => {
    try {
      s = await api('/api/settings', { method: 'PUT', body: patch });
      flash('Gespeichert', 'ok');
    } catch (e) {
      flash(e.message, 'err');
      render();
    }
  };
  const setting = (label, hint, control) => h('div', { class: 'setting' },
    h('div', null, h('div', null, label), hint ? h('div', { class: 'hint' }, hint) : null), h('div', null, control));
  const card = (heading, ...rows) => h('div', { class: 'card' }, h('h2', null, heading), h('div', { style: { marginTop: '8px' } }, rows));

  async function render() {
    const acc = s.account;
    const quota = uploadQuota(acc);
    const custom = s.demo_dirs.length > 0;
    const newDir = h('input', { type: 'text', placeholder: 'Ordner hinzufügen, z. B. ' + (sys.windows ? 'D:\\Replays' : '~/Replays'), style: { flex: '1' } });
    const found = { windows: 'Steam und Epic', linux: 'Steam mit Proton, Heroic, Lutris, Bottles', darwin: 'CrossOver, Whisky, Heroic' }[sys.os] || 'Steam und Epic';
    fill(root, 
      acc.status === 'none' ? h('div', { class: 'banner warn' }, 'Noch nicht mit ballchasing verbunden – ohne Token wird nichts hochgeladen.') : null,
      card('ballchasing',
        setting('Verbindung', acc.from_env ? 'Der Token kommt aus der Umgebungsvariable BALLCHASING_TOKEN.' : null,
          h('div', { class: 'row' },
            h('span', { class: 'chip ' + (acc.status === 'ok' ? 'ok' : 'bad') }, h('span', { class: 'dot' }),
              { ok: 'Angemeldet als ' + acc.name, none: 'nicht verbunden', invalid: 'Token ungültig', offline: 'nicht erreichbar' }[acc.status]),
            acc.status !== 'none' && !acc.from_env ? h('button', { class: 'small danger', onclick: async () => {
              if (await confirmDialog('Verbindung trennen', 'Der Token wird gelöscht. Ohne Token wird nichts hochgeladen.', 'Trennen', true)) {
                await run(() => api('/api/token', { method: 'DELETE' }), 'Getrennt');
                s = await api('/api/settings');
                render();
              }
            } }, 'Trennen') : null)),
        acc.status === 'ok' && acc.tier ? setting('Konto', 'Die Patreon-Stufe bestimmt die Limits von ballchasing.',
          h('span', null, tierNames[acc.tier] || acc.tier, quota ? ` · ${quota.text} Uploads in 24 Stunden` : '')) : null,
        setting(acc.status === 'none' ? 'Token eintragen' : 'Token ändern', null, tokenForm(async () => { s = await api('/api/settings'); render(); }, sys)),
        setting('Standard-Sichtbarkeit', 'Für neue Uploads.', h('select', { onchange: e => save({ visibility: e.target.value }) },
          Object.entries(visibilityNames).map(([v, label]) => h('option', { value: v, selected: s.visibility === v }, label))))),
      card('Replays',
        setting('Replay-Ordner', custom ? 'Eigene Auswahl' : `Automatisch erkannt (${found})`,
          h('div', { class: 'stack' },
            s.dirs.length ? null : h('div', { class: 'muted' }, 'Keiner gefunden. Bitte den Ordner TAGame/Demos des Spiels hinzufügen.'),
            s.dirs.map(d => h('div', { class: 'row nowrap' },
              h('span', { class: 'grow ellipsis', title: d.path }, d.path),
              h('span', { class: 'muted nowrap' }, d.exists ? d.replays + ' Replays' : 'fehlt'),
              custom ? h('button', { class: 'icon-btn', 'aria-label': 'Entfernen', onclick: () => save({ demo_dirs: s.demo_dirs.filter(x => x !== d.path) }) }, '✕') : null)),
            h('div', { class: 'row nowrap' }, newDir, h('button', { onclick: () => {
              const p = newDir.value.trim();
              if (p) save({ demo_dirs: [...(custom ? s.demo_dirs : s.dirs.map(d => d.path)), p] });
            } }, 'Hinzufügen')),
            custom ? h('button', { class: 'small ghost', onclick: () => save({ demo_dirs: [] }) }, 'Wieder automatisch erkennen') : null)),
        setting('Ältere Replays beim Start hochladen', 'Lädt Replays hoch, die beim ersten Start nur registriert wurden.', sw(s.upload_existing, v => save({ upload_existing: v }))),
        setting('Replays nach dem Upload löschen', 'Verschiebt die Datei in den Papierkorb, sobald ballchasing sie verarbeitet hat. Nur eigene Uploads aus dem Replay-Ordner. Zum Anschauen im Spiel lassen sie sich über „Ins Spiel laden“ wieder holen.',
          sw(s.delete_after_upload, v => save({ delete_after_upload: v })))),
      card('Spiel',
        sys.save_key ? null : saveKeyMissing(sys),
        sys.save_key ? setting('Replays automatisch speichern', 'Drückt kurz nach dem ersten Anstoß einmal die Speichern-Taste.', sw(s.auto_mark, v => save({ auto_mark: v }))) : null,
        setting('Nur bei Online-Spielen', 'Offline-Spiele und Training werden ignoriert.', sw(s.online_only, v => save({ online_only: v }))),
        sys.save_key ? setting('Speichern-Taste', 'Die Taste, die im Spiel unter Steuerung fürs Replay-Speichern eingestellt ist. Gesendet wird sie immer nur, wenn Rocket League im Vordergrund ist.',
          h('div', { class: 'stack' }, keyCapture(s.mark_key, code => save({ mark_key: code })), testKeyButton())) : null,
        sys.save_key ? setting('Haltedauer', 'Wie lange die Taste gedrückt wird, in Millisekunden.',
          h('input', { type: 'number', min: 100, max: 5000, step: 100, value: s.mark_hold_ms, onchange: e => save({ mark_hold_ms: Number(e.target.value) }) })) : null,
        setting('Stats-API-Adresse', 'Muss zum Port in der TAStatsAPI.ini passen.',
          h('input', { type: 'text', value: s.stats_api_addr, onchange: e => save({ stats_api_addr: e.target.value.trim() }) }))),
      card('Dein Spieler', h('div', { class: 'muted', style: { margin: '4px 0 10px' } }, 'Damit erkennt das Tool Sieg und Niederlage und hebt dich in Statistiken hervor. Mehrere Accounts (z. B. Steam und Epic) sind möglich.'),
        await identityEditor()),
      sys.hotkeys ? card('Hotkeys',
        sys.linux ? h('div', { class: 'muted', style: { margin: '4px 0 10px' } }, 'Unter Wayland wirken sie nur, solange ein X11-Fenster wie das Spiel im Vordergrund ist. Strg+Alt+F1 bis F12 wechselt unter Linux die Konsole.') : null,
        setting(sys.overlay ? 'Overlay öffnen und schließen' : 'Oberfläche öffnen', null, hotkeyCapture(s.gui_hotkey, v => save({ gui_hotkey: v }), sys.linux ? 'Super' : 'Win')),
        setting('Pausieren und fortsetzen', 'Pausiert Uploads und Auto-Speichern zusammen.', hotkeyCapture(s.pause_hotkey, v => save({ pause_hotkey: v }), sys.linux ? 'Super' : 'Win')),
        sys.quick_sort ? setting('Schnell-Einordnen', sys.overlay ? 'Während das Fenster über dem Spiel sichtbar ist.' : 'Während die Benachrichtigung zum Einordnen sichtbar ist.',
          h('span', null, h('kbd', null, 'Strg+Alt+1'), ' bis ', h('kbd', null, 'Strg+Alt+4'))) : null)
        : card('Hotkeys', h('div', { class: 'muted', style: { marginTop: '4px' } }, sys.mac
          ? 'Systemweite Hotkeys gibt es auf dem Mac nicht. Pausieren und die Oberfläche erreichst du über das Symbol in der Menüleiste.'
          : 'Systemweite Hotkeys brauchen eine X11-Sitzung oder XWayland. Pausieren und die Oberfläche erreichst du über das Symbol im Infobereich.')),
      card('Benachrichtigungen',
        setting('Upload erfolgreich', null, sw(s.notify_uploaded, v => save({ notify_uploaded: v }))),
        setting('Upload fehlgeschlagen', null, sw(s.notify_failed, v => save({ notify_failed: v }))),
        setting('Session beendet', 'Nach 30 Minuten ohne Spiel oder wenn Rocket League geschlossen wird.', sw(s.notify_session, v => save({ notify_session: v }))),
        sys.quick_sort ? setting(sys.overlay ? 'Schnell-Einordnen über dem Spiel' : 'Schnell-Einordnen',
          (sys.overlay ? 'Zeigt nach dem Upload den Zielordner mit Countdown.' : 'Zeigt nach dem Upload eine Benachrichtigung mit Zielordner und Alternativen.') + ' Aus: Regeln ordnen ohne Rückfrage ein.',
          sw(s.quick_sort, v => save({ quick_sort: v }))) : h('div', { class: 'muted', style: { margin: '8px 0' } }, 'Regeln ordnen ohne Rückfrage ein: Benachrichtigungen haben hier keine Knöpfe für eine andere Wahl.'),
        sys.quick_sort ? setting('Countdown', 'Sekunden bis der Regel-Ordner gilt.',
          h('input', { type: 'number', min: 3, max: 60, value: s.quick_sort_seconds, onchange: e => save({ quick_sort_seconds: Number(e.target.value) }) })) : null),
      card('Start',
        sys.autostart ? setting(sys.windows ? 'Mit Windows starten' : 'Bei der Anmeldung starten', sys.mac ? 'Erscheint in den Systemeinstellungen unter Allgemein → Anmeldeobjekte.' : null,
          sw(s.autostart, v => run(() => api('/api/autostart', { method: 'POST', body: { on: v } }), v ? 'Startet ' + (sys.windows ? 'mit Windows' : 'bei der Anmeldung') : 'Startet nicht mehr ' + (sys.windows ? 'mit Windows' : 'bei der Anmeldung')))) : null,
        setting('Minimiert starten', (sys.tray ? `Beim Start nur das Symbol ${sys.mac ? 'in der Menüleiste' : 'im Infobereich'} zeigen, ` : 'Beim Start ') +
          (sys.overlay ? 'das Overlay bleibt zu.' : 'die Oberfläche öffnet sich nicht im Browser.'), sw(s.start_minimized, v => save({ start_minimized: v })))),
      card('Erweitert',
        setting('Log', s.log_path, h('button', { onclick: () => run(() => api('/api/open-log', { method: 'POST' })) }, 'Log anzeigen')),
        setting('Einrichtung', 'Den Assistenten noch einmal durchlaufen.', h('a', { class: 'btn', href: '#/einrichtung' }, 'Assistent öffnen'))));
  }

  await render();
  ctx.on('settings', debounce(async () => { s = await api('/api/settings'); }, 300));
  return {};
}
