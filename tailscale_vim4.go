package kvm

import (
	"strings"
	"sync/atomic"
	"time"

	"github.com/jetkvm/kvm/internal/platform"
	"github.com/jetkvm/kvm/internal/tailscale"
)

// Ubuntu's tailscale chooses the control server at login and accepts
// preference changes from root or its operator only. On VIM4 the helper
// starts that login; status reads stay direct.
func init() {
	if !platform.VIM4 {
		return
	}
	run := tailscale.ExecCommand
	tailscale.ExecCommand = func(args ...string) ([]byte, error) {
		if len(args) == 2 && args[0] == "set" && strings.HasPrefix(args[1], "--login-server=") {
			url := strings.TrimPrefix(args[1], "--login-server=")
			return nil, vim4Helper(40*time.Second, "tailscale-login", map[string]string{"url": url}, nil)
		}
		return run(args...)
	}
}

var vim4TailscaleLoginRequested atomic.Int64

// requestVIM4TailscaleLogin makes tailscaled publish a login URL. The
// appliance image starts this login itself; Ubuntu's tailscaled waits.
func requestVIM4TailscaleLogin(status *tailscale.Status) {
	if status == nil || !status.Installed || status.BackendState != "NeedsLogin" || status.AuthURL != "" {
		return
	}
	now := time.Now().Unix()
	last := vim4TailscaleLoginRequested.Load()
	if now-last < 60 || !vim4TailscaleLoginRequested.CompareAndSwap(last, now) {
		return
	}
	go func() {
		url := tailscale.EffectiveControlURL(config.TailscaleControlURL)
		if err := vim4Helper(30*time.Second, "tailscale-login", map[string]string{"url": url}, nil); err != nil {
			tailscaleLogger.Warn().Err(err).Msg("failed to request a Tailscale login URL")
		}
	}()
}
