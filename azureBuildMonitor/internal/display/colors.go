package display

import (
	"encoding/json"
	"fmt"

	"github.com/dsmithson/quad-t-mqtt-display/azureBuildMonitor/internal/store"
)

const defaultRunningBlinkDurationSeconds = 1.0 // each leg -- 2s full fade cycle

// Group distinguishes the two physically different pixel types on the
// panel: the 7 hexagon pixels sit behind a diffuser and the 4 side strip
// pixels don't, so they need independently tunable colors/brightness --
// there's deliberately no single shared "brightness" knob here, see
// StatusStyle.
type Group string

const (
	GroupHexagon Group = "hexagon"
	GroupStrip   Group = "strip"
)

type LedColor struct {
	R int `json:"r"`
	G int `json:"g"`
	B int `json:"b"`
}

// StatusStyle is the full visual treatment for one build status: separate
// colors for the hexagon (diffused) and strip (undiffused) pixel groups,
// so brightness/intensity differences between the two can be dialed in
// just by picking different RGB values per group -- no separate
// brightness multiplier needed. BlinkDurationSeconds only applies to
// Running (the only status that blinks); it's ignored for the others.
type StatusStyle struct {
	Hexagon              LedColor `json:"hexagon"`
	Strip                LedColor `json:"strip"`
	BlinkDurationSeconds float64  `json:"blinkDurationSeconds,omitempty"`
}

type StatusColors map[store.Status]StatusStyle

// DefaultStatusColors matches the original hardcoded scheme (same value
// for both pixel groups), used whenever the STATUS_COLORS_JSON config is
// unset or fails to parse -- a config typo should degrade to "looks like
// it always did," not a blank panel.
var DefaultStatusColors = StatusColors{
	store.StatusPending:   {Hexagon: LedColor{0, 0, 255}, Strip: LedColor{0, 0, 255}},
	store.StatusRunning:   {Hexagon: LedColor{0, 0, 255}, Strip: LedColor{0, 0, 255}, BlinkDurationSeconds: defaultRunningBlinkDurationSeconds},
	store.StatusSucceeded: {Hexagon: LedColor{0, 255, 0}, Strip: LedColor{0, 255, 0}},
	store.StatusFailed:    {Hexagon: LedColor{255, 0, 0}, Strip: LedColor{255, 0, 0}},
	store.StatusCancelled: {Hexagon: LedColor{128, 128, 128}, Strip: LedColor{128, 128, 128}},
}

// ParseStatusColors decodes the STATUS_COLORS_JSON config value. An empty
// string returns DefaultStatusColors unchanged. Any status missing from
// the parsed JSON falls back to its default entry individually, so a
// config that only overrides e.g. "failed" doesn't need to repeat the
// other four.
func ParseStatusColors(jsonStr string) (StatusColors, error) {
	if jsonStr == "" {
		return DefaultStatusColors, nil
	}
	var parsed StatusColors
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		return nil, fmt.Errorf("parsing STATUS_COLORS_JSON: %w", err)
	}
	merged := make(StatusColors, len(DefaultStatusColors))
	for status, style := range DefaultStatusColors {
		merged[status] = style
	}
	for status, style := range parsed {
		merged[status] = style
	}
	return merged, nil
}

// ledFor returns the r,g,b,displayMode,blinkDuration for a pipeline
// status in the given pixel group. Running is the only status that
// blinks (blinkThruBlack); everything else is solid. A status with no
// configured style (including "no data yet") is off.
func ledFor(styles StatusColors, status store.Status, group Group) (r, g, b int, mode string, blinkDuration float64) {
	style, ok := styles[status]
	if !ok {
		return 0, 0, 0, "solid", 0
	}
	color := style.Hexagon
	if group == GroupStrip {
		color = style.Strip
	}
	if status == store.StatusRunning {
		blink := style.BlinkDurationSeconds
		if blink <= 0 {
			blink = defaultRunningBlinkDurationSeconds
		}
		return color.R, color.G, color.B, "blinkThruBlack", blink
	}
	return color.R, color.G, color.B, "solid", 0
}

func dim(v int, multiplier float64) int {
	d := int(float64(v) * multiplier)
	if d < 0 {
		return 0
	}
	if d > 255 {
		return 255
	}
	return d
}
