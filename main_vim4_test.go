//go:build vim4

package kvm

import (
	"testing"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/jetkvm/kvm/internal/native"
)

func TestVIM4RejectsOperationsWithoutHardware(t *testing.T) {
	for _, method := range []string{"setDisplayRotation", "setBacklightSettings", "setDCPowerState", "setActiveExtension",
		"setATXPowerAction", "setSerialSettings", "setUsbDevices", "setUsbConfig", "setCloudUrl", "deregisterDevice"} {
		if !vim4RPCUnsupported(method) {
			t.Errorf("operation without VIM4 support exposed: %s", method)
		}
	}
	for _, method := range []string{"keyboardReport", "absMouseReport", "getVideoState", "setNetworkSettings", "renewDHCPLease",
		"mountWithStorage", "setEDID", "setUsbEmulationState", "tryUpdate", "factoryReset", "reboot", "setSSHKeyState",
		"getTailscaleStatus", "setAudioConfig", "setMqttSettings", "getWifiSettings"} {
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

func TestVIM4EDIDPresetsKeepJetKVMDefaultFirst(t *testing.T) {
	presets, _ := rpcGetEDIDPresets()
	if presets[0].EDID != native.DefaultEDID || len(presets) != len(edidPresets)+len(vim4EDIDPresets) {
		t.Fatalf("unexpected preset order: %v", presets[0].Name)
	}
	for _, preset := range vim4EDIDPresets {
		if len(preset.EDID) != 512 {
			t.Errorf("%s is not a two-block EDID", preset.Name)
		}
	}
}
