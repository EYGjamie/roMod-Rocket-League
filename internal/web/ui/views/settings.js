import { api, h, fill, run, flash, sw, debounce, confirmDialog, visibilityNames } from '../lib.js';

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
export function hotkeyCapture(current, onChange) {
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
    const mods = [e.ctrlKey && 'Ctrl', e.altKey && 'Alt', e.shiftKey && 'Shift', e.metaKey && 'Win'].filter(Boolean);
    if (!mods.length && !key.startsWith('F')) { flash('Bitte mit Strg, Alt, Umschalt oder Win kombinieren', 'err'); return; }
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

// tokenForm connects to ballchasing; onDone receives the account.
export function tokenForm(onDone) {
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
      ' (nach dem Anmelden mit Steam). Er wird verschlüsselt gespeichert.'));
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

export async function mount(root, ctx) {
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
    const custom = s.demo_dirs.length > 0;
    const newDir = h('input', { type: 'text', placeholder: 'Ordner hinzufügen, z. B. D:\\Replays', style: { flex: '1' } });
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
        setting(acc.status === 'none' ? 'Token eintragen' : 'Token ändern', null, tokenForm(async () => { s = await api('/api/settings'); render(); })),
        setting('Standard-Sichtbarkeit', 'Für neue Uploads.', h('select', { onchange: e => save({ visibility: e.target.value }) },
          Object.entries(visibilityNames).map(([v, label]) => h('option', { value: v, selected: s.visibility === v }, label))))),
      card('Replays',
        setting('Replay-Ordner', custom ? 'Eigene Auswahl' : 'Automatisch erkannt (Steam und Epic)',
          h('div', { class: 'stack' },
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
        setting('Replays automatisch speichern', 'Drückt kurz nach dem ersten Anstoß einmal die Speichern-Taste.', sw(s.auto_mark, v => save({ auto_mark: v }))),
        setting('Nur bei Online-Spielen', 'Offline-Spiele und Training werden ignoriert.', sw(s.online_only, v => save({ online_only: v }))),
        setting('Speichern-Taste', 'Die Taste, die im Spiel unter Steuerung fürs Replay-Speichern eingestellt ist. Gesendet wird sie immer nur, wenn Rocket League im Vordergrund ist.',
          h('div', { class: 'stack' }, keyCapture(s.mark_key, code => save({ mark_key: code })), testKeyButton())),
        setting('Haltedauer', 'Wie lange die Taste gedrückt wird, in Millisekunden.',
          h('input', { type: 'number', min: 100, max: 5000, step: 100, value: s.mark_hold_ms, onchange: e => save({ mark_hold_ms: Number(e.target.value) }) })),
        setting('Stats-API-Adresse', 'Muss zum Port in der TAStatsAPI.ini passen.',
          h('input', { type: 'text', value: s.stats_api_addr, onchange: e => save({ stats_api_addr: e.target.value.trim() }) }))),
      card('Dein Spieler', h('div', { class: 'muted', style: { margin: '4px 0 10px' } }, 'Damit erkennt das Tool Sieg und Niederlage und hebt dich in Statistiken hervor. Mehrere Accounts (z. B. Steam und Epic) sind möglich.'),
        await identityEditor()),
      card('Hotkeys',
        setting('Overlay öffnen und schließen', null, hotkeyCapture(s.gui_hotkey, v => save({ gui_hotkey: v }))),
        setting('Pausieren und fortsetzen', 'Pausiert Uploads und Auto-Speichern zusammen.', hotkeyCapture(s.pause_hotkey, v => save({ pause_hotkey: v }))),
        setting('Schnell-Einordnen', 'Während das Fenster über dem Spiel sichtbar ist.', h('span', null, h('kbd', null, 'Strg+Alt+1'), ' bis ', h('kbd', null, 'Strg+Alt+4')))),
      card('Benachrichtigungen',
        setting('Upload erfolgreich', null, sw(s.notify_uploaded, v => save({ notify_uploaded: v }))),
        setting('Upload fehlgeschlagen', null, sw(s.notify_failed, v => save({ notify_failed: v }))),
        setting('Session beendet', 'Nach 30 Minuten ohne Spiel oder wenn Rocket League geschlossen wird.', sw(s.notify_session, v => save({ notify_session: v }))),
        setting('Schnell-Einordnen über dem Spiel', 'Zeigt nach dem Upload den Zielordner mit Countdown. Aus: Regeln ordnen ohne Rückfrage ein.', sw(s.quick_sort, v => save({ quick_sort: v }))),
        setting('Countdown', 'Sekunden bis der Regel-Ordner gilt.',
          h('input', { type: 'number', min: 3, max: 60, value: s.quick_sort_seconds, onchange: e => save({ quick_sort_seconds: Number(e.target.value) }) }))),
      card('Start',
        setting('Mit Windows starten', null, sw(s.autostart, v => run(() => api('/api/autostart', { method: 'POST', body: { on: v } }), v ? 'Startet mit Windows' : 'Startet nicht mehr mit Windows'))),
        setting('Minimiert starten', 'Beim Start nur das Symbol im Infobereich zeigen, das Overlay bleibt zu.', sw(s.start_minimized, v => save({ start_minimized: v })))),
      card('Erweitert',
        setting('Log', s.log_path, h('button', { onclick: () => run(() => api('/api/open-log', { method: 'POST' })) }, 'Log anzeigen')),
        setting('Einrichtung', 'Den Assistenten noch einmal durchlaufen.', h('a', { class: 'btn', href: '#/einrichtung' }, 'Assistent öffnen'))));
  }

  await render();
  ctx.on('settings', debounce(async () => { s = await api('/api/settings'); }, 300));
  return {};
}
