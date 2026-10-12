// Package vim4hid encodes reports for the system-managed VIM4 USB gadget.
// Device creation and USB-C power negotiation belong to the board services.
package vim4hid

import "fmt"

// Absolute encodes five buttons, unsigned 15-bit coordinates and vertical wheel.
// The installed descriptor has no Report ID: byte zero contains the buttons.
func Absolute(x, y int, buttons uint8, wheel int8) ([6]byte, error) {
	if x < 0 || x > 32767 || y < 0 || y > 32767 {
		return [6]byte{}, fmt.Errorf("absolute coordinates outside 0..32767: %d,%d", x, y)
	}
	if buttons > 31 || wheel == -128 {
		return [6]byte{}, fmt.Errorf("unsupported mouse buttons or wheel value")
	}
	return [6]byte{buttons, byte(x), byte(x >> 8), byte(y), byte(y >> 8), byte(wheel)}, nil
}

// Relative encodes motion and vertical wheel in the descriptor's -127..127 range.
// Horizontal scrolling is not present in this gadget's descriptor.
func Relative(dx, dy int8, buttons uint8, wheel int8) ([4]byte, error) {
	if dx == -128 || dy == -128 || wheel == -128 || buttons > 31 {
		return [4]byte{}, fmt.Errorf("unsupported relative motion, buttons or wheel value")
	}
	return [4]byte{buttons, byte(dx), byte(dy), byte(wheel)}, nil
}
