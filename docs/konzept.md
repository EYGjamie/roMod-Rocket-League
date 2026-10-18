# Konzept: auto-replay-upload 2

Stand: 18.10.2026, umgesetzt. Grundlage sind die Funktionen aus den Mockups, ergänzt um fehlende Abläufe, die Antworten aus der Abstimmung und das, was die ballchasing-API tatsächlich hergibt.

Markierungen: **[da]** gab es vor der Umsetzung schon, **[Umbau]** wurde erweitert, **[neu]** kam neu dazu. Alle fünf Etappen sind umgesetzt. Wo die Umsetzung vom Entwurf abweicht, steht das im Text und gesammelt unter [Stand der Umsetzung](#15-stand-der-umsetzung). Die Annahmen aus [Abschnitt 14](#14-offene-punkte-und-annahmen) sind bestätigt.

## Inhalt

1. [Ziel und Rahmen](#1-ziel-und-rahmen)
2. [Getroffene Entscheidungen](#2-getroffene-entscheidungen)
3. [Architektur](#3-architektur)
4. [Ablauf pro Match](#4-ablauf-pro-match)
5. [Warteschlange, Fehler, Pause](#5-warteschlange-fehler-pause)
6. [Datenbasis: Abgleich mit ballchasing](#6-datenbasis-abgleich-mit-ballchasing)
7. [Oberfläche](#7-oberfläche)
8. [Ordner, Regeln und Eingang](#8-ordner-regeln-und-eingang)
9. [Schnell-Einordnen und Benachrichtigungen](#9-schnell-einordnen-und-benachrichtigungen)
10. [Löschen nach Upload und „Ins Spiel laden“](#10-löschen-nach-upload-und-ins-spiel-laden)
11. [Einstellungen und Erst-Einrichtung](#11-einstellungen-und-erst-einrichtung)
12. [Grenzen der ballchasing-API](#12-grenzen-der-ballchasing-api)
13. [Umsetzung in Etappen](#13-umsetzung-in-etappen)
14. [Offene Punkte und Annahmen](#14-offene-punkte-und-annahmen)
15. [Stand der Umsetzung](#15-stand-der-umsetzung)
16. [macOS und Linux](#16-macos-und-linux)
- [Anhang A: Felder eines Replays](#anhang-a-felder-eines-replays)
- [Anhang B: Standard-Regeln](#anhang-b-standard-regeln)

## 1. Ziel und Rahmen

Rocket-League-Replays landen ohne Zutun auf ballchasing.com, werden dort in Ordner sortiert und lassen sich mit Statistik und Verlauf durchsuchen. Die Festplatte bleibt dabei auf Wunsch leer.

Fester Rahmen, der sich aus Easy Anti-Cheat ergibt:

- Das Tool greift nicht ins Spiel ein. Es liest die offizielle Stats API und den Replay-Ordner und sendet höchstens die Speichern-Taste über `SendInput` (unter Linux über XTEST), und das nur, wenn Rocket League im Vordergrund ist.
- Alles, was „im Spiel“ erscheint, ist ein eigenes Fenster über dem Spiel. Das funktioniert nur im Anzeigemodus „Randloses Fenster“.
- Hauptplattform ist Windows 10/11, wo das Spiel läuft. macOS und Linux werden ebenfalls unterstützt, mit den Unterschieden aus [Abschnitt 16](#16-macos-und-linux).

## 2. Getroffene Entscheidungen

| Thema | Entscheidung |
|---|---|
| Oberfläche | Eine Web-Oberfläche, die im Overlay und im Browser **identisch** ist. Kein zusätzliches App-Fenster. |
| Datenbasis | Dein ganzes ballchasing-Konto **plus** alle Spiele mit dir, die andere hochgeladen haben. Dasselbe Spiel erscheint nur einmal (Abgleich über `match_guid`), dein eigener Upload hat Vorrang. |
| Dein Spieler | Wird automatisch erkannt (häufigster Spieler in deinen Replays) und im Assistenten bestätigt. Mehrere Accounts wie Epic und Steam sind möglich. |
| Session | Endet nach 30 Minuten ohne Spiel oder wenn Rocket League geschlossen wird. |
| Einordnen | Passt eine Regel, zeigt Schnell-Einordnen ihren Ordner mit Countdown, die Tasten 1–4 wählen eine Alternative. Ohne Regel geht das Replay in den Eingang. **Eingang = „Unsortiert“**: umgesetzt als alle Replays ohne ballchasing-Gruppe, im Ordnerbaum ganz oben als „Eingang (unsortiert)“. Eine echte Gruppe „Unsortiert“ gibt es nicht. |
| Schnell-Einordnen im Spiel | Kleines Fenster über dem Spiel, das den Fokus nicht nimmt. Auswahl per Strg+Alt+1–4. Die übrigen Hinweise kommen als eigene kleine Hinweisfenster statt als Windows-Benachrichtigung ([9.2](#92-benachrichtigungen-neu)). |
| Standard-Regeln | Nach Playlist, **Ranked und Casual getrennt**, Extra-Modi einzeln, dazu Private und Turniere ([Anhang B](#anhang-b-standard-regeln)). |
| Ins Spiel laden | Ja. Gelöschte Replays lassen sich zum Anschauen wieder in den Demos-Ordner holen und werden nach dem Anschauen automatisch entfernt. |

## 3. Architektur

```text
Rocket League (Stats API, TCP) ──▶ ┐
Demos-Ordner ────────────────────▶ ├─ Hintergrunddienst (Go, ein Prozess im Tray)
ballchasing.com (HTTPS) ◀────────▶ ┘    • Match-Tracker und Speichern-Taste
                                        • Ordner-Watcher und Upload-Warteschlange
                                        • Abgleich: eigene Replays, Spiele mit dir, Gruppen
                                        • Regeln, Vorschläge, Schnell-Einordnen-Fenster
                                        • Lokale Datenbank (SQLite)
                                        • Tray, Hotkeys, Benachrichtigungen, Autostart
                                        • Lokaler HTTP-Server: Oberfläche, JSON-API, Live-Events
                                              │  http://127.0.0.1:<port>/?k=<Schlüssel>
                                              ├──▶ Overlay (WebView2)    ┐ identische
                                              └──▶ Browser               ┘ Oberfläche
```

- **Ein Prozess** wie heute, im Tray, mit Autostart **[da]**.
- **Web-Oberfläche** **[Umbau]**: aus der heutigen Gruppen-Seite wird eine App mit Navigation. Sie wird vom lokalen Server ausgeliefert und im Overlay (Strg+Alt+B, Linksklick aufs Symbol) oder im Browser geöffnet. Änderungen kommen per Server-Sent Events live in beiden an.
- **Lokale Datenbank** **[neu]**: SQLite (reines Go, ohne cgo) neben der `config.json`. Sie ersetzt `uploaded.json`, das beim ersten Start übernommen wird. Inhalt:
  - Replays: ballchasing-ID, `match_guid`, eigener oder fremder Upload, Kopfdaten, Teams, Ergebnis, zwischengespeicherte Details
  - lokale Dateien mit ihrem Status
  - Warteschlange
  - Ordner-Baum
  - Regeln
  - Einordnungs-Entscheidungen, als Grundlage für Vorschläge
  - deine Spieler-IDs
- **Einstellungen** bleiben in der `config.json`. Die App schreibt sie selbst, eine Änderung gilt sofort ohne Neustart.
- **Sicherheit**: Der lokale Server nimmt wie heute nur Anfragen an 127.0.0.1 mit einem pro Start neuen Schlüssel an **[da]**.

## 4. Ablauf pro Match

1. **Spielstart erkennen** **[da]**: Die Stats API meldet den ersten Anstoß. Die laufende Session wird fortgesetzt oder eine neue beginnt.
2. **Speichern-Taste drücken** **[Umbau]**: 3–8 s nach dem ersten Anstoß, einmal pro Match, nur wenn Rocket League im Vordergrund ist. Die Taste ist jetzt einstellbar (Standard Backspace). Hat es bis Matchende nicht geklappt, gibt es den Warnton **[da]**.
3. **Neue Datei erkennen** **[da]**: Der Watcher meldet eine neue `.replay`-Datei, sobald sich ihre Größe nicht mehr ändert. Dateien, die „Ins Spiel laden“ geholt hat, ignoriert er.
4. **Hochladen** **[Umbau]**: Upload mit der Standard-Sichtbarkeit, noch ohne Ordner.
   - Antwort `201`: eigener neuer Upload.
   - Antwort `409`: Das Replay ist schon auf ballchasing und wird mit dem vorhandenen verknüpft. Ist der Uploader nicht dein Konto, gilt es als fremd: kein Einordnen, kein Löschen.
5. **Auf die Verarbeitung warten** **[neu]**: `GET /replays/{id}` bis `status` = `ok`, nötig, weil erst ballchasing die Playlist bestimmt. Bei `failed` steht der Eintrag in der Warteschlange als „von ballchasing nicht lesbar“.
6. **Details speichern** **[neu]**: alle Felder aus [Anhang A](#anhang-a-felder-eines-replays) in die lokale Datenbank, damit Verlauf und Detailansicht ohne weitere API-Aufrufe auskommen.
7. **Einordnen** **[neu]**:
   - Die Regeln liefern einen Zielordner, und das Schnell-Einordnen-Fenster erscheint mit Countdown ([Abschnitt 9](#9-schnell-einordnen-und-benachrichtigungen)).
   - Am Ende des Countdowns oder nach Tastendruck setzt das Tool die Gruppe per `PATCH`.
   - Ohne passende Regel kommt das Replay in den Eingang.
8. **Lokale Datei löschen** **[neu]**, wenn eingeschaltet und alle Sicherheitsbedingungen erfüllt sind ([Abschnitt 10](#10-löschen-nach-upload-und-ins-spiel-laden)).
9. **Rückmeldung** **[neu]**: Benachrichtigung „hochgeladen“ (abschaltbar), letzte Uploads und Session-Übersicht aktualisieren sich live.

## 5. Warteschlange, Fehler, Pause

**Zustände eines Eintrags**

| Zustand | Bedeutung |
|---|---|
| wartet | Datei erkannt, Upload steht an |
| lädt hoch / wird verarbeitet | Upload läuft bzw. ballchasing verarbeitet noch (`pending`) |
| fertig | hochgeladen, verarbeitet, ggf. eingeordnet |
| neuer Versuch in 4:12 | fehlgeschlagen, automatischer Neuversuch mit Countdown |
| fehlgeschlagen | 3 Versuche verbraucht, keine automatischen Versuche mehr |
| abgelehnt | ballchasing kann die Datei nicht verarbeiten, ein Neuversuch ist sinnlos |
| offline | ballchasing nicht erreichbar, Warteschlange wartet |

**Neuversuche** **[Umbau]**

- Bis zu 3 Versuche: sofort, nach 1 min, nach 5 min. Danach „fehlgeschlagen“ mit Fehlergrund.
- **Zählen nicht als Versuch** (Annahme): Netzwerkausfall, ballchasing nicht erreichbar, Rate-Limit (`429`). Die Warteschlange steht dann auf „offline“ und läuft von selbst weiter, sobald ballchasing antwortet. Sonst wären nach einer kurzen Internetstörung alle Uploads endgültig fehlgeschlagen.
- Ist der Token ungültig, hält die ganze Warteschlange an, und oben erscheint ein Hinweis mit dem Weg in die Einstellungen.
- **„Jetzt erneut“** pro Eintrag und **„Alle fehlgeschlagenen erneut senden“** setzen den Versuchszähler zurück.
- **Fehler-Badge** in der Navigation und als roter Punkt am Tray-Symbol: Anzahl der Einträge „fehlgeschlagen“ und „abgelehnt“.
- Die Warteschlange steht in der Datenbank und übersteht Neustarts.

**Pausieren** **[neu]**

- **Strg+Alt+P** pausiert Überwachung **und** Auto-Speichern zusammen (Annahme, in der Liste hatten beide denselben Hotkey). Einzeln umschalten lässt sich beides im Tray-Menü und in der Übersicht.
- Pausiert heißt: Es wird keine Taste gedrückt und nichts hochgeladen. Das Tray-Symbol ist grau, und der Tooltip zeigt die letzte Meldung („Pausiert: keine Uploads, kein Auto-Speichern“). Einzeln umschalten geht in der Übersicht; das Tray-Menü schaltet beides zusammen.
- Dateien, die während der Pause entstehen, werden nach dem Fortsetzen nachgeholt (Annahme).

## 6. Datenbasis: Abgleich mit ballchasing

**Quellen**

| Quelle | Abruf | Rechte |
|---|---|---|
| Eigene Replays | `GET /replays?uploader=me`, seitenweise, beim ersten Mal komplett, danach nur Neues (`created-after`) | alles: verschieben, Sichtbarkeit, löschen |
| Spiele mit dir von anderen | `GET /replays?player-id=<plattform:id>` für jede deiner IDs | nur ansehen; private Replays anderer sind nicht sichtbar |
| Gruppen | `GET /groups?creator=me` plus `GET /groups?group=<id>` je Gruppe für die Untergruppen | eigene Gruppen |

- **Duplikate**: Gleiche `match_guid` heißt gleiches Spiel. Angezeigt wird ein Eintrag, bevorzugt dein eigener Upload. In der Detailansicht steht, wer es noch hochgeladen hat.
- **Dein Spieler**: Der häufigste Spieler über deine Replays wird vorgeschlagen. Zwei IDs, die oft vorkommen, aber nie im selben Spiel, sind vermutlich deine beiden Accounts (Epic und Steam) und werden zusammen vorgeschlagen. Bestätigt wird im Assistenten, änderbar in den Einstellungen.
- **Sieg oder Niederlage**: Dein Team ist das mit einer deiner IDs, die Tore kommen aus der Replay-Liste. Spiele ohne dich (etwa als Zuschauer aufgenommen) zählen in der Statistik nicht mit und sind im Verlauf als „ohne dich“ markiert.
- **Details nachladen**: Die Replay-Liste enthält pro Spieler nur Name, Score und MVP. Tore, Vorlagen, Paraden und Schüsse brauchen `GET /replays/{id}`.
  - Für neue Uploads holt das Tool die Details sofort.
  - Für ältere Replays beim Aufklappen im Verlauf.
  - Dazu im Hintergrund gedrosselt (1 Abruf alle 6 s, also höchstens 600 pro Stunde bei erlaubten 1000), bis alles vollständig ist. So bleibt Luft für Uploads und das Aufklappen.
- **Rate-Limits**: Ein zentraler Begrenzer pro Endpunkt verteilt die Aufrufe. Bei `429` wartet er und versucht es erneut.
- **Sessions** werden aus den Spielzeiten berechnet (über 30 Minuten Abstand = neue Session) und nicht gespeichert.

## 7. Oberfläche

Eine Web-App, im Overlay und im Browser gleich. Navigation:

| Bereich | Badge |
|---|---|
| Übersicht | Fehler-Badge |
| Verlauf | – |
| Statistik | – |
| Ordner | – |
| Eingang | Anzahl unsortierter Replays |
| Regeln | neue Regel-Vorschläge |
| Einstellungen | Hinweis, wenn die Einrichtung unvollständig ist |

### 7.1 Übersicht

- **Statuszeile**: ballchasing verbunden (Kontoname), Rocket League verbunden (Stats API), Überwachung aktiv/pausiert, Auto-Speichern aktiv/pausiert, jeweils mit Schalter.
- **Warteschlange**: fehlgeschlagene und laufende Einträge mit Fehlergrund, Countdown, „Jetzt erneut“ und „Alle erneut senden“.
- **Letzte Uploads**: Map, Playlist, Ergebnis (Sieg/Niederlage mit Endstand), Dauer, Uhrzeit, Ordner, Status.
- **Session**: Anzahl Spiele, Bilanz (z. B. 5–3), Siegesfolge als Streifen aus grünen und roten Kästchen.
- **Aktionen**: „Replays hochladen“ (Dateiauswahl oder Dateien auf die Seite ziehen), „Replay-Ordner öffnen“.

### 7.2 Verlauf

- **Suche** über Map, Spielername und Dateiname.
- **Filter**:
  - Status: Alle / Hochgeladen / Fehler
  - Playlist
  - Zeitraum
  - Ordner
  - Herkunft: eigene / von Mitspielern (Ergänzung)
- **Zeile**: Endstand mit Teamnamen, Sieg/Niederlage, Map, Playlist, Dauer, Uhrzeit, Ordner. Replays von anderen tragen „hochgeladen von …“.
- **Schnellaktionen**: auf ballchasing öffnen, dazu ein Menü:
  - Ordner ändern
  - Sichtbarkeit ändern
  - Ins Spiel laden
  - Datei im Ordner zeigen (wenn lokal vorhanden)
  - Auf ballchasing löschen
  - Bei fremden Replays gibt es nur Öffnen und Ins Spiel laden.
- **Aufgeklappt**: Spielerstatistik beider Teams mit Punkten, Toren, Vorlagen, Paraden und Schüssen. Du bist hervorgehoben.
- **Detailansicht** **[neu]**: alle Felder aus [Anhang A](#anhang-a-felder-eines-replays), aufgeteilt in Reiter:

  | Reiter | Inhalt |
  |---|---|
  | Überblick | Kopfdaten, Server, Ergebnis, Ballbesitz |
  | Kern | `core` je Spieler und Team |
  | Boost | `boost` |
  | Bewegung | `movement` |
  | Position | `positioning` |
  | Demos | `demo` |
  | Kamera & Auto | `camera`, `car_name`, `steering_sensitivity` |

  Prozentwerte werden als Balken gezeigt, Zeiten als mm:ss, Distanzen gerundet.

### 7.3 Statistik

- **Zeitraum**: 7 Tage, 14 Tage, Monat (= letzte 30 Tage, Annahme), Gesamt.
- **Kennzahlen**: Uploads, Spiele, Siegquote.
- **Spiele pro Tag**: gestapelte Balken für Siege und Niederlagen.
- **Verteilung nach Playlist**: Anzahl und Siegquote je Playlist.
- **Beste Maps**: Siegquote je Map, erst ab 3 Spielen.
- **Vorschlag für später**: deine Durchschnittswerte (Tore, Schüsse, Paraden, Schussquote, Boost pro Minute, Zeit mit Überschall), sobald die Details nachgeladen sind.

Diagramme sind einfache SVGs ohne zusätzliche Bibliothek.

## 8. Ordner, Regeln und Eingang

**Ordner = ballchasing-Gruppe.** Ein Replay kann laut API nur in einer Gruppe stecken. Verschieben ersetzt daher die alte Gruppe.

### 8.1 Ordner

- **Baum** mit Unterordnern und Anzahl, direkt und inklusive Unterordnern (`direct_replays`, `indirect_replays`) **[Umbau]**, dazu oben der Ordner **„Unsortiert“** (= Eingang).
- **Neuer Ordner / neuer Unterordner** **[da]**.
- **Abgleich** mit ballchasing beim Öffnen und über einen Knopf „Aktualisieren“. Ordner, die auf der Website angelegt wurden, erscheinen hier.
- **Mehrfachauswahl** mit Verschieben, Sichtbarkeit und Löschen (auf ballchasing, mit Rückfrage und Anzahl, nicht rückgängig zu machen).
- **Verschieben** per Drag & Drop auf einen Ordner im Baum oder über das Menü.
- **Intelligente Vorschläge** über den unsortierten Replays, zum Beispiel „4 private Spiele → Privat & Training“, mit „Übernehmen“ und „Ignorieren“. Sie entstehen aus Regeln und früheren Entscheidungen für dieselbe Playlist und dieselben Mitspieler.
  - Gewichtung (umgesetzt): Frühere Entscheidungen mit gleicher Playlist **und** gleichen Mitspielern zählen am meisten, dann gleiche Playlist mit einem Teil der Mitspieler, dann gleiche Playlist allein. Gleiche Mitspieler ohne gleiche Playlist zählen nur wenig, sonst landete ein Casual-Spiel mit dem Ranked-Partner im Ranked-Ordner. Zuletzt genutzte Ordner bekommen einen kleinen Bonus.
- **Sessions**: Unsortierte Replays sind nach Session gruppiert. Pro Session gibt es „Alle nach …“ und „Aufteilen“. „Aufteilen“ schlägt pro Playlist einen Ordner vor.

### 8.2 Regeln

- **Bedingungen**: Playlist (eine oder mehrere), Mitspieler (einer oder alle aus einer Auswahl bekannter Spieler, über die Spieler-ID statt den Namen, weil Namen sich ändern). Beides lässt sich kombinieren.
- **Ziel**: ein Ordner oder Unterordner. Fehlt er, legt die App ihn beim ersten Treffer an.
- **Reihenfolge**: Die erste passende Regel gewinnt, Regeln lassen sich per Drag & Drop sortieren. Spezielle Regeln (mit Mitspielern) gehören vor allgemeine (nur Playlist).
- **Standard-Regeln** legt der Assistent an ([Anhang B](#anhang-b-standard-regeln)).
- **Regel-Vorschläge**: Ordnest du mindestens 3 Replays mit derselben Playlist und denselben Mitspielern von Hand in denselben Ordner, schlägt die App eine Regel dafür vor.

### 8.3 Eingang

- Enthält alles, was keine Regel eingeordnet hat, plus Replays, deren Countdown mit „Eingang“ beendet wurde.
- **Pro Replay ein Vorschlag**: Zielordner mit kurzer Begründung („wie 5 frühere Spiele mit Max“).
- **Tastatur**:

  | Taste | Aktion |
  |---|---|
  | 1–4 | in Ordner 1–4 der Vorschlagsliste |
  | N | neuer Ordner |
  | E | überspringen |
  | J / K | nächstes / vorheriges Replay |
  | Enter | Vorschlag übernehmen |
  | O | auf ballchasing öffnen |

## 9. Schnell-Einordnen und Benachrichtigungen

### 9.1 Schnell-Einordnen-Fenster **[neu]**

- **Wann**: sobald ein Replay verarbeitet ist und eine Regel passt. Das ist meist noch auf dem Ergebnisbildschirm oder im nächsten Match.
- **Wo**: klein, oben rechts auf dem Monitor des Spiels, immer im Vordergrund, nimmt dem Spiel nicht den Fokus. Es ist nur im Modus „Randloses Fenster“ sichtbar.
- **Inhalt**: Map, Playlist, Ergebnis, Zielordner der Regel und Countdown-Balken (Standard 10 s, einstellbar).
- **Tasten**, nur registriert, solange das Fenster sichtbar ist:
  - Strg+Alt+1–3 wählt einen der drei Alternativ-Ordner (häufig und zuletzt genutzte für diese Playlist).
  - Strg+Alt+4 schickt das Replay in den Eingang.
  - Läuft der Countdown ab, gilt der Regel-Ordner.
- **Im echten Vollbild** ist das Fenster nicht sichtbar. Die Tasten funktionieren trotzdem, und die Entscheidung steht danach in der Benachrichtigung.
- Mehrere Replays hintereinander werden nacheinander gezeigt.

### 9.2 Benachrichtigungen **[neu]**

Eigene kleine Hinweisfenster unten rechts mit Aktionen, jede Art einzeln abschaltbar (auch direkt im Hinweis über „Nicht mehr anzeigen“):

| Benachrichtigung | Aktionen |
|---|---|
| Hochgeladen: „2s Ranked · Champions Field · Sieg 3:1 → 2s Ranked“ | Auf ballchasing öffnen, Anderer Ordner |
| Fehlgeschlagen: „Upload fehlgeschlagen: ballchasing antwortet nicht“ | Jetzt erneut, Details |
| Session beendet: „8 Spiele, 5–3, längste Siegesfolge 3“ | Session einordnen, Statistik |

- „Hinweis nach jedem Upload“ aus der Einstellungsliste ist dieselbe Einstellung wie „Hochgeladen“.
- **Abweichung vom Entwurf:** Geplant waren Windows-Benachrichtigungen. Die bräuchten bei einem Programm ohne Installer eine Startmenü-Verknüpfung mit App-ID, und Windows hält sie während eines Spiels meist zurück. Umgesetzt sind deshalb eigene Fenster wie beim Schnell-Einordnen: immer im Vordergrund, ohne Fokus, höchstens 3 gleichzeitig, jedes verschwindet nach 8 s.
- Während eines Matches hält das Tool Hinweise zurück und zeigt sie gesammelt, sobald das Match vorbei ist. Das Schnell-Einordnen erscheint dagegen sofort.

### 9.3 Tray **[Umbau]**

- **Linksklick**: Overlay öffnen **[da]**.
- **Menü**:
  - Pausieren / Fortsetzen
  - Replays hochladen…
  - Replay-Ordner öffnen
  - Overlay öffnen
  - Im Browser öffnen
  - Einstellungen
  - Beenden
- **Symbol**: normal, grau (pausiert), mit rotem Punkt (Fehler).
- **Tooltip**: letzte Meldung **[da]**.

## 10. Löschen nach Upload und „Ins Spiel laden“

### 10.1 Löschen nach Upload **[neu]**, Einstellung, Standard aus

Gelöscht wird eine Datei nur, wenn **alle** Bedingungen erfüllt sind:

1. Die App hat sie selbst hochgeladen, mit Antwort `201`, oder `409` und dein Konto ist der Uploader.
2. ballchasing hat sie verarbeitet (`status` = `ok`).
3. Sie wurde nicht durch „Ins Spiel laden“ geholt.
4. Sie ist keine der Altdateien, die beim allerersten Start nur registriert und nie hochgeladen wurden.
5. Sie kommt aus dem Replay-Ordner. Über die Oberfläche hochgeladene Dateien (Kopien in `manuell\`) bleiben liegen (in der Umsetzung ergänzt).

Die Datei wandert in den **Papierkorb**, nicht endgültig weg (Annahme). So ist ein versehentliches Löschen in den ersten Tagen noch zu retten.

### 10.2 Ins Spiel laden **[neu]**

1. „Ins Spiel laden“ im Verlauf holt die Datei über `GET /replays/{id}/file` in den Demos-Ordner, als `bc-<id>.replay`.
2. Das Replay erscheint in der Replay-Liste des Spiels. Unklar ist, ob sofort oder erst nach erneutem Öffnen des Menüs; das muss getestet werden.
3. **Aufräumen**: Das Tool erkennt über die Stats API, wann du ein Replay verlässt (Ereignis `ReplayCreated`, danach `MatchDestroyed`), und löscht dann die geholten Dateien.
   - Einzelne lassen sich mit „Behalten“ davon ausnehmen.
   - Reste ohne passendes Ereignis räumt das Tool beim nächsten Start auf, wenn sie älter als 24 h sind.
4. Der Watcher lädt `bc-*.replay`-Dateien nicht erneut hoch.

Grenzen:

- Für Konten ohne Patreon erlaubt ballchasing 1 Download pro Sekunde und 200 pro Stunde.
- Nach manchen großen Spiel-Updates spielt Rocket League ältere Replays nicht mehr ab. Das gilt genauso für lokal gespeicherte.
- Ohne Datei geht es nicht: Das Spiel spielt nur Dateien aus dem Demos-Ordner ab.

## 11. Einstellungen und Erst-Einrichtung

### 11.1 Einstellungen

Jede Änderung wird sofort gespeichert und wirkt ohne Neustart (Hotkeys werden neu registriert, der Watcher übernimmt neue Ordner).

| Gruppe | Einstellung | Standard |
|---|---|---|
| ballchasing | Token: eintragen, ändern, trennen; Status mit Kontoname (verbunden / ungültig / offline). Ein gespeicherter Token wird nicht wieder angezeigt (umgesetzt ohne „anzeigen“). | – |
| ballchasing | Standard-Sichtbarkeit: Öffentlich / Nicht gelistet / Privat | Privat |
| Replays | Replay-Ordner: erkannt `Demos` und `DemosEpic`, hinzufügen und entfernen, Anzahl Replays je Ordner | automatisch |
| Replays | Vorhandene, noch nicht hochgeladene Replays beim Start nachträglich hochladen | aus |
| Replays | Replays nach erfolgreichem Upload löschen (in den Papierkorb) | aus |
| Spiel | Auto-Speichern an/aus, nur bei Online-Spielen | an, an |
| Spiel | Speichern-Taste: Taste aufnehmen und testen. Gesendet wird immer nur, wenn Rocket League im Vordergrund ist (fest, nur als Hinweis angezeigt). | Backspace |
| Spiel | Haltedauer der Taste (umgesetzt, wie bisher `mark_hold_ms`) | 1200 ms |
| Spiel | Stats-API-Adresse (umgesetzt unter „Spiel“; „Erweitert“ enthält Log und erneuten Assistenten) | 127.0.0.1:49123 |
| Dein Spieler | erkannte Accounts, bestätigen, hinzufügen, entfernen | automatisch |
| Hotkeys | Overlay / Pause / Schnell-Einordnen | Strg+Alt+B / Strg+Alt+P / Strg+Alt+1–4 |
| Benachrichtigungen | Hochgeladen / Fehlgeschlagen / Session beendet / Schnell-Einordnen, je an/aus; Countdown-Dauer | an / an / an / an; 10 s |
| Start | Mit Windows starten | an |
| Start | Minimiert starten (Overlay beim Start nicht öffnen) | an |

Der Token wird mit Windows-DPAPI für den angemeldeten Benutzer verschlüsselt in der `config.json` abgelegt (`ballchasing_token_protected`). Ein Klartext-Token aus einer alten Konfiguration wird gelesen und beim nächsten Speichern verschlüsselt. Die Umgebungsvariable `BALLCHASING_TOKEN` hat weiter Vorrang.

### 11.2 Erst-Einrichtung (Assistent) **[neu]**

Er erscheint beim ersten Start im Overlay und ersetzt das heutige „config.json im Editor öffnen“.

1. **ballchasing verbinden**: Link zu ballchasing.com/upload, Token einfügen, sofort prüfen, Kontoname zeigen. Danach das Konto abgleichen und „Bist du das?“ mit dem erkannten Spieler (bzw. beiden Accounts) bestätigen.
2. **Replay-Ordner**: erkannte Ordner mit Anzahl der Replays, ändern oder ergänzen.
3. **Spiel und Speichern-Taste** (ergänzt):
   - Prüft, ob die Stats API erreichbar ist. Falls nicht, steht die Anleitung für `TAStatsAPI.ini` daneben. Die Datei liegt meist unter `Program Files` und lässt sich ohne Adminrechte nicht ändern.
   - Danach Speichern-Taste wählen und **Test-Knopf**: In 5 s zu Rocket League wechseln (im Match), das Tool drückt die Taste, und im Spiel muss „Replay wird gespeichert“ erscheinen.
4. **Upload und Ordner**: Standard-Sichtbarkeit, Standard-Regeln (Liste aus [Anhang B](#anhang-b-standard-regeln) mit Häkchen und änderbaren Ordnernamen), Löschen nach Upload.
5. **Alte Replays nachladen**: keine / letzte 30 Tage / alle, mit Anzahl. Bereits vorhandene erkennt ballchasing selbst (`409`).

„**Später einrichten**“ ist auf jeder Seite möglich. Die App läuft dann im Tray, und die Einstellungen zeigen einen Hinweis, bis der Token gesetzt ist.

## 12. Grenzen der ballchasing-API

| Grenze | Folge |
|---|---|
| Ein Replay hat nur ein Gruppenfeld | Ordner sind exklusiv, Verschieben ersetzt |
| Gruppen- und Sichtbarkeitsänderungen nur beim eigenen Upload | Mitspieler-Replays nur ansehen |
| Private Replays anderer sind unsichtbar | „Spiele mit dir“ zeigt nur öffentliche und nicht gelistete |
| Playlist steht erst nach der Verarbeitung fest | Einordnen nach dem Upload per `PATCH`, nicht beim Upload |
| Gruppenlisten enthalten keine Elterngruppe | Baum über Abfragen je Gruppe, lokal zwischengespeichert |
| Umbenennen von Gruppen ist laut Doku nicht vorgesehen (`PATCH /groups` kennt `parent`, `shared` und die Erkennungsarten) | Umgesetzt: kein Umbenennen in der App, nur auf der Website |
| Was beim Löschen einer Gruppe mit den Replays passiert, sagt die Doku nicht | Umgesetzt: Löschen nur für leere Ordner ohne Unterordner, auch der Server prüft das |
| Keine Ränge, keine Einzelereignisse (Tore mit Zeit, Heatmaps) | Nicht Teil der App |
| Download: 1/s, 200/h ohne Patreon | „Ins Spiel laden“ nur einzeln, kein Massen-Download |

Bekannte Limits für Konten ohne Patreon: Gruppen auflisten 2/s und 500/h, Gruppen anlegen und Replays ändern je 2/s und 1000/h, Replays löschen 2/s und 1000/h, Download 1/s und 200/h.

## 13. Umsetzung in Etappen

Jede Etappe ist für sich nutzbar.

| Etappe | Inhalt |
|---|---|
| 1. Datenbasis | SQLite, Übernahme von `uploaded.json`, neue Warteschlange mit Zuständen und Neuversuchen, Warten auf `status` = `ok`, Details speichern, Abgleich eigener Replays, Erkennung deines Spielers |
| 2. Neue Oberfläche | App-Gerüst mit Navigation, Übersicht, Verlauf mit Detailansicht, Einstellungen mit Sofort-Speichern, Assistent, Pause mit Hotkey |
| 3. Ordner und Regeln | Ordnerbaum mit Drag & Drop und Mehrfachauswahl, Regeln, Eingang mit Tastatur, Schnell-Einordnen-Fenster, Vorschläge |
| 4. Statistik und Sessions | Statistik-Seite, Session-Übersicht, Benachrichtigungen mit Aktionen |
| 5. Speicher und Mitspieler | Löschen nach Upload, Ins Spiel laden mit Aufräumen, Spiele mit dir von anderen inklusive Duplikat-Abgleich |

Technische Wahl für die Oberfläche: Vorgeschlagen war Preact mit `htm`. Umgesetzt ist sie mit reinen JavaScript-Modulen ohne Bibliothek und ohne Build-Schritt: ein kleiner Helfer `h()` baut die Elemente, jede Seite zeichnet sich bei Live-Events neu. Das spart fremden Code, der eingebettet und aktuell gehalten werden müsste, und `go build` reicht weiterhin. Wird die Oberfläche deutlich größer, lässt sich Preact später Seite für Seite einführen.

## 14. Offene Punkte und Annahmen

Alle zehn Annahmen sind bestätigt (17.10.2026) und so umgesetzt, bis auf Punkt 10 (siehe [Abschnitt 13](#13-umsetzung-in-etappen)). Punkt 9 bleibt offen, solange nicht gegen die echte API getestet ist.

1. **Pause**: Strg+Alt+P pausiert Überwachung und Auto-Speichern zusammen, einzeln geht es über Menü und Übersicht.
2. **Während der Pause gespeicherte Replays** werden nach dem Fortsetzen hochgeladen.
3. **Neuversuche**: sofort, nach 1 min, nach 5 min. Netzwerkausfall und Rate-Limit zählen nicht als Versuch.
4. **„Monat“** in der Statistik bedeutet die letzten 30 Tage.
5. **Löschen nach Upload** verschiebt in den Papierkorb statt endgültig zu löschen.
6. **Schnell-Einordnen**: 1–3 sind Alternativ-Ordner, 4 ist „Eingang“, Countdown 10 s.
7. **Extra-Modi ohne Ranked-Variante** (Heatseeker, Rocket Labs, Dropshot Rumble) bekommen je einen Ordner, Offline-Spiele (`offline`, `season`) keine Regel und landen im Eingang.
8. **Token** wird verschlüsselt (DPAPI) gespeichert.
9. **Ordner umbenennen und Ordner mit Inhalt löschen** kommen erst nach einem Test gegen die API.
10. **Oberflächen-Technik**: Preact + htm ohne Build-Schritt.

## 15. Stand der Umsetzung

Alle fünf Etappen sind umgesetzt, die Bedienung beschreibt die [README](../README.md).

**Aufteilung im Code**

| Paket | Aufgabe |
|---|---|
| `cmd/auto-replay-upload` | Start: Log, eine Instanz pro Konfiguration, Autostart, Tray, Verdrahtung mit dem System |
| `internal/app` | Kern: Ablauf nach dem Upload, Abgleich, Sessions, Statistik, „Ins Spiel laden“, JSON-API und Live-Events der Oberfläche |
| `internal/store` | SQLite-Datenbank `auto-replay-upload.db` (reines Go), Übernahme von `uploaded.json` |
| `internal/upload` | Replay-Ordner beobachten, Warteschlange mit Zuständen und Neuversuchen |
| `internal/rules` | Standard-Regeln, Regel-Abgleich, Ordner- und Regel-Vorschläge |
| `internal/ballchasing` | API-Client mit Drosselung je Endpunkt; `ballchasingtest` ist ein nachgebauter Server für Tests |
| `internal/game` | Stats API, Match-Erkennung, Speichern-Taste, Replay-Ordner je System |
| `internal/popup` | Schnell-Einordnen und Hinweise: Win32 mit eigener Zeichnung, unter Linux Desktop-Benachrichtigungen, unter macOS die Mitteilungszentrale |
| `internal/overlay` | Overlay-Fenster mit WebView2, außerhalb von Windows der Browser |
| `internal/desktop` | Tray, Autostart, Papierkorb, Dateiauswahl, Meldungsfenster für Windows, macOS und Linux |
| `internal/x11` | X-Server unter Linux (auch XWayland): aktives Fenster, Tastencodes, Tastendruck über XTEST |
| `internal/assets` | das Programmsymbol |
| `internal/web/ui` | die Oberfläche |
| `cmd/uidemo` | die Oberfläche mit erfundenen Daten gegen den nachgebauten Server, zum Entwickeln ohne Konto |

**Abweichungen vom Entwurf**

| Thema | Entwurf | Umgesetzt |
|---|---|---|
| Hinweise | Windows-Benachrichtigungen | eigene Hinweisfenster, während eines Matches zurückgehalten ([9.2](#92-benachrichtigungen-neu)) |
| Eingang | Ordner „Unsortiert“ | Replays ohne Gruppe, im Baum als „Eingang (unsortiert)“ |
| Oberflächen-Technik | Preact + htm | reine JavaScript-Module ([13](#13-umsetzung-in-etappen)) |
| Ordner umbenennen / mit Inhalt löschen | nach einem Test | weggelassen, nur leere Ordner lassen sich löschen |
| Details nachladen | 1 Abruf alle 2 s | 1 Abruf alle 6 s |
| Vorschläge | Playlist und Mitspieler | Playlist wiegt schwerer als Mitspieler ([8.1](#81-ordner)) |
| Löschen nach Upload | 4 Bedingungen | zusätzlich: nicht über die Oberfläche hochgeladen |
| Token | anzeigen möglich | nur eintragen, ändern, trennen |

**Geprüft**

- Automatische Tests für alle Pakete, darunter der ganze Ablauf vom Upload bis zum Einordnen gegen den nachgebauten Server, Neuversuche, Ausfälle, Duplikate, Regeln und Vorschläge.
- Die Oberfläche in der Demo: alle Seiten, Assistent, Einordnen mit Countdown, Eingang mit Tastatur, Einstellungen.
- Die echte exe mit sicherer Test-Konfiguration: Start, Tray, lokaler Server, Schutz der API, Datenbank, Beenden.
- Popup-Fenster: liegen oben, nehmen keinen Fokus (`POPUP_LIVE=1`).

**Noch nicht im echten Spiel getestet**

- Schnell-Einordnen und Overlay über Rocket League im randlosen Fenster.
- „Ins Spiel laden“: ob das Spiel die Datei sofort zeigt oder erst nach erneutem Öffnen des Menüs, und ob das Aufräumen nach dem Verlassen zuverlässig greift.
- Upload und Einordnen mit dem echten ballchasing-Konto über mehrere Matches.

## 16. macOS und Linux

Rocket League läuft dort nur unter Proton oder Wine. Das Tool läuft trotzdem vollständig: Hochladen, Abgleich, Regeln und die Oberfläche sind reines Go und Web und unterscheiden sich nicht. Anders ist nur, was das System dazu liefert. Was es kann, meldet das Programm der Oberfläche (`platform` in `/api/state`); sie blendet aus, was fehlt, und erklärt warum.

| Thema | macOS | Linux |
|---|---|---|
| Symbol | Menüleiste über fyne systray (Cocoa, braucht cgo; ohne cgo gebaut kein Symbol); jeder Klick öffnet das Menü | StatusNotifierItem über D-Bus (reines Go); Linksklick öffnet die Oberfläche |
| Oberfläche | im Browser, kein Overlay | im Browser, kein Overlay |
| Autostart | LaunchAgent in `~/Library/LaunchAgents` | `.desktop`-Datei in `~/.config/autostart`; ein in den Desktop-Einstellungen abgeschalteter Eintrag (`Hidden`, `X-GNOME-Autostart-enabled`) bleibt aus |
| Eine Instanz | `flock` auf eine Sperrdatei in `$TMPDIR` | `flock` auf eine Sperrdatei in `$XDG_RUNTIME_DIR` |
| Papierkorb | der Befehl `/usr/bin/trash` neuerer macOS-Versionen, sonst `~/.Trash` bzw. `.Trashes` des Laufwerks, zuletzt der Finder | freedesktop.org-Papierkorb mit `.trashinfo`, für andere Laufwerke `.Trash-<uid>` an deren Wurzel |
| Replay-Ordner | Flaschen von CrossOver und Whisky, Präfixe von Heroic, `~/.wine`, die alte Mac-Version | Proton-Präfix der App 252950 in jeder Steam-Bibliothek (`libraryfolders.vdf`, auch Flatpak und Snap), Präfixe von Heroic, Lutris, Bottles, `~/.wine`, die alte Linux-Version |
| Speichern-Taste | nicht gesendet: macOS erlaubt das nur mit Bedienungshilfen-Rechten und nativem Code. Der Warnton am Matchende erinnert ans Speichern | XTEST mit der physischen Taste (Linux-Eingabecode + 8, passend zu `KeyboardEvent.code`), nur wenn das aktive Fenster das Spiel ist (`WM_CLASS` `steam_app_252950` bzw. `rocketleague.exe` oder Prozess) |
| Hotkeys | keine | `XGrabKey` am Wurzelfenster, unabhängig von Feststell- und Num-Taste, Tastenwiederholung gefiltert |
| Hinweise | `display notification` über osascript, ohne Knöpfe | `org.freedesktop.Notifications` mit Knöpfen und Programmsymbol |
| Schnell-Einordnen | entfällt, Regeln ordnen direkt ein | Benachrichtigung mit bis zu drei Knöpfen (zwei Alternativen und Eingang), dazu Strg+Alt+1–4 |
| Dateiauswahl, Meldungen | AppleScript (`choose file`, `display alert`) | Datei-Portal von xdg-desktop-portal, sonst zenity oder kdialog |
| Token | in der `config.json` mit Rechten nur für den Benutzer | wie macOS |
| Konfiguration | `~/Library/Application Support/auto-replay-upload` | `~/.config/auto-replay-upload` |

Unter Wayland laufen Proton-Spiele über XWayland. Hotkeys und Speichern-Taste wirken dann, solange ein X11-Fenster im Vordergrund ist, also beim Spielen. Strg+Alt+F1 bis F12 schaltet unter Linux die Konsole um und taugt nicht als Hotkey.

**Geprüft** unter Linux mit eigenem X-Server (Xvfb) und D-Bus: Hotkeys mit und ohne Num-Taste, Belegung durch ein anderes Programm, die Speichern-Taste nur im Spielfenster, alle Tastencodes, Benachrichtigungen mit Knöpfen gegen einen nachgebauten Dienst, Papierkorb auch auf einem zweiten Laufwerk, Autostart, eine Instanz, und das echte Programm mit nachgebautem Infobereich (Menü, Tooltip, Pausieren, Beenden). macOS prüft die CI (Bauen mit cgo und die Tests).

## Anhang A: Felder eines Replays

Aus `GET /api/replays/{id}`, geprüft an zwei echten Replays (privates 3v3, Ranked Doubles) und mit der Doku abgeglichen.

**Kopfdaten**

- `id`, `link`, `title`, `rocket_league_id`, `match_guid`
- `created` (Upload-Zeitpunkt), `date` (Spielzeitpunkt), `date_has_timezone`
- `uploader` mit `steam_id`, `name`, `profile_url`, `avatar`, dazu `recorder`
- `status` (`ok`, `pending`, `failed`), `visibility`
- `match_type`, `playlist_id`, `playlist_name`, `team_size`, `season`, `season_type`
- `map_code`, `map_name`
- `duration`, `play_duration`, `overtime`, bei Verlängerung laut Doku zusätzlich `overtime_seconds`
- `server` mit `name` und `region`
- `groups[]` mit `id`, `name`, `link`

**Pro Team** (`blue`, `orange`): `color`, laut Doku auch `name`, dazu `stats`:

| Gruppe | Felder |
|---|---|
| `ball` | `possession_time`, `time_in_side` |
| `core` | `shots`, `shots_against`, `goals`, `goals_against`, `saves`, `assists`, `score`, `shooting_percentage`, `epic_saves`, `clears`, `centers`, `aerial_hits`, `first_touches`, `flip_resets`, `crossbar_hits`, `bicycle_hits`, `juggle_hits` |
| `boost` | `bpm`, `bcpm`, `avg_amount`, `amount_collected` und `amount_stolen` (gesamt, `_big`, `_small`), `count_collected_big/_small`, `count_stolen_big/_small`, `amount_overfill`, `amount_overfill_stolen`, `amount_used_while_supersonic`, `time_zero_boost`, `time_full_boost`, `time_boost_0_25`, `_25_50`, `_50_75`, `_75_100` |
| `movement` | `total_distance`, `time_supersonic_speed`, `time_boost_speed`, `time_slow_speed`, `time_ground`, `time_low_air`, `time_high_air`, `time_powerslide`, `count_powerslide` |
| `positioning` | `time_defensive_third`, `time_neutral_third`, `time_offensive_third`, `time_defensive_half`, `time_offensive_half`, `time_behind_ball`, `time_infront_ball` |
| `demo` | `inflicted`, `taken`, jeweils gesamt sowie `_in_defensive_half` und `_in_offensive_half` |

**Pro Spieler**

- **Allgemein**: `name`, `id` mit `platform` und `id`, `start_time`, `end_time`, `mvp`, `car_id`, `car_name`, `steering_sensitivity`
- **`camera`**: `fov`, `height`, `pitch`, `distance`, `stiffness`, `swivel_speed`, `transition_speed`
- **`stats`**: dieselben Gruppen wie beim Team ohne `ball`, mit diesen Zusätzen:

| Gruppe | Zusätzlich zum Team |
|---|---|
| `core` | `mvp` |
| `boost` | `percent_zero_boost`, `percent_full_boost`, `percent_boost_0_25` bis `_75_100` |
| `movement` | `avg_speed`, `avg_speed_percentage`, `avg_powerslide_duration`, `percent_slow_speed`, `percent_boost_speed`, `percent_supersonic_speed`, `percent_ground`, `percent_low_air`, `percent_high_air` |
| `positioning` | `avg_distance_to_ball` (gesamt, `_possession`, `_no_possession`), `avg_distance_to_mates`, `time_most_back`, `time_most_forward`, `time_closest_to_ball`, `time_farthest_from_ball`, `goals_against_while_last_defender`, zu allen Zeiten ein `percent_`-Gegenstück |

Bei älteren Replays fehlen laut Doku einzelne Felder, etwa `time_wall` und `percent_wall`.

**Replay-Liste** (`GET /api/replays`), Kurzform pro Eintrag: Kopfdaten, Tore pro Team, pro Spieler Name, ID, Score, MVP, Ein- und Austrittszeit.

**Nicht enthalten**: Ränge (auch bei Ranked-Spielen nicht), Einzelereignisse wie Tore oder Schüsse mit Zeitpunkt, Positionsverläufe.

## Anhang B: Standard-Regeln

Nach der Entscheidung „Ranked und Casual getrennt, Extra-Modi einzeln“. Ordnernamen sind im Assistenten änderbar, die Playlist-Kennungen stammen aus der ballchasing-Doku.

| Playlist (`playlist_id`) | Ordner |
|---|---|
| `ranked-duels` | 1s Ranked |
| `ranked-doubles` | 2s Ranked |
| `ranked-standard`, `ranked-solo-standard` | 3s Ranked |
| `unranked-duels` | 1s Casual |
| `unranked-doubles` | 2s Casual |
| `unranked-standard` | 3s Casual |
| `unranked-chaos` | 4s Casual |
| `ranked-hoops` | Hoops Ranked |
| `hoops` | Hoops Casual |
| `ranked-rumble` | Rumble Ranked |
| `rumble` | Rumble Casual |
| `ranked-dropshot` | Dropshot Ranked |
| `dropshot` | Dropshot Casual |
| `ranked-snowday` | Snow Day Ranked |
| `snowday` | Snow Day Casual |
| `heatseeker` | Heatseeker |
| `dropshot-rumble` | Dropshot Rumble |
| `rocketlabs` | Rocket Labs |
| `tournament` | Turniere |
| `private` | Private |
| `offline`, `season`, unbekannte | keine Regel → Eingang |
