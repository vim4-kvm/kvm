package kvm

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jetkvm/kvm/internal/platform"
	"github.com/jetkvm/kvm/internal/vim4media"
)

// The board gadget owns this function and links it only while media is mounted.
const vim4LunPath = "/sys/kernel/config/usb_gadget/kvm/functions/mass_storage.disk0/lun.0"

var vim4Media = &vim4media.Controller{LUN: vim4LunPath, Apply: applyVIM4Media}

func getVIM4VirtualMediaState() (*VirtualMediaState, error) {
	media, owned, err := vim4Media.State()
	if err != nil || media.File == "" {
		return nil, err
	}
	state := &VirtualMediaState{Source: External, Mode: Disk, Filename: filepath.Base(media.File), ReadOnly: &media.ReadOnly}
	if owned {
		state.Source = Storage
	}
	if media.CDROM {
		state.Mode = CDROM
	}
	if info, err := os.Stat(media.File); err == nil {
		state.Size = info.Size()
	}
	return state, nil
}

// vim4MediaCDROM is the mode for the next mount. JetKVM selects the mode
// before choosing the image; on VIM4 both are applied in one re-enumeration.
var vim4MediaCDROM = true

func setMassStorageImage(imagePath string) error {
	if !platform.VIM4 {
		return gadget.SetMassStorageImage(imagePath)
	}
	imagePath = strings.TrimSpace(imagePath)
	if imagePath != "" && filepath.Dir(imagePath) != imagesFolder {
		return fmt.Errorf("image must be in %s", imagesFolder)
	}
	return vim4Media.Change(imagePath, vim4MediaCDROM)
}

func applyVIM4Media(imagePath string, cdrom bool, expected vim4media.Snapshot) error {
	var err error
	setUSBRecoveryTimer(time.Now())
	if imagePath == "" {
		err = vim4Helper(30*time.Second, "media-eject", map[string]any{"expected": expected}, nil)
	} else {
		err = vim4Helper(30*time.Second, "media-mount",
			map[string]any{"file": filepath.Base(imagePath), "cdrom": cdrom, "expected": expected}, nil)
	}
	// The helper re-enumerates the gadget even when it restores the previous
	// functions after a failure, so the HID endpoints need fresh handles.
	gadget.ResetHIDFiles()
	if !tryReopenKeyboard("virtual-media", false) {
		usbLogger.Warn().Msg("keyboard HID file not ready after virtual media change")
	}
	return err
}

func getMassStorageImage() (string, error) {
	if !platform.VIM4 {
		return gadget.GetMassStorageImage()
	}
	data, err := os.ReadFile(filepath.Join(vim4LunPath, "file"))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}
