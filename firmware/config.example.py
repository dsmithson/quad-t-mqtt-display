# Copy this file to config.py and fill in real values.
# config.py is gitignored -- it will contain your WiFi credentials.

WIFI_SSID = "your-wifi-ssid"
WIFI_PASSWORD = "your-wifi-password"

# MQTT broker for this device. No auth today; change here if the broker moves.
MQTT_BROKER = "192.168.86.11"
MQTT_PORT = 1883

# Identifies this panel on the broker; also used to build the topic tree:
#   quadTFrontPanel/<DEVICE_ID>/set     -- subscribe, incoming commands
#   quadTFrontPanel/<DEVICE_ID>/state   -- publish, retained, last-applied state
#   quadTFrontPanel/<DEVICE_ID>/status  -- publish, retained, "online"/"offline" (LWT)
DEVICE_ID = "quadTFrontPanel01"

# --- OLED (Adafruit #661, SSD1306 128x32 SPI) ---
OLED_WIDTH = 128
OLED_HEIGHT = 32
OLED_SPI_ID = 0
OLED_SCK_PIN = 6
OLED_MOSI_PIN = 7
OLED_CS_PIN = 5
OLED_DC_PIN = 4
OLED_RES_PIN = 3

# --- NeoPixels ---
NEOPIXEL_PIN = 15
# Total physical pixels wired in the chain (salvaged board + Jewel).
NUM_PIXELS_PHYSICAL = 15
# Maps each externally-addressable "logical" pixel (index 0 = first pixel
# MQTT can see) to its physical position in the chain. Pixels not listed
# here are physically wired but hidden behind the panel -- boot_flash()
# still exercises them as a hardware self-test, but apply()/as_state()
# never touch or report them. Determine which physical indices are
# visible by sending an alternating test pattern and looking at the
# panel; adjust this list to match your build.
VISIBLE_PIXEL_MAP = [0, 2, 4, 6, 8, 9, 10, 11, 12, 13, 14]

# Render/poll loop tick rate, in Hz.
TICK_HZ = 15
