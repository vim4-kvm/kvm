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

const hdmiInfoPath = "/sys/class/hdmirx/hdmirx0/info"

var videoColor atomic.Uint32

// Read only by the RTP writer synchronously called for the current frame.
func VideoColorSpace() []byte {
	color := videoColor.Load()
	if color == 0 {
		return nil
	}
	return []byte{byte(color >> 24), byte(color >> 16), byte(color >> 8), byte(color)}
}

// The headless backend does not run the local display loop or native supervisor.
// The system service owns HDMI source setup; this backend owns only the encoder.
type vim4Backend struct {
	EmptyNativeInterface
	opts      NativeOptions
	mu        sync.Mutex
	cancel    context.CancelFunc
	done      chan struct{}
	status    atomic.Uint32
	codec     int
	quality   float64
	height    int
	keyframes chan struct{}
}

var _ NativeInterface = (*vim4Backend)(nil)

func NewBackend(opts NativeOptions) (NativeInterface, error) {
	quality := opts.DefaultQualityFactor
	if !validVIM4Quality(quality) {
		quality = 1
	}
	return &vim4Backend{opts: opts, quality: quality, keyframes: make(chan struct{}, 1)}, nil
}

func (v *vim4Backend) VideoSetCodecType(codec int) error {
	if codec != 0 && codec != 1 {
		return fmt.Errorf("unsupported codec type %d", codec)
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.codec != codec {
		v.codec = codec
		v.restartLocked()
	}
	return nil
}

func (v *vim4Backend) VideoGetCodecType() (int, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.codec, nil
}

func (v *vim4Backend) VideoGetQualityFactor() (float64, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.quality, nil
}

// Output scaling changes only the stream, never HDMI timing or EDID.
func (v *vim4Backend) VideoSetOutputHeight(height int) error {
	switch height {
	case 0, 480, 720, 1080, 1440, 1700, 2160:
	default:
		return fmt.Errorf("unsupported output height %d", height)
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.height != height {
		v.height = height
		v.restartLocked()
	}
	return nil
}

// VideoSetQualityFactor selects the encoder bitrate. The vendor encoder cannot
// change its bitrate while running, so a running pipeline is reopened.
func (v *vim4Backend) VideoSetQualityFactor(factor float64) error {
	if !validVIM4Quality(factor) {
		return fmt.Errorf("quality factor %v is outside (0, 5]", factor)
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.quality != factor {
		v.quality = factor
		v.restartLocked()
	}
	return nil
}

func (v *vim4Backend) VideoSetEDID(string) error {
	return errors.New("the VIM4 HDMI receiver EDID is set by the application")
}
func (v *vim4Backend) VideoGetEDID() (string, error) {
	return "", errors.New("the VIM4 HDMI receiver EDID is set by the application")
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

// VideoRequestKeyframe asks the running encoder for an IDR frame, for a viewer
// that joined a running stream or reported loss with a PLI or FIR.
func (v *vim4Backend) VideoRequestKeyframe() {
	select {
	case v.keyframes <- struct{}{}:
	default:
	}
}

func (v *vim4Backend) VideoStart() error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.runningLocked() {
		v.VideoRequestKeyframe()
		return nil
	}
	v.startLocked()
	return nil
}

func (v *vim4Backend) VideoStop() error {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.stopLocked()
	return nil
}

// Keep starts and encoder reconfiguration out until the HDMI operation has
// completed. A reconnect loop can own DMA even before its first active frame.
func (v *vim4Backend) WithVideoPaused(action func() error) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	restart := v.runningLocked()
	v.stopLocked()
	if restart {
		defer v.startLocked()
	}
	return action()
}

func (v *vim4Backend) runningLocked() bool {
	if v.done == nil {
		return false
	}
	select {
	case <-v.done:
		return false
	default:
		return true
	}
}

func (v *vim4Backend) startLocked() {
	ctx, cancel := context.WithCancel(context.Background())
	v.cancel = cancel
	v.done = make(chan struct{})
	go v.capture(ctx, v.done, v.codec, v.quality, v.height)
}

func (v *vim4Backend) stopLocked() {
	if v.cancel != nil {
		v.cancel()
		<-v.done
		v.cancel = nil
		v.done = nil
	}
}

func (v *vim4Backend) restartLocked() {
	if v.runningLocked() {
		v.stopLocked()
		v.startLocked()
	}
}

func (v *vim4Backend) report(state VideoState) {
	v.status.Store(uint32(state.Streaming))
	if v.opts.OnVideoStateChange != nil {
		v.opts.OnVideoStateChange(state)
	}
}

// capture keeps the pipeline running until cancellation. A pipeline error,
// such as a lost HDMI signal, or a changed source rate reopens the encoder.
func (v *vim4Backend) capture(ctx context.Context, done chan struct{}, codec int, quality float64, height int) {
	defer close(done)
	defer v.report(VideoState{Streaming: VideoStreamingStatusInactive})
	for ctx.Err() == nil {
		rate := hdmiFrameRate(vim4HDMIInfo())
		if rate == 0 {
			rate = vim4DefaultFrameRate
		}
		err := v.runPipeline(ctx, codec, quality, rate, height)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			nativeLogger.Warn().Err(err).Msg("VIM4 capture stopped; retrying")
			v.report(VideoState{Streaming: VideoStreamingStatusInactive, Error: "no_signal"})
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second):
			}
		}
	}
}

func (v *vim4Backend) runPipeline(ctx context.Context, codec int, quality float64, rate, height int) error {
	pipelineCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	rateChanged := make(chan struct{})
	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		var change rateChange
		for {
			select {
			case <-pipelineCtx.Done():
				return
			case <-ticker.C:
				if change.observe(rate, hdmiFrameRate(vim4HDMIInfo())) {
					close(rateChanged)
					cancel()
					return
				}
			}
		}
	}()
	// A new encoder starts with an IDR frame; drop requests made for the old one.
	select {
	case <-v.keyframes:
	default:
	}
	nativeLogger.Info().Int("codec", codec).Int("bitrate_kbps", vim4Bitrate(quality)).Int("fps", rate).Msg("VIM4 encoder starting")
	lastState := VideoState{}
	frameCount := 0
	err := gstcapture.Run(pipelineCtx, vim4Pipeline(codec, quality, rate, height), v.keyframes, func(frame gstcapture.Frame) error {
		duration := frame.Duration
		if duration <= 0 {
			duration = time.Second / time.Duration(rate)
		}
		state := VideoState{Ready: true, Streaming: VideoStreamingStatusActive, Width: frame.Width, Height: frame.Height, FramePerSecond: float64(time.Second) / float64(duration)}
		frameCount++
		// Avoid broadcasting PTS jitter as a format change on every frame.
		if !lastState.Ready || frameCount == 2 || state.Width != lastState.Width || state.Height != lastState.Height {
			v.report(state)
			lastState = state
		}
		if v.opts.OnVideoFrameReceived != nil {
			color := frame.ColorSpace
			videoColor.Store(uint32(color[0])<<24 | uint32(color[1])<<16 | uint32(color[2])<<8 | uint32(color[3]))
			v.opts.OnVideoFrameReceived(frame.Data, duration)
		}
		return nil
	})
	select {
	case <-rateChanged:
		nativeLogger.Info().Int("previous_fps", rate).Msg("VIM4 HDMI frame rate changed; reopening the encoder")
		return nil
	default:
	}
	if errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}
