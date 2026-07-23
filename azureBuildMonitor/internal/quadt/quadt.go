// Package quadt is the Quad-T front panel's device protocol -- the wire
// format published to quadTFrontPanel/<device>/set, matching
// firmware/README.md in this repo exactly. Keep this in sync with the
// firmware's JSON schema if that ever changes.
package quadt

import (
	"encoding/json"
	"fmt"
)

// Publisher is the one method this package needs from an MQTT client --
// *mqttclient.Client satisfies it. Depending on this instead of the
// concrete type lets tests inject a fake and assert on exactly what would
// have been published, with no real broker involved.
type Publisher interface {
	Publish(topic string, payload []byte) error
}

type Led struct {
	R             int     `json:"r"`
	G             int     `json:"g"`
	B             int     `json:"b"`
	DisplayMode   string  `json:"displayMode,omitempty"`
	BlinkDuration float64 `json:"blinkDuration,omitempty"`
}

type Display struct {
	DrawMode                     string `json:"drawMode,omitempty"`
	TextLine1                    string `json:"textLine1,omitempty"`
	TextLine2                    string `json:"textLine2,omitempty"`
	TextLine1Align               string `json:"textLine1Align,omitempty"`
	TextLine2Align               string `json:"textLine2Align,omitempty"`
	TextLine1AutoScroll          *bool  `json:"textLine1AutoScroll,omitempty"`
	TextLine2AutoScroll          *bool  `json:"textLine2AutoScroll,omitempty"`
	OledBurnInProtectionInterval *int   `json:"oledBurnInProtectionInterval,omitempty"`
	OledBurnInProtectionMode     string `json:"oledBurnInProtectionMode,omitempty"`
}

type Command struct {
	Display            *Display `json:"display,omitempty"`
	Leds               []Led    `json:"leds,omitempty"`
	TransitionDuration *float64 `json:"transitionDuration,omitempty"`
	TransitionType     string   `json:"transitionType,omitempty"`
	PixelBrightness    *float64 `json:"pixelBrightness,omitempty"`
}

type Client struct {
	mqtt     Publisher
	setTopic string
}

func NewClient(mqttClient Publisher, deviceName string) *Client {
	return &Client{
		mqtt:     mqttClient,
		setTopic: fmt.Sprintf("quadTFrontPanel/%s/set", deviceName),
	}
}

func (c *Client) Publish(cmd Command) error {
	body, err := json.Marshal(cmd)
	if err != nil {
		return fmt.Errorf("marshaling quad-t command: %w", err)
	}
	return c.mqtt.Publish(c.setTopic, body)
}

func BoolPtr(b bool) *bool          { return &b }
func IntPtr(i int) *int             { return &i }
func Float64Ptr(f float64) *float64 { return &f }
