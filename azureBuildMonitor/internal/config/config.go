package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	// Azure DevOps
	AzureDevOpsURL         string
	AzureDevOpsPAT         string
	AzureDevOpsProjectName string
	BuildDefinitionIDs     []int
	FullRefreshInterval    int // seconds

	// MQTT (shared broker -- same one the Quad-T panel uses)
	MQTTServerHost string
	MQTTServerPort int

	// Topic the Azure DevOps webhook (via N8N) publishes run.statechanged
	// events to.
	BuildEventTopic string

	// Quad-T front panel
	QuadTDeviceName         string
	QuadTDimPixelMultiplier float64

	// The pipeline-name line never scrolls (always left-aligned, static).
	// The status/branch line is always left-aligned too, but whether it
	// scrolls when too wide is an experiment worth toggling without a
	// code change -- static-and-clipped reads fine for short branch
	// names, and avoids motion some find distracting.
	Line2AutoScroll bool

	// How long each individual OLED line-2 message (status+time, branch,
	// ...) shows before switching to the next one. A pipeline's full
	// "slot" is however many messages it has times this.
	StatusLineDurationSeconds int

	// Raw JSON for display.ParseStatusColors -- see that function for the
	// shape. Empty string means "use display.DefaultStatusColors".
	StatusColorsJSON string
}

func Load() (*Config, error) {
	cfg := &Config{
		AzureDevOpsURL:         getenv("AZURE_DEVOPS_URL", ""),
		AzureDevOpsPAT:         getenv("AZURE_DEVOPS_PAT", ""),
		AzureDevOpsProjectName: getenv("AZURE_DEVOPS_PROJECT_NAME", ""),
		FullRefreshInterval:    getenvInt("FULL_REFRESH_INTERVAL_SECONDS", 3600),

		MQTTServerHost: getenv("MQTT_SERVER_URL", "mqtt-mosquitto.mqtt.svc"),
		MQTTServerPort: getenvInt("MQTT_SERVER_PORT", 1883),

		BuildEventTopic: getenv("BUILD_EVENT_TOPIC", "azureDevOps/builds/buildEvent"),

		QuadTDeviceName:         getenv("QUADT_DEVICE_NAME", "quadTFrontPanel01"),
		QuadTDimPixelMultiplier: getenvFloat("QUADT_DIM_PIXEL_MULTIPLIER", 0.25),
		Line2AutoScroll:         getenvBool("QUADT_LINE2_AUTOSCROLL", false),

		StatusLineDurationSeconds: getenvInt("STATUS_LINE_DURATION_SECONDS", 5),
		StatusColorsJSON:          getenv("STATUS_COLORS_JSON", ""),
	}

	ids, err := getenvIntList("AZURE_DEVOPS_BUILD_DEFINITION_IDS", nil)
	if err != nil {
		return nil, fmt.Errorf("AZURE_DEVOPS_BUILD_DEFINITION_IDS: %w", err)
	}
	cfg.BuildDefinitionIDs = ids

	if cfg.AzureDevOpsURL == "" {
		return nil, fmt.Errorf("AZURE_DEVOPS_URL is required")
	}
	if cfg.AzureDevOpsPAT == "" {
		return nil, fmt.Errorf("AZURE_DEVOPS_PAT is required")
	}
	if cfg.AzureDevOpsProjectName == "" {
		return nil, fmt.Errorf("AZURE_DEVOPS_PROJECT_NAME is required")
	}
	if len(cfg.BuildDefinitionIDs) == 0 {
		return nil, fmt.Errorf("AZURE_DEVOPS_BUILD_DEFINITION_IDS is required (comma-separated definition IDs)")
	}
	if len(cfg.BuildDefinitionIDs) > 7 {
		return nil, fmt.Errorf("AZURE_DEVOPS_BUILD_DEFINITION_IDS has %d entries, but the Quad-T panel only has 7 hexagon pixels to represent them", len(cfg.BuildDefinitionIDs))
	}

	return cfg, nil
}

func (c *Config) String() string {
	statusColors := "default"
	if c.StatusColorsJSON != "" {
		statusColors = "custom"
	}
	return fmt.Sprintf(
		"adoURL=%s project=%s definitionIDs=%v refreshInterval=%ds mqtt=%s:%d buildEventTopic=%s quadT=%s dimMultiplier=%.2f lineDuration=%ds statusColors=%s line2AutoScroll=%v",
		c.AzureDevOpsURL, c.AzureDevOpsProjectName, c.BuildDefinitionIDs, c.FullRefreshInterval,
		c.MQTTServerHost, c.MQTTServerPort, c.BuildEventTopic,
		c.QuadTDeviceName, c.QuadTDimPixelMultiplier, c.StatusLineDurationSeconds, statusColors, c.Line2AutoScroll,
	)
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func getenvInt(k string, def int) int {
	if v := os.Getenv(k); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func getenvFloat(k string, def float64) float64 {
	if v := os.Getenv(k); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return def
}

func getenvBool(k string, def bool) bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(k)))
	switch v {
	case "1", "true", "t", "yes", "y", "on":
		return true
	case "0", "false", "f", "no", "n", "off":
		return false
	}
	return def
}

func getenvIntList(k string, def []int) ([]int, error) {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return def, nil
	}
	parts := strings.Split(v, ",")
	ids := make([]int, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil, fmt.Errorf("invalid integer %q: %w", p, err)
		}
		ids = append(ids, n)
	}
	return ids, nil
}
