// Package mqttclient is a thin wrapper around paho.mqtt.golang, just
// enough to publish and subscribe with plain []byte payloads -- the
// quadt and webhook packages own their own message shapes.
package mqttclient

import (
	"fmt"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

type Client struct {
	inner mqtt.Client
}

func Connect(host string, port int, clientID string) (*Client, error) {
	opts := mqtt.NewClientOptions()
	opts.AddBroker(fmt.Sprintf("tcp://%s:%d", host, port))
	opts.SetClientID(clientID)
	opts.SetAutoReconnect(true)
	opts.SetConnectRetry(true)
	opts.SetConnectRetryInterval(5 * time.Second)

	c := mqtt.NewClient(opts)
	token := c.Connect()
	if !token.WaitTimeout(15 * time.Second) {
		return nil, fmt.Errorf("timed out connecting to mqtt broker %s:%d", host, port)
	}
	if err := token.Error(); err != nil {
		return nil, fmt.Errorf("connecting to mqtt broker %s:%d: %w", host, port, err)
	}
	return &Client{inner: c}, nil
}

func (c *Client) Publish(topic string, payload []byte) error {
	token := c.inner.Publish(topic, 0, false, payload)
	token.Wait()
	return token.Error()
}

func (c *Client) Subscribe(topic string, handler func(payload []byte)) error {
	token := c.inner.Subscribe(topic, 0, func(_ mqtt.Client, msg mqtt.Message) {
		handler(msg.Payload())
	})
	token.Wait()
	return token.Error()
}

func (c *Client) Close() {
	c.inner.Disconnect(250)
}
