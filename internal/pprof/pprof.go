// Package pprof exposes Go's runtime profiler on the API (#1568).
//
// go2rtc's CPU on an edge scales with WebRTC sessions × streams — measured
// on E000201: 131 wall sessions ≈ 37 % of one core, 0 % with none — and
// attributing that cost to depacketise / re-packetise / SRTP / GOP capture
// needs the profiler, not guesses. The API is internal (WireGuard / LAN,
// fronted by the gateway for clients), the same trust boundary as
// /api/webrtc/sessions and /api/config.
//
//	curl -o cpu.pb.gz 'http://<edge>:1984/api/debug/pprof/profile?seconds=15'
//	go tool pprof -top cpu.pb.gz
package pprof

import (
	"net/http/pprof"

	"github.com/AlexxIT/go2rtc/internal/api"
)

func Init() {
	api.HandleFunc("api/debug/pprof/", pprof.Index)
	api.HandleFunc("api/debug/pprof/profile", pprof.Profile)
	api.HandleFunc("api/debug/pprof/heap", pprof.Handler("heap").ServeHTTP)
	api.HandleFunc("api/debug/pprof/goroutine", pprof.Handler("goroutine").ServeHTTP)
	api.HandleFunc("api/debug/pprof/allocs", pprof.Handler("allocs").ServeHTTP)
}
