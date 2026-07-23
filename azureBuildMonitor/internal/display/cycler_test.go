package display

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/dsmithson/quad-t-mqtt-display/azureBuildMonitor/internal/config"
	"github.com/dsmithson/quad-t-mqtt-display/azureBuildMonitor/internal/quadt"
	"github.com/dsmithson/quad-t-mqtt-display/azureBuildMonitor/internal/store"
)

// fakePublisher records every payload published, so tests can inspect
// exactly what would have gone out over MQTT without a real broker.
type fakePublisher struct {
	published []quadt.Command
}

func (f *fakePublisher) Publish(topic string, payload []byte) error {
	var cmd quadt.Command
	if err := json.Unmarshal(payload, &cmd); err != nil {
		return err
	}
	f.published = append(f.published, cmd)
	return nil
}

func (f *fakePublisher) last() quadt.Command {
	return f.published[len(f.published)-1]
}

func testConfig() *config.Config {
	return &config.Config{
		BuildDefinitionIDs:        []int{82, 85, 44, 97, 52, 6, 1},
		QuadTDimPixelMultiplier:   0.25,
		StatusLineDurationSeconds: 5,
	}
}

func newTestCycler(cfg *config.Config, st *store.Store) (*Cycler, *fakePublisher) {
	fp := &fakePublisher{}
	qc := quadt.NewClient(fp, "quadTFrontPanel01")
	return NewCycler(cfg, st, qc), fp
}

func TestBuildDisplayLine1NeverScrollsBothLinesLeftAligned(t *testing.T) {
	p := store.Pipeline{Name: "Spyder-S - Complete"}
	d := buildDisplay(p, 0, time.Now(), true) // line2AutoScroll=true
	if d.TextLine1Align != "left" || d.TextLine2Align != "left" {
		t.Errorf("expected both lines left-aligned, got line1=%q line2=%q", d.TextLine1Align, d.TextLine2Align)
	}
	if d.TextLine1AutoScroll == nil || *d.TextLine1AutoScroll != false {
		t.Errorf("line1 should never scroll regardless of config, got %v", d.TextLine1AutoScroll)
	}
	if d.TextLine2AutoScroll == nil || *d.TextLine2AutoScroll != true {
		t.Errorf("line2 should follow the line2AutoScroll argument, got %v", d.TextLine2AutoScroll)
	}
}

func TestBuildDisplayLine2AutoScrollFollowsConfigToggle(t *testing.T) {
	p := store.Pipeline{Name: "Spyder-S - Complete"}
	d := buildDisplay(p, 0, time.Now(), false)
	if d.TextLine2AutoScroll == nil || *d.TextLine2AutoScroll != false {
		t.Errorf("expected line2AutoScroll=false when config disables it, got %v", d.TextLine2AutoScroll)
	}
}

func TestPublishAlwaysSendsElevenLEDs(t *testing.T) {
	cfg := testConfig()
	st := store.New(cfg.BuildDefinitionIDs)
	c, fp := newTestCycler(cfg, st)

	c.publish()

	if len(fp.published) != 1 {
		t.Fatalf("expected 1 publish, got %d", len(fp.published))
	}
	leds := fp.last().Leds
	if len(leds) != 11 {
		t.Fatalf("expected 11 LEDs (4 strip + 7 hexagon), got %d", len(leds))
	}
}

func TestPublishNeverSendsPixelBrightness(t *testing.T) {
	// Brightness/intensity is now baked into the configured per-status,
	// per-group colors -- the app shouldn't rely on the device's global
	// pixelBrightness passthrough at all.
	cfg := testConfig()
	st := store.New(cfg.BuildDefinitionIDs)
	c, fp := newTestCycler(cfg, st)

	c.publish()

	if fp.last().PixelBrightness != nil {
		t.Errorf("expected PixelBrightness to be omitted, got %v", *fp.last().PixelBrightness)
	}
}

func TestSelectedHexagonPixelIsFullBrightnessOthersAreDimmed(t *testing.T) {
	cfg := testConfig()
	st := store.New(cfg.BuildDefinitionIDs)
	now := time.Now()
	// All 7 pipelines succeeded (green, 0,255,0) so dimming is easy to spot.
	for _, id := range cfg.BuildDefinitionIDs {
		st.ReplaceBuilds(id, "p", []store.BuildInfo{{ID: id, Status: store.StatusSucceeded, FinishTime: now}})
	}
	c, fp := newTestCycler(cfg, st)

	c.publish() // selectedIndex starts at 0

	hexagon := fp.last().Leds[4:11]
	if hexagon[0].G != 255 {
		t.Errorf("selected pixel (index 0) should be full brightness green, got G=%d", hexagon[0].G)
	}
	for i := 1; i < len(hexagon); i++ {
		wantDimmed := dim(255, cfg.QuadTDimPixelMultiplier)
		if hexagon[i].G != wantDimmed {
			t.Errorf("non-selected pixel (index %d) should be dimmed to G=%d, got G=%d", i, wantDimmed, hexagon[i].G)
		}
	}
}

func TestAdvanceCyclesThroughAllLineMessagesBeforeMovingToNextPipeline(t *testing.T) {
	cfg := testConfig()
	st := store.New(cfg.BuildDefinitionIDs)
	now := time.Now()
	for _, id := range cfg.BuildDefinitionIDs {
		st.ReplaceBuilds(id, "p", []store.BuildInfo{{ID: id, Status: store.StatusSucceeded, FinishTime: now, SourceBranch: "refs/heads/main"}})
	}
	c, _ := newTestCycler(cfg, st)

	// A build with status+time and branch has exactly 2 messages.
	if c.selectedIndex != 0 || c.messageIndex != 0 {
		t.Fatalf("expected fresh cycler to start at pipeline 0, message 0")
	}
	c.advance()
	if c.selectedIndex != 0 || c.messageIndex != 1 {
		t.Errorf("after 1 advance: expected same pipeline, message 1; got index=%d message=%d", c.selectedIndex, c.messageIndex)
	}
	c.advance()
	if c.selectedIndex != 1 || c.messageIndex != 0 {
		t.Errorf("after 2 advances: expected next pipeline, message reset; got index=%d message=%d", c.selectedIndex, c.messageIndex)
	}
}

func TestAdvanceHandlesSingleMessagePipelines(t *testing.T) {
	// A pipeline with no build history yet only has 1 message
	// ("No build data"), so it should move to the next pipeline every
	// single advance, not every 2.
	cfg := testConfig()
	st := store.New(cfg.BuildDefinitionIDs) // no builds recorded for anyone
	c, _ := newTestCycler(cfg, st)

	c.advance()
	if c.selectedIndex != 1 || c.messageIndex != 0 {
		t.Errorf("expected immediate advance to next pipeline for a 1-message pipeline; got index=%d message=%d", c.selectedIndex, c.messageIndex)
	}
}

func TestAdvanceWrapsAroundToFirstPipeline(t *testing.T) {
	cfg := testConfig()
	st := store.New(cfg.BuildDefinitionIDs)
	c, _ := newTestCycler(cfg, st)
	c.selectedIndex = len(cfg.BuildDefinitionIDs) - 1 // last pipeline

	c.advance() // no build history -> 1 message -> advances immediately

	if c.selectedIndex != 0 {
		t.Errorf("expected wraparound to pipeline 0, got %d", c.selectedIndex)
	}
}

func TestStripLEDsPadsMissingHistoryWithOff(t *testing.T) {
	p := store.Pipeline{Builds: []store.BuildInfo{{Status: store.StatusSucceeded}}} // only 1 of 4
	leds := stripLEDs(p, DefaultStatusColors)
	if len(leds) != 4 {
		t.Fatalf("expected 4 strip LEDs, got %d", len(leds))
	}
	if leds[0].G != 255 {
		t.Errorf("strip[0] should reflect the one known build (green), got %+v", leds[0])
	}
	for i := 1; i < 4; i++ {
		if leds[i].R != 0 || leds[i].G != 0 || leds[i].B != 0 {
			t.Errorf("strip[%d] should be off (no history), got %+v", i, leds[i])
		}
	}
}

func TestNewCyclerFallsBackToDefaultsOnInvalidStatusColorsJSON(t *testing.T) {
	cfg := testConfig()
	cfg.StatusColorsJSON = "{not valid"
	st := store.New(cfg.BuildDefinitionIDs)
	c, _ := newTestCycler(cfg, st)

	if len(c.styles) != len(DefaultStatusColors) {
		t.Errorf("expected fallback to default status colors on invalid JSON, got %d entries", len(c.styles))
	}
}
