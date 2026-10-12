//go:build linux && vim4

package native

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jetkvm/kvm/internal/gstcapture"
)

// The headless backend does not run the local display loop or native supervisor.
// The system service owns HDMI source setup; this backend owns only the encoder.
type vim4Backend struct {
	EmptyNativeInterface
	opts   NativeOptions
	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
	status atomic.Uint32
}

var _ NativeInterface = (*vim4Backend)(nil)

func NewBackend(opts NativeOptions) (NativeInterface, error) {
	return &vim4Backend{opts: opts}, nil
}

func (v *vim4Backend) VideoSetCodecType(codec int) error {
	if codec != 0 {
		return errors.New("VIM4 currently supports H.264 only")
	}
	return nil
}
func (v *vim4Backend) VideoGetQualityFactor() (float64, error) { return 1, nil }
func (v *vim4Backend) VideoSetQualityFactor(factor float64) error {
	if factor != 1 {
		return errors.New("VIM4 output scaling is managed by the HDMI service")
	}
	return nil
}
func (v *vim4Backend) VideoSetEDID(string) error {
	return errors.New("VIM4 EDID control is not implemented")
}
func (v *vim4Backend) VideoGetEDID() (string, error) {
	return "", errors.New("VIM4 EDID control is not implemented")
}
func (v *vim4Backend) VideoSetSleepMode(enabled bool) error {
	if enabled {
		return errors.New("VIM4 HDMI sleep is not supported")
	}
	return nil
}
func (v *vim4Backend) VideoLogStatus() (string, error) {
	return fmt.Sprintf("VIM4 GStreamer: %s", v.VideoGetStreamingStatus()), nil
}
func (v *vim4Backend) VideoGetStreamingStatus() VideoStreamingStatus {
	return VideoStreamingStatus(v.status.Load())
}

func (v *vim4Backend) VideoStart() error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.done != nil {
		select {
		case <-v.done:
		default:
			return nil
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	v.cancel = cancel
	v.done = make(chan struct{})
	go v.capture(ctx, v.done)
	return nil
}

func (v *vim4Backend) VideoStop() error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.cancel != nil {
		v.cancel()
		<-v.done
		v.cancel = nil
		v.done = nil
	}
	return nil
}

func (v *vim4Backend) report(state VideoState) {
	v.status.Store(uint32(state.Streaming))
	if v.opts.OnVideoStateChange != nil {
		v.opts.OnVideoStateChange(state)
	}
}

func (v *vim4Backend) capture(ctx context.Context, done chan struct{}) {
	defer close(done)
	lastState := VideoState{}
	frameCount := 0
	// Use the encoder's maximum supported bitrate for 4K desktop detail.
	err := gstcapture.Run(ctx, "vfmcapsrc ! queue max-size-buffers=3 max-size-bytes=0 max-size-time=0 ! amlvenc bitrate=12000 gop=30 ! video/x-h264 ! h264parse config-interval=-1 ! video/x-h264,stream-format=byte-stream,alignment=au ! appsink name=frames sync=false", func(frame gstcapture.Frame) error {
		duration := frame.Duration
		if duration <= 0 {
			duration = time.Second / 60
		}
		state := VideoState{Ready: true, Streaming: VideoStreamingStatusActive, Width: frame.Width, Height: frame.Height, FramePerSecond: float64(time.Second) / float64(duration)}
		frameCount++
		// Avoid broadcasting PTS jitter as a format change on every frame.
		if !lastState.Ready || frameCount == 2 || state.Width != lastState.Width || state.Height != lastState.Height {
			v.report(state)
			lastState = state
		}
		if v.opts.OnVideoFrameReceived != nil {
			v.opts.OnVideoFrameReceived(frame.Data, duration)
		}
		return nil
	})
	state := VideoState{Streaming: VideoStreamingStatusInactive}
	if err != nil && !errors.Is(err, context.Canceled) {
		state.Error = "no_signal"
		nativeLogger.Error().Err(err).Msg("VIM4 capture stopped")
	}
	v.report(state)
}
