# Quad-T Front Panel Lights

MicroPython firmware for a Raspberry Pi Pico W that drives a salvaged
Christie Quad-T video encoder front panel (a 128x32 SSD1306 SPI OLED plus a
NeoPixel chain) over MQTT, a Python tester CLI for driving it from a dev
machine, and a Go service (`azureBuildMonitor/`) that's an actual consumer
of the panel -- monitors Azure DevOps pipelines and displays their status.
They're versioned together in one repo since the client app and the
device it drives need to stay in sync. See `README.md` for the panel's
full pinout, MQTT topic/JSON schema, and usage instructions -- this file
is about how to *work on* the codebase.

## Layout

- `firmware/` -- MicroPython source that runs on the Pico W. `main.py` is
  the entry point; `display.py`, `leds.py`, `mqtt_client.py` are the three
  subsystems it wires together. `firmware/lib/` holds vendored third-party
  drivers (`ssd1306.py`, `umqtt/simple.py`) -- treat these as upstream code,
  not something to hand-edit; if a bug turns up in one, pull the real
  source from `micropython-lib` rather than patching blind, see Gotchas.
  `firmware/config.py` is gitignored (has real WiFi credentials) --
  `config.example.py` is the template that stays in git.
- `tester/quadt_tester.py` -- a paho-mqtt CLI for publishing test payloads
  and watching the device's `state`/`status` topics. This is the fastest
  way to verify a firmware change actually works; prefer it over writing
  one-off MQTT test scripts.
- `azureBuildMonitor/` -- a standalone Go service/app with its own
  `go.mod`, tests, Dockerfile, and Helm chart (`helm/`). See its own
  README for details. Its `internal/quadt` package is a hand-written
  client for the panel's MQTT protocol (mirrors `firmware/README.md`'s
  JSON schema) -- if the firmware's schema changes, that package needs a
  matching update. GitHub Actions for it lives at the repo root
  (`.github/workflows/azure-build-monitor-docker-publish.yml`) since
  workflows can't live in subdirectories; it's path-filtered to only
  trigger on changes under `azureBuildMonitor/`.

## Dev workflow

The firmware is pushed to the device over USB with `mpremote`, and tested
live over MQTT with the tester CLI -- there's no simulator, verification
means actually running it on the hardware. See the `flash-and-test` skill
in `.claude/skills/` for the exact commands (COM port discovery, pushing
files, watching boot output, confirming state over MQTT); it exists
because this workflow has a couple of non-obvious traps (see Gotchas)
worth not rediscovering each session.

## Architecture notes

- **Logical vs. physical LED addressing.** The NeoPixel chain has more
  physical pixels wired than are visible through the front panel (4 of
  the 8 pixels on the salvaged board are hidden). `LedController` in
  `leds.py` takes a physical pixel count and a `visible_map` (physical
  index per logical position, from `config.VISIBLE_PIXEL_MAP`) and only
  ever exposes the logical/visible subset through `apply()`/`as_state()`
  -- i.e. through MQTT. The one exception is `set_all_physical()`, used
  only by the boot self-test flash, which deliberately touches every
  physical pixel including hidden ones as a hardware sanity check.
- **Non-blocking LED transitions.** `LedController` can animate colors
  over time (`transitionDuration`/`transitionType` in the MQTT payload)
  without blocking the main loop -- `tick()` advances any in-flight
  animations and must be called every iteration. It returns `True` the
  instant a one-shot transition finishes, which `main.py` uses to
  re-publish the retained `state` topic (state otherwise only reflects
  the color at the moment the command arrived, not where it ended up).
  Perpetual blinking (below) never triggers that republish on its own --
  that would spam the retained topic forever.
- **Perpetual per-pixel blinking** (`displayMode: blink`/`blinkThruBlack`)
  is a *separate* animation system from the one-shot transitions above --
  `_BlinkState` in `leds.py`, tracked in `LedController._blinks` (keyed by
  physical index, same as `_animations`; a pixel is in at most one of the
  two). It models a cycle as two "legs" (descending toward black,
  ascending back to the target), each `blinkDuration` seconds. If a new
  command retargets a pixel while its descending leg is in flight, the
  redirect is stashed in `_BlinkState.pending` rather than applied
  immediately -- the descent finishes at its already-established rate,
  and the redirect only takes effect the instant it reaches black. Do not
  "simplify" this by applying new blink targets immediately; that's the
  specific jarring behavior it was built to avoid. `as_state()`
  deliberately reports the *stable target* color (`on_color`, or
  `pending`'s target if a redirect is queued), never the live
  instantaneous value -- the latter is just noise in a retained topic
  since it depends on exactly when it's read.
- **`pixelBrightness`** is a simple linear scale applied to every channel
  equally, only at the point `_show()` writes to hardware -- `self.colors`
  and `as_state()` always hold/report the original unscaled values. Keep
  it that way; scaling storage in place would make brightness changes
  lossy and complicate every other piece of pixel-state logic.
- **OLED burn-in mitigation.** `DisplayController.tick()` handles both
  strategies (`invertDisplay` and `bounce`) on the same
  `oledBurnInProtectionInterval` timer, which is always clamped to 30s
  max regardless of what a message requests.
- **OLED periodic self-heal.** `DisplayController.tick()` also
  unconditionally re-sends the full SSD1306 init sequence + redraw every
  `OLED_REINIT_INTERVAL_MS` (15s), regardless of whether anything visibly
  changed. This exists because SPI writes don't fail loudly -- a
  transient glitch (noise, a marginal connection, a brownout) can corrupt
  the controller's internal state without ever raising a Python
  exception, leaving the screen dark or blank until *something* resends
  a full init. This bounds that outage to one interval instead of
  forever; it is not a fix for a genuinely loose/broken physical
  connection, which still needs to be checked by hand.

## Known gotchas (all found the hard way on real hardware -- don't
## re-derive these, they cost real debugging time the first time around)

- **WiFi reconnect must disconnect first.** Calling `wlan.connect()`
  repeatedly without an intervening `wlan.disconnect()` wedges the CYW43
  radio at `STAT_CONNECTING` forever. `connect_wifi()` in `main.py`
  already does clean disconnect -> connect -> bounded-poll cycles; don't
  simplify that back to a bare retry loop.
- **This mosquitto broker rejects `keepalive=0`.** umqtt's default
  keepalive is 0, which causes a specific broker (eclipse-mosquitto
  2.0.14) to accept the CONNECT and then immediately kill the socket
  (`Bad socket read/write: Invalid arguments provided` in its logs, an
  MQTT CONNACK rc=2 "identifier rejected" on the client side -- a
  misleading error that has nothing to do with the actual client ID).
  `mqtt_client.py` connects with a real keepalive and sends periodic
  pings from the main loop tick; don't drop that.
- **Power the Pico from VSYS, not VBUS**, when running off an external 5V
  supply shared with the NeoPixels -- see README's Power section for why.
- **WS2812 3.3V-logic-on-5V marginal voltage.** The Pico's 3.3V GPIO is
  below the WS2812's nominal logic-high threshold at a true 5V rail. Two
  silicon diodes in series in the LED +5V feed (not the data line) drop
  it to a safer ~4.2-4.4V; a single diode's ~0.6-0.7V drop wasn't always
  enough, especially since diode forward voltage drops further at the
  low currents this chain mostly draws.
- **The vendored `firmware/lib/umqtt/simple.py` is intentionally
  byte-for-byte identical to the real `micropython-lib` source** --
  earlier in this project's history a transcription-from-memory copy was
  suspected buggy and "fixed" a few times before it turned out the
  original was correct all along (the real bug was cluster networking,
  see below). If umqtt behavior ever looks wrong again, diff against the
  actual upstream file before changing protocol-level code in it.
- **If the MQTT broker (a separate `homelab-k8s-mqtt` Helm deployment,
  not part of this repo) ever seems to reject well-formed clients with
  strange, inconsistent errors again**, check `externalTrafficPolicy` on
  its Service before suspecting this codebase -- a `Cluster` policy let
  MetalLB's L2 speaker election answer ARP from a node other than the one
  running the mosquitto pod, forcing an extra kube-proxy SNAT hop that
  corrupted connections from at least one client. Fixed by setting it to
  `Local`.

## Verification

There's no automated test suite (bench hardware, not really testable
without the physical device). To verify a change: push firmware with
`mpremote`, reset, watch boot output, then use `tester/quadt_tester.py`
to send commands and confirm both the physical device and the retained
`state`/`status` MQTT topics reflect what's expected.
