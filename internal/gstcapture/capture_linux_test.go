//go:build linux && vim4

package gstcapture

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"
)

func TestPropertyProbeRejectsMissingAndWrongTypes(t *testing.T) {
	if min, max, ok := IntPropertyRange("videotestsrc", "num-buffers"); !ok || min > 0 || max < 60 {
		t.Fatalf("integer property range: %d %d %v", min, max, ok)
	}
	for _, property := range []string{"is-live", "missing-property"} {
		if _, _, ok := IntPropertyRange("videotestsrc", property); ok {
			t.Fatalf("accepted %s", property)
		}
	}
	if _, _, ok := IntPropertyRange("missing-vim4-test-element", "framerate"); ok {
		t.Fatal("accepted missing element")
	}
}

func TestTimingAndEOS(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	frames := 0
	err := Run(ctx, "videotestsrc num-buffers=3 ! video/x-raw,width=16,height=16,framerate=25/1 ! appsink name=frames sync=false", nil, func(f Frame) error {
		if len(f.Data) == 0 || f.Width != 16 || f.Height != 16 || f.Duration != 40*time.Millisecond || f.PTS != time.Duration(frames)*40*time.Millisecond {
			t.Errorf("incorrect buffer metadata: pts=%v duration=%v size=%dx%d bytes=%d", f.PTS, f.Duration, f.Width, f.Height, len(f.Data))
		}
		frames++
		return nil
	})
	if !errors.Is(err, io.EOF) || frames != 3 {
		t.Fatalf("frames=%d error=%v", frames, err)
	}
}

func TestCancellationAndReopen(t *testing.T) {
	for i := 0; i < 2; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err := Run(ctx, "videotestsrc is-live=true ! appsink name=frames sync=false", nil, func(Frame) error { cancel(); return nil })
		cancel()
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("run %d: %v", i, err)
		}
	}
}

func TestInvalidPipeline(t *testing.T) {
	for _, description := range []string{"missing_vim4_element ! appsink name=frames", "videotestsrc ! fakesink name=frames"} {
		if err := Run(context.Background(), description, nil, func(Frame) error { return nil }); err == nil {
			t.Fatalf("accepted %q", description)
		}
	}
}

func TestKeyframeRequest(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	requests := make(chan struct{}, 1)
	frames, requestedAt, keyAt := 0, -1, -1
	err := Run(ctx, "videotestsrc ! video/x-raw,width=64,height=64,framerate=30/1 ! openh264enc gop-size=1000 ! h264parse ! video/x-h264,stream-format=byte-stream,alignment=au ! appsink name=frames sync=false", requests, func(f Frame) error {
		if frames == 0 && !f.Keyframe {
			t.Fatal("stream did not start with a keyframe")
		}
		if frames > 0 && f.Keyframe && requestedAt < 0 {
			t.Fatal("unrequested keyframe")
		}
		if frames == 10 {
			requestedAt = frames
			requests <- struct{}{}
		}
		if requestedAt >= 0 && frames > requestedAt && f.Keyframe {
			keyAt = frames
			cancel()
		}
		frames++
		return nil
	})
	if !errors.Is(err, context.Canceled) || keyAt < 0 || keyAt > requestedAt+5 {
		t.Fatalf("requested at %d, keyframe at %d: %v", requestedAt, keyAt, err)
	}
}
