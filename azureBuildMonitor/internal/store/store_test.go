package store

import (
	"testing"
	"time"
)

func TestNormalizeStatus(t *testing.T) {
	cases := []struct {
		state, result string
		want          Status
	}{
		{"notStarted", "", StatusPending},
		{"postponed", "", StatusPending},
		{"inProgress", "", StatusRunning},
		{"cancelling", "", StatusRunning},
		{"completed", "succeeded", StatusSucceeded},
		{"completed", "failed", StatusFailed},
		{"completed", "partiallySucceeded", StatusFailed},
		{"completed", "canceled", StatusCancelled},
		{"completed", "none", StatusUnknown},
		{"bogus", "bogus", StatusUnknown},
	}
	for _, c := range cases {
		if got := NormalizeStatus(c.state, c.result); got != c.want {
			t.Errorf("NormalizeStatus(%q, %q) = %q, want %q", c.state, c.result, got, c.want)
		}
	}
}

func TestReplaceBuildsIgnoresUntrackedDefinition(t *testing.T) {
	s := New([]int{1, 2})
	s.ReplaceBuilds(999, "not tracked", []BuildInfo{{ID: 1}})
	for _, p := range s.Pipelines() {
		if len(p.Builds) != 0 {
			t.Fatalf("expected no builds recorded for untracked definition, got %+v", p)
		}
	}
}

func TestReplaceBuildsSortsNewestFirstAndCapsAtFour(t *testing.T) {
	s := New([]int{1})
	now := time.Now()
	builds := []BuildInfo{
		{ID: 1, QueueTime: now.Add(-5 * time.Hour)}, // oldest, should be dropped
		{ID: 2, QueueTime: now.Add(-1 * time.Hour)}, // newest
		{ID: 3, QueueTime: now.Add(-3 * time.Hour)},
		{ID: 4, QueueTime: now.Add(-2 * time.Hour)},
		{ID: 5, QueueTime: now.Add(-4 * time.Hour)},
	}
	s.ReplaceBuilds(1, "Pipeline One", builds)

	pipelines := s.Pipelines()
	if len(pipelines) != 1 {
		t.Fatalf("expected 1 pipeline, got %d", len(pipelines))
	}
	p := pipelines[0]
	if p.Name != "Pipeline One" {
		t.Errorf("name = %q, want %q", p.Name, "Pipeline One")
	}
	if len(p.Builds) != 4 {
		t.Fatalf("expected 4 builds (capped), got %d", len(p.Builds))
	}
	wantOrder := []int{2, 4, 3, 5}
	for i, id := range wantOrder {
		if p.Builds[i].ID != id {
			t.Errorf("Builds[%d].ID = %d, want %d (newest-first order)", i, p.Builds[i].ID, id)
		}
	}
}

func TestPatchBuildInsertsAndUpdatesInPlace(t *testing.T) {
	s := New([]int{1})
	now := time.Now()

	s.PatchBuild(1, "Pipeline One", BuildInfo{ID: 100, Status: StatusRunning, QueueTime: now.Add(-1 * time.Minute)})
	latest, ok := s.Pipelines()[0].Latest()
	if !ok || latest.Status != StatusRunning {
		t.Fatalf("expected build 100 running, got %+v (ok=%v)", latest, ok)
	}

	// A later event for the SAME build ID should update in place, not
	// duplicate.
	s.PatchBuild(1, "Pipeline One", BuildInfo{ID: 100, Status: StatusSucceeded, QueueTime: now.Add(-1 * time.Minute)})
	p := s.Pipelines()[0]
	if len(p.Builds) != 1 {
		t.Fatalf("expected build 100 to update in place, not duplicate; got %d builds", len(p.Builds))
	}
	if p.Builds[0].Status != StatusSucceeded {
		t.Errorf("status = %q, want %q", p.Builds[0].Status, StatusSucceeded)
	}
}

func TestPipelinesPreservesConfiguredOrder(t *testing.T) {
	s := New([]int{82, 1, 52})
	got := s.Pipelines()
	want := []int{82, 1, 52}
	if len(got) != len(want) {
		t.Fatalf("expected %d pipelines, got %d", len(want), len(got))
	}
	for i, id := range want {
		if got[i].DefinitionID != id {
			t.Errorf("Pipelines()[%d].DefinitionID = %d, want %d", i, got[i].DefinitionID, id)
		}
	}
}

func TestStatusTimePicksTheRightTimestamp(t *testing.T) {
	now := time.Now()
	running := BuildInfo{Status: StatusRunning, QueueTime: now.Add(-2 * time.Hour), StartTime: now.Add(-1 * time.Hour)}
	if !running.StatusTime().Equal(running.StartTime) {
		t.Errorf("running build should use StartTime, got %v want %v", running.StatusTime(), running.StartTime)
	}

	succeeded := BuildInfo{Status: StatusSucceeded, QueueTime: now.Add(-3 * time.Hour), FinishTime: now.Add(-30 * time.Minute)}
	if !succeeded.StatusTime().Equal(succeeded.FinishTime) {
		t.Errorf("succeeded build should use FinishTime, got %v want %v", succeeded.StatusTime(), succeeded.FinishTime)
	}

	pending := BuildInfo{Status: StatusPending, QueueTime: now.Add(-10 * time.Minute)}
	if !pending.StatusTime().Equal(pending.QueueTime) {
		t.Errorf("pending build should use QueueTime, got %v want %v", pending.StatusTime(), pending.QueueTime)
	}
}
