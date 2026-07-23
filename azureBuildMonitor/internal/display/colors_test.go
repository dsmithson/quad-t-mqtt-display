package display

import (
	"testing"

	"github.com/dsmithson/quad-t-mqtt-display/azureBuildMonitor/internal/store"
)

func TestLedForDefaultColorScheme(t *testing.T) {
	cases := []struct {
		status              store.Status
		group               Group
		wantR, wantG, wantB int
		wantMode            string
		wantBlinkDuration   float64
	}{
		{store.StatusPending, GroupHexagon, 0, 0, 255, "solid", 0},
		{store.StatusRunning, GroupHexagon, 0, 0, 255, "blinkThruBlack", defaultRunningBlinkDurationSeconds},
		{store.StatusSucceeded, GroupStrip, 0, 255, 0, "solid", 0},
		{store.StatusFailed, GroupStrip, 255, 0, 0, "solid", 0},
		{store.StatusCancelled, GroupHexagon, 128, 128, 128, "solid", 0},
		{store.StatusUnknown, GroupHexagon, 0, 0, 0, "solid", 0},
	}
	for _, c := range cases {
		r, g, b, mode, blink := ledFor(DefaultStatusColors, c.status, c.group)
		if r != c.wantR || g != c.wantG || b != c.wantB || mode != c.wantMode || blink != c.wantBlinkDuration {
			t.Errorf("ledFor(default, %q, %q) = (%d,%d,%d,%q,%v), want (%d,%d,%d,%q,%v)",
				c.status, c.group, r, g, b, mode, blink, c.wantR, c.wantG, c.wantB, c.wantMode, c.wantBlinkDuration)
		}
	}
}

func TestLedForHexagonAndStripCanDiffer(t *testing.T) {
	styles := StatusColors{
		store.StatusSucceeded: {
			Hexagon: LedColor{R: 0, G: 80, B: 0},  // dimmer, behind a diffuser
			Strip:   LedColor{R: 0, G: 255, B: 0}, // full, no diffuser
		},
	}
	hr, hg, hb, _, _ := ledFor(styles, store.StatusSucceeded, GroupHexagon)
	sr, sg, sb, _, _ := ledFor(styles, store.StatusSucceeded, GroupStrip)
	if hg != 80 {
		t.Errorf("hexagon green = %d, want 80", hg)
	}
	if sg != 255 {
		t.Errorf("strip green = %d, want 255", sg)
	}
	if hr != 0 || hb != 0 || sr != 0 || sb != 0 {
		t.Errorf("expected only green channel set, got hexagon=(%d,%d,%d) strip=(%d,%d,%d)", hr, hg, hb, sr, sg, sb)
	}
}

func TestParseStatusColorsEmptyReturnsDefault(t *testing.T) {
	styles, err := ParseStatusColors("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(styles) != len(DefaultStatusColors) {
		t.Fatalf("expected default status colors, got %d entries", len(styles))
	}
}

func TestParseStatusColorsInvalidJSONErrors(t *testing.T) {
	_, err := ParseStatusColors("{not valid json")
	if err == nil {
		t.Fatal("expected an error for invalid JSON")
	}
}

func TestParseStatusColorsPartialOverrideKeepsOtherDefaults(t *testing.T) {
	styles, err := ParseStatusColors(`{"failed": {"hexagon": {"r": 90, "g": 0, "b": 0}, "strip": {"r": 200, "g": 0, "b": 0}}}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if styles[store.StatusFailed].Hexagon.R != 90 {
		t.Errorf("failed.hexagon.r = %d, want 90 (the override)", styles[store.StatusFailed].Hexagon.R)
	}
	if styles[store.StatusSucceeded] != DefaultStatusColors[store.StatusSucceeded] {
		t.Errorf("succeeded should be untouched by a failed-only override, got %+v", styles[store.StatusSucceeded])
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
