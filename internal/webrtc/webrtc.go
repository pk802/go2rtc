package webrtc

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"

	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/AlexxIT/go2rtc/internal/api/ws"
	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/webrtc"
	pion "github.com/pion/webrtc/v4"
	"github.com/rs/zerolog"
)

func Init() {
	var cfg struct {
		Mod struct {
			Listen     string           `yaml:"listen"`
			Candidates []string         `yaml:"candidates"`
			IceServers []pion.ICEServer `yaml:"ice_servers"`
			Filters    webrtc.Filters   `yaml:"filters"`
		} `yaml:"webrtc"`
	}

	cfg.Mod.Listen = ":8555"
	cfg.Mod.IceServers = []pion.ICEServer{
		{URLs: []string{"stun:stun.l.google.com:19302"}},
	}

	app.LoadConfig(&cfg)

	log = app.GetLogger("webrtc")

	filters = cfg.Mod.Filters

	address, network, _ := strings.Cut(cfg.Mod.Listen, "/")
	for _, candidate := range cfg.Mod.Candidates {
		AddCandidate(network, candidate)
	}

	var err error

	// create pionAPI with custom codecs list and custom network settings
	serverAPI, err = webrtc.NewServerAPI(network, address, &filters)
	if err != nil {
		log.Error().Err(err).Caller().Send()
		return
	}

	// use same API for WebRTC server and client if no address
	clientAPI = serverAPI

	if address != "" {
		log.Info().Str("addr", cfg.Mod.Listen).Msg("[webrtc] listen")
		clientAPI, _ = webrtc.NewAPI()
	}

	pionConf := pion.Configuration{
		ICEServers:   cfg.Mod.IceServers,
		SDPSemantics: pion.SDPSemanticsUnifiedPlanWithFallback,
	}

	PeerConnection = func(active bool) (*pion.PeerConnection, error) {
		// active - client, passive - server
		if active {
			return clientAPI.NewPeerConnection(pionConf)
		} else {
			return serverAPI.NewPeerConnection(pionConf)
		}
	}

	// async WebRTC server (two API versions)
	ws.HandleFunc("webrtc", asyncHandler)
	ws.HandleFunc("webrtc/offer", asyncHandler)
	ws.HandleFunc("webrtc/candidate", candidateHandler)
	
	// pause/resume controls
	ws.HandleFunc("webrtc/pause", pauseHandler)
	ws.HandleFunc("webrtc/resume", resumeHandler)

	// sync WebRTC server (two API versions)
	api.HandleFunc("api/webrtc", syncHandler)
	
	// HTTP API for pause/resume controls
	api.HandleFunc("api/webrtc/pause", pauseHTTPHandler)
	api.HandleFunc("api/webrtc/resume", resumeHTTPHandler)
	
	// Register session-based pause/resume endpoints
	api.HandleFunc("api/webrtc/session/pause", sessionPauseHTTPHandler)
	api.HandleFunc("api/webrtc/session/resume", sessionResumeHTTPHandler)
	api.HandleFunc("api/webrtc/sessions", listSessionsHTTPHandler)

	// WebRTC client
	streams.HandleFunc("webrtc", streamsHandler)
}

var serverAPI, clientAPI *pion.API

var log zerolog.Logger

var PeerConnection func(active bool) (*pion.PeerConnection, error)

// Connection tracking for pause/resume functionality
var activeConnections = make(map[uint32]*webrtc.Conn)
var connectionsMutex sync.RWMutex

// Session ID tracking for server-controlled pause/resume
var sessionConnections = make(map[string]*webrtc.Conn)
var sessionMutex sync.RWMutex

// generateSessionID creates a unique session identifier
func generateSessionID() string {
	bytes := make([]byte, 16)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// getClientIP extracts the real client IP from the request
func getClientIP(r *http.Request) string {
	// Check for X-Forwarded-For header (most common proxy header)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// X-Forwarded-For can contain multiple IPs, take the first one
		if idx := strings.Index(xff, ","); idx != -1 {
			return strings.TrimSpace(xff[:idx])
		}
		return strings.TrimSpace(xff)
	}
	
	// Check for X-Real-IP header (Nginx)
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}
	
	// Check for Forwarded header (RFC 7239)
	if forwarded := r.Header.Get("Forwarded"); forwarded != "" {
		// Parse for=ip format
		if idx := strings.Index(forwarded, "for="); idx != -1 {
			forPart := forwarded[idx+4:]
			if endIdx := strings.Index(forPart, ";"); endIdx != -1 {
				forPart = forPart[:endIdx]
			}
			// Remove quotes if present
			forPart = strings.Trim(forPart, "\"")
			// Handle IPv6 bracket notation
			if strings.HasPrefix(forPart, "[") && strings.HasSuffix(forPart, "]") {
				forPart = forPart[1 : len(forPart)-1]
			}
			return forPart
		}
	}
	
	// Fall back to RemoteAddr
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr // Return as-is if parsing fails
	}
	return host
}

func asyncHandler(tr *ws.Transport, msg *ws.Message) (err error) {
	var stream *streams.Stream
	var mode core.Mode

	query := tr.Request.URL.Query()
	if name := query.Get("src"); name != "" {
		stream = streams.GetOrPatch(query)
		mode = core.ModePassiveConsumer
		log.Debug().Str("src", name).Msg("[webrtc] new consumer")
	} else if name = query.Get("dst"); name != "" {
		stream = streams.Get(name)
		mode = core.ModePassiveProducer
		log.Debug().Str("src", name).Msg("[webrtc] new producer")
	}

	if stream == nil {
		return errors.New(api.StreamNotFound)
	}

	var offer struct {
		Type       string           `json:"type"`
		SDP        string           `json:"sdp"`
		ICEServers []pion.ICEServer `json:"ice_servers"`
	}

	// V2 - json/object exchange, V1 - raw SDP exchange
	apiV2 := msg.Type == "webrtc"

	if apiV2 {
		if err = msg.Unmarshal(&offer); err != nil {
			return err
		}
	} else {
		offer.SDP = msg.String()
	}

	// create new PeerConnection instance
	var pc *pion.PeerConnection
	if offer.ICEServers == nil {
		pc, err = PeerConnection(false)
	} else {
		pc, err = serverAPI.NewPeerConnection(pion.Configuration{ICEServers: offer.ICEServers})
	}
	if err != nil {
		log.Error().Err(err).Caller().Send()
		return err
	}

	var sendAnswer core.Waiter

	// protect from blocking on errors
	defer sendAnswer.Done(nil)

	conn := webrtc.NewConn(pc)
	conn.Mode = mode
	conn.Protocol = "ws"
	conn.UserAgent = tr.Request.UserAgent()
	
	// Generate unique session ID for server-controlled pause/resume
	conn.SessionID = generateSessionID()
	
	// Capture client IP address
	conn.ClientIP = getClientIP(tr.Request)
	
	// Store stream source for motion detection mapping
	if src := query.Get("src"); src != "" {
		conn.StreamSource = src
	}
	
	// Store viewer ID if provided (for backward compatibility)
	if viewerID := query.Get("viewer_id"); viewerID != "" {
		conn.ViewerID = viewerID
	}
	
	// Set initial pause state if provided
	if pausedParam := query.Get("paused"); pausedParam == "true" {
		conn.Pause()
		log.Info().Str("session", conn.SessionID).Str("viewer", conn.ViewerID).Msg("[webrtc] 🔇 CONNECTION STARTED IN PAUSED STATE")
	}
	
	// Store connection in transport context for pause/resume controls
	tr.WithContext(func(ctx map[any]any) {
		ctx["webrtc_conn"] = conn
	})
	
	// Track connection for motion detection and session control
	connectionsMutex.Lock()
	activeConnections[conn.ID] = conn
	connectionsMutex.Unlock()
	
	sessionMutex.Lock()
	sessionConnections[conn.SessionID] = conn
	sessionMutex.Unlock()
	
	log.Info().Uint32("conn", conn.ID).Str("mode", conn.Mode.String()).Str("session", conn.SessionID).Str("viewer", conn.ViewerID).Str("client_ip", conn.ClientIP).Msg("[webrtc] ✅ CONNECTION TRACKED")
	conn.Listen(func(msg any) {
		switch msg := msg.(type) {
		case pion.PeerConnectionState:
			if msg != pion.PeerConnectionStateClosed {
				return
			}
			
			// Clean up connection tracking
			connectionsMutex.Lock()
			delete(activeConnections, conn.ID)
			connectionsMutex.Unlock()
			
			sessionMutex.Lock()
			delete(sessionConnections, conn.SessionID)
			sessionMutex.Unlock()
			
			switch mode {
			case core.ModePassiveConsumer:
				stream.RemoveConsumer(conn)
			case core.ModePassiveProducer:
				stream.RemoveProducer(conn)
			}

		case *pion.ICECandidate:
			if !FilterCandidate(msg) {
				return
			}
			_ = sendAnswer.Wait()

			s := msg.ToJSON().Candidate
			log.Trace().Str("candidate", s).Msg("[webrtc] local ")
			tr.Write(&ws.Message{Type: "webrtc/candidate", Value: s})
		}
	})

	log.Trace().Msgf("[webrtc] offer:\n%s", offer.SDP)

	// 1. SetOffer, so we can get remote client codecs
	if err = conn.SetOffer(offer.SDP); err != nil {
		log.Warn().Err(err).Caller().Send()
		return err
	}

	switch mode {
	case core.ModePassiveConsumer:
		// 2. AddConsumer, so we get new tracks
		if err = stream.AddConsumer(conn); err != nil {
			log.Debug().Err(err).Msg("[webrtc] add consumer")
			_ = conn.Close()
			return err
		}
	case core.ModePassiveProducer:
		stream.AddProducer(conn)
	}

	// 3. Exchange SDP without waiting all candidates
	answer, err := conn.GetAnswer()
	log.Trace().Msgf("[webrtc] answer\n%s", answer)

	if err != nil {
		log.Error().Err(err).Caller().Send()
		return err
	}

	if apiV2 {
		// Send answer with session ID for server-controlled pause/resume
		response := struct {
			Type      string `json:"type"`
			SDP       string `json:"sdp"`
			SessionID string `json:"session_id"`
		}{
			Type:      "answer",
			SDP:       answer,
			SessionID: conn.SessionID,
		}
		tr.Write(&ws.Message{Type: "webrtc", Value: response})
	} else {
		// For v1 API, send session ID as a separate message
		tr.Write(&ws.Message{Type: "webrtc/answer", Value: answer})
		tr.Write(&ws.Message{Type: "webrtc/session", Value: conn.SessionID})
	}

	sendAnswer.Done(nil)

	asyncCandidates(tr, conn)

	return nil
}

func ExchangeSDP(stream *streams.Stream, offer, desc, userAgent string) (answer string, err error) {
	pc, err := PeerConnection(false)
	if err != nil {
		log.Error().Err(err).Caller().Send()
		return
	}

	// create new webrtc instance
	conn := webrtc.NewConn(pc)
	conn.FormatName = desc
	conn.UserAgent = userAgent
	conn.Protocol = "http"
	conn.Listen(func(msg any) {
		switch msg := msg.(type) {
		case pion.PeerConnectionState:
			if msg != pion.PeerConnectionStateClosed {
				return
			}
			if conn.Mode == core.ModePassiveConsumer {
				stream.RemoveConsumer(conn)
			} else {
				stream.RemoveProducer(conn)
			}
		}
	})

	// 1. SetOffer, so we can get remote client codecs
	log.Trace().Msgf("[webrtc] offer:\n%s", offer)

	if err = conn.SetOffer(offer); err != nil {
		log.Warn().Err(err).Caller().Send()
		return
	}

	if IsConsumer(conn) {
		conn.Mode = core.ModePassiveConsumer

		// 2. AddConsumer, so we get new tracks
		if err = stream.AddConsumer(conn); err != nil {
			log.Warn().Err(err).Caller().Send()
			_ = conn.Close()
			return
		}
	} else {
		conn.Mode = core.ModePassiveProducer

		stream.AddProducer(conn)
	}

	answer, err = conn.GetCompleteAnswer(GetCandidates(), FilterCandidate)
	log.Trace().Msgf("[webrtc] answer\n%s", answer)

	if err != nil {
		log.Error().Err(err).Caller().Send()
	}

	return
}

func IsConsumer(conn *webrtc.Conn) bool {
	// if wants get video - consumer
	for _, media := range conn.GetMedias() {
		if media.Kind == core.KindVideo && media.Direction == core.DirectionSendonly {
			return true
		}
	}
	// if wants send video - producer
	for _, media := range conn.GetMedias() {
		if media.Kind == core.KindVideo && media.Direction == core.DirectionRecvonly {
			return false
		}
	}
	// if wants something - consumer
	for _, media := range conn.GetMedias() {
		if media.Direction == core.DirectionSendonly {
			return true
		}
	}
	return false
}

// Pause/Resume Handler Functions

func pauseHandler(tr *ws.Transport, msg *ws.Message) error {
	log.Info().Msg("[webrtc] 🔥 PAUSE MESSAGE RECEIVED!")
	
	// Use the global connection tracking instead of context
	connectionsMutex.RLock()
	defer connectionsMutex.RUnlock()
	
	log.Info().Int("total_connections", len(activeConnections)).Msg("[webrtc] checking active connections")
	
	pausedCount := 0
	for connID, conn := range activeConnections {
		log.Info().Uint32("conn_id", connID).Str("mode", conn.Mode.String()).Msg("[webrtc] found connection")
		if conn.Mode == core.ModePassiveConsumer { // Only pause consumers (video viewers)
			conn.Pause()
			log.Info().Uint32("conn", connID).Bool("is_paused", conn.IsPaused()).Msg("[webrtc] ✅ CONNECTION PAUSED")
			pausedCount++
		}
	}
	
	if pausedCount > 0 {
		log.Info().Int("paused", pausedCount).Msg("[webrtc] pause: completed")
	} else {
		log.Warn().Msg("[webrtc] pause: no active consumer connections found")
	}
	
	return nil
}

func resumeHandler(tr *ws.Transport, msg *ws.Message) error {
	// Use the global connection tracking instead of context
	connectionsMutex.RLock()
	defer connectionsMutex.RUnlock()
	
	resumedCount := 0
	for connID, conn := range activeConnections {
		if conn.Mode == core.ModePassiveConsumer { // Only resume consumers (video viewers)
			conn.Resume()
			log.Info().Uint32("conn", connID).Msg("[webrtc] ✅ CONNECTION RESUMED")
			resumedCount++
		}
	}
	
	if resumedCount > 0 {
		log.Info().Int("resumed", resumedCount).Msg("[webrtc] resume: completed")
	} else {
		log.Warn().Msg("[webrtc] resume: no active consumer connections found")
	}
	
	return nil
}



// HTTP API Handlers for pause/resume

func pauseHTTPHandler(w http.ResponseWriter, r *http.Request) {
	// Parse request body to get viewer_id
	var reqBody struct {
		Action   string `json:"action"`
		ViewerID string `json:"viewer_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
		log.Warn().Err(err).Msg("[webrtc] HTTP pause: invalid request body")
		reqBody.ViewerID = "" // Fall back to global pause
	}
	
	connectionsMutex.RLock()
	defer connectionsMutex.RUnlock()
	
	log.Info().Int("total_connections", len(activeConnections)).Str("target_viewer", reqBody.ViewerID).Msg("[webrtc] HTTP pause: checking connections")
	
	pausedCount := 0
	for connID, conn := range activeConnections {
		log.Info().Uint32("conn_id", connID).Str("mode", conn.Mode.String()).Str("viewer", conn.ViewerID).Msg("[webrtc] HTTP pause: found connection")
		
		// Check if this connection matches the target viewer (or pause all if no viewer_id specified)
		if conn.Mode == core.ModePassiveConsumer && (reqBody.ViewerID == "" || conn.ViewerID == reqBody.ViewerID) {
			conn.Pause()
			log.Info().Uint32("conn", connID).Str("viewer", conn.ViewerID).Msg("[webrtc] ✅ HTTP CONNECTION PAUSED")
			pausedCount++
		}
	}
	
	response := map[string]interface{}{
		"action": "pause",
		"paused_connections": pausedCount,
		"success": true,
		"viewer_id": reqBody.ViewerID,
	}
	
	if pausedCount == 0 {
		response["success"] = false
		if reqBody.ViewerID != "" {
			response["message"] = "No active consumer connections found for viewer: " + reqBody.ViewerID
		} else {
			response["message"] = "No active consumer connections found"
		}
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
	
	log.Info().Int("paused", pausedCount).Str("viewer", reqBody.ViewerID).Msg("[webrtc] HTTP pause: completed")
}

func resumeHTTPHandler(w http.ResponseWriter, r *http.Request) {
	// Parse request body to get viewer_id
	var reqBody struct {
		Action   string `json:"action"`
		ViewerID string `json:"viewer_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
		log.Warn().Err(err).Msg("[webrtc] HTTP resume: invalid request body")
		reqBody.ViewerID = "" // Fall back to global resume
	}
	
	connectionsMutex.RLock()
	defer connectionsMutex.RUnlock()
	
	log.Info().Int("total_connections", len(activeConnections)).Str("target_viewer", reqBody.ViewerID).Msg("[webrtc] HTTP resume: checking connections")
	
	resumedCount := 0
	for connID, conn := range activeConnections {
		log.Info().Uint32("conn_id", connID).Str("mode", conn.Mode.String()).Str("viewer", conn.ViewerID).Msg("[webrtc] HTTP resume: found connection")
		
		// Check if this connection matches the target viewer (or resume all if no viewer_id specified)
		if conn.Mode == core.ModePassiveConsumer && (reqBody.ViewerID == "" || conn.ViewerID == reqBody.ViewerID) {
			conn.Resume()
			log.Info().Uint32("conn", connID).Str("viewer", conn.ViewerID).Msg("[webrtc] ✅ HTTP CONNECTION RESUMED")
			resumedCount++
		}
	}
	
	response := map[string]interface{}{
		"action": "resume",
		"resumed_connections": resumedCount,
		"success": true,
		"viewer_id": reqBody.ViewerID,
	}
	
	if resumedCount == 0 {
		response["success"] = false
		if reqBody.ViewerID != "" {
			response["message"] = "No active consumer connections found for viewer: " + reqBody.ViewerID
		} else {
			response["message"] = "No active consumer connections found"
		}
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
	
	log.Info().Int("resumed", resumedCount).Str("viewer", reqBody.ViewerID).Msg("[webrtc] HTTP resume: completed")
}

// GetActiveConnections returns a copy of active connections for motion detection
func GetActiveConnections() map[uint32]*webrtc.Conn {
	connectionsMutex.RLock()
	defer connectionsMutex.RUnlock()
	
	result := make(map[uint32]*webrtc.Conn)
	for id, conn := range activeConnections {
		result[id] = conn
	}
	return result
}

// Session-based pause/resume handlers for server-controlled streams

func sessionPauseHTTPHandler(w http.ResponseWriter, r *http.Request) {
	// Handle preflight OPTIONS request
	if r.Method == "OPTIONS" {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		w.WriteHeader(http.StatusOK)
		return
	}
	
	var reqBody struct {
		SessionID string `json:"session_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	
	if reqBody.SessionID == "" {
		http.Error(w, "session_id is required", http.StatusBadRequest)
		return
	}
	
	sessionMutex.RLock()
	conn, exists := sessionConnections[reqBody.SessionID]
	sessionMutex.RUnlock()
	
	if !exists {
		http.Error(w, "Session not found", http.StatusNotFound)
		return
	}
	
	conn.Pause()
	log.Info().Str("session", reqBody.SessionID).Msg("[webrtc] Session paused")
	
	response := map[string]interface{}{
		"success":    true,
		"action":     "pause",
		"session_id": reqBody.SessionID,
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func sessionResumeHTTPHandler(w http.ResponseWriter, r *http.Request) {
	// Handle preflight OPTIONS request
	if r.Method == "OPTIONS" {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		w.WriteHeader(http.StatusOK)
		return
	}
	
	var reqBody struct {
		SessionID string `json:"session_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	
	if reqBody.SessionID == "" {
		http.Error(w, "session_id is required", http.StatusBadRequest)
		return
	}
	
	sessionMutex.RLock()
	conn, exists := sessionConnections[reqBody.SessionID]
	sessionMutex.RUnlock()
	
	if !exists {
		http.Error(w, "Session not found", http.StatusNotFound)
		return
	}
	
	conn.Resume()
	log.Info().Str("session", reqBody.SessionID).Msg("[webrtc] Session resumed")
	
	response := map[string]interface{}{
		"success":    true,
		"action":     "resume",
		"session_id": reqBody.SessionID,
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func listSessionsHTTPHandler(w http.ResponseWriter, r *http.Request) {
	sessionMutex.RLock()
	defer sessionMutex.RUnlock()
	
	sessions := make([]map[string]interface{}, 0, len(sessionConnections))
	for sessionID, conn := range sessionConnections {
		sessions = append(sessions, map[string]interface{}{
			"session_id":    sessionID,
			"connection_id": conn.ID,
			"stream_source": conn.StreamSource,
			"viewer_id":     conn.ViewerID,
			"client_ip":     conn.ClientIP,
			"mode":          conn.Mode.String(),
			"paused":        conn.IsPaused(),
		})
	}
	
	// Sort sessions by connection_id for consistent ordering
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i]["connection_id"].(uint32) < sessions[j]["connection_id"].(uint32)
	})
	
	response := map[string]interface{}{
		"sessions": sessions,
		"count":    len(sessions),
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
