package vim4hid

import "testing"

func TestAbsoluteWireFormat(t *testing.T) {
	got, err := Absolute(0x1234, 32767, 17, -1)
	if err != nil || got != [6]byte{17, 0x34, 0x12, 0xff, 0x7f, 0xff} {
		t.Fatalf("absolute report = %x, %v", got, err)
	}
	for _, point := range [][2]int{{-1, 0}, {0, -1}, {32768, 0}, {0, 32768}} {
		if _, err := Absolute(point[0], point[1], 0, 0); err == nil {
			t.Fatalf("accepted out-of-range coordinates %v", point)
		}
	}
	if _, err := Absolute(0, 0, 32, 0); err == nil {
		t.Fatal("accepted an undeclared button")
	}
	if _, err := Absolute(0, 0, 0, -128); err == nil {
		t.Fatal("accepted wheel below logical minimum")
	}
}

func TestRelativeWireFormat(t *testing.T) {
	got, err := Relative(-127, 127, 31, -1)
	if err != nil || got != [4]byte{31, 0x81, 0x7f, 0xff} {
		t.Fatalf("relative report = %x, %v", got, err)
	}
	for _, motion := range [][3]int8{{-128, 0, 0}, {0, -128, 0}, {0, 0, -128}} {
		if _, err := Relative(motion[0], motion[1], 0, motion[2]); err == nil {
			t.Fatalf("accepted out-of-range motion %v", motion)
		}
	}
	if _, err := Relative(0, 0, 32, 0); err == nil {
		t.Fatal("accepted an undeclared button")
	}
}
