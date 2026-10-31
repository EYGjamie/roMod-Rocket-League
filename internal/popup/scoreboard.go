package popup

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"time"

	"github.com/EYGjamie/roMod-Rocket-League/internal/assets"
)

// rankLogo returns the logo of a tier, 0 Unranked to 22 Supersonic
// Legend, scaled to size×size pixels: premultiplied BGRA rows from the
// top, as AlphaBlend takes them.
func rankLogo(tier, size int) ([]byte, error) {
	data, err := assets.Ranks.ReadFile(fmt.Sprintf("ranks/%d.png", tier))
	if err != nil {
		return nil, err
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	src := image.NewRGBA(img.Bounds()) // premultiplied
	draw.Draw(src, src.Bounds(), img, img.Bounds().Min, draw.Src)
	return shrink(src, size), nil
}

// shrink scales src to size×size: each pixel is the average of the area
// it covers.
func shrink(src *image.RGBA, size int) []byte {
	b := src.Bounds()
	fx, fy := float64(b.Dx())/float64(size), float64(b.Dy())/float64(size)
	out := make([]byte, 0, size*size*4)
	for y := range size {
		y0, y1 := float64(y)*fy, float64(y+1)*fy
		for x := range size {
			x0, x1 := float64(x)*fx, float64(x+1)*fx
			var sum [4]float64
			for sy := int(y0); float64(sy) < y1 && sy < b.Dy(); sy++ {
				wy := min(y1, float64(sy+1)) - max(y0, float64(sy))
				for sx := int(x0); float64(sx) < x1 && sx < b.Dx(); sx++ {
					wt := wy * (min(x1, float64(sx+1)) - max(x0, float64(sx)))
					p := src.Pix[src.PixOffset(b.Min.X+sx, b.Min.Y+sy):]
					for c := range sum {
						sum[c] += wt * float64(p[c])
					}
				}
			}
			area := fx * fy
			out = append(out, byte(sum[2]/area+0.5), byte(sum[1]/area+0.5), byte(sum[0]/area+0.5), byte(sum[3]/area+0.5))
		}
	}
	return out
}

// lobbyBar is the lit part of the bar under the scoreboard, as fractions
// of its width: while the lookup runs a piece slides across, otherwise it
// shrinks until the popup closes.
func lobbyBar(busy bool, seconds int, since time.Duration) (from, to float64) {
	if busy {
		const period = 1200 * time.Millisecond
		t := float64(since%period) / float64(period)
		t = t * t * (3 - 2*t) // ease in and out
		from = 0.4 * (3.5*t - 1)
		return max(from, 0), min(from+0.4, 1)
	}
	return 0, max(1-since.Seconds()/float64(max(seconds, 1)), 0)
}
