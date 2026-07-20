// Package webhook listens for Azure DevOps run.statechanged events
// (relayed through N8N onto MQTT) and patches the store in near
// real-time between the hourly full polls.
//
// The webhook payload's own fields aren't trustworthy enough to use
// directly: it carries no branch name, and its "pipelineId" is actually
// the build/run ID, not the definition ID our config is keyed on (a real
// definition ID never showed up there in testing). So every event
// triggers a single "get build by ID" REST call, which reliably resolves
// the true definition ID, branch, and everything else needed.
package webhook

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/dsmithson/quad-t-mqtt-display/azureBuildMonitor/internal/ado"
	"github.com/dsmithson/quad-t-mqtt-display/azureBuildMonitor/internal/mqttclient"
	"github.com/dsmithson/quad-t-mqtt-display/azureBuildMonitor/internal/store"
)

const enrichTimeout = 15 * time.Second

// Event is the N8N-translated run.statechanged payload. Only RunID is
// actually relied on -- everything else is fetched fresh via REST, see
// the package doc above.
type Event struct {
	RunID int `json:"pipelineId"`
}

type Handler struct {
	ado   *ado.Client
	store *store.Store
}

func NewHandler(adoClient *ado.Client, st *store.Store) *Handler {
	return &Handler{ado: adoClient, store: st}
}

// Subscribe registers this handler on the given MQTT client/topic. Each
// message is handled in its own goroutine so a slow ADO REST call never
// blocks delivery of the next event.
func (h *Handler) Subscribe(mqttClient *mqttclient.Client, topic string) error {
	return mqttClient.Subscribe(topic, func(payload []byte) {
		go h.handle(payload)
	})
}

func (h *Handler) handle(payload []byte) {
	var evt Event
	if err := json.Unmarshal(payload, &evt); err != nil {
		log.Printf("webhook: ignoring unparseable event: %v", err)
		return
	}
	if evt.RunID == 0 {
		log.Printf("webhook: event missing pipelineId (run ID), ignoring: %s", string(payload))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), enrichTimeout)
	defer cancel()

	info, definitionID, definitionName, err := h.ado.GetBuild(ctx, evt.RunID)
	if err != nil {
		log.Printf("webhook: enriching run %d: %v", evt.RunID, err)
		return
	}
	if definitionID == 0 {
		log.Printf("webhook: run %d has no resolvable definition ID, ignoring", evt.RunID)
		return
	}

	h.store.PatchBuild(definitionID, definitionName, info)
	log.Printf("webhook: patched definition %d (%s) from run %d: %s", definitionID, definitionName, evt.RunID, info.Status)
}
