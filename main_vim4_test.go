//go:build vim4

package kvm

import (
	"encoding/hex"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/gin-gonic/gin"
	"github.com/jetkvm/kvm/internal/native"
)

func TestManagerBackendIgnoresOldPublicListenerSettings(t *testing.T) {
	previous := config
	t.Cleanup(func() { config = previous })
	cfg := getDefaultConfig()
	if cfg.LocalAuthMode != "noPassword" || !cfg.LocalLoopbackOnly || cfg.TLSMode != "" {
		t.Fatal("fresh or reset config bypasses the manager gateway")
	}
	cfg.LocalAuthMode = "password"
	cfg.TLSMode = "custom"
	cfg.LocalLoopbackOnly = false
	normalizePlatformWebConfig(&cfg)
	config = &cfg
	if cfg.LocalAuthMode != "noPassword" || cfg.TLSMode != "" || !cfg.LocalLoopbackOnly {
		t.Fatal("old config bypasses the manager gateway")
	}
	if got := getBindAddress(8081); got != "127.0.0.1:8081" {
		t.Fatalf("backend address: %s", got)
	}
	for _, method := range []string{"setTLSState", "setLocalLoopbackOnly"} {
		if !vim4RPCUnsupported(method) {
			t.Fatalf("gateway-owned setting is mutable: %s", method)
		}
	}
	router := gin.New()
	router.Use(vim4HTTP)
	router.POST("/auth/password-local", handleCreatePassword)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest("POST", "/auth/password-local", nil))
	if response.Code != 501 || cfg.LocalAuthMode != "noPassword" {
		t.Fatalf("application password can be enabled: status %d", response.Code)
	}
}

func TestVIM4RejectsUnsupportedOperations(t *testing.T) {
	for _, method := range []string{"setDisplayRotation", "setBacklightSettings", "setDCPowerState", "setActiveExtension",
		"setATXPowerAction", "setSerialSettings", "setUsbDevices", "setUsbConfig", "setCloudUrl", "deregisterDevice",
		"getTailscaleStatus", "getTailscaleControlURL", "setTailscaleControlURL"} {
		if !vim4RPCUnsupported(method) {
			t.Errorf("operation without VIM4 support exposed: %s", method)
		}
	}
	for _, method := range []string{"keyboardReport", "absMouseReport", "getVideoState", "setNetworkSettings", "renewDHCPLease",
		"mountWithStorage", "setEDID", "setUsbEmulationState", "tryUpdate", "factoryReset", "reboot", "setSSHKeyState",
		"setAudioConfig", "setMqttSettings", "getWifiSettings"} {
		if vim4RPCUnsupported(method) {
			t.Errorf("implemented operation rejected: %s", method)
		}
		if _, ok := rpcHandlers[method]; !ok {
			t.Errorf("operation not registered: %s", method)
		}
	}
}

func TestVIM4VersionOrderUsesBuildNumbers(t *testing.T) {
	cases := []struct {
		remote, local string
		newer         bool
	}{
		{"1.0.3+vim4.20261013.31", "1.0.3+vim4.20261013.30", true},
		{"1.0.3+vim4.20261013.9", "1.0.3+vim4.20261013.10", false},
		{"1.0.3+vim4.20261014.1", "1.0.3+vim4.20261013.30", true},
		{"1.0.4+vim4.20261001.1", "1.0.3+vim4.20261013.30", true},
		{"1.0.3+vim4.20261013.30", "1.0.3+vim4.20261013.30", false},
		{"1.0.2+vim4.20261020.1", "1.0.3+vim4.20261013.30", false},
		{"1.0.3+vim4.20261013.30", "1.0.3", true},
		{"2.5.1+vim4.20261016.1020", "2.5.1+vim4.20261016.1020.dev.1.g49c45f2", false},
		{"2.5.1+vim4.20261016.1022", "2.5.1+vim4.20261016.1020.dev.9.g49c45f2.dirty", true},
		{"2.5.1+vim4.20261017.1", "2.5.1+vim4.20261016.1020.dev.9.g49c45f2", true},
		{"2.5.1+vim4.20261016.1020.dev.10.gabcdef0", "2.5.1+vim4.20261016.1020.dev.9.g49c45f2", true},
	}
	for _, c := range cases {
		if got := vim4VersionNewer(semver.MustParse(c.remote), semver.MustParse(c.local)); got != c.newer {
			t.Errorf("%s newer than %s: %v, want %v", c.remote, c.local, got, c.newer)
		}
	}
}

func TestNetworkManagerLeaseParsing(t *testing.T) {
	lease := parseNMDHCPOptions("broadcast_address = 192.168.50.255 | dhcp_lease_time = 43200 | dhcp_server_identifier = 192.168.50.1 | " +
		"domain_name = lan | domain_name_servers = 192.168.50.1 1.1.1.1 | expiry = 1791933832 | ip_address = 192.168.50.210 | " +
		"ntp_servers = 192.168.50.2 | routers = 192.168.50.1 | subnet_mask = 255.255.255.0")
	if lease == nil || lease.IPAddress.String() != "192.168.50.210" || lease.Netmask.String() != "255.255.255.0" ||
		len(lease.DNS) != 2 || lease.DNS[1].String() != "1.1.1.1" || lease.Domain != "lan" ||
		lease.LeaseTime != 12*time.Hour || lease.LeaseExpiry == nil || lease.LeaseExpiry.Unix() != 1791933832 ||
		len(lease.NTPServers) != 1 || lease.ServerID != "192.168.50.1" {
		t.Fatalf("unexpected lease: %+v", lease)
	}
	if parseNMDHCPOptions("") != nil {
		t.Fatal("static configuration produced a lease")
	}
}

func TestVIM4EDIDPresetsKeepPlatformDefaultFirst(t *testing.T) {
	presets, _ := rpcGetEDIDPresets()
	if presets[0].EDID != defaultVideoEDID() || len(presets) != len(edidPresets)+len(vim4EDIDPresets) {
		t.Fatalf("unexpected preset order: %v", presets[0].Name)
	}
	for _, preset := range vim4EDIDPresets {
		data, err := hex.DecodeString(preset.EDID)
		if err != nil || len(data) < 128 || len(data) != 128*(1+int(data[126])) {
			t.Errorf("%s has invalid EDID length or extension count", preset.Name)
		}
	}
}

func TestVIM4DefaultEDIDAndSavedSelections(t *testing.T) {
	previous := config
	t.Cleanup(func() { config = previous })
	initial := getDefaultConfig()
	config = &initial
	wantDefault := vim4EDIDPresets[0].EDID
	if got := configuredVideoEDID(); got != wantDefault {
		t.Fatal("first startup did not select the VIM4 4K60 EDID")
	}
	config.EdidString = native.DisabledEDID
	if got := configuredVideoEDID(); got != wantDefault {
		t.Fatal("restoring a hidden display did not use the platform default")
	}
	for _, saved := range []string{native.DefaultEDID, vim4EDIDPresets[1].EDID, edidPresets[1].EDID} {
		config.EdidString = saved
		if got := configuredVideoEDID(); got != saved {
			t.Fatal("saved EDID selection was replaced with the platform default")
		}
	}
}
