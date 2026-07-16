import json
import time

from umqtt.simple import MQTTClient

KEEPALIVE_S = 60


class DeviceMqtt:
    """Thin wrapper around umqtt.simple.MQTTClient for this device's topic
    tree:
        quadTFrontPanel/<device_id>/set     -- subscribe, incoming commands
        quadTFrontPanel/<device_id>/state   -- publish, retained
        quadTFrontPanel/<device_id>/status  -- publish, retained, LWT
    """

    def __init__(self, config, on_command):
        self.config = config
        self.on_command = on_command

        base = "quadTFrontPanel/%s" % config.DEVICE_ID
        self.set_topic = base + "/set"
        self.state_topic = base + "/state"
        self.status_topic = base + "/status"

        self.client = None
        self._last_ping_ms = 0

    def connect(self):
        client = MQTTClient(
            client_id=self.config.DEVICE_ID,
            server=self.config.MQTT_BROKER,
            port=self.config.MQTT_PORT,
            keepalive=KEEPALIVE_S,
        )
        client.set_last_will(self.status_topic, b"offline", retain=True, qos=0)
        client.set_callback(self._on_message)
        client.connect()
        client.subscribe(self.set_topic)
        client.publish(self.status_topic, b"online", retain=True)
        self.client = client
        self._last_ping_ms = time.ticks_ms()

    def _on_message(self, topic, msg):
        try:
            payload = json.loads(msg)
        except ValueError:
            print("mqtt_client: ignoring non-JSON payload on", topic)
            return
        self.on_command(payload)

    def check_msg(self):
        self.client.check_msg()
        now = time.ticks_ms()
        if time.ticks_diff(now, self._last_ping_ms) >= (KEEPALIVE_S // 2) * 1000:
            self.client.ping()
            self._last_ping_ms = now

    def publish_state(self, state):
        self.client.publish(self.state_topic, json.dumps(state), retain=True)

    def close(self):
        if self.client is not None:
            try:
                self.client.publish(self.status_topic, b"offline", retain=True)
                self.client.disconnect()
            except OSError:
                pass
            self.client = None
