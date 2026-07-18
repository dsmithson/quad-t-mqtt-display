---
name: flash-and-test
description: Push firmware changes to the Quad-T front panel's Pico W over USB and verify them live over MQTT. Use whenever firmware/*.py changes and need to be tested on real hardware -- there is no simulator for this project.
---

# Flash and test the Quad-T front panel firmware

There's no emulator here -- every firmware change has to be pushed to the
physical Pico W and exercised over real MQTT to know if it works. This
skill is the loop for doing that quickly.

## 1. Find `mpremote`

The system Python on this machine may be too old (mpremote's
`platformdirs` dependency needs Python >= 3.9). If `pip install mpremote`
fails with a `platformdirs` resolution error, look for a newer Python
install (check `py -0p` on Windows) and install/run mpremote with that
interpreter instead, e.g.:

```
"C:\Users\<you>\AppData\Local\Programs\Python\Python313\python.exe" -m pip install mpremote
```

Use that same interpreter for every `mpremote` invocation below.

## 2. Find the Pico's serial port

```
python -m mpremote connect list
```

Look for the device with vendor:product ID `2e8a:0005` (Raspberry Pi's
vendor ID, MicroPython's product ID on the Pico) -- that's your COM port
(Windows) or `/dev/tty*` device (Linux/Mac).

## 3. Push changed files

Only copy the files that changed -- no need to always push everything.
`firmware/config.py` is real (gitignored) and already on the device;
don't overwrite it unless you're intentionally changing config values.

```
python -m mpremote connect <PORT> fs cp firmware/main.py firmware/leds.py firmware/display.py :
```

First-time setup only, vendored libs rarely change:

```
python -m mpremote connect <PORT> fs cp -r firmware/lib :lib
```

## 4. Reset and watch boot

```
python -m mpremote connect <PORT> reset
timeout 20 python -m mpremote connect <PORT>
```

(`timeout` because `mpremote` with no subcommand attaches an interactive
REPL that streams boot output and never exits on its own -- 15-20s is
enough to see WiFi connect, then it's fine for the command to time out.)

## 5. Verify over MQTT

Prefer the tester CLI over ad-hoc scripts:

```
cd tester
python quadt_tester.py leds --color 255,0,0
python quadt_tester.py text --line1 "Test"
python quadt_tester.py watch   # live-prints state/status as they change
```

To confirm the device's retained state directly (useful when checking
something that only shows up after a delay, like a completed transition):

```python
import paho.mqtt.client as mqtt, time
received = []
c = mqtt.Client()
c.on_connect = lambda cl, ud, fl, rc: cl.subscribe('quadTFrontPanel/quadTFrontPanel01/state')
c.on_message = lambda cl, ud, msg: received.append(msg.payload.decode())
c.connect('192.168.86.11', 1883)
c.loop_start(); time.sleep(2); c.loop_stop()
print(received[-1] if received else 'none')
```

## Debugging a stuck/silent boot

If the device connects to WiFi but goes silent (no crash, no further
output), it may have succeeded quietly -- the main loop doesn't print
anything in steady state. Check the retained `status`/`state` topics
(step 5) before assuming something's wrong.

If it's genuinely stuck, interrupt and probe interactively instead of
guessing:

```
python -m mpremote connect <PORT> exec "import network; wlan = network.WLAN(network.STA_IF); print(wlan.status(), wlan.isconnected())"
```

`mpremote exec` sends a raw-REPL interrupt first, so this safely takes
over from whatever the device was doing without needing a manual reset.
