// Command azureBuildMonitor polls Azure DevOps for the status of a
// configured set of build pipelines and drives a Quad-T MQTT front panel
// to display it: 7 "hexagon" pixels (one per pipeline, dimmed except the
// currently-highlighted one), a 4-pixel strip showing the highlighted
// pipeline's recent build history, and an OLED cycling through pipeline
// name / status / branch.
package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"
	"time"

	"github.com/dsmithson/quad-t-mqtt-display/azureBuildMonitor/internal/ado"
	"github.com/dsmithson/quad-t-mqtt-display/azureBuildMonitor/internal/config"
	"github.com/dsmithson/quad-t-mqtt-display/azureBuildMonitor/internal/display"
	"github.com/dsmithson/quad-t-mqtt-display/azureBuildMonitor/internal/mqttclient"
	"github.com/dsmithson/quad-t-mqtt-display/azureBuildMonitor/internal/poller"
	"github.com/dsmithson/quad-t-mqtt-display/azureBuildMonitor/internal/quadt"
	"github.com/dsmithson/quad-t-mqtt-display/azureBuildMonitor/internal/store"
	"github.com/dsmithson/quad-t-mqtt-display/azureBuildMonitor/internal/webhook"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	log.Printf("config: %s", cfg)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	adoClient, err := ado.NewClient(ctx, cfg.AzureDevOpsURL, cfg.AzureDevOpsPAT, cfg.AzureDevOpsProjectName)
	if err != nil {
		log.Fatalf("azure devops client: %v", err)
	}

	mqttClient, err := mqttclient.Connect(cfg.MQTTServerHost, cfg.MQTTServerPort, "azureBuildMonitor")
	if err != nil {
		log.Fatalf("mqtt connect: %v", err)
	}
	defer mqttClient.Close()

	st := store.New(cfg.BuildDefinitionIDs)
	quadtClient := quadt.NewClient(mqttClient, cfg.QuadTDeviceName)

	p := poller.New(adoClient, st, cfg.BuildDefinitionIDs, time.Duration(cfg.FullRefreshInterval)*time.Second)
	p.RefreshOnce(ctx) // blocking: populate the store before anything reads it
	go p.Run(ctx)

	handler := webhook.NewHandler(adoClient, st)
	if err := handler.Subscribe(mqttClient, cfg.BuildEventTopic); err != nil {
		log.Fatalf("subscribing to %s: %v", cfg.BuildEventTopic, err)
	}
	log.Printf("listening for build events on %s", cfg.BuildEventTopic)

	cycler := display.NewCycler(cfg, st, quadtClient)
	cycler.Run(ctx) // blocks until ctx is cancelled
}
