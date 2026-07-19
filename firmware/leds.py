import time

import neopixel


def _lerp_color(c0, c1, p):
    return (
        int(c0[0] + (c1[0] - c0[0]) * p),
        int(c0[1] + (c1[1] - c0[1]) * p),
        int(c0[2] + (c1[2] - c0[2]) * p),
    )


class _Animation:
    """A one-shot move from one color to another, used for 'solid' pixels
    (transitionDuration/transitionType from the message)."""

    def __init__(self, start_color, target_color, start_ms, duration_ms, transition_type):
        self.start_color = start_color
        self.target_color = target_color
        self.start_ms = start_ms
        self.duration_ms = duration_ms
        self.transition_type = transition_type

    def color_at(self, now_ms):
        progress = max(0.0, time.ticks_diff(now_ms, self.start_ms) / self.duration_ms)
        if progress >= 1.0:
            return self.target_color, True
        if self.transition_type == "thruBlack":
            black = (0, 0, 0)
            if progress < 0.5:
                return _lerp_color(self.start_color, black, progress * 2), False
            return _lerp_color(black, self.target_color, (progress - 0.5) * 2), False
        # "smooth" (also used as the fallback for any unrecognized type)
        return _lerp_color(self.start_color, self.target_color, progress), False


class _BlinkState:
    """A perpetual on_color<->black cycle for 'blink'/'blinkThruBlack'
    pixels (blinkDuration is the length of each leg -- descending to black,
    or ascending back to on_color -- so a full cycle is 2x blinkDuration).

    If a new color/mode arrives while this is mid-descent (fading/stepping
    toward black), it's stashed in `pending` rather than applied
    immediately, so the descent finishes at its already-established rate;
    the redirect takes effect the instant it reaches black, right before
    the next ascent starts. Arriving during the ascent (or once settled)
    just applies immediately -- there's no "descent in progress" to
    protect at that point.
    """

    def __init__(self, mode, on_color, leg_ms, now_ms, descend_from=None):
        self.mode = mode
        self.on_color = on_color
        self.leg_ms = leg_ms
        self.phase = "descending"
        self.phase_start_ms = now_ms
        self.descend_from = descend_from if descend_from is not None else on_color
        self.pending = None

    def color_at(self, now_ms):
        elapsed_ms = time.ticks_diff(now_ms, self.phase_start_ms)
        progress = max(0.0, min(1.0, elapsed_ms / self.leg_ms)) if self.leg_ms > 0 else 1.0
        if progress >= 1.0:
            if self.phase == "descending":
                if self.pending is not None:
                    self.mode, self.on_color, self.leg_ms = self.pending
                    self.pending = None
                self.phase = "ascending"
            else:
                self.descend_from = self.on_color
                self.phase = "descending"
            self.phase_start_ms = now_ms
            progress = 0.0

        black = (0, 0, 0)
        if self.mode == "blinkThruBlack":
            if self.phase == "descending":
                return _lerp_color(self.descend_from, black, progress)
            return _lerp_color(black, self.on_color, progress)
        # "blink": hard step, no fade -- hold each end of the cycle for the
        # whole leg.
        return black if self.phase == "descending" else self.on_color


class LedController:
    """Wraps a neopixel.NeoPixel chain of physical_count pixels, exposing
    only a "logical" subset externally (via apply()/as_state(), what MQTT
    talks about) per visible_map -- a list of physical indices, one per
    logical position. This lets pixels that are physically wired but not
    visible through the front panel stay invisible to MQTT consumers too.

    apply()'s 'leds' argument is a list of dicts indexed by LOGICAL
    position (index 0 is the first visible pixel): {r, g, b, displayMode,
    blinkDuration}. displayMode is "solid" (default), "blink" (hard on/off
    at blinkDuration-second intervals), or "blinkThruBlack" (fades to
    black and back at blinkDuration-second legs). transition_duration/
    transition_type (from the message, not per-pixel) only apply to
    "solid" pixels. Entries beyond the logical count are ignored; a
    shorter list leaves the remaining logical pixels unchanged.

    pixelBrightness (0.0-1.0, set via set_brightness()) scales every
    channel equally at the point of writing to hardware -- stored/reported
    colors are always the unscaled, original values.

    Call tick() regularly from the main loop to advance transitions/blinks.
    """

    def __init__(self, pin, physical_count, visible_map=None):
        self.physical_count = physical_count
        self.visible_map = list(visible_map) if visible_map is not None else list(range(physical_count))
        self.num_pixels = len(self.visible_map)
        self.np = neopixel.NeoPixel(pin, physical_count)
        self.colors = [(0, 0, 0)] * physical_count
        self.brightness = 1.0
        self._animations = {}
        self._blinks = {}
        self._show()

    def apply(self, leds, transition_duration=0, transition_type="immediate"):
        now = time.ticks_ms()
        duration_ms = int(max(0, transition_duration or 0) * 1000)
        for i, entry in enumerate(leds):
            if i >= self.num_pixels:
                break
            phys = self.visible_map[i]
            target = (int(entry.get("r", 0)), int(entry.get("g", 0)), int(entry.get("b", 0)))
            mode = entry.get("displayMode", "solid")

            if mode not in ("blink", "blinkThruBlack"):
                # "solid" (also the fallback for any unrecognized displayMode)
                self._blinks.pop(phys, None)
                current = self.colors[phys]
                if target == current:
                    self._animations.pop(phys, None)
                    continue
                if duration_ms <= 0 or transition_type == "immediate":
                    self.colors[phys] = target
                    self._animations.pop(phys, None)
                else:
                    self._animations[phys] = _Animation(current, target, now, duration_ms, transition_type)
                continue

            self._animations.pop(phys, None)
            leg_ms = int(max(0.01, entry.get("blinkDuration", 1.0)) * 1000)
            existing = self._blinks.get(phys)
            if (
                existing is not None
                and existing.mode == mode
                and existing.on_color == target
                and existing.leg_ms == leg_ms
                and existing.pending is None
            ):
                continue  # already doing exactly this
            if existing is not None and existing.phase == "descending":
                existing.pending = (mode, target, leg_ms)
            else:
                self._blinks[phys] = _BlinkState(mode, target, leg_ms, now, descend_from=self.colors[phys])
        self._show()

    def tick(self, now_ms):
        """Advances any in-flight transitions/blinks. Returns True when a
        one-shot ('solid') transition just finished this tick, so the
        caller knows it's worth re-publishing state (the retained state
        topic otherwise only reflects the moment the command first
        arrived). Perpetual blinking never triggers a republish on its own
        -- that would spam the retained topic forever.
        """
        changed = False
        finished_transition = False

        if self._animations:
            done = []
            for phys, anim in self._animations.items():
                color, finished = anim.color_at(now_ms)
                self.colors[phys] = color
                if finished:
                    done.append(phys)
            for phys in done:
                del self._animations[phys]
            changed = True
            finished_transition = bool(done)

        if self._blinks:
            for phys, blink in self._blinks.items():
                self.colors[phys] = blink.color_at(now_ms)
            changed = True

        if changed:
            self._show()
        return finished_transition

    def as_state(self):
        # Reports each pixel's stable target color, not the live,
        # mid-blink instantaneous value -- the latter is just noise in a
        # retained state topic (it depends on exactly when it's read).
        state = []
        for phys in self.visible_map:
            blink = self._blinks.get(phys)
            if blink is not None:
                r, g, b = blink.pending[1] if blink.pending is not None else blink.on_color
                mode = blink.pending[0] if blink.pending is not None else blink.mode
                state.append({"r": r, "g": g, "b": b, "displayMode": mode, "blinkDuration": blink.leg_ms / 1000})
            else:
                r, g, b = self.colors[phys]
                state.append({"r": r, "g": g, "b": b, "displayMode": "solid"})
        return state

    def set_brightness(self, value):
        try:
            value = float(value)
        except (TypeError, ValueError):
            value = 1.0
        self.brightness = max(0.0, min(1.0, value))
        self._show()

    def set_all_physical(self, color):
        """Bench/boot self-test only: sets every physical pixel, including
        ones hidden behind the panel and not exposed via apply()/as_state().
        """
        self.colors = [color] * self.physical_count
        self._animations.clear()
        self._blinks.clear()
        self._show()

    def _show(self):
        b = self.brightness
        for i, color in enumerate(self.colors):
            if b >= 1.0:
                self.np[i] = color
            else:
                r, g, bl = color
                self.np[i] = (int(r * b), int(g * b), int(bl * b))
        self.np.write()
