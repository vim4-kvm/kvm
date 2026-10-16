// Package vim4media tracks process-local ownership of the board's shared LUN.
package vim4media

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var ErrForeign = errors.New("Another interface or the management panel mounted this media. Safely eject it on the host computer, then eject it in the management panel.")

type Snapshot struct {
	File     string `json:"file"`
	ReadOnly bool   `json:"ro"`
	CDROM    bool   `json:"cdrom"`
}

// Controller never adopts a pre-existing mount, even inside JetKVM's directory.
// Apply must compare expected with the LUN under the shared helper lock before
// making any USB changes. Only successful operations establish ownership.
type Controller struct {
	LUN   string
	Apply func(file string, cdrom bool, expected Snapshot) error
	mu    sync.Mutex
	owned Snapshot
}

func (c *Controller) read() (Snapshot, error) {
	var media Snapshot
	for _, attr := range []string{"file", "ro", "cdrom"} {
		data, err := os.ReadFile(filepath.Join(c.LUN, attr))
		if err != nil {
			c.owned = Snapshot{}
			return media, fmt.Errorf("read virtual media %s: %w", attr, err)
		}
		value := strings.TrimSpace(string(data))
		if attr == "file" {
			media.File = value
			continue
		}
		if value != "0" && value != "1" {
			c.owned = Snapshot{}
			return media, fmt.Errorf("invalid virtual media %s", attr)
		}
		if attr == "ro" {
			media.ReadOnly = value == "1"
		} else {
			media.CDROM = value == "1"
		}
	}
	if media.File == "" || media != c.owned {
		c.owned = Snapshot{}
	}
	return media, nil
}

// State reads the current LUN and forgets ownership after an observed change.
func (c *Controller) State() (media Snapshot, owned bool, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	media, err = c.read()
	return media, err == nil && media.File != "" && media == c.owned, err
}

// Change refuses both replacement and ejection of media mounted elsewhere.
func (c *Controller) Change(file string, cdrom bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	media, err := c.read()
	if err != nil {
		return err
	}
	if media.File != "" && media != c.owned {
		return ErrForeign
	}
	if file == "" && media.File == "" {
		return nil
	}
	if err := c.Apply(file, cdrom, media); err != nil {
		return err
	}
	c.owned = Snapshot{}
	if file != "" {
		c.owned = Snapshot{File: file, ReadOnly: true, CDROM: cdrom}
	}
	return nil
}
