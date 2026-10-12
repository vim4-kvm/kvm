//go:build vim4

package kvm

import "testing"

func TestVIM4RejectsSystemManagement(t *testing.T) {
	for _, method := range []string{"setNetworkSettings", "renewDHCPLease", "setUsbDevices", "setUsbConfig", "setUsbEmulationState", "setAutoUpdateState", "tryUpdate", "factoryReset", "reboot", "setSSHKeyState", "setActiveExtension"} {
		if vim4RPCSupported(method) {
			t.Errorf("system operation exposed: %s", method)
		}
	}
	for _, method := range []string{"keyboardReport", "absMouseReport", "relMouseReport", "wheelReport", "getVideoState", "getKeyDownState"} {
		if !vim4RPCSupported(method) {
			t.Errorf("KVM operation unavailable: %s", method)
		}
	}
}
