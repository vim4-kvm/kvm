//go:build vim4

package kvm

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
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
	vim4Media = &vim4media.Controller{LUN: dir, Apply: func(string, bool, vim4media.Snapshot) (vim4media.Snapshot, error) {
		t.Fatal("foreign media reached helper")
		return vim4media.Snapshot{}, nil
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

func TestVIM4CatalogMountAndDeleteBoundary(t *testing.T) {
	previous, previousState, previousCatalog := vim4Media, currentVirtualMediaState, getVIM4Catalog
	t.Cleanup(func() { vim4Media, currentVirtualMediaState, getVIM4Catalog = previous, previousState, previousCatalog })
	dir := t.TempDir()
	write := func(s vim4media.Snapshot) {
		t.Helper()
		cdrom := "0"
		if s.CDROM {
			cdrom = "1"
		}
		for key, value := range map[string]string{"file": s.File, "ro": "1", "cdrom": cdrom} {
			if err := os.WriteFile(filepath.Join(dir, key), []byte(value), 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
	getVIM4Catalog = func() ([]vim4CatalogEntry, error) {
		return []vim4CatalogEntry{{ID: "shared/boot.iso", Source: "shared", CDROM: true}, {ID: "data/work.img", Source: "data"}}, nil
	}
	vim4Media = &vim4media.Controller{LUN: dir, Apply: func(id string, cdrom bool, expected vim4media.Snapshot) (vim4media.Snapshot, error) {
		mounted := vim4media.Snapshot{ReadOnly: true, CDROM: cdrom}
		if id != "" {
			mounted.File = "/resolved/" + filepath.Base(id)
		}
		write(mounted)
		return mounted, nil
	}}
	for _, id := range []string{"shared/boot.iso", "data/work.img"} {
		write(vim4media.Snapshot{})
		if err := rpcMountWithStorage(id, Disk); err != nil {
			t.Fatal(err)
		}
		state, err := rpcGetVirtualMediaState()
		if err != nil || state == nil || state.Source != Storage || state.ReadOnly == nil || !*state.ReadOnly {
			t.Fatalf("mounted catalog media: %+v %v", state, err)
		}
		if (state.Mode == CDROM) != strings.HasSuffix(id, ".iso") {
			t.Fatalf("wrong mode: %+v", state)
		}
		if err := rpcDeleteStorageFile(id); err == nil {
			t.Fatal("catalog deletion accepted")
		}
		if err := rpcUnmountImage(); err != nil {
			t.Fatal(err)
		}
	}
	if err := rpcMountWithStorage("data/../outside.img", Disk); err == nil {
		t.Fatal("unknown catalog ID accepted")
	}
}
