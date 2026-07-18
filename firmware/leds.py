import time

import neopixel


def _lerp_color(c0, c1, p):
    return (
        int(c0[0] + (c1[0] - c0[0]) * p),
        int(c0[1] + (c1[1] - c0[1]) * p),
        int(c0[2] + (c1[2] - c0[2]) * p),
    )


class _Animation:
    def __init__(self, start_color, target_color, start_ms, duration_ms, transition_type):
        self.start_color = start_color
        self.target_color = target_color
        self.start_ms = start_ms
        self.duration_ms = duration_ms
        self.transition_type = transition_type

    def color_at(self, now_ms):
        progress = time.ticks_diff(now_ms, self.start_ms) / self.duration_ms
        if progress >= 1.0:
            return self.target_color, True
        if self.transition_type == "thruBlack":
            black = (0, 0, 0)
            if progress < 0.5:
                return _lerp_color(self.start_color, black, progress * 2), False
            return _lerp_color(black, self.target_color, (progress - 0.5) * 2), False
        # "smooth" (also used as the fallback for any unrecognized type)
        return _lerp_color(self.start_color, self.target_color, progress), False


class LedController:
    """Wraps a neopixel.NeoPixel chain and applies the 'leds' array from an
    incoming MQTT message: index 0 is the first pixel in the chain. Pixels
    beyond the array length are left unchanged; entries beyond num_pixels
    are ignored.

    Supports optional non-blocking color transitions (transition_duration in
    seconds, transition_type "immediate"/"smooth"/"thruBlack") -- call
    tick() regularly from the main loop to advance them.
    """

    def __init__(self, pin, num_pixels):
        self.num_pixels = num_pixels
        self.np = neopixel.NeoPixel(pin, num_pixels)
        self.colors = [(0, 0, 0)] * num_pixels
        self._animations = {}
        self._show()

    def apply(self, leds, transition_duration=0, transition_type="immediate"):
        now = time.ticks_ms()
        duration_ms = int(max(0, transition_duration or 0) * 1000)
        for i, entry in enumerate(leds):
            if i >= self.num_pixels:
                break
            target = (int(entry.get("r", 0)), int(entry.get("g", 0)), int(entry.get("b", 0)))
            current = self.colors[i]
            if target == current:
                self._animations.pop(i, None)
                continue
            if duration_ms <= 0 or transition_type == "immediate":
                self.colors[i] = target
                self._animations.pop(i, None)
            else:
                self._animations[i] = _Animation(current, target, now, duration_ms, transition_type)
        self._show()

    def tick(self, now_ms):
        """Advances any in-flight transitions. Returns True when a
        transition just finished this tick, so the caller knows it's worth
        re-publishing state (the retained state topic otherwise only
        reflects the moment the command first arrived).
        """
        if not self._animations:
            return False
        done = []
        for i, anim in self._animations.items():
            color, finished = anim.color_at(now_ms)
            self.colors[i] = color
            if finished:
                done.append(i)
        for i in done:
            del self._animations[i]
        self._show()
        return bool(done)

    def as_state(self):
        return [{"r": r, "g": g, "b": b} for (r, g, b) in self.colors]

    def _show(self):
        for i, color in enumerate(self.colors):
            self.np[i] = color
        self.np.write()
