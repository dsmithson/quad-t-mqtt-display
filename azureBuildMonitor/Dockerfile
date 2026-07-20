############################
# STEP 1: build executable binary (per-arch)
############################
FROM golang:1.25 AS builder
WORKDIR /app

# Let Buildx tell us the target platform
ARG TARGETOS
ARG TARGETARCH

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -ldflags="-s -w" -o /out/app ./cmd/azureBuildMonitor

############################
# STEP 2: minimal runtime (per-arch)
############################
# Pure Go binary, no native deps -- distroless static is enough, and much
# smaller than a full OS base.
FROM --platform=$TARGETPLATFORM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /out/app /app

ENTRYPOINT ["/app"]
