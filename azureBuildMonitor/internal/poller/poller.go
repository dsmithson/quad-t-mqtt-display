// Package poller does the hourly (configurable) full refresh: fetch the
// latest 4 builds for every monitored definition in one Azure DevOps
// call and overwrite the store with the authoritative result. This is
// what keeps the webhook-patched in-memory state from drifting if an
// event is ever missed.
package poller

import (
	"context"
	"log"
	"time"

	"github.com/dsmithson/quad-t-mqtt-display/azureBuildMonitor/internal/ado"
	"github.com/dsmithson/quad-t-mqtt-display/azureBuildMonitor/internal/store"
)

const buildsPerDefinition = 4

type Poller struct {
	ado           *ado.Client
	store         *store.Store
	definitionIDs []int
	interval      time.Duration
}

func New(adoClient *ado.Client, st *store.Store, definitionIDs []int, interval time.Duration) *Poller {
	return &Poller{ado: adoClient, store: st, definitionIDs: definitionIDs, interval: interval}
}

// RefreshOnce does a single blocking full refresh -- call this first, so
// the store has real data before anything else starts reading it.
func (p *Poller) RefreshOnce(ctx context.Context) {
	p.refresh(ctx)
}

// Run refreshes again on every tick until ctx is cancelled. Intended to
// run in its own goroutine, after an initial RefreshOnce.
func (p *Poller) Run(ctx context.Context) {
	if p.interval <= 0 {
		return
	}
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.refresh(ctx)
		}
	}
}

func (p *Poller) refresh(ctx context.Context) {
	byDefinition, names, err := p.ado.LatestBuilds(ctx, p.definitionIDs, buildsPerDefinition)
	if err != nil {
		log.Printf("poller: full refresh failed: %v", err)
		return
	}
	for _, id := range p.definitionIDs {
		p.store.ReplaceBuilds(id, names[id], byDefinition[id])
	}
	log.Printf("poller: refreshed %d pipeline(s)", len(p.definitionIDs))
}
