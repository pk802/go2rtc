# go2rtc WebRTC Pause/Resume API Integration

## Overview

This document covers both **client-controlled** (viewer ID based) and **server-controlled** (session ID based) pause/resume functionality for go2rtc WebRTC streams.

### Key Differences:
- **Client-controlled**: Browser generates viewer ID, suitable for user-initiated controls
- **Server-controlled**: Server generates session ID, perfect for programmatic control and automation

## HTTP API Endpoints

### Base URL
```
http://localhost:1984
```

### Client-Controlled Endpoints (Legacy)

#### Pause Stream by Viewer ID
```http
POST /api/webrtc/pause
Content-Type: application/json

{
  "action": "pause",
  "viewer_id": "optional_viewer_id"
}
```

#### Resume Stream by Viewer ID
```http
POST /api/webrtc/resume
Content-Type: application/json

{
  "action": "resume", 
  "viewer_id": "optional_viewer_id"
}
```

### Server-Controlled Endpoints (Recommended)

#### List Active Sessions
```http
GET /api/webrtc/sessions
```

#### Pause Stream by Session ID
```http
POST /api/webrtc/session/pause
Content-Type: application/json

{
  "session_id": "required_session_id"
}
```

#### Resume Stream by Session ID
```http
POST /api/webrtc/session/resume
Content-Type: application/json

{
  "session_id": "required_session_id"
}
```

## JavaScript Examples

### 1. Server-Controlled Session Management (Recommended)

```javascript
// Get all active sessions
async function getActiveSessions() {
  const response = await fetch('/api/webrtc/sessions');
  return response.json();
}

// Pause specific session
async function pauseSession(sessionId) {
  const response = await fetch('/api/webrtc/session/pause', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ session_id: sessionId })
  });
  return response.json();
}

// Resume specific session  
async function resumeSession(sessionId) {
  const response = await fetch('/api/webrtc/session/resume', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ session_id: sessionId })
  });
  return response.json();
}

// Session Management Class
class SessionManager {
  constructor() {
    this.sessions = new Map();
  }

  async refreshSessions() {
    const data = await getActiveSessions();
    this.sessions.clear();
    data.sessions.forEach(session => {
      this.sessions.set(session.session_id, session);
    });
    return this.sessions;
  }

  async pauseAllSessions() {
    const results = [];
    for (const [sessionId] of this.sessions) {
      results.push(await pauseSession(sessionId));
    }
    return results;
  }

  async resumeAllSessions() {
    const results = [];
    for (const [sessionId] of this.sessions) {
      results.push(await resumeSession(sessionId));
    }
    return results;
  }

  getSessionsByStream(streamSource) {
    return Array.from(this.sessions.values())
      .filter(session => session.stream_source === streamSource);
  }
}

// Usage
const manager = new SessionManager();
await manager.refreshSessions();

// Pause all sessions for a specific camera
const cabinSessions = manager.getSessionsByStream('pk_cabin_camera');
for (const session of cabinSessions) {
  await pauseSession(session.session_id);
  console.log(`Paused session ${session.session_id}`);
}
```

### 2. WebRTC Client with Session Capture

```javascript
// Extend video element to capture session ID from server
class SessionAwareVideo extends HTMLElement {
  constructor() {
    super();
    this.sessionId = null;
  }

  connectedCallback() {
    // Create video-stream element
    this.innerHTML = '<video-stream></video-stream>';
    const videoStream = this.querySelector('video-stream');
    
    // Listen for server-provided session ID
    videoStream.addEventListener('sessionreceived', (event) => {
      this.sessionId = event.detail.sessionID;
      console.log('📦 Session captured:', this.sessionId);
      
      // Trigger custom event with session ID
      this.dispatchEvent(new CustomEvent('sessionready', {
        detail: { sessionId: this.sessionId }
      }));
    });
    
    // Set stream source
    if (this.hasAttribute('src')) {
      videoStream.src = this.getAttribute('src') + '&mode=webrtc';
    }
  }

  async pause() {
    if (!this.sessionId) {
      throw new Error('Session ID not available yet');
    }
    return pauseSession(this.sessionId);
  }

  async resume() {
    if (!this.sessionId) {
      throw new Error('Session ID not available yet');
    }
    return resumeSession(this.sessionId);
  }
}

// Register custom element
customElements.define('session-video', SessionAwareVideo);

// Usage in HTML
// <session-video src="api/ws?src=pk_cabin_camera"></session-video>

// Usage in JavaScript
const video = document.querySelector('session-video');
video.addEventListener('sessionready', async (event) => {
  console.log('Session ready:', event.detail.sessionId);
  
  // Now you can control this specific session
  setTimeout(() => video.pause(), 5000);   // Auto-pause after 5s
  setTimeout(() => video.resume(), 10000); // Auto-resume after 10s
});
```

### 3. Basic Pause/Resume Functions (Legacy)
```javascript
// Pause specific viewer
async function pauseViewer(viewerId) {
  const response = await fetch('/api/webrtc/pause', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      action: 'pause',
      viewer_id: viewerId
    })
  });
  return response.json();
}

// Resume specific viewer
async function resumeViewer(viewerId) {
  const response = await fetch('/api/webrtc/resume', {
    method: 'POST', 
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      action: 'resume',
      viewer_id: viewerId
    })
  });
  return response.json();
}

// Pause ALL viewers (global)
async function pauseAll() {
  const response = await fetch('/api/webrtc/pause', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ action: 'pause' })
  });
  return response.json();
}
```

### 2. React Hook Example
```javascript
import { useState, useCallback } from 'react';

function useWebRTCControl(viewerId) {
  const [isPaused, setIsPaused] = useState(false);
  const [isLoading, setIsLoading] = useState(false);

  const pause = useCallback(async () => {
    setIsLoading(true);
    try {
      const result = await fetch('/api/webrtc/pause', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          action: 'pause',
          viewer_id: viewerId
        })
      });
      const data = await result.json();
      if (data.success) {
        setIsPaused(true);
      }
      return data;
    } finally {
      setIsLoading(false);
    }
  }, [viewerId]);

  const resume = useCallback(async () => {
    setIsLoading(true);
    try {
      const result = await fetch('/api/webrtc/resume', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          action: 'resume',
          viewer_id: viewerId
        })
      });
      const data = await result.json();
      if (data.success) {
        setIsPaused(false);
      }
      return data;
    } finally {
      setIsLoading(false);
    }
  }, [viewerId]);

  return { isPaused, isLoading, pause, resume };
}
```

### 3. MQTT Integration Example
```javascript
import mqtt from 'mqtt';

class MotionControlledStream {
  constructor(mqttBroker, viewerId) {
    this.viewerId = viewerId;
    this.client = mqtt.connect(mqttBroker);
    
    // Listen for Frigate motion events
    this.client.on('message', (topic, message) => {
      const data = JSON.parse(message.toString());
      
      if (topic.includes('motion')) {
        if (data.motion) {
          this.resumeStream();
        } else {
          this.pauseStream();
        }
      }
    });
    
    // Subscribe to motion topics
    this.client.subscribe('frigate/+/motion');
  }

  async pauseStream() {
    console.log('🔴 Motion stopped - pausing stream');
    return fetch('/api/webrtc/pause', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        action: 'pause',
        viewer_id: this.viewerId
      })
    });
  }

  async resumeStream() {
    console.log('🟢 Motion detected - resuming stream');
    return fetch('/api/webrtc/resume', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        action: 'resume',
        viewer_id: this.viewerId
      })
    });
  }
}

// Usage
const motionControl = new MotionControlledStream(
  'mqtt://192.168.1.100:1883',
  'viewer_cabin_camera_001'
);
```

## Python Examples

### 1. Server-Controlled Session Management (Recommended)

```python
import requests
import json
from typing import List, Dict, Optional

class Go2RTCSessionController:
    def __init__(self, base_url="http://localhost:1984"):
        self.base_url = base_url
    
    def get_active_sessions(self) -> Dict:
        """Get all active WebRTC sessions"""
        response = requests.get(f"{self.base_url}/api/webrtc/sessions")
        return response.json()
    
    def pause_session(self, session_id: str) -> Dict:
        """Pause a specific session"""
        response = requests.post(
            f"{self.base_url}/api/webrtc/session/pause",
            headers={"Content-Type": "application/json"},
            json={"session_id": session_id}
        )
        return response.json()
    
    def resume_session(self, session_id: str) -> Dict:
        """Resume a specific session"""
        response = requests.post(
            f"{self.base_url}/api/webrtc/session/resume",
            headers={"Content-Type": "application/json"},
            json={"session_id": session_id}
        )
        return response.json()
    
    def pause_all_sessions(self) -> List[Dict]:
        """Pause all active sessions"""
        sessions = self.get_active_sessions()
        results = []
        for session in sessions.get('sessions', []):
            result = self.pause_session(session['session_id'])
            results.append(result)
        return results
    
    def resume_all_sessions(self) -> List[Dict]:
        """Resume all active sessions"""
        sessions = self.get_active_sessions()
        results = []
        for session in sessions.get('sessions', []):
            result = self.resume_session(session['session_id'])
            results.append(result)
        return results
    
    def get_sessions_by_stream(self, stream_source: str) -> List[Dict]:
        """Get sessions for a specific stream source"""
        sessions = self.get_active_sessions()
        return [
            session for session in sessions.get('sessions', [])
            if session.get('stream_source') == stream_source
        ]
    
    def pause_stream_sessions(self, stream_source: str) -> List[Dict]:
        """Pause all sessions for a specific stream"""
        sessions = self.get_sessions_by_stream(stream_source)
        results = []
        for session in sessions:
            result = self.pause_session(session['session_id'])
            results.append(result)
        return results

# Usage Example
controller = Go2RTCSessionController()

# List all active sessions
sessions_data = controller.get_active_sessions()
print(f"Active sessions: {sessions_data['count']}")

for session in sessions_data['sessions']:
    print(f"Session {session['session_id'][:8]}... "
          f"- Stream: {session['stream_source']} "
          f"- Paused: {session['paused']}")

# Pause all sessions for a specific camera
cabin_sessions = controller.get_sessions_by_stream('pk_cabin_camera')
if cabin_sessions:
    results = controller.pause_stream_sessions('pk_cabin_camera')
    print(f"Paused {len(results)} sessions for pk_cabin_camera")

# Resume all sessions
all_results = controller.resume_all_sessions()
print(f"Resumed {len(all_results)} sessions")
```

### 2. MQTT + Session-Based Motion Control

```python
import paho.mqtt.client as mqtt
import requests
import json
import time
from threading import Timer
from typing import Dict, Set

class SessionBasedMotionController:
    def __init__(self, mqtt_broker: str, go2rtc_url="http://localhost:1984"):
        self.go2rtc_url = go2rtc_url
        self.session_controller = Go2RTCSessionController(go2rtc_url)
        
        # Track motion state per camera
        self.motion_timers: Dict[str, Timer] = {}
        self.idle_timeout = 30  # seconds
        
        # MQTT setup
        self.client = mqtt.Client()
        self.client.on_connect = self.on_connect
        self.client.on_message = self.on_message
        self.client.connect(mqtt_broker, 1883, 60)
        
    def on_connect(self, client, userdata, flags, rc):
        print(f"Connected to MQTT broker with result code {rc}")
        # Subscribe to Frigate events
        client.subscribe("frigate/events")
        client.subscribe("frigate/+/motion")
        
    def on_message(self, client, userdata, msg):
        try:
            data = json.loads(msg.payload.decode())
            
            if 'frigate/events' in msg.topic:
                self.handle_frigate_event(data)
            elif '/motion' in msg.topic:
                camera_name = msg.topic.split('/')[1]
                self.handle_motion_event(camera_name, data.get('motion', False))
                
        except Exception as e:
            print(f"Error processing MQTT message: {e}")
    
    def handle_frigate_event(self, event_data):
        """Handle Frigate event for motion detection"""
        event_type = event_data.get('type')
        camera = event_data.get('after', {}).get('camera')
        
        if event_type == 'new' and camera:
            print(f"🟢 Motion started on {camera}")
            self.resume_camera_sessions(camera)
            self.set_idle_timer(camera)
            
        elif event_type == 'end' and camera:
            print(f"🔴 Motion ended on {camera}")
            self.set_idle_timer(camera)
    
    def handle_motion_event(self, camera: str, motion_active: bool):
        """Handle direct motion state changes"""
        if motion_active:
            print(f"🟢 Motion detected on {camera}")
            self.resume_camera_sessions(camera)
            self.cancel_idle_timer(camera)
        else:
            print(f"🔴 No motion on {camera}")
            self.set_idle_timer(camera)
    
    def set_idle_timer(self, camera: str):
        """Set timer to pause stream after idle timeout"""
        self.cancel_idle_timer(camera)
        
        def timeout_callback():
            print(f"⏰ Idle timeout for {camera} - pausing sessions")
            self.pause_camera_sessions(camera)
        
        self.motion_timers[camera] = Timer(self.idle_timeout, timeout_callback)
        self.motion_timers[camera].start()
    
    def cancel_idle_timer(self, camera: str):
        """Cancel idle timer for camera"""
        if camera in self.motion_timers:
            self.motion_timers[camera].cancel()
            del self.motion_timers[camera]
    
    def resume_camera_sessions(self, camera: str):
        """Resume all sessions for a camera"""
        try:
            sessions = self.session_controller.get_sessions_by_stream(camera)
            for session in sessions:
                if session.get('paused', False):
                    result = self.session_controller.resume_session(session['session_id'])
                    if result.get('success'):
                        print(f"▶️ Resumed session {session['session_id'][:8]}... for {camera}")
        except Exception as e:
            print(f"Error resuming sessions for {camera}: {e}")
    
    def pause_camera_sessions(self, camera: str):
        """Pause all sessions for a camera"""
        try:
            sessions = self.session_controller.get_sessions_by_stream(camera)
            for session in sessions:
                if not session.get('paused', False):
                    result = self.session_controller.pause_session(session['session_id'])
                    if result.get('success'):
                        print(f"⏸️ Paused session {session['session_id'][:8]}... for {camera}")
        except Exception as e:
            print(f"Error pausing sessions for {camera}: {e}")
    
    def start(self):
        """Start the motion controller"""
        print("Starting session-based motion controller...")
        self.client.loop_forever()

# Usage
controller = SessionBasedMotionController("192.168.1.100")
controller.start()
```

### 3. Basic Control (Legacy)
```python
import requests
import json

class Go2RTCController:
    def __init__(self, base_url="http://localhost:1984"):
        self.base_url = base_url
    
    def pause_viewer(self, viewer_id=None):
        payload = {"action": "pause"}
        if viewer_id:
            payload["viewer_id"] = viewer_id
            
        response = requests.post(
            f"{self.base_url}/api/webrtc/pause",
            headers={"Content-Type": "application/json"},
            json=payload
        )
        return response.json()
    
    def resume_viewer(self, viewer_id=None):
        payload = {"action": "resume"}
        if viewer_id:
            payload["viewer_id"] = viewer_id
            
        response = requests.post(
            f"{self.base_url}/api/webrtc/resume", 
            headers={"Content-Type": "application/json"},
            json=payload
        )
        return response.json()

# Usage
controller = Go2RTCController()

# Pause specific viewer
result = controller.pause_viewer("viewer_1234567890_abc123")
print(f"Paused {result['paused_connections']} connections")

# Resume all viewers
result = controller.resume_viewer()
print(f"Resumed {result['resumed_connections']} connections")
```

### 2. MQTT + Motion Detection
```python
import paho.mqtt.client as mqtt
import requests
import json

class FrigateMotionController:
    def __init__(self, mqtt_broker, go2rtc_url="http://localhost:1984"):
        self.go2rtc_url = go2rtc_url
        self.client = mqtt.Client()
        self.client.on_connect = self.on_connect
        self.client.on_message = self.on_message
        self.client.connect(mqtt_broker, 1883, 60)
        
    def on_connect(self, client, userdata, flags, rc):
        print(f"Connected to MQTT broker with result code {rc}")
        # Subscribe to Frigate motion events
        client.subscribe("frigate/+/motion")
        
    def on_message(self, client, userdata, msg):
        topic_parts = msg.topic.split('/')
        camera_name = topic_parts[1]
        
        try:
            data = json.loads(msg.payload.decode())
            if data.get('motion'):
                print(f"🟢 Motion detected on {camera_name}")
                self.resume_stream(f"viewer_{camera_name}")
            else:
                print(f"🔴 Motion stopped on {camera_name}")
                self.pause_stream(f"viewer_{camera_name}")
        except Exception as e:
            print(f"Error processing message: {e}")
    
    def pause_stream(self, viewer_id):
        requests.post(
            f"{self.go2rtc_url}/api/webrtc/pause",
            headers={"Content-Type": "application/json"},
            json={"action": "pause", "viewer_id": viewer_id}
        )
    
    def resume_stream(self, viewer_id):
        requests.post(
            f"{self.go2rtc_url}/api/webrtc/resume",
            headers={"Content-Type": "application/json"},
            json={"action": "resume", "viewer_id": viewer_id}
        )
    
    def start(self):
        self.client.loop_forever()

# Usage
controller = FrigateMotionController("192.168.1.100")
controller.start()
```

## API Response Format

### Session-Based Responses (Recommended)

#### Session List Response
```json
{
  "sessions": [
    {
      "session_id": "a1b2c3d4e5f6789012345678901234567890abcd",
      "connection_id": 1,
      "stream_source": "pk_cabin_camera",
      "viewer_id": "viewer_1754925523259_htwpbqoa9",
      "mode": "passive consumer",
      "paused": false
    }
  ],
  "count": 1
}
```

#### Session Control Success Response
```json
{
  "action": "pause",
  "success": true,
  "session_id": "a1b2c3d4e5f6789012345678901234567890abcd"
}
```

#### Session Control Error Response
```json
{
  "error": "Session not found",
  "status": 404
}
```

### Viewer-Based Responses (Legacy)

#### Success Response
```json
{
  "action": "pause",
  "success": true,
  "paused_connections": 1,
  "viewer_id": "viewer_1234567890_abc123"
}
```

#### Error Response  
```json
{
  "action": "pause",
  "success": false,
  "paused_connections": 0,
  "viewer_id": "viewer_1234567890_abc123",
  "message": "No active consumer connections found for viewer: viewer_1234567890_abc123"
}
```

## Integration Notes

### Server-Controlled Sessions (Recommended)
1. **Session ID Format**: Server-generated 32-character hex string (e.g., `a1b2c3d4e5f6789012345678901234567890abcd`)
2. **Session Lifecycle**: Created during WebRTC signaling, tracked until connection closes
3. **Automatic Assignment**: No client-side configuration needed - server handles everything
4. **Perfect for Automation**: Ideal for motion detection, scheduled pauses, bandwidth management
5. **Session Persistence**: Session ID remains valid for entire WebRTC connection lifetime
6. **Zero Configuration**: Works out-of-the-box with any WebRTC client

### Client-Controlled Viewers (Legacy)
1. **Viewer ID Format**: Client-generated as `viewer_{timestamp}_{random}` in the web UI
2. **Client Dependency**: Requires client-side JavaScript to generate and send viewer ID
3. **Best for UI Controls**: Suitable for user-initiated pause/resume buttons

### General Notes
1. **WebSocket Limitation**: WebRTC signaling WebSocket closes after connection setup - use HTTP API for ongoing control
2. **Zero Bandwidth**: Paused streams drop all RTP packets, achieving true zero bandwidth usage
3. **Keyframe Recovery**: Resume automatically requests keyframes for fast stream recovery
4. **Per-Connection Control**: Each WebRTC connection can be controlled independently
5. **Global Control**: Use session list API to pause/resume multiple connections programmatically

## Testing Commands

### Session-Based Control (Recommended)

```bash
# List all active sessions
curl -s http://localhost:1984/api/webrtc/sessions | jq .

# Example output:
# {
#   "sessions": [
#     {
#       "session_id": "a1b2c3d4e5f6789012345678901234567890abcd",
#       "connection_id": 1,
#       "stream_source": "pk_cabin_camera",
#       "viewer_id": "viewer_1754925523259_htwpbqoa9",
#       "mode": "passive consumer",
#       "paused": false
#     }
#   ],
#   "count": 1
# }

# Pause specific session (replace session_id with actual ID from above)
curl -X POST http://localhost:1984/api/webrtc/session/pause \
  -H "Content-Type: application/json" \
  -d '{"session_id": "a1b2c3d4e5f6789012345678901234567890abcd"}'

# Resume specific session
curl -X POST http://localhost:1984/api/webrtc/session/resume \
  -H "Content-Type: application/json" \
  -d '{"session_id": "a1b2c3d4e5f6789012345678901234567890abcd"}'

# Automated script to pause all active sessions
curl -s http://localhost:1984/api/webrtc/sessions | \
  jq -r '.sessions[].session_id' | \
  while read session_id; do
    curl -X POST http://localhost:1984/api/webrtc/session/pause \
      -H "Content-Type: application/json" \
      -d "{\"session_id\": \"$session_id\"}"
    echo "Paused session: $session_id"
  done
```

### Viewer-Based Control (Legacy)

```bash
# Check active connections
curl -s http://localhost:1984/api/streams | jq .

# Test pause (replace viewer_id with actual ID from logs)
curl -X POST http://localhost:1984/api/webrtc/pause \
  -H "Content-Type: application/json" \
  -d '{"action": "pause", "viewer_id": "viewer_1754917995033_u855qv63l"}'

# Test resume
curl -X POST http://localhost:1984/api/webrtc/resume \
  -H "Content-Type: application/json" \
  -d '{"action": "resume", "viewer_id": "viewer_1754917995033_u855qv63l"}'

# Global pause (all connections)
curl -X POST http://localhost:1984/api/webrtc/pause \
  -H "Content-Type: application/json" \
  -d '{"action": "pause"}'
```

## Migration from Viewer-Based to Session-Based

If you're currently using viewer-based control, migration is simple:

1. **No client changes needed**: Session IDs are automatically generated and used
2. **Update your server scripts**: Replace viewer ID logic with session list + session control
3. **Benefits**: More reliable, no client-side configuration, better for automation

### Migration Example
```python
# Old viewer-based approach
controller.pause_viewer("viewer_1234567890_abc123")

# New session-based approach  
sessions = controller.get_active_sessions()
for session in sessions['sessions']:
    if session['stream_source'] == 'pk_cabin_camera':
        controller.pause_session(session['session_id'])
```

