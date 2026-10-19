//go:build ignore

// gen draws the tool's icon, a replay arrow around an upload arrow on a
// violet tile, and writes every file made from it: the PNG and ICO files of
// the tray in all three states and the SVG of the interface. Run it with
// go generate in this folder.
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"math"
	"os"
	"strings"
)

// look is the drawing in a 256×256 box.
type look struct {
	tileRadius   float64
	ringX, ringY float64
	ringR, ringW float64 // middle and width of the stroke
	// degrees, y down: the arc runs clockwise over the bottom from the head
	// end (up and right) to the tail end (up and left)
	ringFrom, ringTo  float64
	headLen, headHalf float64      // how far the replay arrow's head reaches past the arc, half its width
	arrow             [][2]float64 // the upload arrow, clockwise from the tip
}

var (
	// large is for 32 pixels and more and the interface.
	large = look{56, 128, 132, 78, 20, -48, 222, 38, 27,
		[][2]float64{{128, 64}, {170, 110}, {139, 110}, {139, 186}, {117, 186}, {117, 110}, {86, 110}}}
	// small has strokes of whole pixels at 16 pixels, for the tray.
	small = look{48, 128, 128, 80, 32, -50, 220, 42, 38,
		[][2]float64{{128, 60}, {168, 108}, {144, 108}, {144, 180}, {112, 180}, {112, 108}, {88, 108}}}
)

const (
	badgeX, badgeY, badgeR = 208.0, 48.0, 40.0
	badgeGap               = 14.0
)

type state struct {
	name        string
	top, bottom color.NRGBA // tile gradient
	badge       bool
}

var (
	glyph = color.NRGBA{0xfb, 0xfa, 0xff, 0xff}
	red   = color.NRGBA{0xe5, 0x48, 0x4d, 0xff}

	states = []state{
		{"icon", color.NRGBA{0xa9, 0x9e, 0xf0, 0xff}, color.NRGBA{0x5f, 0x51, 0xb3, 0xff}, false},
		{"icon-paused", color.NRGBA{0xa3, 0xa6, 0xb6, 0xff}, color.NRGBA{0x5b, 0x5e, 0x6d, 0xff}, false},
		{"icon-error", color.NRGBA{0xa9, 0x9e, 0xf0, 0xff}, color.NRGBA{0x5f, 0x51, 0xb3, 0xff}, true},
	}
	icoSizes = []int{256, 64, 48, 40, 32, 24, 20, 16} // largest first: some converters take the first
)

func rad(deg float64) float64 { return deg * math.Pi / 180 }

func (l look) ringPoint(deg float64) (float64, float64) {
	return l.ringX + l.ringR*math.Cos(rad(deg)), l.ringY + l.ringR*math.Sin(rad(deg))
}

// head is the replay arrow's head at the arc's start, pointing against the
// arc's direction (counterclockwise on screen).
func (l look) head() [][2]float64 {
	x, y := l.ringPoint(l.ringFrom)
	t := rad(l.ringFrom)
	tx, ty := math.Sin(t), -math.Cos(t) // tangent, counterclockwise
	nx, ny := math.Cos(t), math.Sin(t)  // outwards
	return [][2]float64{
		{x + tx*l.headLen, y + ty*l.headLen},
		{x + nx*l.headHalf, y + ny*l.headHalf},
		{x - nx*l.headHalf, y - ny*l.headHalf},
	}
}

func inPolygon(px, py float64, poly [][2]float64) bool {
	in := false
	for i, j := 0, len(poly)-1; i < len(poly); j, i = i, i+1 {
		xi, yi, xj, yj := poly[i][0], poly[i][1], poly[j][0], poly[j][1]
		if (yi > py) != (yj > py) && px < (xj-xi)*(py-yi)/(yj-yi)+xi {
			in = !in
		}
	}
	return in
}

func inCircle(px, py, cx, cy, r float64) bool { return (px-cx)*(px-cx)+(py-cy)*(py-cy) <= r*r }

func (l look) inTile(px, py float64) bool {
	dx := math.Max(math.Abs(px-128)-(128-l.tileRadius), 0)
	dy := math.Max(math.Abs(py-128)-(128-l.tileRadius), 0)
	return dx*dx+dy*dy <= l.tileRadius*l.tileRadius
}

func (l look) inRing(px, py float64) bool {
	d := math.Hypot(px-l.ringX, py-l.ringY)
	if math.Abs(d-l.ringR) <= l.ringW/2 {
		a := math.Atan2(py-l.ringY, px-l.ringX) * 180 / math.Pi
		for a < l.ringFrom {
			a += 360
		}
		if a <= l.ringTo {
			return true
		}
	}
	for _, deg := range []float64{l.ringFrom, l.ringTo} { // round caps
		x, y := l.ringPoint(deg)
		if inCircle(px, py, x, y, l.ringW/2) {
			return true
		}
	}
	return false
}

// shade returns the color at a point of the 256 box, transparent outside.
func shade(s state, l look, px, py float64) color.NRGBA {
	if s.badge && inCircle(px, py, badgeX, badgeY, badgeR) {
		return red
	}
	if s.badge && inCircle(px, py, badgeX, badgeY, badgeR+badgeGap) || !l.inTile(px, py) {
		return color.NRGBA{}
	}
	if l.inRing(px, py) || inPolygon(px, py, l.head()) || inPolygon(px, py, l.arrow) {
		return glyph
	}
	f := py / 256
	mix := func(a, b uint8) uint8 { return uint8(math.Round(float64(a) + (float64(b)-float64(a))*f)) }
	return color.NRGBA{mix(s.top.R, s.bottom.R), mix(s.top.G, s.bottom.G), mix(s.top.B, s.bottom.B), 0xff}
}

// render draws the icon at size×size, averaging 8×8 samples per pixel.
func render(s state, size int) *image.NRGBA {
	const n = 8
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	scale := 256 / float64(size)
	l := large
	if size <= 24 {
		l = small
	}
	for y := range size {
		for x := range size {
			var r, g, b, a float64
			for j := range n {
				for i := range n {
					c := shade(s, l, (float64(x)+(float64(i)+.5)/n)*scale, (float64(y)+(float64(j)+.5)/n)*scale)
					ca := float64(c.A) / 255
					r += float64(c.R) * ca
					g += float64(c.G) * ca
					b += float64(c.B) * ca
					a += ca
				}
			}
			if a == 0 {
				continue
			}
			img.SetNRGBA(x, y, color.NRGBA{uint8(math.Round(r / a)), uint8(math.Round(g / a)), uint8(math.Round(b / a)), uint8(math.Round(a / n / n * 255))})
		}
	}
	return img
}

func pngBytes(img image.Image) []byte {
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		log.Fatal(err)
	}
	return b.Bytes()
}

// dib is an ICO image as 32-bit BMP: rows bottom-up in BGRA, then the AND
// mask, which marks fully transparent pixels.
func dib(img *image.NRGBA) []byte {
	w, h := img.Rect.Dx(), img.Rect.Dy()
	maskRow := (w + 31) / 32 * 4
	var b bytes.Buffer
	binary.Write(&b, binary.LittleEndian, struct {
		Size, Width, Height           int32
		Planes, BitCount              uint16
		Compression, SizeImage        uint32
		XPels, YPels, Used, Important int32
	}{40, int32(w), int32(2 * h), 1, 32, 0, uint32(w*h*4 + maskRow*h), 0, 0, 0, 0})
	for y := h - 1; y >= 0; y-- {
		for x := range w {
			c := img.NRGBAAt(x, y)
			b.Write([]byte{c.B, c.G, c.R, c.A})
		}
	}
	for y := h - 1; y >= 0; y-- {
		row := make([]byte, maskRow)
		for x := range w {
			if img.NRGBAAt(x, y).A == 0 {
				row[x/8] |= 0x80 >> (x % 8)
			}
		}
		b.Write(row)
	}
	return b.Bytes()
}

// ico packs the sizes into one file: PNG for 256, BMP below, which every
// Windows version and LoadImage read.
func ico(s state) []byte {
	var images [][]byte
	for _, size := range icoSizes {
		img := render(s, size)
		if size >= 256 {
			images = append(images, pngBytes(img))
		} else {
			images = append(images, dib(img))
		}
	}
	var b bytes.Buffer
	binary.Write(&b, binary.LittleEndian, [3]uint16{0, 1, uint16(len(images))})
	offset := 6 + 16*len(images)
	for i, data := range images {
		dim := byte(icoSizes[i]) // 256 is written as 0
		b.Write([]byte{dim, dim, 0, 0})
		binary.Write(&b, binary.LittleEndian, struct {
			Planes, BitCount uint16
			Size, Offset     uint32
		}{1, 32, uint32(len(data)), uint32(offset)})
		offset += len(data)
	}
	for _, data := range images {
		b.Write(data)
	}
	return b.Bytes()
}

func points(poly [][2]float64) string {
	var p []string
	for _, xy := range poly {
		p = append(p, fmt.Sprintf("%.1f,%.1f", xy[0], xy[1]))
	}
	return strings.Join(p, " ")
}

func hex(c color.NRGBA) string { return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B) }

// svg is the normal icon for the interface, drawn from the same numbers.
func svg(s state) string {
	l := large
	x1, y1 := l.ringPoint(l.ringFrom)
	x2, y2 := l.ringPoint(l.ringTo)
	arc := 0
	if l.ringTo-l.ringFrom > 180 {
		arc = 1
	}
	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 256 256">
<defs><linearGradient id="g" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="%s"/><stop offset="1" stop-color="%s"/></linearGradient></defs>
<rect width="256" height="256" rx="%g" fill="url(#g)"/>
<g fill="%s">
<path d="M%.1f %.1fA%g %g 0 %d 1 %.1f %.1f" fill="none" stroke="%s" stroke-width="%g" stroke-linecap="round"/>
<polygon points="%s"/>
<polygon points="%s"/>
</g>
</svg>
`, hex(s.top), hex(s.bottom), l.tileRadius, hex(glyph), x1, y1, l.ringR, l.ringR, arc, x2, y2, hex(glyph), l.ringW, points(l.head()), points(l.arrow))
}

func write(path string, data []byte) {
	if err := os.WriteFile(path, data, 0o644); err != nil {
		log.Fatal(err)
	}
}

func main() {
	for _, s := range states {
		write(s.name+".png", pngBytes(render(s, 64)))
		write(s.name+".ico", ico(s))
	}
	write("../web/ui/icon.svg", []byte(svg(states[0])))
	if len(os.Args) > 1 { // a preview of the sizes, e.g. go run gen.go preview.png
		const pad = 8
		sheet := image.NewNRGBA(image.Rect(0, 0, 3*(256+pad), 256+64+pad))
		x := 0
		for _, s := range states {
			put(sheet, render(s, 256), x, 0)
			px := x
			for _, size := range []int{64, 48, 32, 24, 20, 16} {
				put(sheet, render(s, size), px, 256+pad)
				px += size + 4
			}
			x += 256 + pad
		}
		write(os.Args[1], pngBytes(sheet))
	}
}

func put(dst *image.NRGBA, src *image.NRGBA, x, y int) {
	for j := range src.Rect.Dy() {
		for i := range src.Rect.Dx() {
			dst.SetNRGBA(x+i, y+j, src.NRGBAAt(i, j))
		}
	}
}
