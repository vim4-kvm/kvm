//go:build vim4

package kvm

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/jetkvm/kvm/internal/vim4media"
)

func TestVIM4MediaRPCUsesLiveLUN(t *testing.T) {
	previous, previousState := vim4Media, currentVirtualMediaState
	t.Cleanup(func() { vim4Media, currentVirtualMediaState = previous, previousState })
	dir := t.TempDir()
	write := func(file string) {
		t.Helper()
		for name, value := range map[string]string{"file": file, "ro": "0", "cdrom": "0"} {
			if err := os.WriteFile(filepath.Join(dir, name), []byte(value+"\n"), 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
	vim4Media = &vim4media.Controller{LUN: dir, Apply: func(string, bool, vim4media.Snapshot) error {
		t.Fatal("foreign media reached helper")
		return nil
	}}
	write("/var/lib/vim4-kvm/usb/disk.img")
	if err := setInitialVirtualMediaState(); err != nil {
		t.Fatal(err)
	}
	state, err := rpcGetVirtualMediaState()
	if err != nil || state == nil || state.Source != External || state.Filename != "disk.img" || state.Mode != Disk || state.ReadOnly == nil || *state.ReadOnly {
		t.Fatalf("foreign disk state: %+v, %v", state, err)
	}
	if err := rpcMountWithStorage("new.iso", CDROM); !errors.Is(err, vim4media.ErrForeign) {
		t.Fatalf("mount: %v", err)
	}
	if err := rpcUnmountImage(); !errors.Is(err, vim4media.ErrForeign) {
		t.Fatalf("unmount: %v", err)
	}
	write("")
	state, err = rpcGetVirtualMediaState()
	if err != nil || state != nil || currentVirtualMediaState != nil {
		t.Fatal("external eject retained stale state")
	}
}
