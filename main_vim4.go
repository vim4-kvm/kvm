package kvm

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/gin-gonic/gin"
	"github.com/jetkvm/kvm/internal/native"
	"github.com/jetkvm/kvm/internal/ota"
	"github.com/jetkvm/kvm/internal/platform"
	"github.com/jetkvm/kvm/internal/usbgadget"
)

// VIM4 runs as an Ubuntu application. Board services exclusively own PD, HDMI
// source activation and gadget creation; Ubuntu owns the network stack, clock
// and SSH. Privileged changes go through the vim4-jetkvm helper.
func mainVIM4() {
	if err := os.MkdirAll(filepath.Dir(configPath), 0700); err != nil {
		logger.Fatal().Err(err).Msg("create state directory")
	}
	LoadConfig()
	// The board gadget provides these functions; HDMI audio is not a USB function.
	vim4Devices := usbgadget.Devices{Keyboard: true, AbsoluteMouse: true, RelativeMouse: true, MassStorage: true, Audio: true}
	changed := config.UsbDevices == nil || *config.UsbDevices != vim4Devices
	config.UsbDevices = &vim4Devices
	// Earlier configurations stored JetKVM's release server, which publishes
	// appliance firmware only.
	if config.UpdateAPIURL == DefaultAPIURL {
		config.UpdateAPIURL = ""
		changed = true
	}
	if changed {
		if err := SaveConfig(); err != nil {
			logger.Warn().Err(err).Msg("failed to save VIM4 configuration defaults")
		}
	}

	var cancel context.CancelFunc
	appCtx, cancel = context.WithCancel(context.Background())
	defer cancel()
	system, app, _ := GetLocalVersion()
	logger.Info().Interface("app_version", app).Msg("starting JetKVM on VIM4")
	initUsbGadget()
	defer gadget.Close()
	defer func() { _ = gadget.KeyboardReport(0, keyboardClearStateKeys); gadget.ReleaseMouseButtons() }()
	// The other KVM application expects a connected display when it starts.
	defer func() {
		if vim4HotplugLow.Load() {
			_ = vim4Helper(10*time.Second, "hotplug", map[string]bool{"enabled": true}, nil)
		}
	}()
	initNative(system, app)
	defer nativeInstance.VideoStop()
	http.DefaultClient.Timeout = time.Minute
	initOta()
	initVIM4Network()
	initPrometheus()
	if err := initImagesFolder(); err != nil {
		logger.Warn().Err(err).Msg("failed to init images folder")
	}
	if err := setInitialVirtualMediaState(); err != nil {
		logger.Warn().Err(err).Msg("failed to set initial virtual media state")
	}
	initJiggler()
	initMQTT()
	defer func() {
		if mqttManager != nil {
			mqttManager.Close()
		}
	}()
	go runAutoUpdate()
	go RunWebServer()
	go RunWebSecureServer()
	if config.TLSMode != "" {
		startWebSecureServer()
	}
	initPublicIPState()

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM, syscall.SIGINT)
	defer signal.Stop(signals)
	<-signals
	logger.Info().Msg("JetKVM shutting down")
}

// vim4Capabilities lists the optional features this platform implements.
func vim4Capabilities() []string {
	capabilities := []string{
		"shell", "custom_edid", "upload_channel", "video_during_update", "custom_bitrate",
		"http_proxy", "mdns", "ipv6", "log_level", "update_source",
	}
	if vim4WifiAvailable() {
		capabilities = append(capabilities, "wifi")
	}
	return capabilities
}

// Keep unsupported operations unreachable through both HTTP and data channels.
// A hidden UI control is not an API boundary.
func vim4HTTP(c *gin.Context) {
	switch c.Request.URL.Path {
	case "/cloud/register", "/auth/password-local", "/auth/local-password":
		c.AbortWithStatusJSON(http.StatusNotImplemented, gin.H{"error": "unavailable on VIM4"})
		return
	}
	c.Next()
}

// vim4RPCUnsupported reports operations for hardware this board does not
// have (built-in display, extension port, DC power, serial console), the
// board-owned USB gadget identity, the JetKVM Cloud and appliance DHCP clients.
// The management panel owns Tailscale for the whole board.
func vim4RPCUnsupported(method string) bool {
	switch method {
	case "setTLSState", "setLocalLoopbackOnly":
		return true
	case "getTailscaleStatus", "getTailscaleControlURL", "setTailscaleControlURL",
		"deregisterDevice", "setCloudUrl",
		"setDisplayRotation", "getDisplayRotation", "setBacklightSettings", "getBacklightSettings",
		"getDCPowerState", "setDCPowerState", "setDCRestoreState",
		"getActiveExtension", "setActiveExtension", "getATXState", "setATXPowerAction",
		"getSerialSettings", "setSerialSettings", "sendCustomCommand",
		"getSerialCommandHistory", "setSerialCommandHistory", "deleteSerialCommandHistory", "setTerminalPaused",
		"setUsbConfig", "setUsbDevices", "setUsbDeviceState", "wakeHost":
		return true
	}
	return false
}

func init() {
	if !platform.VIM4 {
		return
	}
	rpcHandlers["getWifiSettings"] = RPCHandler{Func: rpcGetWifiSettings}
	rpcHandlers["scanWifiNetworks"] = RPCHandler{Func: rpcScanWifiNetworks, Params: []string{"refresh"}}
	rpcHandlers["saveWifiNetwork"] = RPCHandler{Func: rpcSaveWifiNetwork, Params: []string{"ssid", "password", "connect"}}
	rpcHandlers["connectWifiNetwork"] = RPCHandler{Func: rpcConnectWifiNetwork, Params: []string{"ssid"}}
	rpcHandlers["removeWifiNetwork"] = RPCHandler{Func: rpcRemoveWifiNetwork, Params: []string{"ssid"}}
}

// The Advanced page's SSH key goes to this account's authorized_keys2.
func rpcGetSSHUsername() string {
	if platform.VIM4 {
		return "khadas"
	}
	return "root"
}

func rpcGetUpdateSource() string {
	return config.UpdateAPIURL
}

// rpcSetUpdateSource stores the release server; GET {url}/releases returns
// the same metadata format as JetKVM's release API.
func rpcSetUpdateSource(rawURL string) (string, error) {
	rawURL = strings.TrimSuffix(strings.TrimSpace(rawURL), "/")
	if rawURL != "" {
		parsed, err := url.Parse(rawURL)
		if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" ||
			parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
			return "", errors.New("the update server must be an http or https URL without credentials or query")
		}
	}
	config.UpdateAPIURL = rawURL
	if err := SaveConfig(); err != nil {
		return "", fmt.Errorf("failed to save config: %w", err)
	}
	otaState.UpdateReleaseAPIEndpoint(config.GetUpdateAPIURL())
	return rawURL, nil
}

func setVIM4OtaOptions(options *ota.Options) {
	options.SkipConfirmSystem = true
	options.AppUpdatePath = platform.StateDir + "/vim4-jetkvm-update.deb"
	options.InstallApp = func(path string) error {
		if err := vim4Helper(15*time.Minute, "update-install", map[string]string{"path": path}, nil); err != nil {
			return err
		}
		// The helper installed its own copy; the download is no longer needed.
		return os.Remove(path)
	}
	// JetKVM signs only its own releases. The package hash from the
	// administrator's update server still has to match before installation.
	options.UnsignedUpdates = true
	options.IsNewer = vim4VersionNewer
}

// vim4VersionNewer orders package versions such as 1.0.3+vim4.20261013.30.
// Semantic versioning ignores build metadata, which numbers VIM4 builds.
func vim4VersionNewer(remote, local *semver.Version) bool {
	if order := remote.Compare(local); order != 0 {
		return order > 0
	}
	remoteParts, localParts := strings.Split(remote.Metadata(), "."), strings.Split(local.Metadata(), ".")
	for i := 0; i < len(remoteParts) && i < len(localParts); i++ {
		if remoteParts[i] == localParts[i] {
			continue
		}
		r, rErr := strconv.ParseUint(remoteParts[i], 10, 64)
		l, lErr := strconv.ParseUint(localParts[i], 10, 64)
		if rErr == nil && lErr == nil {
			return r > l
		}
		return remoteParts[i] > localParts[i]
	}
	return len(remoteParts) > len(localParts)
}

// vim4SetUsbEmulation connects or disconnects the board gadget from the host.
func vim4SetUsbEmulation(enabled bool) error {
	setUSBRecoveryTimer(time.Now())
	gadget.ResetHIDFiles()
	err := vim4Helper(10*time.Second, "usb-emulation", map[string]bool{"enabled": enabled}, nil)
	if enabled && !tryReopenKeyboard("usb-emulation", false) {
		usbLogger.Warn().Msg("keyboard HID file not ready after enabling USB emulation")
	}
	return err
}

// The first vendor table of HDMI RX port B is the VIM4 default. Keep it
// first in the UI so resetting EDID restores the board's 4K60 capabilities.
var vim4EDIDPresets = []EDIDPreset{
	{Name: "VIM4 3840x2160 60Hz", EDID: "00FFFFFFFFFFFF0005AC30000100000020190103807341780ACF74A3574CB02309484C2108000101010101010101010101010101010104740030F2705A80B0588A0020C23100001E023A801871382D40582C450020C23100001E000000FC00414D4C2054560A202020202020000000FD003B461F8C3C000A20202020202001DA02035FF0585F101F1405130420223C3E1216030711150206015D64625E2F097F051507505707013D07C05F7E01830100006E030C001000B83C2F80800102030468D85DC4014B880102E3054001E70E60616A6B6566E3060D01E5018B849001011D007251D01E206E28550020C23100001E000000000000000000000000000012"},
	// Unmodified table of a physical RTK UHD HDR display; also a vim4-edid preset.
	{Name: "RTK UHD HDR 4K60", EDID: "00FFFFFFFFFFFF004A8B1A1B000000002B1E0103A20000783E6435A5544F9E27125054210800D1C0A9C081C00101010101010101010140D000A0F0703E803020350061632100001A000000FC0052544B20554844204844520A20000000FF0064656D6F7365742D310A203020000000FD00283D1E873C000A202020202020014F020348F151044C5D5E5F606100000000006768696A6BE305C00023097F0783010000E50F00000C0067030C001000387867D85DC401788801E606050169694F681A00000101283DE6023A801871382D40582C450061632100001E565E00A0A0A029503020350061632100001E0000000000000000000000000000000000000096"},
	{Name: "VIM4 1920x1080 60Hz", EDID: "00FFFFFFFFFFFF0005AC3000010000002B1B0103807341780ACF74A3574CB02309484C21080001010101010101010101010101010101023A801871382D40582C450020C23100001E011D00BC52D01E20B828554020C23100001E000000FC00414D4C2054560A202020202020000000FD003B461F8C3C000A202020202020019402033EF053101F1405130420223C3E1216030711150206012F097F051507505707013D07C05F7E01830100006E030C001000B83C2F808001020304E200FB011D007251D01E206E28550020C23100001E8C0AD08A20E02D10103E9600138E21000018000000000000000000000000000000000000000000000000000000000084"},
}

// vim4HotplugLow records that hot-plug was pulled low to hide the display.
var vim4HotplugLow atomic.Bool

// vim4SetEDID loads a table into HDMI RX port B, or pulls hot-plug low for
// native.DisabledEDID. The helper skips a table already loaded in this boot,
// so restarting the application does not make the host renegotiate.
func vim4SetEDID(edid string) error {
	if edid == native.DisabledEDID {
		if err := vim4Helper(10*time.Second, "hotplug", map[string]bool{"enabled": false}, nil); err != nil {
			return err
		}
		vim4HotplugLow.Store(true)
		return nil
	}
	if err := vim4Helper(30*time.Second, "edid-apply", map[string]string{"edid": edid}, nil); err != nil {
		return err
	}
	if vim4HotplugLow.Load() {
		if err := vim4Helper(10*time.Second, "hotplug", map[string]bool{"enabled": true}, nil); err != nil {
			return err
		}
		vim4HotplugLow.Store(false)
	}
	return nil
}

var vim4ClockSynced atomic.Bool

// vim4TimeSynced reports whether chrony has synchronized the clock. Once
// synchronized, the result is cached for TLS handshakes.
func vim4TimeSynced() bool {
	if vim4ClockSynced.Load() {
		return true
	}
	output, err := exec.Command("timedatectl", "show", "-p", "NTPSynchronized", "--value").Output()
	if err == nil && strings.TrimSpace(string(output)) == "yes" {
		vim4ClockSynced.Store(true)
		return true
	}
	return false
}

// vim4AppLog returns this boot's application journal for diagnostics.
func vim4AppLog() ([]byte, error) {
	return exec.Command("journalctl", "-u", "vim4-kvm.service", "-b", "--no-pager", "-o", "short-iso").Output()
}
