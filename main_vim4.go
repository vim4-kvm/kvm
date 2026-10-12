package kvm

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jetkvm/kvm/internal/usbgadget"
)

// VIM4 runs as an Ubuntu application. Board services exclusively own PD, HDMI
// source activation and configfs. Do not initialize appliance system managers.
func mainVIM4() {
	if err := os.MkdirAll(filepath.Dir(configPath), 0700); err != nil {
		logger.Fatal().Err(err).Msg("create state directory")
	}
	LoadConfig()
	config.VideoCodecPreference = "h264"
	config.AudioEnabled = false
	config.UsbDevices = &usbgadget.Devices{Keyboard: true, AbsoluteMouse: true, RelativeMouse: true}
	var cancel context.CancelFunc
	appCtx, cancel = context.WithCancel(context.Background())
	defer cancel()
	system, app, _ := GetLocalVersion()
	initUsbGadget()
	defer gadget.Close()
	defer func() { _ = gadget.KeyboardReport(0, keyboardClearStateKeys); gadget.ReleaseMouseButtons() }()
	initNative(system, app)
	defer nativeInstance.VideoStop()
	initPrometheus()
	server := &http.Server{Addr: ":8080", Handler: setupRouter(), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			logger.Fatal().Err(err).Msg("VIM4 web server")
		}
	}()
	logger.Info().Msg("VIM4 preview listening on :8080")
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM, syscall.SIGINT)
	defer signal.Stop(signals)
	<-signals
	ctx, stop := context.WithTimeout(context.Background(), 5*time.Second)
	defer stop()
	_ = server.Shutdown(ctx)
}

// Keep unsupported system actions unreachable through both HTTP and data
// channels. A disabled UI control or config default is not an API boundary.
func vim4HTTP(c *gin.Context) {
	switch c.Request.URL.Path {
	case "/cloud/register", "/storage/upload", "/diagnostics":
		c.AbortWithStatusJSON(http.StatusNotImplemented, gin.H{"error": "unavailable on VIM4"})
		return
	}
	c.Next()
}

func vim4RPCSupported(method string) bool {
	switch method {
	case "ping", "getDeviceID", "getLocalVersion", "getDeviceCapabilities",
		"getVideoState", "getUSBState", "getUsbState", "getUsbDevices", "getUsbConfig", "getUsbEmulationState",
		"getKeyboardLedState", "getKeyDownState", "getKeyboardLayout", "setKeyboardLayout",
		"getStreamQualityFactor", "getSupportedVideoCodecs", "getVideoCodecPreference",
		"getVideoLogStatus", "getVideoSleepMode", "setVideoStreamPaused",
		"keyboardReport", "keypressReport", "absMouseReport", "relMouseReport", "wheelReport",
		"getKeyboardMacros", "setKeyboardMacros", "executeKeyboardMacro", "cancelKeyboardMacro":
		return true
	}
	return false
}
