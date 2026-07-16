import time

import machine
import network

import config
import ssd1306
from display import DisplayController
from leds import LedController
from mqtt_client import DeviceMqtt

TICK_MS = int(1000 / config.TICK_HZ)
WIFI_CONNECT_TIMEOUT_MS = 15000
WIFI_POLL_MS = 250
MQTT_RETRY_MS = 3000


def connect_wifi():
    wlan = network.WLAN(network.STA_IF)
    wlan.active(True)
    if wlan.isconnected():
        return wlan
    while not wlan.isconnected():
        print("wifi: connecting to", config.WIFI_SSID)
        # A stale/half-joined radio state confuses repeated connect() calls
        # (it can wedge at status=STAT_CONNECTING forever), so start every
        # attempt from a clean disconnect.
        wlan.disconnect()
        time.sleep_ms(300)
        wlan.connect(config.WIFI_SSID, config.WIFI_PASSWORD)
        deadline = time.ticks_add(time.ticks_ms(), WIFI_CONNECT_TIMEOUT_MS)
        while not wlan.isconnected() and time.ticks_diff(deadline, time.ticks_ms()) > 0:
            time.sleep_ms(WIFI_POLL_MS)
        if not wlan.isconnected():
            print("wifi: attempt timed out (status=%s), retrying" % wlan.status())
    print("wifi: connected, ip =", wlan.ifconfig()[0])
    return wlan


def make_oled():
    spi = machine.SPI(
        config.OLED_SPI_ID,
        sck=machine.Pin(config.OLED_SCK_PIN),
        mosi=machine.Pin(config.OLED_MOSI_PIN),
    )
    dc = machine.Pin(config.OLED_DC_PIN)
    res = machine.Pin(config.OLED_RES_PIN)
    cs = machine.Pin(config.OLED_CS_PIN)
    return ssd1306.SSD1306_SPI(config.OLED_WIDTH, config.OLED_HEIGHT, spi, dc, res, cs)


def main():
    connect_wifi()

    oled = make_oled()
    display = DisplayController(oled)
    leds = LedController(machine.Pin(config.NEOPIXEL_PIN), config.NUM_PIXELS)

    def on_command(payload):
        if "display" in payload:
            display.apply(payload["display"])
        if "leds" in payload:
            leds.apply(payload["leds"])
        mqtt.publish_state({"display": display.as_state(), "leds": leds.as_state()})

    mqtt = DeviceMqtt(config, on_command)
    mqtt.connect()
    mqtt.publish_state({"display": display.as_state(), "leds": leds.as_state()})

    while True:
        try:
            if not network.WLAN(network.STA_IF).isconnected():
                raise OSError("wifi dropped")
            mqtt.check_msg()
            display.tick(time.ticks_ms())
            time.sleep_ms(TICK_MS)
        except OSError as exc:
            print("main: connection error, reconnecting:", exc)
            mqtt.close()
            time.sleep_ms(MQTT_RETRY_MS)
            connect_wifi()
            mqtt.connect()
            mqtt.publish_state({"display": display.as_state(), "leds": leds.as_state()})


if __name__ == "__main__":
    main()
