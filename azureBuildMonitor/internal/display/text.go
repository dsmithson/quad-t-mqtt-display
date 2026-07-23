package display

import (
	"fmt"
	"time"

	"github.com/dsmithson/quad-t-mqtt-display/azureBuildMonitor/internal/store"
)

// humanizeDuration renders "24 Mins" / "3 Hrs" / "2 Days" style text --
// coarse and short on purpose, this has to fit a 16-character OLED line
// alongside a status word.
func humanizeDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Minute:
		return "<1 Min"
	case d < time.Hour:
		mins := int(d / time.Minute)
		return fmt.Sprintf("%d Min%s", mins, plural(mins))
	case d < 24*time.Hour:
		hrs := int(d / time.Hour)
		return fmt.Sprintf("%d Hr%s", hrs, plural(hrs))
	default:
		days := int(d / (24 * time.Hour))
		return fmt.Sprintf("%d Day%s", days, plural(days))
	}
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// statusLine renders the "status + time in status" OLED line, e.g.
// "Building: 24 Mins" (ongoing, no "ago") or "Succeeded: 24 Mins ago".
func statusLine(b store.BuildInfo, now time.Time) string {
	elapsed := now.Sub(b.StatusTime())
	switch b.Status {
	case store.StatusPending:
		return fmt.Sprintf("Queued: %s", humanizeDuration(elapsed))
	case store.StatusRunning:
		return fmt.Sprintf("Building: %s", humanizeDuration(elapsed))
	case store.StatusSucceeded:
		return fmt.Sprintf("Succeeded: %s ago", humanizeDuration(elapsed))
	case store.StatusFailed:
		return fmt.Sprintf("Failed: %s ago", humanizeDuration(elapsed))
	case store.StatusCancelled:
		return fmt.Sprintf("Cancelled: %s ago", humanizeDuration(elapsed))
	default:
		return "No build data"
	}
}

// branchLine renders the "Branch: ..." OLED line, trimming the common
// "refs/heads/" prefix Azure DevOps puts on SourceBranch.
func branchLine(b store.BuildInfo) string {
	branch := b.SourceBranch
	const prefix = "refs/heads/"
	if len(branch) > len(prefix) && branch[:len(prefix)] == prefix {
		branch = branch[len(prefix):]
	}
	if branch == "" {
		return "Branch: unknown"
	}
	return fmt.Sprintf("Branch: %s", branch)
}

// lineMessages returns the ordered set of OLED line-2 messages to cycle
// through for a pipeline's latest build -- currently status+time and
// branch, but callers just cycle through however many come back, so
// adding a third message later (e.g. commit message, requested-by) is
// just appending to this slice.
func lineMessages(p store.Pipeline, now time.Time) []string {
	latest, ok := p.Latest()
	if !ok {
		return []string{"No build data"}
	}
	return []string{statusLine(latest, now), branchLine(latest)}
}
