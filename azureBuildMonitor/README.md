# Azure Build Monitor

A Go service that watches a set of Azure DevOps build pipelines and drives
the [Quad-T front panel](../README.md) to show their status.

## How it works

- **Hourly full refresh** (`fullRefreshIntervalSeconds`, default 3600):
  fetches the latest 4 builds for every configured pipeline in one Azure
  DevOps API call and overwrites the in-memory store with the
  authoritative result. This is the drift-correction backstop.
- **Real-time webhook listener**: subscribes to the MQTT topic your N8N
  workflow publishes Azure DevOps `run.statechanged` events to (see
  `buildEventTopic`). The webhook payload alone isn't reliable enough to
  use directly -- it carries no branch name, and its `pipelineId` field is
  actually the *run* ID, not the definition ID the rest of this app is
  keyed on. So every event triggers one "get build by ID" REST call,
  which reliably resolves the true definition ID, branch, and everything
  else needed, and patches the store in place.
- **Display cycler**: every `pipelineMessageDurationSeconds` (default 5),
  advances through: show line 2 of the OLED's "status + time in status"
  message, then "Branch: ..." (a full pipeline "slot" is 2x this), then
  moves to the next pipeline. Publishes a fresh command to the Quad-T on
  every tick reflecting current store state, independent of whether
  anything actually changed.

## LED mapping

- **7 hexagon pixels** (logical LEDs 4-10): one per configured pipeline,
  in `buildDefinitionIds` order. Always shows that pipeline's true status
  color/mode; the non-selected 6 are scaled down by `dimPixelMultiplier`
  so the currently-highlighted one stands out.
- **4-pixel strip** (logical LEDs 0-3): the currently-highlighted
  pipeline's most recent builds, newest first. Slots beyond the available
  history are off.

Color scheme (same for both the hexagon and the strip):

| Status | Color | Mode |
|---|---|---|
| Pending | Blue | solid |
| Running | Blue | blinkThruBlack (1s legs) |
| Succeeded | Green | solid |
| Failed | Red | solid |
| Cancelled | Gray | solid |

`partiallySucceeded` from Azure DevOps is treated as Failed -- it means
something needs attention, same as a hard failure.

## Configuration

All via environment variables (see `internal/config/config.go` for
defaults):

| Variable | Required | Description |
|---|---|---|
| `AZURE_DEVOPS_URL` | yes | e.g. `https://dev.azure.com/your-org` |
| `AZURE_DEVOPS_PAT` | yes | Personal access token, Build (Read) scope is enough |
| `AZURE_DEVOPS_PROJECT_NAME` | yes | e.g. `YourProject` |
| `AZURE_DEVOPS_BUILD_DEFINITION_IDS` | yes | comma-separated, max 7 (one hexagon pixel each) |
| `FULL_REFRESH_INTERVAL_SECONDS` | | default 3600 |
| `MQTT_SERVER_URL` / `MQTT_SERVER_PORT` | | default `mqtt-mosquitto.mqtt.svc` / `1883` |
| `BUILD_EVENT_TOPIC` | | default `azureDevOps/builds/buildEvent` |
| `QUADT_DEVICE_NAME` | | default `quadTFrontPanel01` |
| `QUADT_BRIGHTNESS` | | 0.0-1.0, default 1.0, passed straight through as `pixelBrightness` |
| `QUADT_DIM_PIXEL_MULTIPLIER` | | 0.0-1.0, default 0.25 |
| `PIPELINE_MESSAGE_DURATION_SECONDS` | | default 5 |

## Running locally

```
go build ./cmd/azureBuildMonitor
AZURE_DEVOPS_URL=https://dev.azure.com/your-org \
AZURE_DEVOPS_PAT=<your PAT> \
AZURE_DEVOPS_PROJECT_NAME=YourProject \
AZURE_DEVOPS_BUILD_DEFINITION_IDS=1,2,3,4,5,6,7 \
MQTT_SERVER_URL=192.168.86.11 \
./azureBuildMonitor
```

(Use the broker's LAN IP, not its in-cluster DNS name, when running
outside the cluster.)

## Testing

```
go test ./...
```

Unit tests cover the status/color mapping, dim-multiplier clamping, the
strip/hexagon LED array construction, the pipeline-cycling state machine,
and the store's patch/replace/history-capping logic. There's no
integration test against a real Azure DevOps org or MQTT broker.

## Building & deploying

Docker images are built by `.github/workflows/azure-build-monitor-docker-publish.yml`
(repo root) on push to `main` or on `azureBuildMonitor-v*` tags, published
to `dsmithson/custom-build-monitor` on Docker Hub. Only triggers on
changes under `azureBuildMonitor/`, so firmware-only commits don't kick
off a Docker build.

```
# Create the PAT secret once, out of band (never commit it):
kubectl create secret generic azure-devops-pat -n buildMonitor \
  --from-literal=AZURE_DEVOPS_PAT=<token>

# Deploy/upgrade:
helm upgrade --install azurebuildmonitor ./helm -n buildMonitor --create-namespace
```

## Possible future improvements

- No HTTP server / health endpoint yet -- fine for now, but would be
  needed for a real liveness/readiness probe if this ever needs one.
- `dimPixelMultiplier` and pipeline cycling order are static; could
  eventually prioritize currently-running pipelines to stay visible
  longer.
