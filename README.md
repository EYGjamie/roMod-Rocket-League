# roMod

**Replay operations für Rocket League.** Spielen, speichern, fertig!

roMod läuft unauffällig im Hintergrund, lädt jedes Replay automatisch auf [ballchasing.com](https://ballchasing.com) hoch, sortiert es in den passenden Ordner und zeigt dir Verlauf und Statistiken deiner Spiele. Ohne BakkesMod, kompatibel mit Easy Anti-Cheat, für **Windows und Linux**.

## Warum roMod?

Bis April 2026 hat das BakkesMod-Plugin „AutoReplayUploader“ diese Arbeit erledigt. Seit Rocket League Easy Anti-Cheat nutzt, funktioniert das online nicht mehr und Replays von Hand zu speichern, hochzuladen und einzusortieren macht nach jedem Match niemand gern.

roMod löst das, **ohne ins Spiel einzugreifen**: Es nutzt nur die offizielle [Stats API](https://www.rocketleague.com/developer/stats-api) von Rocket League, den Replay-Ordner und einen einzigen Tastendruck. Nichts wird injiziert, kein Zugriff auf den Spielprozess.

## So funktioniert's

```text
 Match startet ──▶ Replay vormerken ──▶ Match endet ──▶ Upload ──▶ Einsortieren
 (Stats API)       (Speichern-Taste)    (Spiel speichert)  (ballchasing)  (deine Regeln)
```

1. **Match erkennen:** roMod hört der Stats API des Spiels zu und weiß, wann ein Match läuft.
2. **Replay vormerken:** Kurz nach dem ersten Anstoß drückt roMod einmal die Speichern-Taste. Das Spiel legt das Replay am Ende dann von selbst ab. Wichtig: Dass es funktioniert dürft ihr nicht direkt ins Freeplay gehen. Ihr müsst einmal aktiv den Dialog bestätigen.
3. **Hochladen:** Neue Replays gehen sofort an ballchasing, auf Wunsch wandert die Datei danach in den Papierkorb eures Pcs, heißt euer Speicher bleibt Leer :D
4. **Einsortieren:** Deine Regeln (nach Playlist und Mitspielern) wählen den Ordner. Ein kleiner Hinweis zeigt das Ziel mit Countdown, mit Strg+Alt+1–4 (in den Einstellungen änderbar) nimmst du eine Alternative.

## Was drin ist

- 🎮 **Overlay über dem Spiel** (Windows, Strg+Alt+B) bzw. die gleiche Oberfläche im Browser (Vollbild Nutzer müssen teils auf die WebVersion ausweichen)
- 📜 **Verlauf** aller deiner Replays, auch der Spiele, die Mitspieler hochgeladen haben. Durchsuchbar nach Session oder als Liste
- 📊 **Statistik** über 7 Tage, 14 Tage, einen Monat oder alles: Siegquote, Bilanz, Playlists, beste Maps
- 🔍 **Replay-Details** mit Boost, Bewegung, Position und Demos im Vergleich zu Lobby, Team oder Gegnern
- 📁 **Ordner und Regeln** direkt in der Oberfläche verwalten, Replays und ganze Ordner per Drag & Drop verschieben, Regelvorschläge aus deinem Einordnen
- 📈 **Ordner-Analyse** wie auf ballchasing: alle Spieler eines Ordners mit Kern, Boost, Bewegung, Position und Demos
- 📥 **Unsortiert:** was keine Regel einordnet, ist im Verlauf markiert und lässt sich per Auswahl in einen Ordner verschieben
- ▶️ **Ins Spiel laden:** gelöschte Replays von ballchasing zurückholen und im Spiel anschauen
- 🔄 **Automatische Updates** aus den GitHub-Releases nach kurzer Bestätigung von euch
- 🧪 **Entwicklermodus** (mit Passwort): Ränge und Siege der ganzen Lobby beim Matchstart, Smurfs per Hotkey markieren und auswerten (siehe [Anleitung](docs/anleitung.md#entwicklermodus-mmr-abfrage))

Einrichten geht in wenigen Minuten: Ein Assistent führt durch Token, Replay-Ordner, Speichern-Taste und Ordner und schaltet auf Wunsch auch die Stats API im Spiel mit einem Klick ein.

## Schnellstart

Du brauchst ein Konto auf [ballchasing.com](https://ballchasing.com) mit [Upload-Token](https://ballchasing.com/upload).

1. **Herunterladen:** die passende Datei aus den [Releases](https://github.com/EYGjamie/roMod-Rocket-League/releases) holen – oder selbst bauen mit [Go](https://go.dev/dl/) 1.26.2+:

   ```bash
   git clone https://github.com/EYGjamie/roMod-Rocket-League.git
   cd roMod-Rocket-League
   go build -trimpath -ldflags "-s -w -H windowsgui" -o dist/romod.exe ./cmd/romod
   ```

   (macOS: `scripts/macos-app.sh`, Linux: ohne `-H windowsgui`. Details in der [Anleitung](docs/anleitung.md#1-programm-bauen).)

2. **Starten:** Der Assistent öffnet sich und erledigt den Rest.
3. **Spielen.** Unter Windows am besten im Modus „Randloses Fenster“, damit Overlay und Hinweise über dem Spiel liegen, sonst mit zweitem Bildschrim arbeiten oder die Weboberfläche verwenden.

## Plattformen

Rocket League läuft nativ nur unter Windows, unter Linux per Proton oder Wine, auf dem Mac per CrossOver oder Whisky. Hochladen, Einsortieren, Verlauf und Statistik funktionieren überall.

| | Windows | macOS | Linux |
|---|---|---|---|
| Upload, Regeln, Verlauf, Statistik | ✓ | ✓ | ✓ |
| Overlay über dem Spiel | ✓ | Browser | Browser |
| Automatisches Vormerken | ✓ | – (Hinweiston) | ✓ (X11/XWayland) |
| Schnell-Einordnen | Fenster über dem Spiel | – (Regeln direkt) | Benachrichtigung mit Knöpfen |

Die vollständige Übersicht steht in der [Anleitung](docs/anleitung.md#systeme).

## Gut zu wissen

- **Anti-Cheat:** roMod sendet nur einen normalen Tastendruck über das System. Eine offizielle Aussage von Psyonix dazu gibt es nicht, sollte jedoch nicht gebannt werden. Wer kein Risiko will, schaltet das automatische Speichern aus und speichert von Hand. Der Upload läuft trotzdem automatisch.
- **Chat:** Ist im Moment des Tastendrucks der Chat offen, löscht die Rücktaste dort Zeichen. Somit kurz warten oder Tastenbelegung ändern.
- **Eine Kopie:** Mit „nach dem Upload löschen“ liegt das Replay nur noch auf ballchasing.com.

## Unter der Haube

Geschrieben in Go, mit SQLite als lokaler Datenbank und einer Oberfläche aus reinem HTML, CSS und JavaScript ohne Build-Schritt. Systemspezifisches steckt in `_windows.go`, `_darwin.go` und `_linux.go`. Die CI testet und baut auf allen drei Systemen.

Die Oberfläche lässt sich mit erfundenen Daten ausprobieren, ganz ohne ballchasing-Konto:

```bash
go run ./cmd/uidemo
```

## Mehr erfahren

- [Anleitung](docs/anleitung.md) – Installation, Benutzung, Konfiguration, Fehlerbehebung und Entwicklung im Detail
