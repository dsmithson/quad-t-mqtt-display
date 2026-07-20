// Package store holds the in-memory build status for every monitored
// pipeline. It's updated by both the hourly full poll and the real-time
// webhook listener, and read by the display cycler -- safe for concurrent
// use from all three.
package store

import (
	"sort"
	"sync"
	"time"
)

type Status string

const (
	StatusUnknown   Status = ""
	StatusPending   Status = "pending"
	StatusRunning   Status = "running"
	StatusSucceeded Status = "succeeded"
	StatusFailed    Status = "failed"
	StatusCancelled Status = "cancelled"
)

// NormalizeStatus maps Azure DevOps' own state/result vocabulary onto our
// five-color model. partiallySucceeded is treated as Failed -- it means
// something needs attention, same as a hard failure.
func NormalizeStatus(state, result string) Status {
	switch state {
	case "notStarted", "postponed":
		return StatusPending
	case "inProgress", "cancelling":
		return StatusRunning
	case "completed":
		switch result {
		case "succeeded":
			return StatusSucceeded
		case "failed", "partiallySucceeded":
			return StatusFailed
		case "canceled":
			return StatusCancelled
		}
	}
	return StatusUnknown
}

// BuildInfo is one run of a pipeline.
type BuildInfo struct {
	ID           int
	Number       string
	Status       Status
	SourceBranch string
	QueueTime    time.Time
	StartTime    time.Time
	FinishTime   time.Time
}

// StatusTime returns the timestamp that best represents "how long has this
// build been in its current status" -- start time while running, finish
// time once done, queue time if it hasn't started yet.
func (b BuildInfo) StatusTime() time.Time {
	switch b.Status {
	case StatusRunning:
		if !b.StartTime.IsZero() {
			return b.StartTime
		}
		return b.QueueTime
	case StatusSucceeded, StatusFailed, StatusCancelled:
		if !b.FinishTime.IsZero() {
			return b.FinishTime
		}
	}
	return b.QueueTime
}

// Pipeline is one monitored build definition and its recent history
// (newest first, capped at 4).
type Pipeline struct {
	DefinitionID int
	Name         string
	Builds       []BuildInfo
}

// Latest returns the most recent build, if any have been seen yet.
func (p Pipeline) Latest() (BuildInfo, bool) {
	if len(p.Builds) == 0 {
		return BuildInfo{}, false
	}
	return p.Builds[0], true
}

const maxBuildsPerPipeline = 4

type Store struct {
	mu        sync.RWMutex
	order     []int // definition IDs in configured display order
	pipelines map[int]*Pipeline
}

func New(definitionIDs []int) *Store {
	pipelines := make(map[int]*Pipeline, len(definitionIDs))
	order := make([]int, len(definitionIDs))
	copy(order, definitionIDs)
	for _, id := range definitionIDs {
		pipelines[id] = &Pipeline{DefinitionID: id}
	}
	return &Store{order: order, pipelines: pipelines}
}

// ReplaceBuilds overwrites a pipeline's build history wholesale -- used by
// the full poller, which always fetches the authoritative top-N.
func (s *Store) ReplaceBuilds(definitionID int, name string, builds []BuildInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.pipelines[definitionID]
	if !ok {
		return // not one we're configured to track
	}
	if name != "" {
		p.Name = name
	}
	sortNewestFirst(builds)
	if len(builds) > maxBuildsPerPipeline {
		builds = builds[:maxBuildsPerPipeline]
	}
	p.Builds = builds
}

// PatchBuild inserts or updates a single build (by ID) in a pipeline's
// history -- used by the real-time webhook listener between full polls.
func (s *Store) PatchBuild(definitionID int, name string, b BuildInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.pipelines[definitionID]
	if !ok {
		return // a build event for a pipeline we're not configured to track
	}
	if name != "" {
		p.Name = name
	}
	replaced := false
	for i := range p.Builds {
		if p.Builds[i].ID == b.ID {
			p.Builds[i] = b
			replaced = true
			break
		}
	}
	if !replaced {
		p.Builds = append(p.Builds, b)
	}
	sortNewestFirst(p.Builds)
	if len(p.Builds) > maxBuildsPerPipeline {
		p.Builds = p.Builds[:maxBuildsPerPipeline]
	}
}

// Pipelines returns a snapshot of every tracked pipeline, in configured
// display order.
func (s *Store) Pipelines() []Pipeline {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]Pipeline, 0, len(s.order))
	for _, id := range s.order {
		result = append(result, *s.pipelines[id])
	}
	return result
}

func sortNewestFirst(builds []BuildInfo) {
	sort.Slice(builds, func(i, j int) bool {
		return builds[i].QueueTime.After(builds[j].QueueTime)
	})
}
