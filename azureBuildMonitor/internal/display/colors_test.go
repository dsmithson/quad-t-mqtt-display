package display

import (
	"testing"

	"github.com/dsmithson/quad-t-mqtt-display/azureBuildMonitor/internal/store"
)

func TestLedForColorScheme(t *testing.T) {
	cases := []struct {
		status              store.Status
		wantR, wantG, wantB int
		wantMode            string
		wantBlinkDuration   float64
	}{
		{store.StatusPending, 0, 0, 255, "solid", 0},
		{store.StatusRunning, 0, 0, 255, "blinkThruBlack", runningBlinkDurationSeconds},
		{store.StatusSucceeded, 0, 255, 0, "solid", 0},
		{store.StatusFailed, 255, 0, 0, "solid", 0},
		{store.StatusCancelled, 128, 128, 128, "solid", 0},
		{store.StatusUnknown, 0, 0, 0, "solid", 0},
	}
	for _, c := range cases {
		r, g, b, mode, blink := ledFor(c.status)
		if r != c.wantR || g != c.wantG || b != c.wantB || mode != c.wantMode || blink != c.wantBlinkDuration {
			t.Errorf("ledFor(%q) = (%d,%d,%d,%q,%v), want (%d,%d,%d,%q,%v)",
				c.status, r, g, b, mode, blink, c.wantR, c.wantG, c.wantB, c.wantMode, c.wantBlinkDuration)
		}
	}
}

func TestDimClampsToValidRange(t *testing.T) {
	cases := []struct {
		v          int
		multiplier float64
		want       int
	}{
		{255, 0.25, 63},
		{255, 1.0, 255},
		{255, 0.0, 0},
		{100, -1.0, 0},   // negative multiplier shouldn't go negative
		{100, 10.0, 255}, // absurd multiplier shouldn't overflow past 255
	}
	for _, c := range cases {
		if got := dim(c.v, c.multiplier); got != c.want {
			t.Errorf("dim(%d, %v) = %d, want %d", c.v, c.multiplier, got, c.want)
		}
	}
}
