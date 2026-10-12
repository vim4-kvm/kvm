package usbgadget

import "time"

// ReleaseMouseButtons preserves the last absolute position when a client leaves.
func (u *UsbGadget) ReleaseMouseButtons() {
	u.absMouseLock.Lock()
	x, y := u.absX, u.absY
	absPressed := u.absButtons != 0
	u.absMouseLock.Unlock()
	u.relMouseLock.Lock()
	relPressed := u.relButtons != 0
	u.relMouseLock.Unlock()
	if absPressed {
		_ = u.AbsMouseReport(x, y, 0)
	}
	if relPressed {
		_ = u.RelMouseReport(0, 0, 0)
	}
}

func (u *UsbGadget) resetUserInputTime() {
	u.lastUserInput = time.Now()
}

func (u *UsbGadget) GetLastUserInputTime() time.Time {
	return u.lastUserInput
}
