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
# Set this once the real chain length is known (existing strip + Jewel).
NUM_PIXELS = 11

# Render/poll loop tick rate, in Hz.
TICK_HZ = 15
