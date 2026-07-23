#!/usr/bin/env python3
"""Bench tester CLI for the Quad-T front panel MQTT display/LED controller.

Publishes JSON command payloads to quadTFrontPanel/<device-id>/set and can
watch the device's state/status topics to confirm what it applied.

Examples:
  quadt_tester.py text --line1 "Hello world"
  quadt_tester.py text --line1 "Pipeline Name" --line1-align left --line2 "A long branch/name/here" --line2-align left --line2-autoscroll
  quadt_tester.py text --line1 "Ready" --burn-in-mode bounce --burn-in-interval 10
  quadt_tester.py text --line1 "Ready" --burn-in-mode off
  quadt_tester.py leds --color 255,0,0
  quadt_tester.py leds --colors 255,0,0 0,255,0 0,0,255
  quadt_tester.py leds --color 0,255,0 --transition-duration 2 --transition-type smooth
  quadt_tester.py leds --color 255,0,0 --display-mode blinkThruBlack --blink-duration 0.75
  quadt_tester.py brightness 0.3
  quadt_tester.py pixels --file frame.bin
  quadt_tester.py raw --file payload.json
  quadt_tester.py watch
"""
import argparse
import binascii
import json
import sys
import time

import paho.mqtt.client as mqtt

DEFAULT_BROKER = "192.168.86.11"
DEFAULT_PORT = 1883
DEFAULT_DEVICE_ID = "quadTFrontPanel01"


def topics(device_id):
    base = "quadTFrontPanel/%s" % device_id
    return {
        "set": base + "/set",
        "state": base + "/state",
        "status": base + "/status",
    }


def make_client():
    return mqtt.Client()


def publish(args, payload):
    t = topics(args.device_id)
    client = make_client()
    client.connect(args.broker, args.port)
    body = json.dumps(payload)
    client.publish(t["set"], body)
    client.disconnect()
    print("published to", t["set"])
    print(json.dumps(payload, indent=2))


def cmd_text(args):
    display = {
        "drawMode": "text2Line" if args.line2 else "text1Line",
        "textLine1": args.line1,
        "textLine1Align": args.line1_align,
        "textLine1AutoScroll": args.line1_autoscroll,
    }
    if args.line2:
        display["textLine2"] = args.line2
        display["textLine2Align"] = args.line2_align
        display["textLine2AutoScroll"] = args.line2_autoscroll
    if args.burn_in_interval is not None:
        display["oledBurnInProtectionInterval"] = args.burn_in_interval
    if args.burn_in_mode is not None:
        display["oledBurnInProtectionMode"] = args.burn_in_mode
    publish(args, {"display": display})


def cmd_pixels(args):
    with open(args.file, "rb") as f:
        raw = f.read()
    expected = (args.width * args.height) // 8
    if len(raw) != expected:
        print(
            "warning: %s is %d bytes, expected %d for a %dx%d 1bpp frame"
            % (args.file, len(raw), expected, args.width, args.height),
            file=sys.stderr,
        )
    hex_data = binascii.hexlify(raw).decode("ascii")
    display = {"drawMode": "pixels", "pixelData": hex_data}
    if args.burn_in_interval is not None:
        display["oledBurnInProtectionInterval"] = args.burn_in_interval
    if args.burn_in_mode is not None:
        display["oledBurnInProtectionMode"] = args.burn_in_mode
    publish(args, {"display": display})


def _parse_color(s, display_mode=None, blink_duration=None):
    parts = [int(x) for x in s.split(",")]
    if len(parts) != 3:
        raise argparse.ArgumentTypeError("expected r,g,b e.g. 255,0,0")
    r, g, b = parts
    entry = {"r": r, "g": g, "b": b}
    if display_mode is not None:
        entry["displayMode"] = display_mode
    if display_mode in ("blink", "blinkThruBlack") and blink_duration is not None:
        entry["blinkDuration"] = blink_duration
    return entry


def cmd_leds(args):
    if args.color:
        leds = [_parse_color(args.color, args.display_mode, args.blink_duration)] * args.count
    elif args.colors:
        leds = [_parse_color(c, args.display_mode, args.blink_duration) for c in args.colors]
    else:
        raise SystemExit("specify --color or --colors")
    payload = {"leds": leds}
    if args.transition_duration is not None:
        payload["transitionDuration"] = args.transition_duration
    if args.transition_type is not None:
        payload["transitionType"] = args.transition_type
    publish(args, payload)


def cmd_brightness(args):
    publish(args, {"pixelBrightness": args.value})


def cmd_raw(args):
    with open(args.file, "r") as f:
        payload = json.load(f)
    publish(args, payload)


def cmd_watch(args):
    t = topics(args.device_id)

    def on_connect(client, userdata, flags, rc):
        print("connected, subscribing to %s and %s" % (t["state"], t["status"]))
        client.subscribe(t["state"])
        client.subscribe(t["status"])

    def on_message(client, userdata, msg):
        ts = time.strftime("%H:%M:%S")
        payload = msg.payload.decode("utf-8", errors="replace")
        print("[%s] %s: %s" % (ts, msg.topic, payload))

    client = make_client()
    client.on_connect = on_connect
    client.on_message = on_message
    client.connect(args.broker, args.port)
    print("watching (ctrl-c to stop)...")
    client.loop_forever()


def build_parser():
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--broker", default=DEFAULT_BROKER, help="MQTT broker host (default: %(default)s)")
    parser.add_argument("--port", type=int, default=DEFAULT_PORT, help="MQTT broker port (default: %(default)s)")
    parser.add_argument("--device-id", default=DEFAULT_DEVICE_ID, help="device id (default: %(default)s)")

    sub = parser.add_subparsers(dest="command", required=True)

    p_text = sub.add_parser("text", help="set display text")
    p_text.add_argument("--line1", default="", help="first line of text")
    p_text.add_argument("--line2", default="", help="second line of text (switches to text2Line mode)")
    p_text.add_argument("--line1-align", choices=("left", "center"), default="center", help="default: %(default)s")
    p_text.add_argument("--line2-align", choices=("left", "center"), default="center", help="default: %(default)s")
    p_text.add_argument("--line1-autoscroll", action="store_true", help="scroll line 1 if wider than the screen")
    p_text.add_argument("--line2-autoscroll", action="store_true", help="scroll line 2 if wider than the screen")
    p_text.add_argument(
        "--burn-in-interval", type=int, default=None,
        help="invertDisplay: seconds of static content required before inverting once; "
             "bounce: seconds between nudges. Capped at 300, defaults to 60 (device-side).",
    )
    p_text.add_argument(
        "--burn-in-mode", choices=("invertDisplay", "bounce", "off"), default=None,
        help="burn-in mitigation strategy (default: device keeps its current mode)",
    )
    p_text.set_defaults(func=cmd_text)

    p_pixels = sub.add_parser("pixels", help="push a raw 1bpp framebuffer")
    p_pixels.add_argument("--file", required=True, help="path to raw binary framebuffer (128x32/8 = 512 bytes)")
    p_pixels.add_argument("--width", type=int, default=128)
    p_pixels.add_argument("--height", type=int, default=32)
    p_pixels.add_argument("--burn-in-interval", type=int, default=None)
    p_pixels.add_argument("--burn-in-mode", choices=("invertDisplay", "bounce", "off"), default=None)
    p_pixels.set_defaults(func=cmd_pixels)

    p_leds = sub.add_parser("leds", help="set NeoPixel colors")
    p_leds.add_argument("--color", help="apply one r,g,b color to --count pixels")
    p_leds.add_argument("--count", type=int, default=11, help="pixel count when using --color (default: %(default)s)")
    p_leds.add_argument("--colors", nargs="+", help="one r,g,b per pixel, e.g. --colors 255,0,0 0,255,0")
    p_leds.add_argument("--transition-duration", type=float, default=None, help="seconds to transition over (default: immediate)")
    p_leds.add_argument(
        "--transition-type", choices=("immediate", "smooth", "thruBlack"), default=None,
        help="how to transition (default: immediate)",
    )
    p_leds.add_argument(
        "--display-mode", choices=("solid", "blink", "blinkThruBlack"), default=None,
        help="applied to every pixel in this command (default: solid)",
    )
    p_leds.add_argument(
        "--blink-duration", type=float, default=None,
        help="seconds per blink leg (on/off or fade-down/fade-up), only used with --display-mode blink/blinkThruBlack",
    )
    p_leds.set_defaults(func=cmd_leds)

    p_brightness = sub.add_parser("brightness", help="set global NeoPixel brightness")
    p_brightness.add_argument("value", type=float, help="0.0 (off) to 1.0 (full, default)")
    p_brightness.set_defaults(func=cmd_brightness)

    p_raw = sub.add_parser("raw", help="publish a raw JSON file as-is")
    p_raw.add_argument("--file", required=True, help="path to a JSON payload file")
    p_raw.set_defaults(func=cmd_raw)

    p_watch = sub.add_parser("watch", help="subscribe to state/status topics and print updates")
    p_watch.set_defaults(func=cmd_watch)

    return parser


def main():
    parser = build_parser()
    args = parser.parse_args()
    args.func(args)


if __name__ == "__main__":
    main()
