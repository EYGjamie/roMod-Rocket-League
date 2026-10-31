package popup

import (
	"math"
	"testing"
	"time"
)

func TestRankLogos(t *testing.T) {
	for tier := 0; tier <= 22; tier++ {
		for _, size := range []int{34, 51, 136} {
			px, err := rankLogo(tier, size)
			if err != nil {
				t.Fatalf("tier %d: %v", tier, err)
			}
			if len(px) != size*size*4 {
				t.Fatalf("tier %d at %d: %d bytes", tier, size, len(px))
			}
			var opaque int
			for i := 0; i < len(px); i += 4 {
				if px[i] > px[i+3] || px[i+1] > px[i+3] || px[i+2] > px[i+3] {
					t.Fatalf("tier %d at %d: pixel %d not premultiplied: %v", tier, size, i/4, px[i:i+4])
				}
				if px[i+3] > 200 {
					opaque++
				}
			}
			if px[3] != 0 || opaque < size*size/20 {
				t.Errorf("tier %d at %d: corner alpha %d, %d opaque pixels: want a logo on a clear ground", tier, size, px[3], opaque)
			}
		}
	}
	if _, err := rankLogo(23, 34); err == nil {
		t.Error("tier 23 has a logo")
	}
}

func TestLobbyBar(t *testing.T) {
	for _, c := range []struct {
		busy     bool
		since    time.Duration
		from, to float64
	}{
		{false, 0, 0, 1},
		{false, 7500 * time.Millisecond, 0, 0.5},
		{false, 20 * time.Second, 0, 0},
		{true, 0, 0, 0},                          // the piece comes in from the left
		{true, 600 * time.Millisecond, 0.3, 0.7}, // halfway
		{true, 1800 * time.Millisecond, 0.3, 0.7},
	} {
		from, to := lobbyBar(c.busy, 15, c.since)
		if math.Abs(from-c.from) > 1e-9 || math.Abs(to-c.to) > 1e-9 {
			t.Errorf("busy %v after %v: %.3f–%.3f, want %.3f–%.3f", c.busy, c.since, from, to, c.from, c.to)
		}
	}
}
