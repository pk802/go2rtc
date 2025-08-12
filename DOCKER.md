# Docker Images for pk802/go2rtc

This repository provides multiple ways to build and use Docker images for your custom go2rtc fork.

## Available Images

After building, your images will be available as:

- `ghcr.io/pk802/go2rtc:latest` - Standard image with FFmpeg
- `ghcr.io/pk802/go2rtc:hardware` - Hardware acceleration support (Intel/AMD)
- `ghcr.io/pk802/go2rtc:rockchip` - Rockchip hardware support

## Method 1: GitHub Actions (Automatic)

The repository includes GitHub Actions workflows that automatically build and publish Docker images to GitHub Container Registry (ghcr.io).

### Setup:

1. **Enable GitHub Actions**: The workflow file is already created at `.github/workflows/docker.yml`

2. **Enable GitHub Packages**: 
   - Go to your repository Settings → Actions → General
   - Under "Workflow permissions", select "Read and write permissions"
   - Check "Allow GitHub Actions to create and approve pull requests"

3. **Trigger Build**: 
   - Push to `master` branch or create a tag
   - Images will be automatically built for multiple architectures
   - Published to `ghcr.io/pk802/go2rtc`

### Usage:
```bash
# Pull and run the latest image
docker run -d \
  --name go2rtc \
  -p 1984:1984 \
  -p 8554:8554 \
  -v ./config:/config \
  ghcr.io/pk802/go2rtc:latest

# Hardware acceleration variant
docker run -d \
  --name go2rtc-hw \
  -p 1984:1984 \
  -p 8554:8554 \
  -v ./config:/config \
  --device /dev/dri:/dev/dri \
  ghcr.io/pk802/go2rtc:hardware
```

## Method 2: Local Build Script

Use the included build script for local development:

```bash
# Build standard image
./build-docker.sh latest

# Build hardware acceleration image
./build-docker.sh hardware docker/hardware.Dockerfile

# Build rockchip image
./build-docker.sh rockchip docker/rockchip.Dockerfile
```

## Method 3: Docker Compose

### Standard deployment:
```bash
# Start standard image
docker-compose up -d go2rtc

# Access at http://localhost:1984
```

### Hardware acceleration:
```bash
# Start hardware-accelerated image
docker-compose --profile hardware up -d go2rtc-hardware

# Access at http://localhost:1985
```

### Rockchip support:
```bash
# Start rockchip image
docker-compose --profile rockchip up -d go2rtc-rockchip

# Access at http://localhost:1986
```

## Method 4: Manual Docker Build

```bash
# Standard image
docker build -f docker/Dockerfile -t pk802/go2rtc:latest .

# Hardware acceleration
docker build -f docker/hardware.Dockerfile -t pk802/go2rtc:hardware .

# Rockchip support
docker build -f docker/rockchip.Dockerfile -t pk802/go2rtc:rockchip .
```

## Configuration

Create a `config` directory with your `go2rtc.yaml`:

```yaml
# config/go2rtc.yaml
api:
  listen: ":1984"

streams:
  camera1: "rtsp://admin:password@192.168.1.100/stream"
  camera2: "rtsp://user:pass@192.168.1.101/h264"

webrtc:
  candidates:
    - "192.168.1.10:8555"  # Your server IP

rtsp:
  listen: ":8554"
```

## Image Variants

### Standard (`latest`)
- Based on Python Alpine
- Includes FFmpeg
- Supports most common use cases
- Smallest size (~150MB)

### Hardware (`hardware`)
- Based on Debian
- Intel QSV, VAAPI, AMD acceleration
- NVIDIA GPU support
- Larger size (~300MB)

### Rockchip (`rockchip`)
- Optimized for Rockchip SoCs
- Custom FFmpeg build
- Hardware encoding/decoding
- ARM64/ARMv7 only

## Usage in Home Assistant

### Option A: Build from Source (addon/)
- Builds go2rtc from your source code
- Full customization
- Longer build times

### Option B: Pre-built Image (addon-prebuilt/)
- Uses your published Docker image
- Faster installation
- Update by pushing new image

## Publishing to Docker Hub

1. **Create Docker Hub account**: https://hub.docker.com

2. **Login locally**:
   ```bash
   docker login
   ```

3. **Tag and push**:
   ```bash
   docker tag pk802/go2rtc:latest pk802/go2rtc:latest
   docker push pk802/go2rtc:latest
   ```

4. **Update image references** in your add-on configs to use `pk802/go2rtc:latest`

## Environment Variables

- `GO2RTC_CONFIG` - Config file path (default: `/config/go2rtc.yaml`)
- `TZ` - Timezone (default: `UTC`)

## Volumes

- `/config` - Configuration directory
- Mount your config file or directory here

## Ports

- `1984` - Web UI and API
- `8554` - RTSP server
- `8555` - Additional RTSP/UDP (if needed)

## Hardware Requirements

### Intel QSV/VAAPI
- Mount `/dev/dri:/dev/dri`
- Use `hardware` image variant

### NVIDIA GPU
- Install nvidia-docker2
- Use `--gpus all` flag
- Use `hardware` image variant

### Rockchip
- Mount device files:
  - `/dev/dri:/dev/dri`
  - `/dev/dma_heap:/dev/dma_heap`
  - `/dev/rga:/dev/rga`
  - `/dev/mpp_service:/dev/mpp_service`
- Use `rockchip` image variant

## Troubleshooting

### Permission Issues
```bash
# Fix config directory permissions
sudo chown -R $(id -u):$(id -g) ./config
```

### Hardware Acceleration Not Working
```bash
# Check devices
ls -la /dev/dri/

# Check container access
docker exec -it go2rtc ls -la /dev/dri/
```

### Image Not Found
```bash
# Build locally first
./build-docker.sh latest

# Or use GitHub-built image
docker pull ghcr.io/pk802/go2rtc:latest
```
