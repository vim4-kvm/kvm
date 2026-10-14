//go:build vim4

package native

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

const (
	// Preserve existing quality presets and saved factors when extending the range.
	vim4QualityBitrateKbps = 12000
	vim4MaxBitrateKbps     = 60000
	vim4MinBitrateKbps     = 1000
	// Long GOPs keep 4K bitrate for detail; viewers that join or lose packets
	// request an IDR frame instead of waiting for the next periodic one.
	vim4GOP = 120
	// vfm_cap advertises an unspecified rate; without the HDMI rate the
	// encoder budgets for 30 fps and overshoots the bitrate at 60 fps.
	vim4DefaultFrameRate = 30
)

// vim4Bitrate maps quality factors to kbps; custom rates can use factors up to 5.
func vim4Bitrate(quality float64) int {
	return min(vim4MaxBitrateKbps, max(vim4MinBitrateKbps, int(math.Round(vim4QualityBitrateKbps*quality))))
}

func validVIM4Quality(factor float64) bool {
	return !math.IsNaN(factor) && factor > 0 && factor <= float64(vim4MaxBitrateKbps)/vim4QualityBitrateKbps
}

// vim4Pipeline builds the capture and hardware encoder pipeline. codec 1 is H.265.
func vim4Pipeline(codec int, quality float64, frameRate int) string {
	name := "h264"
	if codec == 1 {
		name = "h265"
	}
	// Coalesce PLIs from several viewers into one forced IDR.
	return fmt.Sprintf("vfmcapsrc ! queue max-size-buffers=3 max-size-bytes=0 max-size-time=0 ! "+
		"amlvenc bitrate=%d gop=%d framerate=%d min-force-key-unit-interval=250000000 ! video/x-%s ! "+
		"%sparse config-interval=-1 ! video/x-%s,stream-format=byte-stream,alignment=au ! appsink name=frames sync=false",
		vim4Bitrate(quality), vim4GOP, frameRate, name, name, name)
}

// hdmiFrameRate reads the receiver's rate, reported in hundredths of a frame
// per second, from /sys/class/hdmirx/hdmirx0/info. It returns 0 without a signal.
func hdmiFrameRate(info string) int {
	for _, line := range strings.Split(info, "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok || strings.TrimSpace(key) != "Frame Rate" {
			continue
		}
		hundredths, err := strconv.Atoi(strings.TrimSpace(value))
		if err == nil && hundredths >= 100 && hundredths <= 12000 {
			return (hundredths + 50) / 100
		}
		return 0
	}
	return 0
}

// rateChange debounces a changed HDMI rate before the encoder is reopened;
// the budget of a running vendor encoder cannot change.
type rateChange struct{ candidate, count int }

func (c *rateChange) observe(current, rate int) bool {
	if rate == 0 || rate == current {
		c.candidate, c.count = 0, 0
		return false
	}
	if rate != c.candidate {
		c.candidate, c.count = rate, 0
	}
	c.count++
	return c.count >= 3
}
