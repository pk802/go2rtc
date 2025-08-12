# syntax=docker/dockerfile:labs

# 0. Prepare images
ARG PYTHON_VERSION="3.11"
ARG GO_VERSION="1.24"

# 1. Build go2rtc binary
FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-alpine AS build
ARG TARGETPLATFORM
ARG TARGETOS
ARG TARGETARCH

ENV GOOS=${TARGETOS}
ENV GOARCH=${TARGETARCH}

WORKDIR /build

RUN apk add git

# Cache dependencies
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/root/.cache/go-build go mod download

COPY . .
RUN --mount=type=cache,target=/root/.cache/go-build CGO_ENABLED=0 go build -ldflags "-s -w" -trimpath

# 2. Final image with hardware acceleration
FROM python:${PYTHON_VERSION}-slim

# Prepare apt for buildkit cache
RUN rm -f /etc/apt/apt.conf.d/docker-clean \
  && echo 'Binary::apt::APT::Keep-Downloaded-Packages "true";' >/etc/apt/apt.conf.d/keep-cache

# Install ffmpeg, tini, and hardware acceleration drivers
RUN --mount=type=cache,target=/var/cache/apt,sharing=locked --mount=type=cache,target=/var/lib/apt,sharing=locked \
    apt-get -y update && apt-get -y install --no-install-recommends \
        ffmpeg tini \
        curl jq \
        va-driver-all \
        libasound2-plugins && \
    apt-get clean && rm -rf /var/lib/apt/lists/*

COPY --from=build /build/go2rtc /usr/local/bin/

ENTRYPOINT ["/usr/bin/tini", "--"]
VOLUME /config
WORKDIR /config

# https://github.com/NVIDIA/nvidia-docker/wiki/Installation-(Native-GPU-Support)
ENV NVIDIA_VISIBLE_DEVICES all
ENV NVIDIA_DRIVER_CAPABILITIES compute,video,utility

CMD ["go2rtc", "-config", "/config/go2rtc.yaml"]
