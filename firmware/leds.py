import neopixel


class LedController:
    """Wraps a neopixel.NeoPixel chain and applies the 'leds' array from an
    incoming MQTT message: index 0 is the first pixel in the chain. Pixels
    beyond the array length are left unchanged; entries beyond num_pixels
    are ignored.
    """

    def __init__(self, pin, num_pixels):
        self.num_pixels = num_pixels
        self.np = neopixel.NeoPixel(pin, num_pixels)
        self.colors = [(0, 0, 0)] * num_pixels
        self._show()

    def apply(self, leds):
        for i, entry in enumerate(leds):
            if i >= self.num_pixels:
                break
            r = int(entry.get("r", 0))
            g = int(entry.get("g", 0))
            b = int(entry.get("b", 0))
            self.colors[i] = (r, g, b)
        self._show()

    def as_state(self):
        return [{"r": r, "g": g, "b": b} for (r, g, b) in self.colors]

    def _show(self):
        for i, color in enumerate(self.colors):
            self.np[i] = color
        self.np.write()
