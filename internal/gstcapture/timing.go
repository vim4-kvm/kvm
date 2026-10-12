//go:build linux && vim4

package gstcapture

import "time"

func frameDuration(previous, pts, nominal uint64) time.Duration {
	const unknown = ^uint64(0)
	// amlvenc can stamp a nominal 30 fps duration on a 60 fps stream.
	if previous != unknown && pts != unknown && pts > previous {
		return time.Duration(pts - previous)
	}
	if nominal != unknown {
		return time.Duration(nominal)
	}
	return 0
}
