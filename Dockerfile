# syntax=docker/dockerfile:1.7

# SPDX-License-Identifier: Apache-2.0
# Copyright Daniel Neumann

ARG GO_VERSION=1.26

FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-alpine AS builder
WORKDIR /src

ARG GOOS=linux
ARG GOARCH=amd64
ARG TARGETOS
ARG TARGETARCH

COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal

# Build a fully static Linux binary suitable for distroless/scratch runtimes.
RUN CGO_ENABLED=0 GOOS="${TARGETOS:-$GOOS}" GOARCH="${TARGETARCH:-$GOARCH}" \
  go build -trimpath -ldflags="-s -w" -o /out/cast-off ./cmd/cast-off

# Preferred runtime: Chainguard static image (distroless, minimal, non-root by default).
FROM cgr.dev/chainguard/static:latest

COPY --from=builder /out/cast-off /cast-off

USER 65532:65532
ENTRYPOINT ["/cast-off"]
