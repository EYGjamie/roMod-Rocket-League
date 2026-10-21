//go:build ignore

// gen draws the tool's icon, a download arrow on a violet triangle with
// rounded corners, and writes every file made from it: the PNG and ICO files
// of the tray in all three states and the SVG of the interface. Run it with
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
	corners [][2]float64  // the triangle before its corners are rounded
	radius  float64       // rounding, the triangle grows by it on every side
	arrow   [][2]float64  // the download arrow, clockwise from the tip
	bar     [2][2]float64 // ends of the line under the arrow
	barW    float64       // its width, with round ends
	// the red dot after failed uploads and the gap cut around it
	badgeX, badgeY, badgeR, badgeGap float64
}

var (
	// large is for 32 pixels and more and the interface.
	large = look{[][2]float64{{128, 40}, {226, 212}, {30, 212}}, 26,
		[][2]float64{{128, 188}, {168, 148}, {140, 148}, {140, 96}, {116, 96}, {116, 148}, {88, 148}},
		[2][2]float64{{96, 208}, {160, 208}}, 16, 212, 52, 38, 14}
	// small has strokes of whole pixels at 16 pixels, for the tray.
	small = look{[][2]float64{{128, 44}, {226, 210}, {30, 210}}, 28,
		[][2]float64{{128, 176}, {176, 128}, {144, 128}, {144, 80}, {112, 80}, {112, 128}, {80, 128}},
		[2][2]float64{{96, 208}, {160, 208}}, 32, 208, 48, 44, 14}
)

type state struct {
	name        string
	top, bottom color.NRGBA // triangle gradient
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

// nearSegment reports whether a point is at most d from the segment a–b.
func nearSegment(px, py float64, a, b [2]float64, d float64) bool {
	dx, dy := b[0]-a[0], b[1]-a[1]
	t := math.Max(0, math.Min(1, ((px-a[0])*dx+(py-a[1])*dy)/(dx*dx+dy*dy)))
	return inCircle(px, py, a[0]+t*dx, a[1]+t*dy, d)
}

// inTriangle is the triangle grown by the radius, which rounds its corners.
func (l look) inTriangle(px, py float64) bool {
	if inPolygon(px, py, l.corners) {
		return true
	}
	for i := range l.corners {
		if nearSegment(px, py, l.corners[i], l.corners[(i+1)%len(l.corners)], l.radius) {
			return true
		}
	}
	return false
}

func (l look) inGlyph(px, py float64) bool {
	return inPolygon(px, py, l.arrow) || nearSegment(px, py, l.bar[0], l.bar[1], l.barW/2)
}

// shade returns the color at a point of the 256 box, transparent outside.
func shade(s state, l look, px, py float64) color.NRGBA {
	if s.badge && inCircle(px, py, l.badgeX, l.badgeY, l.badgeR) {
		return red
	}
	if s.badge && inCircle(px, py, l.badgeX, l.badgeY, l.badgeR+l.badgeGap) || !l.inTriangle(px, py) {
		return color.NRGBA{}
	}
	if l.inGlyph(px, py) {
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

// svg is the logo of the interface, drawn from the same numbers: the
// triangle's corners rounded by a stroke with round joins, cropped to it.
func svg(s state) string {
	l := large
	x0, y0, x1, y1 := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	for _, p := range l.corners {
		x0, y0 = math.Min(x0, p[0]-l.radius), math.Min(y0, p[1]-l.radius)
		x1, y1 = math.Max(x1, p[0]+l.radius), math.Max(y1, p[1]+l.radius)
	}
	side := math.Max(x1-x0, y1-y0) + 4
	vx, vy := (x0+x1)/2-side/2, (y0+y1)/2-side/2
	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="%.1f %.1f %g %g">
<defs><linearGradient id="g" gradientUnits="userSpaceOnUse" x1="0" y1="%.1f" x2="0" y2="%.1f"><stop offset="0" stop-color="%s"/><stop offset="1" stop-color="%s"/></linearGradient></defs>
<polygon points="%s" fill="url(#g)" stroke="url(#g)" stroke-width="%g" stroke-linejoin="round"/>
<polygon points="%s" fill="%s"/>
<path d="M%g %gH%g" stroke="%s" stroke-width="%g" stroke-linecap="round"/>
</svg>
`, vx, vy, side, side, y0, y1, hex(s.top), hex(s.bottom), points(l.corners), 2*l.radius,
		points(l.arrow), hex(glyph), l.bar[0][0], l.bar[0][1], l.bar[1][0], hex(glyph), l.barW)
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
