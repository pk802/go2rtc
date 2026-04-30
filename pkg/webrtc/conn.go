package webrtc

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
)

type Conn struct {
	core.Connection
	core.Listener

	Mode core.Mode `json:"mode"`

	pc *webrtc.PeerConnection

	offer  string
	closed core.Waiter

	// Pause/resume functionality
	paused   atomic.Bool
	ringType atomic.Value // "main" or "sub" for stream quality switching

	// Per-consumer GOP-buffer replay hooks. AddTrack registers one
	// callback per H.264 sender; Resume() iterates and fires each so
	// the cached IDR + post-IDR P-chain reaches the browser BEFORE the
	// pause flag is lifted. Camera-agnostic fast-resume — works even
	// when the upstream camera ignores PLI.
	replayMu        sync.Mutex
	replayCallbacks []func()

	// Stream source for motion detection mapping
	StreamSource string `json:"stream_source,omitempty"`
	
	// Viewer ID for per-viewer control (client-generated)
	ViewerID string `json:"viewer_id,omitempty"`
	
	// Session ID for server-controlled pause/resume
	SessionID string `json:"session_id,omitempty"`
	
	// Client IP address for session tracking
	ClientIP string `json:"client_ip,omitempty"`
}

func NewConn(pc *webrtc.PeerConnection) *Conn {
	c := &Conn{
		Connection: core.Connection{
			ID:         core.NewID(),
			FormatName: "webrtc",
			Transport:  pc,
		},
		pc: pc,
	}

	pc.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		// last candidate will be empty
		if candidate != nil {
			c.Fire(candidate)
		}
	})

	pc.OnDataChannel(func(channel *webrtc.DataChannel) {
		c.Fire(channel)
	})

	pc.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		if state != webrtc.ICEConnectionStateChecking {
			return
		}
		pc.SCTP().Transport().ICETransport().OnSelectedCandidatePairChange(
			func(pair *webrtc.ICECandidatePair) {
				// fix situation when candidate pair changes multiple times
				if i := strings.IndexByte(c.Protocol, '+'); i > 0 {
					c.Protocol = c.Protocol[:i]
				}
				c.Protocol += "+" + pair.Remote.Protocol.String()
				c.RemoteAddr = fmt.Sprintf(
					"%s:%d %s", sanitizeIP6(pair.Remote.Address), pair.Remote.Port, pair.Remote.Typ,
				)
				if pair.Remote.RelatedAddress != "" {
					c.RemoteAddr += fmt.Sprintf(
						" %s:%d", sanitizeIP6(pair.Remote.RelatedAddress), pair.Remote.RelatedPort,
					)
				}
			},
		)
	})

	pc.OnTrack(func(remote *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		media, codec := c.getMediaCodec(remote)
		if media == nil {
			return
		}

		track, err := c.GetTrack(media, codec)
		if err != nil {
			return
		}

		switch c.Mode {
		case core.ModePassiveProducer, core.ModeActiveProducer:
			// replace the theoretical list of codecs with the actual list of codecs
			if len(media.Codecs) > 1 {
				media.Codecs = []*core.Codec{codec}
			}
		}

		if c.Mode == core.ModePassiveProducer && remote.Kind() == webrtc.RTPCodecTypeVideo {
			go func() {
				pkts := []rtcp.Packet{&rtcp.PictureLossIndication{MediaSSRC: uint32(remote.SSRC())}}
				for range time.NewTicker(time.Second * 2).C {
					if err := pc.WriteRTCP(pkts); err != nil {
						return
					}
				}
			}()
		}

		for {
			b := make([]byte, ReceiveMTU)
			n, _, err := remote.Read(b)
			if err != nil {
				return
			}

			c.Recv += n

			packet := &rtp.Packet{}
			if err := packet.Unmarshal(b[:n]); err != nil {
				return
			}

			if len(packet.Payload) == 0 {
				continue
			}

			track.WriteRTP(packet)
		}
	})

	// OK connection:
	// 15:01:46 ICE connection state changed: checking
	// 15:01:46 peer connection state changed: connected
	// 15:01:54 peer connection state changed: disconnected
	// 15:02:20 peer connection state changed: failed
	//
	// Fail connection:
	// 14:53:08 ICE connection state changed: checking
	// 14:53:39 peer connection state changed: failed
	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		c.Fire(state)

		switch state {
		case webrtc.PeerConnectionStateConnected:
			for _, sender := range c.Senders {
				sender.Start()
			}
		case webrtc.PeerConnectionStateDisconnected, webrtc.PeerConnectionStateFailed, webrtc.PeerConnectionStateClosed:
			// disconnect event comes earlier, than failed
			// but it comes only for success connections
			_ = c.Close()
		}
	})

	return c
}

func (c *Conn) MarshalJSON() ([]byte, error) {
	return json.Marshal(c.Connection)
}

func (c *Conn) Close() error {
	c.closed.Done(nil)
	return c.pc.Close()
}

func (c *Conn) AddCandidate(candidate string) error {
	// pion uses only candidate value from json/object candidate struct
	return c.pc.AddICECandidate(webrtc.ICECandidateInit{Candidate: candidate})
}

func (c *Conn) GetSenderTrack(mid string) *Track {
	if tr := c.getTranseiver(mid); tr != nil {
		if s := tr.Sender(); s != nil {
			if t := s.Track().(*Track); t != nil {
				return t
			}
		}
	}
	return nil
}

func (c *Conn) getTranseiver(mid string) *webrtc.RTPTransceiver {
	for _, tr := range c.pc.GetTransceivers() {
		if tr.Mid() == mid {
			return tr
		}
	}
	return nil
}

func (c *Conn) getMediaCodec(remote *webrtc.TrackRemote) (*core.Media, *core.Codec) {
	for _, tr := range c.pc.GetTransceivers() {
		// search Transeiver for this TrackRemote
		if tr.Receiver() == nil || tr.Receiver().Track() != remote {
			continue
		}

		// search Media for this MID
		for _, media := range c.Medias {
			if media.ID != tr.Mid() || media.Direction != core.DirectionRecvonly {
				continue
			}

			// search codec for this PayloadType
			for _, codec := range media.Codecs {
				if codec.PayloadType != uint8(remote.PayloadType()) {
					continue
				}
				return media, codec
			}
		}
	}

	// fix moment when core.ModePassiveProducer or core.ModeActiveProducer
	// sends new codec with new payload type to same media
	// check GetTrack
	panic(core.Caller())

	return nil, nil
}

func sanitizeIP6(host string) string {
	if strings.IndexByte(host, ':') > 0 {
		return "[" + host + "]"
	}
	return host
}

// Pause/Resume Methods

// Pause pauses the WebRTC stream by setting the paused flag.
// Live RTP packets continue to arrive at the receiver and are still
// captured into the per-consumer GOP buffer (so the next Resume can
// replay them), but the consumer's chain drops them at the pause filter
// instead of forwarding to the browser.
func (c *Conn) Pause() {
	c.paused.Store(true)
}

// Resume resumes the WebRTC stream. Order matters:
//
//  1. Replay each consumer's cached IDR + post-IDR P-chain to the
//     browser. This bypasses the pause filter on purpose — the replay
//     handler is the slice of the sender chain *after* the filter
//     (h264.RTPPay → write-to-track), so it fires regardless of the
//     paused flag.
//  2. Clear the paused flag so live RTP frames arriving via the normal
//     producer-goroutine path begin forwarding.
//  3. Send a PLI to the camera. Belt-and-braces — if the camera honors
//     PLI we'll get a fresh IDR a few hundred ms later, which the
//     decoder will use as a clean reference. If the camera ignores
//     PLI (most don't), the replay we just did has already brought the
//     decoder current, so resume completes within decode latency.
//
// Net effect: browser shows a frame within ~50-150 ms of un-pause,
// independent of camera GOP and independent of PLI honor.
func (c *Conn) Resume() {
	c.replayMu.Lock()
	callbacks := append([]func(){}, c.replayCallbacks...)
	c.replayMu.Unlock()

	for _, cb := range callbacks {
		cb()
	}
	c.paused.Store(false)
	c.requestKeyframe()
}

// addReplayCallback registers a function that gets fired by Resume()
// before the pause flag is cleared. consumer.go calls this once per
// H.264 sender it sets up.
func (c *Conn) addReplayCallback(cb func()) {
	c.replayMu.Lock()
	c.replayCallbacks = append(c.replayCallbacks, cb)
	c.replayMu.Unlock()
}

// IsPaused returns the current pause state
func (c *Conn) IsPaused() bool {
	return c.paused.Load()
}

// SetRingType sets the stream quality type
func (c *Conn) SetRingType(ringType string) {
	c.ringType.Store(ringType)
}

// GetRingType returns the current ring type
func (c *Conn) GetRingType() string {
	if val := c.ringType.Load(); val != nil {
		if s, ok := val.(string); ok {
			return s
		}
	}
	return "main" // default
}

// requestKeyframe sends a PLI (Picture Loss Indication) to request a keyframe
func (c *Conn) requestKeyframe() {
	for _, receiver := range c.pc.GetReceivers() {
		if receiver.Track() == nil {
			continue
		}
		
		params := []rtcp.Packet{
			&rtcp.PictureLossIndication{
				MediaSSRC: uint32(receiver.Track().SSRC()),
			},
		}
		
		if err := c.pc.WriteRTCP(params); err != nil {
			// Don't log errors as they're common during connection setup/teardown
		}
	}
}
