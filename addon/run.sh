#!/usr/bin/with-contenv bashio
# shellcheck shell=bash
set -e

# Parse inputs from options
CONFIG_PATH="/data/options.json"
GO2RTC_CONFIG="/data/go2rtc.yaml"

# Generate go2rtc configuration from Home Assistant add-on options
bashio::log.info "Generating go2rtc configuration..."

# Create configuration from add-on options
{
    echo "# go2rtc configuration generated from Home Assistant add-on"
    echo ""
    
    # Log level
    if bashio::config.has_value 'log_level'; then
        echo "log:"
        echo "  level: $(bashio::config 'log_level')"
        echo ""
    fi
    
    # API configuration
    echo "api:"
    echo "  listen: \"$(bashio::config 'api.listen')\""
    echo "  origin: \"$(bashio::config 'api.origin')\""
    
    if bashio::config.has_value 'api.username'; then
        echo "  username: \"$(bashio::config 'api.username')\""
        echo "  password: \"$(bashio::config 'api.password')\""
    fi
    
    echo ""
    
    # WebRTC configuration
    echo "webrtc:"
    if bashio::config.has_value 'webrtc.candidates'; then
        echo "  candidates:"
        bashio::config 'webrtc.candidates' | jq -r '.[]' | while read -r candidate; do
            [[ -n "$candidate" ]] && echo "    - \"${candidate}\""
        done
    fi
    
    echo ""
    
    # RTSP configuration
    echo "rtsp:"
    echo "  listen: \"$(bashio::config 'rtsp.listen')\""
    
    if bashio::config.has_value 'rtsp.username'; then
        echo "  username: \"$(bashio::config 'rtsp.username')\""
        echo "  password: \"$(bashio::config 'rtsp.password')\""
    fi
    
    echo ""
    
    # FFmpeg configuration
    echo "ffmpeg:"
    echo "  bin: ffmpeg"
    
    echo ""
    
    # Streams configuration
    echo "streams:"
    if bashio::config.has_value 'streams'; then
        bashio::config 'streams' | jq -r 'to_entries[] | "  " + .key + ": \"" + .value + "\""'
    fi
    
} > "${GO2RTC_CONFIG}"

bashio::log.info "Starting go2rtc..."

# Start go2rtc
exec go2rtc -config "${GO2RTC_CONFIG}"
