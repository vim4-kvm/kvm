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
	return vim4Media.Change(imagePath, vim4MediaCDROM)
}

func applyVIM4Media(imagePath string, cdrom bool, expected vim4media.Snapshot) (vim4media.Snapshot, error) {
	var err error
	var mounted vim4media.Snapshot
	setUSBRecoveryTimer(time.Now())
	if imagePath == "" {
		err = vim4Helper(30*time.Second, "media-eject", map[string]any{"expected": expected}, nil)
	} else {
		if filepath.Dir(imagePath) == imagesFolder {
			imagePath = filepath.Base(imagePath)
		}
		err = vim4Helper(30*time.Second, "media-mount",
			map[string]any{"file": imagePath, "cdrom": cdrom, "expected": expected}, &mounted)
	}
	// The helper re-enumerates the gadget even when it restores the previous
	// functions after a failure, so the HID endpoints need fresh handles.
	gadget.ResetHIDFiles()
	if !tryReopenKeyboard("virtual-media", false) {
		usbLogger.Warn().Msg("keyboard HID file not ready after virtual media change")
	}
	return mounted, err
}

type vim4CatalogEntry struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Source string `json:"source"`
	Size   int64  `json:"size"`
	CDROM  bool   `json:"cdrom"`
}

var getVIM4Catalog = func() ([]vim4CatalogEntry, error) {
	var entries []vim4CatalogEntry
	err := vim4Helper(10*time.Second, "catalog", nil, &entries)
	return entries, err
}

func vim4CatalogFiles() ([]StorageFile, error) {
	entries, err := getVIM4Catalog()
	if err != nil {
		return nil, err
	}
	files := make([]StorageFile, 0)
	for _, entry := range entries {
		if entry.Source == "jetkvm" {
			continue
		}
		files = append(files, StorageFile{Filename: entry.ID, Size: entry.Size, Source: entry.Source, ReadOnly: true})
	}
	return files, nil
}

// Called with virtualMediaStateMutex held, after the live ownership check.
func mountVIM4CatalogImage(identifier string, mode VirtualMediaMode) error {
	entries, err := getVIM4Catalog()
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.ID != identifier {
			continue
		}
		if entry.CDROM {
			mode = CDROM
		}
		if entry.Source == "data" {
			mode = Disk
		}
		if err := setMassStorageMode(mode == CDROM); err != nil {
			return err
		}
		if err := setMassStorageImage(entry.ID); err != nil {
			return err
		}
		currentVirtualMediaState, err = getVIM4VirtualMediaState()
		return err
	}
	return fmt.Errorf("image is not in the media catalog")
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
