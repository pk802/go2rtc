package webrtc

import (
	"sync"

	"github.com/pion/rtp"
)

// gopBuffer caches the most recent decodable H.264 chain (latest IDR,
// any SPS/PPS that arrived, and every P-frame since that IDR). On
// resume, [replay] re-emits the cached chain through a downstream
// handler so the browser's decoder is brought up to a "live now" state
// without waiting for the camera's next scheduled keyframe.
//
// One buffer per consumer (per Sender). It captures NAL units AFTER
// h264.RTPDepay has reassembled them, so each entry is exactly one
// whole NAL — payload[0] holds the H.264 NAL header byte; the low 5
// bits are the NAL type (1 = non-IDR slice, 5 = IDR, 7 = SPS, 8 = PPS,
// 6 = SEI, 9 = AUD).
//
// Memory: bounded by the GOP. For a 4 Mbps stream with 8s GOP that's
// ~4 MB; for 512 kbps sub-streams ~512 KB. No upper cap on P-frame
// count — if a producer goes berserk and never sends another IDR, the
// buffer would grow without bound. In practice cameras emit IDRs on a
// fixed schedule, so the buffer drains every GOP cycle.
type gopBuffer struct {
	mu      sync.Mutex
	sps     *rtp.Packet // latest SPS, kept across IDRs
	pps     *rtp.Packet // latest PPS, kept across IDRs
	idr     *rtp.Packet // most recent IDR; cleared when a new one arrives
	pframes []*rtp.Packet
}

// capture inspects an H.264 NAL unit and updates the buffer. Always safe
// to call regardless of pause state — the buffer is the source of truth
// for "what would the decoder need to bootstrap."
//
// On a new IDR (NAL type 5) the post-IDR P-frame chain is reset — the
// previous chain is no longer reachable from the new reference frame.
// SPS (7) and PPS (8) are kept "stickily" because the decoder needs
// them in front of any IDR to interpret it; cameras usually emit them
// alongside every IDR but not always.
func (b *gopBuffer) capture(packet *rtp.Packet) {
	if len(packet.Payload) == 0 {
		return
	}
	nalType := packet.Payload[0] & 0x1F

	b.mu.Lock()
	defer b.mu.Unlock()

	switch nalType {
	case 5: // IDR slice — start of a new GOP
		b.idr = clonePacket(packet)
		b.pframes = b.pframes[:0]
	case 7: // SPS
		b.sps = clonePacket(packet)
	case 8: // PPS
		b.pps = clonePacket(packet)
	case 1: // Non-IDR slice (P-frame, B-frame)
		// Only buffer P-frames that follow a known IDR. Without an
		// IDR in front, the chain is undecodable so there's no point
		// caching.
		if b.idr != nil {
			b.pframes = append(b.pframes, clonePacket(packet))
		}
	}
	// Other NAL types (6 = SEI, 9 = AUD, 24+ = aggregation/fragmentation
	// formats that depay has already unwrapped) are ignored. SEI is
	// optional metadata; AUD is an access unit delimiter the decoder
	// derives implicitly; aggregations don't reach this layer.
}

// replay sends the cached chain through the supplied handler in
// decoder-friendly order: SPS → PPS → IDR → P-frames. The handler is
// the rest of the consumer chain *after* the buffer-capture stage,
// i.e. h264.RTPPay → inner write-to-track. This bypasses the pause
// filter on purpose — replay is the one path that should fire even
// while the connection is still flagged paused (the caller flips the
// flag right after replay completes).
//
// Returns the number of NALs replayed; 0 if the buffer hasn't seen an
// IDR yet (no decodable starting point).
func (b *gopBuffer) replay(handler func(*rtp.Packet)) int {
	b.mu.Lock()
	// Snapshot the current state so we can release the lock before
	// firing the handler (which may take real time to fragment + write
	// to the network track).
	sps := b.sps
	pps := b.pps
	idr := b.idr
	pframes := append([]*rtp.Packet(nil), b.pframes...)
	b.mu.Unlock()

	if idr == nil {
		return 0
	}

	count := 0
	if sps != nil {
		handler(sps)
		count++
	}
	if pps != nil {
		handler(pps)
		count++
	}
	handler(idr)
	count++
	for _, p := range pframes {
		handler(p)
		count++
	}
	return count
}

// reset clears all buffered state. Called when the underlying media
// pipeline tears down so a stale buffer can't leak into a fresh
// session.
func (b *gopBuffer) reset() {
	b.mu.Lock()
	b.sps = nil
	b.pps = nil
	b.idr = nil
	b.pframes = b.pframes[:0]
	b.mu.Unlock()
}

// clonePacket makes an independent copy of an RTP packet so the buffer
// owns its memory. The producer reuses the underlying byte slices for
// subsequent frames — without cloning, captured packets would be
// corrupted by the time replay reads them.
func clonePacket(packet *rtp.Packet) *rtp.Packet {
	if packet == nil {
		return nil
	}
	cp := *packet
	if packet.Payload != nil {
		cp.Payload = append([]byte(nil), packet.Payload...)
	}
	if packet.CSRC != nil {
		cp.CSRC = append([]uint32(nil), packet.CSRC...)
	}
	return &cp
}
