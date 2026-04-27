package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func testServer(t *testing.T) *Server {
	t.Helper()
	cfg := &Config{
		Password:   "testpass",
		Listen:     ":8081",
		Interfaces: []string{"eth0"},
		StateFile:  "",
		Groups: map[string]Group{
			"kids": {
				DisplayName:  "Kids",
				MACAddresses: []string{"AA:BB:CC:DD:EE:FF"},
			},
		},
	}
	state := NewState("")
	timers := NewTimerManager()
	return NewServer(cfg, state, timers)
}

func authenticate(t *testing.T, srv *Server) *http.Cookie {
	t.Helper()
	body := `{"password":"testpass"}`
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("login failed: status %d, body: %s", w.Code, w.Body.String())
	}
	cookies := w.Result().Cookies()
	for _, c := range cookies {
		if c.Name == sessionCookieName {
			return c
		}
	}
	t.Fatal("no session cookie returned")
	return nil
}

func TestLogin_Success(t *testing.T) {
	srv := testServer(t)
	cookie := authenticate(t, srv)
	if cookie.Value == "" {
		t.Error("cookie value should not be empty")
	}
	if !cookie.HttpOnly {
		t.Error("cookie should be HttpOnly")
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	srv := testServer(t)
	body := `{"password":"wrong"}`
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestLogin_InvalidJSON(t *testing.T) {
	srv := testServer(t)
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader("not json"))
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestLogin_WrongMethod(t *testing.T) {
	srv := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestAuth_Unauthenticated(t *testing.T) {
	srv := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestAuth_RootServesLoginPage(t *testing.T) {
	srv := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	// Should serve HTML (login page), not 401
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/html") {
		t.Errorf("content-type = %q, want text/html", ct)
	}
}

func TestStatus_Empty(t *testing.T) {
	srv := testServer(t)
	cookie := authenticate(t, srv)

	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var resp StatusResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if resp.BlockAll {
		t.Error("block_all should be false initially")
	}
	if len(resp.Groups) != 1 {
		t.Fatalf("groups count = %d, want 1", len(resp.Groups))
	}
	if resp.Groups[0].Blocked {
		t.Error("group should not be blocked initially")
	}
	if len(resp.Interfaces) != 1 || resp.Interfaces[0] != "eth0" {
		t.Errorf("interfaces = %v, want [eth0]", resp.Interfaces)
	}
}

func TestStatus_WithBlockedGroup(t *testing.T) {
	srv := testServer(t)
	cookie := authenticate(t, srv)

	until := time.Now().Add(1 * time.Hour)
	srv.state.SetGroupBlocked("kids", true, &until)

	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	var resp StatusResponse
	json.NewDecoder(w.Body).Decode(&resp)

	if !resp.Groups[0].Blocked {
		t.Error("kids should be blocked")
	}
	if resp.Groups[0].BlockedUntil == nil {
		t.Error("blocked_until should be set")
	}
}

func TestStatus_WithBlockAll(t *testing.T) {
	srv := testServer(t)
	cookie := authenticate(t, srv)

	until := time.Now().Add(30 * time.Minute)
	srv.state.SetBlockAll(true, &until)

	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	var resp StatusResponse
	json.NewDecoder(w.Body).Decode(&resp)

	if !resp.BlockAll {
		t.Error("block_all should be true")
	}
	if resp.BlockAllUntil == nil {
		t.Error("block_all_until should be set")
	}
}

func TestStatus_WrongMethod(t *testing.T) {
	srv := testServer(t)
	cookie := authenticate(t, srv)

	req := httptest.NewRequest(http.MethodPost, "/api/status", nil)
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestTimerManager_StartAndCancel(t *testing.T) {
	tm := NewTimerManager()
	called := make(chan struct{}, 1)

	tm.Start("test", 50*time.Millisecond, func() {
		called <- struct{}{}
	})

	// Cancel before it fires
	tm.Cancel("test")

	select {
	case <-called:
		t.Error("timer should have been cancelled")
	case <-time.After(150 * time.Millisecond):
		// Expected: timer was cancelled
	}
}

func TestTimerManager_Fires(t *testing.T) {
	tm := NewTimerManager()
	called := make(chan struct{}, 1)

	tm.Start("test", 50*time.Millisecond, func() {
		called <- struct{}{}
	})

	select {
	case <-called:
		// Expected
	case <-time.After(500 * time.Millisecond):
		t.Error("timer should have fired")
	}
}

func TestTimerManager_ReplaceTimer(t *testing.T) {
	tm := NewTimerManager()
	first := make(chan string, 1)

	tm.Start("test", 50*time.Millisecond, func() {
		first <- "first"
	})
	// Replace with a new timer
	tm.Start("test", 50*time.Millisecond, func() {
		first <- "second"
	})

	select {
	case val := <-first:
		if val != "second" {
			t.Errorf("got %q, want %q", val, "second")
		}
	case <-time.After(500 * time.Millisecond):
		t.Error("replaced timer should have fired")
	}
}

func TestTimerManager_CancelNonexistent(t *testing.T) {
	tm := NewTimerManager()
	// Should not panic
	tm.Cancel("nonexistent")
}

func TestIndex_Authenticated(t *testing.T) {
	srv := testServer(t)
	cookie := authenticate(t, srv)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/html") {
		t.Errorf("content-type = %q, want text/html", ct)
	}
}

func TestIndex_NotFound(t *testing.T) {
	srv := testServer(t)
	cookie := authenticate(t, srv)

	req := httptest.NewRequest(http.MethodGet, "/nonexistent", nil)
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}
