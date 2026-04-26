package main

import (
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"
)

//go:embed web/index.html
var webFS embed.FS

const sessionCookieName = "nft_blocker_session"

type Server struct {
	cfg      *Config
	state    *State
	timers   *TimerManager
	sessions sync.Map // token string -> struct{}
}

func NewServer(cfg *Config, state *State, timers *TimerManager) *Server {
	return &Server{
		cfg:    cfg,
		state:  state,
		timers: timers,
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/login", s.handleLogin)
	mux.HandleFunc("/api/status", s.requireAuth(s.handleStatus))
	mux.HandleFunc("/api/block", s.requireAuth(s.handleBlock))
	mux.HandleFunc("/api/unblock", s.requireAuth(s.handleUnblock))
	mux.HandleFunc("/api/block-all", s.requireAuth(s.handleBlockAll))
	mux.HandleFunc("/api/unblock-all", s.requireAuth(s.handleUnblockAll))
	mux.HandleFunc("/", s.requireAuth(s.handleIndex))
	return mux
}

func (s *Server) generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func (s *Server) isAuthenticated(r *http.Request) bool {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return false
	}
	_, ok := s.sessions.Load(cookie.Value)
	return ok
}

func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.isAuthenticated(r) {
			if r.URL.Path != "/" && r.URL.Path != "/login" {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}
			// Serve login page for root
			s.serveLoginPage(w)
			return
		}
		next(w, r)
	}
}

func (s *Server) serveLoginPage(w http.ResponseWriter) {
	data, err := webFS.ReadFile("web/index.html")
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(data)
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
		return
	}

	if subtle.ConstantTimeCompare([]byte(req.Password), []byte(s.cfg.Password)) != 1 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"error":"wrong password"}`)
		return
	}

	token, err := s.generateToken()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	s.sessions.Store(token, struct{}{})

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   86400 * 30, // 30 days
	})

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprint(w, `{"ok":true}`)
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	data, err := webFS.ReadFile("web/index.html")
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(data)
}

type GroupStatusResponse struct {
	Name         string   `json:"name"`
	DisplayName  string   `json:"display_name"`
	MACAddresses []string `json:"mac_addresses"`
	Blocked      bool     `json:"blocked"`
	BlockedUntil *string  `json:"blocked_until,omitempty"` // RFC3339 or nil
}

type StatusResponse struct {
	Groups   []GroupStatusResponse `json:"groups"`
	BlockAll bool                  `json:"block_all"`
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	snap := s.state.Snapshot()
	resp := StatusResponse{
		BlockAll: snap.BlockAll,
	}

	for name, group := range s.cfg.Groups {
		gs := GroupStatusResponse{
			Name:         name,
			DisplayName:  group.DisplayName,
			MACAddresses: group.MACAddresses,
			Blocked:      false,
		}
		if st, ok := snap.Groups[name]; ok {
			gs.Blocked = st.Blocked
			if st.BlockedUntil != nil && !st.BlockedUntil.IsZero() {
				t := st.BlockedUntil.Format(time.RFC3339)
				gs.BlockedUntil = &t
			}
		}
		resp.Groups = append(resp.Groups, gs)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleBlock(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Group    string `json:"group"`
		Duration string `json:"duration"` // "15m", "1h", "2h", "12h", "forever"
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
		return
	}

	group, ok := s.cfg.Groups[req.Group]
	if !ok {
		http.Error(w, `{"error":"unknown group"}`, http.StatusBadRequest)
		return
	}

	// Cancel any existing timer
	s.timers.Cancel(req.Group)

	// Apply nftables block
	if err := BlockGroup(req.Group, group.MACAddresses); err != nil {
		log.Printf("ERROR blocking group %s: %v", req.Group, err)
		http.Error(w, fmt.Sprintf(`{"error":"nftables: %s"}`, err), http.StatusInternalServerError)
		return
	}

	var until *time.Time
	if req.Duration != "forever" && req.Duration != "" {
		dur, err := time.ParseDuration(req.Duration)
		if err != nil {
			http.Error(w, `{"error":"invalid duration"}`, http.StatusBadRequest)
			return
		}
		t := time.Now().Add(dur)
		until = &t

		groupName := req.Group
		s.timers.Start(groupName, dur, func() {
			log.Printf("Timer expired for group %s, unblocking", groupName)
			if err := UnblockGroup(groupName); err != nil {
				log.Printf("ERROR auto-unblocking group %s: %v", groupName, err)
			}
			s.state.SetGroupBlocked(groupName, false, nil)
			if err := s.state.Save(); err != nil {
				log.Printf("ERROR saving state: %v", err)
			}
		})
	}

	s.state.SetGroupBlocked(req.Group, true, until)
	if err := s.state.Save(); err != nil {
		log.Printf("ERROR saving state: %v", err)
	}

	log.Printf("Blocked group %s (duration: %s)", req.Group, req.Duration)
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprint(w, `{"ok":true}`)
}

func (s *Server) handleUnblock(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Group string `json:"group"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
		return
	}

	if _, ok := s.cfg.Groups[req.Group]; !ok {
		http.Error(w, `{"error":"unknown group"}`, http.StatusBadRequest)
		return
	}

	s.timers.Cancel(req.Group)

	if err := UnblockGroup(req.Group); err != nil {
		log.Printf("ERROR unblocking group %s: %v", req.Group, err)
		http.Error(w, fmt.Sprintf(`{"error":"nftables: %s"}`, err), http.StatusInternalServerError)
		return
	}

	s.state.SetGroupBlocked(req.Group, false, nil)
	if err := s.state.Save(); err != nil {
		log.Printf("ERROR saving state: %v", err)
	}

	log.Printf("Unblocked group %s", req.Group)
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprint(w, `{"ok":true}`)
}

func (s *Server) handleBlockAll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := BlockAllTraffic(s.cfg.Interface); err != nil {
		log.Printf("ERROR blocking all traffic: %v", err)
		http.Error(w, fmt.Sprintf(`{"error":"nftables: %s"}`, err), http.StatusInternalServerError)
		return
	}

	s.state.SetBlockAll(true)
	if err := s.state.Save(); err != nil {
		log.Printf("ERROR saving state: %v", err)
	}

	log.Printf("Blocked all traffic on %s", s.cfg.Interface)
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprint(w, `{"ok":true}`)
}

func (s *Server) handleUnblockAll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := UnblockAllTraffic(); err != nil {
		log.Printf("ERROR unblocking all traffic: %v", err)
		http.Error(w, fmt.Sprintf(`{"error":"nftables: %s"}`, err), http.StatusInternalServerError)
		return
	}

	s.state.SetBlockAll(false)
	if err := s.state.Save(); err != nil {
		log.Printf("ERROR saving state: %v", err)
	}

	log.Printf("Unblocked all traffic")
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprint(w, `{"ok":true}`)
}

// TimerManager handles per-group auto-unblock timers.
type TimerManager struct {
	mu     sync.Mutex
	timers map[string]*time.Timer
}

func NewTimerManager() *TimerManager {
	return &TimerManager{
		timers: make(map[string]*time.Timer),
	}
}

func (tm *TimerManager) Start(group string, duration time.Duration, fn func()) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	if existing, ok := tm.timers[group]; ok {
		existing.Stop()
	}
	tm.timers[group] = time.AfterFunc(duration, func() {
		fn()
		tm.mu.Lock()
		delete(tm.timers, group)
		tm.mu.Unlock()
	})
}

func (tm *TimerManager) Cancel(group string) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	if t, ok := tm.timers[group]; ok {
		t.Stop()
		delete(tm.timers, group)
	}
}
