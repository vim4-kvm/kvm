//go:build vim4

package native

import (
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"

	"github.com/jetkvm/kvm/internal/gstcapture"
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
func vim4Pipeline(codec int, quality float64, frameRate, height int) string {
	if codec == 1 && H265Main10() {
		return fmt.Sprintf("vim4h265src bitrate=%d gop=%d framerate=%d height=%d ! appsink name=frames sync=false", vim4Bitrate(quality), vim4GOP, frameRate, height)
	}
	name := "h264"
	if codec == 1 {
		name = "h265"
	}
	// Coalesce PLIs from several viewers into one forced IDR.
	source, encoder, gop := "vfmcapsrc", "amlvenc", vim4GOP
	scale := ""
	if os.Getenv("VIM4_VIDEO_BACKEND") == "wave5" {
		source, encoder, gop = fmt.Sprintf("vim4v4l2src height=%d", height), "vim4wave5enc", 0
	} else if os.Getenv("VIM4_SECONDARY_HDMI_CAPTURE") == "1" {
		source = "vim4vdin1src"
	}
	if height > 0 && os.Getenv("VIM4_VIDEO_BACKEND") != "wave5" {
		alignment := 2
		if codec == 1 {
			alignment = 16
		}
		scale = fmt.Sprintf("vim4scale height=%d width-alignment=%d ! queue max-size-buffers=1 max-size-bytes=0 max-size-time=0 ! ", height, alignment)
	}
	return fmt.Sprintf("%s ! queue max-size-buffers=1 max-size-bytes=0 max-size-time=0 ! "+
		"%s%s bitrate=%d gop=%d framerate=%d min-force-key-unit-interval=250000000 ! video/x-%s ! "+
		"%sparse config-interval=-1 ! video/x-%s,stream-format=byte-stream,alignment=au ! appsink name=frames sync=false",
		source, scale, encoder, vim4Bitrate(quality), gop, frameRate, name, name, name)
}

func H265Main10() bool {
	if os.Getenv("VIM4_VIDEO_BACKEND") == "wave5" || os.Getenv("VIM4_SECONDARY_HDMI_CAPTURE") != "1" {
		return false
	}
	capable, ok := gstcapture.IntPropertyValue("vim4h265src", "hdr-capable")
	return ok && capable == 1
}

func vim4HDMIInfo() string {
	if os.Getenv("VIM4_VIDEO_BACKEND") == "wave5" {
		rate, ok := gstcapture.IntPropertyValue("vim4v4l2src", "input-rate")
		if !ok {
			return ""
		}
		return fmt.Sprintf("Frame Rate: %d\n", rate)
	}
	if !vendorSignalReady() {
		return ""
	}
	info, _ := os.ReadFile(hdmiInfoPath)
	if !vendorSignalReady() {
		return ""
	}
	return string(info)
}

// hdmiFrameRate reads the receiver's rate, reported in hundredths of a frame
// per second, from /sys/class/hdmirx/hdmirx0/info. It returns 0 without a signal.
func hdmiFrameRate(info string) int {
	// VRR cadence must not repeatedly reopen an encoder with a fixed budget.
	for _, name := range []string{"Capture Frame Rate", "Frame Rate"} {
		for _, line := range strings.Split(info, "\n") {
			key, value, ok := strings.Cut(line, ":")
			if !ok || strings.TrimSpace(key) != name {
				continue
			}
			hundredths, err := strconv.Atoi(strings.TrimSpace(value))
			if err == nil && hundredths >= 100 && hundredths <= 12000 {
				return (hundredths + 50) / 100
			}
			return 0
		}
	}
	return 0
}
