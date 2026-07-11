# Quad-T Front Panel Lights

Salvaged Christie Quad-T video encoder front panel — a 128x32 Adafruit
SSD1306 SPI OLED (product #661) plus a NeoPixel PCB (4 exposed pixels,
possibly 8 on-board) extended with a 7-pixel NeoPixel Jewel — driven by a
Raspberry Pi Pico W over MQTT.

Send a JSON payload to `quadTFrontPanel/<device-id>/set` to control the
display text/pixels and NeoPixel colors from any MQTT-capable service.

## Repo layout

```
firmware/         MicroPython code that runs on the Pico W
  main.py           entry point
  config.example.py template — copy to config.py and fill in real values
  config.py          gitignored, your real WiFi/broker/pin settings
  display.py         OLED draw modes, invert timer, autoscroll
  leds.py             NeoPixel chain control
  mqtt_client.py       MQTT connect/reconnect + topic handling
  lib/                vendored ssd1306.py and umqtt/simple.py drivers
tester/            Python CLI to publish test payloads from a dev machine
  quadt_tester.py
  requirements.txt
```

## Physical wiring (Pico W)

**OLED (Adafruit #661, SPI0):**

| OLED pin | Pico W physical pin | GPIO |
|---|---|---|
| GND | GND (pin 38) | — |
| Vin | 3V3(OUT) (pin 36) | — |
| CLK | pin 9 | GPIO6 (SPI0 SCK) |
| DATA | pin 10 | GPIO7 (SPI0 MOSI) |
| CS | pin 7 | GPIO5 |
| D/C | pin 6 | GPIO4 |
| RESET | pin 5 | GPIO3 |

**NeoPixels:**

| Signal | Pico W physical pin | GPIO |
|---|---|---|
| Data in (first pixel) | pin 20 | GPIO15 |
| 5V | external 5V supply, common ground with Pico | — |
| GND | common ground with Pico GND | — |

Daisy-chain the existing strip's data-out/5V/GND straight into the Jewel's
data-in/5V/GND when it arrives. Power the LEDs from an external 5V supply
rather than the Pico's USB VBUS — up to ~11 pixels at full white can draw
~660mA, more than is comfortable to push through the Pico alongside WiFi.
Tie all grounds together. A ~1000uF capacitor across 5V/GND at the first
pixel and a ~300-500 ohm resistor in series on the data line are cheap
insurance against the 3.3V-logic-on-5V-LED marginal voltage issue.

Pin numbers live in `firmware/config.py` if you need to wire it differently.

## Flashing the Pico W

1. Flash the latest Raspberry Pi Pico W MicroPython UF2 from
   https://micropython.org/download/RPI_PICO_W/.
2. Copy `firmware/config.example.py` to `firmware/config.py` and fill in
   your WiFi SSID/password and (optionally) adjust `MQTT_BROKER`,
   `DEVICE_ID`, pin numbers, and `NUM_PIXELS`.
3. Push the firmware to the device with [`mpremote`](https://docs.micropython.org/en/latest/reference/mpremote.html)
   (or Thonny):

   ```
   pip install mpremote
   mpremote connect auto fs cp -r firmware/lib :lib
   mpremote connect auto fs cp firmware/config.py firmware/display.py firmware/leds.py firmware/mqtt_client.py firmware/main.py :
   mpremote connect auto run firmware/main.py
   ```

   Once you're happy with it, copy `main.py` to the device as `main.py` (it
   already is) so it runs automatically on boot/reset.
4. Watch the REPL (`mpremote connect auto`) during first boot to confirm
   WiFi and MQTT connect cleanly.

## MQTT topics

- `quadTFrontPanel/<device-id>/set` — publish here to command the device.
- `quadTFrontPanel/<device-id>/state` — device publishes its last-applied
  state here, retained.
- `quadTFrontPanel/<device-id>/status` — `"online"`/`"offline"`, retained,
  `"offline"` is set as an MQTT Last Will so you can tell if the panel
  drops off WiFi.

`<device-id>` defaults to `quadTFrontPanel01` (see `config.py`).

## JSON payload

```json
{
  "display": {
    "drawMode": "pixels | text1Line | text2Line",
    "invertRate": 5,
    "pixelData": "<1024 hex chars = 512 bytes, 128x32 1bpp framebuffer>",
    "textLine1": "My Custom Text",
    "textLine2": "Other custom text",
    "textAutoScroll": false
  },
  "leds": [
    { "r": 255, "g": 0, "b": 0 },
    { "r": 0, "g": 255, "b": 0 }
  ]
}
```

Both `display` and `leds` are optional per message. `leds` is applied by
array position (index 0 = first pixel in the chain); pixels beyond the
array length are left unchanged, entries beyond the configured
`NUM_PIXELS` are ignored.

## Tester CLI

```
cd tester
pip install -r requirements.txt

python quadt_tester.py text --line1 "Hello world"
python quadt_tester.py text --line1 "A long line that needs to scroll across the screen" --autoscroll
python quadt_tester.py leds --color 255,0,0
python quadt_tester.py leds --colors 255,0,0 0,255,0 0,0,255
python quadt_tester.py pixels --file frame.bin
python quadt_tester.py raw --file payload.json
python quadt_tester.py watch
```

All commands default to broker `192.168.86.11:1883` and device id
`quadTFrontPanel01` — override with `--broker`/`--port`/`--device-id`.

`watch` subscribes to the state/status topics and prints updates, handy
for confirming what the device actually applied.
