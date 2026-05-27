# syntax=docker/dockerfile:1.7
#
# Multi-stage build for any of Krypton's Go binaries. Pick which one with
# the COMPONENT build-arg, default = manager.
#
#   docker build --build-arg COMPONENT=manager       -t krypton/manager .
#   docker build --build-arg COMPONENT=control-plane -t krypton/control-plane .
#   docker build --build-arg COMPONENT=gateway       -t krypton/gateway .
#   docker build --build-arg COMPONENT=krypton-proxy -t krypton/krypton-proxy .
#
# The control-plane image is the only one that needs the embedded UI;
# we run `make ui` first when building it. For other components the UI
# step is a cheap no-op.

ARG COMPONENT=manager

# ---- UI build (node) -------------------------------------------------------

FROM node:22-alpine AS ui
WORKDIR /src
COPY ui/package.json ui/pnpm-lock.yaml ./ui/
RUN corepack enable && cd ui && pnpm install --frozen-lockfile
COPY ui ./ui
RUN cd ui && pnpm build

# ---- Go build --------------------------------------------------------------
#
# Pin to $BUILDPLATFORM so buildx runs this stage on the runner's
# native arch (no QEMU emulation). Go cross-compiles to $TARGETARCH
# natively — multi-arch builds (linux/amd64 + linux/arm64) emit both
# binaries from one amd64 runner in seconds.

FROM --platform=$BUILDPLATFORM golang:1.25 AS build
ARG COMPONENT
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Stage the UI dist into the embed path that the control plane reads via
# go:embed. Other components ignore it.
COPY --from=ui /src/ui/dist ./internal/controlplane/embed/dist
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" \
        -o /out/app ./cmd/${COMPONENT}

# ---- Runtime ---------------------------------------------------------------

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/app /app
USER nonroot
ENTRYPOINT ["/app"]
