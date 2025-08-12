# syntax=docker/dockerfile:labs

# 0. Prepare images
# Try trixie for latest ffmpeg, fallback to bookworm if needed
ARG DEBIAN_VERSION="trixie-slim"
ARG GO_VERSION="1.24-bookworm"


# 1. Build go2rtc binary
FROM --platform=$BUILDPLATFORM golang:${GO_VERSION} AS build
ARG TARGETPLATFORM
ARG TARGETOS
ARG TARGETARCH

ENV GOOS=${TARGETOS}
ENV GOARCH=${TARGETARCH}

WORKDIR /build

# Cache dependencies
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/root/.cache/go-build go mod download

COPY . .
RUN --mount=type=cache,target=/root/.cache/go-build CGO_ENABLED=0 go build -ldflags "-s -w" -trimpath


# 2. Final image
FROM debian:${DEBIAN_VERSION}

# Prepare apt for buildkit cache
RUN rm -f /etc/apt/apt.conf.d/docker-clean \
  && echo 'Binary::apt::APT::Keep-Downloaded-Packages "true";' >/etc/apt/apt.conf.d/keep-cache

# Install ffmpeg, tini (for signal handling),
# and other common tools for the echo source.
# Try to install Intel drivers, fallback to generic if not available
RUN --mount=type=cache,target=/var/cache/apt,sharing=locked --mount=type=cache,target=/var/lib/apt,sharing=locked \
    echo 'deb http://deb.debian.org/debian trixie non-free-firmware' >> /etc/apt/sources.list && \
    echo 'deb http://deb.debian.org/debian trixie non-free' >> /etc/apt/sources.list && \
    apt-get -y update && \
    apt-get -y install --no-install-recommends ffmpeg tini python3 curl jq libasound2-plugins && \
    (apt-get -y install --no-install-recommends intel-media-va-driver-non-free || \
     apt-get -y install --no-install-recommends va-driver-all || true) && \
    (apt-get -y install --no-install-recommends mesa-va-drivers || true) && \
    apt-get clean && rm -rf /var/lib/apt/lists/*

COPY --from=build /build/go2rtc /usr/local/bin/

ENTRYPOINT ["/usr/bin/tini", "--"]
VOLUME /config
WORKDIR /config
# https://github.com/NVIDIA/nvidia-docker/wiki/Installation-(Native-GPU-Support)
ENV NVIDIA_VISIBLE_DEVICES all
ENV NVIDIA_DRIVER_CAPABILITIES compute,video,utility

CMD ["go2rtc", "-config", "/config/go2rtc.yaml"]
