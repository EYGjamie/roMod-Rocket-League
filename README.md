# auto-replay-upload

Lädt Rocket-League-Replays automatisch auf [ballchasing.com](https://ballchasing.com) hoch, auch mit Easy Anti-Cheat und ohne BakkesMod.

Seit Easy Anti-Cheat (April 2026) läuft das BakkesMod-Plugin „AutoReplayUploader“ online nicht mehr. Dieses Tool greift nicht ins Spiel ein und erledigt drei Dinge:

1. **Match erkennen** über die offizielle [Stats API](https://www.rocketleague.com/developer/stats-api) des Spiels (lokaler TCP-Port).
2. **Replay vormerken:** Kurz nach dem ersten Anstoß hält das Tool einmal **Backspace** gedrückt (ca. 1,2 s). Das ist die eingebaute Taste, mit der das Spiel das Replay am Matchende automatisch speichert.
3. **Hochladen:** Neue `.replay`-Dateien im Replay-Ordner werden über die ballchasing-API hochgeladen.

Hat das Vormerken nicht geklappt (zum Beispiel weil ein anderes Fenster im Vordergrund war), gibt es am Matchende einen Warnton. Dann im Spiel „Replay speichern“ wählen; der Upload passiert trotzdem automatisch.

## Einrichtung

### 1. Stats API aktivieren

Rocket League beenden. Im Installationsordner die Datei `TAGame\Config\TAStatsAPI.ini` öffnen (gibt es sie nicht, dann `DefaultStatsAPI.ini`) und im Abschnitt `[TAGame.MatchStatsExporter_TA]` setzen:

```ini
[TAGame.MatchStatsExporter_TA]
PacketSendRate=1
Port=49123
```

Bei `PacketSendRate=0` (Standard) ist die API aus. Übliche Installationsordner:

- Steam: `C:\Program Files (x86)\Steam\steamapps\common\rocketleague`
- Epic: `C:\Program Files\Epic Games\rocketleague`

Nach Spielupdates prüfen, ob der Wert noch gesetzt ist.

### 2. Taste im Spiel prüfen

Unter Einstellungen → Steuerung muss die Aktion zum Speichern bzw. Vormerken des Replays auf **Backspace** liegen (Standard). Ausprobieren: In einem Match Backspace etwa eine Sekunde halten, dann meldet das Spiel, dass das Replay am Ende gespeichert wird.

### 3. Tool starten

1. `auto-replay-upload.exe` in einen eigenen Ordner legen und starten. Beim ersten Start wird daneben eine `config.json` angelegt.
2. Den Upload-Token von [ballchasing.com/upload](https://ballchasing.com/upload) bei `ballchasing_token` eintragen.
3. Erneut starten und das Fenster offen lassen, während du spielst.

Beim allerersten Start werden vorhandene Replays nur registriert, nicht hochgeladen. Danach merkt sich das Tool in `uploaded.json`, was erledigt ist. Replays, die gespeichert werden, während das Tool nicht läuft, werden beim nächsten Start nachgeholt.

## Replays gruppieren

**Strg+Alt+B** öffnet ein Overlay-Fenster über dem Spiel, in dem du:

- eine bestehende ballchasing-Gruppe wählst oder eine neue anlegst („Teams mit Wechselspielern“ schaltet die Teamerkennung auf `by-player-clusters`),
- deine hochgeladenen Replays filterst (Spieler, Map, Modus, Gruppe) und anhakst,
- die ausgewählten Replays mit einem Klick in die Gruppe verschiebst.

Das Overlay ist rahmenlos, bleibt immer im Vordergrund und erscheint mittig auf dem Monitor, auf dem das Spiel läuft. **Esc**, das ✕ oder nochmal Strg+Alt+B schließen es, danach ist das Spiel wieder aktiv. Beim nächsten Öffnen lädt es die Replay-Liste neu.

- **Rocket League im Anzeigemodus „Randloses Fenster“ spielen.** Im echten Vollbild kann kein anderes Fenster darüber liegen; das Spiel wird dann beim Öffnen minimiert.
- Das Overlay ist ein eigenes Fenster und wird nicht ins Spiel eingeblendet (das würde Easy Anti-Cheat blockieren). Solange es offen ist, hat das Spiel keinen Fokus.
- Es nutzt die WebView2-Laufzeit, die bei Windows 10 und 11 dabei ist. Fehlt sie, öffnet sich die Seite stattdessen im Browser.

Die Oberfläche läuft intern als lokale Seite (`127.0.0.1`), die nur Anfragen mit einem bei jedem Start neuen Schlüssel annimmt. Ist kein Hotkey gesetzt oder ist die Kombination schon belegt, steht der Link zur Seite im Konsolenfenster.

Verschieben ersetzt die bisherige Gruppe eines Replays, denn die API kennt pro Replay nur ein Gruppenfeld. Wegen des Rate-Limits (2 Änderungen pro Sekunde) dauert das Verschieben etwa 0,6 s pro Replay.

## Konfiguration

| Feld | Standard | Bedeutung |
|---|---|---|
| `ballchasing_token` | – | Upload-Token (alternativ Umgebungsvariable `BALLCHASING_TOKEN`) |
| `visibility` | `private` | `public`, `unlisted` oder `private` |
| `group` | – | ballchasing-Gruppen-ID, in die hochgeladen wird |
| `demo_dirs` | automatisch | Replay-Ordner; leer = `Dokumente\My Games\Rocket League\TAGame\Demos` und `DemosEpic` |
| `stats_api_addr` | `127.0.0.1:49123` | Adresse der Stats API (muss zu `Port` in der ini passen) |
| `auto_mark` | `true` | `false` = kein Tastendruck, nur Warnton am Matchende |
| `mark_hold_ms` | `1200` | Wie lange Backspace gehalten wird |
| `online_only` | `true` | Nur Online-/LAN-Matches vormerken, Offline-Matches und Training ignorieren |
| `gui_hotkey` | `Ctrl+Alt+B` | Öffnet und schließt das Overlay. Modifikatoren `Ctrl`/`Strg`, `Alt`, `Shift`, `Win` plus Buchstabe, Ziffer oder `F1`–`F24`; leer = kein Hotkey |

## Verhalten im Detail

- Vorgemerkt wird einmal pro Match, 3–8 Sekunden nach dem ersten Anstoß (zufällig).
- Die Taste wird **nur** gesendet, wenn `RocketLeague.exe` das aktive Fenster ist. Sonst versucht es das Tool alle 3 Sekunden erneut, bis das Match endet.
- Beim Anschauen gespeicherter Replays wird nichts gedrückt.
- Uploads, die fehlschlagen (Netzwerk, Rate-Limit), werden mit wachsendem Abstand wiederholt (30 s bis 10 min). Replays, die ballchasing ablehnt, werden übersprungen. Bereits vorhandene Replays zählen als erledigt.

## Risiken und Grenzen

- **Anti-Cheat:** Das Tool liest nur die offizielle Stats API und Dateien und sendet einen einzelnen Tastendruck über die normale Windows-Eingabe (`SendInput`). Es injiziert nichts und öffnet keinen Handle auf den Spielprozess. Psyonix verbietet Makros, die einen Vorteil im Spiel bringen; eine offizielle Aussage zu einem Tastendruck fürs Replay-Speichern gibt es nicht. Wer gar kein Risiko will, setzt `auto_mark` auf `false`.
- **Chat:** Ist im Moment des Tastendrucks der Chat offen, löscht Backspace dort Zeichen.
- **Konsole:** Funktioniert nicht, dort gibt es keinen Zugriff auf Replay-Dateien.
- **Plattform:** Tastendruck, Hotkey und Overlay funktionieren nur unter Windows. Auf anderen Systemen laufen nur der Upload (`demo_dirs` muss dann gesetzt sein) und die Gruppen-Seite über den Link in der Konsole.

## Bauen und testen

Benötigt Go 1.26. Die Windows-exe lässt sich auch auf macOS oder Linux bauen:

```bash
GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o dist/auto-replay-upload.exe .
```

```bash
go test ./...
```
