import time

try:
    import binascii
except ImportError:
    import ubinascii as binascii

FONT_W = 8
FONT_H = 8
SCROLL_STEP_PX = 2

# Burn-in mitigation. oledBurnInProtectionMode "invertDisplay" now only
# fires once the display has held genuinely static content (no text
# change, no active per-line scrolling) for this many seconds -- flipping
# blind on a fixed timer regardless of motion was jarring in practice.
# "bounce" is unaffected by staticness and keeps its original
# always-on-timer nudge. oledBurnInProtectionInterval means "seconds of
# staticness required" for invertDisplay, or "seconds between nudges" for
# bounce -- same field, mode-dependent meaning.
DEFAULT_BURN_IN_INTERVAL_S = 60
MAX_BURN_IN_INTERVAL_S = 300
DEFAULT_BURN_IN_MODE = "invertDisplay"

BOUNCE_STEP_PX = 1
BOUNCE_RANGE_X = 8
BOUNCE_RANGE_Y = 5

# Self-heal: periodically re-send the full SSD1306 init sequence and
# redraw, regardless of whether anything changed. A glitch (electrical
# noise, a marginal connection, a brownout) can corrupt the controller's
# internal state -- lost display RAM, or a stray command that leaves it
# powered off -- without it ever raising a Python-visible error, since SPI
# writes don't fail loudly. This bounds how long the screen can stay dark
# from that kind of transient to one interval instead of forever. It's not
# a fix for a genuinely loose/broken physical connection, just a bound on
# how long a transient glitch can leave the screen dark.
OLED_REINIT_INTERVAL_MS = 15000


class DisplayController:
    """Wraps an ssd1306.SSD1306_SPI and applies the display section of the
    MQTT command JSON: drawMode (pixels/text1Line/text2Line),
    oledBurnInProtectionInterval/oledBurnInProtectionMode (burn-in
    mitigation), pixelData (raw 1bpp framebuffer), and per-line alignment/
    scrolling text (textLine1Align/textLine2Align,
    textLine1AutoScroll/textLine2AutoScroll).
    """

    def __init__(self, oled):
        self.oled = oled
        self.width = oled.width
        self.height = oled.height

        self.draw_mode = "text1Line"
        self.burn_in_interval_s = DEFAULT_BURN_IN_INTERVAL_S
        self.burn_in_mode = DEFAULT_BURN_IN_MODE
        self.pixel_data_hex = ""
        self.line1 = ""
        self.line2 = ""
        self.line1_align = "center"
        self.line2_align = "center"
        self.line1_autoscroll = False
        self.line2_autoscroll = False

        self._invert_state = False
        self._last_burn_in_ms = time.ticks_ms()
        self._last_content_change_ms = time.ticks_ms()
        self._last_reinit_ms = time.ticks_ms()
        # Per-line horizontal scroll position/direction, for lines too
        # wide to fit that have autoscroll on. Ping-pongs between 0 (the
        # start of the text, left-aligned) and width-text_w (the end of
        # the text, flush right) -- no wraparound jump.
        self._scroll_x = {1: 0, 2: 0}
        self._scroll_dx = {1: -SCROLL_STEP_PX, 2: -SCROLL_STEP_PX}
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
        old_content = self._content_signature()

        self.draw_mode = cfg.get("drawMode", self.draw_mode)
        interval = cfg.get("oledBurnInProtectionInterval", self.burn_in_interval_s)
        if not interval:
            interval = DEFAULT_BURN_IN_INTERVAL_S
        elif interval > MAX_BURN_IN_INTERVAL_S:
            interval = MAX_BURN_IN_INTERVAL_S
        self.burn_in_interval_s = interval
        self.burn_in_mode = cfg.get("oledBurnInProtectionMode", self.burn_in_mode)
        self.pixel_data_hex = cfg.get("pixelData", self.pixel_data_hex)
        self.line1 = cfg.get("textLine1", self.line1)
        self.line2 = cfg.get("textLine2", self.line2)
        self.line1_align = cfg.get("textLine1Align", self.line1_align)
        self.line2_align = cfg.get("textLine2Align", self.line2_align)
        self.line1_autoscroll = cfg.get("textLine1AutoScroll", self.line1_autoscroll)
        self.line2_autoscroll = cfg.get("textLine2AutoScroll", self.line2_autoscroll)

        self._scroll_x = {1: 0, 2: 0}
        self._scroll_dx = {1: -SCROLL_STEP_PX, 2: -SCROLL_STEP_PX}

        if self._content_signature() != old_content:
            now = time.ticks_ms()
            self._last_content_change_ms = now
            if self._invert_state:
                # Fresh content should always render normally first; don't
                # make someone wait out a stale inversion to read it.
                self._invert_state = False
                self.oled.invert(False)
                self._last_burn_in_ms = now

        self.render()

    def _content_signature(self):
        return (self.draw_mode, self.line1, self.line2, self.pixel_data_hex)

    def as_state(self):
        return {
            "drawMode": self.draw_mode,
            "oledBurnInProtectionInterval": self.burn_in_interval_s,
            "oledBurnInProtectionMode": self.burn_in_mode,
            "textLine1": self.line1,
            "textLine2": self.line2,
            "textLine1Align": self.line1_align,
            "textLine2Align": self.line2_align,
            "textLine1AutoScroll": self.line1_autoscroll,
            "textLine2AutoScroll": self.line2_autoscroll,
        }

    def tick(self, now_ms):
        changed = False

        if time.ticks_diff(now_ms, self._last_reinit_ms) >= OLED_REINIT_INTERVAL_MS:
            self.oled.init_display()
            self.oled.invert(self._invert_state)
            self._last_reinit_ms = now_ms
            changed = True

        if self.burn_in_mode == "off":
            pass
        elif self.burn_in_mode == "bounce":
            elapsed = time.ticks_diff(now_ms, self._last_burn_in_ms)
            if elapsed >= self.burn_in_interval_s * 1000:
                self._advance_bounce()
                self._last_burn_in_ms = now_ms
                changed = True
        else:
            # invertDisplay (also the fallback for any unrecognized mode):
            # only invert once content has been static -- no text change,
            # no active scrolling -- for burn_in_interval_s, and don't
            # re-toggle more often than that even while it stays static.
            static_for = time.ticks_diff(now_ms, self._last_content_change_ms)
            since_toggle = time.ticks_diff(now_ms, self._last_burn_in_ms)
            if static_for >= self.burn_in_interval_s * 1000 and since_toggle >= self.burn_in_interval_s * 1000:
                self._invert_state = not self._invert_state
                self.oled.invert(self._invert_state)
                self._last_burn_in_ms = now_ms

        if self.draw_mode in ("text1Line", "text2Line"):
            for line_no in self._active_line_numbers():
                text = self.line1 if line_no == 1 else self.line2
                autoscroll = self.line1_autoscroll if line_no == 1 else self.line2_autoscroll
                if autoscroll and len(text) * FONT_W > self.width:
                    self._advance_line_scroll(line_no, text)
                    self._last_content_change_ms = now_ms  # scrolling is motion too
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

    def _advance_line_scroll(self, line_no, text):
        text_w = len(text) * FONT_W
        min_x = self.width - text_w  # negative: how far left the text must go to show its tail
        x = self._scroll_x[line_no] + self._scroll_dx[line_no]
        if x <= min_x:
            x = min_x
            self._scroll_dx[line_no] = SCROLL_STEP_PX
        elif x >= 0:
            x = 0
            self._scroll_dx[line_no] = -SCROLL_STEP_PX
        self._scroll_x[line_no] = x

    def _active_line_numbers(self):
        if self.draw_mode == "text1Line":
            return (1,)
        if self.draw_mode == "text2Line":
            return (1, 2)
        return ()

    def _line_x(self, line_no, text, align, autoscroll):
        text_w = len(text) * FONT_W
        if text_w <= self.width:
            if align == "left":
                return 0
            return max(0, (self.width - text_w) // 2)
        if not autoscroll:
            return 0
        return self._scroll_x[line_no]

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
            x = self._line_x(1, self.line1, self.line1_align, self.line1_autoscroll)
            oled.text(self.line1, x + bx, y + by)
        elif self.draw_mode == "text2Line":
            y1 = max(0, self.height // 4 - FONT_H // 2)
            y2 = self.height - y1 - FONT_H
            x1 = self._line_x(1, self.line1, self.line1_align, self.line1_autoscroll)
            x2 = self._line_x(2, self.line2, self.line2_align, self.line2_autoscroll)
            oled.text(self.line1, x1 + bx, y1 + by)
            oled.text(self.line2, x2 + bx, y2 + by)
        oled.show()
