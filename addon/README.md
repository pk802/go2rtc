# go2rtc Home Assistant Add-on

Ultimate camera streaming application with support for RTSP, RTMP, HTTP-FLV, WebRTC, MSE, HLS, MP4, MJPEG, HomeKit, FFmpeg, etc.

![Supports aarch64 Architecture][aarch64-shield]
![Supports amd64 Architecture][amd64-shield]
![Supports armhf Architecture][armhf-shield]
![Supports armv7 Architecture][armv7-shield]
![Supports i386 Architecture][i386-shield]

[aarch64-shield]: https://img.shields.io/badge/aarch64-yes-green.svg
[amd64-shield]: https://img.shields.io/badge/amd64-yes-green.svg
[armhf-shield]: https://img.shields.io/badge/armhf-yes-green.svg
[armv7-shield]: https://img.shields.io/badge/armv7-yes-green.svg
[i386-shield]: https://img.shields.io/badge/i386-yes-green.svg

## About

go2rtc is a camera streaming application that supports multiple protocols and formats. This add-on builds go2rtc from your custom fork and integrates it with Home Assistant.

Key features:
- **Multiple Protocols**: RTSP, RTMP, HTTP-FLV, WebRTC, MSE, HLS, MP4, MJPEG
- **HomeKit Support**: Stream cameras to Apple devices
- **FFmpeg Integration**: Hardware transcoding support
- **Two-way Audio**: Bidirectional communication with cameras
- **Zero-config**: Automatic camera discovery and setup
- **Multi-source**: Combine multiple camera sources

## Installation

### Method 1: Add Repository to Home Assistant

1. Navigate to **Supervisor** → **Add-on Store** in Home Assistant
2. Click the menu (⋮) in the top right corner
3. Select **Repositories**
4. Add this repository URL: `https://github.com/pk802/go2rtc`
5. Click **Add**
6. Find "go2rtc" in the add-on store and click **Install**

### Method 2: Local Installation

1. Copy the `addon` folder to your Home Assistant addons directory:
   ```bash
   cp -r addon /usr/share/hassio/addons/local/go2rtc
   ```
2. Navigate to **Supervisor** → **Add-on Store** → **Local add-ons**
3. Find "go2rtc" and click **Install**

## Configuration

### Basic Configuration

```yaml
log_level: info
streams:
  camera1: "rtsp://admin:password@192.168.1.100/stream"
  camera2: "rtsp://user:pass@192.168.1.101/h264"
api:
  origin: "*"
webrtc:
  candidates: []
rtsp:
  listen: ":8554"
```

### Advanced Configuration

```yaml
log_level: debug
streams:
  front_door:
    - "rtsp://admin:password@192.168.1.100/stream"
    - "ffmpeg:rtsp://admin:password@192.168.1.100/stream#video=copy#audio=opus"
  backyard:
    - "rtsp://admin:password@192.168.1.101/stream"
  virtual_camera:
    - "ffmpeg:device?video=0&audio=1#video=h264#audio=opus"

api:
  listen: ":1984"
  origin: "*"
  username: "admin"
  password: "secret"

webrtc:
  candidates:
    - "192.168.1.10:8555"  # Your Home Assistant IP
  ice_servers:
    - urls: "stun:stun.l.google.com:19302"

rtsp:
  listen: ":8554"
  username: "viewer"
  password: "password"

ffmpeg:
  bin: "ffmpeg"
  global: "-hide_banner -loglevel error"

homekit:
  pin: "12345678"
  listen: ":8080"

mqtt:
  username: "mqtt_user"
  password: "mqtt_pass"
```

## Configuration Options

### General

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `log_level` | string | `info` | Log level (trace, debug, info, warn, error, fatal, panic) |
| `streams` | dict | `{}` | Camera stream configurations |

### API

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `api.listen` | string | `:1984` | API server listen address |
| `api.origin` | string | `*` | CORS origin policy |
| `api.username` | string | - | Basic auth username |
| `api.password` | string | - | Basic auth password |
| `api.static_dir` | string | - | Custom static files directory |
| `api.base_path` | string | - | API base path |

### WebRTC

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `webrtc.candidates` | list | `[]` | ICE candidates (auto-detected if empty) |
| `webrtc.ice_servers` | list | - | Custom ICE servers |
| `webrtc.listen` | string | - | WebRTC listen address |

### RTSP

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `rtsp.listen` | string | `:8554` | RTSP server listen address |
| `rtsp.username` | string | - | RTSP authentication username |
| `rtsp.password` | string | - | RTSP authentication password |

### FFmpeg

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `ffmpeg.bin` | string | `ffmpeg` | FFmpeg binary path |
| `ffmpeg.global` | string | - | Global FFmpeg arguments |
| `ffmpeg.file` | string | - | File input arguments |
| `ffmpeg.http` | string | - | HTTP input arguments |
| `ffmpeg.rtsp` | string | - | RTSP input arguments |

### HomeKit

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `homekit.pin` | string | - | HomeKit setup PIN |
| `homekit.listen` | string | - | HomeKit server address |
| `homekit.public_ip` | string | - | Public IP for HomeKit |

## Stream Configuration Examples

### Basic RTSP Stream
```yaml
streams:
  camera1: "rtsp://admin:password@192.168.1.100/stream"
```

### Multiple Sources
```yaml
streams:
  multi_source:
    - "rtsp://admin:password@192.168.1.100/stream"
    - "ffmpeg:rtsp://admin:password@192.168.1.100/stream#audio=opus"
```

### Transcoding with FFmpeg
```yaml
streams:
  transcoded:
    - "ffmpeg:rtsp://192.168.1.100/stream#video=h264#audio=aac"
```

### USB Camera
```yaml
streams:
  usb_camera:
    - "ffmpeg:device?video=0#video=h264"
```

### RTMP Stream
```yaml
streams:
  rtmp_stream:
    - "rtmp://192.168.1.100/live/stream"
```

## Usage

### Accessing the Web Interface

Once the add-on is running, you can access the web interface at:
- http://homeassistant.local:1984
- http://[HOME_ASSISTANT_IP]:1984

### Integration with Home Assistant

1. **WebRTC Camera Integration**: Use the built-in WebRTC integration with streams from go2rtc
2. **Generic Camera**: Add cameras using the RTSP URLs from go2rtc
3. **Lovelace Cards**: Use Picture Entity or Picture Glance cards

### RTSP URLs

Access your streams via RTSP at:
- `rtsp://homeassistant.local:8554/[stream_name]`
- `rtsp://[HOME_ASSISTANT_IP]:8554/[stream_name]`

## Troubleshooting

### Common Issues

1. **Cannot access web interface**
   - Check if port 1984 is not blocked
   - Verify the add-on is running in the Supervisor

2. **Camera not streaming**
   - Check camera URL and credentials
   - Verify network connectivity
   - Check logs for error messages

3. **WebRTC not working**
   - Configure proper ICE candidates
   - Check firewall settings
   - Ensure UDP ports are accessible

### Logs

View logs in the Home Assistant Supervisor:
1. Go to **Supervisor** → **go2rtc**
2. Click on the **Log** tab

Enable debug logging:
```yaml
log_level: debug
```

### Network Configuration

For external access, configure your router:
- Port 1984 (TCP) - Web interface
- Port 8554 (TCP) - RTSP server
- Port 8555 (UDP) - WebRTC candidates

## Support

- **Issues**: [GitHub Issues](https://github.com/pk802/go2rtc/issues)
- **Documentation**: [go2rtc Documentation](https://github.com/AlexxIT/go2rtc)
- **Home Assistant Community**: [Community Forum](https://community.home-assistant.io)

## License

MIT License - see [LICENSE](https://github.com/pk802/go2rtc/blob/master/LICENSE) for details.
