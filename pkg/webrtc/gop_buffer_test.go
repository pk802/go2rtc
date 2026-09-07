package webrtc

import (
	"encoding/binary"
	"testing"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/pion/rtp"
)

// au builds an AVCC access unit from NAL header bytes: [4-byte len][hdr][pad].
func au(hdrs ...byte) *rtp.Packet {
	var p []byte
	for _, h := range hdrs {
		p = binary.BigEndian.AppendUint32(p, 3)
		p = append(p, h, 0xAA, 0xBB)
	}
	return &rtp.Packet{Payload: p}
}

func replayed(b *gopBuffer) int {
	n := 0
	b.replay(func(*rtp.Packet) { n++ })
	return n
}

// #1565: H.265 must get the same GOP capture + replay H.264 has. The NAL type
// lives in different bits, and the keyframe / inter types differ.
func TestGopBufferH265(t *testing.T) {
	b := &gopBuffer{codec: core.CodecH265}
	trailR := byte(1 << 1) // TRAIL_R (type 1) — inter
	idr := byte(19 << 1)   // IDR_W_RADL (type 19) — keyframe
	cra := byte(21 << 1)   // CRA (type 21) — keyframe
	vps := byte(32 << 1)   // VPS — parameter set, ignored for anchoring

	b.capture(au(trailR))
	if replayed(b) != 0 {
		t.Fatal("an inter slice before any keyframe must not anchor a replay")
	}
	b.capture(au(vps, idr))
	b.capture(au(trailR))
	b.capture(au(trailR))
	if got := replayed(b); got != 3 {
		t.Fatalf("replay after IDR + 2 inter = 3 units, got %d", got)
	}
	b.capture(au(cra)) // a CRA is a random-access point: new GOP, P-chain dropped
	if got := replayed(b); got != 1 {
		t.Fatalf("a CRA must start a new GOP, got %d units", got)
	}
}

// The H.264 rule is unchanged by the codec switch: IDR (5) anchors, slice (1)
// chains, and an H.265 header byte read through the H.264 mask must not.
func TestGopBufferH264Unchanged(t *testing.T) {
	b := &gopBuffer{codec: core.CodecH264}
	b.capture(au(0x65)) // IDR
	b.capture(au(0x41)) // non-IDR slice
	if got := replayed(b); got != 2 {
		t.Fatalf("H.264 IDR + P = 2 units, got %d", got)
	}
	// An unset codec (old callers) behaves as H.264.
	b2 := &gopBuffer{}
	b2.capture(au(0x65))
	if replayed(b2) != 1 {
		t.Fatal("default codec must classify as H.264")
	}
}
