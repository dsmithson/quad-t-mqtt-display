import time

try:
    import binascii
except ImportError:
    import ubinascii as binascii

FONT_W = 8
FONT_H = 8
SCROLL_STEP_PX = 2
SCROLL_GAP_PX = 32

# Burn-in mitigation: whatever the incoming message asks for, never let the
# interval exceed this, and always fall back to it when unset. A "real"
# front panel firmware this was salvaged from used roughly this cadence.
MAX_BURN_IN_INTERVAL_S = 30
DEFAULT_BURN_IN_MODE = "invertDisplay"

BOUNCE_STEP_PX = 1
BOUNCE_RANGE_X = 8
BOUNCE_RANGE_Y = 5


class DisplayController:
    """Wraps an ssd1306.SSD1306_SPI and applies the display section of the
    MQTT command JSON: drawMode (pixels/text1Line/text2Line),
    oledBurnInProtectionInterval/oledBurnInProtectionMode (burn-in
    mitigation), pixelData (raw 1bpp framebuffer), and scrolling text.
    """

    def __init__(self, oled):
        self.oled = oled
        self.width = oled.width
        self.height = oled.height

        self.draw_mode = "text1Line"
        self.burn_in_interval_s = MAX_BURN_IN_INTERVAL_S
        self.burn_in_mode = DEFAULT_BURN_IN_MODE
        self.pixel_data_hex = ""
        self.line1 = ""
        self.line2 = ""
        self.autoscroll = False

        self._invert_state = False
        self._last_burn_in_ms = time.ticks_ms()
        self._scroll_x = {1: 0, 2: 0}
        self._bounce_x = 0
        self._bounce_y = 0
        self._bounce_dx = BOUNCE_STEP_PX
        self._bounce_dy = BOUNCE_STEP_PX

        self.render()

    def apply(self, cfg):
        """cfg is the 'display' dict from an incoming MQTT message. Any key
        that's present replaces the current value; omitted keys keep
        whatever was last set.
        """
        self.draw_mode = cfg.get("drawMode", self.draw_mode)
        interval = cfg.get("oledBurnInProtectionInterval", self.burn_in_interval_s)
        if not interval or interval > MAX_BURN_IN_INTERVAL_S:
            interval = MAX_BURN_IN_INTERVAL_S
        self.burn_in_interval_s = interval
        self.burn_in_mode = cfg.get("oledBurnInProtectionMode", self.burn_in_mode)
        self.pixel_data_hex = cfg.get("pixelData", self.pixel_data_hex)
        self.line1 = cfg.get("textLine1", self.line1)
        self.line2 = cfg.get("textLine2", self.line2)
        self.autoscroll = cfg.get("textAutoScroll", self.autoscroll)
        self._scroll_x = {1: 0, 2: 0}
        self.render()

    def as_state(self):
        return {
            "drawMode": self.draw_mode,
            "oledBurnInProtectionInterval": self.burn_in_interval_s,
            "oledBurnInProtectionMode": self.burn_in_mode,
            "textLine1": self.line1,
            "textLine2": self.line2,
            "textAutoScroll": self.autoscroll,
        }

    def tick(self, now_ms):
        changed = False

        elapsed = time.ticks_diff(now_ms, self._last_burn_in_ms)
        if elapsed >= self.burn_in_interval_s * 1000:
            if self.burn_in_mode == "bounce":
                self._advance_bounce()
                changed = True
            else:
                self._invert_state = not self._invert_state
                self.oled.invert(self._invert_state)
            self._last_burn_in_ms = now_ms

        if self.autoscroll and self.draw_mode in ("text1Line", "text2Line"):
            for line_no in self._active_line_numbers():
                text = self.line1 if line_no == 1 else self.line2
                if len(text) * FONT_W > self.width:
                    self._scroll_x[line_no] += SCROLL_STEP_PX
                    changed = True

        if changed:
            self.render()

    def _advance_bounce(self):
        self._bounce_x += self._bounce_dx
        if self._bounce_x <= -BOUNCE_RANGE_X or self._bounce_x >= BOUNCE_RANGE_X:
            self._bounce_dx = -self._bounce_dx
            self._bounce_x = max(-BOUNCE_RANGE_X, min(self._bounce_x, BOUNCE_RANGE_X))
        self._bounce_y += self._bounce_dy
        if self._bounce_y <= -BOUNCE_RANGE_Y or self._bounce_y >= BOUNCE_RANGE_Y:
            self._bounce_dy = -self._bounce_dy
            self._bounce_y = max(-BOUNCE_RANGE_Y, min(self._bounce_y, BOUNCE_RANGE_Y))

    def _active_line_numbers(self):
        if self.draw_mode == "text1Line":
            return (1,)
        if self.draw_mode == "text2Line":
            return (1, 2)
        return ()

    def _line_x(self, line_no, text):
        text_w = len(text) * FONT_W
        if text_w <= self.width:
            return max(0, (self.width - text_w) // 2)
        if not self.autoscroll:
            return 0
        period = text_w + SCROLL_GAP_PX
        offset = self._scroll_x[line_no] % period
        return self.width - offset

    def render(self):
        oled = self.oled

        if self.draw_mode == "pixels":
            expected_len = (self.width * self.height) // 8
            try:
                raw = binascii.unhexlify(self.pixel_data_hex)
            except (ValueError, TypeError):
                raw = b""
            if len(raw) == expected_len:
                oled.buffer[:] = raw
            else:
                oled.fill(0)
            oled.show()
            return

        bx = self._bounce_x if self.burn_in_mode == "bounce" else 0
        by = self._bounce_y if self.burn_in_mode == "bounce" else 0

        oled.fill(0)
        if self.draw_mode == "text1Line":
            y = (self.height - FONT_H) // 2
            oled.text(self.line1, self._line_x(1, self.line1) + bx, y + by)
        elif self.draw_mode == "text2Line":
            y1 = max(0, self.height // 4 - FONT_H // 2)
            y2 = self.height - y1 - FONT_H
            oled.text(self.line1, self._line_x(1, self.line1) + bx, y1 + by)
            oled.text(self.line2, self._line_x(2, self.line2) + bx, y2 + by)
        oled.show()
