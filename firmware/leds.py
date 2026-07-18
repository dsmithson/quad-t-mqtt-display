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
    """Wraps a neopixel.NeoPixel chain of physical_count pixels, exposing
    only a "logical" subset externally (via apply()/as_state(), what MQTT
    talks about) per visible_map -- a list of physical indices, one per
    logical position. This lets pixels that are physically wired but not
    visible through the front panel stay invisible to MQTT consumers too.

    apply()'s 'leds' argument is a list of {r,g,b} dicts indexed by LOGICAL
    position (index 0 is the first visible pixel). Entries beyond the
    logical count are ignored; a shorter list leaves the remaining logical
    pixels unchanged.

    Supports optional non-blocking color transitions (transition_duration in
    seconds, transition_type "immediate"/"smooth"/"thruBlack") -- call
    tick() regularly from the main loop to advance them.
    """

    def __init__(self, pin, physical_count, visible_map=None):
        self.physical_count = physical_count
        self.visible_map = list(visible_map) if visible_map is not None else list(range(physical_count))
        self.num_pixels = len(self.visible_map)
        self.np = neopixel.NeoPixel(pin, physical_count)
        self.colors = [(0, 0, 0)] * physical_count
        self._animations = {}
        self._show()

    def apply(self, leds, transition_duration=0, transition_type="immediate"):
        now = time.ticks_ms()
        duration_ms = int(max(0, transition_duration or 0) * 1000)
        for i, entry in enumerate(leds):
            if i >= self.num_pixels:
                break
            phys = self.visible_map[i]
            target = (int(entry.get("r", 0)), int(entry.get("g", 0)), int(entry.get("b", 0)))
            current = self.colors[phys]
            if target == current:
                self._animations.pop(phys, None)
                continue
            if duration_ms <= 0 or transition_type == "immediate":
                self.colors[phys] = target
                self._animations.pop(phys, None)
            else:
                self._animations[phys] = _Animation(current, target, now, duration_ms, transition_type)
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
        for phys, anim in self._animations.items():
            color, finished = anim.color_at(now_ms)
            self.colors[phys] = color
            if finished:
                done.append(phys)
        for phys in done:
            del self._animations[phys]
        self._show()
        return bool(done)

    def as_state(self):
        return [{"r": r, "g": g, "b": b} for (r, g, b) in (self.colors[p] for p in self.visible_map)]

    def set_all_physical(self, color):
        """Bench/boot self-test only: sets every physical pixel, including
        ones hidden behind the panel and not exposed via apply()/as_state().
        """
        self.colors = [color] * self.physical_count
        self._animations.clear()
        self._show()

    def _show(self):
        for i, color in enumerate(self.colors):
            self.np[i] = color
        self.np.write()
