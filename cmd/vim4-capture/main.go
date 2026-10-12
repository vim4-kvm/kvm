//go:build linux && vim4

// vim4-capture exercises the same video backend used by WebRTC without starting
// the web server or opening any HID endpoint.
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/jetkvm/kvm/internal/native"
)

func main() {
	output := flag.String("output", "/tmp/vim4-capture.h264", "Annex B output file")
	flag.Parse()
	if err := run(*output); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(output string) error {
	f, err := os.Create(output)
	if err != nil {
		return err
	}
	defer f.Close()
	finished := make(chan error, 1)
	frames := 0
	var duration time.Duration
	backend, err := native.NewBackend(native.NativeOptions{OnVideoFrameReceived: func(frame []byte, d time.Duration) {
		if frames >= 300 {
			return
		}
		if _, err := f.Write(frame); err != nil {
			select {
			case finished <- err:
			default:
			}
			return
		}
		frames++
		duration += d
		if frames == 300 {
			finished <- nil
		}
	}})
	if err != nil {
		return err
	}
	if err = backend.Start(); err != nil {
		return err
	}
	if err = backend.VideoStart(); err != nil {
		return err
	}
	select {
	case err = <-finished:
	case <-time.After(20 * time.Second):
		err = fmt.Errorf("capture timed out")
	}
	_ = backend.VideoStop()
	if err != nil {
		return err
	}
	fmt.Printf("frames=%d media_duration=%s output=%s\n", frames, duration, output)
	return nil
}
