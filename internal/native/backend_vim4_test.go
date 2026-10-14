//go:build linux && vim4

package native

import "testing"

func TestVIM4CustomBitrateSurvivesBackendInitialization(t *testing.T) {
	backend, err := NewBackend(NativeOptions{DefaultQualityFactor: 5})
	if err != nil {
		t.Fatal(err)
	}
	if got, err := backend.VideoGetQualityFactor(); err != nil || got != 5 {
		t.Fatalf("stored 60 Mbps factor changed: %v, %v", got, err)
	}
	if err := backend.VideoSetQualityFactor(6); err == nil {
		t.Fatal("accepted a bitrate above 60 Mbps")
	}
	if got, _ := backend.VideoGetQualityFactor(); got != 5 {
		t.Fatal("rejected bitrate changed the active setting")
	}
}
