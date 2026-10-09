package main

import "testing"

func TestPortIsFixedAndZeroIsRefused(t *testing.T) {
	if DefaultPort != 8200 {
		t.Errorf("default port %d, want 8200", DefaultPort)
	}
	for _, p := range []int{0, -1, 65536} {
		if checkPort(p) == nil {
			t.Errorf("-port %d accepted", p)
		}
	}
	for _, p := range []int{1, DefaultPort, 18200, 65535} {
		if err := checkPort(p); err != nil {
			t.Errorf("-port %d refused: %v", p, err)
		}
	}
}
