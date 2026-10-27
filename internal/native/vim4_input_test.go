//go:build linux && vim4

package native

import (
	"strings"
	"testing"
)

func TestInputChangeRecoversSameRateGeometryAndSignalLoss(t *testing.T) {
	current := parseInputMode("Hactive: 3840\nVactive: 2160\nFrame Rate: 5999\nHDR EOTF: SDR")
	for _, info := range []string{
		"Hactive: 3440\nVactive: 1440\nFrame Rate: 6000\nHDR EOTF: SDR",
		"Hactive: 3840\nVactive: 2160\nFrame Rate: 6000\nHDR EOTF: SMPTE_ST_2048",
		"",
	} {
		var change inputChange
		next := parseInputMode(info)
		if change.observe(current, next) || !change.observe(current, next) {
			t.Fatalf("stable source change was not detected: %q", info)
		}
	}
	var change inputChange
	for _, rate := range []int{50, 0, 30, 60, 50, 60} {
		if change.observe(current, inputMode{rate: rate}) {
			t.Fatal("unstable observations reopened capture")
		}
	}
}

func TestInputKeyIgnoresVRRCadenceAndMeasuredJitter(t *testing.T) {
	info := "Hactive: 1920\nVactive: 1080\nCapture Frame Rate: 12000\nFrame Rate: 4800\nColor Space: 2-YUV444\nColor Depth: 8"
	mode := parseInputMode(info)
	var change inputChange
	for _, rate := range []string{"6000", "9000", "11998", "4800"} {
		if change.observe(mode, parseInputMode(strings.Replace(info, "Frame Rate: 4800", "Frame Rate: "+rate, 1))) {
			t.Fatal("VRR cadence reopened capture")
		}
	}
	if parseInputMode("Frame Rate: 5987") != parseInputMode("Frame Rate: 5999") {
		t.Fatal("measurement jitter changed the input key")
	}
}

func TestStaleTimingsDoNotEstablishReceiverLock(t *testing.T) {
	for _, text := range []string{"3b11", "0", "", "invalid"} {
		if signalLocked(text) {
			t.Fatalf("unlocked receiver accepted: %q", text)
		}
	}
	if !signalLocked("13b11\n") {
		t.Fatal("locked receiver rejected")
	}
}
