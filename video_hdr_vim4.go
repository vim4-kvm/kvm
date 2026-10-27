//go:build vim4

package kvm

import (
	"strings"

	"github.com/jetkvm/kvm/internal/colorspace"
	"github.com/jetkvm/kvm/internal/native"
	"github.com/pion/interceptor"
	"github.com/pion/sdp/v3"
	"github.com/pion/webrtc/v4"
)

func main10Video() bool { return native.H265Main10() }

func supportsMain10(offer string) bool {
	var description sdp.SessionDescription
	if description.Unmarshal([]byte(offer)) != nil {
		return false
	}
	for _, media := range description.MediaDescriptions {
		if media.MediaName.Media != "video" {
			continue
		}
		for _, format := range media.MediaName.Formats {
			hevc, main10 := false, false
			for _, attribute := range media.Attributes {
				payload, value, ok := strings.Cut(attribute.Value, " ")
				if !ok || payload != format {
					continue
				}
				if attribute.Key == "rtpmap" {
					hevc = strings.EqualFold(value, "H265/90000")
				}
				if attribute.Key == "fmtp" {
					for _, parameter := range strings.Split(value, ";") {
						key, value, ok := strings.Cut(strings.TrimSpace(parameter), "=")
						if ok && key == "profile-id" && value == "2" {
							main10 = true
						}
					}
				}
			}
			if hevc && main10 {
				return true
			}
		}
	}
	return false
}

func videoCapability(mime string) webrtc.RTPCodecCapability {
	capability := webrtc.RTPCodecCapability{MimeType: mime, ClockRate: 90000}
	if mime == webrtc.MimeTypeH265 && main10Video() {
		capability.SDPFmtpLine = "profile-id=2;tier-flag=0;level-id=180;tx-mode=SRST"
	}
	return capability
}

func registerMain10(engine *webrtc.MediaEngine) error {
	if !main10Video() {
		return nil
	}
	capability := videoCapability(webrtc.MimeTypeH265)
	capability.RTCPFeedback = []webrtc.RTCPFeedback{{Type: "nack"}, {Type: "nack", Parameter: "pli"}, {Type: "ccm", Parameter: "fir"}}
	return engine.RegisterCodec(webrtc.RTPCodecParameters{RTPCodecCapability: capability, PayloadType: 126}, webrtc.RTPCodecTypeVideo)
}

func registerColorSpace(engine *webrtc.MediaEngine, registry *interceptor.Registry) error {
	if err := engine.RegisterHeaderExtension(webrtc.RTPHeaderExtensionCapability{URI: colorspace.URI}, webrtc.RTPCodecTypeVideo); err != nil {
		return err
	}
	registry.Add(&colorspace.Factory{Current: native.VideoColorSpace})
	return nil
}
