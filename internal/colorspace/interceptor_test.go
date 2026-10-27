package colorspace

import (
	"bytes"
	"testing"

	"github.com/pion/interceptor"
	"github.com/pion/rtp"
)

func TestFrameColorChangesAndNegotiation(t *testing.T) {
	for _, codec := range []string{"video/H265", "video/H264"} {
		for _, negotiated := range []bool{false, true} {
			color := []byte{9, 16, 9, 16}
			factory := &Factory{Current: func() []byte { return color }}
			instance, _ := factory.NewInterceptor("test")
			info := &interceptor.StreamInfo{MimeType: codec}
			if negotiated {
				info.RTPHeaderExtensions = []interceptor.RTPHeaderExtension{{URI: URI, ID: 7}}
			}
			writer := instance.BindLocalStream(info, interceptor.RTPWriterFunc(func(header *rtp.Header, payload []byte, _ interceptor.Attributes) (int, error) {
				want := []byte(nil)
				if codec == "video/H265" && negotiated && header.Marker && len(color) == 4 {
					want = color
				}
				if !bytes.Equal(header.GetExtension(7), want) {
					t.Fatalf("codec=%s negotiated=%v marker=%v color=%v", codec, negotiated, header.Marker, header.GetExtension(7))
				}
				return len(payload), nil
			}))
			for _, next := range [][]byte{{9, 16, 9, 16}, {1, 1, 1, 16}, nil} {
				color = next
				for _, marker := range []bool{false, true} {
					header := &rtp.Header{Version: 2, Marker: marker}
					if _, err := writer.Write(header, []byte{1}, nil); err != nil {
						t.Fatal(err)
					}
					if header.Extension {
						t.Fatal("shared packet header mutated")
					}
				}
			}
		}
	}
}
