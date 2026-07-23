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
	cfg    *config.Config
	store  *store.Store
	quadt  *quadt.Client
	styles StatusColors

	selectedIndex int
	messageIndex  int
}

// NewCycler parses cfg.StatusColorsJSON (falling back to
// DefaultStatusColors and logging a warning if it's invalid -- a config
// typo shouldn't crash the whole service) and returns a ready Cycler.
func NewCycler(cfg *config.Config, st *store.Store, qc *quadt.Client) *Cycler {
	styles, err := ParseStatusColors(cfg.StatusColorsJSON)
	if err != nil {
		log.Printf("display: %v -- falling back to default status colors", err)
		styles = DefaultStatusColors
	}
	return &Cycler{cfg: cfg, store: st, quadt: qc, styles: styles}
}

// Run publishes an initial display state, then advances the cycle
// (the OLED's next line-2 message, or move to the next pipeline once
// every message for the current one has shown) every
// StatusLineDurationSeconds until ctx is cancelled.
func (c *Cycler) Run(ctx context.Context) {
	interval := time.Duration(c.cfg.StatusLineDurationSeconds) * time.Second
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

func (c *Cycler) advance() {
	pipelines := c.store.Pipelines()
	if len(pipelines) == 0 {
		return
	}
	if c.selectedIndex >= len(pipelines) {
		c.selectedIndex = 0
	}
	messages := lineMessages(pipelines[c.selectedIndex], time.Now())

	c.messageIndex++
	if c.messageIndex >= len(messages) {
		c.messageIndex = 0
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
	leds = append(leds, stripLEDs(selected, c.styles)...)
	leds = append(leds, hexagonLEDs(pipelines, c.selectedIndex, c.cfg.QuadTDimPixelMultiplier, c.styles)...)

	cmd := quadt.Command{
		Display: buildDisplay(selected, c.messageIndex, now),
		Leds:    leds,
	}
	if err := c.quadt.Publish(cmd); err != nil {
		log.Printf("display: publish failed: %v", err)
	}
}

// stripLEDs renders logical LEDs 0-3 -- the selected pipeline's up-to-4
// most recent builds, newest first. Slots beyond the available history
// are off.
func stripLEDs(p store.Pipeline, styles StatusColors) []quadt.Led {
	leds := make([]quadt.Led, 4)
	for i := 0; i < 4; i++ {
		if i < len(p.Builds) {
			r, g, b, mode, blink := ledFor(styles, p.Builds[i].Status, GroupStrip)
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
func hexagonLEDs(pipelines []store.Pipeline, selectedIndex int, dimMultiplier float64, styles StatusColors) []quadt.Led {
	leds := make([]quadt.Led, len(pipelines))
	for i, p := range pipelines {
		status := store.StatusUnknown
		if latest, ok := p.Latest(); ok {
			status = latest.Status
		}
		r, g, b, mode, blink := ledFor(styles, status, GroupHexagon)
		if i != selectedIndex {
			r, g, b = dim(r, dimMultiplier), dim(g, dimMultiplier), dim(b, dimMultiplier)
		}
		leds[i] = quadt.Led{R: r, G: g, B: b, DisplayMode: mode, BlinkDuration: blink}
	}
	return leds
}

func buildDisplay(p store.Pipeline, messageIndex int, now time.Time) *quadt.Display {
	line1 := p.Name
	if line1 == "" {
		line1 = "(unknown)"
	}

	messages := lineMessages(p, now)
	if messageIndex >= len(messages) {
		messageIndex = 0
	}

	return &quadt.Display{
		DrawMode:       "text2Line",
		TextLine1:      line1,
		TextLine2:      messages[messageIndex],
		TextAutoScroll: quadt.BoolPtr(true),
	}
}
