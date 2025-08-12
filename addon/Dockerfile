ARG BUILD_FROM
FROM ${BUILD_FROM}

# Set shell
SHELL ["/bin/bash", "-o", "pipefail", "-c"]

# Install dependencies for building Go application
RUN apk add --no-cache \
    go \
    git \
    musl-dev \
    gcc \
    ffmpeg \
    ffmpeg-dev \
    alsa-lib-dev \
    v4l-utils-dev \
    jq \
    curl \
    ca-certificates \
    bash \
    && rm -rf /var/cache/apk/*

# Set Go environment variables
ENV CGO_ENABLED=1
ENV GOOS=linux

# Set working directory
WORKDIR /build

# Copy go.mod and go.sum first (for better caching)
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN go build -ldflags="-s -w" -o go2rtc .

# Install the binary to system path
RUN mv go2rtc /usr/local/bin/go2rtc && \
    chmod +x /usr/local/bin/go2rtc

# Create data directory and set permissions
RUN mkdir -p /data /share && \
    chmod 755 /data /share

# Set default config location
ENV GO2RTC_CONFIG="/data/go2rtc.yaml"

# Labels for Home Assistant addon
LABEL \
    io.hass.name="go2rtc" \
    io.hass.description="Ultimate camera streaming application with support for RTSP, RTMP, HTTP-FLV, WebRTC, MSE, HLS, MP4, MJPEG, HomeKit, FFmpeg, etc." \
    io.hass.arch="${BUILD_ARCH}" \
    io.hass.type="addon" \
    io.hass.version=${BUILD_VERSION} \
    maintainer="pk802" \
    org.opencontainers.image.title="go2rtc" \
    org.opencontainers.image.description="Ultimate camera streaming application" \
    org.opencontainers.image.vendor="pk802" \
    org.opencontainers.image.authors="pk802" \
    org.opencontainers.image.licenses="MIT" \
    org.opencontainers.image.url="https://github.com/pk802/go2rtc" \
    org.opencontainers.image.source="https://github.com/pk802/go2rtc" \
    org.opencontainers.image.documentation="https://github.com/pk802/go2rtc/blob/master/README.md" \
    org.opencontainers.image.created=${BUILD_DATE} \
    org.opencontainers.image.revision=${BUILD_REF} \
    org.opencontainers.image.version=${BUILD_VERSION}

# Expose ports
EXPOSE 1984 8554 8555

# Copy run script
COPY addon/run.sh /
RUN chmod a+x /run.sh

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:1984/api/info || exit 1

# Run
CMD ["/run.sh"]
