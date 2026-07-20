package display

import (
	"testing"
	"time"

	"github.com/dsmithson/quad-t-mqtt-display/azureBuildMonitor/internal/store"
)

func TestHumanizeDuration(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{30 * time.Second, "<1 Min"},
		{1 * time.Minute, "1 Min"},
		{24 * time.Minute, "24 Mins"},
		{1 * time.Hour, "1 Hr"},
		{3 * time.Hour, "3 Hrs"},
		{25 * time.Hour, "1 Day"},
		{50 * time.Hour, "2 Days"},
		{-5 * time.Second, "<1 Min"}, // negative (clock skew) shouldn't crash or go negative
	}
	for _, c := range cases {
		if got := humanizeDuration(c.d); got != c.want {
			t.Errorf("humanizeDuration(%v) = %q, want %q", c.d, got, c.want)
		}
	}
}

func TestStatusLineWording(t *testing.T) {
	now := time.Now()
	cases := []struct {
		status store.Status
		want   string
	}{
		{store.StatusPending, "Queued: 24 Mins"},
		{store.StatusRunning, "Building: 24 Mins"},
		{store.StatusSucceeded, "Succeeded: 24 Mins ago"},
		{store.StatusFailed, "Failed: 24 Mins ago"},
		{store.StatusCancelled, "Cancelled: 24 Mins ago"},
	}
	for _, c := range cases {
		b := store.BuildInfo{Status: c.status, QueueTime: now.Add(-24 * time.Minute)}
		switch c.status {
		case store.StatusRunning:
			b.StartTime = now.Add(-24 * time.Minute)
		case store.StatusSucceeded, store.StatusFailed, store.StatusCancelled:
			b.FinishTime = now.Add(-24 * time.Minute)
		}
		if got := statusLine(b, now); got != c.want {
			t.Errorf("statusLine(%q) = %q, want %q", c.status, got, c.want)
		}
	}
}

func TestBranchLineTrimsRefsHeadsPrefix(t *testing.T) {
	cases := []struct {
		branch string
		want   string
	}{
		{"refs/heads/echo/bbc/dev", "Branch: echo/bbc/dev"},
		{"refs/heads/main", "Branch: main"},
		{"already-trimmed", "Branch: already-trimmed"},
		{"", "Branch: unknown"},
	}
	for _, c := range cases {
		got := branchLine(store.BuildInfo{SourceBranch: c.branch})
		if got != c.want {
			t.Errorf("branchLine(%q) = %q, want %q", c.branch, got, c.want)
		}
	}
}
