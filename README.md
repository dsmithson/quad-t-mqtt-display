# Quad-T Front Panel Lights

Salvaged Christie Quad-T video encoder front panel — a 128x32 Adafruit
SSD1306 SPI OLED (product #661) plus a NeoPixel board (8 pixels, 4 visible
through the panel) extended with a 7-pixel NeoPixel Jewel in the center —
driven by a Raspberry Pi Pico W over MQTT.

Send a JSON payload to `quadTFrontPanel/<device-id>/set` to control the
display text/pixels and NeoPixel colors from any MQTT-capable service.

**Status:** hardware bring-up is complete and working end-to-end — OLED,
all 11 externally-addressable NeoPixels, WiFi/MQTT reconnect handling,
color transitions, and OLED burn-in mitigation. Next up: a proper client
app/service to actually drive this thing with real data.

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
| 5V | shared 5V supply rail (see Power below) | — |
| GND | shared GND rail (see Power below) | — |

Daisy-chain the existing strip's data-out/5V/GND straight into the Jewel's
data-in/5V/GND. A ~220uF (or larger) capacitor across 5V/GND at the first
pixel helps absorb power-on inrush and switching noise.

**3.3V-logic-on-5V-LED voltage margin:** the Pico's 3.3V GPIO is below the
WS2812's nominal logic-high threshold at a true 5V rail, which showed up
here as pixels that received data (visible on a scope) but didn't
reliably light. The fix that actually worked: **two silicon diodes in
series in the LED +5V feed** (not the data line), dropping the rail to
roughly 4.2-4.4V and giving the 3.3V signal real margin. A single diode's
~0.6-0.7V drop wasn't quite enough — diode forward voltage drops further
at low currents, and this chain spends most of its time idle/dim. If your
supply runs high (measure it — cheap USB chargers often read 5.5V+ under
light load) you may need the same fix.

Pin numbers live in `firmware/config.py` if you need to wire it differently.

### Power

Run a single external 5V supply and split it (a "T") between the Pico and
the NeoPixel chain, instead of powering the Pico over USB:

| Rail | Pico W physical pin | Notes |
|---|---|---|
| 5V in | VSYS, pin 39 | Onboard regulator accepts 1.8-5.5V here; this is the intended external-power input, *not* the VBUS/USB pin (pin 40) |
| GND | pin 38 (or any GND pin) | Same rail the NeoPixels and OLED GND tie into |

Splitting one supply this way also gives the Pico and the NeoPixels a
shared ground for free — no separate "tie grounds together" step needed.

Size the supply for both loads combined: the Pico W itself draws roughly
100-150mA (with WiFi TX spikes higher), and the NeoPixel chain can hit
~660mA at 11 pixels full white — a 5V/2A supply gives comfortable
headroom for both plus inrush.

It's safe to also have USB plugged in at the same time as VSYS power
(e.g. for `mpremote` during development) — there's a diode between VBUS
and VSYS on the Pico that keeps the two supplies from fighting each
other. Just never feed external 5V into the VBUS pin itself.

## Flashing the Pico W

1. Flash the latest Raspberry Pi Pico W MicroPython UF2 from
   https://micropython.org/download/RPI_PICO_W/.
2. Copy `firmware/config.example.py` to `firmware/config.py` and fill in
   your WiFi SSID/password and (optionally) adjust `MQTT_BROKER`,
   `DEVICE_ID`, pin numbers, `NUM_PIXELS_PHYSICAL`, and `VISIBLE_PIXEL_MAP`.
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
    "oledBurnInProtectionInterval": 60,
    "oledBurnInProtectionMode": "invertDisplay | bounce | off",
    "pixelData": "<1024 hex chars = 512 bytes, 128x32 1bpp framebuffer>",
    "textLine1": "My Custom Text",
    "textLine2": "Other custom text",
    "textLine1Align": "left | center",
    "textLine2Align": "left | center",
    "textLine1AutoScroll": false,
    "textLine2AutoScroll": false
  },
  "leds": [
    { "r": 255, "g": 0, "b": 0 },
    { "r": 0, "g": 255, "b": 0, "displayMode": "blinkThruBlack", "blinkDuration": 0.75 }
  ],
  "transitionDuration": 2.5,
  "transitionType": "immediate | smooth | thruBlack",
  "pixelBrightness": 1.0
}
```

Both `display` and `leds` are optional per message. `leds` is applied by
array position over a *logical* pixel numbering (index 0 = first
externally-visible pixel) -- `firmware/config.py`'s `VISIBLE_PIXEL_MAP`
maps each logical index to its physical position in the chain, so pixels
that are wired but hidden behind the panel are skipped entirely and MQTT
consumers never need to know they exist. Pixels beyond the array length
are left unchanged, entries beyond the logical pixel count are ignored.

Each `leds` entry's `displayMode` is `solid` (default), `blink` (hard
on/off), or `blinkThruBlack` (fades to black and back, prettier and less
jarring -- good for "this build is currently running" indicators).
`blinkDuration` (seconds, fractional allowed, e.g. `0.5`) is the length of
*each leg* of the cycle -- on-hold/off-hold for `blink`, fade-down/fade-up
for `blinkThruBlack` -- so a full round trip takes `2 x blinkDuration`.
If a new color/mode arrives for a pixel while it's mid-fade-down, the
fade-down finishes at its already-established rate rather than snapping,
and the new color/duration only takes over once it reaches black -- avoids
a jarring cut mid-fade if you're updating colors faster than the blink
cycle.

`transitionDuration` (seconds, fractional allowed) and `transitionType`
apply to `solid` pixels in that same message's `leds` update — one setting
per message, not per pixel (blinking pixels use their own `blinkDuration`
instead). `immediate` (the default) sets colors instantly; `smooth`
linearly blends from each pixel's current color to its target over the
duration; `thruBlack` blends down to off then back up to the target,
split evenly across the duration. A pixel already at its target color is
left alone regardless of these settings.

`pixelBrightness` (0.0-1.0, default 1.0) scales every LED's r/g/b equally
at the point of writing to hardware -- a simple linear multiply, the same
approach Adafruit's own NeoPixel library uses, chosen because scaling all
three channels by the same factor preserves hue/color balance while
dimming (a different curve per channel would shift the perceived color).
It's independent of `leds` -- send it alone to just dim/brighten without
touching any colors. The retained `state` topic always reports each
pixel's original, unscaled color regardless of the current brightness.

`textLine1Align`/`textLine2Align` (default `center`) control horizontal
placement when a line fits within the screen; `left` starts it at the
left edge instead of centering it.

`textLine1AutoScroll`/`textLine2AutoScroll` (default `false`, independent
per line) control what happens when a line is *wider* than the screen:
`false` just left-aligns and clips it; `true` scrolls it back and forth
("ping-pong" -- starts fully visible from the left, scrolls left until
the end of the text is flush with the right edge, then reverses -- no
wraparound jump).

`oledBurnInProtectionInterval`'s meaning depends on the mode: for
`invertDisplay`, it's how many seconds the display must have shown
genuinely *static* content (no text change, no active per-line
scrolling) before inverting once -- inverting on a blind fixed timer
regardless of motion was jarring in practice, so now it only kicks in
once nothing has actually changed for a while, and any new content
(or resuming scroll) immediately un-inverts. New content always renders
right-side-up first; you're never left waiting out a stale inversion to
read it. For `bounce`, it's still just the nudge cadence, unaffected by
staticness. The device clamps this to 300s max and defaults to 60s if
omitted (a value can be lower than 60, just not above 300).
`oledBurnInProtectionMode` picks the strategy: `invertDisplay` (default,
motion-aware as above), `bounce` (nudges the text position around the
screen a pixel at a time, DVD-logo style, always on its own timer), or
`off` (no burn-in mitigation at all).

## Tester CLI

```
cd tester
pip install -r requirements.txt

python quadt_tester.py text --line1 "Hello world"
python quadt_tester.py text --line1 "Pipeline Name" --line1-align left --line2 "A long branch name that needs to scroll" --line2-align left --line2-autoscroll
python quadt_tester.py text --line1 "Ready" --burn-in-mode bounce --burn-in-interval 10
python quadt_tester.py text --line1 "Ready" --burn-in-mode off
python quadt_tester.py leds --color 255,0,0
python quadt_tester.py leds --colors 255,0,0 0,255,0 0,0,255
python quadt_tester.py leds --color 0,255,0 --transition-duration 2 --transition-type smooth
python quadt_tester.py leds --color 255,0,0 --display-mode blinkThruBlack --blink-duration 0.75
python quadt_tester.py brightness 0.3
python quadt_tester.py pixels --file frame.bin
python quadt_tester.py raw --file payload.json
python quadt_tester.py watch
```

All commands default to broker `192.168.86.11:1883` and device id
`quadTFrontPanel01` — override with `--broker`/`--port`/`--device-id`.

`watch` subscribes to the state/status topics and prints updates, handy
for confirming what the device actually applied.
