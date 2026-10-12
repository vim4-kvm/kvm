//go:build linux && vim4

package gstcapture

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"
)

func TestTimingAndEOS(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	frames := 0
	err := Run(ctx, "videotestsrc num-buffers=3 ! video/x-raw,width=16,height=16,framerate=25/1 ! appsink name=frames sync=false", func(f Frame) error {
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
		err := Run(ctx, "videotestsrc is-live=true ! appsink name=frames sync=false", func(Frame) error { cancel(); return nil })
		cancel()
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("run %d: %v", i, err)
		}
	}
}

func TestInvalidPipeline(t *testing.T) {
	for _, description := range []string{"missing_vim4_element ! appsink name=frames", "videotestsrc ! fakesink name=frames"} {
		if err := Run(context.Background(), description, func(Frame) error { return nil }); err == nil {
			t.Fatalf("accepted %q", description)
		}
	}
}
