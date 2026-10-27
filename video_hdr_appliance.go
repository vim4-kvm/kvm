//go:build !vim4

package kvm

import (
	"github.com/pion/interceptor"
	"github.com/pion/webrtc/v4"
)

func main10Video() bool          { return false }
func supportsMain10(string) bool { return false }
func videoCapability(mime string) webrtc.RTPCodecCapability {
	return webrtc.RTPCodecCapability{MimeType: mime}
}
func registerMain10(*webrtc.MediaEngine) error                            { return nil }
func registerColorSpace(*webrtc.MediaEngine, *interceptor.Registry) error { return nil }
