package kvm

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jetkvm/kvm/internal/platform"
)

// The board gadget owns this function and links it only while media is mounted.
const vim4LunPath = "/sys/kernel/config/usb_gadget/kvm/functions/mass_storage.disk0/lun.0"

// vim4MediaCDROM is the mode for the next mount. JetKVM selects the mode
// before choosing the image; on VIM4 both are applied in one re-enumeration.
var vim4MediaCDROM = true

func setMassStorageImage(imagePath string) error {
	if !platform.VIM4 {
		return gadget.SetMassStorageImage(imagePath)
	}
	imagePath = strings.TrimSpace(imagePath)
	var err error
	setUSBRecoveryTimer(time.Now())
	if imagePath == "" {
		err = vim4Helper(30*time.Second, "media-eject", nil, nil)
	} else if filepath.Dir(imagePath) != imagesFolder {
		return fmt.Errorf("image must be in %s", imagesFolder)
	} else {
		err = vim4Helper(30*time.Second, "media-mount",
			map[string]any{"file": filepath.Base(imagePath), "cdrom": vim4MediaCDROM}, nil)
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
