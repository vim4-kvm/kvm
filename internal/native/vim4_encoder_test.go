//go:build vim4

package native

import (
	"math"
	"strings"
	"testing"
)

func TestVIM4BitrateFollowsQualityFactor(t *testing.T) {
	for quality, want := range map[float64]int{1: 12000, 0.5: 6000, 0.1: 1200, 0.01: 1000, 2.5: 30000, 5: 60000} {
		if got := vim4Bitrate(quality); got != want {
			t.Errorf("quality %v: bitrate %d, want %d", quality, got, want)
		}
	}
}

func TestVIM4CustomBitrateBounds(t *testing.T) {
	for _, factor := range []float64{0, -1, 5.001, math.NaN(), math.Inf(1)} {
		if validVIM4Quality(factor) {
			t.Fatalf("accepted invalid quality factor %v", factor)
		}
	}
	if !validVIM4Quality(5) || !strings.Contains(vim4Pipeline(0, 5, 60, 0), "bitrate=60000") {
		t.Fatal("60 Mbps custom bitrate did not reach the encoder")
	}
}

func TestVIM4PipelineSelectsCodec(t *testing.T) {
	h264 := vim4Pipeline(0, 1, 60, 0)
	h265 := vim4Pipeline(1, 0.5, 50, 0)
	for _, part := range []string{"bitrate=12000", "framerate=60", "video/x-h264", "h264parse"} {
		if !strings.Contains(h264, part) {
			t.Errorf("H.264 pipeline lacks %q: %s", part, h264)
		}
	}
	for _, part := range []string{"bitrate=6000", "framerate=50", "video/x-h265", "h265parse"} {
		if !strings.Contains(h265, part) {
			t.Errorf("H.265 pipeline lacks %q: %s", part, h265)
		}
	}
}

func TestHDMIFrameRate(t *testing.T) {
	for info, want := range map[string]int{
		"Frame Rate: 5994\nColor Depth: 8":            60,
		"  Frame Rate :  2500":                        25,
		"Frame Rate: 0":                               0,
		"Frame Rate: abc":                             0,
		"no signal":                                   0,
		"Frame Rate: 4800\nCapture Frame Rate: 12000": 120,
		"Capture Frame Rate: 12000\nFrame Rate: 9000": 120,
		"Capture Frame Rate: 0\nFrame Rate: 4800":     0,
	} {
		if got := hdmiFrameRate(info); got != want {
			t.Errorf("%q: %d, want %d", info, got, want)
		}
	}
}

func TestRateChangeIsDebounced(t *testing.T) {
	var change rateChange
	for i, rate := range []int{50, 0, 50, 50} {
		if change.observe(60, rate) {
			t.Fatalf("reopened after sample %d", i)
		}
	}
	if !change.observe(60, 50) {
		t.Fatal("third consecutive changed rate did not reopen")
	}
	change = rateChange{}
	for _, rate := range []int{50, 30, 50} {
		if change.observe(60, rate) {
			t.Fatal("alternating rates reopened the encoder")
		}
	}
}
