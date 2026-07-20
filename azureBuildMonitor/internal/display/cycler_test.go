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
		BuildDefinitionIDs:      []int{82, 85, 44, 97, 52, 6, 1},
		QuadTBrightness:         0.8,
		QuadTDimPixelMultiplier: 0.25,
		PipelineMessageDuration: 5,
	}
}

func TestPublishAlwaysSendsElevenLEDs(t *testing.T) {
	cfg := testConfig()
	st := store.New(cfg.BuildDefinitionIDs)
	fp := &fakePublisher{}
	qc := quadt.NewClient(fp, "quadTFrontPanel01")
	c := NewCycler(cfg, st, qc)

	c.publish()

	if len(fp.published) != 1 {
		t.Fatalf("expected 1 publish, got %d", len(fp.published))
	}
	leds := fp.last().Leds
	if len(leds) != 11 {
		t.Fatalf("expected 11 LEDs (4 strip + 7 hexagon), got %d", len(leds))
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
	fp := &fakePublisher{}
	qc := quadt.NewClient(fp, "quadTFrontPanel01")
	c := NewCycler(cfg, st, qc)

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

func TestAdvanceTogglesLine2TwiceBeforeMovingToNextPipeline(t *testing.T) {
	cfg := testConfig()
	st := store.New(cfg.BuildDefinitionIDs)
	fp := &fakePublisher{}
	qc := quadt.NewClient(fp, "quadTFrontPanel01")
	c := NewCycler(cfg, st, qc)

	if c.selectedIndex != 0 || c.line2Toggle != 0 {
		t.Fatalf("expected fresh cycler to start at pipeline 0, line2Toggle 0")
	}
	c.advance()
	if c.selectedIndex != 0 || c.line2Toggle != 1 {
		t.Errorf("after 1 advance: expected same pipeline, line2Toggle=1; got index=%d toggle=%d", c.selectedIndex, c.line2Toggle)
	}
	c.advance()
	if c.selectedIndex != 1 || c.line2Toggle != 0 {
		t.Errorf("after 2 advances: expected next pipeline, toggle reset; got index=%d toggle=%d", c.selectedIndex, c.line2Toggle)
	}
}

func TestAdvanceWrapsAroundToFirstPipeline(t *testing.T) {
	cfg := testConfig()
	st := store.New(cfg.BuildDefinitionIDs)
	fp := &fakePublisher{}
	qc := quadt.NewClient(fp, "quadTFrontPanel01")
	c := NewCycler(cfg, st, qc)
	c.selectedIndex = len(cfg.BuildDefinitionIDs) - 1 // last pipeline

	c.advance() // toggles line2 first
	c.advance() // now should wrap

	if c.selectedIndex != 0 {
		t.Errorf("expected wraparound to pipeline 0, got %d", c.selectedIndex)
	}
}

func TestStripLEDsPadsMissingHistoryWithOff(t *testing.T) {
	p := store.Pipeline{Builds: []store.BuildInfo{{Status: store.StatusSucceeded}}} // only 1 of 4
	leds := stripLEDs(p)
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
