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

BOOT_FLASH_COLOR = {"r": 0, "g": 255, "b": 0}
BOOT_FLASH_COUNT = 3
BOOT_FLASH_ON_MS = 150
BOOT_FLASH_OFF_MS = 150

# Gives the external 5V rail (and its bulk cap) time to settle before the
# first NeoPixel write -- writing while the rail is still ramping up right
# after power-on is a plausible cause of a boot flash that only "takes"
# sometimes.
BOOT_SETTLE_MS = 300

# One-time red/green/blue cycle once MQTT is up, a quick "still waiting for
# a real command" proof-of-life without being a perpetual, annoying blink.
IDLE_PATTERN_COLORS = (
    {"r": 255, "g": 0, "b": 0},
    {"r": 0, "g": 255, "b": 0},
    {"r": 0, "g": 0, "b": 255},
    {"r": 0, "g": 0, "b": 0},
)
IDLE_PATTERN_STEP_MS = 350


def boot_flash(leds):
    on = [BOOT_FLASH_COLOR] * leds.num_pixels
    off = [{"r": 0, "g": 0, "b": 0}] * leds.num_pixels
    for _ in range(BOOT_FLASH_COUNT):
        leds.apply(on)
        time.sleep_ms(BOOT_FLASH_ON_MS)
        leds.apply(off)
        time.sleep_ms(BOOT_FLASH_OFF_MS)


def connect_wifi(display=None):
    wlan = network.WLAN(network.STA_IF)
    wlan.active(True)
    if wlan.isconnected():
        return wlan
    if display is not None:
        display.apply(
            {"drawMode": "text2Line", "textLine1": "Connecting to", "textLine2": config.WIFI_SSID}
        )
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
    oled = make_oled()
    display = DisplayController(oled)

    time.sleep_ms(BOOT_SETTLE_MS)
    leds = LedController(machine.Pin(config.NEOPIXEL_PIN), config.NUM_PIXELS)

    # Visible proof-of-life before anything touches the network: a quick
    # green flash on the LEDs, then live status text on the OLED.
    boot_flash(leds)
    connect_wifi(display)
    display.apply({"drawMode": "text1Line", "textLine1": "MQTT connecting"})

    waiting_for_first_command = True
    idle_pattern_index = 0
    idle_pattern_last_ms = time.ticks_ms()

    def on_command(payload):
        nonlocal waiting_for_first_command
        waiting_for_first_command = False
        if "display" in payload:
            display.apply(payload["display"])
        if "leds" in payload:
            leds.apply(
                payload["leds"],
                transition_duration=payload.get("transitionDuration", 0),
                transition_type=payload.get("transitionType", "immediate"),
            )
        mqtt.publish_state({"display": display.as_state(), "leds": leds.as_state()})

    mqtt = DeviceMqtt(config, on_command)
    mqtt.connect()
    display.apply({"drawMode": "text2Line", "textLine1": "MQTT Connected", "textLine2": "Waiting..."})
    mqtt.publish_state({"display": display.as_state(), "leds": leds.as_state()})

    while True:
        try:
            if not network.WLAN(network.STA_IF).isconnected():
                raise OSError("wifi dropped")
            mqtt.check_msg()
            display.tick(time.ticks_ms())
            if leds.tick(time.ticks_ms()) and not waiting_for_first_command:
                mqtt.publish_state({"display": display.as_state(), "leds": leds.as_state()})
            if waiting_for_first_command and idle_pattern_index < len(IDLE_PATTERN_COLORS):
                now = time.ticks_ms()
                if time.ticks_diff(now, idle_pattern_last_ms) >= IDLE_PATTERN_STEP_MS:
                    leds.apply([IDLE_PATTERN_COLORS[idle_pattern_index]] * leds.num_pixels)
                    idle_pattern_index += 1
                    idle_pattern_last_ms = now
            time.sleep_ms(TICK_MS)
        except OSError as exc:
            print("main: connection error, reconnecting:", exc)
            mqtt.close()
            display.apply({"drawMode": "text1Line", "textLine1": "Reconnecting..."})
            time.sleep_ms(MQTT_RETRY_MS)
            connect_wifi(display)
            mqtt.connect()
            display.apply(
                {"drawMode": "text2Line", "textLine1": "MQTT Connected", "textLine2": "Waiting..."}
            )
            mqtt.publish_state({"display": display.as_state(), "leds": leds.as_state()})


if __name__ == "__main__":
    main()
