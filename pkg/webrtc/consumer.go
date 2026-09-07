package webrtc

import (
	"errors"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/AlexxIT/go2rtc/pkg/h265"
	"github.com/AlexxIT/go2rtc/pkg/pcm"
	"github.com/pion/rtp"
)

func (c *Conn) GetMedias() []*core.Media {
	return WithResampling(c.Medias)
}

func (c *Conn) AddTrack(media *core.Media, codec *core.Codec, track *core.Receiver) error {
	core.Assert(media.Direction == core.DirectionSendonly)

	for _, sender := range c.Senders {
		if sender.Codec == codec {
			sender.Bind(track)
			return nil
		}
	}

	switch c.Mode {
	case core.ModePassiveConsumer: // video/audio for browser
	case core.ModeActiveProducer: // go2rtc as WebRTC client (backchannel)
	case core.ModePassiveProducer: // WebRTC/WHIP
	default:
		panic(core.Caller())
	}

	localTrack := c.GetSenderTrack(media.ID)
	if localTrack == nil {
		return errors.New("webrtc: can't get track")
	}

	payloadType := codec.PayloadType

	sender := core.NewSender(media, codec)

	// Innermost handler: write RTP to the WebRTC local track.
	// No pause check here. The pause filter is layered on top so the
	// H.264 path can route GOP-buffer replays through writeToTrack
	// without going through the filter (the only path allowed to fire
	// while paused=true).
	sender.Handler = func(packet *rtp.Packet) {
		c.Send += packet.MarshalSize()
		// Important to send with remote PayloadType.
		_ = localTrack.WriteRTP(payloadType, packet)
	}

	// True when the codec-specific switch case below builds its own
	// pause filter (H.264 and H.265 — the filter doubles as the GOP-buffer
	// capture point). For all other codecs we add a generic pause filter
	// as the last wrap below.
	pauseFilterApplied := false

	switch track.Codec.Name {
	case core.CodecH264:
		// Build chain inner→outer:
		//   write-to-track → RTPPay → capture+pause-filter → RTPDepay
		//
		// Capture happens *after* depay, so it sees whole NAL units
		// (one packet = one NAL, with NAL header in payload[0]). That
		// keeps the buffer compact and lets us inspect NAL type with
		// a single byte read.
		sender.Handler = h264.RTPPay(1200, sender.Handler)

		// Snapshot the chain at this point. replayHandler is
		// "h264.RTPPay → write-to-track" — what the GOP buffer plays
		// back into on resume. It deliberately doesn't include the
		// pause filter, so replay always reaches the browser.
		replayHandler := sender.Handler

		gop := &gopBuffer{codec: core.CodecH264}
		c.addReplayCallback(func() {
			gop.replay(replayHandler)
		})

		// Capture + pause filter. Always capture so the buffer is
		// always current; only forward to the rest of the chain when
		// not paused.
		sender.Handler = func(packet *rtp.Packet) {
			gop.capture(packet)
			if c.paused.Load() {
				return
			}
			replayHandler(packet)
		}
		pauseFilterApplied = true

		if track.Codec.IsRTP() {
			sender.Handler = h264.RTPDepay(track.Codec, sender.Handler)
		} else {
			sender.Handler = h264.RepairAVCC(track.Codec, sender.Handler)
		}

	case core.CodecH265:
		// Same chain as H.264 (#1565): write-to-track → RTPPay →
		// capture+pause-filter → RTPDepay. Without the buffer a resume
		// started mid-GOP; the browser's HEVC decoder is hardware-only and,
		// measured on Chrome 152 / macOS, does not recover from that — the
		// tile stayed black for good while bytes kept arriving.
		sender.Handler = h265.RTPPay(1200, sender.Handler)
		replayHandler := sender.Handler

		gop := &gopBuffer{codec: core.CodecH265}
		c.addReplayCallback(func() {
			gop.replay(replayHandler)
		})

		sender.Handler = func(packet *rtp.Packet) {
			gop.capture(packet)
			if c.paused.Load() {
				return
			}
			replayHandler(packet)
		}
		pauseFilterApplied = true

		if track.Codec.IsRTP() {
			sender.Handler = h265.RTPDepay(track.Codec, sender.Handler)
		} else {
			sender.Handler = h265.RepairAVCC(track.Codec, sender.Handler)
		}

	case core.CodecPCMA, core.CodecPCMU, core.CodecPCM, core.CodecPCML:
		// Fix audio quality https://github.com/AlexxIT/WebRTC/issues/500
		// should be before ResampleToG711, because it will be called last
		sender.Handler = pcm.RepackG711(false, sender.Handler)

		if codec.ClockRate == 0 {
			if codec.Name == core.CodecPCM || codec.Name == core.CodecPCML {
				codec.Name = core.CodecPCMA
			}
			codec.ClockRate = 8000
			sender.Handler = pcm.TranscodeHandler(codec, track.Codec, sender.Handler)
		}
	}

	// Generic pause filter for any codec path that didn't install its
	// own. Drops packets when c.paused is true. H.264 sets
	// pauseFilterApplied=true above because its filter is fused with
	// the GOP-buffer capture step; that one path is the only one that
	// also bypasses pause for replay traffic.
	if !pauseFilterApplied {
		next := sender.Handler
		sender.Handler = func(packet *rtp.Packet) {
			if c.paused.Load() {
				return
			}
			next(packet)
		}
	}

	// TODO: rewrite this dirty logic
	// maybe not best solution, but ActiveProducer connected before AddTrack
	if c.Mode != core.ModeActiveProducer {
		sender.Bind(track)
	} else {
		sender.HandleRTP(track)
	}

	c.Senders = append(c.Senders, sender)
	return nil
}
