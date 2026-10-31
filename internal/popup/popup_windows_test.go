//go:build windows

package popup

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
	"unsafe"
)

// render paints a popup into a bitmap without showing a window.
func render(t *testing.T, m *Manager, w *window, at time.Time) *image.RGBA {
	t.Helper()
	w.w, w.h = m.measure(w)
	hdr := bitmapInfoHeader{width: w.w, height: -w.h, planes: 1, bitCount: 32}
	hdr.size = uint32(unsafe.Sizeof(hdr))
	var bits unsafe.Pointer
	screen, _, _ := procGetDC.Call(0)
	defer procReleaseDC.Call(0, screen)
	bmp, _, _ := procCreateDIBSection.Call(screen, uintptr(unsafe.Pointer(&hdr)), 0, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if bmp == 0 {
		t.Fatal("CreateDIBSection failed")
	}
	defer procDeleteObject.Call(bmp)
	mem, _, _ := procCreateCompatibleDC.Call(screen)
	defer procDeleteDC.Call(mem)
	procSelectObject.Call(mem, bmp)
	m.paint(mem, w, at)
	px := unsafe.Slice((*byte)(bits), int(w.w*w.h*4))
	img := image.NewRGBA(image.Rect(0, 0, int(w.w), int(w.h)))
	for i := 0; i < len(px); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = px[i+2], px[i+1], px[i], 255
	}
	return img
}

func save(t *testing.T, img *image.RGBA, name string) {
	dir := os.Getenv("POPUP_PNG_DIR")
	if dir == "" {
		return
	}
	f, err := os.Create(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	png.Encode(f, img)
}

func distinctColors(img *image.RGBA) int {
	seen := map[color.RGBA]bool{}
	for i := 0; i < len(img.Pix); i += 4 {
		seen[color.RGBA{img.Pix[i], img.Pix[i+1], img.Pix[i+2], 255}] = true
	}
	return len(seen)
}

func TestPaint(t *testing.T) {
	m := New()
	start := time.Now()
	for _, dpi := range []uint32{96, 144} {
		q := &window{dpi: dpi, hot: 1, started: start, quick: &Quick{Title: "Ranked Doubles (2s) · Champions Field · Sieg 3:1",
			Target: "2s Ranked", Options: [4]string{"Liga 2026 › Spieltag 3", "Scrims", "", "Unsortiert"}, Seconds: 10}}
		img := render(t, m, q, start.Add(4*time.Second))
		if distinctColors(img) < 10 || len(q.hits) != 3 {
			t.Errorf("quick sort at %d dpi: %d colors, %d hits", dpi, distinctColors(img), len(q.hits))
		}
		save(t, img, "quick-"+string(rune('0'+dpi/48))+".png")

		ts := &window{dpi: dpi, hot: 1, shown: start, toast: &Toast{Title: "Replay hochgeladen",
			Body:    "Ranked Doubles (2s) · Champions Field · Sieg 3:1 → 2s Ranked",
			Actions: []Action{{Label: "Auf ballchasing öffnen"}, {Label: "Anderer Ordner"}}, Disable: func() {}}}
		img = render(t, m, ts, start)
		if len(ts.hits) != 4 {
			t.Errorf("toast at %d dpi: %d hits, want close, two buttons and the link", dpi, len(ts.hits))
		}
		save(t, img, "toast-"+string(rune('0'+dpi/48))+".png")
	}
	for _, dpi := range []uint32{96, 144} {
		lb := &window{dpi: dpi, shown: start, lobby: editorLobby(false)}
		img := render(t, m, lb, start.Add(5*time.Second))
		if lb.w != scale(lobbyW, dpi) || len(lb.parts) != 2 || lb.parts[1].top <= lb.parts[0].bottom || distinctColors(img) < 200 {
			t.Errorf("lobby at %d dpi: %dx%d, parts %v, %d colors: want the board with logos and the strip below", dpi, lb.w, lb.h, lb.parts, distinctColors(img))
		}
		save(t, img, "lobby-"+string(rune('0'+dpi/48))+".png")

		busy := &window{dpi: dpi, shown: start, lobby: editorLobby(true)}
		img = render(t, m, busy, start.Add(600*time.Millisecond))
		if busy.h != lb.h || distinctColors(img) >= distinctColors(render(t, m, lb, start)) {
			t.Errorf("loading lobby at %d dpi: height %d, want %d and no logos yet", dpi, busy.h, lb.h)
		}
		save(t, img, "lobby-loading-"+string(rune('0'+dpi/48))+".png")
	}
	bare := &window{dpi: 96, shown: start, lobby: &Lobby{Title: "Lobby"}}
	render(t, m, bare, start)
	if len(bare.parts) != 1 || bare.h != bare.parts[0].bottom {
		t.Errorf("lobby without note and hotkeys: parts %v, height %d, want the board alone", bare.parts, bare.h)
	}

	long := &window{dpi: 96, shown: start, toast: &Toast{Title: "Upload fehlgeschlagen",
		Body: "4D2F9A1C8B7E4E21A6F05C3B9D7E1A42.replay: HTTP 500 internal error, ballchasing hat die Datei nicht angenommen und meldet einen Serverfehler"}}
	short := &window{dpi: 96, shown: start, toast: &Toast{Title: "Kurz", Body: "Eine Zeile"}}
	if render(t, m, long, start).Bounds().Dy() <= render(t, m, short, start).Bounds().Dy() {
		t.Error("a long body must make the toast taller")
	}
}

// editorLobby is the example of the design: a 3v3 with a smurf marked,
// the user in blue; busy while the ranks load.
func editorLobby(busy bool) *Lobby {
	player := func(name string, me bool, wins string, tiers ...int) LobbyPlayer {
		p := LobbyPlayer{Name: name, Me: me, Wins: wins}
		for i := range p.Ranks {
			if !busy {
				p.Ranks[i] = Rank{Label: "R" + strconv.Itoa(tiers[2*i]), MMR: strconv.Itoa(tiers[2*i+1]), Tier: tiers[2*i]}
			}
		}
		if busy {
			p.Wins = ""
		}
		return p
	}
	l := &Lobby{Title: "Lobby · 3v3", Size: 3, Rank: 3, Smurf: true, Busy: busy, Seconds: 15, Avg: [2]string{"1137", "1210"},
		Hints: []Hint{{"Strg+Alt+S", "Smurf"}, {"Strg+Alt+M", "1s/2s/3s"}, {"Strg+Alt+L", "schließen"}},
		Teams: [2][]LobbyPlayer{{
			player("EYGjamie", true, "1.842", 15, 1012, 16, 1098, 17, 1187),
			player("Flinkwerk", false, "3.210", 16, 1065, 17, 1170, 16, 1121),
			player("noxa.rl", false, "954", 14, 921, 15, 1035, 16, 1102),
		}, {
			player("0verdrive", false, "212", 19, 1412, 19, 1489, 17, 1203),
			player("Kurzpass", false, "4.517", 16, 1077, 17, 1205, 18, 1268),
			player("Malte_04", false, "2.088", 15, 1009, 16, 1110, 17, 1158),
		}}}
	if busy {
		l.Avg, l.Note = [2]string{"–", "–"}, "Ränge werden geladen …"
	}
	return l
}
