//go:build linux && vim4

package gstcapture

import (
	"testing"
	"time"
)

func TestEncoderNominalDurationDoesNotSlowMediaClock(t *testing.T) {
	step := uint64(time.Second / 60)
	if got := frameDuration(10*step, 11*step, uint64(time.Second/30)); got != time.Duration(step) {
		t.Fatalf("60 fps timestamp spacing became %v", got)
	}
}

func TestMissingOrResetTimestampUsesNominalDuration(t *testing.T) {
	for _, pair := range [][2]uint64{{^uint64(0), 0}, {500, 0}, {500, 500}, {500, ^uint64(0)}} {
		if got := frameDuration(pair[0], pair[1], 40000000); got != 40*time.Millisecond {
			t.Fatalf("%v: %v", pair, got)
		}
	}
	if got := frameDuration(^uint64(0), ^uint64(0), ^uint64(0)); got != 0 {
		t.Fatalf("unknown duration: %v", got)
	}
}
