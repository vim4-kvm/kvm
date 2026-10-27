//go:build linux && vim4

package native

import (
	"os"
	"strconv"
	"strings"
)

// hw_info reads cached receiver state, not hardware registers. Bit 16 is
// FSM_SIG_READY (hdmi_rx_repeater.h); timing fields alone remain stale on loss.
func signalLocked(text string) bool {
	value, err := strconv.ParseUint(strings.TrimSpace(text), 16, 32)
	return err == nil && value&(1<<16) != 0
}

func vendorSignalReady() bool {
	text, err := os.ReadFile("/sys/class/hdmirx/hdmirx0/hw_info")
	if os.IsNotExist(err) {
		return true // Older receivers did not expose cached lock state.
	}
	return err == nil && signalLocked(string(text))
}

// Keep measured VRR cadence out of the key: only changes which invalidate a
// capture/encoder instance should reopen it.
type inputMode struct {
	width, height, rate int
	color, depth, eotf  string
}

func parseInputMode(info string) inputMode {
	mode := inputMode{rate: hdmiFrameRate(info)}
	if mode.rate == 0 {
		return inputMode{}
	}
	for _, line := range strings.Split(info, "\n") {
		name, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		switch strings.TrimSpace(name) {
		case "Hactive":
			mode.width, _ = strconv.Atoi(value)
		case "Vactive":
			mode.height, _ = strconv.Atoi(value)
		case "Color Space":
			mode.color = value
		case "Color Depth":
			mode.depth = value
		case "HDR EOTF":
			mode.eotf = value
		}
	}
	return mode
}

type inputChange struct {
	candidate inputMode
	count     int
}

// Two identical observations reject a single unstable read, including a brief
// loss of lock. Missing signal also releases the old DMA pool promptly.
func (c *inputChange) observe(current, next inputMode) bool {
	if current == next {
		c.count = 0
		return false
	}
	if c.candidate != next {
		c.candidate, c.count = next, 0
	}
	c.count++
	return c.count >= 2
}
