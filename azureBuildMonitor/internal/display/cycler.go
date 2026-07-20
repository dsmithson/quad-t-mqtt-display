// Package display owns the Quad-T panel's visual state: which pipeline is
// currently "selected" (full-brightness hexagon pixel + OLED text), the
// 4-pixel strip showing that pipeline's recent build history, and the
// dimmed status of the other 6 pipelines. It reads store.Store on a
// timer and publishes a fresh quadt.Command each tick -- it never talks
// to Azure DevOps directly.
package display

import (
	"context"
	"log"
	"time"

	"github.com/dsmithson/quad-t-mqtt-display/azureBuildMonitor/internal/config"
	"github.com/dsmithson/quad-t-mqtt-display/azureBuildMonitor/internal/quadt"
	"github.com/dsmithson/quad-t-mqtt-display/azureBuildMonitor/internal/store"
)

type Cycler struct {
	cfg   *config.Config
	store *store.Store
	quadt *quadt.Client

	selectedIndex int
	line2Toggle   int
}

func NewCycler(cfg *config.Config, st *store.Store, qc *quadt.Client) *Cycler {
	return &Cycler{cfg: cfg, store: st, quadt: qc}
}

// Run publishes an initial brightness + display state, then advances the
// cycle (toggle the OLED's second line, or move to the next pipeline)
// every PipelineMessageDuration seconds until ctx is cancelled.
func (c *Cycler) Run(ctx context.Context) {
	if err := c.publishBrightness(); err != nil {
		log.Printf("display: publishing initial brightness: %v", err)
	}

	interval := time.Duration(c.cfg.PipelineMessageDuration) * time.Second
	if interval <= 0 {
		interval = 5 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	c.publish()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.advance()
			c.publish()
		}
	}
}

func (c *Cycler) publishBrightness() error {
	return c.quadt.Publish(quadt.Command{
		PixelBrightness: quadt.Float64Ptr(c.cfg.QuadTBrightness),
	})
}

func (c *Cycler) advance() {
	pipelines := c.store.Pipelines()
	if len(pipelines) == 0 {
		return
	}
	c.line2Toggle++
	if c.line2Toggle >= 2 {
		c.line2Toggle = 0
		c.selectedIndex = (c.selectedIndex + 1) % len(pipelines)
	}
}

func (c *Cycler) publish() {
	pipelines := c.store.Pipelines()
	if len(pipelines) == 0 {
		return
	}
	if c.selectedIndex >= len(pipelines) {
		c.selectedIndex = 0
	}
	selected := pipelines[c.selectedIndex]
	now := time.Now()

	leds := make([]quadt.Led, 0, 11)
	leds = append(leds, stripLEDs(selected)...)
	leds = append(leds, hexagonLEDs(pipelines, c.selectedIndex, c.cfg.QuadTDimPixelMultiplier)...)

	cmd := quadt.Command{
		Display: buildDisplay(selected, c.line2Toggle, now),
		Leds:    leds,
	}
	if err := c.quadt.Publish(cmd); err != nil {
		log.Printf("display: publish failed: %v", err)
	}
}

// stripLEDs renders logical LEDs 0-3 -- the selected pipeline's up-to-4
// most recent builds, newest first. Slots beyond the available history
// are off.
func stripLEDs(p store.Pipeline) []quadt.Led {
	leds := make([]quadt.Led, 4)
	for i := 0; i < 4; i++ {
		if i < len(p.Builds) {
			r, g, b, mode, blink := ledFor(p.Builds[i].Status)
			leds[i] = quadt.Led{R: r, G: g, B: b, DisplayMode: mode, BlinkDuration: blink}
		} else {
			leds[i] = quadt.Led{R: 0, G: 0, B: 0, DisplayMode: "solid"}
		}
	}
	return leds
}

// hexagonLEDs renders logical LEDs 4-10 -- one per monitored pipeline, in
// configured order, each always showing its own true status color/mode.
// Every pipeline except the currently-selected one has its color scaled
// down by dimMultiplier so the selected one stands out.
func hexagonLEDs(pipelines []store.Pipeline, selectedIndex int, dimMultiplier float64) []quadt.Led {
	leds := make([]quadt.Led, len(pipelines))
	for i, p := range pipelines {
		status := store.StatusUnknown
		if latest, ok := p.Latest(); ok {
			status = latest.Status
		}
		r, g, b, mode, blink := ledFor(status)
		if i != selectedIndex {
			r, g, b = dim(r, dimMultiplier), dim(g, dimMultiplier), dim(b, dimMultiplier)
		}
		leds[i] = quadt.Led{R: r, G: g, B: b, DisplayMode: mode, BlinkDuration: blink}
	}
	return leds
}

func buildDisplay(p store.Pipeline, line2Toggle int, now time.Time) *quadt.Display {
	line1 := p.Name
	if line1 == "" {
		line1 = "(unknown)"
	}

	var line2 string
	latest, ok := p.Latest()
	if !ok {
		line2 = "No build data"
	} else if line2Toggle == 0 {
		line2 = statusLine(latest, now)
	} else {
		line2 = branchLine(latest)
	}

	return &quadt.Display{
		DrawMode:       "text2Line",
		TextLine1:      line1,
		TextLine2:      line2,
		TextAutoScroll: quadt.BoolPtr(true),
	}
}
