package vim4media

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func writeLUN(t *testing.T, dir string, media Snapshot) {
	t.Helper()
	attrs := map[string]string{"file": media.File, "ro": "0", "cdrom": "0"}
	if media.ReadOnly {
		attrs["ro"] = "1"
	}
	if media.CDROM {
		attrs["cdrom"] = "1"
	}
	for name, value := range attrs {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(value+"\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func fakeController(t *testing.T) (*Controller, *int) {
	t.Helper()
	dir := t.TempDir()
	writeLUN(t, dir, Snapshot{})
	calls := 0
	c := &Controller{LUN: dir}
	c.Apply = func(file string, cdrom bool, expected Snapshot) (Snapshot, error) {
		calls++
		actual, err := c.read()
		if err != nil {
			return Snapshot{}, err
		}
		if actual != expected {
			return Snapshot{}, errors.New("media changed")
		}
		writeLUN(t, dir, Snapshot{File: file, CDROM: cdrom, ReadOnly: true})
		return Snapshot{File: file, CDROM: cdrom, ReadOnly: true}, nil
	}
	return c, &calls
}

func TestForeignMediaVisibleAndProtected(t *testing.T) {
	for _, media := range []Snapshot{
		{File: "/var/lib/other/foreign.iso", CDROM: true, ReadOnly: true},
		{File: "/var/lib/vim4-kvm/usb/disk.img"},
		{File: "/var/lib/jetkvm/images/manager.iso", CDROM: true, ReadOnly: true},
	} {
		t.Run(media.File, func(t *testing.T) {
			c, calls := fakeController(t)
			writeLUN(t, c.LUN, media)
			got, owned, err := c.State()
			if err != nil || owned || got != media {
				t.Fatalf("state=%+v owned=%v err=%v", got, owned, err)
			}
			for _, file := range []string{"/var/lib/jetkvm/images/new.iso", ""} {
				if err := c.Change(file, true); !errors.Is(err, ErrForeign) {
					t.Fatalf("change %q: %v", file, err)
				}
			}
			got, _, _ = c.State()
			if *calls != 0 || got != media {
				t.Fatal("foreign media was modified")
			}
		})
	}
}

func TestOwnMountEjectAndExternalChanges(t *testing.T) {
	c, calls := fakeController(t)
	file := "/var/lib/jetkvm/images/own.iso"
	if err := c.Change(file, true); err != nil {
		t.Fatal(err)
	}
	media, owned, err := c.State()
	if err != nil || !owned || media.File != file || !media.CDROM || !media.ReadOnly {
		t.Fatalf("own mount: %+v %v %v", media, owned, err)
	}
	if err := c.Change("", false); err != nil {
		t.Fatal(err)
	}
	media, owned, err = c.State()
	if err != nil || owned || media.File != "" || *calls != 2 {
		t.Fatal("own eject failed")
	}
	if err := c.Change(file, false); err != nil {
		t.Fatal(err)
	}
	writeLUN(t, c.LUN, Snapshot{})
	media, owned, err = c.State()
	if err != nil || owned || media.File != "" {
		t.Fatal("external eject retained stale state")
	}
	// The same file mounted later by the manager must not regain ownership.
	writeLUN(t, c.LUN, Snapshot{File: file, ReadOnly: true})
	if err := c.Change("", false); !errors.Is(err, ErrForeign) {
		t.Fatal("ownership survived external eject")
	}
	writeLUN(t, c.LUN, Snapshot{})
	if err := c.Change(file, true); err != nil {
		t.Fatalf("mount after external eject: %v", err)
	}
	writeLUN(t, c.LUN, Snapshot{File: "/other.img"})
	if err := c.Change("", false); !errors.Is(err, ErrForeign) {
		t.Fatal("external replacement was ejected")
	}
}

func TestFailedMountAndUnreadableLUNDoNotGrantOwnership(t *testing.T) {
	c, calls := fakeController(t)
	c.Apply = func(string, bool, Snapshot) (Snapshot, error) { return Snapshot{}, errors.New("helper failed") }
	if err := c.Change("/own.iso", true); err == nil {
		t.Fatal("expected helper failure")
	}
	writeLUN(t, c.LUN, Snapshot{File: "/own.iso", ReadOnly: true, CDROM: true})
	if err := c.Change("", false); !errors.Is(err, ErrForeign) {
		t.Fatal("failed mount granted ownership")
	}
	if err := os.Remove(filepath.Join(c.LUN, "ro")); err != nil {
		t.Fatal(err)
	}
	if err := c.Change("/new.iso", true); err == nil || *calls != 0 {
		t.Fatal("unreadable LUN allowed mutation")
	}
}

func TestCatalogMountOwnsResolvedPathAndEffectiveMode(t *testing.T) {
	c, _ := fakeController(t)
	mounted := Snapshot{File: "/var/lib/vim4-kvm/images/boot.iso", CDROM: true, ReadOnly: true}
	c.Apply = func(file string, cdrom bool, expected Snapshot) (Snapshot, error) {
		if file == "" {
			mounted = Snapshot{}
		}
		writeLUN(t, c.LUN, mounted)
		return mounted, nil
	}
	if err := c.Change("shared/boot.iso", false); err != nil {
		t.Fatal(err)
	}
	state, owned, err := c.State()
	if err != nil || !owned || state != mounted {
		t.Fatalf("state=%+v owned=%v err=%v", state, owned, err)
	}
	if err := c.Change("", false); err != nil {
		t.Fatal(err)
	}
	state, owned, err = c.State()
	if err != nil || owned || state.File != "" {
		t.Fatalf("eject: %+v %v %v", state, owned, err)
	}
}
