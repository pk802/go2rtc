package webrtc

import (
	"encoding/binary"
	"sync"

	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/pion/rtp"
)

// gopBuffer caches the most recent decodable H.264 access units (the
// latest IDR access unit and every P-frame access unit since it). On
// resume, [replay] re-emits the cached chain through a downstream
// handler so the browser's decoder is brought up to a "live now" state
// without waiting for the camera's next scheduled keyframe.
//
// One buffer per consumer (per Sender). It captures packets AFTER
// h264.RTPDepay, which emits **AVCC** access units: one packet =
// one access unit = one or more length-prefixed NALs
// ([4-byte big-endian length][NAL header][NAL data]…). The NAL type is
// therefore at byte offset 4 (payload[4]&0x1F), NOT payload[0] — reading
// payload[0] gets the first length byte (see #406: doing so cached
// nothing and made fast-resume a silent no-op fleet-wide).
//
// We only track whole access units. RTPDepay already prepends SPS/PPS
// (from the camera or the FmtpLine parameter sets) in front of every IDR
// access unit, so a cached IDR packet is self-contained and decodable on
// its own — no separate SPS/PPS bookkeeping is needed.
//
// Memory: bounded by the GOP. For a 4 Mbps stream with 8s GOP that's
// ~4 MB; for 512 kbps sub-streams ~512 KB. No upper cap on P-frame
// count — if a producer goes berserk and never sends another IDR, the
// buffer would grow without bound. In practice cameras emit IDRs on a
// fixed schedule, so the buffer drains every GOP cycle.
type gopBuffer struct {
	mu      sync.Mutex
	idr     *rtp.Packet // latest IDR access unit (self-contained: SPS+PPS+IDR)
	pframes []*rtp.Packet
}

// capture inspects one AVCC access-unit packet and updates the buffer.
// Always safe to call regardless of pause state — the buffer is the
// source of truth for "what would the decoder need to bootstrap."
//
// The packet may bundle several NALs (e.g. SPS+PPS+IDR for a keyframe
// access unit). We scan the NAL types in-place (no allocation) to
// classify the whole access unit:
//   - contains an IDR (type 5) → new GOP: replace idr, drop the P-chain.
//   - otherwise contains a P-frame (type 1) → append to the P-chain,
//     but only once we have an IDR to anchor it.
//
// Parameter-set-only units (SPS/PPS/SEI/AUD with no slice) are ignored:
// they're already folded into the next IDR access unit by RTPDepay.
func (b *gopBuffer) capture(packet *rtp.Packet) {
	p := packet.Payload
	hasIDR, hasP := false, false
	// Walk the length-prefixed NALs. Each NAL: [4-byte length][payload].
	for off := 0; off+5 <= len(p); {
		size := int(binary.BigEndian.Uint32(p[off:]))
		switch p[off+4] & 0x1F {
		case h264.NALUTypeIFrame:
			hasIDR = true
		case h264.NALUTypePFrame:
			hasP = true
		}
		if size <= 0 {
			break // malformed length — stop scanning
		}
		off += 4 + size
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	switch {
	case hasIDR:
		b.idr = clonePacket(packet)
		b.pframes = b.pframes[:0]
	case hasP && b.idr != nil:
		b.pframes = append(b.pframes, clonePacket(packet))
	}
}

// replay re-emits the cached access units in order (IDR → P-frames)
// through the supplied handler. The handler is the rest of the consumer
// chain *after* the buffer-capture stage, i.e. h264.RTPPay → inner
// write-to-track. This bypasses the pause filter on purpose — replay is
// the one path that should fire even while the connection is still
// flagged paused (the caller flips the flag right after replay completes).
//
// Returns the number of access units replayed; 0 if the buffer hasn't
// seen an IDR yet (no decodable starting point).
func (b *gopBuffer) replay(handler func(*rtp.Packet)) int {
	b.mu.Lock()
	// Snapshot the current state so we can release the lock before
	// firing the handler (which may take real time to fragment + write
	// to the network track).
	idr := b.idr
	pframes := append([]*rtp.Packet(nil), b.pframes...)
	b.mu.Unlock()

	if idr == nil {
		return 0
	}

	count := 1
	handler(idr)
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
