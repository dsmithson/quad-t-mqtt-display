#!/usr/bin/env python3
"""Bench tester CLI for the Quad-T front panel MQTT display/LED controller.

Publishes JSON command payloads to quadTFrontPanel/<device-id>/set and can
watch the device's state/status topics to confirm what it applied.

Examples:
  quadt_tester.py text --line1 "Hello world"
  quadt_tester.py text --line1 "A very long line that needs to scroll" --autoscroll
  quadt_tester.py leds --color 255,0,0
  quadt_tester.py leds --colors 255,0,0 0,255,0 0,0,255
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
        "textAutoScroll": args.autoscroll,
    }
    if args.line2:
        display["textLine2"] = args.line2
    if args.invert_rate is not None:
        display["invertRate"] = args.invert_rate
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
    if args.invert_rate is not None:
        display["invertRate"] = args.invert_rate
    publish(args, {"display": display})


def _parse_color(s):
    parts = [int(x) for x in s.split(",")]
    if len(parts) != 3:
        raise argparse.ArgumentTypeError("expected r,g,b e.g. 255,0,0")
    r, g, b = parts
    return {"r": r, "g": g, "b": b}


def cmd_leds(args):
    if args.color:
        leds = [_parse_color(args.color)] * args.count
    elif args.colors:
        leds = [_parse_color(c) for c in args.colors]
    else:
        raise SystemExit("specify --color or --colors")
    publish(args, {"leds": leds})


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
    p_text.add_argument("--autoscroll", action="store_true", help="scroll lines wider than the screen")
    p_text.add_argument("--invert-rate", type=int, default=None, help="seconds between invert toggles (burn-in mitigation)")
    p_text.set_defaults(func=cmd_text)

    p_pixels = sub.add_parser("pixels", help="push a raw 1bpp framebuffer")
    p_pixels.add_argument("--file", required=True, help="path to raw binary framebuffer (128x32/8 = 512 bytes)")
    p_pixels.add_argument("--width", type=int, default=128)
    p_pixels.add_argument("--height", type=int, default=32)
    p_pixels.add_argument("--invert-rate", type=int, default=None)
    p_pixels.set_defaults(func=cmd_pixels)

    p_leds = sub.add_parser("leds", help="set NeoPixel colors")
    p_leds.add_argument("--color", help="apply one r,g,b color to --count pixels")
    p_leds.add_argument("--count", type=int, default=11, help="pixel count when using --color (default: %(default)s)")
    p_leds.add_argument("--colors", nargs="+", help="one r,g,b per pixel, e.g. --colors 255,0,0 0,255,0")
    p_leds.set_defaults(func=cmd_leds)

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
