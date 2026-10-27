//go:build vim4

package kvm

import "testing"

func TestMain10OfferMatchesVideoPayload(t *testing.T) {
	const base = "v=0\r\no=- 1 1 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0\r\n"
	for _, test := range []struct {
		media     string
		supported bool
	}{
		{"m=video 9 RTP/AVP 116\r\na=rtpmap:116 H265/90000\r\na=fmtp:116 profile-id=2;tier-flag=0\r\n", true},
		{"m=video 9 RTP/AVP 116\r\na=rtpmap:116 H265/90000\r\n", false},
		{"m=video 9 RTP/AVP 116 98\r\na=rtpmap:116 H265/90000\r\na=fmtp:116 profile-id=1\r\na=rtpmap:98 VP9/90000\r\na=fmtp:98 profile-id=2\r\n", false},
		{"m=audio 9 RTP/AVP 116\r\na=rtpmap:116 H265/90000\r\na=fmtp:116 profile-id=2\r\n", false},
	} {
		if got := supportsMain10(base + test.media); got != test.supported {
			t.Fatalf("offer=%q got=%v", test.media, got)
		}
	}
}
