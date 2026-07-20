package display

import "github.com/dsmithson/quad-t-mqtt-display/azureBuildMonitor/internal/store"

const runningBlinkDurationSeconds = 1.0 // each leg -- 2s full fade cycle

// ledFor returns the r,g,b,displayMode,blinkDuration for a pipeline
// status, per the color scheme: Pending=blue solid, Running=blue
// blinkThruBlack, Succeeded=green solid, Failed=red solid,
// Cancelled=gray solid. No data yet (a pipeline we haven't successfully
// polled/heard about) is off.
func ledFor(status store.Status) (r, g, b int, mode string, blinkDuration float64) {
	switch status {
	case store.StatusPending:
		return 0, 0, 255, "solid", 0
	case store.StatusRunning:
		return 0, 0, 255, "blinkThruBlack", runningBlinkDurationSeconds
	case store.StatusSucceeded:
		return 0, 255, 0, "solid", 0
	case store.StatusFailed:
		return 255, 0, 0, "solid", 0
	case store.StatusCancelled:
		return 128, 128, 128, "solid", 0
	default:
		return 0, 0, 0, "solid", 0
	}
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
