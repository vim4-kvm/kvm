// Package colorspace preserves encoded-frame colorimetry across WebRTC.
package colorspace

import (
	"github.com/pion/interceptor"
	"github.com/pion/rtp"
)

const URI = "http://www.webrtc.org/experiments/rtp-hdrext/color-space"

type Factory struct{ Current func() []byte }

func (f *Factory) NewInterceptor(string) (interceptor.Interceptor, error) {
	return &sender{current: f.Current}, nil
}

type sender struct {
	interceptor.NoOp
	current func() []byte
}

func (s *sender) BindLocalStream(info *interceptor.StreamInfo, writer interceptor.RTPWriter) interceptor.RTPWriter {
	var id uint8
	if info.MimeType != "video/H265" {
		return writer
	}
	for _, extension := range info.RTPHeaderExtensions {
		if extension.URI == URI {
			id = uint8(extension.ID)
		}
	}
	if id == 0 {
		return writer
	}
	return interceptor.RTPWriterFunc(func(header *rtp.Header, payload []byte, attributes interceptor.Attributes) (int, error) {
		if header.Marker {
			if color := s.current(); len(color) == 4 {
				// Static tracks can fan one frame out to viewers with different
				// extension IDs. Do not add this viewer's ID to a shared header.
				copy := header.Clone()
				header = &copy
				if err := header.SetExtension(id, color); err != nil {
					return 0, err
				}
			}
		}
		return writer.Write(header, payload, attributes)
	})
}
